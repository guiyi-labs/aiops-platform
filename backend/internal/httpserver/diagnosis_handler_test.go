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

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/diagnosis"
	"k8s-aiops.local/backend/internal/requestctx"
)

type diagTestRepo struct {
	record        diagnosis.Record
	list          []diagnosis.Record
	summary       diagnosis.Summary
	getErr        error
	transitionErr error
	feedbackErr   error
	assignErr     error
	listErr       error
	summaryErr    error
}

func (s *diagTestRepo) Save(context.Context, *diagnosis.Record) error { return nil }
func (s *diagTestRepo) List(context.Context, diagnosis.ListFilter) ([]diagnosis.Record, error) {
	return s.list, s.listErr
}
func (s *diagTestRepo) Get(context.Context, int64) (diagnosis.Record, error) {
	return s.record, s.getErr
}
func (s *diagTestRepo) Transition(context.Context, int64, string, diagnosis.ActorRef, string) (diagnosis.Record, error) {
	return s.record, s.transitionErr
}
func (s *diagTestRepo) AddFeedback(context.Context, int64, string, diagnosis.ActorRef, string) (diagnosis.Record, error) {
	return s.record, s.feedbackErr
}
func (s *diagTestRepo) Assign(context.Context, int64, diagnosis.ActorRef, diagnosis.ActorRef, string) (diagnosis.Record, error) {
	return s.record, s.assignErr
}
func (s *diagTestRepo) Summary(context.Context) (diagnosis.Summary, error) {
	return s.summary, s.summaryErr
}

func newDiagnosisRouter(repo *diagTestRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	_ = auth.SystemAdmin
	stub := &userRepositoryStub{user: managedUser(1, "admin")}
	h := &diagnosisHandler{service: diagnosis.NewService(nil, repo), users: auth.NewService(stub, auth.NewPasswordHasher(), auth.NewTokenManager("test-key-32-bytes-long-ok", 15*time.Minute), time.Hour)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{auth.SystemAdmin}, ClusterID: 1, RequestID: "diagnosis-test",
		}))
		c.Next()
	})
	router.GET("/api/v1/diagnoses", h.list)
	router.GET("/api/v1/diagnoses/summary", h.summary)
	router.GET("/api/v1/diagnoses/:diagnosis_id", h.get)
	router.PATCH("/api/v1/diagnoses/:diagnosis_id", h.transition)
	router.PATCH("/api/v1/diagnoses/:diagnosis_id/feedback", h.feedback)
	router.PATCH("/api/v1/diagnoses/:diagnosis_id/assignment", h.assign)
	return router
}

func TestDiagnosisListSummaryGetSuccess(t *testing.T) {
	ext := int64(1)
	sev := "high"
	rec := diagnosis.Record{ID: 5, ClusterID: 1, Severity: sev, RuleID: "pod.oom", Resource: diagnosis.ResourceRef{Kind: "Pod", Namespace: "default", Name: "web-0"}, Status: "confirmed", Evidence: []diagnosis.Evidence{}}
	repo := &diagTestRepo{record: rec, list: []diagnosis.Record{rec}, summary: diagnosis.Summary{Total: 1, Open: 1, Overdue: 0, Recent: []diagnosis.Record{rec}}}
	_ = ext
	router := newDiagnosisRouter(repo)

	// list
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses?cluster_id=1&limit=10", nil))
	if r.Code != http.StatusOK || !contains(r.Body.String(), "web-0") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// summary
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/summary", nil))
	if r.Code != http.StatusOK || !contains(r.Body.String(), `"open":1`) {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// get
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/5", nil))
	if r.Code != http.StatusOK || !contains(r.Body.String(), "web-0") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}

func TestDiagnosisReadErrorBranches(t *testing.T) {
	// list failure -> 500
	router := newDiagnosisRouter(&diagTestRepo{listErr: errors.New("db down")})
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses", nil))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// get not found -> 404
	router = newDiagnosisRouter(&diagTestRepo{getErr: diagnosis.ErrRecordNotFound})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/5", nil))
	if r.Code != http.StatusNotFound || !contains(r.Body.String(), "DIAGNOSIS_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// get generic -> 500
	router = newDiagnosisRouter(&diagTestRepo{getErr: errors.New("db down")})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/5", nil))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// summary failure -> 500
	router = newDiagnosisRouter(&diagTestRepo{summaryErr: errors.New("db down")})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/v1/diagnoses/summary", nil))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}

func TestDiagnosisMutationErrorBranches(t *testing.T) {
	// transition: not found
	router := newDiagnosisRouter(&diagTestRepo{transitionErr: diagnosis.ErrRecordNotFound})
	r := httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5", strings.NewReader(`{"status":"confirmed"}`)))
	if r.Code != http.StatusNotFound || !contains(r.Body.String(), "DIAGNOSIS_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// transition: invalid transition
	router = newDiagnosisRouter(&diagTestRepo{transitionErr: diagnosis.ErrInvalidTransition})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5", strings.NewReader(`{"status":"confirmed"}`)))
	if r.Code != http.StatusConflict || !contains(r.Body.String(), "INVALID_STATUS_TRANSITION") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// transition: generic
	router = newDiagnosisRouter(&diagTestRepo{transitionErr: errors.New("db down")})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5", strings.NewReader(`{"status":"confirmed"}`)))
	if r.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// feedback: invalid verdict
	router = newDiagnosisRouter(&diagTestRepo{})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5/feedback", strings.NewReader(`{"verdict":"bogus"}`)))
	if r.Code != http.StatusBadRequest || !contains(r.Body.String(), "INVALID_FEEDBACK") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// feedback: not found
	router = newDiagnosisRouter(&diagTestRepo{feedbackErr: diagnosis.ErrRecordNotFound})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5/feedback", strings.NewReader(`{"verdict":"accurate"}`)))
	if r.Code != http.StatusNotFound || !contains(r.Body.String(), "DIAGNOSIS_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// assign: assignee is assignable (stub admin), success path
	router = newDiagnosisRouter(&diagTestRepo{})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5/assignment", strings.NewReader(`{"assignee_user_id":1}`)))
	if r.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}

	// assign: already assigned
	router = newDiagnosisRouter(&diagTestRepo{assignErr: diagnosis.ErrAlreadyAssigned})
	r = httptest.NewRecorder()
	router.ServeHTTP(r, httptest.NewRequest(http.MethodPatch, "/api/v1/diagnoses/5/assignment", strings.NewReader(`{"assignee_user_id":1}`)))
	if r.Code != http.StatusConflict || !contains(r.Body.String(), "ALREADY_ASSIGNED") {
		t.Fatalf("status=%d body=%s", r.Code, r.Body.String())
	}
}
