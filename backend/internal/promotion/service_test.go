package promotion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
)

type kubeStub struct {
	mu                sync.Mutex
	manifests         map[string]json.RawMessage
	namespaceExists   bool
	configMaps        map[string]bool
	secrets           map[string]bool
	existingResources map[string]bool
	creates           []createRecord
	createErr         error
	createErrors      map[string]error
}

type createRecord struct {
	path   string
	body   []byte
	dryRun bool
}

func newKubeStub() *kubeStub {
	return &kubeStub{
		manifests:         make(map[string]json.RawMessage),
		configMaps:        make(map[string]bool),
		secrets:           make(map[string]bool),
		existingResources: make(map[string]bool),
		namespaceExists:   true,
	}
}

func manifestKey(kind, ns, name string) string { return kind + "/" + ns + "/" + name }

func (k *kubeStub) RawManifest(_ context.Context, _ int64, kind, ns, name string) (json.RawMessage, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if m, ok := k.manifests[manifestKey(kind, ns, name)]; ok {
		return m, nil
	}
	return nil, fmt.Errorf("manifest not found for %s", manifestKey(kind, ns, name))
}

func (k *kubeStub) NamespaceExists(_ context.Context, _ int64, _ string) (bool, error) {
	return k.namespaceExists, nil
}

func (k *kubeStub) ConfigMapExists(_ context.Context, _ int64, ns, name string) (bool, error) {
	return k.configMaps[ns+"/"+name], nil
}

func (k *kubeStub) SecretExists(_ context.Context, _ int64, ns, name string) (bool, error) {
	return k.secrets[ns+"/"+name], nil
}

func (k *kubeStub) ResourceExists(_ context.Context, _ int64, path string) (bool, error) {
	return k.existingResources[path], nil
}

func (k *kubeStub) CreateResource(_ context.Context, _ int64, path string, body []byte, dryRun bool) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.creates = append(k.creates, createRecord{path: path, body: append([]byte(nil), body...), dryRun: dryRun})
	if k.createErr != nil {
		return nil, k.createErr
	}
	if err := k.createErrors[path]; err != nil {
		return nil, err
	}
	return body, nil
}

type repoStub struct {
	plans         map[string]Plan
	saveErr       error
	claimErr      error
	completeCalls int
	failCalls     int
}

func newRepoStub() *repoStub {
	return &repoStub{plans: make(map[string]Plan)}
}

func (r *repoStub) Save(_ context.Context, plan *Plan) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	for index := range plan.Items {
		plan.Items[index].ID = int64(index + 1)
	}
	r.plans[plan.ID] = *plan
	return nil
}

func (r *repoStub) Get(_ context.Context, id string) (Plan, error) {
	if plan, ok := r.plans[id]; ok {
		return plan, nil
	}
	return Plan{}, ErrNotFound
}

func (r *repoStub) List(_ context.Context, _ int64, _ string) ([]Plan, error) {
	plans := make([]Plan, 0, len(r.plans))
	for _, plan := range r.plans {
		plans = append(plans, plan)
	}
	return plans, nil
}

func (r *repoStub) Claim(_ context.Context, id string, _ []byte, idempotencyKey string, _, _ time.Time) (Plan, bool, error) {
	if r.claimErr != nil {
		return Plan{}, false, r.claimErr
	}
	plan, ok := r.plans[id]
	if !ok {
		return Plan{}, false, ErrNotFound
	}
	if plan.Status != StatusAwaitingConfirmation && plan.Status != StatusExecuting {
		return plan, false, nil
	}
	plan.Status = StatusExecuting
	plan.IdempotencyKey = idempotencyKey
	now := time.Now().UTC()
	plan.LockedAt = &now
	r.plans[id] = plan
	return plan, true, nil
}

func (r *repoStub) Complete(_ context.Context, id, _ string, _ time.Time, itemStatuses map[int64]string, itemErrors map[int64]string) (Plan, error) {
	r.completeCalls++
	plan := r.plans[id]
	applied, failed, skipped := 0, 0, 0
	for _, status := range itemStatuses {
		switch status {
		case ItemStatusApplied:
			applied++
		case ItemStatusFailed:
			failed++
		case ItemStatusSkipped:
			skipped++
		}
	}
	overall := StatusSucceeded
	if (failed > 0 || skipped > 0) && applied > 0 {
		overall = StatusPartial
	} else if failed > 0 || skipped > 0 {
		overall = StatusFailed
	}
	plan.Status = overall
	executedAt := time.Now().UTC()
	plan.ExecutedAt = &executedAt
	plan.LockedAt = nil
	for i := range plan.Items {
		if status, ok := itemStatuses[plan.Items[i].ID]; ok {
			plan.Items[i].ItemStatus = status
			plan.Items[i].LastError = itemErrors[plan.Items[i].ID]
		}
	}
	r.plans[id] = plan
	return plan, nil
}

