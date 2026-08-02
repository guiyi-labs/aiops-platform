package pdb

import (
	"testing"
	"time"
)

func testTime() time.Time {
	return time.Date(2026, 8, 2, 14, 0, 0, 0, time.UTC)
}

func protectedWorkload() WorkloadRef {
	return WorkloadRef{Kind: "Deployment", Namespace: "prod", Name: "web", UID: "u-web", Replicas: 3, Labels: map[string]string{"app": "web"}}
}

func matchingPDB() PDBInfo {
	return PDBInfo{
		Namespace:          "prod",
		Name:               "web-pdb",
		UID:                "u-pdb",
		MinAvailable:       "1",
		SelectorLabels:     map[string]string{"app": "web"},
		ExpectedPods:       3,
		DisruptionsAllowed: 2,
	}
}

// TestEvaluate_ProtectedWorkload_NoFindings: a workload covered by a healthy
// PDB (budget achievable, disruptions allowed) produces zero findings.
func TestEvaluate_ProtectedWorkload_NoFindings(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{protectedWorkload()}, PDBs: []PDBInfo{matchingPDB()}}
	s := Evaluate(7, in, testTime())

	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0; findings=%+v", s.Failed, s.Findings)
	}
	if s.WorkloadsTotal != 1 || s.PDBsTotal != 1 {
		t.Fatalf("counters = %d/%d, want 1/1", s.WorkloadsTotal, s.PDBsTotal)
	}
	if s.UnprotectedWorkloads != 0 {
		t.Fatalf("unprotected_workloads = %d, want 0", s.UnprotectedWorkloads)
	}
	// Total = workloads checks (1) + PDB checks (2 per PDB: selector + budget/disruptions)
	if s.Total != 1+2 {
		t.Fatalf("total = %d, want 3", s.Total)
	}
}

// TestEvaluate_UnprotectedWorkload: a replicable workload with no matching PDB
// in its namespace is flagged.
func TestEvaluate_UnprotectedWorkload(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{protectedWorkload()}, PDBs: []PDBInfo{}}
	s := Evaluate(7, in, testTime())

	if s.UnprotectedWorkloads != 1 {
		t.Fatalf("unprotected_workloads = %d, want 1", s.UnprotectedWorkloads)
	}
	if s.Failed != 1 {
		t.Fatalf("failed = %d, want 1", s.Failed)
	}
	if s.Findings[0].Code != CodeWorkloadUnprotected {
		t.Fatalf("finding code = %s, want %s", s.Findings[0].Code, CodeWorkloadUnprotected)
	}
}

// TestEvaluate_SelectorMismatch: a PDB that does not match the workload labels
// does not protect it.
func TestEvaluate_SelectorMismatch(t *testing.T) {
	in := Inputs{
		Workloads: []WorkloadRef{protectedWorkload()},
		PDBs: []PDBInfo{{
			Namespace: "prod", Name: "other-pdb", UID: "u-other",
			MinAvailable: "1", SelectorLabels: map[string]string{"app": "other"},
			ExpectedPods: 3, DisruptionsAllowed: 2,
		}},
	}
	s := Evaluate(7, in, testTime())

	if s.UnprotectedWorkloads != 1 {
		t.Fatalf("unprotected_workloads = %d, want 1 (selector mismatch)", s.UnprotectedWorkloads)
	}
}

// TestEvaluate_BudgetUnachievable: minAvailable >= expectedPods warns.
func TestEvaluate_BudgetUnachievable(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{protectedWorkload()}, PDBs: []PDBInfo{{
		Namespace: "prod", Name: "web-pdb", UID: "u-pdb",
		MinAvailable: "3", SelectorLabels: map[string]string{"app": "web"},
		ExpectedPods: 3, DisruptionsAllowed: 0,
	}}}
	s := Evaluate(7, in, testTime())

	var sawUnachievable, sawBlocked bool
	for _, f := range s.Findings {
		switch f.Code {
		case CodeBudgetUnachievable:
			sawUnachievable = true
		case CodeDisruptionsBlocked:
			sawBlocked = true
		}
	}
	if !sawUnachievable {
		t.Fatalf("no budget-unachievable finding; all=%+v", s.Findings)
	}
	if !sawBlocked {
		t.Fatalf("no disruptions-blocked finding; all=%+v", s.Findings)
	}
}

