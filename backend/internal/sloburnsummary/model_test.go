package sloburnsummary

import (
	"testing"
	"time"
)

func TestSummarize_StatusClassification(t *testing.T) {
	defs := []DefRef{
		{ID: 1, ClusterID: 7, Service: ServiceRef{Kind: "Deployment", Namespace: "team", Name: "api"}, Template: "request_success_ratio", Objective: 0.99},
		{ID: 2, ClusterID: 7, Service: ServiceRef{Kind: "Deployment", Namespace: "team", Name: "worker"}, Template: "request_success_ratio", Objective: 0.99},
		{ID: 3, ClusterID: 7, Service: ServiceRef{Kind: "StatefulSet", Namespace: "team", Name: "db"}, Template: "request_success_ratio", Objective: 0.99},
		{ID: 4, ClusterID: 7, Service: ServiceRef{Kind: "Deployment", Namespace: "team", Name: "cron"}, Template: "request_success_ratio", Objective: 0.99},
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	latest := map[int64]EvalRef{
		1: {State: "burning_fast", BurnRate: 18.2, Ratio: 0.83, Coverage: "complete", RemainingBudget: 0.31, EvaluatedAt: now},
		2: {State: "healthy", BurnRate: 0.2, Ratio: 0.995, Coverage: "complete", RemainingBudget: 0.99, EvaluatedAt: now},
		3: {State: "unavailable", Coverage: "unavailable", EvaluatedAt: now},
		// ID 4 has no latest evaluation → no_data
	}

	resp := Summarize(defs, latest, 50)

	if resp.Total != 4 {
		t.Fatalf("expected 4 items, got %d", resp.Total)
	}
	if resp.Items[0].Status != StatusBurning || resp.Items[0].SLOID != 1 {
		t.Fatalf("expected burning first, got %+v", resp.Items[0])
	}
	if resp.Items[1].Status != StatusUnavailable || resp.Items[1].SLOID != 3 {
		t.Fatalf("expected unavailable second, got %+v", resp.Items[1])
	}
	if resp.Items[2].Status != StatusNoData || resp.Items[2].SLOID != 4 {
		t.Fatalf("expected no_data third, got %+v", resp.Items[2])
	}
	if resp.Items[3].Status != StatusHealthy || resp.Items[3].SLOID != 2 {
		t.Fatalf("expected healthy last, got %+v", resp.Items[3])
	}
}

func TestSummarize_Truncates(t *testing.T) {
	defs := make([]DefRef, 0, 5)
	latest := map[int64]EvalRef{}
	for i := 0; i < 5; i++ {
		id := int64(i + 1)
		defs = append(defs, DefRef{ID: id, Service: ServiceRef{Kind: "Deployment", Name: string(rune('a' + i))}})
		latest[id] = EvalRef{State: "healthy", Coverage: "complete", EvaluatedAt: time.Now()}
	}
	resp := Summarize(defs, latest, 3)
	if resp.Total != 3 || !resp.Truncated {
		t.Fatalf("expected 3 truncated items, got %d truncated=%v", resp.Total, resp.Truncated)
	}
}

func TestSummarize_Empty(t *testing.T) {
	resp := Summarize(nil, nil, 50)
	if resp.Total != 0 || resp.Truncated {
		t.Fatalf("expected empty response, got %+v", resp)
	}
}
