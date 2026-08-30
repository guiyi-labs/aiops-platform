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
	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
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

func (s *diagTestRepo) ListByClusters(_ context.Context, _ []int64, _, _ string, _ int) ([]diagnosis.FederationDiagnosisRow, error) {
	return []diagnosis.FederationDiagnosisRow{}, nil
}

func (s *diagTestRepo) StatsByClusters(_ context.Context, _ []int64) (diagnosis.FederationDiagnosisStats, error) {
	return diagnosis.FederationDiagnosisStats{ByStatus: map[string]int64{}, BySeverity: map[string]int64{}, ByCluster: []diagnosis.FederationClusterCount{}}, nil
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

// diagSourceStub implements diagnosis.Source with per-kind canned responses.
type diagSourceStub struct {
	err error
}

func (s diagSourceStub) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return k8sgateway.Pod{}, s.err
}
func (s diagSourceStub) PodEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return nil, s.err
}
func (s diagSourceStub) GetService(context.Context, int64, string, string) (k8sgateway.ServiceResource, error) {
	return k8sgateway.ServiceResource{}, s.err
}
func (s diagSourceStub) ServiceEndpoints(context.Context, int64, string, string) (k8sgateway.Endpoints, error) {
	return k8sgateway.Endpoints{}, s.err
}
func (s diagSourceStub) Node(context.Context, int64, string) (k8sgateway.Node, error) {
	return k8sgateway.Node{}, s.err
}
func (s diagSourceStub) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return k8sgateway.Deployment{}, s.err
}
func (s diagSourceStub) Ingress(context.Context, int64, string, string) (k8sgateway.Ingress, error) {
	return k8sgateway.Ingress{}, s.err
}
func (s diagSourceStub) PersistentVolumeClaim(context.Context, int64, string, string) (k8sgateway.PersistentVolumeClaim, error) {
	return k8sgateway.PersistentVolumeClaim{}, s.err
}
func (s diagSourceStub) HorizontalPodAutoscaler(context.Context, int64, string, string) (k8sgateway.HorizontalPodAutoscaler, error) {
	return k8sgateway.HorizontalPodAutoscaler{}, s.err
}
func (s diagSourceStub) ResourceEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return nil, s.err
}

func performDiagnosisCreate(t *testing.T, svc *diagnosis.Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &diagnosisHandler{service: svc}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{auth.SystemAdmin}, ClusterID: 1, RequestID: "diagnosis-create-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/diagnoses", h.create)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/diagnoses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestDiagnosisHandler_CreateValidationBranches(t *testing.T) {
	svc := diagnosis.NewService(diagSourceStub{}, &diagTestRepo{})
	// missing required fields
	missing := performDiagnosisCreate(t, svc, `{}`)
	if missing.Code != http.StatusBadRequest {
		t.Fatalf("empty body = %d, want 400", missing.Code)
	}
	// namespaced without namespace
	noNS := performDiagnosisCreate(t, svc, `{"resource_kind":"Pod","name":"web"}`)
	if noNS.Code != http.StatusBadRequest {
		t.Fatalf("pod without ns = %d, want 400", noNS.Code)
	}
	// unsupported kind
	badKind := performDiagnosisCreate(t, svc, `{"resource_kind":"DaemonSet","namespace":"ns","name":"x"}`)
	if badKind.Code != http.StatusBadRequest {
		t.Fatalf("unsupported kind = %d, want 400", badKind.Code)
	}
	// node needs no namespace → grandfathered
	nodeReq := performDiagnosisCreate(t, svc, `{"resource_kind":"Node","name":"node-1"}`)
	if nodeReq.Code == http.StatusBadRequest {
		t.Fatalf("node without ns should not 400, got %d", nodeReq.Code)
	}
}

func TestDiagnosisHandler_CreateErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not found", k8sgateway.ErrResourceNotFound, http.StatusNotFound},
		{"no rule match", diagnosis.ErrNoRuleMatch, http.StatusUnprocessableEntity},
		{"cluster disabled", cluster.ErrDisabled, http.StatusConflict},
		{"cluster not found", cluster.ErrNotFound, http.StatusNotFound},
		{"generic", errors.New("boom"), http.StatusBadGateway},
	}
	for _, tt := range cases {
		svc := diagnosis.NewService(diagSourceStub{err: tt.err}, &diagTestRepo{})
		rec := performDiagnosisCreate(t, svc, `{"resource_kind":"Pod","namespace":"ns","name":"web"}`)
		if rec.Code != tt.code {
			t.Errorf("%s: code = %d, want %d", tt.name, rec.Code, tt.code)
		}
	}
}

