package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/requestctx"
)

// automationHandler exposes the M44 policy-constrained automation service as
// the safe-execution ceiling of the AIOps Intelligence Plane. Routes cover the
// full action-plan lifecycle: list runbooks, create draft plan, preview policy
// gates, approve (single or four-eyes), execute idempotently with confirmation
// token, cancel, and verify post-action outcome.
//
// Routes:
//
//	GET  /api/v1/aiops/automation/runbooks                       — list executable runbook catalog
//	GET  /api/v1/aiops/automation/plans                          — list action plans (filterable)
//	POST /api/v1/aiops/automation/plans                          — create a draft action plan
//	GET  /api/v1/aiops/automation/plans/{plan_id}                — get one action plan
//	POST /api/v1/aiops/automation/plans/{plan_id}/preview        — evaluate policy gates, transition to previewed
//	POST /api/v1/aiops/automation/plans/{plan_id}/approve        — record approval (single or four-eyes)
//	POST /api/v1/aiops/automation/plans/{plan_id}/execute        — idempotent claim + Kubernetes patch
//	POST /api/v1/aiops/automation/plans/{plan_id}/cancel         — transition non-terminal plan to cancelled
//	POST /api/v1/aiops/automation/plans/{plan_id}/verify         — evaluate post-action evidence
//	GET  /api/v1/aiops/automation/plans/{plan_id}/verification   — get the linked verification
type automationHandler struct {
	service *automation.Service
}

// listRunbooks handles GET /api/v1/aiops/automation/runbooks.
// Returns the server-owned executable runbook catalog. The catalog is the
// single source of truth for which runbooks may be materialized into action
// plans; clients cannot supply arbitrary action codes.
func (h automationHandler) listRunbooks(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	runbooks := automation.AllRunbooks()
	c.JSON(http.StatusOK, gin.H{
		"items":              runbooks,
		"automation_version": automation.AutomationVersion,
	})
}

// createPlanRequest is the request body for POST /api/v1/aiops/automation/plans.
// Only fixed, server-owned fields are admitted: case_id, runbook_id, optional
// investigation/action_candidate linkage, and optional operation overrides.
// Clients cannot inject arbitrary patches, images or rollback revisions
// outside the runbook's action_code contract.
type createPlanRequest struct {
	CaseID             int64                           `json:"case_id" binding:"required"`
	RunbookID          string                          `json:"runbook_id" binding:"required"`
	InvestigationID    *int64                          `json:"investigation_id,omitempty"`
	ActionCandidateID  *int64                          `json:"action_candidate_id,omitempty"`
	OperationOverrides *automation.OperationParameters `json:"operation_overrides,omitempty"`
}

// createPlan handles POST /api/v1/aiops/automation/plans.
// Creates a draft action plan. The runbook must exist in the M43/M44 catalog
// and be eligible per the M42 Action Catalog at preview time.
func (h automationHandler) createPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	var request createPlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "case_id and runbook_id are required")
		return
	}
	if request.CaseID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_CASE_ID", "case_id must be a positive integer")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	input := automation.CreatePlanInput{
		CaseID:             request.CaseID,
		RunbookID:          strings.TrimSpace(request.RunbookID),
		InvestigationID:    request.InvestigationID,
		ActionCandidateID:  request.ActionCandidateID,
		Operator:           automation.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName},
		OperationOverrides: request.OperationOverrides,
	}
	setAuditTarget(c, "ActionPlan", "", strconv.FormatInt(request.CaseID, 10))
	plan, err := h.service.CreatePlan(c.Request.Context(), input)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
	}
	if plan.ID != "" {
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	if err != nil {
		h.writeError(c, err, "unable to create action plan")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// getPlan handles GET /api/v1/aiops/automation/plans/{plan_id}.
func (h automationHandler) getPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	plan, err := h.service.GetPlan(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "unable to get action plan")
		return
	}
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	c.JSON(http.StatusOK, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Approver:    plan.Approver(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// listPlans handles GET /api/v1/aiops/automation/plans.
//
// Query params (all optional):
//
//	case_id    (optional) — filter by correlation case
//	cluster_id (optional) — filter by cluster
//	namespace  (optional) — filter by target namespace
//	status     (optional) — filter by plan status (draft|previewed|approved|executing|succeeded|failed|expired|cancelled|verified)
//	runbook_id (optional) — filter by runbook ID
//	limit      (optional) — max plans, default 100, max 200
func (h automationHandler) listPlans(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	filter := automation.ActionPlanFilter{}
	if v := c.Query("case_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "case_id must be a positive integer")
			return
		}
		filter.CaseID = n
	}
	if v := c.Query("cluster_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "cluster_id must be a positive integer")
			return
		}
		filter.ClusterID = n
	}
	filter.Namespace = strings.TrimSpace(c.Query("namespace"))
	filter.Status = automation.PlanStatus(strings.TrimSpace(c.Query("status")))
	filter.RunbookID = strings.TrimSpace(c.Query("runbook_id"))
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 200 {
			writeError(c, http.StatusBadRequest, "INVALID_QUERY", "limit must be a positive integer <= 200")
			return
		}
		filter.Limit = n
	}
	resp, err := h.service.ListPlans(c.Request.Context(), filter)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "AUTOMATION_QUERY_FAILED", "failed to list action plans")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// previewPlan handles POST /api/v1/aiops/automation/plans/{plan_id}/preview.
