package federation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
)

// fakeRepository is an in-memory Repository for service-level tests. It is
// safe for sequential use within a single test; concurrent access is guarded
// by a mutex so it does not race when ResourceSummary fans out reads.
type fakeRepository struct {
	mu            sync.Mutex
	clusters      map[int64]cluster.Cluster
	events        []FederationEvent
	eventSeq      int64
	getClusterErr error
	listErr       error
	appendErr     error
	countHostErr  error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{clusters: make(map[int64]cluster.Cluster)}
}

func (r *fakeRepository) ListClusters(context.Context) ([]cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := make([]cluster.Cluster, 0, len(r.clusters))
	for _, c := range r.clusters {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeRepository) GetCluster(_ context.Context, id int64) (cluster.Cluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getClusterErr != nil {
		return cluster.Cluster{}, r.getClusterErr
	}
	item, ok := r.clusters[id]
	if !ok {
		return cluster.Cluster{}, ErrClusterNotFound
	}
	return item, nil
}

func (r *fakeRepository) SetClusterRole(_ context.Context, id int64, role string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.clusters[id]
	if !ok {
		return ErrClusterNotFound
	}
	item.ClusterRole = role
	item.UpdatedAt = now
	r.clusters[id] = item
	return nil
}

func (r *fakeRepository) SetFederationStatus(_ context.Context, id int64, status string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.clusters[id]
	if !ok {
		return ErrClusterNotFound
	}
	item.FederationStatus = status
	item.UpdatedAt = now
	r.clusters[id] = item
	return nil
}

func (r *fakeRepository) TouchHeartbeat(_ context.Context, id int64, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.clusters[id]
	if !ok {
		return ErrClusterNotFound
	}
	hb := now
	item.LastHeartbeatAt = &hb
	item.UpdatedAt = now
	r.clusters[id] = item
	return nil
}

func (r *fakeRepository) CountHost(context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.countHostErr != nil {
		return 0, r.countHostErr
	}
	var count int64
	for _, c := range r.clusters {
		if c.ClusterRole == cluster.ClusterRoleHost {
			count++
		}
	}
	return count, nil
}

func (r *fakeRepository) AppendEvent(_ context.Context, event FederationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.appendErr != nil {
		return r.appendErr
	}
	r.eventSeq++
	event.ID = r.eventSeq
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	r.events = append(r.events, event)
	return nil
}

func (r *fakeRepository) ListEvents(_ context.Context, limit int) ([]FederationEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > MaxEventsLimit {
		limit = DefaultEventsLimit
	}
	out := make([]FederationEvent, 0)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, r.events[i])
	}
	return out, nil
}

func (r *fakeRepository) ListEventsByCluster(_ context.Context, clusterID int64, limit int) ([]FederationEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit <= 0 || limit > MaxEventsLimit {
		limit = DefaultEventsLimit
	}
	out := make([]FederationEvent, 0)
	for i := len(r.events) - 1; i >= 0 && len(out) < limit; i-- {
		if r.events[i].ClusterID == clusterID {
			out = append(out, r.events[i])
		}
	}
	return out, nil
}

// fakeLister is an in-memory ClusterLister for ResourceSummary tests.
type fakeLister struct {
	mu     sync.Mutex
	counts map[int64]int
	errs   map[int64]error
	delay  time.Duration
}

func newFakeLister() *fakeLister {
	return &fakeLister{counts: make(map[int64]int), errs: make(map[int64]error)}
}

