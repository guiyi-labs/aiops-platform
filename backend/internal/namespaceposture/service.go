package namespaceposture

import (
	"context"
	"sort"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// KubernetesSource is the reviewed subset of k8sgateway.Service that the
// posture aggregator consumes. Bounding the interface keeps the posture
// read-only and makes accidental mutation-surface discovery obvious in code
// review.
type KubernetesSource interface {
	Namespaces(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Namespace], error)
	Nodes(context.Context, int64, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Node], error)
	ResourceQuotas(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.ResourceQuota], error)
	LimitRanges(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.LimitRange], error)
	PodDisruptionBudgets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.PodDisruptionBudget], error)
	Pods(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Pod], error)
	Deployments(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Deployment], error)
	StatefulSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.StatefulSet], error)
	DaemonSets(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.DaemonSet], error)
	Jobs(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.Job], error)
	CronJobs(context.Context, int64, string, apiquery.ListQuery) (apiquery.ListResponse[k8sgateway.CronJob], error)
}

type Service struct {
	kubernetes KubernetesSource
	now        func() time.Time
	// Per-call section caps keep posture aggregation bounded regardless of
	// cluster size. The posture is a compact evidence-cited rollup, not an
	// unbounded inventory dump.
	perSectionLimit int
}

func NewService(kubernetes KubernetesSource) *Service {
	return &Service{kubernetes: kubernetes, now: time.Now, perSectionLimit: 100}
}

const (
	sectionResourceQuotas = "resource_quotas"
	sectionLimitRanges    = "limit_ranges"
	sectionWorkloads      = "workloads"
	sectionPods           = "pods"
	sectionPDBs           = "pdbs"
	sectionNodeCapacity   = "node_capacity"
)

func boundedQuery(limit int) apiquery.ListQuery {
	return apiquery.ListQuery{Page: 1, Limit: limit, Offset: 0}
}

