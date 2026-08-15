package copyops_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/copyops"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// inmemRepo is a thread-safe copyops.Repository stub backed by a plain map.
// Used for service-level tests that do not need Gorm/SQLite.
type inmemRepo struct {
	mu    sync.Mutex
	plans map[string]copyops.Plan
}

func (r *inmemRepo) Create(_ context.Context, p copyops.Plan) (copyops.Plan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plans[p.ID] = p
	return p, nil
}

func (r *inmemRepo) GetByID(_ context.Context, id string) (copyops.Plan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[id]
	if !ok {
		return copyops.Plan{}, errors.New("not found")
	}
	return p, nil
}

func (r *inmemRepo) ListByUser(_ context.Context, userID int64, page int, pageSize int) ([]copyops.Plan, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]copyops.Plan, 0, len(r.plans))
	for _, p := range r.plans {
		if p.RequestedByUserID != nil && *p.RequestedByUserID == userID {
			out = append(out, p)
		}
	}
	return slicePage(out, page, pageSize)
}

func (r *inmemRepo) ListByCluster(_ context.Context, clusterID int64, page int, pageSize int) ([]copyops.Plan, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]copyops.Plan, 0, len(r.plans))
	for _, p := range r.plans {
		if p.SourceClusterID == clusterID || p.TargetClusterID == clusterID {
			out = append(out, p)
		}
	}
	return slicePage(out, page, pageSize)
}

func (r *inmemRepo) ClaimAndLoad(_ context.Context, planID, idempotencyKey string, confirmationHash []byte, newIdempotencyKey string) (copyops.Plan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[planID]
	if !ok {
		return copyops.Plan{}, copyops.ErrNotFound
	}
	// Mirrors the GormRepository logic.
	switch p.Status {
	case copyops.StatusSucceeded, copyops.StatusFailed, copyops.StatusExpired:
		if p.IdempotencyKey != "" {
			if p.IdempotencyKey != idempotencyKey {
				return copyops.Plan{}, copyops.ErrInvalidIdempotency
			}
			return p, nil
		}
		return copyops.Plan{}, copyops.ErrAlreadyExecuted
	case copyops.StatusExecuting:
		if p.LockedAt != nil && time.Since(*p.LockedAt) < 15*time.Second {
			return copyops.Plan{}, copyops.ErrInProgress
		}
		// Lease expired → refresh locked_at.
	case copyops.StatusAwaitingConfirmation:
		// proceed
	default:
		return copyops.Plan{}, copyops.ErrAlreadyExecuted
	}
	if p.Status == copyops.StatusAwaitingConfirmation {
		if len(p.ConfirmationTokenHash) > 0 {
			h1 := base64.StdEncoding.EncodeToString(p.ConfirmationTokenHash)
			h2 := base64.StdEncoding.EncodeToString(confirmationHash)
			if h1 != h2 {
				return copyops.Plan{}, copyops.ErrConfirmationInvalid
			}
		}
		if p.IdempotencyKey != "" {
			if p.IdempotencyKey != idempotencyKey {
				return copyops.Plan{}, copyops.ErrInvalidIdempotency
			}
			return p, nil
		}
	}
	now := time.Now()
	if p.Status == copyops.StatusAwaitingConfirmation {
		p.Status = copyops.StatusExecuting
	}
	p.LockedAt = &now
	p.IdempotencyKey = newIdempotencyKey
	r.plans[planID] = p
	// return a copy (as-if reload)
	out := r.plans[planID]
	return out, nil
}

func (r *inmemRepo) UpdateExecution(_ context.Context, plan copyops.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plans[plan.ID]; !ok {
		return errors.New("not found")
	}
	r.plans[plan.ID] = plan
	return nil
}

func (r *inmemRepo) UpdateStatus(_ context.Context, planID, status, lastError string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plans[planID]
	if !ok {
		return errors.New("not found")
	}
	p.Status = status
	p.LastError = lastError
	r.plans[planID] = p
	return nil
}

