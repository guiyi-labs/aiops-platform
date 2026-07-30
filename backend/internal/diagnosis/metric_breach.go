package diagnosis

import (
	"fmt"
	"time"

	"k8s-aiops.local/backend/internal/metricshistory"
)

const RuleNodeSustainedMetricBreach = "node.metric_sustained_breach.v1"

func EvaluateSustainedMetricBreach(clusterID int64, eval metricshistory.EvaluationResponse, observedAt time.Time) (Record, bool) {
	if eval.State != metricshistory.EvaluationStateFiring || len(eval.SustainedWindows) == 0 {
		return Record{}, false
	}
	evidence := make([]Evidence, 0, len(eval.SustainedWindows)+1)
	for i, window := range eval.SustainedWindows {
		evidence = append(evidence, Evidence{
			Type:   "metric_sustained_breach",
			Source: fmt.Sprintf("metric_history/%s/%s", eval.Series.MetricName, eval.Series.ResourceName),
			Content: map[string]any{
				"window_index":       i + 1,
				"metric":             eval.Series.MetricName,
				"unit":               eval.Series.Unit,
				"operator":           eval.Operator,
				"threshold":          eval.Threshold,
				"for_seconds":        eval.ForSeconds,
				"minimum_points":     eval.MinimumPoints,
				"breaching_points":   window.BreachingPoints,
				"span_seconds":       window.SpanSeconds,
				"start_collected_at": window.StartCollectedAt.UTC(),
				"end_collected_at":   window.EndCollectedAt.UTC(),
			},
		})
	}
	evidence = append(evidence, Evidence{
		Type:   "metric_evaluation_summary",
		Source: fmt.Sprintf("metric_history/%s/%s", eval.Series.MetricName, eval.Series.ResourceName),
		Content: map[string]any{
			"state":                eval.State,
			"total_points":         eval.PointsEvaluated,
			"breaching_points":     eval.BreachingPoints,
			"observed_span":        eval.ObservedSpanSeconds,
			"sustained_windows":    len(eval.SustainedWindows),
			"coverage_collections": eval.Coverage.Collections,
			"coverage_succeeded":   eval.Coverage.Succeeded,
			"coverage_points":      eval.Coverage.Points,
		},
	})
	severity := metricBreachSeverity(eval.Series.MetricName, eval.Threshold, eval.Series.Unit)
	summary := metricBreachSummary(eval)
	rootCauses := metricBreachRootCauses(eval)
	recommendations := metricBreachRecommendations(eval)
	record := Record{
		ClusterID:       clusterID,
		RuleID:          RuleNodeSustainedMetricBreach,
		Severity:        severity,
		Status:          "open",
		Resource:        ResourceRef{Kind: eval.Series.ResourceKind, Namespace: eval.Series.ResourceNamespace, Name: eval.Series.ResourceName},
		Summary:         summary,
		RootCauses:      rootCauses,
		Recommendations: recommendations,
		Evidence:        evidence,
		ObservedAt:      observedAt.UTC(),
	}
	return record, true
}

func metricBreachSeverity(metric string, threshold int64, unit string) string {
	switch metric {
	case metricshistory.MetricCPU:
		return "high"
	case metricshistory.MetricMemory:
		return "medium"
	default:
		return "medium"
	}
}

func metricBreachSummary(eval metricshistory.EvaluationResponse) string {
	metric := eval.Series.MetricName
	resource := eval.Series.ResourceName
	windows := len(eval.SustainedWindows)
	latest := eval.LatestFiringWindow
	span := int64(0)
	if latest != nil {
		span = latest.SpanSeconds
	}
	return fmt.Sprintf("节点 %s 的 %s 指标在过去窗口中出现 %d 个持续突破阈值的时间段，最近一次持续 %d 秒（%d 个采样点）。",
		resource, metric, windows, span, eval.BreachingPoints)
}

func metricBreachRootCauses(eval metricshistory.EvaluationResponse) []string {
	metric := eval.Series.MetricName
	switch metric {
	case metricshistory.MetricCPU:
		return []string{"节点工作负载持续高 CPU 使用率", "Pod 资源限制设置过低导致 throttling", "应用代码存在 CPU 密集型热点或无限循环"}
	case metricshistory.MetricMemory:
		return []string{"节点工作负载持续高内存使用率", "存在内存泄漏或未释放的缓存", "Pod 内存限制过低导致频繁 OOM"}
	default:
		return []string{"工作负载持续突破资源阈值", "资源限制设置不合理", "需要进一步排查具体指标"}
	}
}

func metricBreachRecommendations(eval metricshistory.EvaluationResponse) []string {
	metric := eval.Series.MetricName
	switch metric {
	case metricshistory.MetricCPU:
		return []string{"检查 Pod 的 CPU request/limit 是否合理", "分析应用 CPU 热点，考虑性能优化或水平扩容", "评估节点整体负载，必要时迁移工作负载或扩容集群"}
	case metricshistory.MetricMemory:
		return []string{"检查 Pod 的内存 request/limit 是否合理", "排查应用是否存在内存泄漏，使用 pprof 等工具分析", "评估节点内存使用趋势，必要时扩容或迁移"}
	default:
		return []string{"确认指标阈值设置是否合理", "分析工作负载使用模式，识别异常增长", "考虑设置合理的 HPA 规则自动扩缩容"}
	}
}
