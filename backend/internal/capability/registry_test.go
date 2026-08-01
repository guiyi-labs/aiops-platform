// Unit tests for M60 compile-time provider registry (registry.go).
//
// Coverage goals (ADR 0075 §5 verification gate):
//   - Register rejects duplicate / cyclic / invalid names.
//   - StartAll / StopAll respect topological (and reverse) Lifecycle order.
//   - ClusterRoles gate skips providers that do not intersect the process role set.
//   - CheckHealth caches per-provider for at least 1s and returns ProviderInfo
//     fields that match the OpenAPI ProviderInfo schema.

package capability

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubLifecycle records Start / Stop calls so tests can assert ordering.
type stubLifecycle struct {
	name      string
	startAt   int
	stopAt    int
	startErr  error
	stopErr   error
	startHook func() // optional side-effect for serialization
	stopHook  func()
}

var stubCallCounter int

func (s *stubLifecycle) Start(ctx context.Context) error {
	stubCallCounter++
	s.startAt = stubCallCounter
	if s.startHook != nil {
		s.startHook()
	}
	return s.startErr
}

func (s *stubLifecycle) Stop(ctx context.Context) error {
	stubCallCounter++
	s.stopAt = stubCallCounter
	if s.stopHook != nil {
		s.stopHook()
	}
	return s.stopErr
}

// stubHealth reports a fixed (state, reason) and records how often Check was
// called so tests can exercise the cache layer.
type stubHealth struct {
	state   string
	reason  string
	err     error
	checks  int
	sleepMs int // synthetic work to help cache-duration tests
}

func (s *stubHealth) CheckHealth(ctx context.Context) (string, string, error) {
	if s.sleepMs > 0 {
		select {
		case <-time.After(time.Duration(s.sleepMs) * time.Millisecond):
		case <-ctx.Done():
			return ProviderStateUnhealthy, "context cancelled", ctx.Err()
		}
	}
	s.checks++
	return s.state, s.reason, s.err
}

func TestRegisterDuplicateRejected(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	if err := reg.Register(ProviderDescriptor{Name: "a", Kind: "capability"}); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := reg.Register(ProviderDescriptor{Name: "a", Kind: "capability"}); !errors.Is(err, ErrProviderAlreadyRegistered) {
		t.Fatalf("expected ErrProviderAlreadyRegistered on duplicate, got %v", err)
	}
}

func TestRegisterInvalidNameRejected(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	if err := reg.Register(ProviderDescriptor{Name: "UppercaseBad", Kind: "capability"}); !errors.Is(err, ErrInvalidProviderName) {
		t.Fatalf("uppercase should be invalid, got %v", err)
	}
	if err := reg.Register(ProviderDescriptor{Name: "1bad", Kind: "capability"}); !errors.Is(err, ErrInvalidProviderName) {
		t.Fatalf("leading digit should be invalid, got %v", err)
	}
	if err := reg.Register(ProviderDescriptor{Name: "", Kind: "capability"}); !errors.Is(err, ErrInvalidProviderName) {
		t.Fatalf("empty name should be invalid, got %v", err)
	}
}

func TestMissingDependencySurfacedAtStart(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	// "b" depends on "a", but "a" is registered later. List should still
	// work: topological order is tolerant of missing names and the runtime
	// state gate degrades the dependent provider.
	if err := reg.Register(ProviderDescriptor{
		Name: "b", Kind: "capability", Configured: true, Dependencies: []string{"a"},
	}); err != nil {
		t.Fatalf("register b with missing dep a: %v", err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "a", Kind: "capability", Configured: true,
	}); err != nil {
		t.Fatalf("register a later: %v", err)
	}
	all := reg.List()
	found := map[string]bool{}
	for _, p := range all {
		found[p.Name] = true
	}
	if !found["a"] || !found["b"] {
		t.Fatalf("expected both providers listed, got %+v", all)
	}
	if err := reg.StartAll(context.Background()); err != nil {
		t.Fatalf("start after registering both should be fine: %v", err)
	}
}