func slicePage[T any](items []T, page, size int) ([]T, int, error) {
	total := len(items)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	start := (page - 1) * size
	if start >= total {
		return []T{}, total, nil
	}
	end := start + size
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

type fakeKubernetes struct {
	rawResource func(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]any, error)
	nsExists    func(ctx context.Context, clusterID int64, namespace string) (bool, error)
	nsIdentity  func(ctx context.Context, clusterID int64, namespace string) (k8sgateway.SourceNamespaceIdentity, error)
	resExists   func(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (bool, error)
	createRes   func(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error)
}

func (f *fakeKubernetes) GetRawResource(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (map[string]any, error) {
	return f.rawResource(ctx, clusterID, group, version, resource, namespace, name)
}
func (f *fakeKubernetes) NamespaceExists(ctx context.Context, clusterID int64, namespace string) (bool, error) {
	return f.nsExists(ctx, clusterID, namespace)
}
func (f *fakeKubernetes) SourceNamespaceIdentity(ctx context.Context, clusterID int64, namespace string) (k8sgateway.SourceNamespaceIdentity, error) {
	return f.nsIdentity(ctx, clusterID, namespace)
}
func (f *fakeKubernetes) NamespacedResourceExists(ctx context.Context, clusterID int64, group, version, resource, namespace, name string) (bool, error) {
	return f.resExists(ctx, clusterID, group, version, resource, namespace, name)
}
func (f *fakeKubernetes) CreateResource(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error) {
	return f.createRes(ctx, clusterID, path, body, dryRun)
}

func sourceDeploymentManifest(namespace, name, uid, rv string) map[string]any {
	return map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"uid":               uid,
			"resourceVersion":   rv,
			"creationTimestamp": "2026-04-01T00:00:00Z",
			"selfLink":          "/apis/apps/v1/namespaces/...",
			"labels": map[string]any{
				"app":                         "nginx",
				"argocd.argoproj.io/instance": "my-app",
			},
			"annotations": map[string]any{
				"kubectl.kubernetes.io/last-applied-configuration": "...",
				"deploy.kubernetes.io/revision":                    "42",
			},
		},
		"spec": map[string]any{
			"replicas": 3,
			"template": map[string]any{
				"spec": map[string]any{
					"nodeName": "pin-this-node",
					"containers": []any{
						map[string]any{"name": "nginx", "image": "nginx:1.25"},
					},
				},
			},
		},
		"status": map[string]any{"replicas": 3, "readyReplicas": 3},
	}
}

func TestValidatePreviewRequest_InvalidCluster(t *testing.T) {
	svc := copyops.NewService(nil, nil)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 0, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrInvalidRequest)
}

func TestValidatePreviewRequest_SameCluster(t *testing.T) {
	svc := copyops.NewService(nil, nil)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 1,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrCrossClusterSame)
}

func TestValidatePreviewRequest_EmptyBundle(t *testing.T) {
	svc := copyops.NewService(nil, nil)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrBundleEmpty)
}

func TestValidatePreviewRequest_BundleTooLarge(t *testing.T) {
	svc := copyops.NewService(nil, nil)
	bundle := make([]copyops.BundleItemRequest, 21)
	for i := range bundle {
		bundle[i] = copyops.BundleItemRequest{Kind: "ConfigMap", Namespace: "prod", Name: "cm"}
	}
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: bundle,
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrBundleTooLarge)
}

func TestValidatePreviewRequest_DisallowedKind(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	seed := uint32(1)
	randFn := func(n int) ([]byte, error) {
		buf := make([]byte, n)
		for i := range buf {
			seed++
			buf[i] = byte(seed)
		}
		return buf, nil
	}
	clock := time.Now
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "ns-uid", ResourceVersion: "10"}, nil
		},
		nsExists:  func(context.Context, int64, string) (bool, error) { return true, nil },
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(context.Context, int64, string, []byte, bool) ([]byte, error) { return nil, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "uid-ss", "rv-42"), nil
		},
	}
	svc := copyops.NewTestService(fake, repo, clock, randFn)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "ValidatingWebhookConfiguration", Namespace: "prod", Name: "wh"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrKindDisallowed)
}

