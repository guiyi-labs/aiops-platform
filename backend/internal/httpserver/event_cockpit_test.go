package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newCockpitTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(1))
		c.Set("workspace_id", int64(1))
		c.Set("workspace_roles", map[int64][]string{1: {"admin"}})
		c.Next()
	})
	h := kubernetesHandler{}
	// Register cockpit route only (no real k8s service needed for param tests).
	r.GET("/api/v1/clusters/:cluster_id/events/cockpit", h.eventCockpit)
	return r
}

func TestCockpit_BadWindowMinutes(t *testing.T) {
	r := newCockpitTestEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit?window_minutes=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_WINDOW") {
		t.Fatalf("expected 400 INVALID_WINDOW, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCockpit_BadMaxGroups(t *testing.T) {
	r := newCockpitTestEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit?max_groups=500", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_GROUPS") {
		t.Fatalf("expected 400 INVALID_GROUPS, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCockpit_BadPageLimit(t *testing.T) {
	r := newCockpitTestEngine(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit?page_limit=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "INVALID_LIMIT") {
		t.Fatalf("expected 400 INVALID_LIMIT, got %d: %s", w.Code, w.Body.String())
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
