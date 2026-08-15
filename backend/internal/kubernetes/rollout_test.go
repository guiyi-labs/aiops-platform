package kubernetes

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"k8s-aiops.local/backend/internal/apiquery"
)

// --- M115-1c: ReplicaSetsByOwner / RolloutHistory / RolloutStatus / PatchNode ---

func TestReplicaSetsByOwnerFiltersByOwnerUIDAndKind(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"items":[
		{"metadata":{"name":"api-v2","uid":"rs-2","resourceVersion":"rv-2","namespace":"demo","ownerReferences":[{"kind":"Deployment","name":"api","uid":"dep-1"}]}},
		{"metadata":{"name":"api-v1","uid":"rs-1","resourceVersion":"rv-1","namespace":"demo","ownerReferences":[{"kind":"ReplicaSet","name":"other","uid":"dep-1"}]}},
		{"metadata":{"name":"other-rs","uid":"rs-9","namespace":"demo","ownerReferences":[{"kind":"Deployment","name":"other","uid":"dep-2"}]}}
	]}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	items, err := service.ReplicaSetsByOwner(context.Background(), 7, "demo", "dep-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Metadata.Name != "api-v2" {
		t.Fatalf("owned = %d items (first name %s), want 1 (api-v2)", len(items), items[0].Metadata.Name)
	}
	if gateway.path != "/apis/apps/v1/namespaces/demo/replicasets" {
		t.Fatalf("path=%q", gateway.path)
	}
}

func TestRolloutHistoryBuildsSortedRevisions(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/apps/v1/namespaces/demo/deployments/api": {
			body: []byte(`{"metadata":{"name":"api","namespace":"demo","uid":"dep-1","annotations":{"deployment.kubernetes.io/revision":"2"}}}`),
		},
		"/apis/apps/v1/namespaces/demo/replicasets": {
			body: []byte(`{"items":[
				{"metadata":{"name":"api-v2","uid":"rs-2","resourceVersion":"rv-2","namespace":"demo","annotations":{"deployment.kubernetes.io/revision":"2"},"ownerReferences":[{"kind":"Deployment","name":"api","uid":"dep-1"}]},"spec":{"template":{"spec":{"containers":[{"name":"api","image":"api:v2"}]}}},"status":{"replicas":3,"readyReplicas":3,"availableReplicas":3}},
				{"metadata":{"name":"api-v1","uid":"rs-1","resourceVersion":"rv-1","namespace":"demo","annotations":{"deployment.kubernetes.io/revision":"1"},"ownerReferences":[{"kind":"Deployment","name":"api","uid":"dep-1"}]},"spec":{"template":{"spec":{"containers":[{"name":"api","image":"api:v1"}]}}},"status":{"replicas":0,"readyReplicas":0,"availableReplicas":0}}
			]}`),
		},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	history, err := service.RolloutHistory(context.Background(), 7, "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if history.CurrentRevision != 2 || len(history.Revisions) != 2 {
		t.Fatalf("current=%d revisions=%d, want 2/2", history.CurrentRevision, len(history.Revisions))
	}
	if history.Revisions[0].Revision != 1 || history.Revisions[1].Revision != 2 {
		t.Fatalf("revisions not sorted by revision: %d %d", history.Revisions[0].Revision, history.Revisions[1].Revision)
	}
	if history.Revisions[1].Current != true || history.Revisions[0].Current != false {
		t.Fatalf("Current flags wrong: %v %v", history.Revisions[0].Current, history.Revisions[1].Current)
	}
	if len(history.Revisions[1].Images) != 1 || history.Revisions[1].Images[0] != "api:v2" {
		t.Fatalf("images = %v", history.Revisions[1].Images)
	}
}

