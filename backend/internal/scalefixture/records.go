package scalefixture

import (
	"fmt"
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

var workloadPrefixes = []string{"api", "checkout", "payments", "catalog", "telemetry", "gateway", "search", "worker"}

type WorkloadRecord struct {
	Deployment k8sgateway.Deployment      `json:"deployment"`
	ReplicaSet k8sgateway.ReplicaSet      `json:"replica_set"`
	Service    k8sgateway.ServiceResource `json:"service"`
	Ingress    k8sgateway.Ingress         `json:"ingress"`
}

type HistorySample struct {
	ClusterID          int64     `json:"cluster_id"`
	ResourceKind       string    `json:"resource_kind"`
	ResourceNamespace  string    `json:"resource_namespace,omitempty"`
	ResourceName       string    `json:"resource_name"`
	ResourceUID        string    `json:"resource_uid"`
	ContainerName      string    `json:"container_name,omitempty"`
	MetricName         string    `json:"metric_name"`
	Value              int64     `json:"value"`
	Unit               string    `json:"unit"`
	SourceTimestamp    time.Time `json:"source_timestamp"`
	WindowMilliseconds int       `json:"window_milliseconds"`
}

func (s HistorySample) Input() metricshistory.SampleInput {
	return metricshistory.SampleInput{
		ResourceKind: s.ResourceKind, ResourceNamespace: s.ResourceNamespace,
		ResourceName: s.ResourceName, ResourceUID: s.ResourceUID,
		ContainerName: s.ContainerName, MetricName: s.MetricName, Value: s.Value,
		SourceTimestamp: s.SourceTimestamp, Window: time.Duration(s.WindowMilliseconds) * time.Millisecond,
	}
}

func Node(c Config, index int) k8sgateway.Node {
	name := nodeName(index)
	node := k8sgateway.Node{Metadata: metadata(c, name, uid("node", c, index), "", index)}
	node.Metadata.Labels = map[string]string{"topology.kubernetes.io/zone": fmt.Sprintf("zone-%02d", index%12), "fixture.aiops.dev/version": c.DatasetVersion}
	node.Status.NodeInfo.KubeletVersion = "v1.36.0-fixture"
	node.Status.NodeInfo.OSImage = "Linux fixture"
	node.Status.NodeInfo.ContainerRuntimeVersion = "containerd://2.0-fixture"
	node.Status.Capacity = map[string]string{"cpu": "16", "memory": "64Gi", "pods": "110"}
	node.Status.Allocatable = map[string]string{"cpu": "15500m", "memory": "60Gi", "pods": "110"}
	return node
}

func Workload(c Config, index int) WorkloadRecord {
	namespace, name := workloadIdentity(c, index)
	deploymentUID := uid("deployment", c, index)
	replicaSetUID := uid("replicaset", c, index)
	serviceUID := uid("service", c, index)
	ingressUID := uid("ingress", c, index)
	labels := map[string]string{"app": name, "fixture.aiops.dev/version": c.DatasetVersion}
	controller := true
	replicas := int32(c.PodsPerWorkload)
	container := k8sgateway.WorkloadContainer{Name: "app", Image: "registry.example.invalid/fixture/" + name + ":v1"}

	deployment := k8sgateway.Deployment{Metadata: metadata(c, name, deploymentUID, namespace, index)}
	deployment.Metadata.Labels = cloneLabels(labels)
	deployment.Spec.Replicas = &replicas
	deployment.Spec.Selector.MatchLabels = map[string]string{"app": name}
	deployment.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{container}
	deployment.Status.Replicas = replicas
	deployment.Status.ReadyReplicas = replicas
	deployment.Status.AvailableReplicas = replicas
	deployment.Status.UpdatedReplicas = replicas

	replicaSet := k8sgateway.ReplicaSet{Metadata: metadata(c, name+"-rs", replicaSetUID, namespace, index)}
	replicaSet.Metadata.Labels = cloneLabels(labels)
	replicaSet.Metadata.OwnerReferences = []k8sgateway.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, UID: deploymentUID, Controller: &controller}}
	replicaSet.Spec.Replicas = &replicas
	replicaSet.Spec.Selector.MatchLabels = map[string]string{"app": name}
	replicaSet.Spec.Template.Spec.Containers = []k8sgateway.WorkloadContainer{container}
	replicaSet.Status.Replicas = replicas
	replicaSet.Status.ReadyReplicas = replicas
	replicaSet.Status.AvailableReplicas = replicas
	replicaSet.Status.FullyLabeled = replicas

	serviceName := name + "-svc"
	service := k8sgateway.ServiceResource{Metadata: metadata(c, serviceName, serviceUID, namespace, index)}
	service.Metadata.Labels = cloneLabels(labels)
	service.Spec.Type = "ClusterIP"
	service.Spec.ClusterIP = clusterIP(index)
	service.Spec.Selector = map[string]string{"app": name}

	backend := &k8sgateway.IngressBackend{Service: &k8sgateway.IngressServiceBackend{Name: serviceName}}
	backend.Service.Port.Number = 80
	ingress := k8sgateway.Ingress{Metadata: metadata(c, name+"-ing", ingressUID, namespace, index)}
	ingress.Metadata.Labels = cloneLabels(labels)
	ingress.Spec.IngressClassName = "nginx"
	ingress.Spec.DefaultBackend = backend

	return WorkloadRecord{Deployment: deployment, ReplicaSet: replicaSet, Service: service, Ingress: ingress}
}

