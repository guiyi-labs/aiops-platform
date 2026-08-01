package deprecatedapi

import (
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Check evaluates a set of objects against a target Kubernetes minor version
// and returns a rollup Status with one Finding per flagged object.
//
// Classification, relative to targetMinor (the minor version the cluster is
// being upgraded to):
//   - "removed"   (critical): the object's apiVersion was removed at or before
//     targetMinor; it will fail to apply after the upgrade.
//   - "deprecated"(warning): the apiVersion still works on targetMinor but is
//     slated for removal; a 3-minor lead-time window is used when an explicit
//     deprecation minor is not recorded in the catalog.
//   - clean:      no known issue for targetMinor.
//
// The function is pure and read-only; it never touches a cluster.
func Check(clusterID int64, targetVersion string, objects []ResourceObject, observedAt time.Time) Status {
	targetMinor := minorVersion(targetVersion)
	status := Status{
		ClusterID:   clusterID,
		TargetMinor: targetMinor,
		Total:       len(objects),
		EvaluatedAt: observedAt.UTC(),
	}

	for _, obj := range objects {
		entry, ok := Lookup(obj.APIVersion, obj.Kind)
		if !ok {
			status.Clean++
			continue
		}

		deprecatedIn := entry.DeprecatedIn
		if deprecatedIn == 0 && entry.RemovedIn > 0 {
			deprecatedIn = entry.RemovedIn - 3
		}

		var fstatus string
		switch {
		case entry.RemovedIn != 0 && targetMinor >= entry.RemovedIn:
			fstatus = StatusRemoved
		case targetMinor >= deprecatedIn:
			fstatus = StatusDeprecated
		default:
			status.Clean++
			continue
		}

		code := CodeDeprecatedAPI
		if fstatus == StatusRemoved {
			code = CodeRemovedAPI
			status.Removed++
		} else {
			status.Deprecated++
		}

		finding := k8sfinding.Finding{
			Code:     code,
			Severity: SeverityFor(fstatus),
			Summary:  entry.Note,
			Resource: k8sfinding.ResourceCitation{
				Kind:      obj.Kind,
				Namespace: obj.Namespace,
				Name:      obj.Name,
				UID:       obj.UID,
			},
			Details: map[string]string{
				"api_version": obj.APIVersion,
				"replacement": entry.Replacement,
				"removed_in":  itoa(entry.RemovedIn),
				"note":        entry.Note,
			},
			ObservedAt: observedAt.UTC().Format(time.RFC3339),
		}
		status.Findings = append(status.Findings, finding)
	}

	return status
}

func itoa(v int) string {
	if v == 0 {
		return ""
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
