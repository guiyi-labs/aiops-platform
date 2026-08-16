package knowledge

import (
	"context"
	"testing"
	"time"
)

func testEntry(id int64, ruleID, severity, kind, name string, noted time.Time, causes []string) Entry {
	return Entry{
		ID: id, RuleID: ruleID, Severity: severity, ResourceKind: kind,
		ResourceName: name, RootCauses: causes,
		Recommendations: []string{"check " + name},
		Summary:         ruleID + " on " + kind + "/" + name,
		NotedAt:         noted,
	}
}

func TestInMemoryRepositoryInsertAndDedup(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)

	// A recurring defect on the same resource + same root cause collapses.
	one := testEntry(0, "crash_loop", "high", "Deployment", "api", base, []string{"image_pull_backoff"})
	first, err := repo.Insert(ctx, one)
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if first.ID != 1 {
		t.Fatalf("first id = %d, want 1", first.ID)
	}
	two := one
	two.SourceDiagnosisID = 99
	two.NotedAt = base.Add(24 * time.Hour)
	second, err := repo.Insert(ctx, two)
	if err != nil {
		t.Fatalf("insert dedup: %v", err)
	}
	if second.ID != 1 {
		t.Fatalf("dedup id = %d, want 1 (replaced, not new)", second.ID)
	}
	if second.SourceDiagnosisID != 99 {
		t.Fatalf("source diagnosis = %d, want 99 (newest wins)", second.SourceDiagnosisID)
	}

	// A different root cause on the same resource is a distinct entry.
	three := testEntry(0, "crash_loop", "high", "Deployment", "api", base.Add(48*time.Hour), []string{"OOMKilled"})
	third, err := repo.Insert(ctx, three)
	if err != nil {
		t.Fatalf("insert distinct: %v", err)
	}
	if third.ID != 2 {
		t.Fatalf("distinct id = %d, want 2", third.ID)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestInMemoryRepositoryFilter(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)

	_, _ = repo.Insert(ctx, testEntry(0, "crash_loop", "high", "Deployment", "api", base.Add(1*time.Hour), []string{"image_pull_backoff"}))
	_, _ = repo.Insert(ctx, testEntry(0, "metric_breach", "warning", "Node", "worker-a", base, []string{"cpu_idle"}))

	// Rule filter.
	byRule, err := repo.ListByFilter(ctx, Filter{RuleID: "crash_loop", Limit: 10})
	if err != nil {
		t.Fatalf("list by rule: %v", err)
	}
	if len(byRule.Items) != 1 || byRule.Items[0].RuleID != "crash_loop" {
		t.Fatalf("rule filter items = %#v", byRule.Items)
	}

	// MinSeverity keeps high+ only.
	byMin, err := repo.ListByFilter(ctx, Filter{MinSeverity: string(SeverityHigh), Limit: 10})
	if err != nil {
		t.Fatalf("list by min severity: %v", err)
	}
	if len(byMin.Items) != 1 || byMin.Items[0].Severity != "high" {
		t.Fatalf("min severity items = %#v", byMin.Items)
	}

	// Severity exact.
	byWarning, err := repo.ListByFilter(ctx, Filter{Severity: string(SeverityWarning), Limit: 10})
	if err != nil {
		t.Fatalf("list by severity: %v", err)
	}
	if len(byWarning.Items) != 1 || byWarning.Items[0].RuleID != "metric_breach" {
		t.Fatalf("warning items = %#v", byWarning.Items)
	}
}

func TestInMemoryRepositoryLimitAndTruncated(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		cause := []string{"cause-a", "variant"} // placeholder replaced below
		cause[0] = "cause-" + string(rune('a'+i))
		_, _ = repo.Insert(ctx, testEntry(0, "crash_loop", "high", "Deployment", "api", base.Add(time.Duration(i)*time.Hour), cause))
	}
	res, err := repo.ListByFilter(ctx, Filter{RuleID: "crash_loop", Limit: 3})
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(res.Items) != 3 {
		t.Fatalf("limited items = %d, want 3", len(res.Items))
	}
	if !res.Truncated {
		t.Fatal("Truncated = false, want true")
	}
}