func TestPreview_Success(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}

	var seed uint32
	randFn := func(n int) ([]byte, error) {
		buf := make([]byte, n)
		for i := range buf {
			seed++
			buf[i] = byte(seed)
		}
		return buf, nil
	}
	clock := frozenClock(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))

	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "ns-uid-prod", ResourceVersion: "42"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "src-uid", "src-rv-1"), nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(_ context.Context, _ int64, _ string, body []byte, dryRun bool) ([]byte, error) {
			if !dryRun {
				return nil, errors.New("expected dry-run only during Preview")
			}
			var obj map[string]any
			require.NoError(t, json.Unmarshal(body, &obj))
			obj["metadata"].(map[string]any)["uid"] = "dry-run-uid"
			obj["metadata"].(map[string]any)["resourceVersion"] = "1"
			return json.Marshal(obj)
		},
	}

	svc := copyops.NewTestService(fake, repo, clock, randFn)
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{
			{Kind: "Deployment", Namespace: "prod", Name: "nginx"},
		},
		StripLabelPrefixes:      []string{"argocd.argoproj.io/"},
		StripAnnotationPrefixes: []string{"kubectl.kubernetes.io/"},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	assert.Equal(t, copyops.StatusAwaitingConfirmation, plan.Status)
	assert.Equal(t, int64(1), plan.SourceClusterID)
	assert.Equal(t, int64(2), plan.TargetClusterID)
	assert.Equal(t, "ns-uid-prod", plan.SourceNamespaceUID)
	assert.NotEmpty(t, plan.ConfirmationToken, "preview should return one-time confirmation token")
	// Persisted in DB:
	saved, err := repo.GetByID(context.Background(), plan.ID)
	require.NoError(t, err)
	assert.Equal(t, "alice", saved.RequestedByName)
	items := copyops.UnmarshalResourceItems(saved.ResourceItems)
	require.Len(t, items, 1)
	// Scrubbed manifest has no status.
	manifest := items[0].Manifest
	assert.NotContains(t, manifest, "status")
	assert.Equal(t, copyops.ModeCreate, copyops.UnmarshalDiff(items[0].Diff).Mode)
	meta := manifest["metadata"].(map[string]any)
	// Sensitive annotations/labels stripped.
	labels := meta["labels"].(map[string]any)
	_, hasArgoCD := labels["argocd.argoproj.io/instance"]
	assert.False(t, hasArgoCD, "strip label prefix should have removed ArgoCD labels")
	ann := meta["annotations"].(map[string]any)
	_, hasLastApplied := ann["kubectl.kubernetes.io/last-applied-configuration"]
	assert.False(t, hasLastApplied, "strip annotation prefix should have removed kubectl annotations")
	// nodeName should be dropped (pinning scrubbed).
	spec := manifest["spec"].(map[string]any)
	tmpl := spec["template"].(map[string]any)
	tmplSpec := tmpl["spec"].(map[string]any)
	_, hasNodeName := tmplSpec["nodeName"]
	assert.False(t, hasNodeName, "nodeName should be scrubbed from template.spec")
	// Destination namespace rewritten.
	assert.Equal(t, "stage", meta["namespace"])
}

func TestPreview_DestinationNamespaceMissing(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	clock := frozenClock(time.Now())
	randFn := staticRand([]byte{0xAB})
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "src-ns-uid", ResourceVersion: "1"}, nil
		},
		nsExists: func(_ context.Context, clusterID int64, namespace string) (bool, error) {
			if clusterID == 2 {
				return false, nil
			}
			return true, nil
		},
	}
	svc := copyops.NewTestService(fake, repo, clock, randFn)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "ConfigMap", Namespace: "prod", Name: "env"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrNamespaceMissing)
}