func Pod(c Config, index int) k8sgateway.Pod {
	namespace, workloadName := workloadIdentity(c, index/c.PodsPerWorkload)
	name := fmt.Sprintf("%s-pod-%02d", workloadName, index%c.PodsPerWorkload)
	workload := Workload(c, index/c.PodsPerWorkload)
	controller := true
	pod := k8sgateway.Pod{Metadata: metadata(c, name, uid("pod", c, index), namespace, index)}
	pod.Metadata.Labels = map[string]string{"app": workloadName, "fixture.aiops.dev/version": c.DatasetVersion, "fixture.aiops.dev/search": searchTerm(index)}
	pod.Metadata.OwnerReferences = []k8sgateway.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: workload.ReplicaSet.Metadata.Name, UID: workload.ReplicaSet.Metadata.UID, Controller: &controller}}
	pod.Spec.NodeName = nodeName(index % c.NodeCount)
	pod.Spec.Containers = []k8sgateway.PodContainer{{Name: "app", Image: "registry.example.invalid/fixture/" + workloadName + ":v1"}}
	pod.Status.PodIP = podIP(index)
	pod.Status.HostIP = nodeIP(index % c.NodeCount)
	pod.Status.Phase = "Running"
	ready := index%97 != 0
	pod.Status.ContainerStatuses = []k8sgateway.ContainerStatus{{Name: "app", Ready: ready, RestartCount: int32(index % 4)}}
	if !ready {
		pod.Status.Phase = "Pending"
		pod.Status.Reason = "ImagePullBackOff"
		pod.Status.Message = "deterministic scale fixture degraded pod"
		pod.Status.ContainerStatuses[0].State.Waiting = &k8sgateway.ContainerStateDetail{Reason: "ImagePullBackOff"}
	}
	return pod
}

func Event(c Config, index int) k8sgateway.Event {
	podIndex := index / 2
	pod := Pod(c, podIndex)
	event := k8sgateway.Event{Metadata: metadata(c, fmt.Sprintf("%s.%02d", pod.Metadata.Name, index%2), uid("event", c, index), pod.Metadata.Namespace, index)}
	event.InvolvedObject.Kind = "Pod"
	event.InvolvedObject.Namespace = pod.Metadata.Namespace
	event.InvolvedObject.Name = pod.Metadata.Name
	event.InvolvedObject.UID = pod.Metadata.UID
	event.Count = 1
	event.EventTime = c.ObservedAt.Add(-time.Duration(index%3600) * time.Second).UTC().Format(time.RFC3339)
	event.LastTimestamp = event.EventTime
	event.FirstTimestamp = event.EventTime
	event.ReportingComponent = "fixture-kubelet"
	event.ReportingInstance = pod.Spec.NodeName
	if index%2 == 0 {
		event.Type = "Normal"
		event.Reason = "Scheduled"
		event.Action = "Schedule"
		event.Message = "Successfully assigned deterministic fixture pod"
	} else if pod.Status.Phase == "Pending" {
		event.Type = "Warning"
		event.Reason = "BackOff"
		event.Action = "BackOff"
		event.Message = "Back-off pulling image in deterministic fixture"
	} else {
		event.Type = "Normal"
		event.Reason = "Pulled"
		event.Action = "Pull"
		event.Message = "Successfully pulled deterministic fixture image"
	}
	return event
}

