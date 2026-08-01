package signal

import (
	"fmt"

	"k8s-aiops.local/backend/internal/diagnosis"
)

// DiagnosisNormalizer converts an M17/M18 diagnosis.Record into one or more
// IngestRequests. Each diagnosis rule maps to exactly one signal id; the
// mapping is compiled here so unregistered rules fail closed at BuildOccurrence.
//
// The normalizer is a pure function: it does not touch the database or
// Kubernetes. The caller is responsible for loading the Record.
type DiagnosisNormalizer struct{}

// NewDiagnosisNormalizer returns a stateless normalizer.
func NewDiagnosisNormalizer() DiagnosisNormalizer { return DiagnosisNormalizer{} }

// ruleToSignalID maps M17/M18 diagnosis rule constants to M39 signal ids.
// A rule not in this map yields an error from Normalize so the caller can
// skip it explicitly rather than silently dropping the signal.
var diagnosisRuleMap = map[string]string{
	diagnosis.RuleImagePullBackOff:                 "diag.pod.image_pull_backoff.v1",
	diagnosis.RuleCrashLoopBackOff:                 "diag.pod.crash_loop_backoff.v1",
	diagnosis.RulePodPending:                       "diag.pod.pending.v1",
	diagnosis.RulePodOOMKilled:                     "diag.pod.oom_killed.v1",
	diagnosis.RuleServiceNoEndpoints:               "diag.service.no_ready_endpoints.v1",
	diagnosis.RuleNodeNotReady:                     "diag.node.not_ready.v1",
	diagnosis.RuleDeploymentReplicasUnavailable:    "diag.deployment.replicas_unavailable.v1",
	diagnosis.RuleNodePressure:                     "diag.node.pressure.v1",
	diagnosis.RulePersistentVolumeClaimPending:     "diag.persistentvolumeclaim.pending.v1",
	diagnosis.RuleHorizontalPodAutoscalerSaturated: "diag.horizontalpodautoscaler.saturated.v1",
	diagnosis.RuleIngressBackendUnavailable:        "diag.ingress.backend_unavailable.v1",
}

// FromRecord builds an IngestRequest from a diagnosis Record. Returns an
// error when the rule is not in the compiled map.
func (DiagnosisNormalizer) FromRecord(r diagnosis.Record, ingestionRunID string) (IngestRequest, error) {
	signalID, ok := diagnosisRuleMap[r.RuleID]
	if !ok {
		return IngestRequest{}, fmt.Errorf("diagnosis rule %q has no signal mapping", r.RuleID)
	}
	state := StateActive
	switch r.Status {
	case "resolved":
		state = StateResolved
	case "dismissed":
		state = StateDismissed
	}
	return IngestRequest{
		SignalID:       signalID,
		Producer:       ProducerDiagnosis,
		ClusterID:      r.ClusterID,
		Namespace:      r.Resource.Namespace,
		Resource:       ResourceCitation{Kind: r.Resource.Kind, Namespace: r.Resource.Namespace, Name: r.Resource.Name, UID: r.Resource.UID},
		Severity:       r.Severity,
		State:          state,
		Coverage:       CoverageComplete,
		Freshness:      r.ObservedAt,
		ObservedAt:     r.ObservedAt,
		Attributes:     map[string]string{"summary": r.Summary, "status": r.Status},
		Evidence:       []EvidenceRef{{Kind: "diagnosis_record", ID: r.ID}},
		IngestionRunID: ingestionRunID,
	}, nil
}
