package optimization

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/capacity"
	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/gitopsdrift"
	"k8s-aiops.local/backend/internal/imagepolicy"
	"k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/netpolicy"
	"k8s-aiops.local/backend/internal/policy"
)

// ClusterLister fetches one Kubernetes List resource and returns its items as
// raw JSON. It is the only cluster access the collector needs, so a fake can
// be supplied in tests (no real cluster, no network).
type ClusterLister interface {
	List(ctx context.Context, clusterID int64, path string) ([]json.RawMessage, error)
}

// MetricsSource supplies p95 usage samples for one pod container. The FinOps
// collector uses it to right-size requests against actual usage. A nil source
// means no usage data: the FinOps collector degrades to request/limit-only
// collection and the analyzer simply reports no over-provisioning (per
// finops.Recommend, which skips containers without a usage signal).
type MetricsSource interface {
	PodContainerP95(ctx context.Context, clusterID int64, namespace, pod, container string) (cpuNanocores, memBytes int64, ok bool)
}

// UsageSeriesSource returns a node's CPU or memory usage time series (raw
// units: nanocores for cpu, bytes for memory) over [from,to]. The capacity
// collector aggregates per-node series into a cluster capacity-trend input. A
// nil source means no usage history: capacity collection degrades to
// capacity-only (the analyzer reports no trend, only current utilization when a
// sample happens to be supplied out-of-band).
type UsageSeriesSource interface {
	NodeUsageSeries(ctx context.Context, clusterID int64, node, metric string, from, to time.Time) ([]capacity.Sample, error)
}

// Collector turns live cluster data into the observation bundles the M61-M63
// (and M70) analyzers consume. It never mutates cluster state (ADR 0004): it
// only reads and maps. The control-plane component flags checked by CIS are
// NOT reachable through the Kubernetes API, so CollectCIS leaves that domain
// empty by design; callers that can supply component flags (node/manifest
// access) may populate cis.Inputs.Components directly and pass the bundle to
// cis.Evaluate.
type Collector struct {
	lister  ClusterLister
	metrics MetricsSource
	usage   UsageSeriesSource
}

// NewCollector builds a Collector. lister is required; metrics and usage may be
// nil (FinOps right-sizing and capacity trend respectively degrade gracefully).
func NewCollector(lister ClusterLister, metrics MetricsSource, usage UsageSeriesSource) *Collector {
	return &Collector{lister: lister, metrics: metrics, usage: usage}
}

// kubernetesLister is the production ClusterLister: it talks to the read-only
// kubernetes Gateway exactly like the rest of the platform (kubernetes.Service
// calls the same Gateway.Get under the hood).
type kubernetesLister struct {
	gateway kubernetes.Gateway
	creds   kubernetes.CredentialSource
}

func newKubernetesLister(gateway kubernetes.Gateway, creds kubernetes.CredentialSource) ClusterLister {
	return kubernetesLister{gateway: gateway, creds: creds}
}

// NewKubernetesLister builds a ClusterLister over the read-only kubernetes
// Gateway. gateway and creds are the same values passed to kubernetes.NewService.
func NewKubernetesLister(gateway kubernetes.Gateway, creds kubernetes.CredentialSource) ClusterLister {
	return newKubernetesLister(gateway, creds)
}

