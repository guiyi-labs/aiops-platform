package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/audit"
)

type auditQueryRepositoryStub struct{ response audit.ListResponse }

func (s auditQueryRepositoryStub) Save(context.Context, *audit.Entry) error { return nil }
func (s auditQueryRepositoryStub) List(context.Context, audit.Filter) (audit.ListResponse, error) {
	return s.response, nil
}

type auditRecorderStub struct{ entries []audit.Entry }

func (s *auditRecorderStub) Record(_ *gin.Context, entry *audit.Entry) error {
	s.entries = append(s.entries, *entry)
	return nil
}

func TestAuditedOperation(t *testing.T) {
	tests := []struct {
		method, path, action string
	}{
		{method: http.MethodPost, path: "/api/v1/clusters", action: "cluster.create"},
		{method: http.MethodPatch, path: "/api/v1/diagnoses/:diagnosis_id", action: "diagnosis.status.update"},
		{method: http.MethodPatch, path: "/api/v1/diagnoses/:diagnosis_id/assignment", action: "diagnosis.assignment.update"},
		{method: http.MethodPost, path: "/api/v1/diagnoses/:diagnosis_id/explanations", action: "diagnosis.ai_explanation.create"},
		{method: http.MethodPost, path: "/api/v1/ai/explanations/:explanation_id/feedback", action: "ai_explanation.feedback.create"},
		{method: http.MethodGet, path: "/api/v1/audit-logs/export", action: "audit.export"},
		{method: http.MethodPost, path: "/api/v1/users", action: "user.create"},
		{method: http.MethodPatch, path: "/api/v1/users/:user_id", action: "user.update"},
		{method: http.MethodPut, path: "/api/v1/clusters/:cluster_id/credentials", action: "cluster.credentials.rotate"},
		{method: http.MethodPost, path: "/api/v1/users/:user_id/password-reset", action: "user.password.reset"},
		{method: http.MethodPost, path: "/api/v1/auth/login", action: "auth.login"},
		{method: http.MethodPost, path: "/api/v1/auth/password-change", action: "auth.password.change"},
		{method: http.MethodDelete, path: "/api/v1/auth/sessions/:session_id", action: "auth.session.revoke"},
		{method: http.MethodPost, path: "/api/v1/auth/sessions/revoke-others", action: "auth.sessions.revoke_others"},
		{method: http.MethodPost, path: "/api/v1/notification-deliveries/:delivery_id/retry", action: "notification.delivery.retry"},
		{method: http.MethodPost, path: "/api/v1/diagnoses/:diagnosis_id/remediations/preview", action: "remediation.preview"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/operations/preview", action: "operation.preview"},
		{method: http.MethodPost, path: "/api/v1/remediations/:remediation_id/execute", action: "remediation.execute"},
		{method: http.MethodPost, path: "/api/v1/fleet/resources/search/filters", action: "global_search_filter.create"},
		{method: http.MethodPatch, path: "/api/v1/fleet/resources/search/filters/:filter_id", action: "global_search_filter.update"},
		{method: http.MethodDelete, path: "/api/v1/fleet/resources/search/filters/:filter_id", action: "global_search_filter.delete"},
	}
	for _, tt := range tests {
		action, _, ok := auditedOperation(tt.method, tt.path)
		if !ok || action != tt.action {
			t.Fatalf("auditedOperation(%q, %q) = %q, %v", tt.method, tt.path, action, ok)
		}
	}
	if _, _, ok := auditedOperation(http.MethodGet, "/api/v1/clusters"); ok {
		t.Fatal("read-only request was audited as a mutation")
	}
}

func TestAuditResult(t *testing.T) {
	if auditResult(http.StatusCreated) != "success" || auditResult(http.StatusForbidden) != "denied" || auditResult(http.StatusConflict) != "failure" {
		t.Fatal("unexpected audit result mapping")
	}
}

func TestAuditTrailRecordsDeniedMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &auditRecorderStub{}
	router := gin.New()
	router.Use(withRequestID(), auditTrail(zaptest.NewLogger(t), recorder))
	router.POST("/api/v1/auth/login", func(c *gin.Context) { c.Status(http.StatusUnauthorized) })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	router.ServeHTTP(response, request)

	if len(recorder.entries) != 1 || recorder.entries[0].Action != "auth.login" || recorder.entries[0].Result != "denied" || recorder.entries[0].RequestID == "" {
		t.Fatalf("entries = %#v", recorder.entries)
	}
}

func TestAuditExportReturnsBoundedCSVHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := audit.NewService(auditQueryRepositoryStub{response: audit.ListResponse{Total: 2, Remaining: 1, Items: []audit.Entry{{ID: 1, Actor: audit.Actor{Name: "Admin"}, Action: "cluster.probe", Result: "success", RequestID: "req-1", StatusCode: 200, Details: map[string]any{}, CreatedAt: time.Now()}}}})
	router := gin.New()
	router.GET("/api/v1/audit-logs/export", auditHandler{service: service}.export)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs/export?limit=1", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("X-Audit-Export-Rows") != "1" || response.Header().Get("X-Audit-Export-Truncated") != "true" {
		t.Fatalf("status=%d headers=%#v", response.Code, response.Header())
	}
	if !strings.HasPrefix(response.Header().Get("Content-Disposition"), `attachment; filename="audit-logs-`) || !strings.HasPrefix(response.Body.String(), "\uFEFFid,created_at") {
		t.Fatalf("headers=%#v body=%q", response.Header(), response.Body.String())
	}
}
