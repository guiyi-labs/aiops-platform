package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/copyops"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/requestctx"
)

type copyOpsKubernetesStub struct{}

func (s copyOpsKubernetesStub) GetRawResource(context.Context, int64, string, string, string, string, string) (map[string]any, error) {
	return nil, nil
}
func (s copyOpsKubernetesStub) NamespaceExists(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (s copyOpsKubernetesStub) SourceNamespaceIdentity(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
	return k8sgateway.SourceNamespaceIdentity{}, nil
}
func (s copyOpsKubernetesStub) NamespacedResourceExists(context.Context, int64, string, string, string, string, string) (bool, error) {
	return false, nil
}
func (s copyOpsKubernetesStub) CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error) {
	return nil, nil
}

type copyOpsRepoStub struct {
	plan         copyops.Plan
	getErr       error
	byUserErr    error
	byClusterErr error
	claimErr     error
}

func (s copyOpsRepoStub) Create(context.Context, copyops.Plan) (copyops.Plan, error) {
	return s.plan, nil
}
func (s copyOpsRepoStub) GetByID(context.Context, string) (copyops.Plan, error) {
	return s.plan, s.getErr
}
func (s copyOpsRepoStub) ListByUser(context.Context, int64, int, int) ([]copyops.Plan, int, error) {
	if s.byUserErr != nil {
		return nil, 0, s.byUserErr
	}
	return []copyops.Plan{s.plan}, 1, nil
}
func (s copyOpsRepoStub) ListByCluster(context.Context, int64, int, int) ([]copyops.Plan, int, error) {
	if s.byClusterErr != nil {
		return nil, 0, s.byClusterErr
	}
	return []copyops.Plan{s.plan}, 1, nil
}
func (s copyOpsRepoStub) ClaimAndLoad(context.Context, string, string, []byte, string) (copyops.Plan, error) {
	return s.plan, s.claimErr
}
func (s copyOpsRepoStub) UpdateExecution(context.Context, copyops.Plan) error { return nil }
func (s copyOpsRepoStub) UpdateStatus(context.Context, string, string, string) error {
	return nil
}

func newCopyOpsRouter(repo copyOpsRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := copyopsHandler{service: copyops.NewService(copyOpsKubernetesStub{}, repo)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "copyops-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/clusters/:cluster_id/copy-plans/preview", h.preview)
	router.POST("/api/v1/copy-plans/:plan_id/execute", h.execute)
	router.GET("/api/v1/copy-plans/:plan_id", h.get)
	router.GET("/api/v1/copy-plans", h.listCurrentUser)
	router.GET("/api/v1/clusters/:cluster_id/copy-plans", h.listByCluster)
	return router
}

func copyPlan() copyops.Plan {
	return copyops.Plan{ID: "plan-1", SourceClusterID: 1, SourceNamespace: "ns", TargetClusterID: 2, TargetNamespace: "ns2", Status: "awaiting_confirmation"}
}