// Evaluates the deterministic policy gates (UID/RV recheck, scope, PDB/blast
// radius, SLO burn, freeze window, concurrent plans, attempt cap, rollback
// point) and transitions a draft plan to previewed. The confirmation token is
// returned in the response body; it must be supplied at execute time.
func (h automationHandler) previewPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	setAuditTarget(c, "ActionPlan", "", id)
	plan, err := h.service.Preview(c.Request.Context(), id)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	if err != nil {
		h.writeError(c, err, "unable to preview action plan")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// approvePlan handles POST /api/v1/aiops/automation/plans/{plan_id}/approve.
// Records the approver and transitions a previewed plan to approved. For
// four-eyes plans (deployment.rollback, deployment.image_update), the approver
// must differ from the requester.
func (h automationHandler) approvePlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	approver := automation.ActorRef{ID: metadata.ActorID, Name: metadata.ActorDisplayName}
	setAuditTarget(c, "ActionPlan", "", id)
	plan, err := h.service.Approve(c.Request.Context(), id, approver)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	if err != nil {
		h.writeError(c, err, "unable to approve action plan")
		return
	}
	c.JSON(http.StatusOK, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Approver:    plan.Approver(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// executePlanRequest is the request body for POST /api/v1/aiops/automation/plans/{plan_id}/execute.
type executePlanRequest struct {
	ConfirmationToken string `json:"confirmation_token" binding:"required"`
}

// executePlan handles POST /api/v1/aiops/automation/plans/{plan_id}/execute.
// Rechecks the policy gates, takes an idempotent claim, applies the Kubernetes
// patch, and transitions the plan to succeeded or failed. Two workers and
// replay produce one business side effect: the claim is keyed by
// (id, idempotency_key, confirmation_token_hash).
func (h automationHandler) executePlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	var request executePlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "confirmation_token is required")
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	setAuditTarget(c, "ActionPlan", "", id)
	plan, err := h.service.Execute(c.Request.Context(), id, request.ConfirmationToken, idempotencyKey)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	if err != nil {
		h.writeError(c, err, "unable to execute action plan")
		return
	}
	c.JSON(http.StatusOK, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Approver:    plan.Approver(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// cancelPlan handles POST /api/v1/aiops/automation/plans/{plan_id}/cancel.
// Transitions a non-terminal plan (draft, previewed, approved) to cancelled.
func (h automationHandler) cancelPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	setAuditTarget(c, "ActionPlan", "", id)
	plan, err := h.service.Cancel(c.Request.Context(), id)
	if plan.ClusterID > 0 {
		setAuditClusterID(c, plan.ClusterID)
		setAuditTarget(c, "ActionPlan", plan.TargetNamespace, plan.ID)
	}
	if err != nil {
		h.writeError(c, err, "unable to cancel action plan")
		return
	}
	c.JSON(http.StatusOK, automation.ActionPlanResponse{
		ActionPlan:  plan,
		Target:      plan.Target(),
		RequestedBy: plan.RequestedBy(),
		Approver:    plan.Approver(),
		Parameters:  operationParametersFromPlan(plan),
		Change:      buildChangePreview(plan),
	})
}

// verifyPlan handles POST /api/v1/aiops/automation/plans/{plan_id}/verify.
// Evaluates the post-action verification for a plan in succeeded or failed
// status. The verifier captures the post-snapshot, compares against the
// pre-snapshot, and returns the final verification status. On a failed or
// ineffective verification, the server-owned rollback contract is evaluated.
func (h automationHandler) verifyPlan(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	setAuditTarget(c, "ActionVerification", "", id)
	verification, err := h.service.Verify(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "unable to verify action plan")
		return
	}
	c.JSON(http.StatusOK, verification)
}

// getVerification handles GET /api/v1/aiops/automation/plans/{plan_id}/verification.
// Returns the most recent verification linked to the plan.
func (h automationHandler) getVerification(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_UNAVAILABLE", "automation service is not configured")
		return
	}
	id := strings.TrimSpace(c.Param("plan_id"))
	if !isValidPlanID(id) {
		writeError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "plan_id must be a valid UUID")
		return
	}
	verification, err := h.service.GetVerification(c.Request.Context(), id)
	if err != nil {
		h.writeError(c, err, "unable to get verification")
		return
	}
	c.JSON(http.StatusOK, verification)
}