// diagSourceWithPod is a source that returns a canned pod + events on Pod()/PodEvents().
type diagSourceWithPod struct {
	diagSourceStub
	pod    k8sgateway.Pod
	events []k8sgateway.Event
}

func (s diagSourceWithPod) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return s.pod, nil
}
func (s diagSourceWithPod) PodEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return s.events, nil
}

func TestDiagnosisHandler_CreatePodSuccess(t *testing.T) {
	pod := k8sgateway.Pod{}
	pod.Metadata.Name = "web"
	pod.Metadata.Namespace = "ns"
	pod.Metadata.UID = "pod-uid-1"
	pod.Status.Phase = "Running"
	oomReason := "OOMKilled"
	pod.Status.ContainerStatuses = []k8sgateway.ContainerStatus{{
		Name:         "app",
		RestartCount: 2,
		State: k8sgateway.ContainerState{
			Terminated: &k8sgateway.ContainerStateDetail{Reason: oomReason, ExitCode: 137, Message: "out of memory"},
		},
	}}
	svc := diagnosis.NewService(diagSourceWithPod{pod: pod}, &diagTestRepo{})
	rec := performDiagnosisCreate(t, svc, `{"resource_kind":"Pod","namespace":"ns","name":"web"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pod create = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Pod") {
		t.Fatalf("response missing Pod kind: %s", rec.Body.String())
	}
}

func TestDiagnosisHandler_CreateDeploymentSuccess(t *testing.T) {
	dep := k8sgateway.Deployment{}
	dep.Metadata.Name = "api"
	dep.Metadata.Namespace = "ns"
	dep.Metadata.UID = "dep-uid-1"
	replicas := int32(3)
	dep.Spec.Replicas = &replicas
	dep.Status.UnavailableReplicas = 3
	// For Deployment, source.Deployment is called — inject via special stub
	depSource := &diagSourceDeploymentStub{dep: dep}
	rec := performDiagnosisCreate(t, diagnosis.NewService(depSource, &diagTestRepo{}), `{"resource_kind":"Deployment","namespace":"ns","name":"api"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dep create = %d; body=%s", rec.Code, rec.Body.String())
	}
}

type diagSourceDeploymentStub struct {
	dep k8sgateway.Deployment
}

func (s *diagSourceDeploymentStub) Pod(context.Context, int64, string, string) (k8sgateway.Pod, error) {
	return k8sgateway.Pod{}, nil
}
func (s *diagSourceDeploymentStub) PodEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return nil, nil
}
func (s *diagSourceDeploymentStub) GetService(context.Context, int64, string, string) (k8sgateway.ServiceResource, error) {
	return k8sgateway.ServiceResource{}, nil
}
func (s *diagSourceDeploymentStub) ServiceEndpoints(context.Context, int64, string, string) (k8sgateway.Endpoints, error) {
	return k8sgateway.Endpoints{}, nil
}
func (s *diagSourceDeploymentStub) Node(context.Context, int64, string) (k8sgateway.Node, error) {
	return k8sgateway.Node{}, nil
}
func (s *diagSourceDeploymentStub) Deployment(context.Context, int64, string, string) (k8sgateway.Deployment, error) {
	return s.dep, nil
}
func (s *diagSourceDeploymentStub) Ingress(context.Context, int64, string, string) (k8sgateway.Ingress, error) {
	return k8sgateway.Ingress{}, nil
}
func (s *diagSourceDeploymentStub) PersistentVolumeClaim(context.Context, int64, string, string) (k8sgateway.PersistentVolumeClaim, error) {
	return k8sgateway.PersistentVolumeClaim{}, nil
}
func (s *diagSourceDeploymentStub) HorizontalPodAutoscaler(context.Context, int64, string, string) (k8sgateway.HorizontalPodAutoscaler, error) {
	return k8sgateway.HorizontalPodAutoscaler{}, nil
}
func (s *diagSourceDeploymentStub) ResourceEvents(context.Context, int64, string, string) ([]k8sgateway.Event, error) {
	return nil, nil
}
