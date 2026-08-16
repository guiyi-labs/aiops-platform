package knowledge

import (
	"context"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
)

func TestDiagnosisIngesterPersistsEntry(t *testing.T) {
	repo := NewInMemoryRepository()
	ingester := NewDiagnosisIngester(repo)
	ctx := context.Background()
	noted := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

	input := diagnosis.KnowledgeEntryInput{
		SourceDiagnosisID: 7,
		RuleID:            "crash_loop",
		Severity:          "high",
		ResourceKind:      "Deployment",
		ResourceNamespace: "payments",
		ResourceName:      "api",
		Summary:           "api pod crash looping",
		RootCauses:        []string{"image_pull_backoff"},
		Recommendations:   []string{"check registry"},
		NotedAt:           noted,
	}
	if err := ingester.IngestResolved(ctx, input); err != nil {
		t.Fatalf("IngestResolved: %v", err)
	}

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	res, err := repo.ListByFilter(ctx, Filter{RuleID: "crash_loop", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(res.Items))
	}
	got := res.Items[0]
	if got.SourceDiagnosisID != 7 || got.Summary != "api pod crash looping" {
		t.Fatalf("entry = %#v", got)
	}
	if len(got.Recommendations) != 1 || got.Recommendations[0] != "check registry" {
		t.Fatalf("recommendations = %#v", got.Recommendations)
	}
}

func TestDiagnosisIngesterDedupPairing(t *testing.T) {
	// Wire the real service hook with both packages to prove the round trip:
	// diagnosis.Transition → IngestResolvedIfEligible → DiagnosisIngester.
	repo := NewInMemoryRepository()
	ctx := context.Background()
	ingester := NewDiagnosisIngester(repo)

	resolvedAt := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	record := diagnosis.Record{
		ID: 7, RuleID: "crash_loop", Severity: "high", Status: "resolved",
		Resource:        diagnosis.ResourceRef{Kind: "Deployment", Namespace: "payments", Name: "api"},
		Summary:         "crash one",
		RootCauses:      []string{"image_pull_backoff"},
		Recommendations: []string{"r1"},
		ResolvedAt:      &resolvedAt,
	}
	diagnosis.IngestResolvedIfEligible(ctx, ingester, record, time.Now)

	// A second resolved record with the same defect collapses via dedup.
	record2 := record
	record2.ID = 8
	record2.Summary = "crash two"
	later := resolvedAt.Add(time.Hour)
	record2.ResolvedAt = &later
	diagnosis.IngestResolvedIfEligible(ctx, ingester, record2, time.Now)

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1 after dedup", count)
	}
	res, _ := repo.ListByFilter(ctx, Filter{RuleID: "crash_loop", Limit: 10})
	if len(res.Items) != 1 || res.Items[0].Summary != "crash two" {
		t.Fatalf("deduped entry = %#v, want newest summary", res.Items[0])
	}
}
