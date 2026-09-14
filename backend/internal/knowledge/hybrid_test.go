package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeEmbeddingProvider lets the failure paths be exercised deterministically.
type fakeEmbeddingProvider struct {
	dim      int
	embedErr error
	// dropVectors, when > 0, returns fewer vectors than texts were supplied —
	// the shape a truncated upstream response takes.
	dropVectors int
	calls       int
}

func (f *fakeEmbeddingProvider) Dimension() int { return f.dim }
func (f *fakeEmbeddingProvider) Name() string   { return "fake" }

func (f *fakeEmbeddingProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	if f.embedErr != nil {
		return nil, f.embedErr
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1, 0, 0}
	}
	if f.dropVectors > 0 && f.dropVectors < len(out) {
		out = out[:len(out)-f.dropVectors]
	}
	return out, nil
}

// seedHybridCorpus writes two entries that a human would call unrelated, each
// recorded under a rule id the hybrid query below does not use.
func seedHybridCorpus(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	_, _ = repo.Insert(ctx, Entry{
		RuleID: "pod.memory_exceeded.v2", Severity: "high", ResourceKind: "Pod",
		ResourceName: "api-0", Summary: "Pod 容器因内存超限被 OOMKilled 并反复重启",
		RootCauses: []string{"memory limit too low"}, NotedAt: base,
	})
	_, _ = repo.Insert(ctx, Entry{
		RuleID: "service.dns_failure.v1", Severity: "high", ResourceKind: "Service",
		ResourceName: "api-svc", Summary: "Service 的 DNS 解析失败导致调用方连接超时",
		RootCauses: []string{"coredns unavailable"}, NotedAt: base.Add(time.Hour),
	})
}

// TestVectorStageRecallsWhatStructuredMisses is the load-bearing test for the
// hybrid pipeline: the exact-match stage finds nothing because the rule id in
// the query is not the rule id the case was recorded under, and only the
// vector stage bridges the gap.
func TestVectorStageRecallsWhatStructuredMisses(t *testing.T) {
	ctx := context.Background()
	query := Query{RuleID: "pod.oom_killed.v1", SummaryHint: "Pod 因内存超限被 OOMKilled，容器反复重启"}

	t.Run("structured only finds nothing", func(t *testing.T) {
		repo := NewInMemoryRepository()
		seedHybridCorpus(t, repo)
		r := NewRetriever(repo, DefaultConfig())
		got, err := r.Retrieve(ctx, query, nil)
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("structured-only got %d entries, want 0 (rule id is absent)", len(got))
		}
	})

	t.Run("vector stage recovers the related case", func(t *testing.T) {
		repo := NewInMemoryRepository()
		seedHybridCorpus(t, repo)
		r := NewRetriever(repo, DefaultConfig()).WithEmbeddingProvider(NewLexicalEmbeddingProvider(512))
		got, err := r.Retrieve(ctx, query, nil)
		if err != nil {
			t.Fatalf("Retrieve: %v", err)
		}
		if len(got) == 0 {
			t.Fatal("hybrid retrieval returned nothing; the vector stage did not run or scored everything out")
		}
		if got[0].RuleID != "pod.memory_exceeded.v2" {
			t.Fatalf("top entry rule = %q, want the wording-matched pod.memory_exceeded.v2", got[0].RuleID)
		}
	})
}

func TestVectorStageGatedByConfiguration(t *testing.T) {
	ctx := context.Background()
	base := RetrieverConfig{ShortlistSize: 10, MaxResults: 3, VectorTopK: 10, VectorCandidateSize: 50, RRFRankConstant: 60}

	cases := []struct {
		name       string
		config     RetrieverConfig
		provider   EmbeddingProvider
		hint       string
		wantCalled bool
	}{
		{
			name: "no provider attached", config: base,
			provider: nil, hint: "Pod OOMKilled", wantCalled: false,
		},
		{
			name: "explicitly disabled", config: RetrieverConfig{ShortlistSize: 10, MaxResults: 3, VectorEnabled: false},
			provider: &fakeEmbeddingProvider{dim: 3}, hint: "Pod OOMKilled", wantCalled: false,
		},
		{
			name: "no summary hint to embed", config: base,
			provider: &fakeEmbeddingProvider{dim: 3, calls: 0}, hint: "   ", wantCalled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewInMemoryRepository()
			seedHybridCorpus(t, repo)
			r := NewRetriever(repo, tc.config)
			if tc.provider != nil {
				r = r.WithEmbeddingProvider(tc.provider)
			}
			if _, err := r.Retrieve(ctx, Query{RuleID: "pod.memory_exceeded.v2", SummaryHint: tc.hint}, nil); err != nil {
				t.Fatalf("Retrieve: %v", err)
			}
			fake, ok := tc.provider.(*fakeEmbeddingProvider)
			if !ok {
				return
			}
			if called := fake.calls > 0; called != tc.wantCalled {
				t.Fatalf("embedder called = %v, want %v", called, tc.wantCalled)
			}
		})
	}
}

