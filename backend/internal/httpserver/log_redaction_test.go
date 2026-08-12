package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"k8s-aiops.local/backend/internal/audit"
)

type auditExportRepositoryStub struct{ response audit.ListResponse }

func (s auditExportRepositoryStub) Save(context.Context, *audit.Entry) error { return nil }
func (s auditExportRepositoryStub) List(context.Context, audit.Filter) (audit.ListResponse, error) {
	return s.response, nil
}

// bufferLogger builds a zap logger that appends every encoded log line to buf.
func bufferLogger(buf *bytes.Buffer) *zap.Logger {
	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	return zap.New(zapcore.NewCore(encoder, zapcore.AddSync(buf), zapcore.DebugLevel))
}

// M100-C: the request logger must only emit routing metadata (method, path,
// status, duration, sizes, client IP, request id) and never query strings,
// headers, cookies or request bodies — the paths where credentials could
// otherwise leak into logs.
func TestRequestLoggerOmitsSensitiveMaterial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	engine := gin.New()
	engine.Use(requestLogger(bufferLogger(&buf)))
	engine.GET("/api/v1/test/echo", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	engine.POST("/api/v1/test/echo", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	const (
		querySecret  = "SecretQueryToken-abc123"
		headerSecret = "SecretBearerToken-xyz789"
		cookieSecret = "SecretRefreshCookie-def456"
		bodySecret   = "SecretPassword-ghi012"
		auditSecret  = "SecretActorValue-jkl345"
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test/echo?token="+querySecret+"&password="+bodySecret, nil)
	req.Header.Set("Authorization", "Bearer "+headerSecret)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: cookieSecret})
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}

	body, _ := json.Marshal(map[string]string{"current_password": "old-" + bodySecret, "new_password": bodySecret})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/test/echo", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+headerSecret)
	engine.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST status = %d", rec2.Code)
	}

	logs := buf.String()
	for _, want := range []string{"/api/v1/test/echo", "GET", "POST"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("request logger missing routing metadata %q in:\n%s", want, logs)
		}
	}
	for _, secret := range []string{querySecret, headerSecret, cookieSecret, bodySecret, auditSecret} {
		if strings.Contains(logs, secret) {
			t.Fatalf("request logger leaked secret %q in:\n%s", secret, logs)
		}
	}
	for _, fragment := range []string{"?token=", "token=", "password=", refreshCookieName, "Authorization", "current_password", "new_password"} {
		if strings.Contains(logs, fragment) {
			t.Fatalf("request logger leaked sensitive fragment %q in:\n%s", fragment, logs)
		}
	}
}

// M100-C: audit entries must carry only actor/action/resource metadata; the
// request body, credentials and other request material must never appear in
// Details or resource fields, even for credential-adjacent actions like
// password change.
func TestAuditTrailEntriesNeverCarryRequestCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetRouteTable()
	routeTable = append(routeTable, registeredRoute{
		Method:        http.MethodPost,
		FullPath:      "/api/v1/auth/password-change",
		AuditAction:   "auth.password.change",
		AuditResource: "UserCredential",
	})
	defer resetRouteTable()

	var buf bytes.Buffer
	recorder := &auditRecorderStub{}
	engine := gin.New()
	engine.Use(auditTrail(bufferLogger(&buf), recorder))
	engine.POST("/api/v1/auth/password-change", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	const newSecret = "BrandNewSecretPassword-987654"
	body, _ := json.Marshal(map[string]string{"current_password": "OldSecretPassword-123456", "new_password": newSecret})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-change", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer SecretBearerToken-abc123")
	engine.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(recorder.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(recorder.entries))
	}
	entry := recorder.entries[0]
	if entry.Action != "auth.password.change" || entry.Resource.Type != "UserCredential" {
		t.Fatalf("entry = %#v", entry)
	}
	rendered, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{newSecret, "OldSecretPassword-123456", "SecretBearerToken-abc123"} {
		if strings.Contains(string(rendered), secret) {
			t.Fatalf("audit entry leaked %q: %s", secret, rendered)
		}
	}
	for _, key := range []string{"current_password", "new_password"} {
		if strings.Contains(string(rendered), key) {
			t.Fatalf("audit entry leaked field %q: %s", key, rendered)
		}
	}
	// Details must stay a fixed closed set of routing metadata.
	for key := range entry.Details {
		switch key {
		case "method", "path_template", "cluster_id":
		default:
			t.Fatalf("audit entry Details carried unexpected key %q: %#v", key, entry.Details)
		}
	}
	if got := entry.Details["method"]; got != http.MethodPost {
		t.Fatalf("Details method = %v", got)
	}
}

func TestAuditCSVExportRedactsAndNeutralizesSensitiveCells(t *testing.T) {
	// M100-C: the audit CSV exporter must keep stable columns and never emit
	// raw request material; formula-safe cells are covered by the audit
	// service tests, this guards the handler wiring end to end.
	entries := audit.ListResponse{Items: []audit.Entry{
		{Actor: audit.Actor{ID: 1, Name: "admin"}, Action: "auth.password.change", Resource: audit.ResourceRef{Type: "UserCredential"},
			Result: "success", RequestID: "req-1", StatusCode: 200, IPAddress: "127.0.0.1", UserAgent: "test-agent",
			Details:   map[string]any{"method": "POST", "path_template": "/api/v1/auth/password-change", "cluster_id": int64(0)},
			CreatedAt: time.Date(2026, 8, 12, 4, 0, 0, 0, time.UTC)},
	}}
	handler := auditHandler{service: audit.NewService(auditExportRepositoryStub{response: entries})}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/audit-logs/export?format=csv", nil)
	handler.export(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d", rec.Code)
	}
	csv := strings.TrimPrefix(rec.Body.String(), "\uFEFF")
	for _, want := range []string{"auth.password.change", "UserCredential", "path_template"} {
		if !strings.Contains(csv, want) {
			t.Fatalf("export missing %q: %s", want, csv)
		}
	}
	for _, secret := range []string{"BrandNewSecretPassword", "OldSecretPassword-123456"} {
		if strings.Contains(csv, secret) {
			t.Fatalf("export leaked %q: %s", secret, csv)
		}
	}
}
