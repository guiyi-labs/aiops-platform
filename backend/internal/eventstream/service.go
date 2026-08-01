// Package eventstream implements a bounded Server-Sent Events stream over
// Kubernetes Events. It reuses the existing read-only Kubernetes gateway
// (ADR 0004) via periodic polling — no Watch API, no transparent proxy, no
// client-supplied field selectors beyond a fixed namespace filter.
//
// The stream is bounded by three structural invariants (ADR 0066):
//
//  1. Polling, not watching — the gateway is read-only and bounded; a Watch
//     would require a persistent Kubernetes client connection and a resource
//     version bookkeeping that the bounded-gateway model forbids.
//  2. Drop-oldest backpressure — when a client's buffered channel is full, the
//     newest event overwrites the oldest; the stream never blocks the poller
//     and never accumulates unbounded memory.
//  3. M35 namespace scope filter — the poller only fetches namespaces the
//     caller is authorized to see; unauthorized namespaces are never polled.
package eventstream

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Errors returned by the service. Handlers map these to HTTP status codes;
// ErrClusterUnavailable mirrors the kubernetes gateway 404/409 mapping so the
// anti-leakage (404 > 403) invariant is preserved.
var (
	ErrInvalidConfig  = errors.New("eventstream: invalid config")
	ErrStreamClosed   = errors.New("eventstream: stream closed")
	ErrClusterMissing = errors.New("eventstream: cluster unavailable")
)

// Constants bounding the stream. The poll interval is the minimum cadence; the
// buffer cap bounds per-client memory and implements drop-oldest backpressure.
const (
	DefaultPollInterval = 5 * time.Second
	MinPollInterval     = 50 * time.Millisecond
	MaxPollInterval     = 60 * time.Second
	DefaultBufferCap    = 256
	MinBufferCap        = 16
	MaxBufferCap        = 1024
	DefaultListLimit    = 500
	MaxListLimit        = 1000
)

// Config bounds the stream. Zero values fall back to defaults in NewService.
type Config struct {
	PollInterval time.Duration
	BufferCap    int
	ListLimit    int
}

