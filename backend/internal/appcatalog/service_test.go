package appcatalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fake KubernetesSource ---

type fakeKubernetesSource struct {
	mu               sync.Mutex
	namespaces       map[string]bool // "clusterID:namespace" -> exists
	existingReleases map[string]bool // "clusterID:namespace:releaseName" -> exists
	createCalls      []createCall
	createErr        error // if non-nil, CreateResource returns this
	dryRunErr        error // if non-nil, dryRun CreateResource returns this
}

type createCall struct {
	clusterID int64
	path      string
	dryRun    bool
}

func newFakeKubernetes() *fakeKubernetesSource {
	return &fakeKubernetesSource{
		namespaces:       map[string]bool{"1:default": true},
		existingReleases: map[string]bool{},
	}
}

func (f *fakeKubernetesSource) NamespaceExists(_ context.Context, clusterID int64, namespace string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.namespaces[key(clusterID, namespace)], nil
}

func (f *fakeKubernetesSource) ResourceExists(_ context.Context, clusterID int64, path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Check if this is a HelmRelease existence check.
	for k := range f.existingReleases {
		if strings.Contains(path, k) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeKubernetesSource) CreateResource(_ context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls = append(f.createCalls, createCall{clusterID: clusterID, path: path, dryRun: dryRun})
	if dryRun && f.dryRunErr != nil {
		return nil, f.dryRunErr
	}
	if !dryRun && f.createErr != nil {
		return nil, f.createErr
	}
	return body, nil
}

func key(clusterID int64, namespace string) string {
	return strings.Join([]string{itoa(clusterID), namespace}, ":")
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// --- fake DataStore ---

type fakeDataStore struct {
	mu       sync.Mutex
	repos    map[int64]*Repository
	plans    map[string]*Plan
	nextID   int64
	saveErr  error
	claimErr error
}

func newFakeDataStore() *fakeDataStore {
	return &fakeDataStore{
		repos:  map[int64]*Repository{},
		plans:  map[string]*Plan{},
		nextID: 1,
	}
}

func (f *fakeDataStore) SaveRepo(_ context.Context, repo *Repository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveErr != nil {
		return f.saveErr
	}
	if repo.ID == 0 {
		repo.ID = f.nextID
		f.nextID++
	}
	stored := *repo
	f.repos[repo.ID] = &stored
	return nil
}

func (f *fakeDataStore) GetRepo(_ context.Context, id int64) (Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo, ok := f.repos[id]
	if !ok {
		return Repository{}, ErrRepoNotFound
	}
	return *repo, nil
}

func (f *fakeDataStore) GetRepoByName(_ context.Context, name string) (Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, repo := range f.repos {
		if repo.Name == name {
			return *repo, nil
		}
	}
	return Repository{}, ErrRepoNotFound
}

func (f *fakeDataStore) ListRepos(_ context.Context) ([]Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Repository, 0, len(f.repos))
	for _, repo := range f.repos {
		result = append(result, *repo)
	}
	return result, nil
}

func (f *fakeDataStore) DeleteRepo(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.repos[id]; !ok {
		return ErrRepoNotFound
	}
	delete(f.repos, id)
	return nil
}

func (f *fakeDataStore) SavePlan(_ context.Context, plan *Plan) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	stored := *plan
	f.plans[plan.ID] = &stored
	return nil
}

func (f *fakeDataStore) GetPlan(_ context.Context, id string) (Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan, ok := f.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	return *plan, nil
}

func (f *fakeDataStore) ListPlans(_ context.Context, clusterID int64, namespace string) ([]Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Plan, 0)
	for _, plan := range f.plans {
		if plan.TargetClusterID == clusterID {
			if namespace == "" || plan.TargetNamespace == namespace {
				result = append(result, *plan)
			}
		}
	}
	return result, nil
}

