package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/eventstream"
	"k8s-aiops.local/backend/internal/requestctx"
)

// eventstreamHandler exposes the M51 bounded SSE stream over Kubernetes Events.
//
// Route:
//
//	GET /api/v1/clusters/:cluster_id/events/stream — bounded SSE event feed
//
// Authorization:
//   - Registered under resourceRoutes, which applies requireClusterAccess +
//     requireNamespaceQueryAccess (M35 scope). The handler resolves the
//     caller's namespace scope via ResolvedNamespaceScope(c) and passes the
//     authorized namespace set to the stream. Empty scope (no grants and not
//     all-namespaces) yields an immediately-closed empty stream — anti-leakage
//     returns an empty stream rather than 404 (ADR 0066 §3).
//   - The optional ?namespace= query param is honoured by the middleware: when
//     present, the scope is narrowed to that single namespace (404 if
//     unauthorized).
type eventstreamHandler struct {
	service *eventstream.Service
}

// streamEvents handles GET /api/v1/clusters/:cluster_id/events/stream.
//
// The handler resolves the M35 namespace scope set by
// requireNamespaceQueryAccess. When the scope is all-namespaces, the stream
// polls cluster-wide; otherwise it polls each authorized namespace. An empty
// scope (no grants, not all-namespaces) yields an immediately-closed empty
// stream — the client receives a final "stream-closed" event and the
// connection ends (anti-leakage: empty scope → empty stream, ADR 0066 §3).
//
// The handler returns 503 when the eventstream service is not configured.
func (h eventstreamHandler) streamEvents(c *gin.Context) {
	if h.service == nil {
		writeError(c, http.StatusServiceUnavailable, "EVENTSTREAM_UNAVAILABLE", "event stream service is not configured")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	scope := ResolvedNamespaceScope(c)
	namespaces := scope.NamespaceGrants
	allNamespaces := scope.AllNamespaces

	// Subscribe with the request context so client disconnect cancels the
	// poller goroutine. The service closes the stream when ctx is cancelled.
	stream, err := h.service.Subscribe(c.Request.Context(), metadata.ClusterID, namespaces, allNamespaces)
	if err != nil {
		if errors.Is(err, eventstream.ErrClusterMissing) {
			writeError(c, http.StatusServiceUnavailable, "EVENTSTREAM_UNAVAILABLE", "event stream service is not configured")
			return
		}
		writeError(c, http.StatusInternalServerError, "EVENTSTREAM_FAILED", "unable to open event stream")
		return
	}

	// SSE headers. X-Accel-Buffering disables proxy buffering (nginx).
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	// Initial hello so the client knows the stream is alive and can detect
	// an immediate close (empty scope) without waiting for the first event.
	writeSSEEvent(c.Writer, flusher, "hello", gin.H{"cluster_id": metadata.ClusterID})

	for {
		select {
		case <-c.Request.Context().Done():
			writeSSEEvent(c.Writer, flusher, "stream-closed", gin.H{"reason": "client-disconnect"})
			return
		case <-stream.Done:
			writeSSEEvent(c.Writer, flusher, "stream-closed", gin.H{"reason": "scope-empty-or-poller-stopped"})
			return
		case event, ok := <-stream.Events:
			if !ok {
				writeSSEEvent(c.Writer, flusher, "stream-closed", gin.H{"reason": "events-channel-closed"})
				return
			}
			writeSSEEvent(c.Writer, flusher, "event", event)
		}
	}
}

// writeSSEEvent writes one SSE message and flushes. The event name is emitted
// as the SSE "event:" field; the payload is JSON-encoded into the "data:"
// field. A nil flusher (e.g. in tests with httptest.NewRecorder) is tolerated
// — the bytes are still written, just not flushed.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	if _, err := w.Write(append(data, '\n', '\n')); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
}
