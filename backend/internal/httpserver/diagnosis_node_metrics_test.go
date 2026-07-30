package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/metricshistory"
)

type diagnosisRepositoryStub struct {
	saved      []*diagnosis.Record
	saveErr    error
	listResult []diagnosis.Record
	getRecord  diagnosis.Record
	getErr     error
	summary    diagnosis.Summary
	summaryErr error
}

func (s *diagnosisRepositoryStub) Save(_ context.Context, record *diagnosis.Record) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	record.ID = int64(len(s.saved) + 1)
	record.CreatedAt = time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	record.UpdatedAt = record.CreatedAt
	s.saved = append(s.saved, record)
	return nil
}

func (s *diagnosisRepositoryStub) List(_ context.Context, f diagnosis.ListFilter) ([]diagnosis.Record, error) {
	return s.listResult, nil
}

func (s *diagnosisRepositoryStub) Get(_ context.Context, id int64) (diagnosis.Record, error) {
	if s.getErr != nil {
		return diagnosis.Record{}, s.getErr
	}
	return s.getRecord, nil
}

func (s *diagnosisRepositoryStub) Transition(_ context.Context, id int64, to string, actor diagnosis.ActorRef, comment string) (diagnosis.Record, error) {
	return s.getRecord, nil
}

func (s *diagnosisRepositoryStub) AddFeedback(_ context.Context, id int64, verdict string, actor diagnosis.ActorRef, comment string) (diagnosis.Record, error) {
	return s.getRecord, nil
}

func (s *diagnosisRepositoryStub) Assign(_ context.Context, id int64, assignee, actor diagnosis.ActorRef, comment string) (diagnosis.Record, error) {
	return s.getRecord, nil
}

func (s *diagnosisRepositoryStub) Summary(_ context.Context) (diagnosis.Summary, error) {
	return s.summary, s.summaryErr
}

type fakeMetricEvaluator struct {
	response metricshistory.EvaluationResponse
	err      error
	query    metricshistory.EvaluationQuery
}

func (f *fakeMetricEvaluator) Evaluate(_ context.Context, query metricshistory.EvaluationQuery) (metricshistory.EvaluationResponse, error) {
	f.query = query
	return f.response, f.err
}

func diagnoseNodeMetricsTestRouter(evaluator diagnosis.MetricEvaluator, repo diagnosis.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := diagnosis.NewService(nil, repo).WithMetricEvaluator(evaluator)
	router := gin.New()
	router.POST("/:cluster_id/diagnoses/node_metrics", withClusterContext(), diagnosisHandler{service: svc}.diagnoseNodeMetrics)
	return router
}

func buildFiringEvaluationResponse(start time.Time, metric string) metricshistory.EvaluationResponse {
	unit := metricshistory.UnitNanocores
	if metric == metricshistory.MetricMemory {
		unit = metricshistory.UnitBytes
	}
	threshold := int64(80_000_000)
	if metric == metricshistory.MetricMemory {
		threshold = 8_000_000_000
	}
	window := metricshistory.SustainedWindow{
		StartCollectedAt: start,
		EndCollectedAt:   start.Add(5 * time.Minute),
		BreachingPoints:  5,
		SpanSeconds:      300,
	}
	return metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metric, Unit: unit,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 6, Succeeded: 6, Points: 6},
		State:     metricshistory.EvaluationStateFiring,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: threshold, ForSeconds: 120, MinimumPoints: 2,
		PointsEvaluated: 6, BreachingPoints: 5, ObservedSpanSeconds: 300,
		SustainedWindows:   []metricshistory.SustainedWindow{window},
		LatestFiringWindow: &window,
	}
}

func TestDiagnoseNodeMetricsHandlerParsesRequestAndCreatesRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	evaluator := &fakeMetricEvaluator{response: buildFiringEvaluationResponse(start, metricshistory.MetricCPU)}
	repo := &diagnosisRepositoryStub{}
	body := bytes.NewBufferString(`{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":80000000,"for_seconds":120,"minimum_points":2}`)
	recorder := httptest.NewRecorder()
	diagnoseNodeMetricsTestRouter(evaluator, repo).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/7/diagnoses/node_metrics", body))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var record diagnosis.Record
	if err := json.Unmarshal(recorder.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.RuleID != diagnosis.RuleNodeSustainedMetricBreach {
		t.Fatalf("expected rule %s, got %s", diagnosis.RuleNodeSustainedMetricBreach, record.RuleID)
	}
	if record.Resource.Name != "worker-a" {
		t.Fatalf("expected resource worker-a, got %s", record.Resource.Name)
	}
	if record.Severity != "high" {
		t.Fatalf("expected severity high for CPU, got %s", record.Severity)
	}
	if len(record.Evidence) < 2 {
		t.Fatalf("expected at least 2 evidence entries, got %d", len(record.Evidence))
	}
	found := false
	for _, e := range record.Evidence {
		wi, _ := e.Content["window_index"].(float64)
		if e.Type == "metric_sustained_breach" && int(wi) == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metric_sustained_breach evidence, got %#v", record.Evidence)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved record, got %d", len(repo.saved))
	}
	if evaluator.query.SeriesQuery.ClusterID != 7 {
		t.Fatalf("expected evaluator query cluster 7, got %d", evaluator.query.SeriesQuery.ClusterID)
	}
}

