package slo

import (
	"context"
	"sort"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
)

// MetricshistorySource serves SLO QuerySLI from the metrics history store.
//
// Only the workload_readiness template is supported: readiness_ready and
// readiness_total gauges recorded per collection run are converted into
// cumulative counters, so the evaluator's delta conversion yields the
// ready-replica fraction observed over the window. A rollout that drops
// readiness therefore reduces the ratio and can trigger a burn signal.
//
// Request-ratio templates (request_success_ratio, request_latency_target_ratio)
// have no truthful source in metrics history and return an empty series: the
// evaluator reports coverage unavailable and applies the SLO's missing-data
// policy. Health is never fabricated.
type MetricshistorySource struct {
	svc *metricshistory.Service
}

// NewMetricshistorySource constructs a source over the metrics history
// service. svc may be nil (source disabled — all queries return no data).
func NewMetricshistorySource(svc *metricshistory.Service) *MetricshistorySource {
	return &MetricshistorySource{svc: svc}
}

// QuerySLI implements MetricsSource.
func (s *MetricshistorySource) QuerySLI(ctx context.Context, def *Definition, start, end time.Time, step time.Duration) (SLISeries, error) {
	if s.svc == nil || def.Template != TemplateWorkloadReadiness {
		return SLISeries{Source: "metricshistory"}, nil
	}
	ready, err := s.svc.Query(ctx, metricshistory.SeriesQuery{
		ClusterID: def.ClusterID, ResourceKind: def.Service.Kind,
		ResourceNamespace: def.Service.Namespace, ResourceName: def.Service.Name,
		MetricName: metricshistory.MetricReadinessReady, From: start, To: end, Limit: 1440,
	})
	if err != nil {
		return SLISeries{}, err
	}
	total, err := s.svc.Query(ctx, metricshistory.SeriesQuery{
		ClusterID: def.ClusterID, ResourceKind: def.Service.Kind,
		ResourceNamespace: def.Service.Namespace, ResourceName: def.Service.Name,
		MetricName: metricshistory.MetricReadinessTotal, From: start, To: end, Limit: 1440,
	})
	if err != nil {
		return SLISeries{}, err
	}
	good, totalSeries := cumulativeReadiness(ready.Points, total.Points)
	return SLISeries{
		Good:            good,
		Total:           totalSeries,
		ExpectedSamples: 0, // unknown expected rate; coverage from actual count
		Source:          "metricshistory",
	}, nil
}

// cumulativeReadiness pairs readiness_ready / readiness_total observations by
// collection timestamp and converts them into monotonic cumulative counters.
// A timestamp is emitted only when both counters were observed (a missing
// readiness_ready observation is not treated as unready). Output is sorted
// oldest-first, matching the MetricsSource contract.
func cumulativeReadiness(ready, total []metricshistory.Point) (good, totals []Sample) {
	type pair struct {
		ready, total int64
		readySeen    bool
		totalSeen    bool
	}
	byTime := make(map[int64]*pair)
	for _, p := range ready {
		key := p.SourceTimestamp.Truncate(time.Second).Unix()
		if byTime[key] == nil {
			byTime[key] = &pair{}
		}
		byTime[key].ready += p.Value
		byTime[key].readySeen = true
	}
	for _, p := range total {
		key := p.SourceTimestamp.Truncate(time.Second).Unix()
		if byTime[key] == nil {
			byTime[key] = &pair{}
		}
		byTime[key].total += p.Value
		byTime[key].totalSeen = true
	}
	keys := make([]int64, 0, len(byTime))
	for k := range byTime {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var cumGood, cumTotal int64
	for _, k := range keys {
		p := byTime[k]
		if !p.readySeen || !p.totalSeen {
			continue
		}
		cumGood += p.ready
		cumTotal += p.total
		ts := time.Unix(k, 0).UTC()
		good = append(good, Sample{Timestamp: ts, Value: float64(cumGood)})
		totals = append(totals, Sample{Timestamp: ts, Value: float64(cumTotal)})
	}
	return good, totals
}
