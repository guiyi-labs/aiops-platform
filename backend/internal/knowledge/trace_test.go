package knowledge

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestInspectUsesProductionPipelineAndRecoversRenamedRule(t *testing.T) {
	repo := NewInMemoryRepository()
	seedHybridCorpus(t, repo)
	retriever := NewRetriever(repo, DefaultConfig()).WithEmbeddingProvider(NewLexicalEmbeddingProvider(512))
	query := Query{RuleID: "pod.oom_killed.v1", SummaryHint: "Pod 容器因内存超限被 OOMKilled 并反复重启"}
	trace, err := retriever.Inspect(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	results, err := retriever.Retrieve(context.Background(), query, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Structured) != 0 || len(trace.Vector) == 0 || len(trace.Results) == 0 {
		t.Fatalf("recall stages not exposed: %+v", trace)
	}
	if !reflect.DeepEqual(trace.Results, results) {
		t.Fatal("inspection diverges from production retrieval")
	}
	if trace.Results[0].RuleID != "pod.memory_exceeded.v2" || trace.VectorStatus != "complete" {
		t.Fatalf("related historical case lost: %+v", trace)
	}
	for _, entry := range trace.Fused {
		var want float64
		for rank, candidate := range trace.Vector {
			if candidate.ID == entry.ID {
				want += 1 / float64(trace.RankConstant+rank+1)
			}
		}
		if entry.Score != want {
			t.Fatalf("incorrect RRF contribution: got %v want %v", entry.Score, want)
		}
	}
}

func TestInspectReportsVectorFailureAndRetainsStructuredBaseline(t *testing.T) {
	repo := NewInMemoryRepository()
	seedHybridCorpus(t, repo)
	provider := &fakeEmbeddingProvider{dim: 3, embedErr: errors.New("offline")}
	retriever := NewRetriever(repo, DefaultConfig()).WithEmbeddingProvider(provider)
	query := Query{RuleID: "pod.memory_exceeded.v2", SummaryHint: "OOMKilled"}
	trace, err := retriever.Inspect(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if trace.VectorStatus != "unavailable" || len(trace.Results) != 1 || len(trace.Vector) != 0 {
		t.Fatalf("broken degradation: %+v", trace)
	}
	if trace.Structured[0].ID != trace.Results[0].ID {
		t.Fatal("failure dropped structured baseline")
	}
	disabled, err := NewRetriever(repo, DefaultConfig()).Inspect(context.Background(), query)
	if err != nil || disabled.VectorStatus != "disabled" || disabled.Embedder != "" {
		t.Fatalf("wrong disabled status: %+v %v", disabled, err)
	}
}
