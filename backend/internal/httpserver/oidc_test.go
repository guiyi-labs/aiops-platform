package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/auth"
)

// TestOIDCRoutesAbsentWhenDisabled proves the OIDC routes are not registered
// when the SessionManager is nil (the default, OIDC-disabled configuration).
// This is the local-only deployment invariant: no public API contract changes
// unless OIDC is explicitly enabled.
func TestOIDCRoutesAbsentWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine, ok := New(zaptest.NewLogger(t), Options{
		Probe: probeStub{},
		Auth:  &auth.Service{},
	}).(*gin.Engine)
	if !ok {
		t.Fatal("http server is not a gin engine")
	}

	registered := make(map[string]struct{})
	for _, route := range engine.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		"GET /api/v1/auth/oidc/login",
		"GET /api/v1/auth/oidc/callback",
		"POST /api/v1/auth/oidc/logout",
	} {
		if _, present := registered[route]; present {
			t.Fatalf("OIDC route %q is registered when OIDC is disabled", route)
		}
	}
}

// TestOIDCCallbackRejectsMissingInputs proves the callback handler fails closed
// with a 400 when the IdP redirect is malformed (missing code/state). It does
// not require a real OIDC provider: the handler validates query parameters
// before touching the session manager.
func TestOIDCCallbackRejectsMissingInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCode   string
	}{
		{name: "missing code and state", query: "", wantStatus: http.StatusBadRequest, wantCode: "OIDC_CALLBACK_INVALID"},
		{name: "missing state", query: "?code=abc", wantStatus: http.StatusBadRequest, wantCode: "OIDC_CALLBACK_INVALID"},
		{name: "missing code", query: "?state=xyz", wantStatus: http.StatusBadRequest, wantCode: "OIDC_CALLBACK_INVALID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			// Construct the handler without a real manager: the missing-input
			// guard runs before the manager is touched, so a nil manager is
			// safe for these cases.
			handler := oidcHandler{authHandler: authHandler{service: &auth.Service{}}}
			router.GET("/api/v1/auth/oidc/callback", handler.callback)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback"+tt.query, nil))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if body := recorder.Body.String(); !containsCode(body, tt.wantCode) {
				t.Fatalf("body = %q, want code %q", body, tt.wantCode)
			}
		})
	}
}

// TestOIDCCallbackRejectsExpiredSession proves the callback handler fails closed
// with an OIDC_SESSION_EXPIRED error when the auth-session cookie is missing.
// This guards against replay attacks where an attacker intercepts the callback
// URL without the accompanying PKCE cookie.
func TestOIDCCallbackRejectsExpiredSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := oidcHandler{authHandler: authHandler{service: &auth.Service{}}}
	router.GET("/api/v1/auth/oidc/callback", handler.callback)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?code=abc&state=xyz", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if body := recorder.Body.String(); !containsCode(body, "OIDC_SESSION_EXPIRED") {
		t.Fatalf("body = %q, want code OIDC_SESSION_EXPIRED", body)
	}
}

func containsCode(body, code string) bool {
	return len(body) > 0 && (body == code || indexOf(body, code) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
