package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/signal"
)

func newSignalTestEngine(t *testing.T, svc *signal.Service, sources signal.SourceReader) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := signalHandler{service: svc, sources: sources}
	api := r.Group("/api/v1/aiops")
	api.GET("/overview", h.overview)
	api.GET("/signals", h.listSignals)
	api.GET("/signals/catalog", h.listSignalCatalog)
	return r
}

func TestSignalHandler_OverviewReturns200(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, signal.NopSourceReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSignalHandler_OverviewReturns503WhenServiceNil(t *testing.T) {
	r := newSignalTestEngine(t, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestSignalHandler_ListSignalsReturns200(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/signals?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSignalHandler_ListSignalsInvalidClusterID(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/signals?cluster_id=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSignalHandler_ListSignalsInvalidStart(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/signals?start=not-a-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSignalHandler_CatalogReturns200(t *testing.T) {
	r := newSignalTestEngine(t, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/signals/catalog", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSignalHandler_OverviewWithClusterScope(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, signal.NopSourceReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/overview?cluster_id=1&namespace=default", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSignalHandler_OverviewInvalidClusterID(t *testing.T) {
	svc := signal.NewService(signal.ServiceOptions{})
	r := newSignalTestEngine(t, svc, signal.NopSourceReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/aiops/overview?cluster_id=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// Ensure the NopSourceReader compiles and satisfies the interface.
var _ signal.SourceReader = signal.NopSourceReader{}

// Ensure mockRepo compiles with the Repository interface in handler tests.
var _ signal.Repository = signal.NopRepository{}

func TestSignalHandler_IntegrationWithIngest(t *testing.T) {
	// This is a compile-time + behavior check: ingest a diagnosis signal and
	// verify the service does not error. The HTTP layer is not exercised here
	// because the NopRepository does not persist.
	svc := signal.NewService(signal.ServiceOptions{
		Repository: &nopRepoForHandlerTest{},
		Now:        func() time.Time { return time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC) },
	})
	req := signal.IngestRequest{
		SignalID:   "diag.pod.pending.v1",
		Producer:   signal.ProducerDiagnosis,
		ClusterID:  1,
		Resource:   signal.ResourceCitation{Kind: "Pod", Namespace: "default", Name: "nginx", UID: "uid-1"},
		ObservedAt: time.Now(),
	}
	_, err := svc.Ingest(context.Background(), req)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
}

type nopRepoForHandlerTest struct{}

func (nopRepoForHandlerTest) Upsert(context.Context, *signal.Occurrence) error { return nil }
func (nopRepoForHandlerTest) List(context.Context, signal.ListFilter) ([]signal.Occurrence, int64, error) {
	return nil, 0, nil
}
func (nopRepoForHandlerTest) CountBySignal(context.Context, *int64, string, time.Time, int) ([]signal.OverviewSignal, error) {
	return nil, nil
}
func (nopRepoForHandlerTest) DeleteExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}
