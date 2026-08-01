package optimization

import (
	"context"
	"sort"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
)

// metricsHistorySource is the production MetricsSource. It reads per-pod usage
// p95 from the metrics history store (populated by the M30 metricshistory
// collector) and aggregates across the pods of a workload, returning the
// worst-case observed p95 for a given container. A workload container therefore
// gets the highest usage any of its pods exhibited — a conservative, safe
// right-sizing signal.
type metricsHistorySource struct {
	svc    *metricshistory.Service
	window time.Duration
}

// NewMetricsHistorySource builds a MetricsSource over the metrics history
// service. window is how far back to look for samples (the metricshistory
// default retention is 7d; 24h is a sensible right-sizing window).
func NewMetricsHistorySource(svc *metricshistory.Service, window time.Duration) MetricsSource {
	if window <= 0 {
		window = 24 * time.Hour
	}
	return metricsHistorySource{svc: svc, window: window}
}

func (m metricsHistorySource) PodContainerP95(ctx context.Context, clusterID int64, namespace, pod, container string) (int64, int64, bool) {
	from := time.Now().Add(-m.window)
	to := time.Now()
	cpu, okCPU := m.p95(ctx, clusterID, namespace, pod, container, metricshistory.MetricCPU, from, to)
	mem, okMem := m.p95(ctx, clusterID, namespace, pod, container, metricshistory.MetricMemory, from, to)
	return cpu, mem, okCPU || okMem
}

func (m metricsHistorySource) p95(ctx context.Context, clusterID int64, namespace, pod, container, metric string, from, to time.Time) (int64, bool) {
	resp, err := m.svc.Query(ctx, metricshistory.SeriesQuery{
		ClusterID:         clusterID,
		ResourceKind:      metricshistory.ResourcePod,
		ResourceNamespace: namespace,
		ResourceName:      pod,
		ContainerName:     container,
		MetricName:        metric,
		From:              from,
		To:                to,
		Limit:             2000,
	})
	if err != nil || len(resp.Points) == 0 {
		return 0, false
	}
	values := make([]int64, len(resp.Points))
	for i, p := range resp.Points {
		values[i] = p.Value
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	idx := int(float64(len(values)-1) * 0.95)
	if idx < 0 {
		idx = 0
	}
	return values[idx], true
}
