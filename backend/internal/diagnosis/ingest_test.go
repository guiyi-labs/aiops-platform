package diagnosis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingIngester struct {
	calls []KnowledgeEntryInput
	err   error
}

func (r *recordingIngester) IngestResolved(ctx context.Context, input KnowledgeEntryInput) error {
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, input)
	return nil
}

func testResolvedRecord() Record {
	resolvedAt := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	return Record{
		ID:              7,
		RuleID:          "crash_loop",
		Severity:        "high",
		Status:          "resolved",
		Resource:        ResourceRef{Kind: "Deployment", Namespace: "payments", Name: "api"},
		Summary:         "api pod crash looping",
		RootCauses:      []string{"image_pull_backoff"},
		Recommendations: []string{"check registry credentials", "bump image tag"},
		ResolvedAt:      &resolvedAt,
	}
}

func TestIngestResolvedIfEligibleWrites(t *testing.T) {
	ingester := &recordingIngester{}
	ctx := context.Background()
	now := func() time.Time { return time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC) }

	IngestResolvedIfEligible(ctx, ingester, testResolvedRecord(), now)
	if len(ingester.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(ingester.calls))
	}
	got := ingester.calls[0]
	if got.SourceDiagnosisID != 7 {
		t.Fatalf("source id = %d, want 7", got.SourceDiagnosisID)
	}
	if got.RuleID != "crash_loop" || got.ResourceKind != "Deployment" || got.ResourceName != "api" {
		t.Fatalf("distilled fields = %#v", got)
	}
	if len(got.RootCauses) != 1 || got.RootCauses[0] != "image_pull_backoff" {
		t.Fatalf("root causes = %#v", got.RootCauses)
	}
	if len(got.Recommendations) != 2 {
		t.Fatalf("recommendations = %#v", got.Recommendations)
	}
	// resolved_at wins over now().
	if !got.NotedAt.Equal((*testResolvedRecord().ResolvedAt)) {
		t.Fatalf("noted_at = %v, want resolved_at", got.NotedAt)
	}
}

func TestIngestResolvedIfEligibleSkipsNilIngester(t *testing.T) {
	ctx := context.Background()
	// A nil ingester must be a no-op (no panic).
	IngestResolvedIfEligible(ctx, nil, testResolvedRecord(), time.Now)
}

func TestIngestResolvedIfEligibleSkipsNonResolved(t *testing.T) {
	ingester := &recordingIngester{}
	ctx := context.Background()
	record := testResolvedRecord()
	record.Status = "open"
	IngestResolvedIfEligible(ctx, ingester, record, time.Now)
	if len(ingester.calls) != 0 {
		t.Fatalf("calls = %d, want 0 for open record", len(ingester.calls))
	}
}

func TestIngestResolvedIfEligibleSkipsEmptyContent(t *testing.T) {
	ingester := &recordingIngester{}
	ctx := context.Background()
	record := testResolvedRecord()
	record.RootCauses = nil
	record.Recommendations = nil
	IngestResolvedIfEligible(ctx, ingester, record, time.Now)
	if len(ingester.calls) != 0 {
		t.Fatalf("calls = %d, want 0 for empty content", len(ingester.calls))
	}
}

func TestIngestResolvedSwallowsErrors(t *testing.T) {
	ingester := &recordingIngester{err: errors.New("knowledge down")}
	ctx := context.Background()
	// Must not panic and must not propagate — the transition stays intact.
	IngestResolvedIfEligible(ctx, ingester, testResolvedRecord(), time.Now)
}

func TestTruncateSummary(t *testing.T) {
	short := "ok"
	if got := truncateSummary(short); got != short {
		t.Fatalf("short summary mutated: %q", got)
	}
	long := strings.Repeat("x", 500)
	got := truncateSummary(long)
	if len([]rune(got)) > 200+1 {
		t.Fatalf("truncated summary length = %d, want <= 201", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated summary missing ellipsis: %q", got)
	}
}

func TestNotedAtFallbackToNow(t *testing.T) {
	now := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	record := testResolvedRecord()
	record.ResolvedAt = nil
	if got := notedAtForKnowledge(record, func() time.Time { return now }); !got.Equal(now) {
		t.Fatalf("noted_at = %v, want now fallback", got)
	}
}