func (k kubernetesLister) List(ctx context.Context, clusterID int64, path string) ([]json.RawMessage, error) {
	_, kubeconfig, err := k.creds.Access(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	body, err := k.gateway.Get(ctx, clusterID, kubeconfig, path, nil, 10<<20)
	if err != nil {
		return nil, err
	}
	var env struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

// CollectCIS builds the CIS observation bundle from live cluster data:
// workload security contexts, RBAC bindings (with resolved role rules), and
// namespace Pod Security Admission labels.
func (c *Collector) CollectCIS(ctx context.Context, clusterID int64) (cis.Inputs, error) {
	in := cis.Inputs{}

	podItems, err := c.lister.List(ctx, clusterID, "/api/v1/pods")
	if err != nil {
		return in, err
	}
	for _, raw := range podItems {
		var p podRaw
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		wl := cis.WorkloadSecurity{Kind: "Pod", Namespace: p.Metadata.Namespace, Name: p.Metadata.Name, UID: p.Metadata.UID}
		hostPaths := make(map[string]bool, len(p.Spec.Volumes))
		for _, v := range p.Spec.Volumes {
			if v.HostPath != nil {
				hostPaths[v.Name] = true
			}
		}
		for _, ctr := range p.Spec.Containers {
			cs := cis.ContainerSecurity{
				Name:        ctr.Name,
				HostNetwork: p.Spec.HostNetwork,
				HostPID:     p.Spec.HostPID,
				HostIPC:     p.Spec.HostIPC,
			}
			if sc := ctr.SecurityContext; sc != nil {
				cs.Privileged = sc.Privileged
				cs.AllowPrivilegeEscalation = sc.AllowPrivilegeEscalation
				cs.RunAsNonRoot = sc.RunAsNonRoot
				cs.RunAsUser = sc.RunAsUser
				cs.ReadOnlyRootFilesystem = sc.ReadOnlyRootFilesystem
				if sc.Capabilities != nil {
					cs.CapabilitiesDrop = sc.Capabilities.Drop
				}
			}
			hp := 0
			for _, vm := range ctr.VolumeMounts {
				if hostPaths[vm.Name] {
					hp++
				}
			}
			cs.HostPathVolumes = hp
			wl.Containers = append(wl.Containers, cs)
		}
		in.Workloads = append(in.Workloads, wl)
	}

	bindings, err := c.collectRBAC(ctx, clusterID)
	if err != nil {
		return in, err
	}
	in.Bindings = bindings

	nsItems, err := c.lister.List(ctx, clusterID, "/api/v1/namespaces")
	if err != nil {
		return in, err
	}
	for _, raw := range nsItems {
		var n kubernetes.Namespace
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		in.Namespaces = append(in.Namespaces, cis.NamespacePodSecurity{
			Name:    n.Metadata.Name,
			UID:     n.Metadata.UID,
			Enforce: n.Metadata.Labels["pod-security.kubernetes.io/enforce"],
			Audit:   n.Metadata.Labels["pod-security.kubernetes.io/audit"],
			Warn:    n.Metadata.Labels["pod-security.kubernetes.io/warn"],
		})
	}

	return in, nil
}

func (c *Collector) collectRBAC(ctx context.Context, clusterID int64) ([]cis.RBACBinding, error) {
	bindings := make([]cis.RBACBinding, 0)

	// Cluster-scoped: clusterrolebindings reference clusterroles.
	crbItems, err := c.lister.List(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings")
	if err != nil {
		return nil, err
	}
	crItems, err := c.lister.List(ctx, clusterID, "/apis/rbac.authorization.k8s.io/v1/clusterroles")
	if err != nil {
		return nil, err
	}
	clusterRoleRules := make(map[string][]cis.PolicyRule, len(crItems))
	for _, raw := range crItems {
		var r roleRaw
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		clusterRoleRules[r.Metadata.Name] = toPolicyRules(r.Rules)
	}
	for _, raw := range crbItems {
		var b roleBindingRaw
		if json.Unmarshal(raw, &b) != nil {
			continue
		}
		bindings = append(bindings, cis.RBACBinding{
			Kind:      "ClusterRoleBinding",
			Name:      b.Metadata.Name,
			UID:       b.Metadata.UID,
			RoleName:  b.RoleRef.Name,
			RoleKind:  b.RoleRef.Kind,
			RoleRules: clusterRoleRules[b.RoleRef.Name],
			Subjects:  toSubjects(b.Subjects),
		})
	}

	// Namespaced: rolebindings reference namespaced roles.
	nsItems, err := c.lister.List(ctx, clusterID, "/api/v1/namespaces")
	if err != nil {
		return nil, err
	}
	for _, raw := range nsItems {
		var n kubernetes.Namespace
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		ns := n.Metadata.Name
		rbPath := "/apis/rbac.authorization.k8s.io/v1/namespaces/" + ns + "/rolebindings"
		rolePath := "/apis/rbac.authorization.k8s.io/v1/namespaces/" + ns + "/roles"
		rbItems, rerr := c.lister.List(ctx, clusterID, rbPath)
		roleItems, roleErr := c.lister.List(ctx, clusterID, rolePath)
		if rerr != nil || roleErr != nil {
			continue // namespace may be mid-deletion; skip rather than fail the whole scan
		}
		roleRules := make(map[string][]cis.PolicyRule, len(roleItems))
		for _, raw := range roleItems {
			var r roleRaw
			if json.Unmarshal(raw, &r) != nil {
				continue
			}
			roleRules[r.Metadata.Name] = toPolicyRules(r.Rules)
		}
		for _, raw := range rbItems {
			var b roleBindingRaw
			if json.Unmarshal(raw, &b) != nil {
				continue
			}
			bindings = append(bindings, cis.RBACBinding{
				Kind:      "RoleBinding",
				Namespace: ns,
				Name:      b.Metadata.Name,
				UID:       b.Metadata.UID,
				RoleName:  b.RoleRef.Name,
				RoleKind:  b.RoleRef.Kind,
				RoleRules: roleRules[b.RoleRef.Name],
				Subjects:  toSubjects(b.Subjects),
			})
		}
	}

	return bindings, nil
}

// deprecatedResourcePaths are the API list paths scanned for deprecated /
// removed apiVersions. A 404 (resource type not installed) is treated as
// "nothing to scan" and skipped, so adding new paths is safe.
var deprecatedResourcePaths = []string{
	"/api/v1/pods",
	"/api/v1/services",
	"/api/v1/endpoints",
	"/api/v1/resourcequotas",
	"/api/v1/limitranges",
	"/api/v1/persistentvolumes",
	"/api/v1/persistentvolumeclaims",
	"/api/v1/configmaps",
	"/api/v1/secrets",
	"/api/v1/serviceaccounts",
	"/apis/apps/v1/deployments",
	"/apis/apps/v1/statefulsets",
	"/apis/apps/v1/daemonsets",
	"/apis/apps/v1/replicasets",
	"/apis/batch/v1/jobs",
	"/apis/batch/v1/cronjobs",
	"/apis/networking.k8s.io/v1/ingresses",
	"/apis/networking.k8s.io/v1/networkpolicies",
	"/apis/autoscaling/v2/horizontalpodautoscalers",
	"/apis/policy/v1/poddisruptionbudgets",
	"/apis/rbac.authorization.k8s.io/v1/clusterroles",
	"/apis/rbac.authorization.k8s.io/v1/clusterrolebindings",
	"/apis/rbac.authorization.k8s.io/v1/roles",
	"/apis/rbac.authorization.k8s.io/v1/rolebindings",
	"/apis/storage.k8s.io/v1/storageclasses",
}

// CollectDeprecatedAPI scans the common resource types and returns every
// object's apiVersion/kind so the analyzer can flag deprecated or removed
// versions relative to a target Kubernetes minor version.
func (c *Collector) CollectDeprecatedAPI(ctx context.Context, clusterID int64) ([]deprecatedapi.ResourceObject, error) {
	objects := make([]deprecatedapi.ResourceObject, 0)
	for _, path := range deprecatedResourcePaths {
		items, err := c.lister.List(ctx, clusterID, path)
		if err != nil {
			continue // resource type not installed / not listable here
		}
		for _, raw := range items {
			var o objectRaw
			if json.Unmarshal(raw, &o) != nil {
				continue
			}
			if o.Kind == "" || o.Metadata.Name == "" {
				continue
			}
			objects = append(objects, deprecatedapi.ResourceObject{
				APIVersion: o.APIVersion,
				Kind:       o.Kind,
				Namespace:  o.Metadata.Namespace,
				Name:       o.Metadata.Name,
				UID:        o.Metadata.UID,
			})
		}
	}
	return objects, nil
}

// CollectNetPolicy builds the network posture observation bundle: namespaces,
// pods (labels plus declared container ports), Services and NetworkPolicies.
// Everything is a read-only List; no probe traffic is ever generated.
func (c *Collector) CollectNetPolicy(ctx context.Context, clusterID int64) (netpolicy.Inputs, error) {
	in := netpolicy.Inputs{}

	nsItems, err := c.lister.List(ctx, clusterID, "/api/v1/namespaces")
	if err != nil {
		return in, err
	}
	for _, raw := range nsItems {
		var n kubernetes.Namespace
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		in.Namespaces = append(in.Namespaces, netpolicy.NamespaceInfo{
			Name:   n.Metadata.Name,
			UID:    n.Metadata.UID,
			Labels: n.Metadata.Labels,
		})
	}

	podItems, err := c.lister.List(ctx, clusterID, "/api/v1/pods")
	if err != nil {
		return in, err
	}
	for _, raw := range podItems {
		var p podRaw
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		info := netpolicy.PodInfo{
			Namespace:   p.Metadata.Namespace,
			Name:        p.Metadata.Name,
			UID:         p.Metadata.UID,
			Labels:      p.Metadata.Labels,
			HostNetwork: p.Spec.HostNetwork,
		}
		for _, ctr := range p.Spec.Containers {
			for _, port := range ctr.Ports {
				info.Ports = append(info.Ports, netpolicy.ContainerPort{
					Name:          port.Name,
					ContainerPort: port.ContainerPort,
					Protocol:      port.Protocol,
				})
			}
		}
		in.Pods = append(in.Pods, info)
	}

	svcItems, err := c.lister.List(ctx, clusterID, "/api/v1/services")
	if err != nil {
		return in, err
	}
	for _, raw := range svcItems {
		var s serviceRaw
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		svc := netpolicy.ServiceInfo{
			Namespace:    s.Metadata.Namespace,
			Name:         s.Metadata.Name,
			UID:          s.Metadata.UID,
			Type:         s.Spec.Type,
			Selector:     s.Spec.Selector,
			ClusterIP:    s.Spec.ClusterIP,
			ExternalName: s.Spec.ExternalName,
		}
		for _, port := range s.Spec.Ports {
			svc.Ports = append(svc.Ports, netpolicy.ServicePort{
				Name:       port.Name,
				Port:       port.Port,
				TargetPort: intOrString(port.TargetPort),
				Protocol:   port.Protocol,
				NodePort:   port.NodePort,
			})
		}
		in.Services = append(in.Services, svc)
	}

	// NetworkPolicy support is optional: a cluster without the networking API
	// (or without permission to list it) still yields a useful reachability
	// report, so a failure here is not fatal.
	if npItems, npErr := c.lister.List(ctx, clusterID, "/apis/networking.k8s.io/v1/networkpolicies"); npErr == nil {
		for _, raw := range npItems {
			var np networkPolicyRaw
			if json.Unmarshal(raw, &np) != nil {
				continue
			}
			policy := netpolicy.Policy{
				Namespace:   np.Metadata.Namespace,
				Name:        np.Metadata.Name,
				UID:         np.Metadata.UID,
				PolicyTypes: np.Spec.PolicyTypes,
			}
			if sel := toSelector(np.Spec.PodSelector); sel != nil {
				policy.PodSelector = *sel
			}
			policy.Ingress = toRules(np.Spec.Ingress, true)
			policy.Egress = toRules(np.Spec.Egress, false)
			in.Policies = append(in.Policies, policy)
		}
	}

	return in, nil
}

// imagePolicySources are the workload collections scanned for container image
// references. Controllers are read instead of their Pods so one image is
// counted once per workload rather than once per replica; bare Pods are picked
// up separately (see CollectImagePolicy) only when they have no owner.
//
// CronJobs are deliberately omitted: the Jobs they create are listed here
// already, so including both would double-count the same image.
var imagePolicySources = []struct {
	kind string
	path string
}{
	{"Deployment", "/apis/apps/v1/deployments"},
	{"StatefulSet", "/apis/apps/v1/statefulsets"},
	{"DaemonSet", "/apis/apps/v1/daemonsets"},
	{"Job", "/apis/batch/v1/jobs"},
}

// gitopsDriftSources are the resource kinds scanned for GitOps configuration
// drift. They are the high-value, low-false-positive kinds: workloads whose
// spec is directly applied by GitOps, plus ConfigMaps/Secrets whose data is.
//
// Services are intentionally excluded: their spec.clusterIP is assigned by the
// API server, so a last-applied-configuration with an empty clusterIP always
// appears to drift from the live object, producing noise. Jobs/CronJobs are
// excluded because they are usually generated by controllers.
var gitopsDriftSources = []struct {
	kind string
	path string
}{
	{"Deployment", "/apis/apps/v1/deployments"},
	{"StatefulSet", "/apis/apps/v1/statefulsets"},
	{"DaemonSet", "/apis/apps/v1/daemonsets"},
	{"ConfigMap", "/api/v1/configmaps"},
	{"Secret", "/api/v1/secrets"},
}

// CollectImagePolicy builds the image supply-chain observation bundle: every
// container image referenced by a workload controller, plus the images of
// standalone Pods. Init containers are included because they ship the same
// supply-chain risk as app containers.
//
// Everything is a read-only List against the API server. No registry is ever
// contacted and no manifest is pulled.
func (c *Collector) CollectImagePolicy(ctx context.Context, clusterID int64) (imagepolicy.Inputs, error) {
	in := imagepolicy.Inputs{}

	for _, src := range imagePolicySources {
		items, err := c.lister.List(ctx, clusterID, src.path)
		if err != nil {
			return imagepolicy.Inputs{}, err
		}
		for _, raw := range items {
			var w workloadImageRaw
			if json.Unmarshal(raw, &w) != nil {
				continue
			}
			in.Usages = append(in.Usages, imageUsages(src.kind, w.Metadata.Namespace, w.Metadata.Name, w.Spec.Template.Spec)...)
		}
	}

	podItems, err := c.lister.List(ctx, clusterID, "/api/v1/pods")
	if err != nil {
		return imagepolicy.Inputs{}, err
	}
	for _, raw := range podItems {
		var p workloadImageRaw
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		// Pods created by a controller are skipped; their images were already
		// collected from the controller's pod template above.
		if len(p.Metadata.OwnerReferences) > 0 {
			continue
		}
		in.Usages = append(in.Usages, imageUsages("Pod", p.Metadata.Namespace, p.Metadata.Name, p.Spec.podSpecImageRaw)...)
	}

	return in, nil
}

// CollectGitOpsDrift builds the GitOps configuration-drift observation bundle:
// every scanned workload/ConfigMap/Secret resource with its
// last-applied-configuration annotation (when present) and its live spec/data,
// plus the namespaces detected as GitOps-managed (so resources without the
// annotation there can be reported as unmanaged).
//
// Everything is a read-only List against the API server. No Git provider is
// ever contacted and no manifest is re-applied.
func (c *Collector) CollectGitOpsDrift(ctx context.Context, clusterID int64) (gitopsdrift.Inputs, error) {
	in := gitopsdrift.Inputs{}

	nsItems, err := c.lister.List(ctx, clusterID, "/api/v1/namespaces")
	if err != nil {
		return in, err
	}
	managed := make([]string, 0)
	for _, raw := range nsItems {
		var n kubernetes.Namespace
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		if namespaceManagedByGitOps(n.Metadata.Annotations) {
			managed = append(managed, n.Metadata.Name)
		}
	}
	in.ManagedNamespaces = managed

	for _, src := range gitopsDriftSources {
		items, err := c.lister.List(ctx, clusterID, src.path)
		if err != nil {
			return gitopsdrift.Inputs{}, err
		}
		for _, raw := range items {
			var o gitopsObjectRaw
			if json.Unmarshal(raw, &o) != nil {
				continue
			}
			mr := gitopsdrift.ManagedResource{
				Kind:      firstNonEmpty(o.Kind, src.kind),
				Namespace: o.Metadata.Namespace,
				Name:      o.Metadata.Name,
				UID:       o.Metadata.UID,
			}
			switch {
			case o.Metadata.Annotations[gitopsdriftLastApplied] != "":
				mr.Manager = gitopsdrift.ManagerKubectl
			case hasAnnotationPrefix(o.Metadata.Annotations, "fluxcd.io"):
				mr.Manager = gitopsdrift.ManagerFlux
			case hasAnnotationPrefix(o.Metadata.Annotations, "argocd.argoproj.io"):
				mr.Manager = gitopsdrift.ManagerArgoCD
			}
			if ann := o.Metadata.Annotations[gitopsdriftLastApplied]; ann != "" {
				// The annotation value is a JSON string whose content is the
				// applied manifest, so decode it once to the inner object; if
				// it is not a JSON string (already raw object text), keep it
				// verbatim. Either way AppliedConfig is the manifest object so
				// the analyzer can unmarshal it directly.
				decoded := json.RawMessage(ann)
				var inner json.RawMessage
				if json.Unmarshal([]byte(ann), &inner) == nil {
					decoded = inner
				}
				mr.AppliedConfig = decoded
			}
			if len(o.Spec) > 0 {
				mr.LiveBody = o.Spec
			} else if len(o.Data) > 0 {
				mr.LiveBody = o.Data
			}
			in.Resources = append(in.Resources, mr)
		}
	}

	return in, nil
}

// gitopsdriftLastApplied is the annotation key whose value is the manifest a
// GitOps tool last applied.
const gitopsdriftLastApplied = "kubectl.kubernetes.io/last-applied-configuration"

// CollectCapacity builds the capacity-trend observation bundle: the sum of
// node allocatable CPU/memory capacity, plus the aggregate node usage time
// series over the last 24h (when a UsageSeriesSource is configured). The usage
// series are point-wise summed per minute across nodes into a single cluster
// series, so the analyzer projects total-cluster saturation rather than any
// single node.
//
// Everything is a read-only List against the API server plus read-only metrics
// history queries. Nothing is mutated and no metrics backend is written to.
func (c *Collector) CollectCapacity(ctx context.Context, clusterID int64) (capacity.Inputs, error) {
	in := capacity.Inputs{HorizonDays: capacity.DefaultHorizonDays}

	nodeItems, err := c.lister.List(ctx, clusterID, "/api/v1/nodes")
	if err != nil {
		return in, err
	}
	var cpuCap, memCap int64
	nodeNames := make([]string, 0, len(nodeItems))
	for _, raw := range nodeItems {
		var n nodeRaw
		if json.Unmarshal(raw, &n) != nil {
			continue
		}
		nodeNames = append(nodeNames, n.Metadata.Name)
		cpuCap += parseCPU(n.Status.Allocatable["cpu"])
		memCap += parseMem(n.Status.Allocatable["memory"])
	}
	in.CPU.Capacity = cpuCap
	in.Memory.Capacity = memCap

	if c.usage == nil {
		// No usage history: the analyzer reports current utilization only
		// when a sample is supplied out-of-band; with none it reports capacity.
		return in, nil
	}

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	cpuAgg := map[int64]float64{} // minute bucket -> sum nanocores
	memAgg := map[int64]float64{}
	for _, name := range nodeNames {
		if cpuPts, cpuErr := c.usage.NodeUsageSeries(ctx, clusterID, name, metricshistory.MetricCPU, from, to); cpuErr == nil {
			for _, s := range cpuPts {
				cpuAgg[bucket(s.Timestamp)] += s.Value
			}
		}
		if memPts, memErr := c.usage.NodeUsageSeries(ctx, clusterID, name, metricshistory.MetricMemory, from, to); memErr == nil {
			for _, s := range memPts {
				memAgg[bucket(s.Timestamp)] += s.Value
			}
		}
	}
	in.CPU.Samples = toSamples(cpuAgg)
	in.Memory.Samples = toSamples(memAgg)
	return in, nil
}

// metricsHistoryUsageSource is the production UsageSeriesSource. It reads node
// usage samples from the metrics history store (populated by the M30
// metricshistory collector) and maps them to capacity.Sample values.
type metricsHistoryUsageSource struct {
	svc    *metricshistory.Service
	window time.Duration
}

// NewNodeUsageSource builds a UsageSeriesSource over the metrics history
// service. window is how far back to look for samples (the metricshistory
// default retention is 7d; 24h is a sensible capacity-trend window).
func NewNodeUsageSource(svc *metricshistory.Service, window time.Duration) UsageSeriesSource {
	if window <= 0 {
		window = 24 * time.Hour
	}
	return metricsHistoryUsageSource{svc: svc, window: window}
}

func (m metricsHistoryUsageSource) NodeUsageSeries(ctx context.Context, clusterID int64, node, metric string, from, to time.Time) ([]capacity.Sample, error) {
	resp, err := m.svc.Query(ctx, metricshistory.SeriesQuery{
		ClusterID:    clusterID,
		ResourceKind: metricshistory.ResourceNode,
		ResourceName: node,
		MetricName:   metric,
		From:         from,
		To:           to,
		Limit:        1440,
	})
	if err != nil {
		return nil, err
	}
	out := make([]capacity.Sample, 0, len(resp.Points))
	for _, p := range resp.Points {
		out = append(out, capacity.Sample{Timestamp: p.SourceTimestamp, Value: float64(p.Value)})
	}
	return out, nil
}

// nodeRaw decodes the identity and allocatable capacity of a Node.
type nodeRaw struct {
	Metadata struct {
		Name string `json:"name"`
		UID  string `json:"uid"`
	} `json:"metadata"`
	Status struct {
		Allocatable map[string]string `json:"allocatable"`
	} `json:"status"`
}

// parseCPU parses a Kubernetes CPU quantity (e.g. "4" cores, "4000m") into
// nanocores, returning 0 when empty or unparseable.
func parseCPU(raw string) int64 {
	if raw == "" {
		return 0
	}
	q := finops.QuantityFromResourceMap(map[string]string{"cpu": raw}, nil)
	if q.CPURequest != finops.Unset {
		return q.CPURequest
	}
	return 0
}

// parseMem parses a Kubernetes memory quantity (e.g. "16Gi") into bytes,
// returning 0 when empty or unparseable.
func parseMem(raw string) int64 {
	if raw == "" {
		return 0
	}
	q := finops.QuantityFromResourceMap(map[string]string{"memory": raw}, nil)
	if q.MemRequest != finops.Unset {
		return q.MemRequest
	}
	return 0
}

// bucket rounds a timestamp down to the minute so per-node samples that share
// a collection tick align into a single cluster aggregate point.
func bucket(t time.Time) int64 {
	return t.Truncate(time.Minute).Unix()
}

// toSamples turns a minute-bucketed aggregate map into a time-ordered slice.
func toSamples(agg map[int64]float64) []capacity.Sample {
	if len(agg) == 0 {
		return nil
	}
	keys := make([]int64, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := make([]capacity.Sample, 0, len(keys))
	for _, k := range keys {
		out = append(out, capacity.Sample{Timestamp: time.Unix(k, 0).UTC(), Value: agg[k]})
	}
	return out
}

// policyWorkloadSources are the workload controllers scanned for policy
// evaluation. Controllers are read instead of their Pods so each manifest is
// checked once rather than once per replica; bare Pods are picked up
// separately (see CollectPolicy) only when they have no owner.
//
// Jobs/CronJobs are omitted: their containers are short-lived batch tasks
// where probes and long-running resource baselines do not apply.
var policyWorkloadSources = []struct {
	kind string
	path string
}{
	{"Deployment", "/apis/apps/v1/deployments"},
	{"StatefulSet", "/apis/apps/v1/statefulsets"},
	{"DaemonSet", "/apis/apps/v1/daemonsets"},
}

// CollectPolicy builds the policy-as-code observation bundle: every workload
// controller's pod template (containers with their resource requests/limits,
// security context and probes, plus the pod-level host access flags), plus
// standalone Pods. This is what the policy analyzer evaluates against its
// declarative rule set.
//
// Everything is a read-only List against the API server. No policy engine is
// invoked here and nothing is ever mutated.
func (c *Collector) CollectPolicy(ctx context.Context, clusterID int64) (policy.Inputs, error) {
	in := policy.Inputs{}

	for _, src := range policyWorkloadSources {
		items, err := c.lister.List(ctx, clusterID, src.path)
		if err != nil {
			return policy.Inputs{}, err
		}
		for _, raw := range items {
			var w policyWorkloadRaw
			if json.Unmarshal(raw, &w) != nil {
				continue
			}
			if w.Metadata.Name == "" {
				continue
			}
			in.Workloads = append(in.Workloads, policyWorkloadFromTemplate(src.kind, w))
		}
	}

	// Standalone Pods (no owner) are evaluated as bare workloads: their
	// container specs live directly under .spec rather than .spec.template.
	podItems, err := c.lister.List(ctx, clusterID, "/api/v1/pods")
	if err != nil {
		return policy.Inputs{}, err
	}
	for _, raw := range podItems {
		var p policyPodRaw
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		if len(p.Metadata.OwnerReferences) > 0 || p.Metadata.Name == "" {
			continue
		}
		in.Workloads = append(in.Workloads, policyWorkloadFromSpec("Pod", p.Metadata.Namespace, p.Metadata.Name, p.Metadata.UID, p.Spec))
	}

	return in, nil
}

// policyWorkloadFromTemplate maps one decoded controller into the analyzer's
// workload model, reading the pod template under spec.template.spec.
func policyWorkloadFromTemplate(kind string, w policyWorkloadRaw) policy.WorkloadPolicy {
	return policyWorkloadFromSpec(kind, w.Metadata.Namespace, w.Metadata.Name, w.Metadata.UID, w.Spec.Template.Spec)
}

// policyWorkloadFromSpec maps one decoded pod spec into the analyzer's
// workload model.
func policyWorkloadFromSpec(kind, namespace, name, uid string, spec policyPodSpecRaw) policy.WorkloadPolicy {
	wl := policy.WorkloadPolicy{
		Kind:        kind,
		Namespace:   namespace,
		Name:        name,
		UID:         uid,
		HostNetwork: spec.HostNetwork,
		HostPID:     spec.HostPID,
		HostIPC:     spec.HostIPC,
	}
	for _, ctr := range spec.Containers {
		cp := policy.ContainerPolicy{Name: ctr.Name}
		if ctr.Resources.Requests != nil {
			cp.CPURequest = ctr.Resources.Requests["cpu"] != ""
			cp.MemoryRequest = ctr.Resources.Requests["memory"] != ""
		}
		if ctr.Resources.Limits != nil {
			cp.HasResourceLimits = ctr.Resources.Limits["cpu"] != "" || ctr.Resources.Limits["memory"] != ""
		}
		if sc := ctr.SecurityContext; sc != nil {
			cp.Privileged = sc.Privileged
			cp.AllowPrivilegeEscalation = sc.AllowPrivilegeEscalation
			cp.RunAsNonRoot = sc.RunAsNonRoot
		}
		// A probe key is present when its JSON pointer decodes to non-nil.
		cp.LivenessProbe = ctr.LivenessProbe != nil
		cp.ReadinessProbe = ctr.ReadinessProbe != nil
		cp.StartupProbe = ctr.StartupProbe != nil
		wl.Containers = append(wl.Containers, cp)
	}
	return wl
}

// policyWorkloadRaw decodes a controller whose pod template carries the fields
// the policy analyzer needs. The embedded policyPodSpecRaw covers bare Pods
// (spec.containers / spec.hostNetwork at the top level) — encoding/json
// promotes the exported fields of the embedded struct — while Template.Spec
// holds the controller pod template.
type policyWorkloadRaw struct {
	Metadata struct {
		Namespace       string `json:"namespace"`
		Name            string `json:"name"`
		UID             string `json:"uid"`
		OwnerReferences []struct {
			Kind string `json:"kind"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		policyPodSpecRaw
		Template struct {
			Spec policyPodSpecRaw `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

// policyPodRaw is the bare-Pod variant: the pod spec sits directly under
// .spec, with ownerReferences under .metadata.
type policyPodRaw struct {
	Metadata struct {
		Namespace       string `json:"namespace"`
		Name            string `json:"name"`
		UID             string `json:"uid"`
		OwnerReferences []struct {
			Kind string `json:"kind"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec policyPodSpecRaw `json:"spec"`
}

// policyPodSpecRaw is the policy-relevant subset of a PodSpec. Probes are
// captured as raw JSON pointers so their presence (a non-nil pointer) is
// observable without decoding the probe bodies.
type policyPodSpecRaw struct {
	HostNetwork bool `json:"hostNetwork"`
	HostPID     bool `json:"hostPID"`
	HostIPC     bool `json:"hostIPC"`
	Containers  []struct {
		Name      string `json:"name"`
		Resources struct {
			Requests map[string]string `json:"requests"`
			Limits   map[string]string `json:"limits"`
		} `json:"resources"`
		SecurityContext *struct {
			Privileged               *bool `json:"privileged"`
			AllowPrivilegeEscalation *bool `json:"allowPrivilegeEscalation"`
			RunAsNonRoot             *bool `json:"runAsNonRoot"`
		} `json:"securityContext"`
		LivenessProbe  *json.RawMessage `json:"livenessProbe"`
		ReadinessProbe *json.RawMessage `json:"readinessProbe"`
		StartupProbe   *json.RawMessage `json:"startupProbe"`
	} `json:"containers"`
}

// namespaceManagedByGitOps reports whether a namespace is managed by a GitOps
// controller, detected from Flux or Argo CD annotations.
func namespaceManagedByGitOps(annotations map[string]string) bool {
	return hasAnnotationPrefix(annotations, "fluxcd.io") || hasAnnotationPrefix(annotations, "argocd.argoproj.io")
}

// hasAnnotationPrefix reports whether any annotation key carries the given
// suffix-domain prefix (e.g. "fluxcd.io" matches "kustomize.toolkit.fluxcd.io/...").
func hasAnnotationPrefix(annotations map[string]string, prefix string) bool {
	for k := range annotations {
		if strings.Contains(k, prefix) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// imageUsages flattens one pod spec into the per-container usage records the
// analyzer consumes.
func imageUsages(kind, namespace, name string, spec podSpecImageRaw) []imagepolicy.ImageUsage {
	containers := make([]containerImageRaw, 0, len(spec.InitContainers)+len(spec.Containers))
	containers = append(containers, spec.InitContainers...)
	containers = append(containers, spec.Containers...)

	usages := make([]imagepolicy.ImageUsage, 0, len(containers))
	for _, ctr := range containers {
		if ctr.Image == "" {
			continue
		}
		img := imagepolicy.ParseImage(ctr.Image)
		img.PullPolicy = ctr.ImagePullPolicy
		usages = append(usages, imagepolicy.ImageUsage{
			Image: img,
			Container: imagepolicy.ContainerRef{
				Namespace:    namespace,
				WorkloadKind: kind,
				WorkloadName: name,
				Container:    ctr.Name,
			},
		})
	}
	return usages
}

// CollectFinOps builds the FinOps container-input bundle from live workload
// specs (requests/limits/replicas) plus observed p95 usage when a MetricsSource
// is configured.
func (c *Collector) CollectFinOps(ctx context.Context, clusterID int64) ([]finops.ContainerInput, error) {
	pods := c.listPodLite(ctx, clusterID)

	inputs := make([]finops.ContainerInput, 0)

	depItems, err := c.lister.List(ctx, clusterID, "/apis/apps/v1/deployments")
	if err != nil {
		return nil, err
	}
	for _, raw := range depItems {
		var d kubernetes.Deployment
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		replicas := int32(1)
		if d.Spec.Replicas != nil {
			replicas = *d.Spec.Replicas
		}
		for _, ctr := range d.Spec.Template.Spec.Containers {
			inputs = append(inputs, c.finopsInput(ctx, clusterID, "Deployment", d.Metadata.Namespace, d.Metadata.Name, replicas, ctr, pods, d.Spec.Selector.MatchLabels))
		}
	}

	stsItems, err := c.lister.List(ctx, clusterID, "/apis/apps/v1/statefulsets")
	if err != nil {
		return nil, err
	}
	for _, raw := range stsItems {
		var s kubernetes.StatefulSet
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		replicas := int32(1)
		if s.Spec.Replicas != nil {
			replicas = *s.Spec.Replicas
		}
		for _, ctr := range s.Spec.Template.Spec.Containers {
			inputs = append(inputs, c.finopsInput(ctx, clusterID, "StatefulSet", s.Metadata.Namespace, s.Metadata.Name, replicas, ctr, pods, s.Spec.Selector.MatchLabels))
		}
	}

	dsItems, err := c.lister.List(ctx, clusterID, "/apis/apps/v1/daemonsets")
	if err != nil {
		return nil, err
	}
	for _, raw := range dsItems {
		var ds kubernetes.DaemonSet
		if json.Unmarshal(raw, &ds) != nil {
			continue
		}
		replicas := ds.Status.DesiredNumberScheduled
		if replicas == 0 {
			replicas = 1
		}
		for _, ctr := range ds.Spec.Template.Spec.Containers {
			inputs = append(inputs, c.finopsInput(ctx, clusterID, "DaemonSet", ds.Metadata.Namespace, ds.Metadata.Name, replicas, ctr, pods, ds.Spec.Selector.MatchLabels))
		}
	}

	return inputs, nil
}

func (c *Collector) finopsInput(ctx context.Context, clusterID int64, kind, namespace, name string, replicas int32, ctr kubernetes.WorkloadContainer, pods []podLite, selector map[string]string) finops.ContainerInput {
	// QuantityFromResourceMap parses both requests (from the first arg) and
	// limits (from the second arg) into one Quantity value.
	q := finops.QuantityFromResourceMap(ctr.Resources.Requests, ctr.Resources.Limits)
	in := finops.ContainerInput{
		ClusterID:     clusterID,
		Namespace:     namespace,
		WorkloadKind:  kind,
		WorkloadName:  name,
		ContainerName: ctr.Name,
		Requests:      q,
		Limits:        q,
		Replicas:      replicas,
	}
	if c.metrics != nil {
		cpu, mem, ok := c.podP95(ctx, clusterID, namespace, pods, selector, ctr.Name)
		if ok {
			in.CPUUsageP95 = cpu
			in.MemUsageP95 = mem
		}
	}
	return in
}

func (c *Collector) podP95(ctx context.Context, clusterID int64, namespace string, pods []podLite, selector map[string]string, container string) (int64, int64, bool) {
	if c.metrics == nil {
		return 0, 0, false
	}
	var bestCPU, bestMem int64
	matched := false
	for _, p := range pods {
		if p.Namespace != namespace || !matchLabels(p.Labels, selector) {
			continue
		}
		matched = true
		cpu, mem, ok := c.metrics.PodContainerP95(ctx, clusterID, namespace, p.Name, container)
		if !ok {
			continue
		}
		if cpu > bestCPU {
			bestCPU = cpu
		}
		if mem > bestMem {
			bestMem = mem
		}
	}
	if !matched {
		return 0, 0, false
	}
	return bestCPU, bestMem, true
}

func (c *Collector) listPodLite(ctx context.Context, clusterID int64) []podLite {
	items, err := c.lister.List(ctx, clusterID, "/api/v1/pods")
	if err != nil {
		return nil
	}
	out := make([]podLite, 0, len(items))
	for _, raw := range items {
		var p podRaw
		if json.Unmarshal(raw, &p) != nil {
			continue
		}
		out = append(out, podLite{Namespace: p.Metadata.Namespace, Name: p.Metadata.Name, Labels: p.Metadata.Labels})
	}
	return out
}

// --- raw decode helpers (fields the typed kubernetes structs omit) ---

// containerImageRaw is the image-bearing subset of a container spec.
type containerImageRaw struct {
	Name            string `json:"name"`
	Image           string `json:"image"`
	ImagePullPolicy string `json:"imagePullPolicy"`
}

// podSpecImageRaw is the image-bearing subset of a PodSpec.
type podSpecImageRaw struct {
	Containers     []containerImageRaw `json:"containers"`
	InitContainers []containerImageRaw `json:"initContainers"`
}

// workloadImageRaw decodes both controller objects, where the containers live
// under .spec.template.spec, and bare Pods, where they live directly under
// .spec. The embedded podSpecImageRaw covers the latter: encoding/json
// promotes the exported fields of an embedded struct, so "spec.containers"
// decodes into Spec.Containers.
type workloadImageRaw struct {
	Metadata struct {
		Namespace       string `json:"namespace"`
		Name            string `json:"name"`
		OwnerReferences []struct {
			Kind string `json:"kind"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		podSpecImageRaw
		Template struct {
			Spec podSpecImageRaw `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
}

type podRaw struct {
	Metadata struct {
		Namespace string            `json:"namespace"`
		Name      string            `json:"name"`
		UID       string            `json:"uid"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		HostNetwork bool `json:"hostNetwork"`
		HostPID     bool `json:"hostPID"`
		HostIPC     bool `json:"hostIPC"`
		Containers  []struct {
			Name            string `json:"name"`
			SecurityContext *struct {
				Privileged               *bool  `json:"privileged"`
				AllowPrivilegeEscalation *bool  `json:"allowPrivilegeEscalation"`
				RunAsNonRoot             *bool  `json:"runAsNonRoot"`
				RunAsUser                *int64 `json:"runAsUser"`
				ReadOnlyRootFilesystem   *bool  `json:"readOnlyRootFilesystem"`
				Capabilities             *struct {
					Drop []string `json:"drop"`
				} `json:"capabilities"`
			} `json:"securityContext"`
			VolumeMounts []struct {
				Name string `json:"name"`
			} `json:"volumeMounts"`
			Ports []struct {
				Name          string `json:"name"`
				ContainerPort int32  `json:"containerPort"`
				Protocol      string `json:"protocol"`
			} `json:"ports"`
		} `json:"containers"`
		Volumes []struct {
			Name     string `json:"name"`
			HostPath *struct {
				Path string `json:"path"`
			} `json:"hostPath"`
		} `json:"volumes"`
	} `json:"spec"`
}

type podLite struct {
	Namespace string
	Name      string
	Labels    map[string]string
}

type roleRaw struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Rules []struct {
		APIGroups []string `json:"apiGroups"`
		Resources []string `json:"resources"`
		Verbs     []string `json:"verbs"`
	} `json:"rules"`
}

type roleBindingRaw struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		UID       string `json:"uid"`
	} `json:"metadata"`
	RoleRef struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"roleRef"`
	Subjects []struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"subjects"`
}

type objectRaw struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		UID       string `json:"uid"`
	} `json:"metadata"`
}

// gitopsObjectRaw decodes the fields the GitOps drift collector needs from any
// scanned kind: identity, annotations (for last-applied-configuration and GitOps
// manager detection), and the live spec/data as raw JSON for byte-faithful
// comparison against the applied manifest.
type gitopsObjectRaw struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Namespace   string            `json:"namespace"`
		Name        string            `json:"name"`
		UID         string            `json:"uid"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
	Data json.RawMessage `json:"data"`
}

func toPolicyRules(rules []struct {
	APIGroups []string `json:"apiGroups"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}) []cis.PolicyRule {
	out := make([]cis.PolicyRule, 0, len(rules))
	for _, r := range rules {
		out = append(out, cis.PolicyRule{APIGroups: r.APIGroups, Resources: r.Resources, Verbs: r.Verbs})
	}
	return out
}

func toSubjects(subjects []struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}) []cis.RBACSubject {
	out := make([]cis.RBACSubject, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, cis.RBACSubject{Kind: s.Kind, Name: s.Name, Namespace: s.Namespace})
	}
	return out
}

