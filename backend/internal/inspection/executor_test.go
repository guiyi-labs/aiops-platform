package inspection

// Covers the DefaultExecutor: Execute dispatch, every rule function and the
// error paths, using a fake read-only gateway that serves canned Kubernetes
// list responses (no cluster access, ADR 0004).

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"k8s-aiops.local/backend/internal/cluster"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

type fakeCreds struct{}

func (fakeCreds) Access(context.Context, int64) (cluster.Cluster, []byte, error) {
	return cluster.Cluster{}, []byte("stub"), nil
}

type fakeGateway struct {
	byPath map[string]string
	err    error
}

func (f *fakeGateway) Get(_ context.Context, _ int64, _ []byte, path string, _ url.Values, _ int64) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	body, ok := f.byPath[path]
	if !ok {
		return nil, errors.New("fake gateway: unexpected path " + path)
	}
	return []byte(body), nil
}

func mustExec(t *testing.T, g *fakeGateway) *DefaultExecutor {
	t.Helper()
	return NewDefaultExecutor(k8sgateway.NewService(fakeCreds{}, g, nil))
}

const nodesJSON = `{"items":[
	{"metadata":{"name":"worker-bad","uid":"n1"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"KubeletNotReady","message":"node down"},{"type":"MemoryPressure","status":"True","reason":"KubeletHasInsufficientMemory","message":"low mem"}]}},
	{"metadata":{"name":"worker-ok","uid":"n2"},"status":{"conditions":[{"type":"Ready","status":"True","reason":"KubeletReady"}]}}
]}`

const podsJSON = `{"items":[
  {"metadata":{"name":"p-restart","namespace":"ns1","uid":"u1"},"status":{"containerStatuses":[{"name":"app","restartCount":7,"ready":true}]}},
  {"metadata":{"name":"p-ok","namespace":"ns1","uid":"u2"},"status":{"phase":"Running"}},
  {"metadata":{"name":"p-pending","namespace":"ns1","uid":"u3","creationTimestamp":"2026-07-01T00:00:00Z"},"status":{"phase":"Pending","reason":"Unschedulable","message":"0/1 nodes"}},
  {"metadata":{"name":"p-oom","namespace":"ns1","uid":"u4"},"status":{"phase":"Running","containerStatuses":[{"name":"app","restartCount":3,"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137}}}]}}
]}`

func ruleFor(code string) RuleDescriptor {
	return RuleDescriptor{Code: code, SignalCode: "signal." + code, Timeout: 5_000_000_000}
}

