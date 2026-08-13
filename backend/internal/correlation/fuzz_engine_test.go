package correlation

import (
	"context"
	"testing"
	"time"
)

// FuzzEngineCorrelate drives the deterministic correlation engine with
// arbitrary structured inputs and verifies it never panics and always returns
// internally consistent results: catalog rule ids, valid confidence classes,
// active status and a non-empty case identity.
func FuzzEngineCorrelate(f *testing.F) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	f.Add(1, 1, 0, 0, 0)
	f.Add(2, 1, 1, 1, 1)
	f.Add(3, 3, 3, 3, 3)

	f.Fuzz(func(t *testing.T, nSignals, nChanges, nEdges, nDiagnoses, seed int) {
		nSignals %= 4
		nChanges %= 4
		nEdges %= 4
		nDiagnoses %= 4
		rng := uint64(seed)
		next := func() uint64 {
			rng = rng*6364136223846793005 + 1442695040888963407
			return rng
		}
		pick := func(items []string) string {
			if len(items) == 0 {
				return ""
			}
			return items[int(next()%uint64(len(items)))]
		}

		signalIDs := []string{"diag.node.not_ready.v1", "diag.pod.oom_killed.v1", "diag.pod.crash_loop_backoff.v1", "diag.deployment.replicas_unavailable.v1", "garbage.signal.v9"}
		producers := []string{"diagnosis", "alert", "metric", "", "slo"}
		kinds := []string{"Node", "Pod", "Service", "Deployment", "PersistentVolumeClaim", ""}
		names := []string{"demo-node", "demo-pod", "web-0", "api", "pvc-1", ""}
		uids := []string{"u-1", "u-2", "u-3", "", "u-5"}
		sevs := []string{"critical", "warning", "info", ""}
		states := []string{"active", "resolved", "", "stale"}

		inputs := EngineInputs{Now: now}
		for i := 0; i < nSignals; i++ {
			inputs.Signals = append(inputs.Signals, SignalOccurrenceInput{
				ID:         int64(i + 1),
				SignalID:   pick(signalIDs),
				Producer:   pick(producers),
				ClusterID:  1,
				Namespace:  "demo",
				Resource:   ResourceCitation{Kind: pick(kinds), Namespace: "demo", Name: pick(names), UID: pick(uids)},
				Severity:   pick(sevs),
				State:      pick(states),
				ObservedAt: now.Add(-time.Duration(int64(next()%7200)) * time.Second),
			})
		}

		changeKinds := []string{"rollout", "promotion", "maintenance", "restore", "audit", ""}
		results := []string{"succeeded", "failed", "pending", "partial", ""}
		actions := []string{"rollout_restart", "drain", "backup", ""}
		for i := 0; i < nChanges; i++ {
			inputs.Changes = append(inputs.Changes, ChangeEventInput{
				ID:         int64(i + 100),
				ClusterID:  1,
				Namespace:  "demo",
				Kind:       pick(changeKinds),
				PlanID:     "plan-" + pick(names),
				Target:     ResourceCitation{Kind: pick(kinds), Namespace: "demo", Name: pick(names), UID: pick(uids)},
				Action:     pick(actions),
				Result:     pick(results),
				StartedAt:  now.Add(-time.Duration(int64(next()%7200)) * time.Second),
				Confidence: pick([]string{"high", "low", ""}),
				Source:     pick([]string{"platform", "k8s_event", ""}),
			})
		}

		edgeKinds := []string{"Owns", "Selects", "RoutesTo", "BackedBy", "RunsOn", ""}
		for i := 0; i < nEdges; i++ {
			inputs.Edges = append(inputs.Edges, TopologyEdgeInput{
				ID:        int64(i + 200),
				ClusterID: 1,
				Kind:      pick(edgeKinds),
				Source:    ResourceCitation{Kind: pick(kinds), Namespace: "demo", Name: pick(names), UID: pick(uids)},
				Target:    ResourceCitation{Kind: pick(kinds), Namespace: "demo", Name: pick(names), UID: pick(uids)},
				ValidFrom: now.Add(-time.Duration(int64(next()%7200)) * time.Second),
			})
		}

		statuses := []string{"open", "confirmed", "resolved", ""}
		for i := 0; i < nDiagnoses; i++ {
			inputs.Diagnoses = append(inputs.Diagnoses, DiagnosisRef{
				ID:         int64(i + 300),
				ClusterID:  1,
				RuleID:     pick(signalIDs),
				Resource:   ResourceCitation{Kind: pick(kinds), Namespace: "demo", Name: pick(names), UID: pick(uids)},
				Severity:   pick(sevs),
				Status:     pick(statuses),
				ObservedAt: now.Add(-time.Duration(int64(next()%7200)) * time.Second),
			})
		}

		engine := NewEngine()
		out, err := engine.Correlate(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Correlate errored: %v", err)
		}
		for _, r := range out {
			if r.Case.RuleID == "" {
				t.Fatal("result with empty rule id")
			}
			if _, ok := catalog[r.Case.RuleID]; !ok {
				t.Fatalf("result with unknown rule id %q", r.Case.RuleID)
			}
			if r.Case.CaseKey == "" {
				t.Fatal("result with empty case key")
			}
			switch r.Case.Confidence {
			case ConfidenceConfirmed, ConfidenceCandidate, ConfidenceContradicted, ConfidenceUnknown:
			default:
				t.Fatalf("result with invalid confidence %q", r.Case.Confidence)
			}
			if r.Case.Status != CaseStatusActive {
				t.Fatalf("result with non-active status %q", r.Case.Status)
			}
		}
	})
}