func (f *fakeDataStore) ClaimPlan(_ context.Context, id string, tokenHash []byte, idempotencyKey string, now, _ time.Time) (Plan, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan, ok := f.plans[id]
	if !ok {
		return Plan{}, false, ErrPlanNotFound
	}
	if f.claimErr != nil {
		return *plan, false, f.claimErr
	}
	// Simple constant-time compare.
	if len(plan.ConfirmationTokenHash) != len(tokenHash) {
		return *plan, false, ErrConfirmationInvalid
	}
	for i := range tokenHash {
		if plan.ConfirmationTokenHash[i] != tokenHash[i] {
			return *plan, false, ErrConfirmationInvalid
		}
	}
	if plan.Status == StatusExpired {
		return *plan, false, ErrExpired
	}
	if plan.Status == StatusAwaitingConfirmation {
		plan.Status = StatusExecuting
		plan.IdempotencyKey = idempotencyKey
		plan.LockedAt = &now
		return *plan, true, nil
	}
	if plan.Status == StatusExecuting {
		if plan.IdempotencyKey != idempotencyKey {
			return *plan, false, ErrAlreadyExecuted
		}
		return *plan, true, nil
	}
	// Already completed.
	return *plan, false, nil
}

func (f *fakeDataStore) CompletePlan(_ context.Context, id, _ string, executedAt time.Time) (Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan, ok := f.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	plan.Status = StatusSucceeded
	plan.ExecutedAt = &executedAt
	plan.LockedAt = nil
	plan.LastError = ""
	return *plan, nil
}

func (f *fakeDataStore) FailPlan(_ context.Context, id, _, message string) (Plan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	plan, ok := f.plans[id]
	if !ok {
		return Plan{}, ErrPlanNotFound
	}
	plan.Status = StatusFailed
	plan.LastError = message
	plan.LockedAt = nil
	return *plan, nil
}

func (f *fakeDataStore) ExpireStalePlans(_ context.Context, _ time.Time) error {
	return nil
}

// --- fake ChartIndexSource ---

type fakeChartIndexSource struct {
	body []byte
	err  error
}

func (f *fakeChartIndexSource) FetchIndex(_ context.Context, _, _, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

// --- test helpers ---

func newTestService(t *testing.T) (*Service, *fakeKubernetesSource, *fakeDataStore, *fakeChartIndexSource) {
	t.Helper()
	k8s := newFakeKubernetes()
	store := newFakeDataStore()
	idx := &fakeChartIndexSource{}
	svc := &Service{
		kubernetes: k8s,
		repository: store,
		index:      idx,
		planTTL:    15 * time.Minute,
		claimTTL:   2 * time.Minute,
		now:        func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC) },
	}
	return svc, k8s, store, idx
}

const validIndexYAML = `apiVersion: v1
entries:
  nginx:
    - name: nginx
      version: 1.2.3
      appVersion: "1.25"
      description: A test nginx chart
      home: https://example.com
      icon: https://example.com/icon.png
      digest: abc123
      created: "2026-07-01T00:00:00Z"
      maintainers:
        - name: alice
          email: alice@example.com
    - name: nginx
      version: 1.1.0
      appVersion: "1.24"
      description: Older nginx chart
      digest: def456
      created: "2026-06-01T00:00:00Z"
  redis:
    - name: redis
      version: 0.9.0
      appVersion: "7.0"
      description: A redis chart
`

// --- Repository CRUD tests ---

func TestCreateRepository_ValidRequest(t *testing.T) {
	svc, _, store, _ := newTestService(t)
	repo, err := svc.CreateRepository(context.Background(), CreateRepositoryRequest{
		Name:        "test-repo",
		DisplayName: "Test Repo",
		URL:         "https://charts.example.com",
		Username:    "user",
		Password:    "pass",
	}, ActorRef{ID: 1, Name: "admin"})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if repo.ID == 0 {
		t.Error("expected non-zero repo ID")
	}
	if repo.Name != "test-repo" {
		t.Errorf("Name = %q, want %q", repo.Name, "test-repo")
	}
	// Verify credentials are stored.
	if len(repo.CredentialsJSON) == 0 {
		t.Error("expected non-empty CredentialsJSON")
	}
	// Verify the store has the repo.
	stored, _ := store.GetRepo(context.Background(), repo.ID)
	if stored.Name != "test-repo" {
		t.Errorf("stored Name = %q", stored.Name)
	}
}