func TestExecuteNodeNotReady(t *testing.T) {
	exec := mustExec(t, &fakeGateway{byPath: map[string]string{"/api/v1/nodes": nodesJSON}})
	findings, err := exec.Execute(context.Background(), 1, ruleFor("node_not_ready"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].ResourceName != "worker-bad" {
		t.Fatalf("findings = %+v", findings)
	}
	if findings[0].Evidence["reason"] != "KubeletNotReady" {
		t.Errorf("evidence = %+v", findings[0].Evidence)
	}
}

func TestExecuteNodePressure(t *testing.T) {
	g := mustExec(t, &fakeGateway{byPath: map[string]string{"/api/v1/nodes": nodesJSON}})
	findings, err := g.Execute(context.Background(), 1, ruleFor("node_pressure"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].ResourceName != "worker-bad" {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestExecutePodRules(t *testing.T) {
	g := mustExec(t, &fakeGateway{byPath: map[string]string{"/api/v1/pods": podsJSON}})

	restarts, err := g.Execute(context.Background(), 1, ruleFor("pod_restart_loop"))
	if err != nil || len(restarts) != 1 || restarts[0].ResourceName != "p-restart" {
		t.Fatalf("pod_restart_loop = %+v, %v", restarts, err)
	}
	pending, err := g.Execute(context.Background(), 1, ruleFor("pod_pending"))
	if err != nil || len(pending) != 1 || pending[0].ResourceName != "p-pending" {
		t.Fatalf("pod_pending = %+v, %v", pending, err)
	}
	ooms, err := g.Execute(context.Background(), 1, ruleFor("container_oom_killed"))
	if err != nil || len(ooms) != 1 || ooms[0].ResourceName != "p-oom" {
		t.Fatalf("container_oom_killed = %+v, %v", ooms, err)
	}
}

func TestExecuteWorkloadReplicas(t *testing.T) {
	g := mustExec(t, &fakeGateway{byPath: map[string]string{
		"/apis/apps/v1/deployments": `{"items":[
		  {"metadata":{"name":"d-bad","namespace":"ns1","uid":"d1"},"spec":{"replicas":2},"status":{"unavailableReplicas":1,"readyReplicas":1,"availableReplicas":1}},
		  {"metadata":{"name":"d-ok","namespace":"ns1","uid":"d2"},"spec":{"replicas":1},"status":{"readyReplicas":1,"availableReplicas":1}}
		]}`,
		"/apis/apps/v1/statefulsets": `{"items":[
		  {"metadata":{"name":"s-bad","namespace":"ns1","uid":"s1"},"spec":{"replicas":2},"status":{"readyReplicas":0}}
		]}`,
	}})
	findings, err := g.Execute(context.Background(), 1, ruleFor("workload_replicas_unavailable"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want 2", findings)
	}
}

func TestExecutePVCPending(t *testing.T) {
	g := mustExec(t, &fakeGateway{byPath: map[string]string{"/api/v1/persistentvolumeclaims": `{"items":[
	  {"metadata":{"name":"pvc-bad","namespace":"ns1","uid":"pv1","creationTimestamp":"2026-07-01T00:00:00Z"},"status":{"phase":"Pending"},"spec":{"storageClassName":"fast"}}
	]}`}})
	findings, err := g.Execute(context.Background(), 1, ruleFor("pvc_pending"))
	if err != nil || len(findings) != 1 || findings[0].ResourceName != "pvc-bad" {
		t.Fatalf("pvc_pending = %+v, %v", findings, err)
	}
}

func TestExecuteIngressBackend(t *testing.T) {
	g := mustExec(t, &fakeGateway{byPath: map[string]string{
		"/apis/networking.k8s.io/v1/ingresses": `{"items":[
		  {"metadata":{"name":"i1","namespace":"demo","uid":"i1"},"spec":{"rules":[{"host":"h.test","http":{"paths":[{"path":"/","backend":{"service":{"name":"backend","port":{"number":80}}}}]}}]}}
		]}`,
		"/apis/discovery.k8s.io/v1/namespaces/demo/endpointslices": `{"items":[]}`,
	}})
	findings, err := g.Execute(context.Background(), 1, ruleFor("ingress_backend_unhealthy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].ResourceName != "i1" {
		t.Fatalf("findings = %+v", findings)
	}
	if findings[0].Evidence["service_name"] != "backend" {
		t.Errorf("evidence = %+v", findings[0].Evidence)
	}
}

func TestExecuteUnknownCodeAndNilGateway(t *testing.T) {
	exec := NewDefaultExecutor(k8sgateway.NewService(fakeCreds{}, &fakeGateway{}, nil))
	if _, err := exec.Execute(context.Background(), 1, ruleFor("nope")); !errors.Is(err, ErrInvalidRuleCode) {
		t.Errorf("unknown rule err = %v, want ErrInvalidRuleCode", err)
	}
	if _, err := (&DefaultExecutor{}).Execute(context.Background(), 1, ruleFor("node_not_ready")); err == nil {
		t.Error("nil gateway should fail closed")
	}
}

func TestExecuteGatewayErrorPropagates(t *testing.T) {
	g := mustExec(t, &fakeGateway{err: errors.New("cluster unreachable")})
	if _, err := g.Execute(context.Background(), 1, ruleFor("node_not_ready")); err == nil {
		t.Error("gateway error should be propagated")
	}
}
