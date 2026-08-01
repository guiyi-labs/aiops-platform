package signal

import (
	"fmt"

	"k8s-aiops.local/backend/internal/namespaceposture"
)

// PostureNormalizer converts M29 namespaceposture.Finding into signal
// occurrences. Posture findings are computed on demand and not persisted by
// M29, so the normalizer is a pure mapping function.
type PostureNormalizer struct{}

func NewPostureNormalizer() PostureNormalizer { return PostureNormalizer{} }

// postureCodeMap maps M29 Finding.Code constants to M39 signal ids.
var postureCodeMap = map[string]string{
	namespaceposture.CodeMissingQuota:   "posture.missing_quota.v1",
	namespaceposture.CodeExhaustedQuota: "posture.exhausted_quota.v1",
	namespaceposture.CodeNodePressure:   "posture.node_pressure.v1",
	namespaceposture.CodeNoMatchingPDB:  "posture.missing_pdb.v1",
}

// FromFinding builds an IngestRequest from a posture Finding. clusterID is
// required because M29 findings do not carry it. Returns ok=false when the
// finding code has no signal mapping.
func (PostureNormalizer) FromFinding(clusterID int64, ns string, f namespaceposture.Finding, ingestionRunID string) (IngestRequest, bool, error) {
	signalID, ok := postureCodeMap[f.Code]
	if !ok {
		return IngestRequest{}, false, nil
	}
	return IngestRequest{
		SignalID:  signalID,
		Producer:  ProducerPosture,
		ClusterID: clusterID,
		Namespace: ns,
		Resource: ResourceCitation{
			Kind:      f.Resource.Kind,
			Namespace: f.Resource.Namespace,
			Name:      f.Resource.Name,
			UID:       f.Resource.UID,
		},
		Severity:       f.Severity,
		State:          StateActive,
		Coverage:       CoverageComplete,
		Attributes:     map[string]string{"code": f.Code, "summary": f.Summary},
		IngestionRunID: ingestionRunID,
	}, true, nil
}

// ChangeNormalizer converts M23-M31 platform-operation outcomes into signal
// occurrences. The caller passes a typed ChangeInput so a single adapter
// covers promotion, backup, maintenance and restore.
type ChangeNormalizer struct{}

func NewChangeNormalizer() ChangeNormalizer { return ChangeNormalizer{} }

// ChangeKind enumerates the supported change sources.
type ChangeKind string

const (
	ChangeKindPromotion   ChangeKind = "promotion"
	ChangeKindBackup      ChangeKind = "backup"
	ChangeKindMaintenance ChangeKind = "maintenance"
	ChangeKindRestore     ChangeKind = "restore"
)

// ChangeInput is the typed input for ChangeNormalizer.FromOutcome.
type ChangeInput struct {
	Kind       ChangeKind
	ClusterID  int64
	Namespace  string
	Resource   ResourceCitation
	Status     string // "succeeded" | "failed" | "pending"
	PlanID     int64
	ActorName  string
	StartedAt  interface{ IsZero() bool }
	FinishedAt interface{ IsZero() bool }
}

// ChangeOutcome is a concrete, time-bearing version used by the adapter to
// avoid the interface dance above.
type ChangeOutcome struct {
	Kind       ChangeKind
	ClusterID  int64
	Namespace  string
	Resource   ResourceCitation
	Status     string
	PlanID     int64
	ActorName  string
	StartedAt  interface{}
	FinishedAt interface{}
}

// FromOutcome builds an IngestRequest from a change outcome. Returns
// ok=false for non-terminal statuses (only succeeded/failed produce signals).
func (ChangeNormalizer) FromOutcome(o ChangeOutcome, observedAt interface{}, ingestionRunID string) (IngestRequest, bool, error) {
	signalID, ok := changeSignalID(o.Kind, o.Status)
	if !ok {
		return IngestRequest{}, false, nil
	}
	return IngestRequest{
		SignalID:       signalID,
		Producer:       ProducerChange,
		ClusterID:      o.ClusterID,
		Namespace:      o.Namespace,
		Resource:       o.Resource,
		Severity:       changeSeverity(o.Status),
		State:          changeState(o.Status),
		Coverage:       CoverageComplete,
		Attributes:     map[string]string{"kind": string(o.Kind), "status": o.Status, "plan_id": fmt.Sprintf("%d", o.PlanID), "actor": o.ActorName},
		Evidence:       []EvidenceRef{{Kind: "change_record", ID: o.PlanID}},
		IngestionRunID: ingestionRunID,
	}, true, nil
}

func changeSignalID(k ChangeKind, status string) (string, bool) {
	switch k {
	case ChangeKindPromotion:
		if status == "succeeded" {
			return "change.promotion.completed.v1", true
		}
		if status == "failed" {
			return "change.promotion.failed.v1", true
		}
	case ChangeKindBackup:
		if status == "succeeded" {
			return "change.backup.completed.v1", true
		}
		if status == "failed" {
			return "change.backup.failed.v1", true
		}
	case ChangeKindMaintenance:
		if status == "succeeded" {
			return "change.maintenance.completed.v1", true
		}
		if status == "failed" {
			return "change.maintenance.failed.v1", true
		}
	case ChangeKindRestore:
		if status == "succeeded" {
			return "change.restore.completed.v1", true
		}
		if status == "failed" {
			return "change.restore.failed.v1", true
		}
	}
	return "", false
}

func changeSeverity(status string) string {
	if status == "failed" {
		return "warning"
	}
	return "info"
}

func changeState(status string) State {
	if status == "failed" {
		return StateActive
	}
	return StateResolved
}