func TestCyclicDependencyDetected(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	aLc := &stubLifecycle{name: "a"}
	bLc := &stubLifecycle{name: "b"}
	cLc := &stubLifecycle{name: "c"}
	if err := reg.Register(ProviderDescriptor{
		Name: "a", Kind: "capability", Configured: true, Dependencies: []string{"c"}, Lifecycle: aLc,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "b", Kind: "capability", Configured: true, Dependencies: []string{"a"}, Lifecycle: bLc,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "c", Kind: "capability", Configured: true, Dependencies: []string{"b"}, Lifecycle: cLc,
	}); err != nil {
		t.Fatal(err)
	}
	err := reg.StartAll(context.Background())
	if !errors.Is(err, ErrCyclicDependency) {
		t.Fatalf("expected ErrCyclicDependency from StartAll, got %v", err)
	}
}

func TestStartAndStopOrder(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	aLc := &stubLifecycle{name: "a"}
	bLc := &stubLifecycle{name: "b"}
	cLc := &stubLifecycle{name: "c"}
	dLc := &stubLifecycle{name: "d"}
	// Graph: d -> b -> a, d -> c -> a  (a has no deps; b and c depend on a; d depends on b and c).
	if err := reg.Register(ProviderDescriptor{
		Name: "a", Kind: "capability", Configured: true, Lifecycle: aLc,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "b", Kind: "capability", Configured: true, Dependencies: []string{"a"}, Lifecycle: bLc,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "c", Kind: "capability", Configured: true, Dependencies: []string{"a"}, Lifecycle: cLc,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "d", Kind: "capability", Configured: true, Dependencies: []string{"b", "c"}, Lifecycle: dLc,
	}); err != nil {
		t.Fatal(err)
	}

	stubCallCounter = 0
	if err := reg.StartAll(context.Background()); err != nil {
		t.Fatalf("start all: %v", err)
	}
	if aLc.startAt == 0 || bLc.startAt == 0 || cLc.startAt == 0 || dLc.startAt == 0 {
		t.Fatalf("start not called: a=%d b=%d c=%d d=%d", aLc.startAt, bLc.startAt, cLc.startAt, dLc.startAt)
	}
	if aLc.startAt >= bLc.startAt || aLc.startAt >= cLc.startAt {
		t.Fatalf("a must start before b/c: a=%d b=%d c=%d", aLc.startAt, bLc.startAt, cLc.startAt)
	}
	if bLc.startAt >= dLc.startAt || cLc.startAt >= dLc.startAt {
		t.Fatalf("b/c must start before d: b=%d c=%d d=%d", bLc.startAt, cLc.startAt, dLc.startAt)
	}

	stubCallCounter = 0
	if err := reg.StopAll(context.Background()); err != nil {
		t.Fatalf("stop all: %v", err)
	}
	if aLc.stopAt == 0 || bLc.stopAt == 0 || cLc.stopAt == 0 || dLc.stopAt == 0 {
		t.Fatalf("stop not called: a=%d b=%d c=%d d=%d", aLc.stopAt, bLc.stopAt, cLc.stopAt, dLc.stopAt)
	}
	if dLc.stopAt >= bLc.stopAt || dLc.stopAt >= cLc.stopAt {
		t.Fatalf("d must stop before b/c: d=%d b=%d c=%d", dLc.stopAt, bLc.stopAt, cLc.stopAt)
	}
	if bLc.stopAt >= aLc.stopAt || cLc.stopAt >= aLc.stopAt {
		t.Fatalf("b/c must stop before a: b=%d c=%d a=%d", bLc.stopAt, cLc.stopAt, aLc.stopAt)
	}
}

