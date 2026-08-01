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
		method, path, action, resource string
	}{
		{method: http.MethodPost, path: "/api/v1/clusters", action: "cluster.create", resource: "Cluster"},
		{method: http.MethodPatch, path: "/api/v1/diagnoses/:diagnosis_id", action: "diagnosis.status.update", resource: "Diagnosis"},
		{method: http.MethodPatch, path: "/api/v1/diagnoses/:diagnosis_id/assignment", action: "diagnosis.assignment.update", resource: "Diagnosis"},
		{method: http.MethodPost, path: "/api/v1/diagnoses/:diagnosis_id/explanations", action: "diagnosis.ai_explanation.create", resource: "DiagnosisAIExplanation"},
		{method: http.MethodPost, path: "/api/v1/ai/explanations/:explanation_id/feedback", action: "ai_explanation.feedback.create", resource: "AIExplanationFeedback"},
		{method: http.MethodGet, path: "/api/v1/audit-logs/export", action: "audit.export", resource: "AuditExport"},
		{method: http.MethodPost, path: "/api/v1/users", action: "user.create", resource: "User"},
		{method: http.MethodPatch, path: "/api/v1/users/:user_id", action: "user.update", resource: "User"},
		{method: http.MethodPut, path: "/api/v1/clusters/:cluster_id/credentials", action: "cluster.credentials.rotate", resource: "ClusterCredential"},
		{method: http.MethodPost, path: "/api/v1/users/:user_id/password-reset", action: "user.password.reset", resource: "User"},
		{method: http.MethodPost, path: "/api/v1/auth/login", action: "auth.login", resource: "Session"},
		{method: http.MethodPost, path: "/api/v1/auth/password-change", action: "auth.password.change", resource: "UserCredential"},
		{method: http.MethodDelete, path: "/api/v1/auth/sessions/:session_id", action: "auth.session.revoke", resource: "Session"},
		{method: http.MethodPost, path: "/api/v1/auth/sessions/revoke-others", action: "auth.sessions.revoke_others", resource: "Session"},
		{method: http.MethodPost, path: "/api/v1/notification-deliveries/:delivery_id/retry", action: "notification.delivery.retry", resource: "NotificationDelivery"},
		{method: http.MethodPost, path: "/api/v1/diagnoses/:diagnosis_id/remediations/preview", action: "remediation.preview", resource: "RemediationPlan"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/operations/preview", action: "operation.preview", resource: "ControlledOperation"},
		{method: http.MethodPost, path: "/api/v1/remediations/:remediation_id/execute", action: "remediation.execute", resource: "RemediationPlan"},
		{method: http.MethodPost, path: "/api/v1/fleet/resources/search/filters", action: "global_search_filter.create", resource: "GlobalSearchFilter"},
		{method: http.MethodPatch, path: "/api/v1/fleet/resources/search/filters/:filter_id", action: "global_search_filter.update", resource: "GlobalSearchFilter"},
		{method: http.MethodDelete, path: "/api/v1/fleet/resources/search/filters/:filter_id", action: "global_search_filter.delete", resource: "GlobalSearchFilter"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/alert-rules", action: "alert_rule.create", resource: "AlertRule"},
		{method: http.MethodPatch, path: "/api/v1/clusters/:cluster_id/alert-rules/:rule_id", action: "alert_rule.update", resource: "AlertRule"},
		{method: http.MethodDelete, path: "/api/v1/clusters/:cluster_id/alert-rules/:rule_id", action: "alert_rule.delete", resource: "AlertRule"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/backup-plans/preview", action: "backup.preview", resource: "BackupPlan"},
		{method: http.MethodPost, path: "/api/v1/backup-plans/:plan_id/execute", action: "backup.execute", resource: "BackupPlan"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/maintenance-plans/preview", action: "maintenance.preview", resource: "MaintenancePlan"},
		{method: http.MethodPost, path: "/api/v1/maintenance-plans/:plan_id/execute", action: "maintenance.execute", resource: "MaintenancePlan"},
		{method: http.MethodPost, path: "/api/v1/clusters/:cluster_id/restore-plans/preview", action: "restore.preview", resource: "RestorePlan"},
		{method: http.MethodPost, path: "/api/v1/restore-plans/:plan_id/execute", action: "restore.execute", resource: "RestorePlan"},
	}
	// Seed routeTable from the descriptor records so the lookup is driven by
	// the same table populated by routeRegistrar.register in production.
	resetRouteTable()
	for _, tt := range tests {
		routeTable = append(routeTable, registeredRoute{
			Method:        tt.method,
			FullPath:      tt.path,
			AuditAction:   tt.action,
			AuditResource: tt.resource,
		})
	}
	for _, tt := range tests {
		action, resource, ok := auditedOperation(tt.method, tt.path)
		if !ok || action != tt.action {
			t.Fatalf("auditedOperation(%q, %q) action = %q, %v", tt.method, tt.path, action, ok)
		}
		if resource != tt.resource {
			t.Fatalf("auditedOperation(%q, %q) resource = %q, want %q", tt.method, tt.path, resource, tt.resource)
		}
	}
	if _, _, ok := auditedOperation(http.MethodGet, "/api/v1/clusters"); ok {
		t.Fatal("read-only request was audited as a mutation")
	}
	resetRouteTable()
}

func TestAuditResult(t *testing.T) {
	if auditResult(http.StatusCreated) != "success" || auditResult(http.StatusForbidden) != "denied" || auditResult(http.StatusConflict) != "failure" {
		t.Fatal("unexpected audit result mapping")
	}
}

func TestAuditTrailRecordsDeniedMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Seed routeTable so the audit middleware can resolve auth.login from the
	// descriptor-driven lookup, mirroring how routeRegistrar.register populates
	// the table in the real router.
	resetRouteTable()
	routeTable = append(routeTable, registeredRoute{
		Method:        http.MethodPost,
		FullPath:      "/api/v1/auth/login",
		AuditAction:   "auth.login",
		AuditResource: "Session",
	})
	defer resetRouteTable()

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