func History(c Config, index int) HistorySample {
	series := index / c.HistoryPoints
	point := index % c.HistoryPoints
	metricIndex := series % 2
	resourceIndex := series / 2
	sample := HistorySample{ClusterID: c.ClusterID, MetricName: metricshistory.MetricCPU, Unit: metricshistory.UnitNanocores, WindowMilliseconds: 60000}
	if metricIndex == 1 {
		sample.MetricName = metricshistory.MetricMemory
		sample.Unit = metricshistory.UnitBytes
	}
	if resourceIndex < c.NodeCount {
		node := Node(c, resourceIndex)
		sample.ResourceKind = metricshistory.ResourceNode
		sample.ResourceName = node.Metadata.Name
		sample.ResourceUID = node.Metadata.UID
		sample.Value = historyValue(c, resourceIndex, metricIndex, point, true)
	} else {
		podIndex := resourceIndex - c.NodeCount
		pod := Pod(c, podIndex)
		sample.ResourceKind = metricshistory.ResourcePod
		sample.ResourceNamespace = pod.Metadata.Namespace
		sample.ResourceName = pod.Metadata.Name
		sample.ResourceUID = pod.Metadata.UID
		sample.ContainerName = "app"
		sample.Value = historyValue(c, podIndex, metricIndex, point, false)
	}
	sample.SourceTimestamp = c.ObservedAt.Add(-time.Duration(c.HistoryPoints-1-point) * time.Minute).UTC()
	return sample
}

func metadata(c Config, name, resourceUID, namespace string, index int) k8sgateway.ObjectMeta {
	return k8sgateway.ObjectMeta{
		Name: name, Namespace: namespace, UID: resourceUID,
		CreationTimestamp: c.ObservedAt.Add(-time.Duration(24+index%72) * time.Hour).UTC().Format(time.RFC3339),
	}
}

func workloadIdentity(c Config, index int) (string, string) {
	workloadsPerNamespace := c.WorkloadCount() / c.NamespaceCount
	namespace := fmt.Sprintf("fleet-%03d", index/workloadsPerNamespace)
	name := fmt.Sprintf("%s-%05d", workloadPrefixes[mix(c.Seed, uint64(index))%uint64(len(workloadPrefixes))], index)
	return namespace, name
}

func searchTerm(index int) string { return workloadPrefixes[index%len(workloadPrefixes)] }

func nodeName(index int) string { return fmt.Sprintf("node-%03d", index) }

func nodeIP(index int) string { return fmt.Sprintf("172.20.%d.%d", index/250, index%250+1) }

func podIP(index int) string {
	return fmt.Sprintf("10.%d.%d.%d", index/65536+1, index/256%256, index%256)
}

func clusterIP(index int) string { return fmt.Sprintf("10.96.%d.%d", index/250, index%250+1) }

func uid(kind string, c Config, index int) string {
	return fmt.Sprintf("%s-%08x-%06d", kind, uint32(mix(c.Seed, uint64(index))), index)
}

func cloneLabels(labels map[string]string) map[string]string {
	copy := make(map[string]string, len(labels))
	for key, value := range labels {
		copy[key] = value
	}
	return copy
}

func historyValue(c Config, index, metricIndex, point int, node bool) int64 {
	base := int64(mix(c.Seed, uint64(index*31+metricIndex*7+point)))
	if node {
		if metricIndex == 0 {
			return 100_000_000 + base%4_000_000_000
		}
		return 1_073_741_824 + base%34_359_738_368
	}
	if metricIndex == 0 {
		return 5_000_000 + base%500_000_000
	}
	return 33_554_432 + base%2_147_483_648
}

func mix(seed, value uint64) uint64 {
	x := seed + value + 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
