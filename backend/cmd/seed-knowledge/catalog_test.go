package main

import (
	"testing"
	"time"
)

func TestBuildSeedEntriesCount(t *testing.T) {
	entries := buildSeedEntries(time.Now())
	if len(entries) == 0 {
		t.Fatal("expected at least one seed entry")
	}
	if len(entries) > 20 {
		t.Fatalf("too many seed entries (%d); keep the demo focused", len(entries))
	}
}

func TestBuildSeedEntriesAllRequiredFieldsPopulated(t *testing.T) {
	entries := buildSeedEntries(time.Now())
	for i, e := range entries {
		if e.RuleID == "" {
			t.Errorf("[%d] RuleID empty", i)
		}
		if e.Severity == "" {
			t.Errorf("[%d] Severity empty", i)
		}
		if e.ResourceKind == "" {
			t.Errorf("[%d] ResourceKind empty", i)
		}
		if e.ResourceName == "" {
			t.Errorf("[%d] ResourceName empty", i)
		}
		if e.Summary == "" {
			t.Errorf("[%d] Summary empty", i)
		}
		if len(e.RootCauses) == 0 {
			t.Errorf("[%d] RootCauses empty", i)
		}
		if len(e.Recommendations) == 0 {
			t.Errorf("[%d] Recommendations empty", i)
		}
	}
}

func TestBuildSeedEntriesNoDuplicates(t *testing.T) {
	entries := buildSeedEntries(time.Now())
	type key struct {
		ruleID       string
		resourceKind string
		resourceName string
	}
	seen := map[key]bool{}
	for i, e := range entries {
		k := key{e.RuleID, e.ResourceKind, e.ResourceName}
		if seen[k] {
			t.Errorf("[%d] duplicate key: %s/%s/%s", i, e.RuleID, e.ResourceKind, e.ResourceName)
		}
		seen[k] = true
	}
}

func TestBuildSeedEntriesDeterministicOrdering(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	a := buildSeedEntries(base)
	b := buildSeedEntries(base)
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].RuleID != b[i].RuleID {
			t.Errorf("[%d] non-deterministic: %q vs %q", i, a[i].RuleID, b[i].RuleID)
		}
		if a[i].NotedAt != b[i].NotedAt {
			t.Errorf("[%d] noted_at mismatch: %v vs %v", i, a[i].NotedAt, b[i].NotedAt)
		}
	}
}

func TestBuildSeedEntriesNotedAtStaggered(t *testing.T) {
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	entries := buildSeedEntries(base)
	for i := 1; i < len(entries); i++ {
		if !entries[i].NotedAt.After(entries[i-1].NotedAt) {
			t.Errorf("[%d] noted_at not staggered after [%d]", i, i-1)
		}
	}
}

func TestSeedProvenanceMarker(t *testing.T) {
	if seedProvenanceMarker == "" {
		t.Fatal("seedProvenanceMarker must not be empty")
	}
}
