package gitopsdrift

import (
	"encoding/json"
	"testing"
	"time"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func resource(kind, namespace, name, manager string, applied, live string) ManagedResource {
	r := ManagedResource{Kind: kind, Namespace: namespace, Name: name, Manager: manager}
	if applied != "" {
		r.AppliedConfig = raw(applied)
	}
	if live != "" {
		r.LiveBody = raw(live)
	}
	return r
}

func findingsByCode(s Status) map[string]int {
	out := map[string]int{}
	for _, f := range s.Findings {
		out[f.Code]++
	}
	return out
}

func TestEvaluateEmptyBundle(t *testing.T) {
	status := Evaluate(7, Inputs{}, time.Now())
	if status.Total != 0 {
		t.Fatalf("expected Total 0, got %d", status.Total)
	}
	if status.Findings == nil {
		t.Fatal("expected non-nil Findings slice")
	}
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings, got %d", len(status.Findings))
	}
}

func TestEvaluateDriftDetected(t *testing.T) {
	applied := `{"apiVersion":"apps/v1","kind":"Deployment","spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"img:1"}]}}}}`
	live := `{"apiVersion":"apps/v1","kind":"Deployment","spec":{"replicas":5,"template":{"spec":{"containers":[{"name":"app","image":"img:1"}]}}}}`
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())

	codes := findingsByCode(status)
	if codes[CodeDriftDetected] != 1 {
		t.Fatalf("expected 1 drift finding, got %v", codes)
	}
	if status.DriftedResources != 1 {
		t.Fatalf("expected DriftedResources 1, got %d", status.DriftedResources)
	}
	if status.Total != 1 || status.Failed != 1 || status.Passed != 0 {
		t.Fatalf("rollup wrong: %+v", status)
	}
	f := status.Findings[0]
	if f.Severity != SeverityWarning {
		t.Fatalf("expected warning severity, got %s", f.Severity)
	}
	if f.Resource.Namespace != "shop" || f.Resource.Name != "api" {
		t.Fatalf("resource citation wrong: %+v", f.Resource)
	}
	if f.Details["family"] != FamilyGitOpsDrift {
		t.Fatalf("family wrong: %s", f.Details["family"])
	}
	if f.Details["manager"] != ManagerKubectl {
		t.Fatalf("manager wrong: %s", f.Details["manager"])
	}
	if f.Details["field_count"] == "" {
		t.Fatal("expected field_count detail")
	}
}

func TestEvaluateNoDrift(t *testing.T) {
	applied := `{"apiVersion":"apps/v1","kind":"Deployment","spec":{"replicas":3,"template":{"spec":{"containers":[{"name":"app","image":"img:1"}]}}}}`
	live := applied
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", status.Findings)
	}
	if status.Total != 1 || status.Passed != 1 {
		t.Fatalf("rollup wrong: %+v", status)
	}
}

func TestEvaluateUnmanagedInManagedNamespace(t *testing.T) {
	// A resource without last-applied-configuration in a GitOps-managed
	// namespace should be flagged unmanaged.
	in := Inputs{
		ManagedNamespaces: []string{"gitops"},
		Resources: []ManagedResource{
			// no AppliedConfig, no manager annotation
			resource("Deployment", "gitops", "manual", "", "", `{"apiVersion":"apps/v1","kind":"Deployment","spec":{"replicas":1}}`),
		},
	}
	status := Evaluate(7, in, time.Now())
	codes := findingsByCode(status)
	if codes[CodeUnmanaged] != 1 {
		t.Fatalf("expected 1 unmanaged finding, got %v", codes)
	}
	if status.UnmanagedResources != 1 {
		t.Fatalf("expected UnmanagedResources 1, got %d", status.UnmanagedResources)
	}
	if status.Findings[0].Severity != SeverityInfo {
		t.Fatalf("expected info severity, got %s", status.Findings[0].Severity)
	}
}

