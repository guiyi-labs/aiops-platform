package httpserver

import (
	"context"
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

type metricsHistoryRepositoryStub struct {
	query  metricshistory.SeriesQuery
	result metricshistory.RepositorySeriesResult
	err    error
}

func (r *metricsHistoryRepositoryStub) SaveCollection(context.Context, metricshistory.Collection) (int64, error) {
	return 0, r.err
}

func (r *metricsHistoryRepositoryStub) QuerySeries(_ context.Context, query metricshistory.SeriesQuery) (metricshistory.RepositorySeriesResult, error) {
	r.query = query
	return r.result, r.err
}

func (r *metricsHistoryRepositoryStub) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, r.err
}

func metricsHistoryTestRouter(repository metricshistory.Repository) *gin.Engine {
	service, _ := metricshistory.NewService(metricshistory.Config{}, repository)
	router := gin.New()
	router.GET("/:cluster_id/metrics/history", withClusterContext(), metricsHistoryHandler{service: service}.series)
	return router
}

func TestMetricsHistoryHandlerParsesExactSeriesQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	from := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	repository := &metricsHistoryRepositoryStub{result: metricshistory.RepositorySeriesResult{
		Points: []metricshistory.Point{{Value: 1048576, SourceTimestamp: from.Add(time.Minute), CollectedAt: from.Add(2 * time.Minute), WindowMilliseconds: 15000}},
		Total:  1, Coverage: metricshistory.QueryCoverage{Collections: 2, Points: 1, Missing: 1},
	}}
	path := "/7/metrics/history?resource_kind=Pod&namespace=prod&name=api-0&container=api&metric=memory&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z&limit=100"
	recorder := httptest.NewRecorder()
	metricsHistoryTestRouter(repository).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if repository.query.ClusterID != 7 || repository.query.ResourceKind != metricshistory.ResourcePod || repository.query.ResourceNamespace != "prod" ||
		repository.query.ResourceName != "api-0" || repository.query.ContainerName != "api" || repository.query.MetricName != metricshistory.MetricMemory ||
		repository.query.Limit != 100 || !repository.query.From.Equal(from) || !repository.query.To.Equal(from.Add(time.Hour)) {
		t.Fatalf("query = %#v", repository.query)
	}
	var response metricshistory.SeriesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Series.Unit != metricshistory.UnitBytes || len(response.Points) != 1 || response.Coverage.Missing != 1 || response.Truncated {
		t.Fatalf("response = %#v", response)
	}
}

func TestMetricsHistoryHandlerRejectsInvalidQueryShapes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := "/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z"
	tests := []string{
		"/0/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z",
		"/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&to=2026-07-29T01:00:00Z",
		"/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=not-a-time&to=2026-07-29T01:00:00Z",
		"/7/metrics/history?resource_kind=Node&namespace=default&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z",
		"/7/metrics/history?resource_kind=Pod&namespace=default&name=api&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z",
		"/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-27T00:00:00Z&to=2026-07-29T01:00:00Z",
		base + "&limit=1441",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			metricsHistoryTestRouter(&metricsHistoryRepositoryStub{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestMetricsHistoryHandlerMapsRepositoryErrorsWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := "/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z"
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{err: metricshistory.ErrClusterNotFound, status: http.StatusNotFound, code: "CLUSTER_NOT_FOUND"},
		{err: errors.New("postgres connection secret"), status: http.StatusInternalServerError, code: "METRICS_HISTORY_QUERY_FAILED"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		metricsHistoryTestRouter(&metricsHistoryRepositoryStub{err: test.err}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
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

func TestMetricsHistoryRouteRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := metricshistory.NewService(metricshistory.Config{}, &metricsHistoryRepositoryStub{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/7/metrics/history?resource_kind=Node&name=worker-a&metric=cpu&from=2026-07-29T00:00:00Z&to=2026-07-29T01:00:00Z", nil)
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