func (l *fakeLister) ListResource(ctx context.Context, clusterID int64, _ GVR, _ string) (CountResult, error) {
	if l.delay > 0 {
		select {
		case <-time.After(l.delay):
		case <-ctx.Done():
			return CountResult{Err: ctx.Err()}, ctx.Err()
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err, ok := l.errs[clusterID]; ok {
		return CountResult{Err: err}, err
	}
	return CountResult{Total: l.counts[clusterID]}, nil
}

// newServiceWithClock constructs a Service with a deterministic clock. The
// returned time is fixed at t0 so assertions on RegisteredAt/LastHeartbeatAt
// are stable.
func newServiceWithClock(repo Repository, lister ClusterLister, t0 time.Time) *Service {
	svc := NewService(repo, lister)
	svc.now = func() time.Time { return t0 }
	return svc
}

// seedCluster inserts a cluster row into the fake repository and returns it.
func seedCluster(repo *fakeRepository, id int64, name string, role, status string) cluster.Cluster {
	c := cluster.Cluster{
		ID:               id,
		Name:             name,
		APIServer:        "https://" + name + ".example",
		Enabled:          true,
		Status:           cluster.StatusReady,
		ClusterRole:      role,
		FederationStatus: status,
	}
	repo.clusters[id] = c
	return c
}

// ============================================================================
// RegisterCluster
// ============================================================================

func TestRegisterClusterMemberSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(repo, nil, t0)

	out, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 1,
		Role:      cluster.ClusterRoleMember,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleMember {
		t.Fatalf("role = %q, want member", out.ClusterRole)
	}
	if out.LastHeartbeatAt == nil || !out.LastHeartbeatAt.Equal(t0) {
		t.Fatalf("heartbeat = %v, want %v", out.LastHeartbeatAt, t0)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].EventType != EventRegistered {
		t.Fatalf("events = %+v, want one registered event", events)
	}
}

func TestRegisterClusterHostSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := newServiceWithClock(repo, nil, time.Now())

	out, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 1,
		Role:      cluster.ClusterRoleHost,
		Status:    cluster.FederationStatusHealthy,
	})
	if err != nil {
		t.Fatalf("register host: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleHost {
		t.Fatalf("role = %q, want host", out.ClusterRole)
	}
	if out.FederationStatus != cluster.FederationStatusHealthy {
		t.Fatalf("status = %q, want healthy", out.FederationStatus)
	}
}

func TestRegisterClusterAlreadyRegisteredRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 1,
		Role:      cluster.ClusterRoleMember,
	})
	if !errors.Is(err, ErrClusterAlreadyRegistered) {
		t.Fatalf("error = %v, want ErrClusterAlreadyRegistered", err)
	}
}

func TestRegisterClusterSecondHostRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := NewService(repo, nil)

	_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 2,
		Role:      cluster.ClusterRoleHost,
	})
	if !errors.Is(err, ErrHostAlreadyExists) {
		t.Fatalf("error = %v, want ErrHostAlreadyExists", err)
	}
}

func TestRegisterClusterInvalidRoleRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := NewService(repo, nil)

	cases := []string{"", "standalone", "agent", "HOST"}
	for _, role := range cases {
		_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
			ClusterID: 1,
			Role:      role,
		})
		if !errors.Is(err, ErrInvalidClusterRole) {
			t.Fatalf("role %q: error = %v, want ErrInvalidClusterRole", role, err)
		}
	}
}

func TestRegisterClusterInvalidStatusRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := NewService(repo, nil)

	_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 1,
		Role:      cluster.ClusterRoleMember,
		Status:    "unknown",
	})
	if !errors.Is(err, ErrInvalidFederationStatus) {
		t.Fatalf("error = %v, want ErrInvalidFederationStatus", err)
	}
}

func TestRegisterClusterNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 999,
		Role:      cluster.ClusterRoleMember,
	})
	if !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("error = %v, want ErrClusterNotFound", err)
	}
}

func TestRegisterClusterZeroIDRejected(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	_, err := svc.RegisterCluster(context.Background(), RegisterClusterInput{
		ClusterID: 0,
		Role:      cluster.ClusterRoleMember,
	})
	if err == nil {
		t.Fatal("expected error for zero cluster_id")
	}
}

// ============================================================================
// DeregisterCluster
// ============================================================================

func TestDeregisterMemberSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(repo, nil, t0)

	out, err := svc.DeregisterCluster(context.Background(), 1)
	if err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleStandalone {
		t.Fatalf("role = %q, want standalone", out.ClusterRole)
	}
	if out.FederationStatus != cluster.FederationStatusRegistered {
		t.Fatalf("status = %q, want registered", out.FederationStatus)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].EventType != EventDeregistered {
		t.Fatalf("events = %+v, want one deregistered event", events)
	}
}

func TestDeregisterHostRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.DeregisterCluster(context.Background(), 1)
	if !errors.Is(err, ErrCannotDeregisterHost) {
		t.Fatalf("error = %v, want ErrCannotDeregisterHost", err)
	}
}

