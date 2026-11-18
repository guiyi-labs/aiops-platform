package knowledge

import "context"

// RetrievalTrace observes the same pipeline used by Retrieve. Ranks and RRF
// contributions describe ordering, never diagnostic confidence.
type RetrievalTrace struct {
	Query               Query   `json:"query"`
	Structured          []Entry `json:"structured"`
	StructuredTotal     int64   `json:"structured_total"`
	StructuredTruncated bool    `json:"structured_truncated"`
	Vector              []Entry `json:"vector"`
	Fused               []Entry `json:"fused"`
	Results             []Entry `json:"results"`
	VectorStatus        string  `json:"vector_status"`
	Embedder            string  `json:"embedder"`
	Dimension           int     `json:"dimension"`
	PoolCount           int     `json:"pool_count"`
	PoolTotal           int64   `json:"pool_total"`
	PoolTruncated       bool    `json:"pool_truncated"`
	ShortlistLimit      int     `json:"shortlist_limit"`
	VectorLimit         int     `json:"vector_limit"`
	ResultLimit         int     `json:"result_limit"`
	RankConstant        int     `json:"rank_constant"`
}

// Inspect compares stages without invoking an LLM or writing cases. The
// baseline is the exact structured stage of the same run.
func (r *Retriever) Inspect(ctx context.Context, query Query) (RetrievalTrace, error) {
	trace := RetrievalTrace{
		Query: query, VectorStatus: "disabled", ShortlistLimit: r.config.ShortlistSize,
		VectorLimit: r.config.VectorTopK, ResultLimit: r.config.MaxResults, RankConstant: r.config.RRFRankConstant,
	}
	if r.embedder != nil {
		trace.Embedder = r.embedder.Name()
		trace.Dimension = r.embedder.Dimension()
	}
	results, err := r.retrieve(ctx, query, nil, &trace)
	trace.Results = results
	if trace.Structured == nil {
		trace.Structured = []Entry{}
	}
	if trace.Vector == nil {
		trace.Vector = []Entry{}
	}
	if trace.Fused == nil {
		trace.Fused = []Entry{}
	}
	if trace.Results == nil {
		trace.Results = []Entry{}
	}
	return trace, err
}