func TestExecute_IdempotencyReplay(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}

	var counter int32
	randFn := func(n int) ([]byte, error) {
		i := atomic.AddInt32(&counter, 1)
		buf := make([]byte, n)
		for j := range buf {
			buf[j] = byte(i)
		}
		return buf, nil
	}
	clock := frozenClock(time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC))
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "src-ns-uid", ResourceVersion: "1"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			m := map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":            name,
					"namespace":       ns,
					"uid":             "src-cm-uid",
					"resourceVersion": "42",
				},
				"data": map[string]any{"KEY": "value"},
			}
			return m, nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(_ context.Context, clusterID int64, _ string, body []byte, dryRun bool) ([]byte, error) {
			var obj map[string]any
			_ = json.Unmarshal(body, &obj)
			if meta, ok := obj["metadata"].(map[string]any); ok {
				meta["uid"] = "apply-" + base64.RawURLEncoding.EncodeToString(body[:4])
				meta["resourceVersion"] = "99"
			}
			return json.Marshal(obj)
		},
	}
	svc := copyops.NewTestService(fake, repo, clock, randFn)
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "ConfigMap", Namespace: "prod", Name: "env"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)

	// First execute.
	const idemKey = "idem-demo-1"
	token := plan.ConfirmationToken
	result1, err := svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: token, IdempotencyKey: idemKey,
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	require.Equal(t, copyops.StatusSucceeded, result1.Status)
	items := copyops.UnmarshalResourceItems(result1.ResourceItems)
	require.Len(t, items, 1)
	assert.Equal(t, copyops.ItemStatusApplied, items[0].ItemStatus)
	assert.NotEmpty(t, items[0].AppliedUID)

	// Idempotent replay of the *same* idempotency key should return the
	// already-committed plan without re-applying.
	result2, err := svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: token, IdempotencyKey: idemKey,
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	assert.Equal(t, result1.ID, result2.ID)
	assert.Equal(t, result1.Status, result2.Status)

	// A different idempotency key against the same plan should be rejected.
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: token, IdempotencyKey: idemKey + "-different",
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.ErrorIs(t, err, copyops.ErrInvalidIdempotency)
}

func TestExecute_CASDrift(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	clock := frozenClock(time.Now())
	randFn := staticRand([]byte{0x11})
	first := true
	fake := &fakeKubernetes{
		nsIdentity: func(_ context.Context, clusterID int64, namespace string) (k8sgateway.SourceNamespaceIdentity, error) {
			if first {
				first = false
				return k8sgateway.SourceNamespaceIdentity{Name: namespace, UID: "original-ns-uid", ResourceVersion: "1"}, nil
			}
			// Simulate source namespace was recreated between preview and execute.
			return k8sgateway.SourceNamespaceIdentity{Name: namespace, UID: "different-uid", ResourceVersion: "999"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return map[string]any{
				"apiVersion": "v1", "kind": "ConfigMap",
				"metadata": map[string]any{
					"name": name, "namespace": ns, "uid": "u", "resourceVersion": "1",
				},
				"data": map[string]any{"k": "v"},
			}, nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(context.Context, int64, string, []byte, bool) ([]byte, error) {
			return nil, nil
		},
	}
	svc := copyops.NewTestService(fake, repo, clock, randFn)
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "ConfigMap", Namespace: "prod", Name: "env"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)

	result, err := svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: plan.ConfirmationToken, IdempotencyKey: "k1",
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err, "execute does not return HTTP errors; it persists failure status")
	assert.Equal(t, copyops.StatusFailed, result.Status, "CAS drift should fail the plan cleanly")
	assert.Contains(t, result.LastError, "drift")
}

func TestMarshalHelpers(t *testing.T) {
	items := []copyops.ResourceItem{{Kind: "ConfigMap", SourceNamespace: "prod", SourceName: "env", DestinationNamespace: "stage", DestinationName: "env"}}
	j := copyops.MarshalResourceItems(items)
	got := copyops.UnmarshalResourceItems(j)
	require.Len(t, got, 1)
	assert.Equal(t, "ConfigMap", got[0].Kind)

	diff := copyops.MarshalPlanDiff(copyops.PlanDiff{ResourceCount: 3, WillCreateCount: 2, WillSkipCount: 1})
	pd := copyops.UnmarshalPlanDiff(diff)
	assert.Equal(t, 3, pd.ResourceCount)

	summary := copyops.MarshalCopySummary([]copyops.CopySummaryItem{{Kind: "ConfigMap", Namespace: "prod", Name: "env", DestNamespace: "stage", DestName: "env"}})
	s := copyops.UnmarshalCopySummary(summary)
	require.Len(t, s, 1)
	assert.Equal(t, "env", s[0].Name)
}

// --- small helpers -----------------------------------------------------------

func frozenClock(now time.Time) func() time.Time { return func() time.Time { return now } }

func staticRand(buf []byte) func(n int) ([]byte, error) {
	return func(n int) ([]byte, error) {
		out := make([]byte, n)
		copy(out, buf)
		return out, nil
	}
}

