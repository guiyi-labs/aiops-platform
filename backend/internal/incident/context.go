package incident

import (
	"fmt"
	"sort"
	"time"
)

// ResourceContext carries the cross-M112–M114 "resource context contract".
// Every aggregated response (cockpit, coverage, future event/inspection
// views) must include this block so the client always knows what scope was
// observed, when, by whom, how fresh the data is, and how to interpret
// empty results.
type ResourceContext struct {
	Scope       ResourceScope   `json:"scope"`
	ObservedAt  time.Time       `json:"observed_at"`
	Source      string          `json:"source"`
	Freshness   FreshnessInfo   `json:"freshness"`
	EmptySample EmptySampleInfo `json:"empty_sample"`
}

// ResourceScope is the cluster/namespace/resource slice this aggregate covers.
type ResourceScope struct {
	ClusterID  int64  `json:"cluster_id"`
	Namespace  string `json:"namespace,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Name       string `json:"name,omitempty"`
	SourceType string `json:"source_type,omitempty"`
}

// FreshnessInfo describes data age relative to observed_at.
type FreshnessInfo struct {
	AgeSeconds int64  `json:"age_seconds"`
	AsOf       string `json:"as_of"`
}

// EmptySampleInfo tells the client what a 0-count or null result means.
// "fail_closed" means an empty result is treated as unknown/unhealthy
// (never confused with "everything healthy"); "safe_absent" means the
// absence is itself the expected state.
type EmptySampleInfo struct {
	Count    int    `json:"count"`
	Bounded  bool   `json:"bounded"`
	Semantic string `json:"semantic"`
}

// HealthSummary is the deterministic lifecycle health of the incident
// itself — it does NOT fabricate cluster/node health from external APIs.
// Real cluster health is reached via the deep links in each evidence block
// (and expanded by the M114 event cockpit).
type HealthSummary struct {
	Status            string `json:"status"`
	Overdue           bool   `json:"overdue"`
	EvidenceAvailable bool   `json:"evidence_available"`
	RunbookAvailable  bool   `json:"runbook_available"`
	NoteCount         int    `json:"note_count"`
	SystemEventCount  int    `json:"system_event_count"`
}

// EvidenceSourceSummary groups evidence by source type with counts.
type EvidenceSourceSummary struct {
	SourceType string `json:"source_type"`
	Count      int    `json:"count"`
	DeepLink   string `json:"deep_link"`
}

// RecommendedAction is a dry-run candidate surfaced in the cockpit. It is
// a read-only pointer into the existing remediation catalog; no action is
// executed from the cockpit.
type RecommendedAction struct {
	Action      string `json:"action"`
	TargetKind  string `json:"target_kind"`
	DryRunFirst bool   `json:"dry_run_first"`
	Summary     string `json:"summary"`
}

// ContextCockpit is the M112-1 aggregated incident context. It combines
// the incident snapshot, SLA state, evidence sources, recent timeline,
// runbook brief, and recommended actions into one deterministic read.
//
// The cockpit makes zero Kubernetes API calls: everything is derived from
// the incident record, the evidence resolver and the insight runbook
// mapping. Real-time cluster health is reached via each evidence block's
// deep link.
type ContextCockpit struct {
	ResourceContext ResourceContext        `json:"resource_context"`
	Incident        IncidentSnapshot       `json:"incident"`
	SLA             SLASummary             `json:"sla"`
	Health          HealthSummary          `json:"health"`
	EvidenceSources []EvidenceSourceSummary `json:"evidence_sources"`
	RecentEvents    []TimelineEvent        `json:"recent_events"`
	RunbookBrief    *RunbookBrief          `json:"runbook_brief,omitempty"`
	RecommendedActions []RecommendedAction `json:"recommended_actions"`
}

// IncidentSnapshot is a condensed view of the incident for the cockpit.
type IncidentSnapshot struct {
	ID         int64       `json:"id"`
	Number     string      `json:"number"`
	Title      string      `json:"title"`
	Severity   string      `json:"severity"`
	Status     string      `json:"status"`
	Summary    string      `json:"summary"`
	SourceType string      `json:"source_type"`
	Resource   ResourceRef `json:"resource"`
	Version    int64       `json:"version"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// SLASummary distills SLA state for the cockpit badge.
type SLASummary struct {
	DueAt        time.Time `json:"due_at"`
	Overdue      bool      `json:"overdue"`
	Remaining    string    `json:"remaining"`
	DeadlineText string    `json:"deadline_text"`
}

// RunbookBrief is a summary of the insight runbook for the cockpit.
type RunbookBrief struct {
	Domain          string `json:"domain"`
	FindingCode     string `json:"finding_code"`
	DiagnosisRoutes int    `json:"diagnosis_routes"`
	InspectionRules int    `json:"inspection_rules"`
	OperationCount  int    `json:"operation_count"`
}

// ContextCockpitInput gathers the data needed to build the cockpit. The
// handler resolves evidence, runbook and source data; the builder only
// assembles the deterministic response.
type ContextCockpitInput struct {
	Incident           Incident
	Evidence           []EvidenceItem
	RunbookBrief       *RunbookBrief
	RecommendedActions []RecommendedAction
}

// maxRecentEvents is the hard ceiling on how many timeline events the
// cockpit will include.
const maxRecentEvents = 10

// BuildContextCockpit assembles the M112-1 context cockpit from an
// incident, its evidence, and an optional runbook. This is a pure
// deterministic function — no database or Kubernetes calls.
func BuildContextCockpit(input ContextCockpitInput, observedAt time.Time) ContextCockpit {
	inc := input.Incident

	scope := ResourceScope{
		ClusterID:  inc.ClusterID,
		Namespace:  inc.Resource.Namespace,
		Kind:       inc.Resource.Kind,
		Name:       inc.Resource.Name,
		SourceType: inc.SourceType,
	}

	freshnessAge := oldestEvidenceAge(input.Evidence, observedAt)
	noteCount, systemCount := countTimeline(inc.Timeline)

	return ContextCockpit{
		ResourceContext: ResourceContext{
			Scope:       scope,
			ObservedAt:  observedAt,
			Source:      "incident_snapshot",
			Freshness:   FreshnessInfo{AgeSeconds: freshnessAge, AsOf: observedAt.UTC().Format(time.RFC3339)},
			EmptySample: EmptySampleInfo{Count: 0, Bounded: true, Semantic: "fail_closed"},
		},
		Incident: IncidentSnapshot{
			ID:         inc.ID,
			Number:     inc.Number,
			Title:      inc.Title,
			Severity:   inc.Severity,
			Status:     inc.Status,
			Summary:    inc.Summary,
			SourceType: inc.SourceType,
			Resource:   inc.Resource,
			Version:    inc.Version,
			CreatedAt:  inc.CreatedAt,
			UpdatedAt:  inc.UpdatedAt,
		},
		SLA: SLASummary{
			DueAt:        inc.SLADueAt,
			Overdue:      inc.Overdue,
			Remaining:    remainingText(inc.SLADueAt, observedAt),
			DeadlineText: slaDeadlineText(inc.SLADueAt, inc.Overdue, observedAt),
		},
		Health: HealthSummary{
			Status:            inc.Status,
			Overdue:           inc.Overdue,
			EvidenceAvailable: len(input.Evidence) > 0,
			RunbookAvailable:  input.RunbookBrief != nil,
			NoteCount:         noteCount,
			SystemEventCount:  systemCount,
		},
		EvidenceSources:      buildEvidenceSources(input.Evidence),
		RecentEvents:         recentTimeline(inc.Timeline, maxRecentEvents),
		RunbookBrief:         input.RunbookBrief,
		RecommendedActions:   input.RecommendedActions,
	}
}

// --- helpers ---

func oldestEvidenceAge(items []EvidenceItem, observedAt time.Time) int64 {
	var oldest time.Time
	for _, e := range items {
		if e.ObservedAt == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, e.ObservedAt); err == nil {
			if oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
		}
	}
	if oldest.IsZero() {
		return 0
	}
	age := int64(observedAt.Sub(oldest).Seconds())
	if age < 0 {
		return 0
	}
	return age
}