func (r *repoStub) Fail(_ context.Context, id, _, message string) (Plan, error) {
	r.failCalls++
	plan := r.plans[id]
	plan.Status = StatusFailed
	plan.LastError = message
	plan.LockedAt = nil
	r.plans[id] = plan
	return plan, nil
}

func (r *repoStub) ExpireStale(_ context.Context, _ time.Time) error { return nil }

func deploymentManifest(ns, name, image string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":%q,"namespace":%q,"uid":"source-uid-1","resourceVersion":"100","creationTimestamp":"2026-01-01T00:00:00Z","labels":{"app":%q}},"spec":{"replicas":2,"selector":{"matchLabels":{"app":%q}},"template":{"metadata":{"labels":{"app":%q}},"spec":{"containers":[{"name":"app","image":%q}]}}},"status":{"readyReplicas":2}}`, name, ns, name, name, name, image))
}

func serviceManifest(ns, name string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"apiVersion":"v1","kind":"Service","metadata":{"name":%q,"namespace":%q,"uid":"svc-uid-1","resourceVersion":"101","creationTimestamp":"2026-01-01T00:00:00Z"},"spec":{"type":"ClusterIP","ports":[{"port":80,"targetPort":8080}]},"status":{}}`, name, ns))
}

func deploymentWithConfigMapRef(ns, name, cmName string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":%q,"namespace":%q,"uid":"source-uid-2","resourceVersion":"200"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":%q}},"template":{"metadata":{"labels":{"app":%q}},"spec":{"containers":[{"name":"app","image":"nginx:1.27","envFrom":[{"configMapRef":{"name":%q}}]}]}}},"status":{}}`, name, ns, name, name, cmName))
}

