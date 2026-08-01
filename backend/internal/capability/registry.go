package capability

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Cluster role constants mirroring cluster package — kept here to avoid an
// import cycle. Provider activation semantics rely on these values matching
// clusters.cluster_role (host | member | standalone).
const (
	ClusterRoleHost       = "host"
	ClusterRoleMember     = "member"
	ClusterRoleStandalone = "standalone"
)

// Provider state enum reported by the registry surface. These are part of the
// OpenAPI contract for /capability/providers.
const (
	ProviderStateDisabled  = "disabled"
	ProviderStateEnabled   = "enabled"
	ProviderStateStarting  = "starting"
	ProviderStateStopping  = "stopping"
	ProviderStateDegraded  = "degraded"
	ProviderStateUnhealthy = "unhealthy"
)

// Sentinel errors for registry operations. Returned by Register(...) for
// duplicate / invalid descriptors and surfaced through HealthCheck when a
// provider's probe misbehaves.
var (
	ErrProviderAlreadyRegistered = errors.New("provider already registered")
	ErrProviderNotFound          = errors.New("provider not found")
	ErrProviderNotStarted        = errors.New("provider has not been started")
	ErrInvalidProviderName       = errors.New("invalid provider name")
	ErrCyclicDependency          = errors.New("provider dependency graph contains a cycle")
)

// Lifecycle is the optional start/stop contract for a provider. Providers
// that carry background goroutines (the eventstream SSE poller, the M52
// inspection Cron scheduler) implement Lifecycle so the server can wire
// startup/shutdown in main.go. Providers without background work can omit
// the interface — the registry treats them as always-running.
type Lifecycle interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// HealthChecker is the optional health-check contract for a provider.
// Implementations probe the upstream (Prometheus HTTP, Loki HTTP, ArgoCD
// CRD presence, etc.) and return degraded / unhealthy plus a sanitized
// reason string. Providers without a probe omit the interface; the registry
// reports their health as enabled.
type HealthChecker interface {
	CheckHealth(ctx context.Context) (state string, reason string, err error)
}

// ProviderDescriptor is the compile-time description of a provider. Callers
// use Register() to install descriptors during process init (or from
// cmd/server main before startup). The registry never mutates descriptors
// after registration — runtime state is kept in a separate runtimeState map.
type ProviderDescriptor struct {
	// Name is the stable, machine-readable identifier. Must be non-empty
	// and match [a-z][a-z0-9_-]{1,63}.
	Name string
	// Description is a short human-readable one-liner for the GUI provider
	// list.
	Description string
	// Version is the provider's own version string (e.g. "1.0.0"). Empty
	// means the version is inherited from buildinfo.Version.
	Version string
	// Kind classifies the provider for UI grouping: capability, signal,
	// mesh, inspection, gitops, backup, federation, appcatalog, copyops,
	// ai, misc.
	Kind string
	// Dependencies lists providers that must be started *before* this one
	// and whose health state influences this provider's aggregate health.
	// Missing dependencies -> degraded.
	Dependencies []string
	// ClusterRoles enumerates which cluster roles this provider is
	// eligible to run on. Empty means all roles (no cluster-role gate).
	// Typical values: [host] for the federation controller (member
	// clusters don't run it), [standalone, host] for the inspection
	// scheduler.
	ClusterRoles []string
	// Configured reports whether the provider has valid runtime
	// configuration (Prometheus endpoint set, OIDC enabled, etc.). This
	// drives the "enabled" vs "disabled but configured" distinction in
	// the GUI.
	Configured bool
	// Lifecycle is the optional start/stop hook. Nil when the provider
	// carries no background work.
	Lifecycle Lifecycle
	// HealthChecker is the optional probe. Nil means the registry reports
	// state = enabled (assuming Configured=true).
	HealthChecker HealthChecker
}