// sha256Sum helper so tests can hash a token manually.
func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

var _ = sha256Sum
var _ apiquery.ListQuery

// --- M115-1h: Get / ListByUser / ListByCluster / NewService ---

func previewK8sFake(t *testing.T) *fakeKubernetes {
	t.Helper()
	return &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "src", UID: "ns-uid", ResourceVersion: "42"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "src-uid", "src-rv-1"), nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(_ context.Context, _ int64, _ string, body []byte, dryRun bool) ([]byte, error) {
			if !dryRun {
				return nil, errors.New("expected dry-run during Preview")
			}
			return body, nil
		},
	}
}

func TestServiceGetReturnsPlan(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), staticRand([]byte{1, 2, 3}))
	uid := int64(7)
	created, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}},
	}, copyops.ActorRef{ID: uid, Name: "alice"})
	require.NoError(t, err)

	got, err := svc.Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
}

func TestServiceGetRejectsEmptyID(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), staticRand([]byte{1}))
	_, err := svc.Get(context.Background(), "")
	assert.ErrorIs(t, err, copyops.ErrInvalidRequest)
}

func TestServiceListByUser(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	var seed uint32
	randFn := func(n int) ([]byte, error) {
		buf := make([]byte, n)
		for i := range buf {
			seed++
			buf[i] = byte(seed)
		}
		return buf, nil
	}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), randFn)
	uidA, uidB := int64(7), int64(8)
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{SourceClusterID: 1, TargetClusterID: 2, SourceNamespace: "prod", TargetNamespace: "stage", Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}}}, copyops.ActorRef{ID: uidA, Name: "a"})
	require.NoError(t, err)
	_, err = svc.Preview(context.Background(), copyops.PreviewRequest{SourceClusterID: 3, TargetClusterID: 4, SourceNamespace: "dev", TargetNamespace: "test", Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}}}, copyops.ActorRef{ID: uidB, Name: "b"})
	require.NoError(t, err)

	items, total, err := svc.ListByUser(context.Background(), uidA, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
}

func TestServiceListByUserRejectsInvalid(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), staticRand([]byte{1}))
	_, _, err := svc.ListByUser(context.Background(), 0, 0, 10)
	assert.ErrorIs(t, err, copyops.ErrInvalidRequest)
}

func TestServiceListByCluster(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), staticRand([]byte{1}))
	_, err := svc.Preview(context.Background(), copyops.PreviewRequest{SourceClusterID: 1, TargetClusterID: 2, SourceNamespace: "prod", TargetNamespace: "stage", Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "nginx"}}}, copyops.ActorRef{ID: 7, Name: "a"})
	require.NoError(t, err)

	items, total, err := svc.ListByCluster(context.Background(), 1, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, items, 1)
}

func TestServiceListByClusterRejectsInvalid(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(previewK8sFake(t), repo, frozenClock(time.Now()), staticRand([]byte{1}))
	_, _, err := svc.ListByCluster(context.Background(), 0, 0, 10)
	assert.ErrorIs(t, err, copyops.ErrInvalidRequest)
}

func TestServiceNewServiceDefaults(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewService(nil, repo)
	assert.NotNil(t, svc)
}

// --- M115-1p: Execute validation + failPlan error branches ---

func TestExecuteValidationBranches(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(&fakeKubernetes{}, repo, frozenClock(time.Now()), staticRand([]byte{0xAA}))
	_, err := svc.Execute(context.Background(), copyops.ExecuteRequest{PlanID: "short"}, copyops.ActorRef{ID: 1, Name: "x"})
	if err != copyops.ErrInvalidRequest {
		t.Fatalf("bad plan id = %v", err)
	}
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ConfirmationToken: "",
		IdempotencyKey:    "k1",
	}, copyops.ActorRef{ID: 1, Name: "x"})
	if err != copyops.ErrConfirmationInvalid {
		t.Fatalf("empty token = %v", err)
	}
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ConfirmationToken: "tok",
		IdempotencyKey:    "",
	}, copyops.ActorRef{ID: 1, Name: "x"})
	if err != copyops.ErrInvalidIdempotency {
		t.Fatalf("empty idem = %v", err)
	}
}