func deploymentWithSecretRef(ns, name, secretName string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":%q,"namespace":%q,"uid":"source-uid-3","resourceVersion":"300"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":%q}},"template":{"metadata":{"labels":{"app":%q}},"spec":{"containers":[{"name":"app","image":"nginx:1.27","env":[{"name":"TOKEN","valueFrom":{"secretKeyRef":{"name":%q,"key":"token"}}}]}],"volumes":[{"name":"secret","secret":{"secretName":%q}}]}}},"status":{}}`, name, ns, name, name, secretName, secretName))
}

func actor() ActorRef { return ActorRef{ID: 1, Name: "admin@example.com"} }

func TestPreviewRejectsSameSourceAndDestinationCluster(t *testing.T) {
	service := NewService(newKubeStub(), newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 5,
		SourceNamespace: "demo", DestinationNamespace: "demo",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestPreviewRejectsEmptyBundle(t *testing.T) {
	service := NewService(newKubeStub(), newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "demo",
		Bundle: []BundleItemRequest{},
	}, actor())
	if !errors.Is(err, ErrBundleEmpty) {
		t.Fatalf("expected ErrBundleEmpty, got %v", err)
	}
}

func TestPreviewFailsWhenDestinationNamespaceMissing(t *testing.T) {
	kube := newKubeStub()
	kube.namespaceExists = false
	service := NewService(kube, newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if !errors.Is(err, ErrNamespaceMissing) {
		t.Fatalf("expected ErrNamespaceMissing, got %v", err)
	}
}

func TestPreviewFailsWhenDestinationResourceExists(t *testing.T) {
	kube := newKubeStub()
	kube.existingResources["/apis/apps/v1/namespaces/staging/deployments/api"] = true
	service := NewService(kube, newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestPreviewStripsRuntimeFieldsAndRewritesNamespace(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentManifest("demo", "api", "nginx:1.27")
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if plan.ID == "" || plan.ConfirmationToken == "" {
		t.Fatal("preview must return a plan id and a one-time confirmation token")
	}
	if plan.SourceClusterID != 5 || plan.DestinationClusterID != 6 {
		t.Fatal("plan must carry source and destination cluster ids")
	}
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 bundle item, got %d", len(plan.Items))
	}
	item := plan.Items[0]
	if item.SourceUID != "source-uid-1" || item.SourceResourceVersion != "100" {
		t.Fatalf("source evidence must be captured, got uid=%q rv=%q", item.SourceUID, item.SourceResourceVersion)
	}
	var manifest map[string]any
	if err := json.Unmarshal(item.Manifest, &manifest); err != nil {
		t.Fatalf("decode stripped manifest: %v", err)
	}
	if _, ok := manifest["status"]; ok {
		t.Fatal("status must be stripped from the promotion manifest")
	}
	metadata := manifest["metadata"].(map[string]any)
	if _, ok := metadata["uid"]; ok {
		t.Fatal("metadata.uid must be stripped")
	}
	if _, ok := metadata["resourceVersion"]; ok {
		t.Fatal("metadata.resourceVersion must be stripped")
	}
	if _, ok := metadata["creationTimestamp"]; ok {
		t.Fatal("metadata.creationTimestamp must be stripped")
	}
	if metadata["namespace"] != "staging" {
		t.Fatalf("metadata.namespace must be rewritten to staging, got %v", metadata["namespace"])
	}
	if len(kube.creates) != 1 || !kube.creates[0].dryRun {
		t.Fatalf("preview must perform a single dry-run create, got %d creates", len(kube.creates))
	}
}

func TestPreviewRequiresDependencyMappings(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentWithConfigMapRef("demo", "api", "app-config")
	service := NewService(kube, newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if !errors.Is(err, ErrDependencyUnresolved) {
		t.Fatalf("expected ErrDependencyUnresolved when no mapping is provided, got %v", err)
	}
}

func TestPreviewFailsWhenDestinationDependencyMissing(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentWithConfigMapRef("demo", "api", "app-config")
	service := NewService(kube, newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle:             []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
		DependencyMappings: []DependencyMapping{{Kind: "ConfigMap", SourceNamespace: "demo", SourceName: "app-config", DestinationNamespace: "staging", DestinationName: "app-config"}},
	}, actor())
	if !errors.Is(err, ErrDependencyUnresolved) {
		t.Fatalf("expected ErrDependencyUnresolved when destination ConfigMap is missing, got %v", err)
	}
}

func TestPreviewSucceedsWithResolvedDependencies(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentWithConfigMapRef("demo", "api", "app-config")
	kube.configMaps["staging/app-config"] = true
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle:             []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
		DependencyMappings: []DependencyMapping{{Kind: "ConfigMap", SourceNamespace: "demo", SourceName: "app-config", DestinationNamespace: "staging", DestinationName: "app-config"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if len(plan.Dependencies) != 1 || !plan.Dependencies[0].Resolved {
		t.Fatalf("dependency must be marked resolved, got %+v", plan.Dependencies)
	}
}

func TestPreviewRewritesMappedDependencyNames(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentWithConfigMapRef("demo", "api", "source-config")
	kube.manifests[manifestKey("Service", "demo", "api")] = serviceManifest("demo", "api")
	kube.configMaps["staging/destination-config"] = true
	plan, err := NewService(kube, newRepoStub()).Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{
			{Kind: "Deployment", Namespace: "demo", Name: "api"},
			{Kind: "Service", Namespace: "demo", Name: "api"},
		},
		DependencyMappings: []DependencyMapping{{Kind: "ConfigMap", SourceNamespace: "demo", SourceName: "source-config", DestinationNamespace: "staging", DestinationName: "destination-config"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(plan.Items[0].Manifest, &manifest); err != nil {
		t.Fatalf("decode promoted manifest: %v", err)
	}
	containers := walkPath(manifest, "spec", "template", "spec")["containers"].([]any)
	envFrom := containers[0].(map[string]any)["envFrom"].([]any)
	configMapRef := envFrom[0].(map[string]any)["configMapRef"].(map[string]any)
	if configMapRef["name"] != "destination-config" {
		t.Fatalf("mapped dependency name = %v, want destination-config", configMapRef["name"])
	}
	if len(plan.Dependencies) != 1 || !plan.Dependencies[0].Resolved {
		t.Fatalf("bundle dependency records must be deduplicated and resolved, got %#v", plan.Dependencies)
	}
}

func TestPreviewStripsServiceAllocatedFields(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Service", "demo", "api")] = json.RawMessage(`{
		"apiVersion":"v1","kind":"Service","metadata":{"name":"api","namespace":"demo","uid":"svc-1","resourceVersion":"9"},
		"spec":{"type":"NodePort","clusterIP":"10.96.0.20","clusterIPs":["10.96.0.20"],"ipFamilies":["IPv4"],"ipFamilyPolicy":"SingleStack","healthCheckNodePort":30123,"ports":[{"port":80,"targetPort":8080,"nodePort":30080}]},"status":{}
	}`)
	plan, err := NewService(kube, newRepoStub()).Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Service", Namespace: "demo", Name: "api"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(plan.Items[0].Manifest, &manifest); err != nil {
		t.Fatalf("decode promoted service: %v", err)
	}
	spec := manifest["spec"].(map[string]any)
	for _, field := range []string{"clusterIP", "clusterIPs", "ipFamilies", "ipFamilyPolicy", "healthCheckNodePort"} {
		if _, exists := spec[field]; exists {
			t.Fatalf("server-allocated service field %q was not stripped", field)
		}
	}
	port := spec["ports"].([]any)[0].(map[string]any)
	if _, exists := port["nodePort"]; exists {
		t.Fatal("server-allocated Service nodePort was not stripped")
	}
}

func TestPreviewScansSecretRefsInVolumesAndEnv(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentWithSecretRef("demo", "api", "app-secret")
	kube.secrets["staging/app-secret"] = true
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle:             []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
		DependencyMappings: []DependencyMapping{{Kind: "Secret", SourceNamespace: "demo", SourceName: "app-secret", DestinationNamespace: "staging", DestinationName: "app-secret"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if len(plan.Dependencies) != 1 {
		t.Fatalf("expected 1 resolved dependency, got %d", len(plan.Dependencies))
	}
}

func TestPreviewDryRunFailureReturnsPreviewFailed(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentManifest("demo", "api", "nginx:1.27")
	kube.createErr = cluster.APIStatusError{StatusCode: 422}
	service := NewService(kube, newRepoStub())
	_, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if !errors.Is(err, ErrPreviewFailed) {
		t.Fatalf("expected ErrPreviewFailed, got %v", err)
	}
}

func TestExecuteAppliesBundleItemsInOrder(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentManifest("demo", "api", "nginx:1.27")
	kube.manifests[manifestKey("Service", "demo", "api")] = serviceManifest("demo", "api")
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{
			{Kind: "Deployment", Namespace: "demo", Name: "api"},
			{Kind: "Service", Namespace: "demo", Name: "api"},
		},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	kube.mu.Lock()
	kube.creates = nil
	kube.mu.Unlock()
	executed, err := service.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "idempotency-key-1234")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if executed.Status != StatusSucceeded {
		t.Fatalf("expected status succeeded, got %s", executed.Status)
	}
	kube.mu.Lock()
	defer kube.mu.Unlock()
	if len(kube.creates) != 2 {
		t.Fatalf("expected 2 creates, got %d", len(kube.creates))
	}
	for _, create := range kube.creates {
		if create.dryRun {
			t.Fatal("execute must apply with dryRun=false")
		}
	}
}

func TestExecuteConflictMarksItemSkippedAndPlanPartial(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentManifest("demo", "api", "nginx:1.27")
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{{Kind: "Deployment", Namespace: "demo", Name: "api"}},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	kube.mu.Lock()
	kube.creates = nil
	kube.createErr = cluster.APIStatusError{StatusCode: 409}
	kube.mu.Unlock()
	_, err = service.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "idempotency-key-1234")
	if err == nil {
		t.Fatal("execute with a skipped item should not return nil error")
	}
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed, got %v", err)
	}
	if repo.completeCalls != 1 || repo.failCalls != 0 {
		t.Fatalf("expected outcomes to be completed transactionally, complete=%d fail=%d", repo.completeCalls, repo.failCalls)
	}
}

func TestExecutePersistsPerItemOutcomesOnFailure(t *testing.T) {
	kube := newKubeStub()
	kube.manifests[manifestKey("Deployment", "demo", "api")] = deploymentManifest("demo", "api", "nginx:1.27")
	kube.manifests[manifestKey("Service", "demo", "api")] = serviceManifest("demo", "api")
	repo := newRepoStub()
	service := NewService(kube, repo)
	plan, err := service.Preview(context.Background(), PreviewRequest{
		SourceClusterID: 5, DestinationClusterID: 6,
		SourceNamespace: "demo", DestinationNamespace: "staging",
		Bundle: []BundleItemRequest{
			{Kind: "Deployment", Namespace: "demo", Name: "api"},
			{Kind: "Service", Namespace: "demo", Name: "api"},
		},
	}, actor())
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	kube.createErrors = map[string]error{"/api/v1/namespaces/staging/services": errors.New("destination write failed")}
	executed, err := service.Execute(context.Background(), plan.ID, plan.ConfirmationToken, "idempotency-key-1234")
	if !errors.Is(err, ErrExecutionFailed) {
		t.Fatalf("expected ErrExecutionFailed, got %v", err)
	}
	if executed.Status != StatusPartial || len(executed.Items) != 2 {
		t.Fatalf("executed plan = %#v", executed)
	}
	if executed.Items[0].ItemStatus != ItemStatusApplied || executed.Items[1].ItemStatus != ItemStatusFailed {
		t.Fatalf("item outcomes = %#v", executed.Items)
	}
	if repo.completeCalls != 1 || repo.failCalls != 0 {
		t.Fatalf("complete=%d fail=%d", repo.completeCalls, repo.failCalls)
	}
}

func TestPlanJSONOmitsExecutionManifest(t *testing.T) {
	encoded, err := json.Marshal(Plan{Items: []BundleItem{{Manifest: JSON(`{"password":"must-not-escape"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "must-not-escape") || strings.Contains(string(encoded), `"manifest"`) {
		t.Fatalf("promotion response exposed internal execution manifest: %s", encoded)
	}
}

