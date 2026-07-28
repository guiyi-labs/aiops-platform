package diagnosis

import (
	"encoding/json"
	"testing"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func TestEvaluateImagePullBackOff(t *testing.T) {
	pod := k8sgateway.Pod{}
	pod.Metadata.Name = "broken-api"
	pod.Metadata.Namespace = "demo"
	pod.Metadata.UID = "pod-1"
	pod.Spec.Containers = append(pod.Spec.Containers, struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}{Name: "app", Image: "registry.example/missing:v9"})
	pod.Status.ContainerStatuses = []k8sgateway.ContainerStatus{{Name: "app", RestartCount: 2, State: k8sgateway.ContainerState{Waiting: &k8sgateway.ContainerStateDetail{Reason: "ImagePullBackOff", Message: "Back-off pulling image"}}}}
	event := k8sgateway.Event{Type: "Warning", Reason: "Failed", Message: "Failed to pull image", Count: 3}
	event.Metadata.Name = "pull-failed"
	record, matched := EvaluateImagePullBackOff(7, pod, []k8sgateway.Event{event}, time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC))
	if !matched {
		t.Fatal("rule did not match")
	}
	if record.RuleID != RuleImagePullBackOff || record.Resource.Name != "broken-api" || len(record.Evidence) != 2 {
		t.Fatalf("record = %#v", record)
	}
	if record.Evidence[0].Content["image"] != "registry.example/missing:v9" {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestSpecificContainerFailurePrecedesPendingPhase(t *testing.T) {
	pod := k8sgateway.Pod{}
	pod.Metadata.Name = "broken-api"
	pod.Metadata.Namespace = "demo"
	pod.Status.Phase = "Pending"
	pod.Status.ContainerStatuses = []k8sgateway.ContainerStatus{{
		Name:  "app",
		State: k8sgateway.ContainerState{Waiting: &k8sgateway.ContainerStateDetail{Reason: "ImagePullBackOff"}},
	}}
	record, matched := evaluatePod(7, pod, nil, time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC))
	if !matched || record.RuleID != RuleImagePullBackOff {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
}

func TestEvaluateCrashLoopBackOff(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{
		"metadata":{"name":"crashing-api","namespace":"demo","uid":"pod-2"},
		"status":{"containerStatuses":[{"name":"app","restartCount":8,"state":{"waiting":{"reason":"CrashLoopBackOff","message":"back-off 5m"}},"lastState":{"terminated":{"reason":"Error","exitCode":1,"finishedAt":"2026-07-17T02:00:00Z"}}}]}}
	`, &pod)
	event := k8sgateway.Event{Type: "Warning", Reason: "BackOff", Message: "Back-off restarting failed container", Count: 8}
	event.Metadata.Name = "restart-backoff"
	record, matched := EvaluateCrashLoopBackOff(7, pod, []k8sgateway.Event{event}, time.Date(2026, 7, 17, 2, 1, 0, 0, time.UTC))
	if !matched || record.RuleID != RuleCrashLoopBackOff || record.Severity != "high" {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	if len(record.Evidence) != 2 || record.Evidence[0].Content["restart_count"] != float64(8) && record.Evidence[0].Content["restart_count"] != int32(8) {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestEvaluatePodPending(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{
		"metadata":{"name":"pending-api","namespace":"demo","uid":"pod-pending"},
		"status":{"phase":"Pending","reason":"Unschedulable","message":"0/1 nodes are available","conditions":[{"type":"PodScheduled","status":"False","reason":"Unschedulable","message":"Insufficient cpu"}]}}
	`, &pod)
	event := k8sgateway.Event{Type: "Warning", Reason: "FailedScheduling", Message: "0/1 nodes are available: insufficient cpu", Count: 4}
	event.Metadata.Name = "pending-schedule"
	record, matched := EvaluatePodPending(7, pod, []k8sgateway.Event{event}, time.Date(2026, 7, 17, 2, 2, 0, 0, time.UTC))
	if !matched || record.RuleID != RulePodPending || record.Severity != "high" || len(record.Evidence) != 3 {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	if record.Evidence[1].Content["reason"] != "Unschedulable" {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestEvaluatePodOOMKilled(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{
		"metadata":{"name":"memory-api","namespace":"demo","uid":"pod-oom"},
		"status":{"phase":"Running","containerStatuses":[{"name":"app","restartCount":3,"lastState":{"terminated":{"reason":"OOMKilled","exitCode":137,"finishedAt":"2026-07-17T02:00:00Z"}}}]}}
	`, &pod)
	event := k8sgateway.Event{Type: "Warning", Reason: "OOMKilled", Message: "container was killed due to out of memory", Count: 2}
	event.Metadata.Name = "oom-killed"
	record, matched := EvaluatePodOOMKilled(7, pod, []k8sgateway.Event{event}, time.Date(2026, 7, 17, 2, 1, 0, 0, time.UTC))
	if !matched || record.RuleID != RulePodOOMKilled || record.Severity != "critical" || len(record.Evidence) != 2 {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	if record.Evidence[0].Content["exit_code"] != int32(137) && record.Evidence[0].Content["exit_code"] != float64(137) {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestEvaluatePodPendingDoesNotMatchRunningPod(t *testing.T) {
	var pod k8sgateway.Pod
	pod.Status.Phase = "Running"
	if _, matched := EvaluatePodPending(1, pod, nil, time.Now()); matched {
		t.Fatal("running Pod matched pending rule")
	}
}

func TestEvaluateCrashLoopBackOffDoesNotMatchTerminatedJob(t *testing.T) {
	var pod k8sgateway.Pod
	mustDecode(t, `{"metadata":{"name":"finished"},"status":{"phase":"Succeeded","containerStatuses":[{"name":"job","restartCount":0,"state":{"terminated":{"reason":"Completed","exitCode":0}}}]}}`, &pod)
	if _, matched := EvaluateCrashLoopBackOff(1, pod, nil, time.Now()); matched {
		t.Fatal("completed Pod matched rule")
	}
}

func TestEvaluateServiceNoEndpoints(t *testing.T) {
	var service k8sgateway.ServiceResource
	var endpoints k8sgateway.Endpoints
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"service-1"},"spec":{"type":"ClusterIP","selector":{"app":"api"},"ports":[{"port":80,"targetPort":8080,"protocol":"TCP"}]}}`, &service)
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"subsets":[{"notReadyAddresses":[{"ip":"10.0.0.9"}]}]}`, &endpoints)
	record, matched := EvaluateServiceNoEndpoints(7, service, endpoints, time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC))
	if !matched || record.RuleID != RuleServiceNoEndpoints || record.Resource.Kind != "Service" || len(record.Evidence) != 2 {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	content := record.Evidence[1].Content
	if content["ready_addresses"] != 0 || content["not_ready_addresses"] != 1 || content["source_api"] != "core/v1" {
		t.Fatalf("endpoint evidence = %#v", content)
	}
}

func TestEvaluateServiceNoEndpointsPreservesEndpointSliceSource(t *testing.T) {
	var service k8sgateway.ServiceResource
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"spec":{"selector":{"app":"api"}}}`, &service)
	endpoints := k8sgateway.Endpoints{SourceAPI: "discovery.k8s.io/v1", Subsets: []k8sgateway.EndpointSubset{{NotReadyAddresses: []k8sgateway.EndpointAddress{{IP: "10.0.0.9"}}}}}
	record, matched := EvaluateServiceNoEndpoints(7, service, endpoints, time.Now())
	if !matched || record.Evidence[1].Content["source_api"] != "discovery.k8s.io/v1" || record.Evidence[1].Content["subset_count"] != 1 {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
}

func TestEvaluateServiceNoEndpointsSkipsExpectedCases(t *testing.T) {
	for _, raw := range []string{
		`{"metadata":{"name":"external"},"spec":{"externalName":"database.example.com"}}`,
		`{"metadata":{"name":"manual"},"spec":{"type":"ClusterIP"}}`,
		`{"metadata":{"name":"ready"},"spec":{"selector":{"app":"api"}}}`,
	} {
		var service k8sgateway.ServiceResource
		var endpoints k8sgateway.Endpoints
		mustDecode(t, raw, &service)
		if service.Metadata.Name == "ready" {
			mustDecode(t, `{"subsets":[{"addresses":[{"ip":"10.0.0.8"}]}]}`, &endpoints)
		}
		if _, matched := EvaluateServiceNoEndpoints(1, service, endpoints, time.Now()); matched {
			t.Fatalf("service %q unexpectedly matched", service.Metadata.Name)
		}
	}
}

func TestEvaluateNodeNotReady(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1","uid":"node-1"},"status":{"conditions":[{"type":"Ready","status":"False","reason":"KubeletNotReady","message":"runtime is unavailable","lastTransitionTime":"2026-07-26T10:00:00Z"},{"type":"MemoryPressure","status":"True","reason":"KubeletHasInsufficientMemory","message":"memory pressure","lastTransitionTime":"2026-07-26T10:01:00Z"}]}}`, &node)
	record, matched := EvaluateNodeNotReady(7, node, time.Date(2026, 7, 26, 10, 2, 0, 0, time.UTC))
	if !matched || record.RuleID != RuleNodeNotReady || record.Resource.Kind != "Node" || record.Severity != "critical" || len(record.Evidence) != 2 {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	if record.Evidence[0].Content["reason"] != "KubeletNotReady" || record.Evidence[0].Content["last_transition_time"] != "2026-07-26T10:00:00Z" {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
}

func TestEvaluateNodeNotReadySkipsReadyNode(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[{"type":"Ready","status":"True"}]}}`, &node)
	if _, matched := EvaluateNodeNotReady(1, node, time.Now()); matched {
		t.Fatal("ready Node matched not-ready rule")
	}
}

func TestEvaluateNodeNotReadyMatchesMissingReadyCondition(t *testing.T) {
	var node k8sgateway.Node
	mustDecode(t, `{"metadata":{"name":"worker-1"},"status":{"conditions":[]}}`, &node)
	record, matched := EvaluateNodeNotReady(1, node, time.Now())
	if !matched || len(record.Evidence) != 1 || record.Evidence[0].Content["reason"] != "ReadyConditionMissing" {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
}

func TestEvaluateDeploymentReplicasUnavailable(t *testing.T) {
	var deployment k8sgateway.Deployment
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo","uid":"deployment-1"},"spec":{"replicas":3},"status":{"replicas":2,"readyReplicas":1,"availableReplicas":1,"updatedReplicas":2,"unavailableReplicas":1}}`, &deployment)
	record, matched := EvaluateDeploymentReplicasUnavailable(7, deployment, time.Date(2026, 7, 26, 10, 3, 0, 0, time.UTC))
	if !matched || record.RuleID != RuleDeploymentReplicasUnavailable || record.Resource.Namespace != "demo" || record.Severity != "high" {
		t.Fatalf("record = %#v, matched = %v", record, matched)
	}
	content := record.Evidence[0].Content
	if content["desired_replicas"] != int32(3) && content["desired_replicas"] != float64(3) {
		t.Fatalf("evidence = %#v", content)
	}
}