// writeError maps automation service errors to stable HTTP responses. The
// mapping mirrors the remediation/maintenance patterns: 404 for not-found,
// 409 for state conflicts (not draft, not previewed, in progress, already
// executed, target changed, operation no-change), 410 for expired, 403 for
// confirmation/self-approval failures, 400 for invalid input, 502 for
// execution failures, 503 for disabled.
func (automationHandler) writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, automation.ErrPlanNotFound):
		writeError(c, http.StatusNotFound, "ACTION_PLAN_NOT_FOUND", "action plan does not exist")
	case errors.Is(err, automation.ErrVerificationNotFound):
		writeError(c, http.StatusNotFound, "VERIFICATION_NOT_FOUND", "action verification does not exist")
	case errors.Is(err, automation.ErrCaseNotFound):
		writeError(c, http.StatusNotFound, "CASE_NOT_FOUND", "correlation case not found")
	case errors.Is(err, automation.ErrInvalidRunbook), errors.Is(err, automation.ErrInvalidOperation), errors.Is(err, automation.ErrInvalidIdempotency):
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
	case errors.Is(err, automation.ErrRunbookNotInCatalog):
		writeError(c, http.StatusBadRequest, "RUNBOOK_NOT_IN_CATALOG", "runbook is not in the executable catalog")
	case errors.Is(err, automation.ErrAdvisoryRunbookNotExecutable):
		writeError(c, http.StatusBadRequest, "ADVISORY_RUNBOOK_NOT_EXECUTABLE", "advisory runbooks cannot be materialized into action plans")
	case errors.Is(err, automation.ErrRunbookNotEligible):
		writeError(c, http.StatusConflict, "RUNBOOK_NOT_ELIGIBLE", "runbook action code is not eligible for this case")
	case errors.Is(err, automation.ErrUnsupportedAction), errors.Is(err, automation.ErrUnsupportedTargetKind):
		writeError(c, http.StatusBadRequest, "UNSUPPORTED_AUTOMATION", err.Error())
	case errors.Is(err, automation.ErrOperationNoChange):
		writeError(c, http.StatusConflict, "OPERATION_NO_CHANGE", "the target already has the requested value")
	case errors.Is(err, automation.ErrNoRollbackPoint):
		writeError(c, http.StatusConflict, "NO_ROLLBACK_POINT", "no non-current ReplicaSet revision exists for the target Deployment")
	case errors.Is(err, automation.ErrNotDraft):
		writeError(c, http.StatusConflict, "PLAN_NOT_DRAFT", "action plan is not in draft status")
	case errors.Is(err, automation.ErrNotPreviewed):
		writeError(c, http.StatusConflict, "PLAN_NOT_PREVIEWED", "action plan is not in previewed status")
	case errors.Is(err, automation.ErrNotApproved):
		writeError(c, http.StatusConflict, "PLAN_NOT_APPROVED", "action plan is not approved")
	case errors.Is(err, automation.ErrNotVerifiable):
		writeError(c, http.StatusConflict, "PLAN_NOT_VERIFIABLE", "action plan is not in a verifiable status")
	case errors.Is(err, automation.ErrSelfApprovalForbidden):
		writeError(c, http.StatusForbidden, "SELF_APPROVAL_FORBIDDEN", "requester cannot self-approve a four-eyes plan")
	case errors.Is(err, automation.ErrConfirmationInvalid):
		writeError(c, http.StatusForbidden, "CONFIRMATION_INVALID", "confirmation token is invalid")
	case errors.Is(err, automation.ErrPolicyGateFailed):
		writeError(c, http.StatusConflict, "POLICY_GATE_FAILED", "one or more policy gates failed")
	case errors.Is(err, automation.ErrTargetChanged):
		writeError(c, http.StatusConflict, "AUTOMATION_TARGET_CHANGED", "the target resource changed after preview")
	case errors.Is(err, automation.ErrExpired):
		writeError(c, http.StatusGone, "PLAN_EXPIRED", "action plan has expired")
	case errors.Is(err, automation.ErrInProgress):
		writeError(c, http.StatusConflict, "PLAN_IN_PROGRESS", "action plan execution is already in progress")
	case errors.Is(err, automation.ErrAlreadyExecuted):
		writeError(c, http.StatusConflict, "PLAN_ALREADY_USED", "action plan was already used with another idempotency key")
	case errors.Is(err, automation.ErrExecutionFailed):
		writeError(c, http.StatusBadGateway, "AUTOMATION_FAILED", "Kubernetes API rejected or failed the automation")
	case errors.Is(err, automation.ErrDisabled):
		writeError(c, http.StatusServiceUnavailable, "AUTOMATION_DISABLED", "automation service is disabled")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", fallback)
	}
}

