package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/authz"
	"k8s-aiops.local/backend/internal/eventstream"
	"k8s-aiops.local/backend/internal/requestctx"
)

// eventStreamFakeLister is a controllable eventstream.EventLister for handler
// tests. It returns canned pages of events on each ListEvents call, mirroring
// the service-level fakeLister.
type eventStreamFakeLister struct {
	mu    sync.Mutex
	pages [][]eventstream.EventSummary
	idx   int
	err   error
}

func (f *eventStreamFakeLister) ListEvents(_ context.Context, _ int64, _ string, _ int) ([]eventstream.EventSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if f.idx >= len(f.pages) {
		return nil, nil
	}
	page := f.pages[f.idx]
	f.idx++
	return page, nil
}

// newEventStreamRouter builds a test gin engine that wraps the eventstream
// handler with a middleware stub populating requestctx.Metadata. The optional
// scopeSetter injects a restrictive namespace scope (for anti-leakage tests).
func newEventStreamRouter(handler eventstreamHandler, clusterID int64, scopeSetter func(c *gin.Context)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID:   1,
			Roles:     []string{auth.SystemAdmin},
			ClusterID: clusterID,
			RequestID: "eventstream-test",
		}))
		if scopeSetter != nil {
			scopeSetter(c)
		}
		c.Next()
	})
	router.GET("/api/v1/clusters/:cluster_id/events/stream", handler.streamEvents)
	return router
}

// setNamespaceScope injects an authz.ClusterScope into the gin context so
// ResolvedNamespaceScope returns it (mirrors requireNamespaceQueryAccess).
func setNamespaceScope(scope authz.ClusterScope) func(c *gin.Context) {
	return func(c *gin.Context) {
		c.Set(namespaceScopeKey, scope)
	}
}

func TestEventStreamReturns503WhenServiceNil(t *testing.T) {
	router := newEventStreamRouter(eventstreamHandler{service: nil}, 1, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/stream", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestEventStreamEmptyScopeClosesImmediately(t *testing.T) {
	// Empty scope (no grants, not all-namespaces) → stream closes immediately.
	// The handler writes a hello + stream-closed event, then returns.
	lister := &eventStreamFakeLister{pages: [][]eventstream.EventSummary{{newSSEEvent("a")}}}
	svc, _ := eventstream.NewService(eventstream.Config{PollInterval: 50 * time.Millisecond}, lister)
	router := newEventStreamRouter(eventstreamHandler{service: svc}, 1, setNamespaceScope(authz.ClusterScope{AllNamespaces: false}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/stream", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: hello") {
		t.Fatalf("expected hello event, got: %s", body)
	}
	if !strings.Contains(body, "event: stream-closed") {
		t.Fatalf("expected stream-closed event, got: %s", body)
	}
	// Empty scope must not deliver any events.
	if strings.Contains(body, "event: event") {
		t.Fatalf("empty scope must not deliver events, got: %s", body)
	}
}

func TestEventStreamAllNamespacesDeliversEvents(t *testing.T) {
	lister := &eventStreamFakeLister{pages: [][]eventstream.EventSummary{
		{newSSEEvent("a"), newSSEEvent("b")},
	}}
	svc, _ := eventstream.NewService(eventstream.Config{PollInterval: 50 * time.Millisecond, BufferCap: 64}, lister)
	router := newEventStreamRouter(eventstreamHandler{service: svc}, 1, setNamespaceScope(authz.ClusterScope{AllNamespaces: true}))

	// Use a cancellable context to stop the stream after receiving events.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// Run the handler in a goroutine; cancel after a short delay.
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	// Wait briefly for events to be delivered, then cancel.
	select {
	case <-time.After(300 * time.Millisecond):
	case <-done:
	}
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event: hello") {
		t.Fatalf("expected hello event, got: %s", body)
	}
	if !strings.Contains(body, "\"uid\":\"a\"") || !strings.Contains(body, "\"uid\":\"b\"") {
		t.Fatalf("expected events a and b, got: %s", body)
	}
}

func TestEventStreamNamespaceScopedDeliversEvents(t *testing.T) {
	// Namespace-scoped: poll only the authorized namespace.
	lister := &eventStreamFakeLister{pages: [][]eventstream.EventSummary{
		{newSSEEvent("scoped-a")},
	}}
	svc, _ := eventstream.NewService(eventstream.Config{PollInterval: 50 * time.Millisecond, BufferCap: 64}, lister)
	scope := authz.ClusterScope{AllNamespaces: false, NamespaceGrants: []string{"default"}}
	router := newEventStreamRouter(eventstreamHandler{service: svc}, 1, setNamespaceScope(scope))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-time.After(300 * time.Millisecond):
	case <-done:
	}
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "\"uid\":\"scoped-a\"") {
		t.Fatalf("expected scoped-a event, got: %s", body)
	}
}

func TestEventStreamSetsSSEHeaders(t *testing.T) {
	lister := &eventStreamFakeLister{}
	svc, _ := eventstream.NewService(eventstream.Config{PollInterval: 50 * time.Millisecond}, lister)
	router := newEventStreamRouter(eventstreamHandler{service: svc}, 1, setNamespaceScope(authz.ClusterScope{AllNamespaces: true}))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/1/events/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()
	// Wait briefly for headers to be written, then cancel.
	select {
	case <-time.After(150 * time.Millisecond):
	case <-done:
	}
	cancel()
	<-done

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cc)
	}
	if ab := rec.Header().Get("X-Accel-Buffering"); ab != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", ab)
	}
}

// newSSEEvent builds an EventSummary for handler tests.
func newSSEEvent(uid string) eventstream.EventSummary {
	return eventstream.EventSummary{
		UID: uid, Name: uid, Namespace: "default", Kind: "Pod",
		Type: "Normal", Reason: "Started", Message: uid,
		LastTimestamp: "2026-08-01T00:00:00Z",
	}
}
