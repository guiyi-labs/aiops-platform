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

	"k8s-aiops.local/backend/internal/aiexplain"
	"k8s-aiops.local/backend/internal/diagnosis"
)

type aiexplainDiagnosisStub struct {
	record diagnosis.Record
	err    error
}

func (s aiexplainDiagnosisStub) Get(context.Context, int64) (diagnosis.Record, error) {
	return s.record, s.err
}

type aiexplainProviderStub struct {
	result aiexplain.ProviderResult
	err    error
}

func (s aiexplainProviderStub) Generate(context.Context, aiexplain.Prompt) (aiexplain.ProviderResult, error) {
	return s.result, s.err
}

type aiexplainRepoStub struct {
	items       []aiexplain.Explanation
	feedback    aiexplain.FeedbackResult
	usage       aiexplain.Usage
	quality     aiexplain.QualitySummary
	reserveErr  error
	saveErr     error
	listErr     error
	feedbackErr error
	usageErr    error
	qualityErr  error
}

func (s aiexplainRepoStub) Save(_ context.Context, item *aiexplain.Explanation) error {
	return s.saveErr
}
func (s aiexplainRepoStub) List(context.Context, int64, int64) ([]aiexplain.Explanation, error) {
	return s.items, s.listErr
}
func (s aiexplainRepoStub) AddFeedback(context.Context, int64, aiexplain.ActorRef, string, string) (aiexplain.FeedbackResult, error) {
	return s.feedback, s.feedbackErr
}
func (s aiexplainRepoStub) Quality(context.Context) (aiexplain.QualitySummary, error) {
	return s.quality, s.qualityErr
}
func (s aiexplainRepoStub) Usage(context.Context) (aiexplain.Usage, error) {
	return s.usage, s.usageErr
}
func (s aiexplainRepoStub) Reserve(context.Context, aiexplain.Reservation, int) error {
	return s.reserveErr
}
func (s aiexplainRepoStub) Release(context.Context, string) error { return nil }

func newAIExplanationRouter(service *aiexplain.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := aiExplanationHandler{service: service}
	router := gin.New()
	router.GET("/api/v1/ai/status", h.status)
	router.GET("/api/v1/ai/quality", h.quality)
	router.POST("/api/v1/ai/explanations/:explanation_id/feedback", h.feedback)
	router.GET("/api/v1/diagnoses/:diagnosis_id/explanations", h.list)
	router.POST("/api/v1/diagnoses/:diagnosis_id/explanations", h.generate)
	return router
}

func aiExplanationService(diagnoses aiexplainDiagnosisStub, provider aiexplainProviderStub, repo aiexplainRepoStub) *aiexplain.Service {
	return aiexplain.NewService(aiexplain.ServiceConfig{Enabled: true, MaxConcurrentRequests: 8, DailyTokenBudget: 1_000_000, MaxOutputTokens: 512, ReservationTTL: time.Minute}, diagnoses, provider, repo)
}

func diagnosisRecordWithEvidence() diagnosis.Record {
	return diagnosis.Record{ID: 7, Summary: "oom", Evidence: []diagnosis.Evidence{{Type: "signal", Source: "kubelet", Content: map[string]any{"message": "OOMKilled"}}}}
}

