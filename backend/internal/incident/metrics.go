package incident

import (
	"context"
	"strings"
	"time"
)

const (
	DefaultMetricsWindowDays = 30
	MaxMetricsWindowDays     = 90
	MetricsSampleLimit       = 200
)

type MetricsFilter struct {
	ClusterID  int64
	WindowDays int
}

type Metrics struct {
	WindowDays        int      `json:"window_days"`
	ClusterID         int64    `json:"cluster_id"`
	SampleLimit       int      `json:"sample_limit"`
	Sampled           int      `json:"sampled"`
	Truncated         bool     `json:"truncated"`
	Assigned          int      `json:"assigned"`
	Acknowledged      int      `json:"acknowledged"`
	Resolved          int      `json:"resolved"`
	Overdue           int      `json:"overdue"`
	SLAEvaluated      int      `json:"sla_evaluated"`
	SLACompliant      int      `json:"sla_compliant"`
	SLAComplianceRate *float64 `json:"sla_compliance_rate"`
	FirstAssignedSecs *float64 `json:"first_assigned_seconds"`
	MTTASeconds       *float64 `json:"mtta_seconds"`
	MTTRSeconds       *float64 `json:"mttr_seconds"`
}

func (s *Service) Metrics(ctx context.Context, filter MetricsFilter) (Metrics, error) {
	windowDays := filter.WindowDays
	if windowDays <= 0 {
		windowDays = DefaultMetricsWindowDays
	}
	if windowDays > MaxMetricsWindowDays {
		windowDays = MaxMetricsWindowDays
	}
	now := time.Now().UTC()
	items, err := s.repo.List(ctx, ListFilter{ClusterID: filter.ClusterID, Limit: MetricsSampleLimit})
	if err != nil {
		return Metrics{}, err
	}
	cutoff := now.Add(-time.Duration(windowDays) * 24 * time.Hour)
	selected := make([]Incident, 0, len(items))
	for _, item := range items {
		if item.CreatedAt.IsZero() || item.CreatedAt.Before(cutoff) {
			continue
		}
		selected = append(selected, item)
	}
	metrics := DeriveMetrics(selected)
	metrics.WindowDays = windowDays
	metrics.ClusterID = filter.ClusterID
	metrics.SampleLimit = MetricsSampleLimit
	metrics.Sampled = len(selected)
	metrics.Truncated = len(items) >= MetricsSampleLimit
	return metrics, nil
}

func DeriveMetrics(items []Incident) Metrics {
	metrics := Metrics{}
	var firstAssignedTotal, mttaTotal, mttrTotal time.Duration
	var firstAssignedCount, mttaCount int
	for _, item := range items {
		if item.Assignee != nil {
			metrics.Assigned++
		}
		if item.Overdue {
			metrics.Overdue++
		}
		if acknowledgedAt, ok := firstTimelineEvent(item.Timeline, func(event TimelineEvent) bool {
			return strings.Contains(event.Content, " to confirmed")
		}); ok {
			metrics.Acknowledged++
			if elapsed, valid := elapsedSince(item.CreatedAt, acknowledgedAt); valid {
				mttaTotal += elapsed
				mttaCount++
			}
		}
		if assignedAt, ok := firstTimelineEvent(item.Timeline, func(event TimelineEvent) bool {
			return strings.HasPrefix(event.Content, "handoff from ")
		}); ok {
			if elapsed, valid := elapsedSince(item.CreatedAt, assignedAt); valid {
				firstAssignedTotal += elapsed
				firstAssignedCount++
			}
		}
		if item.ResolvedAt != nil {
			metrics.Resolved++
			metrics.SLAEvaluated++
			if !item.ResolvedAt.After(item.SLADueAt) {
				metrics.SLACompliant++
			}
			if elapsed, valid := elapsedSince(item.CreatedAt, *item.ResolvedAt); valid {
				mttrTotal += elapsed
			}
		}
	}
	metrics.FirstAssignedSecs = averageSeconds(firstAssignedTotal, firstAssignedCount)
	metrics.MTTASeconds = averageSeconds(mttaTotal, mttaCount)
	metrics.MTTRSeconds = averageSeconds(mttrTotal, metrics.Resolved)
	if metrics.SLAEvaluated > 0 {
		rate := float64(metrics.SLACompliant) / float64(metrics.SLAEvaluated)
		metrics.SLAComplianceRate = &rate
	}
	return metrics
}

func firstTimelineEvent(events []TimelineEvent, match func(TimelineEvent) bool) (time.Time, bool) {
	var first time.Time
	for _, event := range events {
		if !match(event) || event.CreatedAt.IsZero() {
			continue
		}
		if first.IsZero() || event.CreatedAt.Before(first) {
			first = event.CreatedAt
		}
	}
	return first, !first.IsZero()
}

func elapsedSince(start, end time.Time) (time.Duration, bool) {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0, false
	}
	return end.Sub(start), true
}

func averageSeconds(total time.Duration, count int) *float64 {
	if count == 0 {
		return nil
	}
	average := total.Seconds() / float64(count)
	return &average
}