func TestEvaluateUnmanagedNotFlaggedOutsideManagedNamespace(t *testing.T) {
	// Same resource but the namespace is NOT detected as GitOps-managed: no
	// unmanaged finding (would be noise).
	in := Inputs{
		ManagedNamespaces: []string{"gitops"},
		Resources: []ManagedResource{
			resource("Deployment", "random", "manual", "", "", `{"spec":{}}`),
		},
	}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings, got %v", status.Findings)
	}
}

func TestEvaluateOrderInsensitiveArrayOfMaps(t *testing.T) {
	// Reordered env list (array of maps) must NOT be reported as drift.
	applied := `{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"},{"name":"B","value":"2"}]}]}}`
	live := `{"spec":{"containers":[{"name":"app","env":[{"name":"B","value":"2"},{"name":"A","value":"1"}]}]}}`
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no drift for reordered env, got %v", status.Findings)
	}
}

func TestEvaluateArrayElementRemovedIsDrift(t *testing.T) {
	applied := `{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"},{"name":"B","value":"2"}]}]}}`
	live := `{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"}]}]}}`
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if findingsByCode(status)[CodeDriftDetected] != 1 {
		t.Fatalf("expected drift when array element removed, got %v", findingsByCode(status))
	}
}

func TestEvaluateArrayElementValueChangedIsDrift(t *testing.T) {
	applied := `{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"1"}]}]}}`
	live := `{"spec":{"containers":[{"name":"app","env":[{"name":"A","value":"2"}]}]}}`
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if findingsByCode(status)[CodeDriftDetected] != 1 {
		t.Fatalf("expected drift when env value changed, got %v", findingsByCode(status))
	}
}

func TestEvaluateUnparseableAppliedConfigSkipped(t *testing.T) {
	in := Inputs{Resources: []ManagedResource{resource("Deployment", "shop", "api", ManagerKubectl, "{not json", `{"spec":{}}`)}}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no findings for unparseable annotation, got %v", status.Findings)
	}
	if status.Total != 1 {
		t.Fatalf("expected Total 1, got %d", status.Total)
	}
}

func TestEvaluateConfigMapDataDrift(t *testing.T) {
	applied := `{"apiVersion":"v1","kind":"ConfigMap","data":{"KEY":"old"}}`
	live := `{"apiVersion":"v1","kind":"ConfigMap","data":{"KEY":"new"}}`
	in := Inputs{Resources: []ManagedResource{resource("ConfigMap", "shop", "cfg", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if findingsByCode(status)[CodeDriftDetected] != 1 {
		t.Fatalf("expected ConfigMap data drift, got %v", findingsByCode(status))
	}
}

func TestEvaluateLiveFieldNotInAppliedIgnored(t *testing.T) {
	// A field present only in live (added by a controller) must not be drift.
	applied := `{"spec":{"replicas":3}}`
	live := `{"spec":{"replicas":3,"clusterIP":"10.0.0.5"}}`
	in := Inputs{Resources: []ManagedResource{resource("Service", "shop", "svc", ManagerKubectl, applied, live)}}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 0 {
		t.Fatalf("expected no drift from controller-added field, got %v", status.Findings)
	}
}

func TestEvaluateFindingsSortedBySeverity(t *testing.T) {
	drift := resource("Deployment", "shop", "drift", ManagerKubectl,
		`{"spec":{"replicas":3}}`, `{"spec":{"replicas":9}}`)
	in := Inputs{
		ManagedNamespaces: []string{"gitops"},
		Resources: []ManagedResource{
			drift,
			resource("Deployment", "gitops", "manual", "", "", `{"spec":{}}`), // unmanaged (info)
		},
	}
	status := Evaluate(7, in, time.Now())
	if len(status.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(status.Findings))
	}
	// warning (drift) must sort before info (unmanaged)
	if status.Findings[0].Severity != SeverityWarning {
		t.Fatalf("expected warning first, got %s", status.Findings[0].Severity)
	}
	if status.Findings[1].Severity != SeverityInfo {
		t.Fatalf("expected info second, got %s", status.Findings[1].Severity)
	}
}