func TestDeregisterStandaloneIsIdempotent(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := NewService(repo, nil)

	out, err := svc.DeregisterCluster(context.Background(), 1)
	if err != nil {
		t.Fatalf("deregister standalone: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleStandalone {
		t.Fatalf("role = %q, want standalone", out.ClusterRole)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0 (idempotent no-op)", len(events))
	}
}

func TestDeregisterClusterNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	_, err := svc.DeregisterCluster(context.Background(), 999)
	if !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("error = %v, want ErrClusterNotFound", err)
	}
}

// ============================================================================
// PromoteToHost
// ============================================================================

func TestPromoteToHostSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.PromoteToHost(context.Background(), 1)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleHost {
		t.Fatalf("role = %q, want host", out.ClusterRole)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].EventType != EventRoleChange {
		t.Fatalf("events = %+v, want one role_change event", events)
	}
}

func TestPromoteToHostIdempotentWhenAlreadyHost(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.PromoteToHost(context.Background(), 1)
	if err != nil {
		t.Fatalf("promote idempotent: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleHost {
		t.Fatalf("role = %q, want host", out.ClusterRole)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0 (idempotent)", len(events))
	}
}

func TestPromoteToHostRejectedWhenHostExists(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.PromoteToHost(context.Background(), 2)
	if !errors.Is(err, ErrHostAlreadyExists) {
		t.Fatalf("error = %v, want ErrHostAlreadyExists", err)
	}
}

// ============================================================================
// DemoteHost
// ============================================================================

func TestDemoteHostToMemberSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.DemoteHost(context.Background(), 1, cluster.ClusterRoleMember)
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleMember {
		t.Fatalf("role = %q, want member", out.ClusterRole)
	}
}

func TestDemoteHostToStandaloneSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.DemoteHost(context.Background(), 1, cluster.ClusterRoleStandalone)
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if out.ClusterRole != cluster.ClusterRoleStandalone {
		t.Fatalf("role = %q, want standalone", out.ClusterRole)
	}
}

func TestDemoteHostInvalidTargetRoleRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	cases := []string{"", "host", "agent"}
	for _, role := range cases {
		_, err := svc.DemoteHost(context.Background(), 1, role)
		if !errors.Is(err, ErrInvalidClusterRole) {
			t.Fatalf("target %q: error = %v, want ErrInvalidClusterRole", role, err)
		}
	}
}

func TestDemoteHostRejectedWhenNotHost(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.DemoteHost(context.Background(), 1, cluster.ClusterRoleMember)
	if err == nil {
		t.Fatal("expected error when demoting non-host")
	}
}

// ============================================================================
// RecordHeartbeat
// ============================================================================

func TestRecordHeartbeatWithStatusUpdatesFederationStatus(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusRegistered)
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(repo, nil, t0)

	out, err := svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusHealthy)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if out.FederationStatus != cluster.FederationStatusHealthy {
		t.Fatalf("status = %q, want healthy", out.FederationStatus)
	}
	if out.LastHeartbeatAt == nil || !out.LastHeartbeatAt.Equal(t0) {
		t.Fatalf("heartbeat = %v, want %v", out.LastHeartbeatAt, t0)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].EventType != EventHeartbeat {
		t.Fatalf("events = %+v, want one heartbeat event", events)
	}
}

func TestRecordHeartbeatWithoutStatusKeepsFederationStatus(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusDegraded)
	svc := NewService(repo, nil)

	out, err := svc.RecordHeartbeat(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if out.FederationStatus != cluster.FederationStatusDegraded {
		t.Fatalf("status = %q, want degraded (unchanged)", out.FederationStatus)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].Status != cluster.FederationStatusHealthy {
		t.Fatalf("event status = %q, want healthy (default)", events[0].Status)
	}
}

func TestRecordHeartbeatInvalidStatusRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.RecordHeartbeat(context.Background(), 1, "unknown")
	if !errors.Is(err, ErrInvalidFederationStatus) {
		t.Fatalf("error = %v, want ErrInvalidFederationStatus", err)
	}
}

func TestRecordHeartbeatClusterNotFound(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	_, err := svc.RecordHeartbeat(context.Background(), 999, cluster.FederationStatusHealthy)
	if !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("error = %v, want ErrClusterNotFound", err)
	}
}

