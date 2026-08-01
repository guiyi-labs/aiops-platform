package signal

import (
	"fmt"
	"time"

	"k8s-aiops.local/backend/internal/alert"
)

// AlertNormalizer converts M27 alert Rule+Instance transitions into signal
// occurrences. Alert rules are currently Node-only and do not persist a UID,
// so the resulting ResourceCitation is marked Incomplete.
type AlertNormalizer struct{}

func NewAlertNormalizer() AlertNormalizer { return AlertNormalizer{} }

// FromInstance builds an IngestRequest for an alert state transition. The
// caller passes the Rule so the normalizer can fill in ResourceKind/Name.
// state must be "firing" or "resolved".
func (AlertNormalizer) FromInstance(rule alert.Rule, inst alert.Instance, ingestionRunID string) (IngestRequest, error) {
	signalID := "alert.firing.v1"
	state := StateActive
	observedAt := inst.LastFiredAt
	if inst.State == alert.StateResolved {
		signalID = "alert.resolved.v1"
		state = StateResolved
		if inst.ResolvedAt != nil {
			observedAt = *inst.ResolvedAt
		}
	}
	if observedAt.IsZero() {
		observedAt = inst.UpdatedAt
	}
	return IngestRequest{
		SignalID:       signalID,
		Producer:       ProducerAlert,
		ClusterID:      rule.ClusterID,
		Namespace:      "",
		Resource:       ResourceCitation{Kind: rule.ResourceKind, Name: rule.ResourceName},
		Severity:       "warning",
		State:          state,
		Coverage:       CoverageComplete,
		Freshness:      observedAt,
		ObservedAt:     observedAt,
		Attributes:     map[string]string{"rule_id": fmt.Sprintf("%d", rule.ID), "display_name": rule.DisplayName, "metric": rule.MetricName},
		Evidence:       []EvidenceRef{{Kind: "alert_instance", ID: inst.ID}},
		IngestionRunID: ingestionRunID,
	}, nil
}

// MetricBreachNormalizer converts an M21 sustained-window evaluation into a
// metric.sustained_breach.v1 signal. The caller passes the cluster id, the
// resource citation (with UID when available), the evaluation response and
// the series identity.
type MetricBreachNormalizer struct{}

func NewMetricBreachNormalizer() MetricBreachNormalizer { return MetricBreachNormalizer{} }

// MetricBreachInput is the typed input for MetricBreachNormalizer.FromEvaluation.
type MetricBreachInput struct {
	ClusterID       int64
	Resource        ResourceCitation
	MetricName      string
	EvaluationState string // "firing" | "normal" | "insufficient_data"
	WindowStart     time.Time
	WindowEnd       time.Time
	ObservedAt      time.Time
}

// FromEvaluation builds an IngestRequest for a sustained metric breach.
// Returns ok=false when the evaluation state is not "firing" so the caller
// can skip ingestion without an error.
func (MetricBreachNormalizer) FromEvaluation(in MetricBreachInput, ingestionRunID string) (IngestRequest, bool, error) {
	if in.EvaluationState != "firing" {
		return IngestRequest{}, false, nil
	}
	return IngestRequest{
		SignalID:       "metric.sustained_breach.v1",
		Producer:       ProducerMetric,
		ClusterID:      in.ClusterID,
		Namespace:      in.Resource.Namespace,
		Resource:       in.Resource,
		Severity:       "warning",
		State:          StateActive,
		Coverage:       CoverageComplete,
		Freshness:      in.ObservedAt,
		WindowStart:    &in.WindowStart,
		WindowEnd:      &in.WindowEnd,
		ObservedAt:     in.ObservedAt,
		Attributes:     map[string]string{"metric": in.MetricName},
		Evidence:       []EvidenceRef{{Kind: "metric_window"}},
		IngestionRunID: ingestionRunID,
	}, true, nil
}
