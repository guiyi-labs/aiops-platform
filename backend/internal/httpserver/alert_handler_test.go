package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/alert"
	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/requestctx"
)

type alertRepoStub struct {
	rule         alert.Rule
	ruleErr      error
	rules        []alert.Rule
	rulesErr     error
	createErr    error
	patchErr     error
	deleteErr    error
	instance     alert.Instance
	instanceErr  error
	instances    []alert.Instance
	instancesErr error
}

func (s alertRepoStub) CreateRule(_ context.Context, rule *alert.Rule, _ time.Duration) error {
	if s.createErr != nil {
		return s.createErr
	}
	rule.ID = 5
	return nil
}
func (s alertRepoStub) GetRule(context.Context, int64) (alert.Rule, error) { return s.rule, s.ruleErr }
func (s alertRepoStub) ListRules(context.Context, alert.RuleListFilter) ([]alert.Rule, error) {
	return s.rules, s.rulesErr
}
func (s alertRepoStub) PatchRule(context.Context, int64, alert.PatchRuleInput, alert.ActorRef) (alert.Rule, error) {
	return s.rule, s.patchErr
}
func (s alertRepoStub) DeleteRule(context.Context, int64) error { return s.deleteErr }
func (s alertRepoStub) GetUnresolvedInstance(context.Context, int64) (*alert.Instance, error) {
	return nil, nil
}
func (s alertRepoStub) CreateInstance(context.Context, *alert.Instance) error { return nil }
func (s alertRepoStub) CreateFiring(context.Context, *diagnosis.Record, *alert.Instance) error {
	return nil
}
func (s alertRepoStub) TouchInstance(context.Context, int64, time.Time, string) error { return nil }
func (s alertRepoStub) ResolveInstance(context.Context, int64, time.Time) error       { return nil }
func (s alertRepoStub) ListInstances(context.Context, alert.InstanceListFilter) ([]alert.Instance, error) {
	return s.instances, s.instancesErr
}
func (s alertRepoStub) GetInstance(context.Context, int64) (alert.Instance, error) {
	return s.instance, s.instanceErr
}
func (s alertRepoStub) ClaimDueRules(context.Context, time.Time, int, time.Duration) ([]alert.Rule, error) {
	return nil, nil
}
func (s alertRepoStub) ReleaseClaim(context.Context, int64, time.Time, string, time.Time, string) error {
	return nil
}
func (s alertRepoStub) UpdateRuleHealth(context.Context, int64, string, time.Time, string) error {
	return nil
}

type alertDiagnosisRepoStub struct{}

func (s alertDiagnosisRepoStub) Save(context.Context, *diagnosis.Record) error { return nil }

type alertMetricEvaluatorStub struct{}

func (s alertMetricEvaluatorStub) Evaluate(context.Context, metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error) {
	return metricshistory.EvaluationResponse{}, nil
}

func newAlertRouter(repo alertRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := alert.NewService(repo, alertDiagnosisRepoStub{}, alertMetricEvaluatorStub{}, time.Minute)
	h := &alertHandler{service: service, users: &auth.Service{}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "alert-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/clusters/:cluster_id/alert-rules", h.createRule)
	router.GET("/api/v1/clusters/:cluster_id/alert-rules", h.listRules)
	router.GET("/api/v1/clusters/:cluster_id/alert-rules/:rule_id", h.getRule)
	router.PATCH("/api/v1/clusters/:cluster_id/alert-rules/:rule_id", h.patchRule)
	router.DELETE("/api/v1/clusters/:cluster_id/alert-rules/:rule_id", h.deleteRule)
	router.GET("/api/v1/clusters/:cluster_id/alert-instances", h.listInstances)
	router.GET("/api/v1/clusters/:cluster_id/alert-instances/:alert_id", h.getInstance)
	return router
}

func alertRule() alert.Rule {
	return alert.Rule{ID: 5, ClusterID: 1, DisplayName: "CPU high", ResourceKind: "Node", ResourceName: "demo-node", MetricName: "cpu_usage", Operator: ">", Threshold: 80, ForSeconds: 60, MinimumPoints: 1, Enabled: true}
}

