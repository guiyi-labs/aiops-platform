package main

// Deterministic fixture objects served by demo-kube-mock. Field shapes mirror
// the Kubernetes JSON contract consumed by the platform's internal gateway.

func fixtureNode() map[string]any {
	return map[string]any{
		"kind": "Node", "apiVersion": "v1",
		"metadata": map[string]any{
			"name": "demo-node", "uid": "demo-node-uid-0001", "resourceVersion": "10",
			"labels": map[string]any{
				"kubernetes.io/hostname":          "demo-node",
				"node-role.kubernetes.io/worker": "",
				"node.kubernetes.io/instance-type": "t3.large",
			},
			"annotations": map[string]any{"k8s-aiops.local/demo": "node-not-ready"},
		},
		"spec": map[string]any{"unschedulable": false, "taints": []any{}},
		"status": map[string]any{
			"nodeInfo": map[string]any{
				"kubeletVersion": "v1.36.0", "osImage": "Ubuntu 24.04 LTS",
				"containerRuntimeVersion": "containerd://1.7.27",
			},
			"addresses": []any{
				map[string]any{"type": "InternalIP", "address": "10.0.0.11"},
				map[string]any{"type": "Hostname", "address": "demo-node"},
			},
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "False", "reason": "KubeletNotReady", "message": "kubelet is not ready: container runtime is down", "lastTransitionTime": "2026-08-12T06:30:00Z"},
				map[string]any{"type": "MemoryPressure", "status": "True", "reason": "KubeletHasInsufficientMemory", "message": "kubelet has insufficient memory available", "lastTransitionTime": "2026-08-12T06:25:00Z"},
				map[string]any{"type": "DiskPressure", "status": "False", "reason": "KubeletHasNoDiskPressure", "message": "kubelet has no disk pressure", "lastTransitionTime": "2026-08-12T06:00:00Z"},
				map[string]any{"type": "PIDPressure", "status": "False", "reason": "KubeletHasNoPIDPressure", "message": "kubelet has no PID pressure", "lastTransitionTime": "2026-08-12T06:00:00Z"},
				map[string]any{"type": "NetworkUnavailable", "status": "False", "reason": "RouteCreated", "message": "kubelet has no network pressure", "lastTransitionTime": "2026-08-12T06:00:00Z"},
			},
			"capacity":    map[string]any{"cpu": "8", "memory": "16Gi", "pods": "110"},
			"allocatable": map[string]any{"cpu": "7", "memory": "14Gi", "pods": "110"},
		},
	}
}

func fixturePod() map[string]any {
	return map[string]any{
		"kind": "Pod", "apiVersion": "v1",
		"metadata": map[string]any{
			"name": "demo-pod", "namespace": "demo", "uid": "demo-pod-uid-0001", "resourceVersion": "20",
			"creationTimestamp": "2026-08-12T06:00:00Z",
			"labels":            map[string]any{"app": "demo-app", "pod-template-hash": "abc123"},
			"ownerReferences": []any{map[string]any{
				"kind": "ReplicaSet", "name": "demo-app-abc123", "uid": "demo-rs-uid-0001", "apiVersion": "apps/v1",
			}},
		},
		"spec": map[string]any{
			"nodeName": "demo-node",
			"containers": []any{map[string]any{
				"name": "app", "image": "demo/app:1.0.0",
				"resources": map[string]any{"limits": map[string]any{"memory": "256Mi"}, "requests": map[string]any{"memory": "128Mi"}},
			}},
		},
		"status": map[string]any{
			"phase":  "Running",
			"podIP":  "10.244.0.15",
			"hostIP": "10.0.0.11",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "False", "reason": "ContainersNotReady", "message": "containers with unready status: [app]"},
				map[string]any{"type": "Initialized", "status": "True"},
				map[string]any{"type": "PodScheduled", "status": "True"},
			},
			"containerStatuses": []any{map[string]any{
				"name": "app", "ready": false, "restartCount": 12,
				"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff", "message": "back-off 5m0s restarting failed container"}},
				"lastState": map[string]any{"terminated": map[string]any{
					"reason": "OOMKilled", "message": "container memory usage exceeds limit",
					"exitCode": 137, "signal": 0,
					"startedAt": "2026-08-12T06:10:00Z", "finishedAt": "2026-08-12T06:11:00Z",
				}},
			}},
		},
	}
}

