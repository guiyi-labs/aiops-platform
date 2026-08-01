package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/automation"
	"k8s-aiops.local/backend/internal/requestctx"
)

// newAutomationTestEngine builds a gin engine wired to the automation
// handler, mirroring the route shapes registered in router.go.
func newAutomationTestEngine(t *testing.T, svc *automation.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := automationHandler{service: svc}
	api := r.Group("/api/v1/aiops/automation")
	api.GET("/runbooks", h.listRunbooks)
	api.GET("/plans", h.listPlans)
	api.POST("/plans", h.createPlan)
	api.GET("/plans/:plan_id", h.getPlan)
	api.POST("/plans/:plan_id/preview", h.previewPlan)
	api.POST("/plans/:plan_id/approve", h.approvePlan)
	api.POST("/plans/:plan_id/execute", h.executePlan)
	api.POST("/plans/:plan_id/cancel", h.cancelPlan)
	api.POST("/plans/:plan_id/verify", h.verifyPlan)
	api.GET("/plans/:plan_id/verification", h.getVerification)
	return r
}

// withAutomationActor attaches actor metadata to the request context.
func withAutomationActor(req *http.Request, actorID int64, actorName string) *http.Request {
	return req.WithContext(requestctx.WithMetadata(req.Context(), requestctx.Metadata{
		ActorID:          actorID,
		ActorName:        actorName,
		ActorDisplayName: actorName,
		RequestID:        "automation-test",
	}))
}

// automationCaseReader is a test-only automation.CaseReader.
type automationCaseReader struct {
	ctx   automation.CaseContext
	codes map[string]bool
	err   error
}

func (r automationCaseReader) GetCase(context.Context, int64) (automation.CaseContext, error) {
	if r.err != nil {
		return automation.CaseContext{}, r.err
	}
	return r.ctx, nil
}

func (r automationCaseReader) EligibleActionCodes(context.Context, int64) (map[string]bool, error) {
	return r.codes, nil
}

func automationCaseContext() automation.CaseContext {
	return automation.CaseContext{
		CaseID:           42,
		ClusterID:        1,
		PrimaryKind:      "Deployment",
		PrimaryNamespace: "default",
		PrimaryName:      "web",
		PrimaryUID:       "uid-deployment-1",
	}
}

