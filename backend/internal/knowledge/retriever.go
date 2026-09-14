package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Retriever finds historical knowledge entries relevant to a diagnosis context.
//
// Retrieval is a hybrid pipeline. The structured stage selects candidates from
// PostgreSQL by exact field match (rule_id + severity + resource kind, newest
// first), and the vector stage recalls candidates by embedding similarity over
// a loosely-filtered pool. The two ranked lists are merged with Reciprocal
// Rank Fusion, and the fused shortlist is then handed to the optional LLM
// re-ranker.
//
// Every stage after the first is additive and fail-open: a missing embedder,
// a failed embedding call or a failed re-rank all fall back to the previous
// stage's output, so a knowledge-layer fault can never block diagnosis.
type Retriever struct {
	repo     Repository
	config   RetrieverConfig
	embedder EmbeddingProvider
}

// WithEmbeddingProvider attaches a vector-stage embedder and returns the
// retriever, enabling the hybrid path. Existing callers of NewRetriever keep
// the structured-only behaviour unchanged, so attaching an embedder is an
// explicit opt-in rather than a default.
func (r *Retriever) WithEmbeddingProvider(provider EmbeddingProvider) *Retriever {
	r.embedder = provider
	return r
}

// vectorStageActive reports whether the vector stage will run for this query.
// Both an embedder and a query hint are required: without hint text there is
// nothing to embed, and falling through to the structured stage is strictly
// better than inventing a query vector.
func (r *Retriever) vectorStageActive(query Query) bool {
	return r.config.VectorEnabled &&
		r.embedder != nil &&
		strings.TrimSpace(query.SummaryHint) != ""
}

// RetrieverConfig controls retrieval behaviour.
type RetrieverConfig struct {
	// ShortlistSize is the number of candidates fetched in the structured
	// phase. The re-ranker narrows this further.
	ShortlistSize int
	// RerankEnabled gates the LLM re-rank call. When false, the shortlist
	// is returned as-is (zero LLM cost).
	RerankEnabled bool
	// MaxResults caps the final output sent to the caller.
	MaxResults int
	// VectorEnabled gates the vector (semantic) recall stage. It takes effect
	// only when an EmbeddingProvider is attached; otherwise the retriever
	// runs the original structured → re-rank path unchanged.
	VectorEnabled bool
	// VectorTopK bounds how many entries the vector stage contributes to the
	// fusion. It is intentionally a separate knob from ShortlistSize: the two
	// stages draw from different candidate pools.
	VectorTopK int
	// VectorCandidateSize bounds the loose-filter pool the vector stage scores.
	// Candidates are fetched with a severity floor only (no rule_id / resource
	// kind narrowing) so the stage can surface entries the structured stage
	// excluded — that exclusion is the recall gap this stage closes.
	VectorCandidateSize int
	// RRFRankConstant is the k in Reciprocal Rank Fusion, 1/(k+rank). 60 is
	// the value from the original RRF paper and is used when unset.
	RRFRankConstant int
}

// Query describes what to search for, sourced from the current diagnosis
// context.  Only fields with non-zero values narrow the search; zero-value
// fields are ignored.
type Query struct {
	RuleID       string
	Severity     string
	ResourceKind string
	// SummaryHint is the current diagnosis summary text, included for
	// context when the re-ranker is enabled.
	SummaryHint string
}

// RerankRequest is the prompt payload sent to the LLM re-ranker.
// The model picks the N most relevant entries from the shortlist.
type RerankRequest struct {
	CurrentContext string        `json:"current_context"`
	Shortlist      []RerankEntry `json:"shortlist"`
	MaxPick        int           `json:"max_pick"`
}

// RerankEntry is one candidate sent to the re-ranker for scoring.
type RerankEntry struct {
	Index           int      `json:"index"`
	RuleID          string   `json:"rule_id"`
	Severity        string   `json:"severity"`
	Summary         string   `json:"summary"`
	RootCauses      []string `json:"root_causes"`
	Recommendations []string `json:"recommendations"`
	NotedAt         string   `json:"noted_at"`
}

// RerankResult is the re-ranker's output: the ordered index list it picked.
type RerankResult struct {
	PickedIndices []int `json:"picked_indices"`
}

