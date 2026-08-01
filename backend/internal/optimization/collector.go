package optimization

import (
	"context"
	"encoding/json"

	"k8s-aiops.local/backend/internal/cis"
	"k8s-aiops.local/backend/internal/deprecatedapi"
	"k8s-aiops.local/backend/internal/finops"
	"k8s-aiops.local/backend/internal/kubernetes"
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

// Collector turns live cluster data into the observation bundles the M61-M63
// analyzers consume. It never mutates cluster state (ADR 0004): it only reads
// and maps. The control-plane component flags checked by CIS are NOT reachable
// through the Kubernetes API, so CollectCIS leaves that domain empty by design;
// callers that can supply component flags (node/manifest access) may populate
// cis.Inputs.Components directly and pass the bundle to cis.Evaluate.
type Collector struct {
	lister  ClusterLister
	metrics MetricsSource
}

// NewCollector builds a Collector. lister is required; metrics may be nil.
func NewCollector(lister ClusterLister, metrics MetricsSource) *Collector {
	return &Collector{lister: lister, metrics: metrics}
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