func TestCreateRepository_InvalidName(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.CreateRepository(context.Background(), CreateRepositoryRequest{
		Name: "INVALID_NAME!",
		URL:  "https://charts.example.com",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestCreateRepository_InvalidURL(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.CreateRepository(context.Background(), CreateRepositoryRequest{
		Name: "test-repo",
		URL:  "not-a-url",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrRepoURLInvalid) {
		t.Fatalf("expected ErrRepoURLInvalid, got %v", err)
	}
}

func TestCreateRepository_InvalidActor(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.CreateRepository(context.Background(), CreateRepositoryRequest{
		Name: "test-repo",
		URL:  "https://charts.example.com",
	}, ActorRef{ID: 0, Name: ""})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestDeleteRepository_NotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	err := svc.DeleteRepository(context.Background(), 999)
	if !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("expected ErrRepoNotFound, got %v", err)
	}
}

func TestDeleteRepository_Success(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	repo, _ := svc.CreateRepository(context.Background(), CreateRepositoryRequest{
		Name: "test-repo",
		URL:  "https://charts.example.com",
	}, ActorRef{ID: 1, Name: "admin"})
	err := svc.DeleteRepository(context.Background(), repo.ID)
	if err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
}

func TestRepositoryViewFrom_RedactsCredentials(t *testing.T) {
	repo := Repository{
		ID:              1,
		Name:            "test",
		URL:             "https://example.com",
		CredentialsJSON: JSON(`{"username":"user","password":"secret"}`),
	}
	view := RepositoryViewFrom(repo)
	if view.HasAuth != true {
		t.Error("expected HasAuth=true")
	}
	// RepositoryView has no CredentialsJSON field, so it's structurally redacted.
}

func TestHasCredentials(t *testing.T) {
	tests := []struct {
		name  string
		creds JSON
		want  bool
	}{
		{"empty", JSON(""), false},
		{"null", JSON("null"), false},
		{"empty object", JSON("{}"), false},
		{"with username", JSON(`{"username":"user"}`), true},
		{"with password", JSON(`{"password":"pass"}`), true},
		{"both empty", JSON(`{"username":"","password":""}`), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCredentials(tt.creds); got != tt.want {
				t.Errorf("hasCredentials(%s) = %v, want %v", tt.creds, got, tt.want)
			}
		})
	}
}

// --- Chart listing / detail tests ---

func TestListCharts_Success(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test", URL: "https://example.com"}

	charts, err := svc.ListCharts(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListCharts: %v", err)
	}
	if len(charts) != 2 {
		t.Fatalf("expected 2 charts, got %d", len(charts))
	}
	// Find nginx chart and verify latest version.
	var nginx *ChartSummary
	for i := range charts {
		if charts[i].Name == "nginx" {
			nginx = &charts[i]
		}
	}
	if nginx == nil {
		t.Fatal("nginx chart not found")
	}
	if nginx.Version != "1.2.3" {
		t.Errorf("nginx version = %q, want %q", nginx.Version, "1.2.3")
	}
}

func TestListCharts_RepoNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.ListCharts(context.Background(), 999)
	if !errors.Is(err, ErrRepoNotFound) {
		t.Fatalf("expected ErrRepoNotFound, got %v", err)
	}
}

func TestListCharts_RepoUnreachable(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.err = ErrRepoUnreachable
	store.repos[1] = &Repository{ID: 1, Name: "test", URL: "https://example.com"}
	_, err := svc.ListCharts(context.Background(), 1)
	if !errors.Is(err, ErrRepoUnreachable) {
		t.Fatalf("expected ErrRepoUnreachable, got %v", err)
	}
}

func TestGetChart_Success(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test", URL: "https://example.com"}

	detail, err := svc.GetChart(context.Background(), 1, "nginx")
	if err != nil {
		t.Fatalf("GetChart: %v", err)
	}
	if detail.Name != "nginx" {
		t.Errorf("Name = %q", detail.Name)
	}
	if len(detail.Versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(detail.Versions))
	}
	if detail.Versions[0].Version != "1.2.3" {
		t.Errorf("first version = %q, want %q", detail.Versions[0].Version, "1.2.3")
	}
}

func TestGetChart_NotFound(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test", URL: "https://example.com"}

	_, err := svc.GetChart(context.Background(), 1, "nonexistent")
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("expected ErrChartNotFound, got %v", err)
	}
}

func TestGetChart_EmptyName(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.GetChart(context.Background(), 1, "")
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("expected ErrChartNotFound, got %v", err)
	}
}

// --- Deploy preview tests ---

func TestPreview_Success(t *testing.T) {
	svc, k8s, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	plan, err := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if plan.ID == "" {
		t.Error("expected non-empty plan ID")
	}
	if plan.Status != StatusAwaitingConfirmation {
		t.Errorf("Status = %q, want %q", plan.Status, StatusAwaitingConfirmation)
	}
	if plan.ConfirmationToken == "" {
		t.Error("expected non-empty confirmation token")
	}
	if len(plan.ReleaseManifest) == 0 {
		t.Error("expected non-empty release manifest")
	}
	// Verify dry-run was called.
	k8s.mu.Lock()
	defer k8s.mu.Unlock()
	if len(k8s.createCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(k8s.createCalls))
	}
	if !k8s.createCalls[0].dryRun {
		t.Error("expected dry-run=true")
	}
}