func TestEvaluateDeploymentReplicasUnavailableUsesKubernetesDefault(t *testing.T) {
	var deployment k8sgateway.Deployment
	mustDecode(t, `{"metadata":{"name":"api","namespace":"demo"},"status":{"readyReplicas":0,"availableReplicas":0}}`, &deployment)
	if _, matched := EvaluateDeploymentReplicasUnavailable(1, deployment, time.Now()); !matched {
		t.Fatal("deployment with omitted replicas should use Kubernetes default of one")
	}
	deployment.Status.ReadyReplicas = 1
	deployment.Status.AvailableReplicas = 1
	if _, matched := EvaluateDeploymentReplicasUnavailable(1, deployment, time.Now()); matched {
		t.Fatal("healthy deployment matched unavailable rule")
	}
	var scaledToZero k8sgateway.Deployment
	mustDecode(t, `{"metadata":{"name":"idle","namespace":"demo"},"spec":{"replicas":0},"status":{}}`, &scaledToZero)
	if _, matched := EvaluateDeploymentReplicasUnavailable(1, scaledToZero, time.Now()); matched {
		t.Fatal("deployment intentionally scaled to zero matched unavailable rule")
	}
}

func mustDecode(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateImagePullBackOffDoesNotMatchRunningPod(t *testing.T) {
	pod := k8sgateway.Pod{}
	pod.Metadata.Name = "healthy"
	pod.Status.Phase = "Running"
	if _, matched := EvaluateImagePullBackOff(1, pod, nil, time.Now()); matched {
		t.Fatal("healthy Pod matched rule")
	}
}