// operationParametersFromPlan mirrors automation.toResponse but is exposed here
// so the handler can build the response without reaching into private fields.
func operationParametersFromPlan(plan automation.ActionPlan) automation.OperationParameters {
	return automation.OperationParameters{
		DesiredReplicas:  plan.DesiredReplicas,
		DesiredSuspended: plan.DesiredSuspended,
		ContainerName:    plan.ContainerName,
		BeforeImage:      plan.BeforeImage,
		DesiredImage:     plan.DesiredImage,
		RollbackRevision: plan.RollbackRevision,
	}
}

// buildChangePreview returns the preview diff for the plan's action. Mirrors
// automation.buildChange but exposed here so the handler does not need to
// call the unexported service helper.
func buildChangePreview(plan automation.ActionPlan) *automation.OperationChange {
	switch plan.ActionCode {
	case "deployment.scale":
		if plan.BeforeReplicas != nil && plan.DesiredReplicas != nil {
			return &automation.OperationChange{Field: "spec.replicas", Before: *plan.BeforeReplicas, After: *plan.DesiredReplicas}
		}
	case "cronjob.suspend", "cronjob.resume":
		if plan.BeforeSuspended != nil && plan.DesiredSuspended != nil {
			return &automation.OperationChange{Field: "spec.suspend", Before: *plan.BeforeSuspended, After: *plan.DesiredSuspended}
		}
	case "deployment.image_update":
		if plan.BeforeImage != "" && plan.DesiredImage != "" {
			return &automation.OperationChange{Field: "spec.template.spec.containers[" + plan.ContainerName + "].image", Before: plan.BeforeImage, After: plan.DesiredImage}
		}
	case "deployment.rollback":
		if plan.RollbackRevision != nil {
			return &automation.OperationChange{Field: "spec.template (revision rollback)", Before: "current", After: *plan.RollbackRevision}
		}
	}
	return nil
}

// isValidPlanID returns true when id looks like a UUID v4 (36 chars, 5 groups).
// Mirrors the remediation/maintenance plan ID validation.
func isValidPlanID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}
