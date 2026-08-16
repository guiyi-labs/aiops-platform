package main

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

// demoCluster returns built-in demo pods and events so the CLI is usable
// without any cluster: `aiops diagnose` runs the exact same compiled-in rules
// against curated failure fixtures (crash loop, image pull back-off, OOM kill,
// pending) plus one healthy pod. Everything below is a faithful Kubernetes
// API projection using the same types the rules consume.
func demoCluster() ([]k8sgateway.Pod, map[string][]k8sgateway.Event) {
	observed := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	pods := []k8sgateway.Pod{
		demoCrashLoopPod(observed),
		demoImagePullPod(observed),
		demoOOMKilledPod(observed),
		demoPendingPod(observed),
		demoHealthyPod(observed),
	}
	events := map[string][]k8sgateway.Event{}
	for _, pod := range pods {
		events[pod.Metadata.UID] = demoEventsFor(pod, observed)
	}
	return pods, events
}

func demoEvent(name, uid, kind, namespace, reason, message string, count int32, observed time.Time) k8sgateway.Event {
	return k8sgateway.Event{
		Metadata:       k8sgateway.ObjectMeta{Name: name, Namespace: namespace, UID: "evt-" + name},
		Type:           "Warning",
		Reason:         reason,
		Message:        message,
		Count:          count,
		FirstTimestamp: observed.Format(time.RFC3339),
		LastTimestamp:  observed.Format(time.RFC3339),
		InvolvedObject: struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace,omitempty"`
			Name      string `json:"name"`
			UID       string `json:"uid,omitempty"`
		}{Kind: kind, Namespace: namespace, Name: name, UID: uid},
	}
}

func demoCrashLoopPod(observed time.Time) k8sgateway.Pod {
	waiting := k8sgateway.ContainerStateDetail{Reason: "CrashLoopBackOff", Message: "back-off 40s restarting failed container"}
	terminated := k8sgateway.ContainerStateDetail{Reason: "Error", Message: "exited with code 1", ExitCode: 1, StartedAt: observed.Add(-2 * time.Minute).Format(time.RFC3339), FinishedAt: observed.Add(-90 * time.Second).Format(time.RFC3339)}
	return k8sgateway.Pod{
		Metadata: k8sgateway.ObjectMeta{Name: "web-7c9d5f4b6-abc12", Namespace: "demo", UID: "uid-crashloop"},
		Spec: struct {
			NodeName       string                    `json:"nodeName,omitempty"`
			Containers     []k8sgateway.PodContainer `json:"containers"`
			InitContainers []k8sgateway.PodContainer `json:"initContainers,omitempty"`
			Volumes        []k8sgateway.PodVolume    `json:"volumes,omitempty"`
		}{NodeName: "node-a", Containers: []k8sgateway.PodContainer{{Name: "web", Image: "example.io/web:latest"}}},
		Status: struct {
			Phase                 string                       `json:"phase"`
			PodIP                 string                       `json:"podIP,omitempty"`
			HostIP                string                       `json:"hostIP,omitempty"`
			Reason                string                       `json:"reason,omitempty"`
			Message               string                       `json:"message,omitempty"`
			Conditions            []k8sgateway.PodCondition    `json:"conditions,omitempty"`
			ContainerStatuses     []k8sgateway.ContainerStatus `json:"containerStatuses,omitempty"`
			InitContainerStatuses []k8sgateway.ContainerStatus `json:"initContainerStatuses,omitempty"`
		}{
			Phase: "Running",
			ContainerStatuses: []k8sgateway.ContainerStatus{{
				Name: "web", Ready: false, RestartCount: 5,
				State:     k8sgateway.ContainerState{Waiting: &waiting},
				LastState: k8sgateway.ContainerState{Terminated: &terminated},
			}},
		},
	}
}