// ============================================================================
// UpdateFederationStatus
// ============================================================================

func TestUpdateFederationStatusSuccess(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.UpdateFederationStatus(context.Background(), 1, cluster.FederationStatusDegraded, "probe timeout")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if out.FederationStatus != cluster.FederationStatusDegraded {
		t.Fatalf("status = %q, want degraded", out.FederationStatus)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 || events[0].EventType != EventStatusChange {
		t.Fatalf("events = %+v, want one status_change event", events)
	}
	if events[0].Message != "probe timeout" {
		t.Fatalf("message = %q, want 'probe timeout'", events[0].Message)
	}
}

func TestUpdateFederationStatusIdempotentWhenUnchanged(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	out, err := svc.UpdateFederationStatus(context.Background(), 1, cluster.FederationStatusHealthy, "")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if out.FederationStatus != cluster.FederationStatusHealthy {
		t.Fatalf("status = %q, want healthy", out.FederationStatus)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0 (idempotent)", len(events))
	}
}

func TestUpdateFederationStatusInvalidRejected(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.UpdateFederationStatus(context.Background(), 1, "unknown", "")
	if !errors.Is(err, ErrInvalidFederationStatus) {
		t.Fatalf("error = %v, want ErrInvalidFederationStatus", err)
	}
}

func TestUpdateFederationStatusDefaultMessage(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, err := svc.UpdateFederationStatus(context.Background(), 1, cluster.FederationStatusDegraded, "")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	events, _ := repo.ListEvents(context.Background(), 10)
	if len(events) != 1 {
		t.Fatalf("events len = %d, want 1", len(events))
	}
	if events[0].Message == "" {
		t.Fatal("default message should be populated")
	}
}

// ============================================================================
// Overview
// ============================================================================

func TestOverviewClassifiesHostMembersStandalone(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "host", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "m1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 3, "m2", cluster.ClusterRoleMember, cluster.FederationStatusDegraded)
	seedCluster(repo, 4, "s1", cluster.ClusterRoleStandalone, cluster.FederationStatusRegistered)
	svc := NewService(repo, nil)

	overview, err := svc.Overview(context.Background(), nil)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Host == nil || overview.Host.ClusterID != 1 {
		t.Fatalf("host = %+v, want cluster 1", overview.Host)
	}
	if len(overview.Members) != 2 {
		t.Fatalf("members len = %d, want 2", len(overview.Members))
	}
	if overview.Members[0].ClusterID != 2 || overview.Members[1].ClusterID != 3 {
		t.Fatalf("members order = %+v, want 2 then 3", overview.Members)
	}
	if len(overview.Standalone) != 1 {
		t.Fatalf("standalone len = %d, want 1", len(overview.Standalone))
	}
	if overview.TotalClusters != 4 {
		t.Fatalf("total = %d, want 4", overview.TotalClusters)
	}
	if overview.HealthyCount != 2 || overview.DegradedCount != 1 || overview.RegisteredCount != 1 {
		t.Fatalf("counts = healthy=%d degraded=%d registered=%d, want 2/1/1", overview.HealthyCount, overview.DegradedCount, overview.RegisteredCount)
	}
}

func TestOverviewRespectsVisibleClusterIDs(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "host", cluster.ClusterRoleHost, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "m1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 3, "m2", cluster.ClusterRoleMember, cluster.FederationStatusDegraded)
	svc := NewService(repo, nil)

	// User is authorized to see only cluster 2.
	overview, err := svc.Overview(context.Background(), []int64{2})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Host != nil {
		t.Fatalf("host = %+v, want nil (hidden)", overview.Host)
	}
	if len(overview.Members) != 1 || overview.Members[0].ClusterID != 2 {
		t.Fatalf("members = %+v, want only cluster 2", overview.Members)
	}
	if overview.TotalClusters != 1 {
		t.Fatalf("total = %d, want 1", overview.TotalClusters)
	}
}

func TestOverviewNilVisibleReturnsAll(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	overview, err := svc.Overview(context.Background(), nil)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.TotalClusters != 2 {
		t.Fatalf("total = %d, want 2", overview.TotalClusters)
	}
}

func TestOverviewNeverReturnsNilSlices(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	overview, err := svc.Overview(context.Background(), nil)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.Members == nil {
		t.Fatal("members should be non-nil empty slice")
	}
	if overview.Standalone == nil {
		t.Fatal("standalone should be non-nil empty slice")
	}
}

// ============================================================================
// ListEvents / ListEventsByCluster
// ============================================================================

func TestListEventsNewestFirst(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc := newServiceWithClock(repo, nil, t0)

	_, _ = svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusHealthy)
	// Advance clock between events.
	svc.now = func() time.Time { return t0.Add(time.Minute) }
	_, _ = svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusDegraded)

	events, err := svc.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if !events[0].OccurredAt.After(events[1].OccurredAt) {
		t.Fatal("events not newest-first")
	}
}