// --- network posture raw decode helpers ---

type serviceRaw struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		UID       string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		Type         string            `json:"type"`
		Selector     map[string]string `json:"selector"`
		ClusterIP    string            `json:"clusterIP"`
		ExternalName string            `json:"externalName"`
		Ports        []struct {
			Name       string          `json:"name"`
			Port       int32           `json:"port"`
			TargetPort json.RawMessage `json:"targetPort"`
			Protocol   string          `json:"protocol"`
			NodePort   int32           `json:"nodePort"`
		} `json:"ports"`
	} `json:"spec"`
}

type selectorRaw struct {
	MatchLabels      map[string]string `json:"matchLabels"`
	MatchExpressions []json.RawMessage `json:"matchExpressions"`
}

type peerRaw struct {
	NamespaceSelector *selectorRaw `json:"namespaceSelector"`
	PodSelector       *selectorRaw `json:"podSelector"`
	IPBlock           *struct {
		CIDR   string   `json:"cidr"`
		Except []string `json:"except"`
	} `json:"ipBlock"`
}

type netRuleRaw struct {
	From  []peerRaw `json:"from"`
	To    []peerRaw `json:"to"`
	Ports []struct {
		Protocol string          `json:"protocol"`
		Port     json.RawMessage `json:"port"`
		EndPort  int32           `json:"endPort"`
	} `json:"ports"`
}

