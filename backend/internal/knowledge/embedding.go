package knowledge

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// DefaultEmbeddingDimension is the vector width of the offline embedder.
const DefaultEmbeddingDimension = 256

// EmbeddingProvider turns text into a fixed-dimension vector so the retriever
// can run a vector stage alongside the structured B-tree selection.
//
// The interface is deliberately narrow and transport-agnostic. This package
// ships one implementation (LexicalEmbeddingProvider) that is deterministic
// and needs no network or model download; a production deployment may attach
// a neural embedder instead. A nil provider disables the vector stage
// entirely — the retriever then behaves exactly as it did before Phase 2.
type EmbeddingProvider interface {
	// Embed returns one vector per input text, in the same order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Dimension is the fixed width of every returned vector.
	Dimension() int
	// Name identifies the provider in observability output and bench reports.
	Name() string
}

// LexicalEmbeddingProvider is a deterministic, dependency-free embedder that
// hashes character n-grams into a fixed-width vector (the "hashing trick"
// with signed features).
//
// It is a LEXICAL embedder, not a neural one: two texts score high cosine
// similarity when they share n-grams — wording, resource names, rule ids —
// not when they merely mean the same thing. That is a deliberate trade of
// generality for reproducibility: the vector stage must produce identical
// results in unit tests and in the offline benchmark, with no API key, no
// model file, and no network. The vector stage is always additive — it
// augments the structured stage rather than replacing it — so the retrieval
// quality floor is set by the deterministic path, not by this one.
type LexicalEmbeddingProvider struct {
	dim int
}

// NewLexicalEmbeddingProvider builds an embedder of the given width. A
// non-positive width falls back to DefaultEmbeddingDimension.
func NewLexicalEmbeddingProvider(dim int) *LexicalEmbeddingProvider {
	if dim <= 0 {
		dim = DefaultEmbeddingDimension
	}
	return &LexicalEmbeddingProvider{dim: dim}
}

// Dimension returns the fixed vector width.
func (p *LexicalEmbeddingProvider) Dimension() int { return p.dim }

// Name returns a stable identifier used in reports.
func (p *LexicalEmbeddingProvider) Name() string { return "lexical-hash-v1" }

// Embed hashes each text into a vector. The implementation is pure and
// allocation-bounded: it never fails, so the error return exists only to
// satisfy the interface and to allow a network-backed provider to report
// transport failures.
func (p *LexicalEmbeddingProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		out[i] = p.embedOne(text)
	}
	return out, nil
}

// embedOne builds one vector: token frequency, signed hashing, L2 norm.
func (p *LexicalEmbeddingProvider) embedOne(text string) []float32 {
	vec := make([]float32, p.dim)
	counts := make(map[string]int, 32)
	for _, token := range tokenize(text) {
		counts[token]++
	}
	for token, tf := range counts {
		hasher := fnv.New64a()
		_, _ = hasher.Write([]byte(token))
		sum := hasher.Sum64()
		index := int(sum % uint64(p.dim))
		// Sub-linear term weighting keeps a repeated boilerplate token from
		// dominating the vector.
		weight := float32(1 + math.Log(float64(tf)))
		// The high bit supplies the sign, which cancels part of the bias
		// introduced by collisions in a 256-wide space.
		if sum&(1<<63) != 0 {
			weight = -weight
		}
		vec[index] += weight
	}
	return l2Normalize(vec)
}

// tokenize splits text into lowercase ASCII word tokens and CJK bigrams.
//
// Bigrams rather than single Han characters are what let two Chinese
// summaries with different wording but shared phrasing score as neighbours;
// emitting every individual character as well would make all Chinese text
// look alike. A Han run of length one (very short labels, units) still
// contributes its single character so it is not dropped entirely.
func tokenize(text string) []string {
	runes := []rune(strings.ToLower(text))
	tokens := make([]string, 0, len(runes))
	var word strings.Builder
	flushWord := func() {
		if word.Len() > 0 {
			tokens = append(tokens, word.String())
			word.Reset()
		}
	}
	hanRun := make([]rune, 0, 8)
	flushHan := func() {
		switch len(hanRun) {
		case 0:
		case 1:
			tokens = append(tokens, string(hanRun))
		default:
			for i := 0; i+1 < len(hanRun); i++ {
				tokens = append(tokens, string(hanRun[i:i+2]))
			}
		}
		hanRun = hanRun[:0]
	}
	for _, r := range runes {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			flushHan()
			word.WriteRune(r)
		case unicode.Is(unicode.Han, r):
			flushWord()
			hanRun = append(hanRun, r)
		default:
			flushWord()
			flushHan()
		}
	}
	flushWord()
	flushHan()
	return tokens
}

// l2Normalize scales vec to unit length so the dot product equals the cosine.
// A zero vector is returned unchanged (an empty text has no direction).
func l2Normalize(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return vec
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

// CosineSimilarity returns the cosine of the angle between two vectors.
//
// Vectors from one provider always share a width; the differing-width branch
// exists only so a mismatched pair degrades to a defined answer instead of a
// panic. It is computed over the shared prefix — both the dot product and the
// norms — because normalising the norms over the full length while taking the
// dot product over the prefix would not be a cosine of anything. A
// zero-length or zero-magnitude operand scores 0, meaning "no similarity".
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EmbeddingText renders an entry into the text the vector stage indexes.
// Rule id, resource identity and the distilled summary carry the retrievable
// symptom signal; recommendations are deliberately excluded because they
// describe the remediation rather than the failure being matched.
func (e Entry) EmbeddingText() string {
	parts := make([]string, 0, 3+len(e.RootCauses))
	for _, p := range []string{e.RuleID, e.ResourceKind, e.ResourceName, e.Summary} {
		if strings.TrimSpace(p) != "" {
			parts = append(parts, p)
		}
	}
	for _, rc := range e.RootCauses {
		if strings.TrimSpace(rc) != "" {
			parts = append(parts, rc)
		}
	}
	return strings.Join(parts, " ")
}