func TestRolloutHistorySkipsZeroRevisionReplicaSets(t *testing.T) {
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/apps/v1/namespaces/demo/deployments/api": {
			body: []byte(`{"metadata":{"name":"api","namespace":"demo","uid":"dep-1","annotations":{"deployment.kubernetes.io/revision":"1"}}}`),
		},
		"/apis/apps/v1/namespaces/demo/replicasets": {
			body: []byte(`{"items":[
				{"metadata":{"name":"legacy","uid":"rs-0","namespace":"demo","ownerReferences":[{"kind":"Deployment","name":"api","uid":"dep-1"}]},"status":{"replicas":1}}
			]}`),
		},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	history, err := service.RolloutHistory(context.Background(), 7, "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Revisions) != 0 {
		t.Fatalf("revisions = %d, want 0 (zero-revision rs skipped)", len(history.Revisions))
	}
}

func TestRolloutStatusPopulatesPhaseAndDefaults(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"api","namespace":"demo","uid":"dep-1","annotations":{"deployment.kubernetes.io/revision":"3"}},"spec":{"replicas":3},"status":{"updatedReplicas":2,"readyReplicas":2,"availableReplicas":2,"unavailableReplicas":1,"conditions":[{"type":"Progressing","status":"False","reason":"ProgressDeadlineExceeded","message":"deployment exceeded its progress deadline","lastTransitionTime":"2026-08-01T00:00:00Z"}]}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	status, err := service.RolloutStatus(context.Background(), 7, "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if status.CurrentRevision != 3 || status.DesiredReplicas != 3 {
		t.Fatalf("revision=%d desired=%d", status.CurrentRevision, status.DesiredReplicas)
	}
	if status.Phase == "" {
		t.Fatal("phase is empty for ProgressDeadlineExceeded condition")
	}
	if status.Reason == "" || status.Message == "" {
		t.Fatalf("reason/message missing: %q %q", status.Reason, status.Message)
	}
}

func TestRolloutStatusDefaultsDesiredReplicasTo1(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"api","namespace":"demo","uid":"dep-1"}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	status, err := service.RolloutStatus(context.Background(), 7, "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredReplicas != 1 {
		t.Fatalf("DesiredReplicas=%d, want 1", status.DesiredReplicas)
	}
	if status.Conditions == nil {
		t.Fatal("Conditions is nil, want empty slice")
	}
}