// EventSummary is the JSON-serializable event pushed to SSE clients. It is a
// projection of kubernetes.Event — only the fields needed for the console
// event feed, no raw manifest, no internal cluster IDs.
type EventSummary struct {
	UID            string    `json:"uid"`
	Name           string    `json:"name"`
	Namespace      string    `json:"namespace"`
	Kind           string    `json:"kind"`
	Type           string    `json:"type"`
	Reason         string    `json:"reason"`
	Message        string    `json:"message"`
	Count          int32     `json:"count"`
	LastTimestamp  string    `json:"last_timestamp"`
	FirstTimestamp string    `json:"first_timestamp"`
	ClusterID      int64     `json:"cluster_id"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// EventLister is the minimal interface the poller needs. It is satisfied by
// *kubernetes.Service; abstracting it keeps the eventstream package decoupled
// from the kubernetes package and lets handlers inject a fake in tests.
type EventLister interface {
	// ListEvents returns recent events for a cluster/namespace pair. An empty
	// namespace means cluster-wide (only valid when the caller has cluster-wide
	// scope). The limit bounds the page size.
	ListEvents(ctx context.Context, clusterID int64, namespace string, limit int) ([]EventSummary, error)
}

// Stream is a single client subscription. Events are delivered on the Events
// channel; Done is closed when the stream terminates (client disconnect,
// context cancel, or poller error). The poller dedupes by UID before pushing.
type Stream struct {
	Events <-chan EventSummary
	Done   <-chan struct{}

	events chan EventSummary
	done   chan struct{}
	once   sync.Once
}

// close terminates the stream. Safe to call multiple times.
func (s *Stream) close() {
	s.once.Do(func() {
		close(s.done)
		// Drain the events channel without blocking so the poller's send does
		// not deadlock on a closed client. Non-blocking send + close is safe
		// because the poller is the only sender and it has already stopped.
		close(s.events)
	})
}

// Service manages bounded event streams. It is safe for concurrent use.
type Service struct {
	lister       EventLister
	pollInterval time.Duration
	bufferCap    int
	listLimit    int
}

// NewService returns a service with validated config. A nil lister is allowed
// (the handler returns 503), mirroring the monitoring service pattern.
func NewService(config Config, lister EventLister) (*Service, error) {
	if lister == nil {
		// Nil lister is permitted for route-contract wiring; the handler
		// returns 503. No config validation is needed in this case.
		return &Service{
			lister:       nil,
			pollInterval: DefaultPollInterval,
			bufferCap:    DefaultBufferCap,
			listLimit:    DefaultListLimit,
		}, nil
	}
	poll := config.PollInterval
	if poll == 0 {
		poll = DefaultPollInterval
	}
	if poll < MinPollInterval || poll > MaxPollInterval {
		return nil, ErrInvalidConfig
	}
	cap := config.BufferCap
	if cap == 0 {
		cap = DefaultBufferCap
	}
	if cap < MinBufferCap || cap > MaxBufferCap {
		return nil, ErrInvalidConfig
	}
	limit := config.ListLimit
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit < 1 || limit > MaxListLimit {
		return nil, ErrInvalidConfig
	}
	return &Service{
		lister:       lister,
		pollInterval: poll,
		bufferCap:    cap,
		listLimit:    limit,
	}, nil
}

// Subscribe opens a bounded stream for one cluster + namespace set. The
// namespaces slice is the M35-authorized namespace set; when empty (and
// allNamespaces is true) the poller fetches cluster-wide. When empty and
// allNamespaces is false, the stream immediately closes with no events
// (anti-leakage: empty scope → empty stream, not 404).
//
// The stream runs until ctx is cancelled. The caller is responsible for
// propagating client-disconnect cancellation through ctx.
func (s *Service) Subscribe(ctx context.Context, clusterID int64, namespaces []string, allNamespaces bool) (*Stream, error) {
	if s.lister == nil {
		return nil, ErrClusterMissing
	}
	events := make(chan EventSummary, s.bufferCap)
	done := make(chan struct{})
	stream := &Stream{Events: events, Done: done, events: events, done: done}

	// If the caller has no namespace grants and not all-namespaces, there is
	// nothing to stream. Close immediately with an empty events channel.
	if !allNamespaces && len(namespaces) == 0 {
		stream.close()
		return stream, nil
	}

	go s.poll(ctx, clusterID, namespaces, allNamespaces, stream)
	return stream, nil
}

// poll is the per-client goroutine. It polls the lister at pollInterval,
// dedupes by UID against a bounded ring of recently-seen UIDs, and pushes
// new events to the stream. Drop-oldest backpressure: when the events channel
// is full, the oldest buffered event is discarded and the new one is pushed.
func (s *Service) poll(ctx context.Context, clusterID int64, namespaces []string, allNamespaces bool, stream *Stream) {
	defer stream.close()

	seen := newSeenRing(s.bufferCap)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	// Determine the set of (namespace) targets to poll. Cluster-wide is a
	// single empty-namespace fetch; otherwise poll each authorized namespace.
	targets := namespaces
	if allNamespaces {
		targets = []string{""}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-stream.done:
			return
		case <-ticker.C:
		}
		for _, ns := range targets {
			if ctx.Err() != nil {
				return
			}
			events, err := s.lister.ListEvents(ctx, clusterID, ns, s.listLimit)
			if err != nil {
				// A poll error does not terminate the stream; the next tick
				// retries. This keeps transient gateway errors from killing a
				// long-lived SSE connection.
				continue
			}
			for _, event := range events {
				if !seen.add(event.UID) {
					continue // already delivered
				}
				select {
				case <-ctx.Done():
					return
				case <-stream.done:
					return
				default:
				}
				pushEvent(stream.events, event)
			}
		}
	}
}

// pushEvent delivers an event with drop-oldest backpressure. When the buffered
// channel is full, one event is drained (oldest) before the new one is pushed.
func pushEvent(ch chan EventSummary, event EventSummary) {
	for {
		select {
		case ch <- event:
			return
		default:
		}
		select {
		case <-ch: // drop oldest
		default:
			// Another goroutine drained between the non-blocking send and
			// receive; retry the send.
		}
	}
}

// seenRing is a bounded FIFO set of UIDs for dedup. Once full, the oldest UID
// is evicted, bounding memory to O(bufferCap). add returns true when the UID
// was not previously seen (i.e. the event is new and should be delivered).
type seenRing struct {
	uids []string
	idx  map[string]struct{}
	head int
	full bool
}

func newSeenRing(capacity int) *seenRing {
	return &seenRing{
		uids: make([]string, capacity),
		idx:  make(map[string]struct{}, capacity),
	}
}

func (r *seenRing) add(uid string) bool {
	if uid == "" {
		return false
	}
	if _, exists := r.idx[uid]; exists {
		return false
	}
	if r.full {
		delete(r.idx, r.uids[r.head])
	}
	r.uids[r.head] = uid
	r.idx[uid] = struct{}{}
	r.head = (r.head + 1) % len(r.uids)
	if r.head == 0 {
		r.full = true
	}
	return true
}
