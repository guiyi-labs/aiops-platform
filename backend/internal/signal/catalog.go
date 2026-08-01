package signal

import "time"

// SchemaVersionV1 is the current signal envelope schema version.
const SchemaVersionV1 = "1.0"

// DefaultRetention is used when a descriptor does not override it.
const DefaultRetention = 30 * 24 * time.Hour

// catalog is the compiled, immutable registry of signal descriptors. An
// occurrence whose SignalID is not in this map fails closed at ingestion.
//
// Adding a descriptor is a contract change: it must be paired with a producer
// adapter, an OpenAPI update and unit tests. Never register a signal that no
// adapter emits.
var catalog = map[string]SignalDescriptor{
	// --- Diagnosis signals (M17/M18 deterministic hits) ---
	"diag.pod.image_pull_backoff.v1": {
		Code:          "diag.pod.image_pull_backoff.v1",
		SchemaVersion: SchemaVersionV1,
		Domain:        "workload",
		SeverityPolicy: SeverityPolicy{
			Mappings: map[string]Severity{"critical": SeverityCritical, "warning": SeverityWarning},
			Fallback: SeverityCritical,
		},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"rollout_restart", "scale_replicas"},
		Retention:        DefaultRetention,
		Description:      "Pod is in ImagePullBackOff state",
	},
	"diag.pod.crash_loop_backoff.v1": {
		Code:          "diag.pod.crash_loop_backoff.v1",
		SchemaVersion: SchemaVersionV1,
		Domain:        "workload",
		SeverityPolicy: SeverityPolicy{
			Mappings: map[string]Severity{"critical": SeverityCritical, "warning": SeverityWarning},
			Fallback: SeverityCritical,
		},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"rollout_restart", "scale_replicas"},
		Retention:        DefaultRetention,
		Description:      "Pod is in CrashLoopBackOff state",
	},
	"diag.pod.pending.v1": {
		Code:             "diag.pod.pending.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "workload",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Pod is Pending",
	},
	"diag.pod.oom_killed.v1": {
		Code:             "diag.pod.oom_killed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "workload",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityCritical},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"rollout_restart"},
		Retention:        DefaultRetention,
		Description:      "Pod was OOMKilled",
	},
	"diag.service.no_ready_endpoints.v1": {
		Code:             "diag.service.no_ready_endpoints.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "network",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityCritical},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Service has no ready endpoints",
	},
	"diag.node.not_ready.v1": {
		Code:             "diag.node.not_ready.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "node",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityCritical},
		CorrelationDims:  []string{"resource_uid", "cluster_id"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"node_maintenance"},
		Retention:        DefaultRetention,
		Description:      "Node is NotReady",
	},
	"diag.deployment.replicas_unavailable.v1": {
		Code:             "diag.deployment.replicas_unavailable.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "workload",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"rollout_restart", "scale_replicas"},
		Retention:        DefaultRetention,
		Description:      "Deployment replicas unavailable",
	},
	"diag.node.pressure.v1": {
		Code:             "diag.node.pressure.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "node",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{"node_maintenance"},
		Retention:        DefaultRetention,
		Description:      "Node is under memory/disk pressure",
	},
	"diag.persistentvolumeclaim.pending.v1": {
		Code:             "diag.persistentvolumeclaim.pending.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "storage",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "PVC is Pending",
	},
	"diag.horizontalpodautoscaler.saturated.v1": {
		Code:             "diag.horizontalpodautoscaler.saturated.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "workload",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "HPA is saturated at max replicas",
	},
	"diag.ingress.backend_unavailable.v1": {
		Code:             "diag.ingress.backend_unavailable.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "network",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"diagnosis_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Ingress backend is unavailable",
	},

	// --- Alert signals (M27 lifecycle transitions) ---
	"alert.firing.v1": {
		Code:             "alert.firing.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "metric",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "namespace", "resource_name"},
		RequiredEvidence: []string{"alert_instance"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Alert rule is firing",
	},
	"alert.resolved.v1": {
		Code:             "alert.resolved.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "metric",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace", "resource_name"},
		RequiredEvidence: []string{"alert_instance"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Alert rule transitioned to resolved",
	},

	// --- Metric breach signals (M21 sustained window) ---
	"metric.sustained_breach.v1": {
		Code:             "metric.sustained_breach.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "metric",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id", "namespace"},
		RequiredEvidence: []string{"metric_window"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Sustained metric breach detected by M21 evaluator",
	},

	// --- Posture signals (M29 governance findings) ---
	"posture.missing_quota.v1": {
		Code:             "posture.missing_quota.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "governance",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"posture_finding"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Namespace has no ResourceQuota",
	},
	"posture.exhausted_quota.v1": {
		Code:             "posture.exhausted_quota.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "governance",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"posture_finding"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Namespace ResourceQuota is exhausted",
	},
	"posture.node_pressure.v1": {
		Code:             "posture.node_pressure.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "node",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"resource_uid", "cluster_id"},
		RequiredEvidence: []string{"posture_finding"},
		AllowedActions:   []string{"node_maintenance"},
		Retention:        DefaultRetention,
		Description:      "Node under pressure (posture finding)",
	},
	"posture.missing_pdb.v1": {
		Code:             "posture.missing_pdb.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "governance",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"posture_finding"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Namespace workload has no PDB",
	},

	// --- Change outcome signals (M23-M31 platform operations) ---
	"change.promotion.completed.v1": {
		Code:             "change.promotion.completed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Cross-cluster promotion completed",
	},
	"change.promotion.failed.v1": {
		Code:             "change.promotion.failed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Cross-cluster promotion failed",
	},
	"change.backup.completed.v1": {
		Code:             "change.backup.completed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Velero backup completed",
	},
	"change.backup.failed.v1": {
		Code:             "change.backup.failed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Velero backup failed",
	},
	"change.maintenance.completed.v1": {
		Code:             "change.maintenance.completed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "resource_uid"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Node maintenance completed",
	},
	"change.maintenance.failed.v1": {
		Code:             "change.maintenance.failed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "resource_uid"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Node maintenance failed",
	},
	"change.restore.completed.v1": {
		Code:             "change.restore.completed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityInfo},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Workload restore rehearsal completed",
	},
	"change.restore.failed.v1": {
		Code:             "change.restore.failed.v1",
		SchemaVersion:    SchemaVersionV1,
		Domain:           "change",
		SeverityPolicy:   SeverityPolicy{Fallback: SeverityWarning},
		CorrelationDims:  []string{"cluster_id", "namespace"},
		RequiredEvidence: []string{"change_record"},
		AllowedActions:   []string{},
		Retention:        DefaultRetention,
		Description:      "Workload restore rehearsal failed",
	},
}

// Lookup returns the descriptor for a signal code. ok is false when the code
// is not registered; the service must fail closed in that case.
func Lookup(code string) (SignalDescriptor, bool) {
	d, ok := catalog[code]
	return d, ok
}

// All returns every registered descriptor. Used by the catalog endpoint and
// by tests that need to assert contract invariants.
func All() []SignalDescriptor {
	out := make([]SignalDescriptor, 0, len(catalog))
	for _, d := range catalog {
		out = append(out, d)
	}
	return out
}

// MapSeverity applies a descriptor's severity policy to a producer-local
// severity string.
func MapSeverity(d SignalDescriptor, producerSeverity string) Severity {
	if s, ok := d.SeverityPolicy.Mappings[producerSeverity]; ok {
		return s
	}
	return d.SeverityPolicy.Fallback
}