// Get returns the posture for a single Namespace. It fans out concurrently
// to the reviewed KubernetesSource sections and records every partial
// failure or truncation explicitly via EvidenceCitation.
func (s *Service) Get(ctx context.Context, clusterID int64, namespace string) (NamespacePosture, error) {
	collectedAt := s.now()
	posture := NamespacePosture{
		Name:            namespace,
		Phase:           "",
		Labels:          map[string]string{},
		Annotations:     map[string]string{},
		CreatedAt:       "",
		PartialSections: []string{},
	}

	// Namespace metadata is required; if we can't read it, return error.
	nsQuery := boundedQuery(1)
	nsQuery.Name = namespace
	nsResp, err := s.kubernetes.Namespaces(ctx, clusterID, nsQuery)
	if err != nil {
		return posture, err
	}
	if len(nsResp.Items) == 0 {
		return posture, k8sgateway.ErrResourceNotFound
	}
	ns := nsResp.Items[0]
	posture.Name = ns.Metadata.Name
	posture.Phase = ns.Status.Phase
	if ns.Metadata.Labels != nil {
		posture.Labels = ns.Metadata.Labels
	}
	if ns.Metadata.Annotations != nil {
		posture.Annotations = ns.Metadata.Annotations
	}
	posture.CreatedAt = ns.Metadata.CreationTimestamp

	// Fan out the remaining six sections concurrently. Each section is
	// independent; a failure in one never masks success in another.
	// NodeCapacity is cluster-level (not Namespace-scoped) but included in
	// every posture as denominator context.
	var wg sync.WaitGroup
	var mu sync.Mutex

	wg.Add(6)
	q := boundedQuery(s.perSectionLimit)

	go func() {
		defer wg.Done()
		resp, err := s.kubernetes.ResourceQuotas(ctx, clusterID, namespace, q)
		mu.Lock()
		defer mu.Unlock()
		posture.ResourceQuotas.Evidence = newCitation(namespacedAPIPath("/api/v1", namespace, "resourcequotas"), collectedAt)
		if err != nil {
			markError(&posture.ResourceQuotas.Evidence, err)
			posture.PartialSections = append(posture.PartialSections, sectionResourceQuotas)
			return
		}
		markTruncated(&posture.ResourceQuotas.Evidence, resp.Total, len(resp.Items), resp.Remaining)
		entries := make([]ResourceQuotaEntry, 0, len(resp.Items))
		for _, item := range resp.Items {
			entries = append(entries, ResourceQuotaEntry{
				Name: item.Metadata.Name,
				Hard: copyMap(item.Status.Hard),
				Used: copyMap(item.Status.Used),
			})
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		posture.ResourceQuotas.Quotas = entries
		if posture.ResourceQuotas.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionResourceQuotas)
		}
	}()

	go func() {
		defer wg.Done()
		resp, err := s.kubernetes.LimitRanges(ctx, clusterID, namespace, q)
		mu.Lock()
		defer mu.Unlock()
		posture.LimitRanges.Evidence = newCitation(namespacedAPIPath("/api/v1", namespace, "limitranges"), collectedAt)
		if err != nil {
			markError(&posture.LimitRanges.Evidence, err)
			posture.PartialSections = append(posture.PartialSections, sectionLimitRanges)
			return
		}
		markTruncated(&posture.LimitRanges.Evidence, resp.Total, len(resp.Items), resp.Remaining)
		posture.LimitRanges.Ranges = resp.Items
		if posture.LimitRanges.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionLimitRanges)
		}
	}()

	go func() {
		defer wg.Done()
		summary, err := s.collectWorkloads(ctx, clusterID, namespace, q, collectedAt)
		mu.Lock()
		defer mu.Unlock()
		posture.Workloads = summary
		if err != nil || summary.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionWorkloads)
		}
	}()

	go func() {
		defer wg.Done()
		summary, err := s.collectPods(ctx, clusterID, namespace, q, collectedAt)
		mu.Lock()
		defer mu.Unlock()
		posture.Pods = summary
		if err != nil || summary.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionPods)
		}
	}()

	go func() {
		defer wg.Done()
		resp, err := s.kubernetes.PodDisruptionBudgets(ctx, clusterID, namespace, q)
		mu.Lock()
		defer mu.Unlock()
		posture.PDBs.Evidence = newCitation(namespacedAPIPath("/apis/policy/v1", namespace, "poddisruptionbudgets"), collectedAt)
		if err != nil {
			markError(&posture.PDBs.Evidence, err)
			posture.PartialSections = append(posture.PartialSections, sectionPDBs)
			return
		}
		markTruncated(&posture.PDBs.Evidence, resp.Total, len(resp.Items), resp.Remaining)
		entries := make([]PDBEntry, 0, len(resp.Items))
		for _, item := range resp.Items {
			entries = append(entries, PDBEntry{
				Name:               item.Metadata.Name,
				MinAvailable:       stringValue(item.Spec.MinAvailable),
				MaxUnavailable:     stringValue(item.Spec.MaxUnavailable),
				CurrentHealthy:     item.Status.CurrentHealthy,
				DesiredHealthy:     item.Status.DesiredHealthy,
				DisruptionsAllowed: item.Status.DisruptionsAllowed,
				ExpectedPods:       item.Status.ExpectedPods,
			})
		}
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		posture.PDBs.PDBs = entries
		posture.PDBs.Count = int32(len(entries))
		if posture.PDBs.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionPDBs)
		}
	}()

	go func() {
		defer wg.Done()
		capacity, err := s.collectNodeCapacity(ctx, clusterID, q, collectedAt)
		mu.Lock()
		defer mu.Unlock()
		posture.NodeCapacity = capacity
		if err != nil || capacity.Evidence.Status != SourceComplete {
			posture.PartialSections = append(posture.PartialSections, sectionNodeCapacity)
		}
	}()

	wg.Wait()
	sort.Strings(posture.PartialSections)
	return posture, nil
}

