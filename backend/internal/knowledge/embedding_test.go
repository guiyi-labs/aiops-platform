package knowledge

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestLexicalEmbeddingProviderIsDeterministic(t *testing.T) {
	p := NewLexicalEmbeddingProvider(0) // non-positive → default width
	if p.Dimension() != DefaultEmbeddingDimension {
		t.Fatalf("dimension = %d, want default %d", p.Dimension(), DefaultEmbeddingDimension)
	}
	if p.Name() == "" {
		t.Fatal("provider must report a stable name")
	}

	ctx := context.Background()
	texts := []string{"Pod OOMKilled 反复重启", "Service DNS 解析失败"}
	first, err := p.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	second, err := p.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(first) != len(texts) {
		t.Fatalf("got %d vectors, want %d", len(first), len(texts))
	}
	for i := range first {
		if len(first[i]) != p.Dimension() {
			t.Fatalf("vector %d width = %d, want %d", i, len(first[i]), p.Dimension())
		}
		for j := range first[i] {
			if first[i][j] != second[i][j] {
				t.Fatalf("embedding not deterministic at vector %d index %d", i, j)
			}
		}
	}
}

func TestLexicalEmbeddingIsUnitNorm(t *testing.T) {
	p := NewLexicalEmbeddingProvider(64)
	vectors, err := p.Embed(context.Background(), []string{"Pod 内存超限 OOMKilled"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	var sum float64
	for _, v := range vectors[0] {
		sum += float64(v) * float64(v)
	}
	if norm := math.Sqrt(sum); math.Abs(norm-1) > 1e-5 {
		t.Fatalf("vector norm = %f, want 1", norm)
	}
}

// The vector stage exists to rank shared wording above unrelated wording; if
// this ordering does not hold the whole hybrid pipeline is decoration.
func TestLexicalEmbeddingRanksSharedWordingHigher(t *testing.T) {
	p := NewLexicalEmbeddingProvider(512)
	hint := "Pod 容器因内存超限被 OOMKilled 并反复重启"

	vectors, err := p.Embed(context.Background(), []string{
		hint,
		"Pod 容器因内存超限被 OOMKilled 并反复重启，内存 limit 设置过低",
		"Service 的 DNS 解析失败导致调用方连接超时",
	})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	related := CosineSimilarity(vectors[0], vectors[1])
	unrelated := CosineSimilarity(vectors[0], vectors[2])
	if related <= unrelated {
		t.Fatalf("related similarity %f must exceed unrelated %f", related, unrelated)
	}
}

func TestLexicalEmbeddingEmptyTextIsZeroVector(t *testing.T) {
	p := NewLexicalEmbeddingProvider(32)
	vectors, err := p.Embed(context.Background(), []string{"", "   ", "!!!"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	for i, v := range vectors {
		for _, x := range v {
			if x != 0 {
				t.Fatalf("vector %d of token-free text must be zero", i)
			}
		}
	}
	if got := CosineSimilarity(vectors[0], vectors[0]); got != 0 {
		t.Fatalf("zero-vector self similarity = %f, want 0", got)
	}
}

func TestTokenize(t *testing.T) {
	got := tokenize("Pod OOMKilled 内存超限")
	want := []string{"pod", "oomkilled", "内存", "存超", "超限"}
	if len(got) != len(want) {
		t.Fatalf("tokenize = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokenize[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestTokenizeIsolatesLoneHanCharacter(t *testing.T) {
	got := tokenize("cpu 高")
	want := []string{"cpu", "高"}
	if len(got) != len(want) {
		t.Fatalf("tokenize = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokenize[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(tokenize("")) != 0 {
		t.Fatal("empty text must produce no tokens")
	}
}

func TestCosineSimilarityEdgeCases(t *testing.T) {
	if got := CosineSimilarity(nil, []float32{1}); got != 0 {
		t.Fatalf("nil operand = %f, want 0", got)
	}
	if got := CosineSimilarity([]float32{0, 0}, []float32{1, 1}); got != 0 {
		t.Fatalf("zero-vector operand = %f, want 0", got)
	}
	// Differing widths compare over the shared prefix instead of panicking.
	if got := CosineSimilarity([]float32{1, 0}, []float32{1, 0, 5}); got != 1 {
		t.Fatalf("shared-prefix similarity = %f, want 1", got)
	}
	if got := CosineSimilarity([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Fatalf("orthogonal similarity = %f, want 0", got)
	}
}

func TestEntryEmbeddingText(t *testing.T) {
	e := Entry{
		RuleID: "pod.oom_killed.v1", ResourceKind: "Pod", ResourceName: "api-0",
		Summary: "container was OOMKilled", RootCauses: []string{"memory limit too low"},
		Recommendations: []string{"raise the memory limit"},
	}
	text := e.EmbeddingText()
	for _, want := range []string{"pod.oom_killed.v1", "Pod", "api-0", "OOMKilled", "memory limit too low"} {
		if !strings.Contains(text, want) {
			t.Fatalf("EmbeddingText() = %q, missing %q", text, want)
		}
	}
	// Recommendations describe the remediation, not the symptom being matched.
	if strings.Contains(text, "raise the memory limit") {
		t.Fatalf("EmbeddingText() must exclude recommendations, got %q", text)
	}
	// Blank fields are skipped rather than leaving doubled separators.
	if strings.Contains(Entry{RuleID: "r"}.EmbeddingText(), "  ") {
		t.Fatal("blank fields must not leave doubled separators")
	}
}