// A knowledge-layer fault must never reach the caller: both embedding failure
// shapes fall back to the structured result rather than surfacing an error.
func TestVectorStageFailuresFallBackToStructured(t *testing.T) {
	ctx := context.Background()
	query := Query{RuleID: "pod.memory_exceeded.v2", SummaryHint: "Pod 内存超限 OOMKilled"}

	cases := []struct {
		name     string
		provider *fakeEmbeddingProvider
	}{
		{"provider error", &fakeEmbeddingProvider{dim: 3, embedErr: errors.New("embedding endpoint down")}},
		{"truncated vector list", &fakeEmbeddingProvider{dim: 3, dropVectors: 1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewInMemoryRepository()
			seedHybridCorpus(t, repo)
			r := NewRetriever(repo, DefaultConfig()).WithEmbeddingProvider(tc.provider)

			got, err := r.Retrieve(ctx, query, nil)
			if err != nil {
				t.Fatalf("a failing vector stage must not error, got %v", err)
			}
			if len(got) == 0 {
				t.Fatal("structured fallback returned nothing")
			}
			if got[0].RuleID != "pod.memory_exceeded.v2" {
				t.Fatalf("fallback top entry = %q, want the structured match", got[0].RuleID)
			}
		})
	}
}

// TestVectorStageTopKBoundsContribution pins the knob that keeps the vector
// stage from flooding the fusion with loosely-related cases.
func TestVectorStageTopKBoundsContribution(t *testing.T) {
	repo := NewInMemoryRepository()
	seedHybridCorpus(t, repo)
	cfg := DefaultConfig()
	cfg.VectorTopK = 1
	r := NewRetriever(repo, cfg).WithEmbeddingProvider(NewLexicalEmbeddingProvider(512))

	got, err := r.Retrieve(context.Background(),
		Query{RuleID: "pod.oom_killed.v1", SummaryHint: "Pod 因内存超限被 OOMKilled，容器反复重启"}, nil)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1 (VectorTopK=1 and no structured match)", len(got))
	}
}

// TestHybridFusionFeedsReranker proves the fusion is real rather than
// cosmetic: an entry the structured stage could not reach must be visible to
// the re-ranker, which then selects it.
func TestHybridFusionFeedsReranker(t *testing.T) {
	repo := NewInMemoryRepository()
	seedHybridCorpus(t, repo)
	r := NewRetriever(repo, DefaultConfig()).WithEmbeddingProvider(NewLexicalEmbeddingProvider(512))

	// Pick the last fusion slot, which is where the vector-only entry lands.
	reranker := &fakeReranker{pickedIndices: []int{1}}
	got, err := r.Retrieve(context.Background(),
		Query{RuleID: "pod.oom_killed.v1", SummaryHint: "Pod 因内存超限被 OOMKilled，容器反复重启"}, reranker)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if reranker.got == nil {
		t.Fatal("re-ranker was never invoked")
	}
	if len(reranker.got.Shortlist) < 2 {
		t.Fatalf("re-ranker shortlist has %d entries, want the fused pool (>=2)", len(reranker.got.Shortlist))
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want the single re-rank pick", len(got))
	}
}

func TestFuseReciprocalRank(t *testing.T) {
	a := Entry{ID: 1, RuleID: "a"}
	b := Entry{ID: 2, RuleID: "b"}
	c := Entry{ID: 3, RuleID: "c"}

	t.Run("entry ranked by both lists wins", func(t *testing.T) {
		fused := fuseReciprocalRank(60, []Entry{a, b}, []Entry{b, c})
		got := []string{fused[0].RuleID, fused[1].RuleID, fused[2].RuleID}
		want := []string{"b", "a", "c"}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("fused order = %v, want %v", got, want)
			}
		}
		if len(fused) != 3 {
			t.Fatalf("fused %d entries, want 3 (no duplicates)", len(fused))
		}
		if fused[0].Score <= fused[2].Score {
			t.Fatalf("fused scores not descending: %v", []float64{fused[0].Score, fused[2].Score})
		}
	})

	t.Run("ties keep first-seen order", func(t *testing.T) {
		fused := fuseReciprocalRank(60, []Entry{a}, []Entry{b})
		if fused[0].RuleID != "a" || fused[1].RuleID != "b" {
			t.Fatalf("tie order = %v, want stable first-seen [a b]",
				[]string{fused[0].RuleID, fused[1].RuleID})
		}
	})

	t.Run("degenerate inputs", func(t *testing.T) {
		if got := fuseReciprocalRank(60); len(got) != 0 {
			t.Fatalf("no lists → %d entries, want 0", len(got))
		}
		fused := fuseReciprocalRank(0, []Entry{a}, []Entry{b}) // k<=0 falls back to 60
		if len(fused) != 2 {
			t.Fatalf("got %d entries, want 2", len(fused))
		}
		// An empty structured list must not suppress the vector list.
		fused = fuseReciprocalRank(60, nil, []Entry{a, b})
		if len(fused) != 2 || fused[0].RuleID != "a" {
			t.Fatalf("nil list mishandled: %+v", fused)
		}
	})
}