// TestEvaluate_DisruptionsBlocked: disruptionsAllowed == 0 warns even when
// the budget is otherwise fine.
func TestEvaluate_DisruptionsBlocked(t *testing.T) {
	in := Inputs{PDBs: []PDBInfo{{
		Namespace: "prod", Name: "web-pdb", UID: "u-pdb",
		MinAvailable: "1", SelectorLabels: map[string]string{"app": "web"},
		ExpectedPods: 3, DisruptionsAllowed: 0,
	}}}
	s := Evaluate(7, in, testTime())

	sawBlocked := false
	for _, f := range s.Findings {
		if f.Code == CodeDisruptionsBlocked {
			sawBlocked = true
		}
	}
	if !sawBlocked {
		t.Fatalf("no disruptions-blocked finding; all=%+v", s.Findings)
	}
}

// TestEvaluate_EmptySelector: a PDB with no selector labels protects nothing.
func TestEvaluate_EmptySelector(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{protectedWorkload()}, PDBs: []PDBInfo{{
		Namespace: "prod", Name: "empty-pdb", UID: "u-empty",
		MinAvailable: "1", ExpectedPods: 3, DisruptionsAllowed: 2,
	}}}
	s := Evaluate(7, in, testTime())

	if s.UnprotectedWorkloads != 1 {
		t.Fatalf("unprotected_workloads = %d, want 1 (empty PDB selector protects nothing)", s.UnprotectedWorkloads)
	}
	sawEmpty := false
	for _, f := range s.Findings {
		if f.Code == CodeSelectorNoMatches {
			sawEmpty = true
		}
	}
	if !sawEmpty {
		t.Fatalf("no selector-no-matches finding; all=%+v", s.Findings)
	}
}

// TestEvaluate_SingleReplicaWorkload_Skipped: replicas <= 1 are not expected
// to have a PDB.
func TestEvaluate_SingleReplicaWorkload_Skipped(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{{
		Kind: "Deployment", Namespace: "prod", Name: "solo", UID: "u-solo", Replicas: 1,
		Labels: map[string]string{"app": "solo"},
	}}}
	s := Evaluate(7, in, testTime())

	if s.UnprotectedWorkloads != 0 {
		t.Fatalf("unprotected_workloads = %d, want 0 (single replica)", s.UnprotectedWorkloads)
	}
	if s.Failed != 0 {
		t.Fatalf("failed = %d, want 0", s.Failed)
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

// TestEvaluate_SortDeterministic: same input yields the same finding order.
func TestEvaluate_SortDeterministic(t *testing.T) {
	in := Inputs{Workloads: []WorkloadRef{
		{Kind: "Deployment", Namespace: "b", Name: "z", UID: "1", Replicas: 2, Labels: map[string]string{"a": "z"}},
		{Kind: "Deployment", Namespace: "a", Name: "y", UID: "2", Replicas: 2, Labels: map[string]string{"a": "y"}},
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

// TestBudgetValue: integer and percentage parsing.
func TestBudgetValue(t *testing.T) {
	cases := []struct {
		raw string
		v   int
		ok  bool
	}{
		{"1", 1, true},
		{"50%", 50, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-1", 0, false},
	}
	for _, c := range cases {
		v, ok := budgetValue(c.raw)
		if v != c.v || ok != c.ok {
			t.Errorf("budgetValue(%q) = (%d,%v), want (%d,%v)", c.raw, v, ok, c.v, c.ok)
		}
	}
}
