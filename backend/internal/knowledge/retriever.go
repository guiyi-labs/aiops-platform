package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Retriever finds historical knowledge entries relevant to a diagnosis context.
// Phase 1 (this implementation) uses structured B-tree selection.
// Phase 2 (future, pgvector) will add a semantic embedding stage.
type Retriever struct {
	repo   Repository
	config RetrieverConfig
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
// re-rank enabled, final 3.
func DefaultConfig() RetrieverConfig {
	return RetrieverConfig{ShortlistSize: 10, RerankEnabled: true, MaxResults: 3}
}

// NewRetriever builds a knowledge retriever backed by the given repository.
func NewRetriever(repo Repository, cfg RetrieverConfig) *Retriever {
	if cfg.ShortlistSize <= 0 {
		cfg.ShortlistSize = 10
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 3
	}
	return &Retriever{repo: repo, config: cfg}
}

// Retrieve runs the two-stage pipeline and returns the final, ranked result
// set.  A failed re-rank is silently dropped (caller falls back to the
// structured shortlist), keeping the knowledge base non-blocking.
func (r *Retriever) Retrieve(ctx context.Context, query Query, reranker Reranker) ([]Entry, error) {
	// Stage 1: structured B-tree selection.
	filter := Filter{
		RuleID:       query.RuleID,
		Severity:     query.Severity,
		ResourceKind: query.ResourceKind,
		MinSeverity:  string(SeverityHigh),
		Limit:        r.config.ShortlistSize,
	}
	resp, err := r.repo.ListByFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("knowledge stage 1: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, nil // empty = no match, caller degrades silently
	}

	// Stage 2: LLM re-rank (optional; failure drops back to stage 1).
	if r.config.RerankEnabled && reranker != nil && query.SummaryHint != "" {
		shortlist := make([]RerankEntry, 0, len(resp.Items))
		for i, e := range resp.Items {
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
				if idx >= 0 && idx < len(resp.Items) {
					e := resp.Items[idx]
					e.Score = float64(1) - float64(idx)*0.1 // rank order → score
					picked = append(picked, e)
				}
			}
			if len(picked) > 0 {
				return picked, nil
			}
		}
		// re-rank failed or returned empty → fall back to structured shortlist
	}

	// Return structured shortlist (truncate to MaxResults).
	n := len(resp.Items)
	if n > r.config.MaxResults {
		n = r.config.MaxResults
	}
	return resp.Items[:n], nil
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