func TestPreview_NamespaceMissing(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	_, err := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "nonexistent",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrNamespaceMissing) {
		t.Fatalf("expected ErrNamespaceMissing, got %v", err)
	}
}

func TestPreview_ChartNotFound(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	_, err := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nonexistent",
		ChartVersion:    "1.0.0",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-release",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("expected ErrChartNotFound, got %v", err)
	}
}

func TestPreview_InvalidRequest(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	tests := []struct {
		name    string
		request DeployPreviewRequest
	}{
		{"empty chart name", DeployPreviewRequest{RepoID: 1, ChartName: "", ChartVersion: "1.0.0", TargetClusterID: 1, TargetNamespace: "default", ReleaseName: "release"}},
		{"invalid namespace", DeployPreviewRequest{RepoID: 1, ChartName: "nginx", ChartVersion: "1.0.0", TargetClusterID: 1, TargetNamespace: "INVALID!", ReleaseName: "release"}},
		{"invalid release name", DeployPreviewRequest{RepoID: 1, ChartName: "nginx", ChartVersion: "1.0.0", TargetClusterID: 1, TargetNamespace: "default", ReleaseName: "INVALID!"}},
		{"zero cluster ID", DeployPreviewRequest{RepoID: 1, ChartName: "nginx", ChartVersion: "1.0.0", TargetClusterID: 0, TargetNamespace: "default", ReleaseName: "release"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Preview(context.Background(), tt.request, ActorRef{ID: 1, Name: "admin"})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("expected ErrInvalidRequest, got %v", err)
			}
		})
	}
}

func TestPreview_DryRunFails(t *testing.T) {
	svc, k8s, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}
	k8s.dryRunErr = errors.New("admission denied")

	_, err := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})
	if !errors.Is(err, ErrPreviewFailed) {
		t.Fatalf("expected ErrPreviewFailed, got %v", err)
	}
}

// --- Execute tests ---

func TestExecute_Success(t *testing.T) {
	svc, k8s, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	plan, _ := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})

	executed, err := svc.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "test-idempotency-key-1234")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if executed.Status != StatusSucceeded {
		t.Errorf("Status = %q, want %q", executed.Status, StatusSucceeded)
	}
	if executed.ExecutedAt == nil {
		t.Error("expected non-nil ExecutedAt")
	}
	// Verify actual create (non-dry-run) was called.
	k8s.mu.Lock()
	defer k8s.mu.Unlock()
	hasNonDryRun := false
	for _, call := range k8s.createCalls {
		if !call.dryRun {
			hasNonDryRun = true
		}
	}
	if !hasNonDryRun {
		t.Error("expected at least one non-dry-run create call")
	}
}

func TestExecute_InvalidToken(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	plan, _ := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})

	_, err := svc.Execute(context.Background(), plan.ID, "wrong-token", "test-idempotency-key-1234")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("expected ErrConfirmationInvalid, got %v", err)
	}
}

func TestExecute_InvalidIdempotencyKey(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	plan, _ := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})

	_, err := svc.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "short")
	if !errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("expected ErrInvalidIdempotency, got %v", err)
	}
}

func TestExecute_PlanNotFound(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.Execute(context.Background(), "nonexistent-id", "token", "test-idempotency-key-1234")
	if !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("expected ErrPlanNotFound, got %v", err)
	}
}

func TestExecute_IdempotentReplay(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	plan, _ := svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})

	// First execution.
	_, err := svc.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "test-idempotency-key-1234")
	if err != nil {
		t.Fatalf("first Execute: %v", err)
	}

	// Second execution with same idempotency key should succeed without re-applying.
	_, err = svc.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "test-idempotency-key-1234")
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
}

// --- ListPlans / GetPlan tests ---