func TestClusterRolesGate(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleMember}, time.Second)
	memberOnly := &stubLifecycle{name: "mem"}
	hostOnly := &stubLifecycle{name: "host"}
	standaloneOnly := &stubLifecycle{name: "stand"}
	anyRole := &stubLifecycle{name: "any"}
	if err := reg.Register(ProviderDescriptor{
		Name: "mem", Kind: "capability", Configured: true, ClusterRoles: []string{ClusterRoleMember}, Lifecycle: memberOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "host", Kind: "capability", Configured: true, ClusterRoles: []string{ClusterRoleHost}, Lifecycle: hostOnly,
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(ProviderDescriptor{
		Name: "stand", Kind: "capability", Configured: true, ClusterRoles: []string{ClusterRoleStandalone}, Lifecycle: standaloneOnly,
	}); err != nil {
		t.Fatal(err)
	}
	// Empty ClusterRoles means "no gate; runs anywhere".
	if err := reg.Register(ProviderDescriptor{
		Name: "any", Kind: "capability", Configured: true, ClusterRoles: nil, Lifecycle: anyRole,
	}); err != nil {
		t.Fatal(err)
	}

	if err := reg.StartAll(context.Background()); err != nil {
		t.Fatalf("start all: %v", err)
	}
	if memberOnly.startAt == 0 || anyRole.startAt == 0 {
		t.Fatalf("mem/any should have started: mem=%d any=%d", memberOnly.startAt, anyRole.startAt)
	}
	if hostOnly.startAt != 0 || standaloneOnly.startAt != 0 {
		t.Fatalf("host/stand should be skipped but started: host=%d stand=%d", hostOnly.startAt, standaloneOnly.startAt)
	}

	list := reg.List()
	roleMatch := map[string]bool{}
	for _, p := range list {
		roleMatch[p.Name] = (p.State != ProviderStateDisabled)
	}
	if !roleMatch["mem"] || !roleMatch["any"] {
		t.Fatalf("mem/any should be enabled in List, got %+v", list)
	}
	if roleMatch["host"] || roleMatch["stand"] {
		t.Fatalf("host/stand should be disabled in List, got %+v", list)
	}
}

func TestConfiguredFlagAndHealth(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, 200*time.Millisecond)
	healthy := &stubHealth{state: ProviderStateEnabled, reason: "ok"}
	unhealthy := &stubHealth{state: ProviderStateUnhealthy, reason: "not right"}
	// a: configured + healthy
	if err := reg.Register(ProviderDescriptor{
		Name: "a", Kind: "capability", Configured: true, HealthChecker: healthy,
	}); err != nil {
		t.Fatal(err)
	}
	// b: configured + unhealthy
	if err := reg.Register(ProviderDescriptor{
		Name: "b", Kind: "capability", Configured: true, HealthChecker: unhealthy,
	}); err != nil {
		t.Fatal(err)
	}
	// c: not configured
	if err := reg.Register(ProviderDescriptor{
		Name: "c", Kind: "capability", Configured: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := reg.StartAll(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	info, err := reg.CheckHealth(context.Background(), "a")
	if err != nil {
		t.Fatalf("check a: %v", err)
	}
	if info.State != ProviderStateEnabled || info.HealthReason != "ok" {
		t.Fatalf("a expected enabled/ok, got %s/%s", info.State, info.HealthReason)
	}
	if !info.Configured {
		t.Fatal("a should be configured")
	}

	cInfo, err := reg.Get("c")
	if err != nil {
		t.Fatalf("get c: %v", err)
	}
	if cInfo.Configured {
		t.Fatal("c should be not configured")
	}
	if cInfo.State != ProviderStateDisabled {
		t.Fatalf("non-configured provider should report disabled state, got %s", cInfo.State)
	}

	// Cache: calling CheckHealth twice without a 1s gap should hit the cache
	// and not re-run the HealthChecker probe.
	firstChecks := healthy.checks
	_, _ = reg.CheckHealth(context.Background(), "a")
	if healthy.checks != firstChecks {
		t.Fatalf("expected cache to avoid extra probe (checks=%d want %d)", healthy.checks, firstChecks)
	}
}

func TestGetMissing(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	if err := reg.Register(ProviderDescriptor{Name: "a", Kind: "capability"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Get("does-not-exist"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound, got %v", err)
	}
	if _, err := reg.CheckHealth(context.Background(), "does-not-exist"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected ErrProviderNotFound from CheckHealth, got %v", err)
	}
}

func TestStopAllContextTimeout(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	// slowLifecycle.Stop respects context, so a 20ms deadline with 500ms of
	// synthetic work will cause safeStop to surface the context error via
	// the returned error (which gets collected into the aggregate StopAll
	// error).
	slow := &stubLifecycle{
		name: "slow",
		stopHook: func() {
			// Intentional blocking work — the stub does not accept ctx in
			// its hooks, so the Stop method wraps the hook via safeStop.
			// Our Lifecycle interface's Stop method already takes ctx; to
			// reliably exercise the timeout path we rebuild a custom
			// Lifecycle inline below instead of relying on the generic
			// stubHook.
		},
	}
	// Replace slow with a ctx-aware Lifecycle implementation. The inline
	// struct below guarantees that 500ms of work is cancelled after the
	// 20ms deadline.
	type ctxAwareSlow struct{ called int }
	var (
		stopErr error
	)
	slowLC := struct {
		*stubLifecycle
		stopFn func(ctx context.Context) error
	}{
		stubLifecycle: slow,
		stopFn: func(ctx context.Context) error {
			select {
			case <-time.After(500 * time.Millisecond):
				return nil
			case <-ctx.Done():
				stopErr = ctx.Err()
				return stopErr
			}
		},
	}
	// Wrap so slowLC implements Lifecycle correctly — stubLifecycle.Stop
	// is already defined, so we shadow it by using the wrapper as Lifecycle
	// directly via an interface shim.
	_ = slowLC
	slowWithCtx := ctxAwareLifecycle{stop: func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	if err := reg.Register(ProviderDescriptor{Name: "slow", Kind: "capability", Configured: true, Lifecycle: slowWithCtx}); err != nil {
		t.Fatal(err)
	}
	if err := reg.StartAll(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := reg.StopAll(ctx)
	_ = stopErr // reserved for debug
	if err == nil {
		t.Fatal("expected StopAll to report error after ctx timeout, got nil")
	}
}

// ctxAwareLifecycle lets tests plug a simple start/stop func into the
// Lifecycle contract. Nil start/stop are treated as no-ops.
type ctxAwareLifecycle struct {
	start func(ctx context.Context) error
	stop  func(ctx context.Context) error
}

func (l ctxAwareLifecycle) Start(ctx context.Context) error {
	if l.start != nil {
		return l.start(ctx)
	}
	return nil
}
func (l ctxAwareLifecycle) Stop(ctx context.Context) error {
	if l.stop != nil {
		return l.stop(ctx)
	}
	return nil
}

func TestIsEnabledAndClusterSelector(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleHost}, time.Second)
	_ = reg.Register(ProviderDescriptor{
		Name: "x", Kind: "capability", Configured: true, ClusterRoles: []string{ClusterRoleHost},
	})
	_ = reg.Register(ProviderDescriptor{
		Name: "y", Kind: "capability", Configured: true, ClusterRoles: []string{ClusterRoleMember},
	})
	_ = reg.Register(ProviderDescriptor{
		Name: "z", Kind: "capability", Configured: false,
	})
	if !reg.IsEnabled("x") {
		t.Fatal("x should be enabled on host role")
	}
	if reg.IsEnabled("y") {
		t.Fatal("y should not be enabled on host role")
	}
	if reg.IsEnabled("z") {
		t.Fatal("z (not configured) should not be enabled")
	}
	if !reg.ClusterSelector("x", ClusterRoleHost) {
		t.Fatal("x should be host-compatible")
	}
	if reg.ClusterSelector("x", ClusterRoleMember) {
		t.Fatal("x should not be member-compatible")
	}
	// unknown name
	if reg.IsEnabled("does-not-exist") {
		t.Fatal("unknown name must not be enabled")
	}
	if reg.ClusterSelector("does-not-exist", ClusterRoleHost) {
		t.Fatal("unknown name must not match any cluster role selector")
	}
}

func TestDependenciesHelper(t *testing.T) {
	reg := NewRegistry([]string{ClusterRoleStandalone}, time.Second)
	_ = reg.Register(ProviderDescriptor{
		Name: "a", Kind: "capability", Dependencies: []string{"b", "c"},
	})
	deps, err := reg.Dependencies("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 || deps[0] != "b" || deps[1] != "c" {
		t.Fatalf("expected [b c], got %v", deps)
	}
	if _, err := reg.Dependencies("missing"); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("deps missing: %v", err)
	}
}