func TestDiagnoseNodeMetricsHandlerDefaultMinimumPoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	evaluator := &fakeMetricEvaluator{response: buildFiringEvaluationResponse(start, metricshistory.MetricMemory)}
	repo := &diagnosisRepositoryStub{}
	body := bytes.NewBufferString(`{"name":"worker-a","metric":"node_memory","operator":"gte","threshold":8000000000,"for_seconds":120}`)
	recorder := httptest.NewRecorder()
	diagnoseNodeMetricsTestRouter(evaluator, repo).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/7/diagnoses/node_metrics", body))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var record diagnosis.Record
	if err := json.Unmarshal(recorder.Body.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record.Severity != "medium" {
		t.Fatalf("expected severity medium for memory, got %s", record.Severity)
	}
}

func TestDiagnoseNodeMetricsHandlerRejectsInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &diagnosisRepositoryStub{}
	evaluator := &fakeMetricEvaluator{}
	tests := []struct {
		name string
		body string
	}{
		{"missing fields", `{}`},
		{"empty name", `{"name":"","metric":"node_cpu","operator":"gte","threshold":1,"for_seconds":60}`},
		{"empty metric", `{"name":"worker-a","metric":"","operator":"gte","threshold":1,"for_seconds":60}`},
		{"unknown metric", `{"name":"worker-a","metric":"disk","operator":"gte","threshold":1,"for_seconds":60}`},
		{"bad operator", `{"name":"worker-a","metric":"node_cpu","operator":"gt","threshold":1,"for_seconds":60}`},
		{"for_seconds too low", `{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":1,"for_seconds":59}`},
		{"for_seconds too high", `{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":1,"for_seconds":86401}`},
		{"minimum_points too high", `{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":1,"for_seconds":60,"minimum_points":1441}`},
		{"not JSON", `not json`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			diagnoseNodeMetricsTestRouter(evaluator, repo).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/7/diagnoses/node_metrics", bytes.NewBufferString(tc.body)))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("%s: status=%d body=%s", tc.name, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestDiagnoseNodeMetricsHandlerNoRuleMatchForNormalState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	start := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	evaluator := &fakeMetricEvaluator{response: metricshistory.EvaluationResponse{
		Series: metricshistory.Series{
			ClusterID: 7, ResourceKind: metricshistory.ResourceNode,
			ResourceName: "worker-a", MetricName: metricshistory.MetricCPU,
			Unit: metricshistory.UnitNanocores,
		},
		From: start, To: start.Add(1 * time.Hour),
		Coverage:  metricshistory.QueryCoverage{Collections: 6, Succeeded: 6, Points: 6},
		State:     metricshistory.EvaluationStateNormal,
		Operator:  metricshistory.OperatorGreaterThanOrEqual,
		Threshold: 80_000_000, ForSeconds: 120, MinimumPoints: 2,
		PointsEvaluated: 6,
	}}
	repo := &diagnosisRepositoryStub{}
	body := bytes.NewBufferString(`{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":80000000,"for_seconds":120}`)
	recorder := httptest.NewRecorder()
	diagnoseNodeMetricsTestRouter(evaluator, repo).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/7/diagnoses/node_metrics", body))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for normal state, status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "NO_RULE_MATCH") {
		t.Fatalf("expected NO_RULE_MATCH error, body=%s", recorder.Body.String())
	}
	if len(repo.saved) != 0 {
		t.Fatalf("expected no saved record for normal state, got %d", len(repo.saved))
	}
}

func TestDiagnoseNodeMetricsHandlerMapsEvaluationErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		evaluatorErr   error
		expectedStatus int
		expectedCode   string
	}{
		{"invalid query", metricshistory.ErrInvalidQuery, http.StatusBadRequest, "INVALID_REQUEST"},
		{"invalid evaluation", metricshistory.ErrInvalidEvaluation, http.StatusBadRequest, "INVALID_REQUEST"},
		{"cluster not found", metricshistory.ErrClusterNotFound, http.StatusNotFound, "CLUSTER_NOT_FOUND"},
		{"unknown error", errors.New("boom"), http.StatusBadGateway, "DIAGNOSIS_FAILED"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := &fakeMetricEvaluator{err: tc.evaluatorErr}
			repo := &diagnosisRepositoryStub{}
			body := bytes.NewBufferString(`{"name":"worker-a","metric":"node_cpu","operator":"gte","threshold":80000000,"for_seconds":120}`)
			recorder := httptest.NewRecorder()
			diagnoseNodeMetricsTestRouter(evaluator, repo).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/7/diagnoses/node_metrics", body))
			if recorder.Code != tc.expectedStatus {
				t.Fatalf("%s: expected status %d, got %d body=%s", tc.name, tc.expectedStatus, recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tc.expectedCode) {
				t.Fatalf("%s: expected code %s, body=%s", tc.name, tc.expectedCode, recorder.Body.String())
			}
		})
	}
}