func TestListPlans_Success(t *testing.T) {
	svc, _, store, idx := newTestService(t)
	idx.body = []byte(validIndexYAML)
	store.repos[1] = &Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}

	_, _ = svc.Preview(context.Background(), DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetClusterID: 1,
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
	}, ActorRef{ID: 1, Name: "admin"})

	plans, err := svc.ListPlans(context.Background(), 1, "default")
	if err != nil {
		t.Fatalf("ListPlans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
}

func TestListPlans_InvalidClusterID(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	_, err := svc.ListPlans(context.Background(), 0, "")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

// --- HelmRelease manifest builder tests ---

func TestBuildHelmReleaseManifest(t *testing.T) {
	repo := Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}
	entry := helmIndexEntry{
		Name:       "nginx",
		Version:    "1.2.3",
		AppVersion: "1.25",
	}
	request := DeployPreviewRequest{
		RepoID:          1,
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
		ValuesYAML:      "replicaCount: 3",
	}
	manifest, meta, err := buildHelmReleaseManifest(request, repo, entry)
	if err != nil {
		t.Fatalf("buildHelmReleaseManifest: %v", err)
	}
	manifestStr := string(manifest)
	if !strings.Contains(manifestStr, `"kind":"HelmRelease"`) {
		t.Errorf("manifest missing kind: %s", manifestStr)
	}
	if !strings.Contains(manifestStr, `"name":"my-nginx"`) {
		t.Errorf("manifest missing release name: %s", manifestStr)
	}
	if !strings.Contains(manifestStr, `"chart":"nginx"`) {
		t.Errorf("manifest missing chart name: %s", manifestStr)
	}
	metaStr := string(meta)
	if !strings.Contains(metaStr, `"version":"1.2.3"`) {
		t.Errorf("metadata missing version: %s", metaStr)
	}
}

func TestBuildHelmReleaseManifest_InvalidYAML(t *testing.T) {
	repo := Repository{ID: 1, Name: "test-repo", URL: "https://example.com"}
	entry := helmIndexEntry{Name: "nginx", Version: "1.2.3"}
	request := DeployPreviewRequest{
		ChartName:       "nginx",
		ChartVersion:    "1.2.3",
		TargetNamespace: "default",
		ReleaseName:     "my-nginx",
		ValuesYAML:      "{{invalid yaml",
	}
	_, _, err := buildHelmReleaseManifest(request, repo, entry)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

// --- HelmRelease path helper tests ---

func TestHelmReleasePath(t *testing.T) {
	path := helmReleasePath("default", "my-release")
	expected := "/apis/helm.toolkit.fluxcd.io/v2beta1/namespaces/default/helmreleases/my-release"
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func TestHelmReleaseCreatePath(t *testing.T) {
	path := helmReleaseCreatePath("production")
	expected := "/apis/helm.toolkit.fluxcd.io/v2beta1/namespaces/production/helmreleases"
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

// --- ValidateRepoRequest tests ---

func TestValidateRepoRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateRepositoryRequest
		wantErr error
	}{
		{"valid", CreateRepositoryRequest{Name: "test", URL: "https://example.com"}, nil},
		{"empty name", CreateRepositoryRequest{Name: "", URL: "https://example.com"}, ErrInvalidRequest},
		{"invalid name", CreateRepositoryRequest{Name: "TEST", URL: "https://example.com"}, ErrInvalidRequest},
		{"empty url", CreateRepositoryRequest{Name: "test", URL: ""}, ErrRepoURLInvalid},
		{"ftp url", CreateRepositoryRequest{Name: "test", URL: "ftp://example.com"}, ErrRepoURLInvalid},
		{"too long display name", CreateRepositoryRequest{Name: "test", URL: "https://example.com", DisplayName: strings.Repeat("x", 129)}, ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepoRequest(tt.req)
			if tt.wantErr == nil && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// --- findChartEntry tests ---

func TestFindChartEntry(t *testing.T) {
	index := helmIndex{
		Entries: map[string][]helmIndexEntry{
			"nginx": {
				{Version: "1.2.3"},
				{Version: "1.1.0"},
			},
		},
	}
	entry, err := findChartEntry(index, "nginx", "1.1.0")
	if err != nil {
		t.Fatalf("findChartEntry: %v", err)
	}
	if entry.Version != "1.1.0" {
		t.Errorf("version = %q, want %q", entry.Version, "1.1.0")
	}

	_, err = findChartEntry(index, "nginx", "9.9.9")
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("expected ErrChartNotFound, got %v", err)
	}

	_, err = findChartEntry(index, "nonexistent", "1.0.0")
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("expected ErrChartNotFound, got %v", err)
	}
}