// List returns a posture-summary row for every Namespace matching the query.
// The list contract intentionally avoids the full posture per Namespace (it
// would be O(kinds*namespaces) and the fan-out is unbounded); callers that
// need a single Namespace's full evidence should call Get.
func (s *Service) List(ctx context.Context, clusterID int64, query apiquery.ListQuery) (apiquery.ListResponse[PostureListEntry], error) {
	nsResp, err := s.kubernetes.Namespaces(ctx, clusterID, query)
	if err != nil {
		return apiquery.ListResponse[PostureListEntry]{}, err
	}
	entries := make([]PostureListEntry, 0, len(nsResp.Items))
	q := boundedQuery(s.perSectionLimit)
	for _, ns := range nsResp.Items {
		entry := PostureListEntry{
			Name:            ns.Metadata.Name,
			Phase:           ns.Status.Phase,
			CreatedAt:       ns.Metadata.CreationTimestamp,
			PartialSections: []string{},
		}
		// Compact rollups are best-effort per Namespace; a failure in one
		// Namespace's section is recorded as partial and never fails the
		// whole list response.
		if rq, err := s.kubernetes.ResourceQuotas(ctx, clusterID, ns.Metadata.Name, q); err == nil {
			entry.QuotaCount = int32(len(rq.Items))
		} else {
			entry.PartialSections = append(entry.PartialSections, sectionResourceQuotas)
		}
		if lr, err := s.kubernetes.LimitRanges(ctx, clusterID, ns.Metadata.Name, q); err == nil {
			entry.LimitRangeCount = int32(len(lr.Items))
		} else {
			entry.PartialSections = append(entry.PartialSections, sectionLimitRanges)
		}
		if pdb, err := s.kubernetes.PodDisruptionBudgets(ctx, clusterID, ns.Metadata.Name, q); err == nil {
			entry.PDBCount = int32(len(pdb.Items))
		} else {
			entry.PartialSections = append(entry.PartialSections, sectionPDBs)
		}
		if pods, err := s.kubernetes.Pods(ctx, clusterID, ns.Metadata.Name, q); err == nil {
			entry.PodCount = int32(len(pods.Items))
		} else {
			entry.PartialSections = append(entry.PartialSections, sectionPods)
		}
		wlCount := int32(0)
		wlPartial := false
		for _, fn := range workloadFetchers(s.kubernetes, ctx, clusterID, ns.Metadata.Name, q) {
			n, err := fn()
			if err != nil {
				wlPartial = true
				continue
			}
			wlCount += n
		}
		entry.WorkloadCount = wlCount
		if wlPartial {
			entry.PartialSections = append(entry.PartialSections, sectionWorkloads)
		}
		sort.Strings(entry.PartialSections)
		entries = append(entries, entry)
	}
	return apiquery.ListResponse[PostureListEntry]{
		Items:     entries,
		Total:     nsResp.Total,
		Remaining: nsResp.Remaining,
	}, nil
}

func (s *Service) collectWorkloads(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery, collectedAt time.Time) (WorkloadSummary, error) {
	summary := WorkloadSummary{Evidence: newCitation("<aggregate: apps/v1 + batch/v1 workloads>", collectedAt)}
	byKind := make([]WorkloadKindCount, 0, 5)
	partial := false

	type kindResult struct {
		kc  WorkloadKindCount
		err error
	}
	results := make(chan kindResult, 5)
	kinds := []struct {
		name string
		fn   func() (WorkloadKindCount, error)
	}{
		{"Deployment", func() (WorkloadKindCount, error) { return s.fetchDeployments(ctx, clusterID, namespace, q) }},
		{"StatefulSet", func() (WorkloadKindCount, error) { return s.fetchStatefulSets(ctx, clusterID, namespace, q) }},
		{"DaemonSet", func() (WorkloadKindCount, error) { return s.fetchDaemonSets(ctx, clusterID, namespace, q) }},
		{"Job", func() (WorkloadKindCount, error) { return s.fetchJobs(ctx, clusterID, namespace, q) }},
		{"CronJob", func() (WorkloadKindCount, error) { return s.fetchCronJobs(ctx, clusterID, namespace, q) }},
	}
	for _, k := range kinds {
		k := k
		go func() {
			kc, err := k.fn()
			kc.Kind = k.name
			results <- kindResult{kc, err}
		}()
	}
	for i := 0; i < len(kinds); i++ {
		r := <-results
		if r.err != nil {
			partial = true
			continue
		}
		byKind = append(byKind, r.kc)
		summary.TotalCount += r.kc.Count
		summary.DesiredTotal += r.kc.DesiredReplicas
		summary.ReadyTotal += r.kc.ReadyReplicas
	}
	summary.Evidence.Returned = int(summary.TotalCount)
	if partial {
		summary.Evidence.Status = SourcePartial
	}
	sort.SliceStable(byKind, func(i, j int) bool { return byKind[i].Kind < byKind[j].Kind })
	summary.ByKind = byKind
	var err error
	if partial {
		err = errPartialWorkload
	}
	return summary, err
}