// Reranker ranks a shortlist of entries against the current diagnosis context.
// The re-ranker is optional: when nil, the structured shortlist is returned as-is.
type Reranker interface {
	Rerank(ctx context.Context, req RerankRequest) (RerankResult, error)
}

// DefaultConfig returns a config suitable for production use: shortlist 10,
// re-rank enabled, final 3, and the vector stage armed for callers that attach
// an embedder (it stays completely inert otherwise, so this default is safe
// for the structured-only path).
func DefaultConfig() RetrieverConfig {
	return RetrieverConfig{
		ShortlistSize:       10,
		RerankEnabled:       true,
		MaxResults:          3,
		VectorEnabled:       true,
		VectorTopK:          10,
		VectorCandidateSize: 200,
		RRFRankConstant:     60,
	}
}

// NewRetriever builds a knowledge retriever backed by the given repository.
// The vector stage stays disabled until WithEmbeddingProvider is called.
func NewRetriever(repo Repository, cfg RetrieverConfig) *Retriever {
	if cfg.ShortlistSize <= 0 {
		cfg.ShortlistSize = 10
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 3
	}
	if cfg.VectorTopK <= 0 {
		cfg.VectorTopK = 10
	}
	if cfg.VectorCandidateSize <= 0 {
		cfg.VectorCandidateSize = 200
	}
	if cfg.RRFRankConstant <= 0 {
		cfg.RRFRankConstant = 60
	}
	return &Retriever{repo: repo, config: cfg}
}

