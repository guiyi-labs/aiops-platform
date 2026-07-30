package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap/zaptest"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/metricshistory"
)

func metricsEvaluationTestRouter(repository metricshistory.Repository) *gin.Engine {
	service, _ := metricshistory.NewService(metricshistory.Config{}, repository)
	router := gin.New()
	router.GET("/:cluster_id/metrics/history/evaluate", withClusterContext(), metricsHistoryHandler{service: service}.evaluate)
	return router
}

func TestMetricsEvaluationHandlerParsesRuleAndReturnsEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	from := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	repository := &metricsHistoryRepositoryStub{result: metricshistory.RepositorySeriesResult{
		Points: []metricshistory.Point{
			{Value: 100, SourceTimestamp: from, CollectedAt: from, WindowMilliseconds: 15000},
			{Value: 200, SourceTimestamp: from.Add(time.Minute), CollectedAt: from.Add(time.Minute), WindowMilliseconds: 15000},
		},
		Total: 2, Coverage: metricshistory.QueryCoverage{Collections: 2, Succeeded: 2, Points: 2},
	}}
	path := "/7/metrics/history/evaluate?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z&operator=gte&threshold=100&for_seconds=60"
	recorder := httptest.NewRecorder()
	metricsEvaluationTestRouter(repository).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.query.ClusterID != 7 || repository.query.ResourceKind != metricshistory.ResourceNode || repository.query.ResourceName != "worker-a" ||
		repository.query.MetricName != metricshistory.MetricCPU || repository.query.Limit != 1440 || !repository.query.From.Equal(from) || !repository.query.To.Equal(from.Add(time.Hour)) {
		t.Fatalf("query = %#v", repository.query)
	}
	var response metricshistory.EvaluationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.State != metricshistory.EvaluationStateFiring || response.MinimumPoints != 2 || response.PointsEvaluated != 2 ||
		response.BreachingPoints != 2 || response.ObservedSpanSeconds != 60 || response.Series.Unit != metricshistory.UnitNanocores {
		t.Fatalf("response = %#v", response)
	}
}

func TestMetricsEvaluationHandlerParsesPodLTEAndMinimumPoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	from := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	repository := &metricsHistoryRepositoryStub{result: metricshistory.RepositorySeriesResult{
		Points: []metricshistory.Point{
			{Value: 3, CollectedAt: from}, {Value: 2, CollectedAt: from.Add(time.Minute)}, {Value: 1, CollectedAt: from.Add(2 * time.Minute)},
		},
		Total: 3, Coverage: metricshistory.QueryCoverage{Collections: 3, Succeeded: 3, Points: 3},
	}}
	path := "/7/metrics/history/evaluate?resource_kind=Pod&namespace=prod&name=api-0&container=api&metric=memory&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z&operator=lte&threshold=3&for_seconds=120&minimum_points=3"
	recorder := httptest.NewRecorder()
	metricsEvaluationTestRouter(repository).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response metricshistory.EvaluationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.State != metricshistory.EvaluationStateFiring || response.Operator != metricshistory.OperatorLessThanOrEqual || response.MinimumPoints != 3 {
		t.Fatalf("response = %#v", response)
	}
}

func TestMetricsEvaluationHandlerRejectsInvalidRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := "/7/metrics/history/evaluate?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z"
	tests := []string{
		base + "&threshold=1&for_seconds=60",
		base + "&operator=gt&threshold=1&for_seconds=60",
		base + "&operator=gte&for_seconds=60",
		base + "&operator=gte&threshold=-1&for_seconds=60",
		base + "&operator=gte&threshold=9223372036854775808&for_seconds=60",
		base + "&operator=gte&threshold=1",
		base + "&operator=gte&threshold=1&for_seconds=59",
		base + "&operator=gte&threshold=1&for_seconds=86401",
		base + "&operator=gte&threshold=1&for_seconds=60&minimum_points=1",
		base + "&operator=gte&threshold=1&for_seconds=60&minimum_points=1441",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			metricsEvaluationTestRouter(&metricsHistoryRepositoryStub{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var response errorResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Code != "INVALID_QUERY" {
				t.Fatalf("response=%#v err=%v", response, err)
			}
		})
	}
}

func TestMetricsEvaluationHandlerMapsErrorsWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := "/7/metrics/history/evaluate?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z&operator=gte&threshold=1&for_seconds=60"
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: metricshistory.ErrClusterNotFound, status: http.StatusNotFound, code: "CLUSTER_NOT_FOUND"},
		{err: errors.New("postgres password secret"), status: http.StatusInternalServerError, code: "METRICS_EVALUATION_FAILED"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		metricsEvaluationTestRouter(&metricsHistoryRepositoryStub{err: test.err}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != test.status {
			t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		var response errorResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != test.code || response.Message == test.err.Error() {
			t.Fatalf("response = %#v", response)
		}
	}
}

func TestMetricsEvaluationRouteRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := metricshistory.NewService(metricshistory.Config{}, &metricsHistoryRepositoryStub{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/metrics/history/evaluate?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z&operator=gte&threshold=1&for_seconds=60", nil)
	New(zaptest.NewLogger(t), Options{Probe: probeStub{}, Auth: &auth.Service{}, MetricsHistory: service}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "ACCESS_TOKEN_REQUIRED" {
		t.Fatalf("response = %#v", response)
	}
}
