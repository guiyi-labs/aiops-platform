package automation

import "errors"

// Sentinel errors for the automation service. These mirror the
// remediation/maintenance patterns so HTTP handlers can map them to
// stable responses.
var (
	// ErrInvalidRunbook: runbook ID is empty or exceeds the length cap.
	ErrInvalidRunbook = errors.New("invalid runbook id")
	// ErrRunbookNotInCatalog: runbook ID is not in the M43 catalog.
	ErrRunbookNotInCatalog = errors.New("runbook not in catalog")
	// ErrAdvisoryRunbookNotExecutable: the runbook is advisory-only (no
	// action_code) and cannot be materialized into an action plan.
	ErrAdvisoryRunbookNotExecutable = errors.New("advisory runbook is not executable")
	// ErrRunbookNotEligible: the runbook's action code is not in the
	// case's M42 ActionCandidate list.
	ErrRunbookNotEligible = errors.New("runbook is not eligible for this case")
	// ErrNoRollbackPoint: no non-current ReplicaSet revision exists for
	// the target Deployment.
	ErrNoRollbackPoint = errors.New("no rollback point available")
	// ErrUnsupportedAction: action_code is not supported by the service.
	ErrUnsupportedAction = errors.New("unsupported automation action")
	// ErrUnsupportedTargetKind: target kind is not supported.
	ErrUnsupportedTargetKind = errors.New("unsupported target kind")
	// ErrInvalidOperation: operation parameters are invalid.
	ErrInvalidOperation = errors.New("controlled operation parameters are invalid")
	// ErrOperationNoChange: the operation would not change the target.
	ErrOperationNoChange = errors.New("controlled operation would not change the target")
	// ErrTargetChanged: the target UID/RV changed after preview.
	ErrTargetChanged = errors.New("automation target changed after preview")
	// ErrNotDraft: plan is not in draft status (required for Preview).
	ErrNotDraft = errors.New("action plan is not in draft status")
	// ErrNotPreviewed: plan is not in previewed status (required for
	// Approve).
	ErrNotPreviewed = errors.New("action plan is not in previewed status")
	// ErrSelfApprovalForbidden: requester attempted to self-approve a
	// four-eyes plan.
	ErrSelfApprovalForbidden = errors.New("requester cannot self-approve a four-eyes plan")
	// ErrPolicyGateFailed: one or more policy gates failed.
	ErrPolicyGateFailed = errors.New("policy gate failed")
	// ErrInvalidIdempotency: idempotency key is invalid (length outside
	// 8..128).
	ErrInvalidIdempotency = errors.New("idempotency key is invalid")
	// ErrExecutionFailed: Kubernetes patch failed.
	ErrExecutionFailed = errors.New("automation execution failed")
	// ErrNotVerifiable: plan is not in Succeeded/Failed status (required
	// for Verify).
	ErrNotVerifiable = errors.New("action plan is not in a verifiable status")
)