// Retrieve runs the hybrid pipeline and returns the final, ranked result set.
//
// Stages after the structured selection are additive and fail-open: a failed
// embedding call or a failed re-rank is silently dropped and the caller falls
// back to the previous stage's output, keeping the knowledge base
// non-blocking.
func (r *Retriever) Retrieve(ctx context.Context, query Query, reranker Reranker) ([]Entry, error) {
	// Stage 1: structured B-tree selection.
	structured, err := r.repo.ListByFilter(ctx, Filter{
		RuleID:       query.RuleID,
		Severity:     query.Severity,
		ResourceKind: query.ResourceKind,
		MinSeverity:  string(SeverityHigh),
		Limit:        r.config.ShortlistSize,
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge stage 1: %w", err)
	}

	// Stage 2: vector recall over a loosely-filtered pool. A failure here is
	// non-blocking by design: a broken embedder degrades retrieval to the
	// structured path rather than failing the diagnosis that asked for it.
	var vectorHits []Entry
	if r.vectorStageActive(query) {
		if hits, verr := r.vectorRecall(ctx, query); verr == nil {
			vectorHits = hits
		}
	}

	if len(structured.Items) == 0 && len(vectorHits) == 0 {
		return nil, nil // empty = no match, caller degrades silently
	}

	// Stage 3: fuse the two ranked lists. Reciprocal Rank Fusion needs no
	// score calibration between the stages — which is the point, because a
	// B-tree rank position and a cosine similarity are not comparable numbers
	// and any weighted blend of them would be an arbitrary constant.
	candidates := structured.Items
	if len(vectorHits) > 0 {
		candidates = fuseReciprocalRank(r.config.RRFRankConstant, structured.Items, vectorHits)
	}

	// Stage 4: LLM re-rank (optional; failure drops back to the fusion).
	if r.config.RerankEnabled && reranker != nil && query.SummaryHint != "" {
		shortlist := make([]RerankEntry, 0, len(candidates))
		for i, e := range candidates {
			shortlist = append(shortlist, RerankEntry{
				Index: i, RuleID: e.RuleID, Severity: e.Severity,
				Summary: e.Summary, RootCauses: e.RootCauses,
				Recommendations: e.Recommendations, NotedAt: e.NotedAt.Format("2006-01-02"),
			})
		}
		req := RerankRequest{CurrentContext: query.SummaryHint, Shortlist: shortlist, MaxPick: r.config.MaxResults}
		rerankResult, rerankErr := reranker.Rerank(ctx, req)
		if rerankErr == nil && len(rerankResult.PickedIndices) > 0 {
			picked := make([]Entry, 0, len(rerankResult.PickedIndices))
			for _, idx := range rerankResult.PickedIndices {
				if idx >= 0 && idx < len(candidates) {
					e := candidates[idx]
					e.Score = float64(1) - float64(idx)*0.1 // rank order → score
					picked = append(picked, e)
				}
			}
			if len(picked) > 0 {
				return picked, nil
			}
		}
		// re-rank failed or returned empty → fall back to the fused shortlist
	}

	// Return the fused/structured shortlist (truncate to MaxResults).
	n := len(candidates)
	if n > r.config.MaxResults {
		n = r.config.MaxResults
	}
	return candidates[:n], nil
}

// vectorRecall runs the vector stage: it embeds the query hint and the
// candidate corpus, then keeps the top-K by cosine similarity.
//
// Candidates are drawn with a severity floor only — no rule_id or resource
// kind narrowing — because that narrowing is precisely the recall gap this
// stage exists to close. A case whose rule id changed, or whose resource kind
// is recorded differently, is unreachable by exact match but is still a
// neighbour in vector space.
//
// This is an application-layer implementation: the corpus is scored in Go
// after being fetched. That keeps the stage free of any database extension or
// ANN index, at the cost of bounding the corpus by VectorCandidateSize. A
// deployment beyond that scale needs materialised vectors and an index — see
// the package documentation for the boundary.
func (r *Retriever) vectorRecall(ctx context.Context, query Query) ([]Entry, error) {
	pool, err := r.repo.ListByFilter(ctx, Filter{
		MinSeverity: string(SeverityHigh),
		Limit:       r.config.VectorCandidateSize,
	})
	if err != nil {
		return nil, fmt.Errorf("knowledge vector pool: %w", err)
	}
	if len(pool.Items) == 0 {
		return nil, nil
	}

	texts := make([]string, 0, len(pool.Items)+1)
	texts = append(texts, query.SummaryHint)
	for _, e := range pool.Items {
		texts = append(texts, e.EmbeddingText())
	}
	vectors, err := r.embedder.Embed(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("knowledge vector embed: %w", err)
	}
	if len(vectors) != len(texts) {
		return nil, fmt.Errorf("knowledge vector embed: provider returned %d vectors for %d texts", len(vectors), len(texts))
	}

	queryVector := vectors[0]
	scored := make([]Entry, 0, len(pool.Items))
	for i, e := range pool.Items {
		e.Score = CosineSimilarity(queryVector, vectors[i+1])
		scored = append(scored, e)
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > r.config.VectorTopK {
		scored = scored[:r.config.VectorTopK]
	}
	return scored, nil
}

// fuseReciprocalRank merges ranked lists with Reciprocal Rank Fusion:
// every list contributes 1/(k+rank) to each document it ranks, and the merged
// order is by descending total. Documents missing from a list simply earn
// nothing from it, so an entry ranked highly by either stage survives.
//
// Ties break on first-seen order, which keeps the fusion deterministic — a
// property the offline benchmark relies on.
func fuseReciprocalRank(k int, lists ...[]Entry) []Entry {
	if k <= 0 {
		k = 60
	}
	scores := make(map[int64]float64)
	byID := make(map[int64]Entry)
	order := make([]int64, 0)
	for _, list := range lists {
		for rank, entry := range list {
			if _, seen := byID[entry.ID]; !seen {
				byID[entry.ID] = entry
				order = append(order, entry.ID)
			}
			scores[entry.ID] += 1 / float64(k+rank+1)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	fused := make([]Entry, 0, len(order))
	for _, id := range order {
		entry := byID[id]
		entry.Score = scores[id]
		fused = append(fused, entry)
	}
	return fused
}

// BuildPromptContext renders the retrieved entries into a text block that can
// be injected into an AI prompt (aiexplain / aiinvestigator).
func BuildPromptContext(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## 历史相似案例（经验证的根因 + 处置措施）\n\n")
	for i, e := range entries {
		causes := strings.Join(e.RootCauses, "; ")
		recs := strings.Join(e.Recommendations, "; ")
		fmt.Fprintf(&sb, "[%d] RuleID=%s | Severity=%s | RootCause=%s | Recommendation=%s | Resolved %s\n",
			i+1, e.RuleID, e.Severity, causes, recs, e.NotedAt.Format("2006-01-02"))
	}
	return sb.String()
}

// MustParseRerankResult is a test helper that parses a rerank result from JSON.
func MustParseRerankResult(data []byte) RerankResult {
	var out RerankResult
	_ = json.Unmarshal(data, &out)
	return out
}