var errPartialWorkload = errPartial("workload aggregation partial")

type errPartial string

func (e errPartial) Error() string { return string(e) }

func (s *Service) fetchDeployments(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (WorkloadKindCount, error) {
	resp, err := s.kubernetes.Deployments(ctx, clusterID, namespace, q)
	if err != nil {
		return WorkloadKindCount{}, err
	}
	kc := WorkloadKindCount{Count: int32(len(resp.Items))}
	for _, item := range resp.Items {
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		kc.DesiredReplicas += desired
		kc.ReadyReplicas += item.Status.ReadyReplicas
		kc.AvailableReplicas += item.Status.AvailableReplicas
		kc.UpdatedReplicas += item.Status.UpdatedReplicas
		kc.FailedReplicas += item.Status.UnavailableReplicas
	}
	return kc, nil
}

func (s *Service) fetchStatefulSets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (WorkloadKindCount, error) {
	resp, err := s.kubernetes.StatefulSets(ctx, clusterID, namespace, q)
	if err != nil {
		return WorkloadKindCount{}, err
	}
	kc := WorkloadKindCount{Count: int32(len(resp.Items))}
	for _, item := range resp.Items {
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		kc.DesiredReplicas += desired
		kc.ReadyReplicas += item.Status.ReadyReplicas
		kc.AvailableReplicas += item.Status.AvailableReplicas
		kc.UpdatedReplicas += item.Status.UpdatedReplicas
	}
	return kc, nil
}

func (s *Service) fetchDaemonSets(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (WorkloadKindCount, error) {
	resp, err := s.kubernetes.DaemonSets(ctx, clusterID, namespace, q)
	if err != nil {
		return WorkloadKindCount{}, err
	}
	kc := WorkloadKindCount{Count: int32(len(resp.Items))}
	for _, item := range resp.Items {
		kc.DesiredReplicas += item.Status.DesiredNumberScheduled
		kc.ReadyReplicas += item.Status.NumberReady
		kc.AvailableReplicas += item.Status.NumberAvailable
		kc.UpdatedReplicas += item.Status.UpdatedNumberScheduled
		kc.FailedReplicas += item.Status.NumberUnavailable
	}
	return kc, nil
}

func (s *Service) fetchJobs(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (WorkloadKindCount, error) {
	resp, err := s.kubernetes.Jobs(ctx, clusterID, namespace, q)
	if err != nil {
		return WorkloadKindCount{}, err
	}
	kc := WorkloadKindCount{Count: int32(len(resp.Items))}
	for _, item := range resp.Items {
		parallelism := int32(1)
		if item.Spec.Parallelism != nil {
			parallelism = *item.Spec.Parallelism
		}
		kc.DesiredReplicas += parallelism
		kc.ReadyReplicas += item.Status.Active
		kc.AvailableReplicas += item.Status.Succeeded
		kc.FailedReplicas += item.Status.Failed
	}
	return kc, nil
}

func (s *Service) fetchCronJobs(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) (WorkloadKindCount, error) {
	resp, err := s.kubernetes.CronJobs(ctx, clusterID, namespace, q)
	if err != nil {
		return WorkloadKindCount{}, err
	}
	kc := WorkloadKindCount{Count: int32(len(resp.Items))}
	// CronJobs have no desired replica dimension; desired tracks active
	// children. This posture intentionally records zero for desired/ready
	// rather than inventing a synthetic metric.
	return kc, nil
}

func (s *Service) collectPods(ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery, collectedAt time.Time) (PodSummary, error) {
	summary := PodSummary{Evidence: newCitation(namespacedAPIPath("/api/v1", namespace, "pods"), collectedAt)}
	resp, err := s.kubernetes.Pods(ctx, clusterID, namespace, q)
	if err != nil {
		markError(&summary.Evidence, err)
		return summary, err
	}
	markTruncated(&summary.Evidence, resp.Total, len(resp.Items), resp.Remaining)
	phaseCount := map[string]int32{}
	nodeCount := map[string]int32{}
	scheduled := int32(0)
	for _, item := range resp.Items {
		phase := item.Status.Phase
		if phase == "" {
			phase = "Unknown"
		}
		phaseCount[phase]++
		if item.Spec.NodeName != "" {
			nodeCount[item.Spec.NodeName]++
			scheduled++
		}
	}
	summary.Total = int32(len(resp.Items))
	summary.Scheduled = scheduled
	summary.ByPhase = sortedPhaseCounts(phaseCount)
	summary.ByNode = sortedNodeSpreads(nodeCount)
	summary.UniqueNodeCount = int32(len(nodeCount))
	return summary, nil
}

func (s *Service) collectNodeCapacity(ctx context.Context, clusterID int64, q apiquery.ListQuery, collectedAt time.Time) (NodeCapacityPosture, error) {
	posture := NodeCapacityPosture{Evidence: newCitation("/api/v1/nodes", collectedAt)}
	resp, err := s.kubernetes.Nodes(ctx, clusterID, q)
	if err != nil {
		markError(&posture.Evidence, err)
		return posture, err
	}
	markTruncated(&posture.Evidence, resp.Total, len(resp.Items), resp.Remaining)
	entries := make([]NodeCapacityEntry, 0, len(resp.Items))
	for _, item := range resp.Items {
		entries = append(entries, NodeCapacityEntry{
			Name:        item.Metadata.Name,
			Capacity:    copyMap(item.Status.Capacity),
			Allocatable: copyMap(item.Status.Allocatable),
			Schedulable: !item.Spec.Unschedulable,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	posture.Nodes = entries
	posture.Count = int32(len(entries))
	return posture, nil
}

func workloadFetchers(src KubernetesSource, ctx context.Context, clusterID int64, namespace string, q apiquery.ListQuery) []func() (int32, error) {
	return []func() (int32, error){
		func() (int32, error) {
			r, e := src.Deployments(ctx, clusterID, namespace, q)
			return int32(len(r.Items)), e
		},
		func() (int32, error) {
			r, e := src.StatefulSets(ctx, clusterID, namespace, q)
			return int32(len(r.Items)), e
		},
		func() (int32, error) {
			r, e := src.DaemonSets(ctx, clusterID, namespace, q)
			return int32(len(r.Items)), e
		},
		func() (int32, error) { r, e := src.Jobs(ctx, clusterID, namespace, q); return int32(len(r.Items)), e },
		func() (int32, error) {
			r, e := src.CronJobs(ctx, clusterID, namespace, q)
			return int32(len(r.Items)), e
		},
	}
}

func namespacedAPIPath(groupVersion, namespace, kind string) string {
	if namespace == "" {
		return groupVersion + "/" + kind
	}
	return groupVersion + "/namespaces/" + namespace + "/" + kind
}

func stringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return intString(int64(val))
	case int64:
		return intString(val)
	case int32:
		return intString(int64(val))
	case int:
		return intString(int64(val))
	default:
		return ""
	}
}

func intString(v int64) string {
	if v == 0 {
		return "0"
	}
	buf := []byte{}
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		buf = append([]byte{byte('0' + v%10)}, buf...)
		v /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func sortedPhaseCounts(m map[string]int32) []PodPhaseCount {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PodPhaseCount, 0, len(keys))
	for _, k := range keys {
		out = append(out, PodPhaseCount{Phase: k, Count: m[k]})
	}
	return out
}

func sortedNodeSpreads(m map[string]int32) []PodNodeSpread {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]PodNodeSpread, 0, len(keys))
	for _, k := range keys {
		out = append(out, PodNodeSpread{NodeName: k, Count: m[k]})
	}
	return out
}

func copyMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