func TestExecute_ClaimNotFound(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	svc := copyops.NewTestService(&fakeKubernetes{}, repo, frozenClock(time.Now()), staticRand([]byte{0xAA}))
	_, err := svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID:            "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		ConfirmationToken: "tok",
		IdempotencyKey:    "k1",
	}, copyops.ActorRef{ID: 1, Name: "x"})
	if err != copyops.ErrNotFound {
		t.Fatalf("claim missing = %v", err)
	}
}

func TestExecute_NSIdentityError(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	first := true
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			if first {
				first = false
				return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "ns-uid", ResourceVersion: "42"}, nil
			}
			return k8sgateway.SourceNamespaceIdentity{}, errors.New("identity failed")
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "u1", "rv1"), nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(context.Context, int64, string, []byte, bool) ([]byte, error) {
			return []byte(`{"metadata":{"uid":"applied","resourceVersion":"1"}}`), nil
		},
	}
	svc := copyops.NewTestService(fake, repo, frozenClock(time.Now()), staticRand([]byte{0xBB}))
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "app"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: plan.ConfirmationToken, IdempotencyKey: "k1",
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	got, _ := svc.Get(context.Background(), plan.ID)
	assert.Equal(t, copyops.StatusFailed, got.Status)
	assert.Contains(t, got.LastError, "identity read failed")
}

func TestExecute_DestNamespaceDeleted(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	nsExistsFirst := true
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "ns-uid", ResourceVersion: "42"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) {
			if nsExistsFirst {
				nsExistsFirst = false
				return true, nil
			}
			return false, nil
		},
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "u1", "rv1"), nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(context.Context, int64, string, []byte, bool) ([]byte, error) {
			return []byte(`{"metadata":{"uid":"x","resourceVersion":"1"}}`), nil
		},
	}
	svc := copyops.NewTestService(fake, repo, frozenClock(time.Now()), staticRand([]byte{0xCC}))
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "app"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: plan.ConfirmationToken, IdempotencyKey: "k1",
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	got, _ := svc.Get(context.Background(), plan.ID)
	assert.Equal(t, copyops.StatusFailed, got.Status)
	assert.Contains(t, got.LastError, "deleted between Preview and Execute")
}

func TestExecute_CreateResourceError(t *testing.T) {
	repo := &inmemRepo{plans: map[string]copyops.Plan{}}
	createCalls := 0
	fake := &fakeKubernetes{
		nsIdentity: func(context.Context, int64, string) (k8sgateway.SourceNamespaceIdentity, error) {
			return k8sgateway.SourceNamespaceIdentity{Name: "prod", UID: "ns-uid", ResourceVersion: "42"}, nil
		},
		nsExists: func(context.Context, int64, string) (bool, error) { return true, nil },
		rawResource: func(_ context.Context, _ int64, _ string, _ string, _ string, ns string, name string) (map[string]any, error) {
			return sourceDeploymentManifest(ns, name, "u1", "rv1"), nil
		},
		resExists: func(context.Context, int64, string, string, string, string, string) (bool, error) { return false, nil },
		createRes: func(ctx context.Context, clusterID int64, path string, body []byte, dryRun bool) ([]byte, error) {
			createCalls++
			if dryRun {
				// Preview dry-run succeeds; Execute real create fails.
				return []byte(`{"metadata":{"uid":"x","resourceVersion":"1"}}`), nil
			}
			return nil, errors.New("admission denied")
		},
	}
	svc := copyops.NewTestService(fake, repo, frozenClock(time.Now()), staticRand([]byte{0xDD}))
	plan, err := svc.Preview(context.Background(), copyops.PreviewRequest{
		SourceClusterID: 1, TargetClusterID: 2,
		SourceNamespace: "prod", TargetNamespace: "stage",
		Bundle: []copyops.BundleItemRequest{{Kind: "Deployment", Namespace: "prod", Name: "app"}},
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	_, err = svc.Execute(context.Background(), copyops.ExecuteRequest{
		PlanID: plan.ID, ConfirmationToken: plan.ConfirmationToken, IdempotencyKey: "k1",
	}, copyops.ActorRef{ID: 7, Name: "alice"})
	require.NoError(t, err)
	got, _ := svc.Get(context.Background(), plan.ID)
	assert.Equal(t, copyops.StatusFailed, got.Status)
}