func fixtureDeployment() map[string]any {
	return map[string]any{
		"kind": "Deployment", "apiVersion": "apps/v1",
		"metadata": map[string]any{
			"name": "demo-app", "namespace": "demo", "uid": "demo-app-uid-0001", "resourceVersion": "40",
			"generation": 1, "labels": map[string]any{"app": "demo-app"},
			"annotations": map[string]any{},
		},
		"spec": map[string]any{
			"replicas": 2,
			"selector": map[string]any{"matchLabels": map[string]any{"app": "demo-app"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": "demo-app"}},
				"spec": map[string]any{"containers": []any{
					map[string]any{"name": "app", "image": "demo/app:1.0.0"},
				}},
			},
		},
		"status": map[string]any{
			"replicas": 2, "readyReplicas": 1, "availableReplicas": 1,
			"updatedReplicas": 2, "unavailableReplicas": 1,
			"conditions": []any{
				map[string]any{"type": "Available", "status": "True", "reason": "MinimumReplicasAvailable"},
				map[string]any{"type": "Progressing", "status": "True", "reason": "NewReplicaSetAvailable"},
			},
		},
	}
}

func fixtureReplicaSet() map[string]any {
	return map[string]any{
		"kind": "ReplicaSet", "apiVersion": "apps/v1",
		"metadata": map[string]any{
			"name": "demo-app-abc123", "namespace": "demo", "uid": "demo-rs-uid-0001", "resourceVersion": "35",
			"ownerReferences": []any{map[string]any{
				"kind": "Deployment", "name": "demo-app", "uid": "demo-app-uid-0001", "apiVersion": "apps/v1",
			}},
		},
		"spec": map[string]any{"replicas": 2, "selector": map[string]any{"matchLabels": map[string]any{"app": "demo-app", "pod-template-hash": "abc123"}}},
		"status": map[string]any{"replicas": 2, "readyReplicas": 1, "availableReplicas": 1},
	}
}

func fixtureEvents() []map[string]any {
	return []map[string]any{
		{
			"kind": "Event", "apiVersion": "v1",
			"metadata": map[string]any{"name": "demo-pod-oom.001", "namespace": "demo", "resourceVersion": "30", "creationTimestamp": "2026-08-12T06:11:00Z"},
			"type": "Warning", "reason": "OOMKilling",
			"message": "Container app of Pod demo-pod in namespace demo was OOMKilled: memory usage exceeds limit",
			"count": 12, "firstTimestamp": "2026-08-12T06:05:00Z", "lastTimestamp": "2026-08-12T06:11:00Z",
			"involvedObject": map[string]any{"kind": "Pod", "namespace": "demo", "name": "demo-pod", "uid": "demo-pod-uid-0001"},
		},
		{
			"kind": "Event", "apiVersion": "v1",
			"metadata": map[string]any{"name": "demo-node-notready.001", "namespace": "demo", "resourceVersion": "31", "creationTimestamp": "2026-08-12T06:30:00Z"},
			"type": "Warning", "reason": "NodeNotReady",
			"message": "Node demo-node status is now: NodeNotReady",
			"count": 3, "firstTimestamp": "2026-08-12T06:30:00Z", "lastTimestamp": "2026-08-12T06:32:00Z",
			"involvedObject": map[string]any{"kind": "Node", "namespace": "", "name": "demo-node", "uid": "demo-node-uid-0001"},
		},
	}
}

func fixtureNodeMetric() map[string]any {
	return map[string]any{
		"kind": "NodeMetrics", "apiVersion": "metrics.k8s.io/v1beta1",
		"metadata": map[string]any{"name": "demo-node", "resourceVersion": "50"},
		"timestamp": "2026-08-12T07:00:00Z", "window": "30s",
		"usage": map[string]any{"cpu": "3500m", "memory": "10Gi"},
	}
}

func fixturePodMetric() map[string]any {
	return map[string]any{
		"kind": "PodMetrics", "apiVersion": "metrics.k8s.io/v1beta1",
		"metadata": map[string]any{"name": "demo-pod", "namespace": "demo", "resourceVersion": "51"},
		"timestamp": "2026-08-12T07:00:00Z", "window": "30s",
		"containers": []any{map[string]any{
			"name": "app", "usage": map[string]any{"cpu": "250m", "memory": "300Mi"},
		}},
	}
}
