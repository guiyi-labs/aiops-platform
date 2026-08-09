package diagnosis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// EvidenceCategory classifies timeline entries for the unified diagnosis
// narrative (M94). Categories mirror the product taxonomy: resource state,
// events, log excerpts, alerts, changes and automation results. Unknown
// evidence types degrade to resource_state so old records remain readable.
type EvidenceCategory string

const (
	CategoryResourceState EvidenceCategory = "resource_state"
	CategoryEvent         EvidenceCategory = "event"
	CategoryLog           EvidenceCategory = "log"
	CategoryAlert         EvidenceCategory = "alert"
	CategoryChange        EvidenceCategory = "change"
	CategoryAutomation    EvidenceCategory = "automation"
)

// TimelineEntry is one normalized evidence item on the diagnosis timeline.
// OccurredAt is a RFC3339 string when a time could be extracted, otherwise
// empty; Missing marks evidence that is explicitly absent (e.g. a missing
// Ready condition) rather than silently hiding it.
type TimelineEntry struct {
	Index         int              `json:"index"`
	Category      EvidenceCategory `json:"category"`
	Type          string           `json:"type"`
	Source        string           `json:"source"`
	Ref           string           `json:"ref"`
	Integrity     string           `json:"integrity"`
	OccurredAt    string           `json:"occurred_at,omitempty"`
	Missing       bool             `json:"missing"`
	MissingReason string           `json:"missing_reason,omitempty"`
	Summary       string           `json:"summary"`
}

// RootCauseCard is the first-screen summary of a diagnosis: conclusion,
// severity, status, first observation time, confidence source, impact scope
// and the key immutable evidence refs.
type RootCauseCard struct {
	Conclusion       string      `json:"conclusion"`
	Severity         string      `json:"severity"`
	Status           string      `json:"status"`
	FirstObservedAt  string      `json:"first_observed_at"`
	Confidence       string      `json:"confidence"`
	ConfidenceSource string      `json:"confidence_source"`
	Resource         ResourceRef `json:"resource"`
	KeyEvidenceRefs  []string    `json:"key_evidence_refs"`
}

// keyEvidenceRefLimit bounds the number of refs surfaced on the card.
const keyEvidenceRefLimit = 5

// WithNarrative returns the record augmented with the read-only evidence
// timeline and root cause card. It is a pure projection: it never writes,
// never reaches a cluster and never changes the persisted record.
func WithNarrative(record Record) Record {
	timeline := buildTimeline(record)
	record.Timeline = timeline
	card := buildRootCauseCard(record, timeline)
	record.RootCauseCard = &card
	return record
}

func buildTimeline(record Record) []TimelineEntry {
	entries := make([]TimelineEntry, 0, len(record.Evidence))
	for index, evidence := range record.Evidence {
		entry := TimelineEntry{
			Index:     index,
			Category:  classifyEvidence(evidence.Type),
			Type:      evidence.Type,
			Source:    evidence.Source,
			Ref:       evidenceRef(record, index, evidence),
			Integrity: evidenceIntegrity(evidence.Content),
			Summary:   summarizeEvidence(evidence),
		}
		entry.OccurredAt, entry.Missing, entry.MissingReason = extractEvidenceTime(evidence, record.ObservedAt)
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i].OccurredAt, entries[j].OccurredAt
		if left != right {
			return left < right
		}
		return entries[i].Index < entries[j].Index
	})
	return entries
}

func classifyEvidence(evidenceType string) EvidenceCategory {
	switch evidenceType {
	case "event":
		return CategoryEvent
	case "metric_sustained_breach", "metric_evaluation_summary":
		return CategoryAlert
	case "container_termination", "container_state", "node_condition", "pod_condition",
		"pod_status", "deployment_status", "endpoints", "service_spec",
		"hpa_condition", "hpa_scale", "ingress_backend", "persistent_volume_claim":
		return CategoryResourceState
	default:
		return CategoryResourceState
	}
}

func evidenceRef(record Record, index int, evidence Evidence) string {
	if evidence.Type == "event" && evidence.Source != "" {
		return evidence.Source
	}
	return fmt.Sprintf("diagnosis:%d:evidence:%d", record.ID, index)
}