func demoImagePullPod(observed time.Time) k8sgateway.Pod {
	waiting := k8sgateway.ContainerStateDetail{Reason: "ImagePullBackOff", Message: `Back-off pulling image "example.io/api:1.0.0"`}
	return k8sgateway.Pod{
		Metadata: k8sgateway.ObjectMeta{Name: "api-6b8f4c9d7-def34", Namespace: "demo", UID: "uid-imagepull"},
		Spec: struct {
			NodeName       string                    `json:"nodeName,omitempty"`
			Containers     []k8sgateway.PodContainer `json:"containers"`
			InitContainers []k8sgateway.PodContainer `json:"initContainers,omitempty"`
			Volumes        []k8sgateway.PodVolume    `json:"volumes,omitempty"`
		}{NodeName: "node-a", Containers: []k8sgateway.PodContainer{{Name: "api", Image: "example.io/api:1.0.0"}}},
		Status: struct {
			Phase                 string                       `json:"phase"`
			PodIP                 string                       `json:"podIP,omitempty"`
			HostIP                string                       `json:"hostIP,omitempty"`
			Reason                string                       `json:"reason,omitempty"`
			Message               string                       `json:"message,omitempty"`
			Conditions            []k8sgateway.PodCondition    `json:"conditions,omitempty"`
			ContainerStatuses     []k8sgateway.ContainerStatus `json:"containerStatuses,omitempty"`
			InitContainerStatuses []k8sgateway.ContainerStatus `json:"initContainerStatuses,omitempty"`
		}{
			Phase: "Running",
			ContainerStatuses: []k8sgateway.ContainerStatus{{
				Name: "api", Ready: false, RestartCount: 2,
				State: k8sgateway.ContainerState{Waiting: &waiting},
			}},
		},
	}
}

func demoOOMKilledPod(observed time.Time) k8sgateway.Pod {
	terminated := k8sgateway.ContainerStateDetail{Reason: "OOMKilled", Message: "Memory limit exceeded", ExitCode: 137, FinishedAt: observed.Add(-3 * time.Minute).Format(time.RFC3339)}
	return k8sgateway.Pod{
		Metadata: k8sgateway.ObjectMeta{Name: "worker-5d4c9f8e2-ghi56", Namespace: "demo", UID: "uid-oom"},
		Spec: struct {
			NodeName       string                    `json:"nodeName,omitempty"`
			Containers     []k8sgateway.PodContainer `json:"containers"`
			InitContainers []k8sgateway.PodContainer `json:"initContainers,omitempty"`
			Volumes        []k8sgateway.PodVolume    `json:"volumes,omitempty"`
		}{NodeName: "node-b", Containers: []k8sgateway.PodContainer{{Name: "worker", Image: "example.io/worker:2.1.0"}}},
		Status: struct {
			Phase                 string                       `json:"phase"`
			PodIP                 string                       `json:"podIP,omitempty"`
			HostIP                string                       `json:"hostIP,omitempty"`
			Reason                string                       `json:"reason,omitempty"`
			Message               string                       `json:"message,omitempty"`
			Conditions            []k8sgateway.PodCondition    `json:"conditions,omitempty"`
			ContainerStatuses     []k8sgateway.ContainerStatus `json:"containerStatuses,omitempty"`
			InitContainerStatuses []k8sgateway.ContainerStatus `json:"initContainerStatuses,omitempty"`
		}{
			Phase: "Running",
			ContainerStatuses: []k8sgateway.ContainerStatus{{
				Name: "worker", Ready: false, RestartCount: 3,
				State: k8sgateway.ContainerState{Terminated: &terminated},
			}},
		},
	}
}

