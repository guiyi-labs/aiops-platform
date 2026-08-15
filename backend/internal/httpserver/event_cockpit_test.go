package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestCockpit_DefaultsApplied(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	parsed, ok := parseCockpitRequest(c)
	if !ok || parsed.WindowMinutes != 1440 || parsed.MaxGroups != 50 || parsed.PageLimit != 500 {
		t.Fatalf("defaults = %+v ok=%v, want {1440 50 500} true", parsed, ok)
	}
}

func TestCockpit_ValidExplicitParams(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit?window_minutes=60&max_groups=10&page_limit=100", nil)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	parsed, ok := parseCockpitRequest(c)
	if !ok || parsed.WindowMinutes != 60 || parsed.MaxGroups != 10 || parsed.PageLimit != 100 {
		t.Fatalf("explicit = %+v ok=%v, want {60 10 100} true", parsed, ok)
	}
}

func TestCockpit_NaNParamsRejected(t *testing.T) {
	r := newCockpitTestEngine(t)
	for _, query := range []string{
		"?window_minutes=abc", "?max_groups=abc", "?page_limit=abc",
		"?window_minutes=0", "?max_groups=0", "?page_limit=0",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/cockpit"+query, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("query=%s status = %d, want 400", query, w.Code)
		}
	}
}

func TestParseEventTimeFormats(t *testing.T) {
	if !parseEventTime("2026-08-14T08:00:00Z").Equal(time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)) {
		t.Fatal("RFC3339 parse failed")
	}
	if !parseEventTime("2026-08-14T08:00:00Z").Equal(time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)) {
		t.Fatal("Z-suffixed parse failed")
	}
	if !parseEventTime("").IsZero() || !parseEventTime("bogus").IsZero() {
		t.Fatal("invalid timestamps must be zero")
	}
}