func countTimeline(events []TimelineEvent) (notes, systems int) {
	for _, e := range events {
		switch e.EventType {
		case EventTypeNote:
			notes++
		case EventTypeSystem:
			systems++
		}
	}
	return
}

func buildEvidenceSources(items []EvidenceItem) []EvidenceSourceSummary {
	counts := map[string]int{}
	deepLinks := map[string]string{}
	for _, e := range items {
		counts[e.SourceType]++
		if deepLinks[e.SourceType] == "" {
			deepLinks[e.SourceType] = e.DeepLink
		}
	}
	if len(counts) == 0 {
		return nil
	}
	result := make([]EvidenceSourceSummary, 0, len(counts))
	for st, count := range counts {
		result = append(result, EvidenceSourceSummary{SourceType: st, Count: count, DeepLink: deepLinks[st]})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SourceType < result[j].SourceType })
	return result
}

func recentTimeline(events []TimelineEvent, max int) []TimelineEvent {
	if len(events) == 0 {
		return nil
	}
	total := len(events)
	start := total - max
	if start < 0 {
		start = 0
	}
	selected := events[start:]
	result := make([]TimelineEvent, len(selected))
	for i, e := range selected {
		result[len(selected)-1-i] = e
	}
	return result
}

func remainingText(dueAt, observedAt time.Time) string {
	if dueAt.IsZero() {
		return "--"
	}
	remaining := dueAt.Sub(observedAt)
	if remaining < 0 {
		return fmt.Sprintf("逾期 %s", formatDuration(-remaining))
	}
	return formatDuration(remaining)
}

func slaDeadlineText(dueAt time.Time, overdue bool, observedAt time.Time) string {
	if dueAt.IsZero() {
		return "未设置"
	}
	if overdue {
		return "已逾期"
	}
	return "剩余 " + remainingText(dueAt, observedAt)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	if d < time.Hour {
		return fmt.Sprintf("%d分钟", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%d小时", hours)
	}
	return fmt.Sprintf("%d小时%d分钟", hours, minutes)
}

// BuildRunbookBriefFromResolved creates a RunbookBrief from insight.Resolve
// output. The handler converts the insight.Runbook without importing insight
// into this layer; a nil (empty-domain) runbook yields nil so the cockpit's
// Health.RunbookAvailable stays honest.
func BuildRunbookBriefFromResolved(domain, findingCode string, diagnosisRoutes, inspectionRules, operationCount int) *RunbookBrief {
	if domain == "" {
		return nil
	}
	return &RunbookBrief{
		Domain:          domain,
		FindingCode:     findingCode,
		DiagnosisRoutes: diagnosisRoutes,
		InspectionRules: inspectionRules,
		OperationCount:  operationCount,
	}
}