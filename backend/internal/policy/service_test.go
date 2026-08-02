package policy

import (
	"testing"
	"time"
)

func boolPtr(v bool) *bool { return &v }

func testTime() time.Time {
	return time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
}

func compliantWorkload() WorkloadPolicy {
	return WorkloadPolicy{
		Kind: "Deployment", Namespace: "prod", Name: "web", UID: "u-web",
		Containers: []ContainerPolicy{{
			Name:                     "app",
			CPURequest:               true,
			MemoryRequest:            true,
			HasResourceLimits:        true,
			Privileged:               boolPtr(false),
			AllowPrivilegeEscalation: boolPtr(false),
			RunAsNonRoot:             boolPtr(true),
			LivenessProbe:            true,
			ReadinessProbe:           true,
			StartupProbe:             true,
		}},
	}
}

// TestEvaluate_CompliantWorkload_NoFindings: a workload that satisfies every
// rule produces zero findings and counts as compliant.
func TestEvaluate_CompliantWorkload_NoFindings(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{compliantWorkload()}}
	s := Evaluate(7, in, testTime())

	if s.Total != 3+3+3+2 { // container checks (9) + pod-level (2)
		t.Fatalf("total = %d, want 11", s.Total)
	}
	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0; findings=%+v", s.Failed, s.Findings)
	}
	if s.Passed != s.Total {
		t.Fatalf("passed = %d, want %d", s.Passed, s.Total)
	}
	if s.CompliantWorkloads != 1 {
		t.Fatalf("compliant_workloads = %d, want 1", s.CompliantWorkloads)
	}
	if len(s.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", s.Findings)
	}
}

// TestEvaluate_MissingEverything_CriticalFirst: an empty container spec
// produces the full rule set; the privileged finding must sort first.
func TestEvaluate_MissingEverything_CriticalFirst(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{{
		Kind: "StatefulSet", Namespace: "default", Name: "db", UID: "u-db",
		HostNetwork: true,
		Containers: []ContainerPolicy{{
			Name:       "db",
			Privileged: boolPtr(true),
		}},
	}}}
	s := Evaluate(7, in, testTime())

	if s.WorkloadsTotal != 1 || s.ContainersTotal != 1 {
		t.Fatalf("counters = %d/%d, want 1/1", s.WorkloadsTotal, s.ContainersTotal)
	}
	// 9 container checks (3 resources + 3 security + 3 probes) + 2 pod-level.
	if s.Total != 11 {
		t.Fatalf("total = %d, want 11", s.Total)
	}
	// Findings: no cpu req, no mem req, no limits, privileged, esc, runasroot,
	// no liveness, no readiness, no startup, hostNetwork = 10.
	if s.Failed != 10 {
		t.Fatalf("failed = %d, want 10; findings=%d", s.Failed, len(s.Findings))
	}
	if s.CompliantWorkloads != 0 {
		t.Fatalf("compliant_workloads = %d, want 0", s.CompliantWorkloads)
	}
	if s.BySeverity[SeverityCritical] != 1 || s.BySeverity[SeverityWarning] != 7 || s.BySeverity[SeverityInfo] != 2 {
		t.Fatalf("by_severity = %+v, want critical=1 warning=7 info=2", s.BySeverity)
	}
	// The single critical finding must sort first.
	if len(s.Findings) == 0 || s.Findings[0].Code != CodePrivileged {
		t.Fatalf("first finding = %+v, want privileged", s.Findings[0])
	}
}

// TestEvaluate_PodLevelOnly_Compliance: host-level findings attach to the
// workload citation even when the container is empty.
func TestEvaluate_PodLevelOnly_Compliance(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{{
		Kind: "DaemonSet", Namespace: "kube-system", Name: "node-exporter", UID: "u-ne",
		HostPID: true, HostIPC: true,
	}}}
	s := Evaluate(7, in, testTime())

	if s.Total != 2 {
		t.Fatalf("total = %d, want 2 (pod-level only)", s.Total)
	}
	if s.Failed != 1 {
		t.Fatalf("failed = %d, want 1 (host pid/ipc)", s.Failed)
	}
	if len(s.Findings) != 1 || s.Findings[0].Code != CodeHostPIDOrIPC {
		t.Fatalf("findings = %+v, want single host pid/ipc", s.Findings)
	}
	if s.Findings[0].Resource.Name != "node-exporter" {
		t.Fatalf("finding resource = %+v, want node-exporter", s.Findings[0].Resource)
	}
	if s.CompliantWorkloads != 0 {
		t.Fatalf("compliant_workloads = %d, want 0", s.CompliantWorkloads)
	}
}

