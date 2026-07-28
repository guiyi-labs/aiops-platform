package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluateNodePressure(clusterID int64, node k8sgateway.Node, observedAt time.Time) (Record, bool) {
	ready := false
	pressure := make([]Evidence, 0, 3)
	pressureTypes := map[string]bool{"MemoryPressure": true, "DiskPressure": true, "PIDPressure": true}
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			ready = condition.Status == "True"
		}
		if pressureTypes[condition.Type] && condition.Status == "True" {
			pressure = append(pressure, Evidence{Type: "node_condition", Source: "node.status.conditions", Content: map[string]any{
				"type": condition.Type, "status": condition.Status, "reason": condition.Reason,
				"message": condition.Message, "last_transition_time": condition.LastTransitionTime,
			}})
		}
	}
	if !ready || len(pressure) == 0 {
		return Record{}, false
	}
	return Record{
		ClusterID: clusterID, RuleID: RuleNodePressure, Severity: "high", Status: "open",
		Resource:        ResourceRef{Kind: "Node", Name: node.Metadata.Name, UID: node.Metadata.UID},
		Summary:         "The Ready Node reports active memory, disk, or PID pressure.",
		RootCauses:      []string{"Node resources are exhausted or close to eviction thresholds", "A workload or host process is consuming capacity faster than it is released"},
		Recommendations: []string{"Inspect the matching pressure Conditions and node allocatable capacity", "Identify high-consumption workloads and review eviction or capacity settings", "Restore headroom before scheduling additional workloads"},
		Evidence:        pressure, ObservedAt: observedAt.UTC(),
	}, true
}