func TestListEventsLimitClampedToMax(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	// Request limit above MaxEventsLimit; service should clamp to Default.
	_, err := svc.ListEvents(context.Background(), MaxEventsLimit+100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
}

func TestListEventsByClusterFiltersByCluster(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	svc := NewService(repo, nil)

	_, _ = svc.RecordHeartbeat(context.Background(), 1, cluster.FederationStatusHealthy)
	_, _ = svc.RecordHeartbeat(context.Background(), 2, cluster.FederationStatusHealthy)

	c1Events, err := svc.ListEventsByCluster(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("list by cluster: %v", err)
	}
	if len(c1Events) != 1 || c1Events[0].ClusterID != 1 {
		t.Fatalf("c1 events = %+v, want only cluster 1", c1Events)
	}
}

func TestListEventsByClusterZeroIDRejected(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	_, err := svc.ListEventsByCluster(context.Background(), 0, 10)
	if err == nil {
		t.Fatal("expected error for zero cluster_id")
	}
}

// ============================================================================
// ResourceSummary
// ============================================================================

func TestResourceSummaryNilListerReturnsEmpty(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil)

	summary, err := svc.ResourceSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	if len(summary.Items) != 0 {
		t.Fatalf("items len = %d, want 0 when lister is nil", len(summary.Items))
	}
}

func TestResourceSummaryAggregatesCountsAcrossClusters(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	lister := newFakeLister()
	lister.counts[1] = 10
	lister.counts[2] = 20
	svc := NewService(repo, lister)

	summary, err := svc.ResourceSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	if len(summary.Items) != len(FixedGVRWhitelist) {
		t.Fatalf("items len = %d, want %d", len(summary.Items), len(FixedGVRWhitelist))
	}
	// Each entry should have 2 cluster counts; total for the first entry should be 30.
	first := summary.Items[0]
	if len(first.ByCluster) != 2 {
		t.Fatalf("by_cluster len = %d, want 2", len(first.ByCluster))
	}
	if first.TotalCount != 30 {
		t.Fatalf("total = %d, want 30", first.TotalCount)
	}
}