// TestAutomationHandler_ListRunbooksReturns200 verifies the runbook catalog
// endpoint returns the server-owned executable catalog.
func TestAutomationHandler_ListRunbooksReturns200(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/runbooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items             []automation.RunbookDescriptor `json:"items"`
		AutomationVersion string                         `json:"automation_version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Error("expected at least one runbook in catalog")
	}
	if resp.AutomationVersion != automation.AutomationVersion {
		t.Errorf("automation_version: want %s, got %s", automation.AutomationVersion, resp.AutomationVersion)
	}
}

// TestAutomationHandler_ListRunbooksReturns503WhenServiceNil verifies the
// handler returns 503 when the service is not configured.
func TestAutomationHandler_ListRunbooksReturns503WhenServiceNil(t *testing.T) {
	r := newAutomationTestEngine(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/runbooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// TestAutomationHandler_ListPlansReturns200 verifies the list endpoint
// returns the paginated response.
func TestAutomationHandler_ListPlansReturns200(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans?limit=50", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp automation.ActionPlanListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected total=0 for nop repo, got %d", resp.Total)
	}
}

// TestAutomationHandler_ListPlansRejectsInvalidLimit verifies the list
// endpoint rejects invalid limit values.
func TestAutomationHandler_ListPlansRejectsInvalidLimit(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans?limit=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestAutomationHandler_ListPlansRejectsInvalidCaseID verifies the list
// endpoint rejects invalid case_id values.
func TestAutomationHandler_ListPlansRejectsInvalidCaseID(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans?case_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestAutomationHandler_CreatePlanRejectsMissingFields verifies the create
// endpoint requires case_id and runbook_id.
func TestAutomationHandler_CreatePlanRejectsMissingFields(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, automationCaseReader{ctx: automationCaseContext(), codes: map[string]bool{"deployment.rollback": true}}, nil)
	r := newAutomationTestEngine(t, svc)
	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans", body)
	req.Header.Set("Content-Type", "application/json")
	req = withAutomationActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_CreatePlanRejectsUnknownRunbook verifies the create
// endpoint rejects runbook IDs that are not in the catalog.
func TestAutomationHandler_CreatePlanRejectsUnknownRunbook(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, automationCaseReader{ctx: automationCaseContext()}, nil)
	r := newAutomationTestEngine(t, svc)
	body := bytes.NewReader([]byte(`{"case_id":42,"runbook_id":"nonexistent_runbook"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans", body)
	req.Header.Set("Content-Type", "application/json")
	req = withAutomationActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_CreatePlanRejectsWhenCaseNotFound verifies the create
// endpoint returns 404 when the case does not exist.
func TestAutomationHandler_CreatePlanRejectsWhenCaseNotFound(t *testing.T) {
	reader := automationCaseReader{err: automation.ErrCaseNotFound}
	svc := automation.NewService(automation.NopRepository{}, reader, nil)
	r := newAutomationTestEngine(t, svc)
	body := bytes.NewReader([]byte(`{"case_id":99,"runbook_id":"rollback_last_rollout"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans", body)
	req.Header.Set("Content-Type", "application/json")
	req = withAutomationActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_CreatePlanRejectsIneligibleRunbook verifies the create
// endpoint returns 409 when the runbook is not eligible for the case.
func TestAutomationHandler_CreatePlanRejectsIneligibleRunbook(t *testing.T) {
	// CaseReader returns codes that don't include deployment.rollback
	reader := automationCaseReader{
		ctx:   automationCaseContext(),
		codes: map[string]bool{"deployment.scale": true}, // rollback not eligible
	}
	svc := automation.NewService(automation.NopRepository{}, reader, nil)
	r := newAutomationTestEngine(t, svc)
	body := bytes.NewReader([]byte(`{"case_id":42,"runbook_id":"rollback_last_rollout"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans", body)
	req.Header.Set("Content-Type", "application/json")
	req = withAutomationActor(req, 1, "alice")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_GetPlanRejectsInvalidPlanID verifies the get endpoint
// rejects malformed plan IDs.
func TestAutomationHandler_GetPlanRejectsInvalidPlanID(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestAutomationHandler_GetPlanReturns404WhenNotFound verifies the get
// endpoint returns 404 when the plan does not exist.
func TestAutomationHandler_GetPlanReturns404WhenNotFound(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_PreviewRejectsInvalidPlanID verifies the preview
// endpoint rejects malformed plan IDs.
func TestAutomationHandler_PreviewRejectsInvalidPlanID(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/short/preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestAutomationHandler_PreviewReturns404WhenNotFound verifies the preview
// endpoint returns 404 when the plan does not exist.
func TestAutomationHandler_PreviewReturns404WhenNotFound(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_ApproveRejectsInvalidPlanID verifies the approve
// endpoint rejects malformed plan IDs.
func TestAutomationHandler_ApproveRejectsInvalidPlanID(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/bad/approve", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestAutomationHandler_ExecuteRejectsMissingConfirmationToken verifies the
// execute endpoint requires a confirmation token in the body.
func TestAutomationHandler_ExecuteRejectsMissingConfirmationToken(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	body := bytes.NewReader([]byte(`{}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/execute", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_CancelReturns404WhenNotFound verifies the cancel
// endpoint returns 404 when the plan does not exist.
func TestAutomationHandler_CancelReturns404WhenNotFound(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_VerifyReturns404WhenNotFound verifies the verify
// endpoint returns 404 when the plan does not exist.
func TestAutomationHandler_VerifyReturns404WhenNotFound(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/verify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_GetVerificationReturns404WhenNotFound verifies the
// get verification endpoint returns 404 when no verification exists.
func TestAutomationHandler_GetVerificationReturns404WhenNotFound(t *testing.T) {
	svc := automation.NewService(automation.NopRepository{}, nil, nil)
	r := newAutomationTestEngine(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/automation/plans/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/verification", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAutomationHandler_WriteErrorMapsSentinelErrors verifies the error
// mapper produces the correct HTTP status codes for each sentinel error.
func TestAutomationHandler_WriteErrorMapsSentinelErrors(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"plan_not_found", automation.ErrPlanNotFound, http.StatusNotFound},
		{"verification_not_found", automation.ErrVerificationNotFound, http.StatusNotFound},
		{"case_not_found", automation.ErrCaseNotFound, http.StatusNotFound},
		{"invalid_runbook", automation.ErrInvalidRunbook, http.StatusBadRequest},
		{"runbook_not_in_catalog", automation.ErrRunbookNotInCatalog, http.StatusBadRequest},
		{"advisory_not_executable", automation.ErrAdvisoryRunbookNotExecutable, http.StatusBadRequest},
		{"invalid_operation", automation.ErrInvalidOperation, http.StatusBadRequest},
		{"invalid_idempotency", automation.ErrInvalidIdempotency, http.StatusBadRequest},
		{"unsupported_action", automation.ErrUnsupportedAction, http.StatusBadRequest},
		{"unsupported_target_kind", automation.ErrUnsupportedTargetKind, http.StatusBadRequest},
		{"runbook_not_eligible", automation.ErrRunbookNotEligible, http.StatusConflict},
		{"operation_no_change", automation.ErrOperationNoChange, http.StatusConflict},
		{"no_rollback_point", automation.ErrNoRollbackPoint, http.StatusConflict},
		{"not_draft", automation.ErrNotDraft, http.StatusConflict},
		{"not_previewed", automation.ErrNotPreviewed, http.StatusConflict},
		{"not_approved", automation.ErrNotApproved, http.StatusConflict},
		{"not_verifiable", automation.ErrNotVerifiable, http.StatusConflict},
		{"policy_gate_failed", automation.ErrPolicyGateFailed, http.StatusConflict},
		{"target_changed", automation.ErrTargetChanged, http.StatusConflict},
		{"in_progress", automation.ErrInProgress, http.StatusConflict},
		{"already_executed", automation.ErrAlreadyExecuted, http.StatusConflict},
		{"self_approval_forbidden", automation.ErrSelfApprovalForbidden, http.StatusForbidden},
		{"confirmation_invalid", automation.ErrConfirmationInvalid, http.StatusForbidden},
		{"expired", automation.ErrExpired, http.StatusGone},
		{"execution_failed", automation.ErrExecutionFailed, http.StatusBadGateway},
		{"disabled", automation.ErrDisabled, http.StatusServiceUnavailable},
		{"unknown_error", errors.New("something broke"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			automationHandler{}.writeError(c, tc.err, "fallback")
			if c.Writer.Status() != tc.wantCode {
				t.Errorf("err=%v: want %d, got %d", tc.err, tc.wantCode, c.Writer.Status())
			}
		})
	}
}

// TestAutomationHandler_IsValidPlanID verifies the plan ID validation
// accepts valid UUIDs and rejects invalid ones.
func TestAutomationHandler_IsValidPlanID(t *testing.T) {
	valid := []string{
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		"12345678-1234-1234-1234-123456789012",
		"abcdef01-2345-6789-abcd-ef0123456789",
	}
	invalid := []string{
		"",
		"short",
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa",   // too short
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaaa", // too long
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaZ",  // bad char
		"aaaaaaaa-aaaa-aaaa-aaaa_aaaaaaaaaaaa",  // bad separator
	}
	for _, id := range valid {
		if !isValidPlanID(id) {
			t.Errorf("expected valid: %s", id)
		}
	}
	for _, id := range invalid {
		if isValidPlanID(id) {
			t.Errorf("expected invalid: %s", id)
		}
	}
}

// TestAutomationHandler_BuildChangePreview verifies the change preview
// builder for each action code.
func TestAutomationHandler_BuildChangePreview(t *testing.T) {
	// deployment.scale
	before := int32(2)
	desired := int32(5)
	plan := automation.ActionPlan{
		ActionCode:      "deployment.scale",
		BeforeReplicas:  &before,
		DesiredReplicas: &desired,
	}
	change := buildChangePreview(plan)
	if change == nil || change.Field != "spec.replicas" {
		t.Errorf("scale: expected spec.replicas change, got %+v", change)
	}

	// deployment.rollback
	rev := int32(3)
	plan = automation.ActionPlan{
		ActionCode:       "deployment.rollback",
		RollbackRevision: &rev,
	}
	change = buildChangePreview(plan)
	if change == nil || change.Field != "spec.template (revision rollback)" {
		t.Errorf("rollback: expected revision rollback change, got %+v", change)
	}

	// Unknown action → nil
	plan = automation.ActionPlan{ActionCode: "unknown"}
	change = buildChangePreview(plan)
	if change != nil {
		t.Errorf("unknown: expected nil change, got %+v", change)
	}
}
