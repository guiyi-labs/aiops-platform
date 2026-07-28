package diagnosis

import (
	"time"

	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
)

func EvaluatePersistentVolumeClaimPending(clusterID int64, claim k8sgateway.PersistentVolumeClaim, events []k8sgateway.Event, observedAt time.Time) (Record, bool) {
	if claim.Status.Phase != "Pending" {
		return Record{}, false
	}
	warnings := make([]Evidence, 0, len(events))
	for _, event := range events {
		if event.Type != "Warning" {
			continue
		}
		warnings = append(warnings, Evidence{Type: "event", Source: "events.k8s.io/core.v1", Content: map[string]any{
			"name": event.Metadata.Name, "reason": event.Reason, "message": event.Message,
			"count": event.Count, "event_time": event.EventTime, "last_timestamp": event.LastTimestamp,
		}})
	}
	if len(warnings) == 0 {
		return Record{}, false
	}
	storageClass := ""
	if claim.Spec.StorageClassName != nil {
		storageClass = *claim.Spec.StorageClassName
	}
	evidence := []Evidence{{Type: "persistent_volume_claim", Source: "persistentvolumeclaim", Content: map[string]any{
		"phase": claim.Status.Phase, "storage_class_name": storageClass, "access_modes": claim.Spec.AccessModes,
		"requested_storage": claim.Spec.Resources.Requests["storage"], "volume_name": claim.Spec.VolumeName,
	}}}
	evidence = append(evidence, warnings...)
	return Record{
		ClusterID: clusterID, RuleID: RulePersistentVolumeClaimPending, Severity: "high", Status: "open",
		Resource:        ResourceRef{Kind: "PersistentVolumeClaim", Namespace: claim.Metadata.Namespace, Name: claim.Metadata.Name, UID: claim.Metadata.UID},
		Summary:         "The PersistentVolumeClaim is Pending and has exact UID-linked Warning Events.",
		RootCauses:      []string{"The requested StorageClass or provisioner cannot satisfy the claim", "Capacity, topology, access mode, or provisioning policy rejected the request"},
		Recommendations: []string{"Read the attached Warning Events before changing the claim", "Verify the StorageClass, provisioner, requested capacity, access modes, and topology", "Do not treat WaitForFirstConsumer without a Warning Event as a failure"},
		Evidence:        evidence, ObservedAt: observedAt.UTC(),
	}, true
}