func TestCopyOpsPreviewValidation(t *testing.T) {
	router := newCopyOpsRouter(copyOpsRepoStub{})

	// target_cluster_id is required when not supplied by the body.
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/copy-plans/preview", strings.NewReader(`{"source_namespace":"ns","target_namespace":"ns2","bundle":[{"kind":"ConfigMap","name":"cm-1"}]}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "target_cluster_id is required") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// bundle too large (max 20)
	items := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		items = append(items, `{"kind":"ConfigMap","name":"cm-`+string(rune('a'+i))+`"}`)
	}
	body := `{"source_namespace":"ns","target_namespace":"ns2","target_cluster_id":2,"bundle":[` + strings.Join(items, ",") + `]}`
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/copy-plans/preview", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "COPY_BUNDLE_TOO_LARGE") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// same cluster rejected
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/clusters/1/copy-plans/preview", strings.NewReader(`{"source_namespace":"ns","target_namespace":"ns2","target_cluster_id":1,"bundle":[{"kind":"ConfigMap","name":"cm-1"}]}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "COPY_SAME_CLUSTER") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCopyOpsExecuteValidation(t *testing.T) {
	router := newCopyOpsRouter(copyOpsRepoStub{})

	// missing confirmation token
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/copy-plans/12345678-1234-1234-1234-123456789012/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing idempotency key
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/copy-plans/12345678-1234-1234-1234-123456789012/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY_KEY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// expired plan surfaces 410
	router = newCopyOpsRouter(copyOpsRepoStub{claimErr: copyops.ErrExpired})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/copy-plans/12345678-1234-1234-1234-123456789012/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "COPY_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCopyOpsGetAndList(t *testing.T) {
	router := newCopyOpsRouter(copyOpsRepoStub{plan: copyPlan()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/copy-plans/12345678-1234-1234-1234-123456789012", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"id":"plan-1"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/copy-plans", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/copy-plans?offset=-1&limit=999", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"offset":0`) || !contains(rec.Body.String(), `"limit":20`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// error paths
	router = newCopyOpsRouter(copyOpsRepoStub{getErr: copyops.ErrNotFound})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/copy-plans/12345678-1234-1234-1234-123456789012", nil))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "COPY_PLAN_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newCopyOpsRouter(copyOpsRepoStub{byUserErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/copy-plans", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	router = newCopyOpsRouter(copyOpsRepoStub{byClusterErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/copy-plans", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid cluster id
	rec = httptest.NewRecorder()
	newCopyOpsRouter(copyOpsRepoStub{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/0/copy-plans", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_CLUSTER_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestParsePagingBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		query                 string
		wantOffset, wantLimit int
	}{
		{query: "", wantOffset: 0, wantLimit: 20},
		{query: "offset=10&limit=50", wantOffset: 10, wantLimit: 50},
		{query: "offset=-3&limit=0", wantOffset: 0, wantLimit: 20},
		{query: "offset=abc&limit=999", wantOffset: 0, wantLimit: 20},
		{query: "offset=1&limit=200", wantOffset: 1, wantLimit: 200},
	}
	for _, tt := range cases {
		router := gin.New()
		router.GET("/x", func(c *gin.Context) {
			offset, limit := parsePaging(c, 0, 20)
			c.JSON(http.StatusOK, gin.H{"offset": offset, "limit": limit})
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x?"+tt.query, nil))
		if !contains(rec.Body.String(), `"offset":`+itoa(int64(tt.wantOffset))) || !contains(rec.Body.String(), `"limit":`+itoa(int64(tt.wantLimit))) {
			t.Fatalf("query=%q body=%s want offset=%d limit=%d", tt.query, rec.Body.String(), tt.wantOffset, tt.wantLimit)
		}
	}
}

func TestCopyOpsWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := copyopsHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: copyops.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_COPY_REQUEST"},
		{err: copyops.ErrBundleEmpty, status: http.StatusBadRequest, code: "COPY_BUNDLE_EMPTY"},
		{err: copyops.ErrBundleTooLarge, status: http.StatusBadRequest, code: "COPY_BUNDLE_TOO_LARGE"},
		{err: copyops.ErrKindDisallowed, status: http.StatusBadRequest, code: "COPY_KIND_DISALLOWED"},
		{err: copyops.ErrCrossClusterSame, status: http.StatusBadRequest, code: "COPY_SAME_CLUSTER"},
		{err: copyops.ErrSourceUnavailable, status: http.StatusBadGateway, code: "COPY_SOURCE_UNAVAILABLE"},
		{err: copyops.ErrSourceNotFound, status: http.StatusNotFound, code: "COPY_SOURCE_NOT_FOUND"},
		{err: copyops.ErrDestinationUnavailable, status: http.StatusBadGateway, code: "COPY_DESTINATION_UNAVAILABLE"},
		{err: copyops.ErrNamespaceMissing, status: http.StatusBadRequest, code: "COPY_NAMESPACE_MISSING"},
		{err: copyops.ErrConflict, status: http.StatusConflict, code: "COPY_CONFLICT"},
		{err: copyops.ErrPreviewFailed, status: http.StatusUnprocessableEntity, code: "COPY_PREVIEW_FAILED"},
		{err: copyops.ErrConfirmationInvalid, status: http.StatusForbidden, code: "COPY_CONFIRMATION_INVALID"},
		{err: copyops.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: copyops.ErrExpired, status: http.StatusGone, code: "COPY_EXPIRED"},
		{err: copyops.ErrInProgress, status: http.StatusConflict, code: "COPY_IN_PROGRESS"},
		{err: copyops.ErrAlreadyExecuted, status: http.StatusConflict, code: "COPY_ALREADY_USED"},
		{err: copyops.ErrExecutionFailed, status: http.StatusBadGateway, code: "COPY_EXECUTION_FAILED"},
		{err: copyops.ErrNotFound, status: http.StatusNotFound, code: "COPY_PLAN_NOT_FOUND"},
		{err: errors.New("boom"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := gin.New()
		router.GET("/x", func(c *gin.Context) { h.writeError(c, tt.err, "fallback") })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("err=%v status=%d body=%s want status=%d code=%s", tt.err, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}