// TestEvaluate_MixedWorkloads_ComplianceIsPerWorkload: one clean and one dirty
// workload yields compliant_workloads == 1.
func TestEvaluate_MixedWorkloads_ComplianceIsPerWorkload(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{
		compliantWorkload(),
		{Kind: "Deployment", Namespace: "staging", Name: "api", UID: "u-api",
			Containers: []ContainerPolicy{{Name: "api"}}}, // missing everything
	}}
	s := Evaluate(7, in, testTime())

	if s.WorkloadsTotal != 2 {
		t.Fatalf("workloads_total = %d, want 2", s.WorkloadsTotal)
	}
	if s.CompliantWorkloads != 1 {
		t.Fatalf("compliant_workloads = %d, want 1 (only web is clean)", s.CompliantWorkloads)
	}
	if s.Passed != s.Total-s.Failed {
		t.Fatalf("passed = %d, want total-failed = %d", s.Passed, s.Total-s.Failed)
	}
}

// TestEvaluate_EmptyInputs_EmptyResult: nothing to analyze means zero checks.
func TestEvaluate_EmptyInputs_EmptyResult(t *testing.T) {
	s := Evaluate(7, Inputs{}, testTime())

	if s.Total != 0 || s.Failed != 0 || s.Passed != 0 {
		t.Fatalf("empty input counters = %+v, want all zero", s)
	}
	if s.Findings == nil {
		t.Fatal("findings must serialize as [] rather than null")
	}
	if s.BySeverity == nil || s.ByFamily == nil {
		t.Fatal("maps must be initialized")
	}
}

// TestFindingDetails_CarryContainerAndRemediation: per-container findings
// include the container name plus family and remediation.
func TestFindingDetails_CarryContainerAndRemediation(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{{
		Kind: "Deployment", Namespace: "prod", Name: "web", UID: "u-web",
		Containers: []ContainerPolicy{{Name: "app"}}, // no cpu request
	}}}
	s := Evaluate(7, in, testTime())

	var cpuReq *Finding
	for i := range s.Findings {
		if s.Findings[i].Code == CodeNoCPURequest {
			cpuReq = &s.Findings[i]
		}
	}
	if cpuReq == nil {
		t.Fatalf("no cpu-request finding; all=%+v", s.Findings)
	}
	if cpuReq.Details["container"] != "app" {
		t.Fatalf("container detail = %q, want app", cpuReq.Details["container"])
	}
	if cpuReq.Details["family"] != FamilyResources {
		t.Fatalf("family detail = %q, want resources", cpuReq.Details["family"])
	}
	if cpuReq.Details["remediation"] == "" {
		t.Fatal("remediation must be present")
	}
	if cpuReq.ObservedAt != "2026-08-02T10:00:00Z" {
		t.Fatalf("observed_at = %q, want RFC3339 UTC", cpuReq.ObservedAt)
	}
}

// TestEvaluate_SortIsDeterministic: same input always yields the same order.
func TestEvaluate_SortIsDeterministic(t *testing.T) {
	in := Inputs{Workloads: []WorkloadPolicy{
		{Kind: "Deployment", Namespace: "b", Name: "z", Containers: []ContainerPolicy{{Name: "c"}}},
		{Kind: "Deployment", Namespace: "a", Name: "y", Containers: []ContainerPolicy{{Name: "c"}}},
	}}
	a := Evaluate(7, in, testTime())
	b := Evaluate(7, in, testTime())

	if len(a.Findings) != len(b.Findings) {
		t.Fatalf("finding counts differ: %d vs %d", len(a.Findings), len(b.Findings))
	}
	for i := range a.Findings {
		if a.Findings[i].Code != b.Findings[i].Code || a.Findings[i].Resource.Name != b.Findings[i].Resource.Name {
			t.Fatalf("order mismatch at %d: %+v vs %+v", i, a.Findings[i], b.Findings[i])
		}
	}
}
