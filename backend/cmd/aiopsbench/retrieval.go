package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s-aiops.local/backend/internal/knowledge"
)

// benchmarkRules mirrors the compiled-in diagnosis rules that distill into
// the case library on resolution.
var benchmarkRules = []string{
	"pod.image_pull_backoff.v1",
	"pod.crash_loop_backoff.v1",
	"pod.oom_killed.v1",
	"pod.pending.v1",
	"service.no_ready_endpoints.v1",
	"node.not_ready.v1",
	"node.pressure.v1",
	"deployment.replicas_unavailable.v1",
	"persistentvolumeclaim.pending.v1",
	"horizontalpodautoscaler.saturated.v1",
	"ingress.backend_unavailable.v1",
	"node.metric_sustained_breach.v1",
}

// benchmarkKinds cycles so each rule holds entries across resource kinds;
// Pod-kind entries appear every 4th insertion index (0, 4, 8, ...).
var benchmarkKinds = []string{"Pod", "Deployment", "Service", "Node"}

type retrievalScaleResult struct {
	EntriesPerRule int     `json:"entries_per_rule"`
	CorpusSize     int     `json:"corpus_size"`
	PodKindPerRule int     `json:"pod_kind_entries_per_rule"`
	Queries        int     `json:"queries"`
	HitAt1         float64 `json:"hit_at_1"`
	HitAt3         float64 `json:"hit_at_3"`
	MRR            float64 `json:"mrr"`
}

type retrievalReport struct {
	Tool          string                 `json:"tool"`
	GeneratedAt   time.Time              `json:"generated_at"`
	ShortlistSize int                    `json:"shortlist_size"`
	MaxResults    int                    `json:"max_results"`
	Method        string                 `json:"method"`
	Scales        []retrievalScaleResult `json:"scales"`
}

func runRetrieval(args []string) error {
	fs := flag.NewFlagSet("retrieval", flag.ExitOnError)
	shortlist := fs.Int("shortlist", 10, "stage-1 shortlist size")
	maxResults := fs.Int("max", 3, "final result cap")
	jsonOut := fs.String("json", "", "write the machine-readable report to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	scales := []int{2, 5, 8, 10, 15, 20, 30}
	report := retrievalReport{
		Tool: "aiopsbench retrieval", GeneratedAt: time.Now().UTC(),
		ShortlistSize: *shortlist, MaxResults: *maxResults,
		Method: "structured two-field selection (rule_id + resource_kind), recency-ranked; " +
			"ground truth is the OLDEST Pod-kind entry of the queried rule, i.e. the hardest " +
			"case for a recency-ordered index; one query per compiled-in rule per scale",
	}

	for _, n := range scales {
		report.Scales = append(report.Scales, runRetrievalScale(n, *shortlist, *maxResults))
	}

	printRetrievalTable(&report)

	if *jsonOut != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o600); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Printf("report written to %s\n", *jsonOut)
	}
	return nil
}

// runRetrievalScale builds a deterministic synthetic case library with n
// entries per rule and measures Hit@1 / Hit@3 / MRR for one query per rule.
func runRetrievalScale(n, shortlistSize, maxResults int) retrievalScaleResult {
	repo := knowledge.NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	gt := map[string]int64{} // rule -> oldest Pod-kind entry ID

	id := int64(0)
	for _, rule := range benchmarkRules {
		oldestID := int64(0)
		var oldestAt time.Time
		for i := 0; i < n; i++ {
			id++
			kind := benchmarkKinds[i%len(benchmarkKinds)]
			severity := "high"
			if i%2 == 0 {
				severity = "critical"
			}
			entry := knowledge.Entry{
				SourceDiagnosisID: id,
				RuleID:            rule,
				Severity:          severity,
				ResourceKind:      kind,
				ResourceNamespace: "prod",
				ResourceName:      fmt.Sprintf("%s-%02d", rulePrefix(rule), i),
				Summary:           fmt.Sprintf("resolved %s incident #%d on %s", rule, i, kind),
				RootCauses:        []string{fmt.Sprintf("verified root cause %d", i)},
				Recommendations:   []string{fmt.Sprintf("verified remediation %d", i)},
				NotedAt:           base.Add(-time.Duration(i) * 24 * time.Hour),
			}
			stored, err := repo.Insert(ctx, entry)
			if err != nil {
				continue // hermetic bench: in-memory insert cannot fail
			}
			if kind == "Pod" && (oldestID == 0 || stored.NotedAt.Before(oldestAt)) {
				oldestID = stored.ID
				oldestAt = stored.NotedAt
			}
		}
		gt[rule] = oldestID
	}

	cfg := knowledge.RetrieverConfig{ShortlistSize: shortlistSize, RerankEnabled: false, MaxResults: maxResults}
	retriever := knowledge.NewRetriever(repo, cfg)

	var hit1, hit3, mrr float64
	queries := 0
	for _, rule := range benchmarkRules {
		want := gt[rule]
		if want == 0 {
			continue // no Pod-kind entry at this scale
		}
		queries++
		entries, err := retriever.Retrieve(ctx, knowledge.Query{
			RuleID:       rule,
			ResourceKind: "Pod",
		}, nil)
		if err != nil {
			continue
		}
		rank := 0
		for i, e := range entries {
			if e.ID == want {
				rank = i + 1
				break
			}
		}
		switch {
		case rank == 1:
			hit1++
			hit3++
			mrr++
		case rank > 0:
			if rank <= 3 {
				hit3++
			}
			mrr += 1 / float64(rank)
		}
	}
	if queries == 0 {
		return retrievalScaleResult{EntriesPerRule: n}
	}
	podPerRule := (n + len(benchmarkKinds) - 1) / len(benchmarkKinds)
	return retrievalScaleResult{
		EntriesPerRule: n,
		CorpusSize:     int(id),
		PodKindPerRule: podPerRule,
		Queries:        queries,
		HitAt1:         hit1 / float64(queries),
		HitAt3:         hit3 / float64(queries),
		MRR:            mrr / float64(queries),
	}
}

// rulePrefix returns the leading segment of a rule id for readable names.
func rulePrefix(rule string) string {
	for i := 0; i < len(rule); i++ {
		if rule[i] == '.' {
			return rule[:i]
		}
	}
	return rule
}

func printRetrievalTable(r *retrievalReport) {
	fmt.Printf("aiopsbench retrieval — structured phase (shortlist=%d, max=%d)\n\n", r.ShortlistSize, r.MaxResults)
	fmt.Printf("%10s %12s %14s %8s %8s %8s\n", "PER-RULE", "CORPUS-SIZE", "POD-KIND/RULE", "HIT@1", "HIT@3", "MRR")
	for _, s := range r.Scales {
		fmt.Printf("%10d %12d %14d %8.3f %8.3f %8.3f\n",
			s.EntriesPerRule, s.CorpusSize, s.PodKindPerRule, s.HitAt1, s.HitAt3, s.MRR)
	}
	fmt.Println("\nMethod: " + r.Method)
}