func evidenceIntegrity(content map[string]any) string {
	canonical, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// extractEvidenceTime returns the normalized RFC3339 time, the missing flag
// and a missing reason. Missing is only set when the evidence itself is
// explicitly absent (e.g. a condition with status "Missing"); an unparsable
// timestamp falls back to the record observation time and is not "missing".
func extractEvidenceTime(evidence Evidence, observedAt time.Time) (occurredAt string, missing bool, missingReason string) {
	fallback := observedAt.UTC().Format(time.RFC3339)
	switch evidence.Type {
	case "event":
		for _, key := range []string{"last_timestamp", "first_timestamp"} {
			if raw, ok := stringContent(evidence.Content, key); ok {
				if parsed, ok := parseRFC3339(raw); ok {
					return parsed, false, ""
				}
			}
		}
		return fallback, false, ""
	case "node_condition", "pod_condition":
		if raw, ok := stringContent(evidence.Content, "last_transition_time"); ok {
			if parsed, ok := parseRFC3339(raw); ok {
				return parsed, false, ""
			}
		}
		if status, ok := stringContent(evidence.Content, "status"); ok && status == "Missing" {
			reason, _ := stringContent(evidence.Content, "reason")
			return "", true, reason
		}
		return fallback, false, ""
	case "container_termination":
		if raw, ok := stringContent(evidence.Content, "finished_at"); ok {
			if parsed, ok := parseRFC3339(raw); ok {
				return parsed, false, ""
			}
		}
		return fallback, false, ""
	case "metric_sustained_breach", "metric_evaluation_summary":
		if raw, ok := stringContent(evidence.Content, "window_end"); ok {
			if parsed, ok := parseRFC3339(raw); ok {
				return parsed, false, ""
			}
		}
		return fallback, false, ""
	default:
		return fallback, false, ""
	}
}

func stringContent(content map[string]any, key string) (string, bool) {
	raw, exists := content[key]
	if !exists {
		return "", false
	}
	value, ok := raw.(string)
	return value, ok && value != ""
}

func parseRFC3339(raw string) (string, bool) {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return "", false
	}
	return parsed.UTC().Format(time.RFC3339), true
}

func summarizeEvidence(evidence Evidence) string {
	content := evidence.Content
	message, _ := stringContent(content, "message")
	if len(message) > 200 {
		message = message[:200] + "…"
	}
	switch evidence.Type {
	case "event":
		reason, _ := stringContent(content, "reason")
		if reason != "" && message != "" {
			return fmt.Sprintf("%s · %s", reason, message)
		}
		if reason != "" {
			return reason
		}
	case "node_condition", "pod_condition":
		conditionType, _ := stringContent(content, "type")
		status, _ := stringContent(content, "status")
		if conditionType != "" {
			return fmt.Sprintf("%s = %s", conditionType, status)
		}
	case "container_state", "container_termination":
		container, _ := stringContent(content, "container")
		reason, _ := stringContent(content, "reason")
		if container != "" && reason != "" {
			return fmt.Sprintf("%s · %s", container, reason)
		}
		if container != "" {
			return container
		}
	case "deployment_status":
		desired, _ := numberContent(content, "desired_replicas")
		ready, _ := numberContent(content, "ready_replicas")
		available, _ := numberContent(content, "available_replicas")
		return fmt.Sprintf("desired=%v ready=%v available=%v", desired, ready, available)
	case "endpoints", "service_spec", "service_endpoints":
		return "Service 无可用 Endpoints"
	case "metric_sustained_breach", "metric_evaluation_summary":
		metric, _ := stringContent(content, "metric")
		if metric != "" {
			return fmt.Sprintf("指标持续越界：%s", metric)
		}
	}
	return evidence.Type
}

func numberContent(content map[string]any, key string) (any, bool) {
	raw, exists := content[key]
	return raw, exists
}

func buildRootCauseCard(record Record, timeline []TimelineEntry) RootCauseCard {
	card := RootCauseCard{
		Conclusion:       record.Summary,
		Severity:         record.Severity,
		Status:           record.Status,
		FirstObservedAt:  record.ObservedAt.UTC().Format(time.RFC3339),
		Confidence:       "deterministic",
		ConfidenceSource: record.RuleID,
		Resource:         record.Resource,
		KeyEvidenceRefs:  make([]string, 0, keyEvidenceRefLimit),
	}
	for _, entry := range timeline {
		if entry.OccurredAt != "" && entry.OccurredAt < card.FirstObservedAt {
			card.FirstObservedAt = entry.OccurredAt
		}
	}
	for _, entry := range timeline {
		if entry.Missing {
			continue
		}
		if len(card.KeyEvidenceRefs) >= keyEvidenceRefLimit {
			break
		}
		card.KeyEvidenceRefs = append(card.KeyEvidenceRefs, entry.Ref)
	}
	return card
}
