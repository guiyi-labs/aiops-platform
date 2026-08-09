package inspection

// Cover the pure mapping helpers of the inspection service: view mappers,
// fingerprinting, cluster ID extraction, evidence JSON decode and the bounded
// string helpers. No cluster access is needed.

import (
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/kubernetes"
)

func TestEffectiveClusterIDs(t *testing.T) {
	got := effectiveClusterIDs([]struct {
		ID   int64
		Name string
	}{{ID: 3, Name: "c"}, {ID: 7, Name: "d"}})
	if len(got) != 2 || got[0] != 3 || got[1] != 7 {
		t.Errorf("effectiveClusterIDs = %v, want [3 7]", got)
	}
	if got := effectiveClusterIDs(nil); len(got) != 0 {
		t.Errorf("effectiveClusterIDs(nil) = %v, want empty", got)
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	now := time.Now()
	f := Finding{
		RuleCode: "pod_pending", SignalCode: "x", Namespace: "ns",
		ResourceKind: "Pod", ResourceName: "p", ResourceUID: "u",
		ObservedAt: now, Evidence: map[string]interface{}{"k": "v"},
	}
	a := fingerprint(1, f)
	b := fingerprint(1, f)
	if a != b {
		t.Errorf("fingerprint not deterministic: %q vs %q", a, b)
	}
	if len(a) != MaxFingerprintLen {
		t.Errorf("fingerprint len = %d, want %d", len(a), MaxFingerprintLen)
	}
	// A differing resource must change the fingerprint.
	other := f
	other.ResourceUID = "u2"
	if fingerprint(1, other) == a {
		t.Error("different resource should produce different fingerprint")
	}
}

func TestJoinSummaryTruncates(t *testing.T) {
	if got := joinSummary(nil); got != "" {
		t.Errorf("joinSummary(nil) = %q, want empty", got)
	}
	if got := joinSummary([]string{"a", "b"}); got != "a; b" {
		t.Errorf("joinSummary = %q, want \"a; b\"", got)
	}
	long := strings.Repeat("x", MaxReasonLen+100)
	got := joinSummary([]string{long})
	if len(got) != MaxReasonLen {
		t.Errorf("joinSummary truncation len = %d, want %d", len(got), MaxReasonLen)
	}
}

func TestPString(t *testing.T) {
	p := pstring("hello")
	if p == nil || *p != "hello" {
		t.Errorf("pstring = %v", p)
	}
}

func TestPlanViewFromCopies(t *testing.T) {
	now := time.Now()
	plan := Plan{
		ID: 1, Name: "p", CreatorID: 2,
		ClusterIDs: Int64Array{3, 4}, RuleCodes: StringArray{"a"},
		CronSpec: "0 0 * * *", Enabled: true,
		LastRunAt: &now, NextRunAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	v := planViewFrom(plan)
	if v.ID != 1 || v.Name != "p" || len(v.ClusterIDs) != 2 || len(v.RuleCodes) != 1 {
		t.Errorf("planViewFrom = %+v", v)
	}
	// mutate source should not affect view (deep copy of slices)
	plan.ClusterIDs[0] = 99
	if v.ClusterIDs[0] != 3 {
		t.Error("planViewFrom should copy cluster ids")
	}
}

func TestTaskViewFrom(t *testing.T) {
	now := time.Now()
	task := Task{
		ID: 9, ClusterIDs: Int64Array{1}, RuleCodes: StringArray{"b"},
		Status: TaskStatusCompleted, TotalClusters: 2, CompletedClusters: 2,
		FindingCount: 5, ErrorSummary: "none", CreatedAt: now,
	}
	v := taskViewFrom(task)
	if v.ID != 9 || v.Status != TaskStatusCompleted || v.TotalClusters != 2 || v.FindingCount != 5 {
		t.Errorf("taskViewFrom = %+v", v)
	}
}

func TestResultViewFromParsesEvidence(t *testing.T) {
	now := time.Now()
	ok := resultViewFrom(Result{
		ID: 1, TaskID: 2, ClusterID: 3, RuleCode: "r", SignalCode: "s",
		Severity: SeverityWarning, Namespace: "ns", ResourceKind: "Pod",
		ResourceName: "p", ResourceUID: "u", Fingerprint: "fp",
		EvidenceSnapshot: `{"a":1,"b":"x"}`,
		ObservedAt:       now, CreatedAt: now,
	})
	if ok.Evidence["a"].(float64) != 1 || ok.Evidence["b"] != "x" {
		t.Errorf("resultViewFrom parsed evidence = %+v", ok.Evidence)
	}
	bad := resultViewFrom(Result{EvidenceSnapshot: `not-json`})
	if bad.Evidence != nil {
		t.Errorf("resultViewFrom should leave Evidence nil on parse failure, got %+v", bad.Evidence)
	}
}

func TestSummarizePodConditions(t *testing.T) {
	conds := []kubernetes.PodCondition{
		{Type: "Ready", Status: "True"},
		{Type: "Scheduled", Status: "False", Reason: "Unschedulable", Message: "no nodes"},
	}
	out := summarizePodConditions(conds)
	if len(out) != 1 {
		t.Fatalf("summarizePodConditions = %d, want 1 (Ready status True skipped)", len(out))
	}
	if out[0]["type"] != "Scheduled" || out[0]["reason"] != "Unschedulable" {
		t.Errorf("summarizePodConditions[0] = %+v", out[0])
	}
	long := strings.Repeat("m", 500)
	out2 := summarizePodConditions([]kubernetes.PodCondition{{Type: "x", Status: "False", Message: long}})
	if len(out2[0]["message"]) >= len(long) {
		t.Errorf("summarizePodConditions should redact long messages, got len=%d", len(out2[0]["message"]))
	}
}

func TestRedactContainerStatuses(t *testing.T) {
	statuses := []kubernetes.ContainerStatus{
		{
			Name: "app", Ready: true, RestartCount: 3,
			State:     kubernetes.ContainerState{Terminated: &kubernetes.ContainerStateDetail{Reason: "OOMKilled", ExitCode: 137}},
			LastState: kubernetes.ContainerState{Terminated: &kubernetes.ContainerStateDetail{Reason: "Completed", ExitCode: 0, FinishedAt: "t0"}},
		},
		{Name: "init", Ready: false},
	}
	out := redactContainerStatuses(statuses)
	if len(out) != 2 {
		t.Fatalf("redactContainerStatuses = %d, want 2", len(out))
	}
	if out[0]["state_reason"] != "OOMKilled" || out[0]["state_exit_code"].(int32) != 137 {
		t.Errorf("redactContainerStatuses[0] = %+v", out[0])
	}
	if out[0]["last_reason"] != "Completed" || out[0]["last_finished_at"] != "t0" {
		t.Errorf("redactContainerStatuses[0] last = %+v", out[0])
	}
	if _, ok := out[1]["state_reason"]; ok {
		t.Errorf("init container with no terminated state should not have state_reason, got %+v", out[1])
	}
}

func TestMemoryLimitFor(t *testing.T) {
	pod := kubernetes.Pod{}
	pod.Spec.Containers = []kubernetes.PodContainer{
		{
			Name:  "app",
			Image: "nginx",
			Resources: kubernetes.ResourceRequirements{
				Limits: map[string]string{"memory": "256Mi"},
			},
		},
	}
	if got := memoryLimitFor(pod, "app"); got != "256Mi" {
		t.Errorf("memoryLimitFor(app) = %q, want 256Mi", got)
	}
	if got := memoryLimitFor(pod, "missing"); got != "" {
		t.Errorf("memoryLimitFor(missing) = %q, want empty", got)
	}
}

func TestRedactLong(t *testing.T) {
	if got := redactLong("short", 100); got != "short" {
		t.Errorf("redactLong short = %q", got)
	}
	in := strings.Repeat("x", 300)
	got := redactLong(in, 10)
	if len(got) != 13 { // 10 bytes + 3-byte ellipsis
		t.Errorf("redactLong len = %d, want 13", len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("redactLong should append ellipsis, got %q", got)
	}
}
