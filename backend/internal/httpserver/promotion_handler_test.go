package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/promotion"
	"k8s-aiops.local/backend/internal/requestctx"
)

type promotionKubernetesStub struct{}

func (s promotionKubernetesStub) RawManifest(context.Context, int64, string, string, string) (json.RawMessage, error) {
	return json.RawMessage(`{"apiVersion":"v1"}`), nil
}
func (s promotionKubernetesStub) NamespaceExists(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (s promotionKubernetesStub) ConfigMapExists(context.Context, int64, string, string) (bool, error) {
	return false, nil
}
func (s promotionKubernetesStub) SecretExists(context.Context, int64, string, string) (bool, error) {
	return false, nil
}
func (s promotionKubernetesStub) ResourceExists(context.Context, int64, string) (bool, error) {
	return false, nil
}
func (s promotionKubernetesStub) CreateResource(context.Context, int64, string, []byte, bool) ([]byte, error) {
	return nil, nil
}

type promotionRepoStub struct {
	plan     promotion.Plan
	getErr   error
	listErr  error
	claimErr error
}

func (s promotionRepoStub) Save(context.Context, *promotion.Plan) error { return nil }
func (s promotionRepoStub) Get(context.Context, string) (promotion.Plan, error) {
	return s.plan, s.getErr
}
func (s promotionRepoStub) List(context.Context, int64, string) ([]promotion.Plan, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return []promotion.Plan{s.plan}, nil
}
func (s promotionRepoStub) Claim(context.Context, string, []byte, string, time.Time, time.Time) (promotion.Plan, bool, error) {
	return s.plan, false, s.claimErr
}
func (s promotionRepoStub) Complete(context.Context, string, string, time.Time, map[int64]string, map[int64]string) (promotion.Plan, error) {
	return s.plan, nil
}
func (s promotionRepoStub) Fail(context.Context, string, string, string) (promotion.Plan, error) {
	return s.plan, nil
}
func (s promotionRepoStub) ExpireStale(context.Context, time.Time) error { return nil }

func newPromotionRouter(repo promotionRepoStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := promotionHandler{service: promotion.NewService(promotionKubernetesStub{}, repo)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{"system_admin"}, RequestID: "promotion-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/promotions/preview", h.preview)
	router.POST("/api/v1/promotions/:promotion_id/execute", h.execute)
	router.GET("/api/v1/promotions/:promotion_id", h.get)
	router.GET("/api/v1/promotions", h.list)
	return router
}

const promotionID = "12345678-1234-1234-1234-123456789012"

func TestPromotionExecuteAndGet(t *testing.T) {
	router := newPromotionRouter(promotionRepoStub{plan: promotion.Plan{ID: promotionID}})

	// invalid promotion id
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/promotions/short/execute", strings.NewReader(`{"confirmation_token":"tok"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_PROMOTION_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing confirmation token (binding enforced)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/promotions/"+promotionID+"/execute", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing idempotency key -> service ErrInvalidIdempotency
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/promotions/"+promotionID+"/execute", strings.NewReader(`{"confirmation_token":"tok"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_IDEMPOTENCY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// expired plan -> 410
	router = newPromotionRouter(promotionRepoStub{plan: promotion.Plan{ID: promotionID}, claimErr: promotion.ErrExpired})
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/"+promotionID+"/execute", strings.NewReader(`{"confirmation_token":"tok"}`))
	req.Header.Set("Idempotency-Key", "idem-12345678")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone || !contains(rec.Body.String(), "PROMOTION_EXPIRED") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// get: success
	router = newPromotionRouter(promotionRepoStub{plan: promotion.Plan{ID: promotionID}})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/promotions/"+promotionID, nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"id":"`+promotionID+`"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// get: not found -> 404
	router = newPromotionRouter(promotionRepoStub{getErr: promotion.ErrNotFound})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/promotions/"+promotionID, nil))
	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "PROMOTION_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPromotionListAndPreview(t *testing.T) {
	router := newPromotionRouter(promotionRepoStub{plan: promotion.Plan{ID: promotionID}})

	// list: missing source_cluster_id
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/promotions", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "source_cluster_id query parameter is required") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// list: invalid source_cluster_id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/promotions?source_cluster_id=abc", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "must be a positive integer") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// list: success
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/promotions?source_cluster_id=1", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// preview: unknown field rejected
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/promotions/preview", strings.NewReader(`{"bogus":1}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// preview: bundle empty -> ErrBundleEmpty
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/promotions/preview", strings.NewReader(`{"source_cluster_id":1,"destination_cluster_id":2,"source_namespace":"ns","destination_namespace":"ns2","bundle":[]}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "PROMOTION_BUNDLE_EMPTY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPromotionWriteErrorMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := promotionHandler{}
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{err: promotion.ErrNotFound, status: http.StatusNotFound, code: "PROMOTION_NOT_FOUND"},
		{err: promotion.ErrInvalidRequest, status: http.StatusBadRequest, code: "INVALID_PROMOTION"},
		{err: promotion.ErrBundleEmpty, status: http.StatusBadRequest, code: "PROMOTION_BUNDLE_EMPTY"},
		{err: promotion.ErrSourceUnavailable, status: http.StatusConflict, code: "PROMOTION_SOURCE_UNAVAILABLE"},
		{err: promotion.ErrDestinationUnavailable, status: http.StatusConflict, code: "PROMOTION_DESTINATION_UNAVAILABLE"},
		{err: promotion.ErrNamespaceMissing, status: http.StatusConflict, code: "PROMOTION_NAMESPACE_MISSING"},
		{err: promotion.ErrDependencyUnresolved, status: http.StatusConflict, code: "PROMOTION_DEPENDENCY_UNRESOLVED"},
		{err: promotion.ErrConflict, status: http.StatusConflict, code: "PROMOTION_CONFLICT"},
		{err: promotion.ErrPreviewFailed, status: http.StatusBadRequest, code: "PROMOTION_PREVIEW_FAILED"},
		{err: promotion.ErrConfirmationInvalid, status: http.StatusForbidden, code: "PROMOTION_CONFIRMATION_INVALID"},
		{err: promotion.ErrInvalidIdempotency, status: http.StatusBadRequest, code: "INVALID_IDEMPOTENCY_KEY"},
		{err: promotion.ErrExpired, status: http.StatusGone, code: "PROMOTION_EXPIRED"},
		{err: promotion.ErrInProgress, status: http.StatusConflict, code: "PROMOTION_IN_PROGRESS"},
		{err: promotion.ErrAlreadyExecuted, status: http.StatusConflict, code: "PROMOTION_ALREADY_USED"},
		{err: promotion.ErrExecutionFailed, status: http.StatusBadGateway, code: "PROMOTION_FAILED"},
		{err: cluster.ErrDisabled, status: http.StatusConflict, code: "CLUSTER_DISABLED"},
		{err: cluster.ErrNotFound, status: http.StatusNotFound, code: "CLUSTER_NOT_FOUND"},
		{err: k8sgateway.ErrResourceNotFound, status: http.StatusNotFound, code: "RESOURCE_NOT_FOUND"},
		{err: k8sgateway.ErrResourceConflict, status: http.StatusConflict, code: "PROMOTION_CONFLICT"},
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