type networkPolicyRaw struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
		UID       string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		PodSelector *selectorRaw `json:"podSelector"`
		PolicyTypes []string     `json:"policyTypes"`
		Ingress     []netRuleRaw `json:"ingress"`
		Egress      []netRuleRaw `json:"egress"`
	} `json:"spec"`
}

// intOrString renders a Kubernetes IntOrString value as the string form the
// analyzer expects: "8080" for a number, "http" for a named port, "" when the
// field is absent.
func intOrString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n int32
	if json.Unmarshal(raw, &n) == nil {
		return strconv.Itoa(int(n))
	}
	return ""
}

// toSelector maps a raw LabelSelector, preserving the load-bearing difference
// between an absent selector (nil) and a present-but-empty one (select all).
func toSelector(raw *selectorRaw) *netpolicy.Selector {
	if raw == nil {
		return nil
	}
	return &netpolicy.Selector{
		MatchLabels:    raw.MatchLabels,
		HasExpressions: len(raw.MatchExpressions) > 0,
	}
}

func toRules(raw []netRuleRaw, ingress bool) []netpolicy.Rule {
	if len(raw) == 0 {
		return nil
	}
	rules := make([]netpolicy.Rule, 0, len(raw))
	for _, r := range raw {
		rule := netpolicy.Rule{}
		peers := r.To
		if ingress {
			peers = r.From
		}
		for _, p := range peers {
			peer := netpolicy.Peer{
				NamespaceSelector: toSelector(p.NamespaceSelector),
				PodSelector:       toSelector(p.PodSelector),
			}
			if p.IPBlock != nil {
				peer.IPBlockCIDR = p.IPBlock.CIDR
				peer.IPBlockExcept = p.IPBlock.Except
			}
			rule.Peers = append(rule.Peers, peer)
		}
		for _, port := range r.Ports {
			rule.Ports = append(rule.Ports, netpolicy.PortRule{
				Protocol: port.Protocol,
				Port:     intOrString(port.Port),
				EndPort:  port.EndPort,
			})
		}
		rules = append(rules, rule)
	}
	return rules
}

// matchLabels reports whether all selector key/value pairs are present in the
// object labels (the subset match Kubernetes uses for Deployment/StatefulSet/
// DaemonSet pod selectors). An empty selector matches everything.
func matchLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