func TestExecuteInvalidIdempotencyKeyRejected(t *testing.T) {
	service := NewService(newKubeStub(), newRepoStub())
	_, err := service.Execute(context.Background(), "plan-id", "token", "short")
	if !errors.Is(err, ErrInvalidIdempotency) {
		t.Fatalf("expected ErrInvalidIdempotency, got %v", err)
	}
}

func TestExecuteMissingConfirmationTokenRejected(t *testing.T) {
	service := NewService(newKubeStub(), newRepoStub())
	_, err := service.Execute(context.Background(), "plan-id", "", "idempotency-key")
	if !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("expected ErrConfirmationInvalid, got %v", err)
	}
}

// --- M115-1i: Get / List tests ---

func previewForGetTest(t *testing.T) (string, *repoStub) {
	t.Helper()
	kube := newKubeStub()
	kube.manifests["Deployment/prod/nginx"] = json.RawMessage(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"nginx","namespace":"prod","uid":"dep-1","resourceVersion":"42","labels":{"app":"nginx"},"annotations":{"kubectl.kubernetes.io/last-applied-configuration":"{}"}},"spec":{"replicas":2,"selector":{"matchLabels":{"app":"nginx"}},"template":{"metadata":{"labels":{"app":"nginx"}},"spec":{"containers":[{"name":"nginx","image":"nginx:1.21","ports":[{"containerPort":80}]}]}}}}`)
	repo := newRepoStub()
	svc := NewService(kube, repo)
	plan, err := svc.Preview(context.Background(), PreviewRequest{
		SourceClusterID:      7,
		DestinationClusterID: 2,
		SourceNamespace:      "prod",
		DestinationNamespace: "stage",
		Bundle: []BundleItemRequest{
			{Kind: "Deployment", Namespace: "prod", Name: "nginx"},
		},
	}, ActorRef{ID: 1, Name: "req-name"})
	if err != nil {
		t.Fatal(err)
	}
	return plan.ID, repo
}

func TestGetReturnsPlan(t *testing.T) {
	planID, repo := previewForGetTest(t)
	svc := NewService(newKubeStub(), repo)
	got, err := svc.Get(context.Background(), planID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != planID {
		t.Fatalf("ID = %q, want %q", got.ID, planID)
	}
}

func TestGetReturnsErrNotFound(t *testing.T) {
	svc := NewService(newKubeStub(), newRepoStub())
	_, err := svc.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListReturnsPlans(t *testing.T) {
	planID, repo := previewForGetTest(t)
	svc := NewService(newKubeStub(), repo)
	plans, err := svc.List(context.Background(), 7, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].ID != planID {
		t.Fatalf("plans = %d, first ID=%v", len(plans), plans)
	}
}

func TestListRejectsInvalidClusterID(t *testing.T) {
	svc := NewService(newKubeStub(), newRepoStub())
	_, err := svc.List(context.Background(), 0, "")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
}