func demoPendingPod(observed time.Time) k8sgateway.Pod {
	return k8sgateway.Pod{
		Metadata: k8sgateway.ObjectMeta{Name: "db-7f8e6a5c4-jkl78", Namespace: "demo", UID: "uid-pending"},
		Spec: struct {
			NodeName       string                    `json:"nodeName,omitempty"`
			Containers     []k8sgateway.PodContainer `json:"containers"`
			InitContainers []k8sgateway.PodContainer `json:"initContainers,omitempty"`
			Volumes        []k8sgateway.PodVolume    `json:"volumes,omitempty"`
		}{Containers: []k8sgateway.PodContainer{{Name: "db", Image: "postgres:16"}}},
		Status: struct {
			Phase                 string                       `json:"phase"`
			PodIP                 string                       `json:"podIP,omitempty"`
			HostIP                string                       `json:"hostIP,omitempty"`
			Reason                string                       `json:"reason,omitempty"`
			Message               string                       `json:"message,omitempty"`
			Conditions            []k8sgateway.PodCondition    `json:"conditions,omitempty"`
			ContainerStatuses     []k8sgateway.ContainerStatus `json:"containerStatuses,omitempty"`
			InitContainerStatuses []k8sgateway.ContainerStatus `json:"initContainerStatuses,omitempty"`
		}{
			Phase: "Pending", Message: "0/3 nodes are available",
			Conditions: []k8sgateway.PodCondition{{Type: "PodScheduled", Status: "False", Reason: "Unschedulable", Message: "0/3 nodes are available: 3 Insufficient memory.", LastTransitionTime: observed.Format(time.RFC3339)}},
		},
	}
}

func demoHealthyPod(observed time.Time) k8sgateway.Pod {
	running := k8sgateway.ContainerStateDetail{Reason: "", Message: ""}
	return k8sgateway.Pod{
		Metadata: k8sgateway.ObjectMeta{Name: "healthy-1", Namespace: "demo", UID: "uid-healthy"},
		Spec: struct {
			NodeName       string                    `json:"nodeName,omitempty"`
			Containers     []k8sgateway.PodContainer `json:"containers"`
			InitContainers []k8sgateway.PodContainer `json:"initContainers,omitempty"`
			Volumes        []k8sgateway.PodVolume    `json:"volumes,omitempty"`
		}{NodeName: "node-a", Containers: []k8sgateway.PodContainer{{Name: "app", Image: "example.io/app:latest"}}},
		Status: struct {
			Phase                 string                       `json:"phase"`
			PodIP                 string                       `json:"podIP,omitempty"`
			HostIP                string                       `json:"hostIP,omitempty"`
			Reason                string                       `json:"reason,omitempty"`
			Message               string                       `json:"message,omitempty"`
			Conditions            []k8sgateway.PodCondition    `json:"conditions,omitempty"`
			ContainerStatuses     []k8sgateway.ContainerStatus `json:"containerStatuses,omitempty"`
			InitContainerStatuses []k8sgateway.ContainerStatus `json:"initContainerStatuses,omitempty"`
		}{
			Phase: "Running",
			ContainerStatuses: []k8sgateway.ContainerStatus{{
				Name: "app", Ready: true, RestartCount: 0,
				State: k8sgateway.ContainerState{Waiting: &running},
			}},
		},
	}
}

func demoEventsFor(pod k8sgateway.Pod, observed time.Time) []k8sgateway.Event {
	switch pod.Metadata.UID {
	case "uid-crashloop":
		return []k8sgateway.Event{
			demoEvent("web-backoff", pod.Metadata.Name, "Pod", pod.Metadata.Namespace, "BackOff", "Back-off restarting failed container", 5, observed),
		}
	case "uid-imagepull":
		return []k8sgateway.Event{
			demoEvent("api-pull-failed", pod.Metadata.Name, "Pod", pod.Metadata.Namespace, "Failed", `Failed to pull image "example.io/api:1.0.0": pull access denied`, 2, observed),
		}
	case "uid-oom":
		return []k8sgateway.Event{
			demoEvent("worker-oomkill", pod.Metadata.Name, "Pod", pod.Metadata.Namespace, "OOMKilling", "Kill 12345 (worker) memory usage 512MiB exceeds limit 256MiB", 1, observed),
		}
	case "uid-pending":
		return []k8sgateway.Event{
			demoEvent("db-unschedulable", pod.Metadata.Name, "Pod", pod.Metadata.Namespace, "FailedScheduling", "0/3 nodes are available: 3 Insufficient memory.", 4, observed),
		}
	default:
		return nil
	}
}