func TestRolloutHistoryRejectsEmptyUID(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"api"}}`)} // no uid
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.RolloutHistory(context.Background(), 7, "demo", "api")
	if err == nil {
		t.Fatal("expected ErrResourceNotFound for empty UID")
	}
}

func TestPatchNodeSuccess(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"worker-1","uid":"node-1","resourceVersion":"44"}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	patch := []byte(`{"spec":{"unschedulable":true}}`)
	item, err := service.PatchNode(context.Background(), 7, "worker-1", patch, false)
	if err != nil {
		t.Fatal(err)
	}
	if item.Metadata.ResourceVersion != "44" || gateway.path != "/api/v1/nodes/worker-1" {
		t.Fatalf("item=%#v path=%q", item, gateway.path)
	}
	if gateway.patchType != "application/strategic-merge-patch+json" {
		t.Fatalf("patchType=%q", gateway.patchType)
	}
}

func TestPatchNodeDryRunSetsQuery(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.PatchNode(context.Background(), 7, "worker-1", []byte(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.query.Get("dryRun") != "All" {
		t.Fatalf("dryRun query = %q", gateway.query.Get("dryRun"))
	}
}

func TestPatchNodeNonPatchGatewayRejected(t *testing.T) {
	// gatewayStub implements PatchGateway, so wrap a getter-only fake.
	service := NewService(credentialStub{enabled: true}, getterOnlyGateway{}, nil)
	_, err := service.PatchNode(context.Background(), 7, "worker-1", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected error for non-PatchGateway")
	}
}

type getterOnlyGateway struct{}

func (getterOnlyGateway) Get(context.Context, int64, []byte, string, url.Values, int64) ([]byte, error) {
	return nil, nil
}

func TestPatchNodeDisabledCluster(t *testing.T) {
	service := NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err := service.PatchNode(context.Background(), 7, "worker-1", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected ErrDisabled")
	}
}

func TestWorkloadTemplateUnmarshalJSON(t *testing.T) {
	var template WorkloadTemplate
	if err := json.Unmarshal([]byte(`{"spec":{"containers":[{"name":"web","image":"web:v1"}]}}`), &template); err != nil {
		t.Fatal(err)
	}
	if len(template.Spec.Containers) != 1 || template.Spec.Containers[0].Name != "web" {
		t.Fatalf("containers = %+v", template.Spec.Containers)
	}
	if !strings.Contains(string(template.Raw), "web:v1") {
		t.Fatalf("Raw not preserved: %s", template.Raw)
	}
}

// --- Error branches for list functions (Namespaces, Nodes, StatefulSets, etc.) ---

func TestNamespacesErrorBranches(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.Namespaces(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected gateway error")
	}
	// Disabled cluster
	service = NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err = service.Namespaces(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
}

func TestNodesErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.Nodes(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestDeploymentsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.Deployments(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStatefulSetsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.StatefulSets(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDaemonSetsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.DaemonSets(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReplicaSetsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.ReplicaSets(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJobsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.Jobs(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestCronJobsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.CronJobs(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHPAListErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.HorizontalPodAutoscalers(context.Background(), 7, "default", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPatchDeploymentErrorBranches(t *testing.T) {
	// Disabled cluster
	service := NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err := service.PatchDeployment(context.Background(), 7, "ns", "app", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
	// Gateway error
	service = NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err = service.PatchDeployment(context.Background(), 7, "ns", "app", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestPatchCronJobErrorBranches(t *testing.T) {
	// Disabled cluster
	service := NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err := service.PatchCronJob(context.Background(), 7, "ns", "job", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
	// Gateway error
	service = NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err = service.PatchCronJob(context.Background(), 7, "ns", "job", []byte(`{}`), false)
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestReplicaSetsByOwnerGatewayError(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.ReplicaSetsByOwner(context.Background(), 7, "ns", "dep-1")
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestRolloutHistoryGatewayError(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.RolloutHistory(context.Background(), 7, "ns", "api")
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestRolloutStatusGatewayError(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.RolloutStatus(context.Background(), 7, "ns", "api")
	if err == nil {
		t.Fatal("expected gateway error")
	}
}

func TestNamespacesDisabledCluster(t *testing.T) {
	service := NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err := service.Namespaces(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
}

func TestDeploymentsDisabledCluster(t *testing.T) {
	service := NewService(credentialStub{enabled: false}, &gatewayStub{}, nil)
	_, err := service.Deployments(context.Background(), 7, "ns", apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error for disabled cluster")
	}
}

func TestNodeMetricsErrorBranch(t *testing.T) {
	service := NewService(credentialStub{enabled: true}, &gatewayStub{err: context.DeadlineExceeded}, nil)
	_, err := service.NodeMetrics(context.Background(), 7, apiquery.ListQuery{Page: 1, Limit: 20})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRolloutStatusRejectsEmptyUID(t *testing.T) {
	gateway := &gatewayStub{body: []byte(`{"metadata":{"name":"api"}}`)}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.RolloutStatus(context.Background(), 7, "demo", "api")
	if err == nil {
		t.Fatal("expected ErrResourceNotFound for empty UID")
	}
}

func TestRolloutHistoryReplicaSetsGatewayError(t *testing.T) {
	// Deployment succeeds but ReplicaSetsByOwner fails
	gateway := &gatewayStub{responses: map[string]gatewayResponse{
		"/apis/apps/v1/namespaces/demo/deployments/api": {
			body: []byte(`{"metadata":{"name":"api","uid":"dep-1"}}`),
		},
		"/apis/apps/v1/namespaces/demo/replicasets": {
			err: context.DeadlineExceeded,
		},
	}}
	service := NewService(credentialStub{enabled: true}, gateway, nil)
	_, err := service.RolloutHistory(context.Background(), 7, "demo", "api")
	if err == nil {
		t.Fatal("expected ReplicaSets gateway error")
	}
}
