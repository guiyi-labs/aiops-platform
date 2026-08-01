package eventstream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeLister is a controllable EventLister for tests.
type fakeLister struct {
	mu       sync.Mutex
	pages    [][]EventSummary // each poll returns pages[next] then advances
	calls    int
	err      error
	advanced chan struct{}
}

func (f *fakeLister) ListEvents(_ context.Context, clusterID int64, namespace string, limit int) ([]EventSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if len(f.pages) == 0 {
		return nil, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	if f.advanced != nil {
		select {
		case f.advanced <- struct{}{}:
		default:
		}
	}
	return page, nil
}

func newEvent(uid string) EventSummary {
	return EventSummary{UID: uid, Name: uid, Namespace: "default", Kind: "Pod", Type: "Normal", Reason: "Started", Message: uid, LastTimestamp: "2026-08-01T00:00:00Z"}
}

func TestNewServiceDefaults(t *testing.T) {
	svc, err := NewService(Config{}, &fakeLister{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.pollInterval != DefaultPollInterval {
		t.Fatalf("pollInterval = %v, want %v", svc.pollInterval, DefaultPollInterval)
	}
	if svc.bufferCap != DefaultBufferCap {
		t.Fatalf("bufferCap = %v, want %v", svc.bufferCap, DefaultBufferCap)
	}
	if svc.listLimit != DefaultListLimit {
		t.Fatalf("listLimit = %v, want %v", svc.listLimit, DefaultListLimit)
	}
}

func TestNewServiceNilLister(t *testing.T) {
	svc, err := NewService(Config{}, nil)
	if err != nil {
		t.Fatalf("NewService nil lister: %v", err)
	}
	if svc.lister != nil {
		t.Fatalf("lister should be nil")
	}
}

func TestNewServiceInvalidConfig(t *testing.T) {
	cases := []Config{
		{PollInterval: 10 * time.Millisecond},
		{PollInterval: 61 * time.Second},
		{BufferCap: 8},
		{BufferCap: 2048},
		{ListLimit: 2000},
	}
	for i, cfg := range cases {
		if _, err := NewService(cfg, &fakeLister{}); err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
	}
}

func TestSubscribeNilListerReturnsError(t *testing.T) {
	svc, _ := NewService(Config{}, nil)
	_, err := svc.Subscribe(context.Background(), 1, nil, true)
	if !errors.Is(err, ErrClusterMissing) {
		t.Fatalf("Subscribe nil lister: err = %v, want ErrClusterMissing", err)
	}
}

func TestSubscribeEmptyScopeClosesImmediately(t *testing.T) {
	svc, _ := NewService(Config{PollInterval: 50 * time.Millisecond}, &fakeLister{})
	stream, err := svc.Subscribe(context.Background(), 1, nil, false)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	select {
	case <-stream.Done:
	case <-time.After(time.Second):
		t.Fatal("empty-scope stream should close immediately")
	}
	// Events channel should be closed; receive should not block.
	select {
	case _, ok := <-stream.Events:
		if ok {
			t.Fatal("expected events channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("events channel should be closed")
	}
}

func TestSubscribeDeliversDedupedEvents(t *testing.T) {
	lister := &fakeLister{
		pages: [][]EventSummary{
			{newEvent("a"), newEvent("b")},
			{newEvent("b"), newEvent("c")}, // "b" is a duplicate
		},
	}
	svc, _ := NewService(Config{PollInterval: 50 * time.Millisecond, BufferCap: 64}, lister)
	stream, err := svc.Subscribe(context.Background(), 1, []string{"default"}, false)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stream.close()

	got := make(map[string]struct{})
	timeout := time.After(2 * time.Second)
	for len(got) < 3 {
		select {
		case ev, ok := <-stream.Events:
			if !ok {
				t.Fatalf("stream closed early; got=%v", got)
			}
			got[ev.UID] = struct{}{}
		case <-timeout:
			t.Fatalf("timeout; got=%v", got)
		}
	}
	for _, want := range []string{"a", "b", "c"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing event %q; got=%v", want, got)
		}
	}
}

func TestPollErrorDoesNotTerminateStream(t *testing.T) {
	lister := &fakeLister{err: errors.New("gateway down")}
	svc, _ := NewService(Config{PollInterval: 50 * time.Millisecond}, lister)
	stream, err := svc.Subscribe(context.Background(), 1, []string{"default"}, false)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stream.close()
	// Let a few poll cycles elapse.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-stream.Done:
		t.Fatal("poll error should not close the stream")
	default:
	}
}

func TestPushEventDropOldestBackpressure(t *testing.T) {
	ch := make(chan EventSummary, 2)
	ch <- newEvent("a")
	ch <- newEvent("b")          // full now
	pushEvent(ch, newEvent("c")) // should drop "a"
	got := []string{}
	for len(got) < 2 {
		select {
		case ev := <-ch:
			got = append(got, ev.UID)
		case <-time.After(time.Second):
			t.Fatal("timeout draining channel")
		}
	}
	if got[0] != "b" || got[1] != "c" {
		t.Fatalf("drop-oldest = %v, want [b c]", got)
	}
}

func TestSeenRingAddAndEvict(t *testing.T) {
	r := newSeenRing(3)
	if !r.add("a") {
		t.Fatal("a should be new")
	}
	if r.add("a") {
		t.Fatal("a should be duplicate")
	}
	r.add("b")
	r.add("c")
	// Ring capacity 3: adding "d" evicts "a".
	if !r.add("d") {
		t.Fatal("d should be new")
	}
	if !r.add("a") {
		t.Fatal("a should be new again after eviction")
	}
}

func TestSeenRingEmptyUID(t *testing.T) {
	r := newSeenRing(4)
	if r.add("") {
		t.Fatal("empty UID should never be delivered")
	}
}

func TestSubscribeContextCancelClosesStream(t *testing.T) {
	lister := &fakeLister{}
	svc, _ := NewService(Config{PollInterval: 50 * time.Millisecond}, lister)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := svc.Subscribe(ctx, 1, []string{"default"}, false)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	select {
	case <-stream.Done:
	case <-time.After(time.Second):
		t.Fatal("context cancel should close the stream")
	}
}