func TestAIExplanationGenerateSuccess(t *testing.T) {
	service := aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{result: aiexplain.ProviderResult{Provider: "mock", Model: "m1", Summary: "explained"}}, aiexplainRepoStub{})
	router := newAIExplanationRouter(service)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses/7/explanations", nil))
	if rec.Code != http.StatusCreated || !contains(rec.Body.String(), `"summary":"explained"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAIExplanationGenerateErrorBranches(t *testing.T) {
	cases := []struct {
		name    string
		service *aiexplain.Service
		status  int
		code    string
	}{
		{
			name:    "diagnosis missing",
			service: aiExplanationService(aiexplainDiagnosisStub{err: diagnosis.ErrRecordNotFound}, aiexplainProviderStub{}, aiexplainRepoStub{}),
			status:  http.StatusNotFound,
			code:    "DIAGNOSIS_NOT_FOUND",
		},
		{
			name: "no evidence",
			service: aiExplanationService(
				aiexplainDiagnosisStub{record: diagnosis.Record{ID: 7, Summary: "no evidence"}},
				aiexplainProviderStub{}, aiexplainRepoStub{},
			),
			status: http.StatusUnprocessableEntity,
			code:   "AI_EVIDENCE_REQUIRED",
		},
		{
			name:    "budget exceeded",
			service: aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{}, aiexplainRepoStub{reserveErr: aiexplain.ErrBudgetExceeded}),
			status:  http.StatusTooManyRequests,
			code:    "AI_BUDGET_EXCEEDED",
		},
		{
			name:    "provider failure",
			service: aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{err: aiexplain.ErrProviderFailure}, aiexplainRepoStub{}),
			status:  http.StatusBadGateway,
			code:    "AI_PROVIDER_ERROR",
		},
		{
			name:    "invalid output",
			service: aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{err: aiexplain.ErrInvalidOutput}, aiexplainRepoStub{}),
			status:  http.StatusBadGateway,
			code:    "AI_INVALID_OUTPUT",
		},
		{
			name:    "generic failure",
			service: aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{}, aiexplainRepoStub{reserveErr: errors.New("db down")}),
			status:  http.StatusInternalServerError,
			code:    "INTERNAL_ERROR",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			router := newAIExplanationRouter(tt.service)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses/7/explanations", nil))
			if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
				t.Fatalf("status=%d body=%s want status=%d code=%s", rec.Code, rec.Body.String(), tt.status, tt.code)
			}
		})
	}
}

func TestAIExplanationDisabled(t *testing.T) {
	service := aiexplain.NewService(aiexplain.ServiceConfig{Enabled: false}, aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{})
	router := newAIExplanationRouter(service)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses/7/explanations", nil))
	if rec.Code != http.StatusServiceUnavailable || !contains(rec.Body.String(), "AI_DISABLED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAIExplanationStatusAndQuality(t *testing.T) {
	router := newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{usage: aiexplain.Usage{UsedTokensToday: 10, ExplanationCount: 2}}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ai/status", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"enabled":true`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{quality: aiexplain.QualitySummary{TotalFeedback: 3}}))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ai/quality", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"total_feedback":3`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Status/quality failures surface as 500.
	router = newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{usageErr: errors.New("db down")}))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ai/status", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAIExplanationFeedback(t *testing.T) {
	success := aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{feedback: aiexplain.FeedbackResult{}})
	router := newAIExplanationRouter(success)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/explanations/5/feedback", strings.NewReader(`{"verdict":"helpful","comment":"ok"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid explanation id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/explanations/abc/feedback", strings.NewReader(`{"verdict":"helpful"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_EXPLANATION_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing verdict
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/explanations/5/feedback", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// comment exceeds 1000 chars
	longComment := strings.Repeat("a", 1001)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/explanations/5/feedback", strings.NewReader(`{"verdict":"helpful","comment":"`+longComment+`"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// sentinel error branches
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: aiexplain.ErrInvalidFeedback, status: http.StatusBadRequest, code: "INVALID_AI_FEEDBACK"},
		{err: aiexplain.ErrExplanationNotFound, status: http.StatusNotFound, code: "AI_EXPLANATION_NOT_FOUND"},
		{err: aiexplain.ErrFeedbackExists, status: http.StatusConflict, code: "AI_FEEDBACK_EXISTS"},
		{err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		service := aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{feedbackErr: tt.err})
		router := newAIExplanationRouter(service)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ai/explanations/5/feedback", strings.NewReader(`{"verdict":"helpful"}`)))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s want status=%d code=%s", tt.err, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}

func TestAIExplanationList(t *testing.T) {
	router := newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{}, aiexplainRepoStub{items: []aiexplain.Explanation{{ID: 5, Summary: "explained"}}}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/7/explanations", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"summary":"explained"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// diagnosis missing
	router = newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{err: diagnosis.ErrRecordNotFound}, aiexplainProviderStub{}, aiexplainRepoStub{}))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/7/explanations", nil))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "DIAGNOSIS_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// generic list failure
	router = newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{record: diagnosisRecordWithEvidence()}, aiexplainProviderStub{}, aiexplainRepoStub{listErr: errors.New("db down")}))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/7/explanations", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid diagnosis id
	router = newAIExplanationRouter(aiExplanationService(aiexplainDiagnosisStub{}, aiexplainProviderStub{}, aiexplainRepoStub{}))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/0/explanations", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_DIAGNOSIS_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
