package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

func newAlertOverviewRouter(repo alertRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := alert.NewService(repo, alertDiagnosisRepoStub{}, alertMetricEvaluatorStub{}, time.Minute)
	h := &alertHandler{service: service, users: &auth.Service{}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "alert-overview-test",
		}))
		c.Next()
	})
	router.GET("/api/v1/clusters/:cluster_id/alerts", h.listInstances)
	router.GET("/api/v1/clusters/:cluster_id/alerts/overview", h.overview)
	return router
}

func TestAlertOverview_QueryValidation(t *testing.T) {
	router := newAlertOverviewRouter(alertRepoStub{})
	cases := []string{
		"/api/v1/clusters/1/alerts/overview?window_minutes=0",
		"/api/v1/clusters/1/alerts/overview?window_minutes=99999",
		"/api/v1/clusters/1/alerts/overview?max_groups=0",
		"/api/v1/clusters/1/alerts/overview?max_groups=999",
		"/api/v1/clusters/1/alerts/overview?limit=0",
		"/api/v1/clusters/1/alerts/overview?limit=9999",
	}
	for _, path := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("path=%s status=%d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestAlertOverview_AggregatesInstances(t *testing.T) {
	now := time.Now().UTC()
	router := newAlertOverviewRouter(alertRepoStub{
		rules: []alert.Rule{
			{ID: 1, ClusterID: 1, DisplayName: "CPU high", ResourceKind: "Node", ResourceName: "demo-node", MetricName: "cpu"},
			{ID: 2, ClusterID: 1, DisplayName: "Mem high", ResourceKind: "Node", ResourceName: "demo-node", MetricName: "mem"},
		},
		instances: []alert.Instance{
			{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-2 * time.Hour), LastFiredAt: now.Add(-5 * time.Minute)},
			{RuleID: 1, State: "firing", FirstFiredAt: now.Add(-3 * time.Hour), LastFiredAt: now.Add(-1 * time.Minute)},
			{RuleID: 2, State: "resolved", FirstFiredAt: now.Add(-4 * time.Hour), LastFiredAt: now.Add(-3 * time.Hour)},
		},
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alerts/overview?window_minutes=1440&max_groups=50&limit=100", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"groups_total":2`) {
		t.Fatalf("expected 2 groups, got %s", body)
	}
	if !strings.Contains(body, `"firing_count":2`) || !strings.Contains(body, `"resolved_count":1`) {
		t.Fatalf("expected firing/resolved counts, got %s", body)
	}
	if !strings.Contains(body, `"display_name":"CPU high"`) {
		t.Fatalf("expected display name from rule join, got %s", body)
	}
	if !strings.Contains(body, `"fail_closed":false`) {
		t.Fatalf("expected fail_closed=false, got %s", body)
	}
}

func TestAlertOverview_FailClosedEmpty(t *testing.T) {
	router := newAlertOverviewRouter(alertRepoStub{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alerts/overview", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"fail_closed":true`) {
		t.Fatalf("expected fail_closed=true, got %s", rec.Body.String())
	}
}

func TestAlertOverview_InvalidClusterID(t *testing.T) {
	router := newAlertOverviewRouter(alertRepoStub{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/0/alerts/overview", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cluster id, got %d", rec.Code)
	}
}