func TestResourceSummaryRespectsVisibleClusterIDs(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	lister := newFakeLister()
	lister.counts[1] = 10
	lister.counts[2] = 20
	svc := NewService(repo, lister)

	summary, err := svc.ResourceSummary(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	first := summary.Items[0]
	if len(first.ByCluster) != 1 {
		t.Fatalf("by_cluster len = %d, want 1 (visible only)", len(first.ByCluster))
	}
	if first.ByCluster[0].ClusterID != 1 {
		t.Fatalf("cluster id = %d, want 1", first.ByCluster[0].ClusterID)
	}
	if first.TotalCount != 10 {
		t.Fatalf("total = %d, want 10", first.TotalCount)
	}
}

func TestResourceSummarySkipsDisabledClusters(t *testing.T) {
	repo := newFakeRepository()
	c1 := seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	c1.Enabled = false
	repo.clusters[1] = c1
	seedCluster(repo, 2, "c2", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	lister := newFakeLister()
	lister.counts[2] = 5
	svc := NewService(repo, lister)

	summary, err := svc.ResourceSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	first := summary.Items[0]
	if len(first.ByCluster) != 1 {
		t.Fatalf("by_cluster len = %d, want 1 (disabled cluster skipped)", len(first.ByCluster))
	}
	if first.ByCluster[0].ClusterID != 2 {
		t.Fatalf("cluster id = %d, want 2", first.ByCluster[0].ClusterID)
	}
}

func TestResourceSummaryRecordsErrorOnListerFailure(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	lister := newFakeLister()
	lister.errs[1] = errors.New("kubernetes api unavailable")
	svc := NewService(repo, lister)

	summary, err := svc.ResourceSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	first := summary.Items[0]
	if len(first.ByCluster) != 1 {
		t.Fatalf("by_cluster len = %d, want 1", len(first.ByCluster))
	}
	if first.ByCluster[0].Error != "QUERY_FAILED" {
		t.Fatalf("error = %q, want QUERY_FAILED", first.ByCluster[0].Error)
	}
	if first.TotalCount != 0 {
		t.Fatalf("total = %d, want 0 (failed cluster contributes 0)", first.TotalCount)
	}
}

func TestResourceSummaryTimeoutProducesTimeoutCode(t *testing.T) {
	repo := newFakeRepository()
	seedCluster(repo, 1, "c1", cluster.ClusterRoleMember, cluster.FederationStatusHealthy)
	// The lister simulates a per-cluster call whose context deadline has
	// already expired. This exercises the countOne error-mapping path
	// (errors.Is(err, context.DeadlineExceeded) → "TIMEOUT") without waiting
	// for the real ResourceSummaryTimeout on each of the 9 GVRs.
	lister := &timeoutLister{}
	svc := NewService(repo, lister)

	summary, err := svc.ResourceSummary(context.Background(), nil)
	if err != nil {
		t.Fatalf("resource summary: %v", err)
	}
	first := summary.Items[0]
	if len(first.ByCluster) != 1 {
		t.Fatalf("by_cluster len = %d, want 1", len(first.ByCluster))
	}
	if first.ByCluster[0].Error != "TIMEOUT" {
		t.Fatalf("error = %q, want TIMEOUT", first.ByCluster[0].Error)
	}
}

// timeoutLister is a ClusterLister that always returns context.DeadlineExceeded,
// simulating a per-cluster call that exceeded its timeout.
type timeoutLister struct{}

func (timeoutLister) ListResource(context.Context, int64, GVR, string) (CountResult, error) {
	return CountResult{Err: context.DeadlineExceeded}, context.DeadlineExceeded
}

// ============================================================================
// Helpers
// ============================================================================

func TestIsValidEvent(t *testing.T) {
	valid := []string{EventRegistered, EventDeregistered, EventHeartbeat, EventStatusChange, EventRoleChange}
	for _, e := range valid {
		if !IsValidEvent(e) {
			t.Fatalf("%q should be valid", e)
		}
	}
	invalid := []string{"", "unknown", "REGISTERED"}
	for _, e := range invalid {
		if IsValidEvent(e) {
			t.Fatalf("%q should be invalid", e)
		}
	}
}

func TestIsValidFederationStatus(t *testing.T) {
	valid := []string{
		cluster.FederationStatusRegistered,
		cluster.FederationStatusHealthy,
		cluster.FederationStatusDegraded,
		cluster.FederationStatusDisconnected,
	}
	for _, s := range valid {
		if !isValidFederationStatus(s) {
			t.Fatalf("%q should be valid", s)
		}
	}
	if isValidFederationStatus("unknown") {
		t.Fatal("unknown should be invalid")
	}
}

func TestToSummaryProjectsAllFields(t *testing.T) {
	now := time.Now().UTC()
	probe := now
	c := cluster.Cluster{
		ID:                7,
		Name:              "c7",
		APIServer:         "https://c7.example",
		Enabled:           true,
		Status:            cluster.StatusReady,
		KubernetesVersion: "1.30.0",
		LastProbedAt:      &probe,
		ClusterRole:       cluster.ClusterRoleHost,
		FederationStatus:  cluster.FederationStatusHealthy,
		RegisteredAt:      &probe,
		LastHeartbeatAt:   &probe,
	}
	summary := toSummary(c)
	if summary.ClusterID != c.ID || summary.ClusterName != c.Name || summary.APIServer != c.APIServer {
		t.Fatalf("summary = %+v, mismatch", summary)
	}
	if summary.ClusterRole != c.ClusterRole || summary.FederationStatus != c.FederationStatus {
		t.Fatalf("federation fields mismatch: %+v", summary)
	}
}