func validRuleBody() string {
	return `{"display_name":"CPU high","resource_kind":"Node","resource_name":"demo-node","metric_name":"cpu","operator":"gte","threshold":80,"for_seconds":60,"minimum_points":2}`
}

func TestAlertCreateRule(t *testing.T) {
	router := newAlertRouter(alertRepoStub{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/alert-rules", strings.NewReader(validRuleBody())))
	if rec.Code != http.StatusCreated || !contains(rec.Body.String(), `"display_name":"CPU high"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid cluster id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/0/alert-rules", strings.NewReader(validRuleBody())))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_CLUSTER_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing required field -> binding failure
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/alert-rules", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid rule", err: alert.ErrInvalidRule, status: http.StatusBadRequest, code: "INVALID_RULE"},
		{name: "cluster limit", err: alert.ErrClusterLimit, status: http.StatusConflict, code: "CLUSTER_LIMIT"},
		{name: "duplicate", err: alert.ErrDuplicateName, status: http.StatusConflict, code: "DUPLICATE_NAME"},
		{name: "generic", err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := newAlertRouter(alertRepoStub{createErr: tt.err})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/alert-rules", strings.NewReader(validRuleBody())))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("%s: status=%d body=%s want status=%d code=%s", tt.name, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}

func TestAlertListAndGetRule(t *testing.T) {
	router := newAlertRouter(alertRepoStub{rules: []alert.Rule{alertRule()}, rule: alertRule()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "CPU high") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// nil rules coerced to []
	router = newAlertRouter(alertRepoStub{})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// list failure
	router = newAlertRouter(alertRepoStub{rulesErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// get success
	router = newAlertRouter(alertRepoStub{rule: alertRule()})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules/5", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "CPU high") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid rule id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules/abc", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_RULE_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// rule missing -> 404
	router = newAlertRouter(alertRepoStub{ruleErr: alert.ErrRuleNotFound})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules/5", nil))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// generic failure -> 500
	router = newAlertRouter(alertRepoStub{ruleErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-rules/5", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAlertPatchAndDeleteRule(t *testing.T) {
	router := newAlertRouter(alertRepoStub{rule: alertRule()})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/clusters/1/alert-rules/5", strings.NewReader(`{"display_name":"CPU Very High"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/clusters/1/alert-rules/abc", strings.NewReader(`{"display_name":"x"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_RULE_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid body
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/clusters/1/alert-rules/5", strings.NewReader(`{invalid`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: alert.ErrRuleNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "deleted", err: alert.ErrRuleDeleted, status: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "invalid rule", err: alert.ErrInvalidRule, status: http.StatusBadRequest, code: "INVALID_RULE"},
		{name: "duplicate", err: alert.ErrDuplicateName, status: http.StatusConflict, code: "DUPLICATE_NAME"},
		{name: "generic", err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := newAlertRouter(alertRepoStub{rule: alertRule(), patchErr: tt.err})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/clusters/1/alert-rules/5", strings.NewReader(`{"display_name":"x"}`)))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("%s: status=%d body=%s want status=%d code=%s", tt.name, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}

	// delete success -> 204
	router = newAlertRouter(alertRepoStub{rule: alertRule()})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/1/alert-rules/5", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// delete unresolved alert -> 409
	router = newAlertRouter(alertRepoStub{rule: alertRule(), deleteErr: alert.ErrRuleUnresolvedAlert})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/clusters/1/alert-rules/5", nil))
	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "UNRESOLVED_ALERT") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAlertInstances(t *testing.T) {
	instance := alert.Instance{ID: 9, RuleID: 5, State: "firing"}
	router := newAlertRouter(alertRepoStub{instances: []alert.Instance{instance}, instance: instance, rule: alertRule()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances?state=firing&limit=10", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"state":"firing"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// nil instances coerced to []
	router = newAlertRouter(alertRepoStub{})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `[]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// list failure
	router = newAlertRouter(alertRepoStub{instancesErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// get instance success
	router = newAlertRouter(alertRepoStub{instance: instance, rule: alertRule()})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances/9", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"state":"firing"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid alert id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances/abc", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_ALERT_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing instance -> 404
	router = newAlertRouter(alertRepoStub{instanceErr: alert.ErrAlertNotFound})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/alert-instances/9", nil))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
