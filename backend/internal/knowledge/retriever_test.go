package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeReranker struct {
	// pickedIndices is returned verbatim when non-nil.
	pickedIndices []int
	// err is returned when set.
	err error
	// got records the RerankRequest for assertions.
	got *RerankRequest
}

func (f *fakeReranker) Rerank(ctx context.Context, req RerankRequest) (RerankResult, error) {
	f.got = &req
	if f.err != nil {
		return RerankResult{}, f.err
	}
	return RerankResult{PickedIndices: f.pickedIndices}, nil
}

func seedEntries(t *testing.T, repo Repository, n int, ruleID string) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	// Give each entry a unique first cause so the dedup rule keeps all n.
	for i := 0; i < n; i++ {
		cause := []string{"cause", "variant"}
		cause[0] = "cause-" + string(rune('a'+i))
		_, _ = repo.Insert(ctx, testEntry(0, ruleID, "high", "Deployment", "api", base.Add(time.Duration(i)*time.Hour), cause))
	}
}

func TestRetrieveStructuredOnly(t *testing.T) {
	repo := NewInMemoryRepository()
	seedEntries(t, repo, 5, "crash_loop")
	r := NewRetriever(repo, RetrieverConfig{ShortlistSize: 10, RerankEnabled: false, MaxResults: 3})

	got, err := r.Retrieve(context.Background(), Query{RuleID: "crash_loop", SummaryHint: ""}, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want MaxResults=3", len(got))
	}
	for _, e := range got {
		if e.RuleID != "crash_loop" {
			t.Fatalf("entry rule = %q, want crash_loop", e.RuleID)
		}
	}
}

func TestRetrieveWithRerankPicks(t *testing.T) {
	repo := NewInMemoryRepository()
	seedEntries(t, repo, 4, "crash_loop")
	r := NewRetriever(repo, RetrieverConfig{ShortlistSize: 10, RerankEnabled: true, MaxResults: 3})

	reranker := &fakeReranker{pickedIndices: []int{2, 0}}
	got, err := r.Retrieve(context.Background(), Query{RuleID: "crash_loop", SummaryHint: "pod crash"}, reranker)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (rerank picks)", len(got))
	}
	if reranker.got == nil {
		t.Fatal("reranker was not invoked")
	}
	if reranker.got.MaxPick != 3 {
		t.Fatalf("MaxPick = %d, want 3", reranker.got.MaxPick)
	}
	if !strings.Contains(reranker.got.CurrentContext, "pod crash") {
		t.Fatalf("CurrentContext = %q, want summary hint", reranker.got.CurrentContext)
	}
}

func TestRetrieveRerankFailureFallsBack(t *testing.T) {
	repo := NewInMemoryRepository()
	seedEntries(t, repo, 5, "crash_loop")
	r := NewRetriever(repo, RetrieverConfig{ShortlistSize: 10, RerankEnabled: true, MaxResults: 3})

	reranker := &fakeReranker{err: errors.New("LLM down")}
	got, err := r.Retrieve(context.Background(), Query{RuleID: "crash_loop", SummaryHint: "crash"}, reranker)
	if err != nil {
		t.Fatalf("Retrieve with failing rerank must not error, got %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want structured fallback 3", len(got))
	}
}

func TestRetrieveRerankEmptyPicksFallsBack(t *testing.T) {
	repo := NewInMemoryRepository()
	seedEntries(t, repo, 4, "crash_loop")
	r := NewRetriever(repo, RetrieverConfig{ShortlistSize: 10, RerankEnabled: true, MaxResults: 3})

	reranker := &fakeReranker{pickedIndices: []int{}}
	got, err := r.Retrieve(context.Background(), Query{RuleID: "crash_loop", SummaryHint: "crash"}, reranker)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("empty picks must fall back to shortlist, got %d", len(got))
	}
}

func TestRetrieveNoMatches(t *testing.T) {
	repo := NewInMemoryRepository()
	r := NewRetriever(repo, DefaultConfig())
	got, err := r.Retrieve(context.Background(), Query{RuleID: "missing_rule", SummaryHint: "x"}, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries for empty repo, want 0", len(got))
	}
}

func TestBuildPromptContext(t *testing.T) {
	base := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	entries := []Entry{
		{RuleID: "crash_loop", Severity: "high", RootCauses: []string{"image_pull_backoff"}, Recommendations: []string{"check registry"}, NotedAt: base},
		{RuleID: "crash_loop", Severity: "critical", RootCauses: []string{"OOMKilled"}, Recommendations: []string{"raise limits"}, NotedAt: base.Add(24 * time.Hour)},
	}
	out := BuildPromptContext(entries)
	if !strings.Contains(out, "crash_loop") || !strings.Contains(out, "image_pull_backoff") || !strings.Contains(out, "check registry") {
		t.Fatalf("prompt context missing content:\n%s", out)
	}
	if !strings.Contains(out, "[1]") || !strings.Contains(out, "[2]") {
		t.Fatalf("prompt context missing indices:\n%s", out)
	}

	if BuildPromptContext(nil) != "" {
		t.Fatal("empty entries must produce empty context")
	}
}

func TestRetrieverMinSeverityExcludesLow(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	_, _ = repo.Insert(ctx, testEntry(0, "crash_loop", "info", "Deployment", "api", base, []string{"idle"}))
	_, _ = repo.Insert(ctx, testEntry(0, "crash_loop", "high", "Deployment", "api", base.Add(1*time.Hour), []string{"real"}))

	r := NewRetriever(repo, RetrieverConfig{ShortlistSize: 10, RerankEnabled: false, MaxResults: 3})
	got, err := r.Retrieve(context.Background(), Query{RuleID: "crash_loop"}, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, e := range got {
		if e.Severity != "high" {
			t.Fatalf("entry severity = %q, want only high (min severity filter)", e.Severity)
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1 (info excluded)", len(got))
	}
}