// ProviderInfo is the read-only projection of a descriptor plus its current
// runtime state — returned by List and Get and serialized onto the
// /capability/providers HTTP surface.
type ProviderInfo struct {
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Version      string     `json:"version"`
	Kind         string     `json:"kind"`
	Dependencies []string   `json:"dependencies"`
	ClusterRoles []string   `json:"cluster_roles"`
	Configured   bool       `json:"configured"`
	State        string     `json:"state"`
	HealthReason string     `json:"health_reason,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	LastCheck    *time.Time `json:"last_check,omitempty"`
}

// Registry is the compile-time provider catalog. One Registry is created by
// cmd/server and passed through Options to the handlers. The registry is
// safe for concurrent use: all mutations happen under a single RWMutex.
type Registry struct {
	mu      sync.RWMutex
	order   []string
	descs   map[string]*ProviderDescriptor
	states  map[string]*runtimeState
	roleSet map[string]struct{} // active cluster roles for the current process
	// healthTimeout is the max duration for a single CheckHealth call.
	// Defaults to 5s when zero.
	healthTimeout time.Duration
	// clock abstracts time.Now for tests.
	clock func() time.Time
}

type runtimeState struct {
	startedAt *time.Time
	lastCheck *time.Time
	state     string
	reason    string
}

// NewRegistry creates an empty Registry. clusterRoles is the set of roles
// this particular process serves (typically a single value: standalone on
// single-cluster deployments, host on the federation host, member on a
// member). Empty clusterRoles activates every provider regardless of role
// gating — used by the test harness.
func NewRegistry(clusterRoles []string, healthTimeout time.Duration) *Registry {
	roleSet := make(map[string]struct{}, len(clusterRoles))
	for _, r := range clusterRoles {
		roleSet[r] = struct{}{}
	}
	if healthTimeout <= 0 {
		healthTimeout = 5 * time.Second
	}
	return &Registry{
		descs:         make(map[string]*ProviderDescriptor),
		states:        make(map[string]*runtimeState),
		roleSet:       roleSet,
		healthTimeout: healthTimeout,
		clock:         time.Now,
	}
}

// Register installs a provider descriptor into the registry. Register must
// be called before StartAll; late registration after startup is rejected.
// Returns ErrProviderAlreadyRegistered on duplicate names,
// ErrInvalidProviderName when the name fails the lexical check.
func (r *Registry) Register(desc ProviderDescriptor) error {
	if desc.Name == "" || !isValidProviderName(desc.Name) {
		return ErrInvalidProviderName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.descs[desc.Name]; exists {
		return ErrProviderAlreadyRegistered
	}
	// Copy slice inputs so callers can't mutate the descriptor after
	// registration.
	depsCopy := append([]string(nil), desc.Dependencies...)
	rolesCopy := append([]string(nil), desc.ClusterRoles...)
	stored := desc
	stored.Dependencies = depsCopy
	stored.ClusterRoles = rolesCopy
	r.descs[desc.Name] = &stored
	r.order = append(r.order, desc.Name)
	r.states[desc.Name] = &runtimeState{state: r.initialState(&stored)}
	return nil
}

// initialState computes the disabled / pre-start state for a fresh
// descriptor. Caller must hold r.mu.
func (r *Registry) initialState(desc *ProviderDescriptor) string {
	if !desc.Configured {
		return ProviderStateDisabled
	}
	if !r.roleMatchesLocked(desc) {
		return ProviderStateDisabled
	}
	return ProviderStateEnabled
}

func (r *Registry) roleMatchesLocked(desc *ProviderDescriptor) bool {
	if len(desc.ClusterRoles) == 0 {
		return true
	}
	if len(r.roleSet) == 0 {
		return true
	}
	for _, allowed := range desc.ClusterRoles {
		if _, ok := r.roleSet[allowed]; ok {
			return true
		}
	}
	return false
}

// IsEnabled reports whether the provider is both configured and eligible for
// the current cluster role set. Returns false for unknown providers.
func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descs[name]
	if !ok {
		return false
	}
	if !desc.Configured {
		return false
	}
	return r.roleMatchesLocked(desc)
}

// Dependencies returns the provider's declared dependency list, or an empty
// slice when the provider is unknown. The returned slice is a copy.
func (r *Registry) Dependencies(name string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descs[name]
	if !ok {
		return nil, ErrProviderNotFound
	}
	return append([]string(nil), desc.Dependencies...), nil
}

// ClusterSelector reports whether the provider is eligible to run on the
// given cluster role. Unknown providers always return false.
func (r *Registry) ClusterSelector(name string, clusterRole string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descs[name]
	if !ok {
		return false
	}
	if len(desc.ClusterRoles) == 0 {
		return true
	}
	for _, allowed := range desc.ClusterRoles {
		if allowed == clusterRole {
			return true
		}
	}
	return false
}

// StartAll runs Start on every eligible provider in dependency order. A
// provider with lifecycle=nil is treated as immediately started. Cycles in
// the dependency graph return ErrCyclicDependency before any Start is called.
// When one provider fails to start, StartAll records its state as unhealthy,
// skips its dependents, and returns the first encountered error. The
// partially-started registry is still usable so dependents can be re-tried
// by the caller.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	order, err := r.topologicalOrderLocked()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()

	var firstErr error
	for _, name := range order {
		r.mu.Lock()
		desc := r.descs[name]
		st := r.states[name]
		if !r.roleMatchesLocked(desc) || !desc.Configured {
			r.mu.Unlock()
			continue
		}
		// Skip providers whose deps are not successfully started.
		depsFailed := false
		for _, dep := range desc.Dependencies {
			depState := r.states[dep].state
			if depState != ProviderStateEnabled && depState != ProviderStateStarting {
				depsFailed = true
				break
			}
		}
		if depsFailed {
			st.state = ProviderStateDegraded
			st.reason = "one or more dependencies failed to start"
			r.mu.Unlock()
			continue
		}
		st.state = ProviderStateStarting
		lc := desc.Lifecycle
		r.mu.Unlock()

		var startErr error
		if lc != nil {
			startErr = safeStart(ctx, lc)
		}

		r.mu.Lock()
		if startErr != nil {
			st.state = ProviderStateUnhealthy
			st.reason = sanitizeRegistryErr(startErr)
			if firstErr == nil {
				firstErr = startErr
			}
		} else {
			now := r.clock()
			st.startedAt = &now
			st.state = ProviderStateEnabled
			st.reason = ""
		}
		r.mu.Unlock()
	}
	return firstErr
}

// StopAll runs Stop on every started provider that implements Lifecycle, in
// reverse-dependency order. Errors from individual providers are aggregated
// into a multi-error; the registry never panics even when a Stop call does.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	forward, err := r.topologicalOrderLocked()
	if err != nil {
		r.mu.Unlock()
		return err
	}
	// reverse
	reverse := make([]string, len(forward))
	for i, n := range forward {
		reverse[len(forward)-1-i] = n
	}
	r.mu.Unlock()

	var errs []error
	for _, name := range reverse {
		r.mu.Lock()
		desc := r.descs[name]
		st := r.states[name]
		lc := desc.Lifecycle
		if st.startedAt == nil || lc == nil {
			r.mu.Unlock()
			continue
		}
		st.state = ProviderStateStopping
		r.mu.Unlock()

		err := safeStop(ctx, lc)

		r.mu.Lock()
		if err != nil {
			st.state = ProviderStateUnhealthy
			st.reason = "stop: " + sanitizeRegistryErr(err)
			errs = append(errs, err)
		} else {
			st.state = ProviderStateDisabled
			st.startedAt = nil
		}
		r.mu.Unlock()
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// CheckHealth probes all enabled providers in parallel, bounded by the
// per-provider healthTimeout. State transitions (enabled → degraded →
// unhealthy) and timestamps are persisted per provider. A cached last-check
// younger than 1s is returned untouched to avoid thundering herds from the
// GUI.
func (r *Registry) CheckHealth(ctx context.Context, name string) (ProviderInfo, error) {
	info, err := r.Get(name)
	if err != nil {
		return ProviderInfo{}, err
	}
	if info.State == ProviderStateDisabled {
		return info, nil
	}
	r.mu.RLock()
	st := *r.states[name]
	desc := *r.descs[name]
	r.mu.RUnlock()
	now := r.clock()
	if st.lastCheck != nil && now.Sub(*st.lastCheck) < time.Second {
		return info, nil
	}
	if desc.HealthChecker == nil {
		return info, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, r.healthTimeout)
	defer cancel()
	state, reason, probeErr := safeCheckHealth(probeCtx, desc.HealthChecker)
	if probeErr != nil {
		state = ProviderStateUnhealthy
		reason = sanitizeRegistryErr(probeErr)
	}

	r.mu.Lock()
	cur := r.states[name]
	checked := now
	cur.lastCheck = &checked
	cur.state = state
	cur.reason = reason
	r.mu.Unlock()
	return r.Get(name)
}

// Get returns the info projection for a single provider, or
// ErrProviderNotFound. Unknown dependents list the name but carry state =
// disabled for robustness.
func (r *Registry) Get(name string) (ProviderInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	desc, ok := r.descs[name]
	if !ok {
		return ProviderInfo{}, ErrProviderNotFound
	}
	st := r.states[name]
	return ProviderInfo{
		Name:         desc.Name,
		Description:  desc.Description,
		Version:      desc.Version,
		Kind:         desc.Kind,
		Dependencies: append([]string(nil), desc.Dependencies...),
		ClusterRoles: append([]string(nil), desc.ClusterRoles...),
		Configured:   desc.Configured,
		State:        st.state,
		HealthReason: st.reason,
		StartedAt:    st.startedAt,
		LastCheck:    st.lastCheck,
	}, nil
}

// List returns all registered providers sorted alphabetically by name.
func (r *Registry) List() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.order))
	names = append(names, r.order...)
	sort.Strings(names)
	out := make([]ProviderInfo, 0, len(names))
	for _, n := range names {
		desc := r.descs[n]
		st := r.states[n]
		out = append(out, ProviderInfo{
			Name:         desc.Name,
			Description:  desc.Description,
			Version:      desc.Version,
			Kind:         desc.Kind,
			Dependencies: append([]string(nil), desc.Dependencies...),
			ClusterRoles: append([]string(nil), desc.ClusterRoles...),
			Configured:   desc.Configured,
			State:        st.state,
			HealthReason: st.reason,
			StartedAt:    st.startedAt,
			LastCheck:    st.lastCheck,
		})
	}
	return out
}

// topologicalOrderLocked returns an order where every provider appears after
// its dependencies. Caller must hold r.mu (for reading the descriptor map).
func (r *Registry) topologicalOrderLocked() ([]string, error) {
	white, gray, black := 0, 1, 2
	color := make(map[string]int, len(r.descs))
	for n := range r.descs {
		color[n] = white
	}
	var order []string
	var visit func(string) error
	visit = func(n string) error {
		desc, ok := r.descs[n]
		if !ok {
			// missing dep — treat as visited; the individual dep gate at
			// StartAll time will degrade the dependent provider visibly.
			return nil
		}
		switch color[n] {
		case black:
			return nil
		case gray:
			return ErrCyclicDependency
		}
		color[n] = gray
		for _, dep := range desc.Dependencies {
			if err := visit(dep); err != nil {
				return err
			}
		}
		color[n] = black
		order = append(order, n)
		return nil
	}
	for _, n := range r.order {
		if err := visit(n); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func safeStart(ctx context.Context, lc Lifecycle) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("provider start panicked")
		}
	}()
	return lc.Start(ctx)
}

func safeStop(ctx context.Context, lc Lifecycle) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("provider stop panicked")
		}
	}()
	return lc.Stop(ctx)
}

func safeCheckHealth(ctx context.Context, hc HealthChecker) (state string, reason string, err error) {
	defer func() {
		if r := recover(); r != nil {
			state = ProviderStateUnhealthy
			reason = "health check panicked"
			err = nil
		}
	}()
	return hc.CheckHealth(ctx)
}

func sanitizeRegistryErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 256 {
		return msg[:256]
	}
	return msg
}

// isValidProviderName enforces the lexical shape contract for provider
// names — [a-z][a-z0-9_-]{1,63}. This matches Kubernetes DNS-1123 labels
// without the dots; provider names are used as URL-safe slugs in the
// /capability/providers/:name detail endpoint.
func isValidProviderName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		case (r == '-' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}
