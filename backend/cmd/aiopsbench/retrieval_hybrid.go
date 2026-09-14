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

// legacyRuleSuffix is appended to a rule id to model the entries a case
// library accumulated before the rule was renamed.
//
// This is not a contrived scenario for the benchmark's benefit: every rule id
// in this platform carries a version suffix (".v1"), so promoting a rule to
// ".v2" silently strands every case distilled under the previous id. Exact
// field matching can no longer reach them — the recall gap the vector stage
// exists to close. The benchmark measures that gap and what closing it buys.
const legacyRuleSuffix = ".legacy"

// hybridRule pairs a compiled-in rule with the phrasing an operator would use
// to describe its symptom, plus the root cause the library records. The symptom
// phrasing is what a query carries; the entry text carries both, which is the
// wording difference a lexical embedder has to bridge.
type hybridRule struct {
	ID        string
	Symptom   string
	RootCause string
}

var hybridRules = []hybridRule{
	{ID: "pod.image_pull_backoff.v1", Symptom: "镜像拉取失败 反复重试 容器起不来 ImagePullBackOff", RootCause: "registry unreachable or image tag missing"},
	{ID: "pod.crash_loop_backoff.v1", Symptom: "容器反复崩溃 重启循环 CrashLoopBackOff", RootCause: "application exits non-zero on start"},
	{ID: "pod.oom_killed.v1", Symptom: "容器内存超限 被 OOMKilled 反复重启", RootCause: "memory limit set below working set"},
	{ID: "pod.pending.v1", Symptom: "pod 一直 Pending 调度不上 没有可用节点", RootCause: "insufficient allocatable resources"},
	{ID: "service.no_ready_endpoints.v1", Symptom: "service 没有就绪的后端 请求失败 503", RootCause: "all backend pods failed readiness"},
	{ID: "node.not_ready.v1", Symptom: "节点 NotReady 无法调度工作负载", RootCause: "kubelet heartbeat stopped"},
	{ID: "node.pressure.v1", Symptom: "节点磁盘或内存压力 触发驱逐", RootCause: "node resource pressure threshold crossed"},
	{ID: "deployment.replicas_unavailable.v1", Symptom: "deployment 可用副本不足 少于期望副本", RootCause: "replicas failed to become ready"},
	{ID: "persistentvolumeclaim.pending.v1", Symptom: "存储卷 PVC 一直 Pending 绑定失败", RootCause: "no matching persistent volume"},
	{ID: "horizontalpodautoscaler.saturated.v1", Symptom: "HPA 副本数达到上限 无法继续扩容", RootCause: "max replicas reached under sustained load"},
	{ID: "ingress.backend_unavailable.v1", Symptom: "ingress 后端不可用 网关报错 502", RootCause: "backend service has no endpoints"},
	{ID: "node.metric_sustained_breach.v1", Symptom: "节点指标持续超过阈值 CPU 使用率居高不下", RootCause: "sustained metric breach above threshold"},
}

// hybridCell is one (scenario, method) measurement.
type hybridCell struct {
	Scenario string  `json:"scenario"`
	Method   string  `json:"method"`
	Queries  int     `json:"queries"`
	HitAt1   float64 `json:"hit_at_1"`
	HitAt3   float64 `json:"hit_at_3"`
	MRR      float64 `json:"mrr"`
}

type hybridScaleResult struct {
	EntriesPerRule int          `json:"entries_per_rule"`
	CorpusSize     int          `json:"corpus_size"`
	Cells          []hybridCell `json:"cells"`
}

type hybridReport struct {
	Tool        string              `json:"tool"`
	GeneratedAt time.Time           `json:"generated_at"`
	Method      string              `json:"method"`
	Scales      []hybridScaleResult `json:"scales"`
}

func runRetrievalHybrid(args []string) error {
	fs := flag.NewFlagSet("retrieval-hybrid", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the machine-readable report to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report := hybridReport{
		Tool:        "aiopsbench retrieval-hybrid",
		GeneratedAt: time.Now().UTC(),
		Method: "synthetic case library in which every rule's cases live under a " + legacyRuleSuffix +
			" id, modelling a rule renamed after its cases were distilled. Ground truth is each rule's " +
			"newest legacy entry, uniquely identified by resource name so the target is not ambiguous. " +
			"'aligned' queries reuse the legacy id, so exact field matching can reach the target; " +
			"'renamed' queries use the current id, so it cannot. One query per rule per cell; " +
			"re-rank disabled so the comparison isolates the vector stage.",
	}
	for _, perRule := range []int{2, 5, 10} {
		report.Scales = append(report.Scales, runHybridScale(perRule))
	}

	printHybridTable(&report)

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

// runHybridScale measures all four (scenario, method) cells at one corpus
// scale. Each cell rebuilds the corpus so the cells stay independent.
func runHybridScale(perRule int) hybridScaleResult {
	result := hybridScaleResult{EntriesPerRule: perRule}
	scenarios := []struct {
		label   string
		renamed bool
	}{
		{"aligned", false},
		{"renamed", true},
	}
	methods := []struct {
		label  string
		vector bool
	}{
		{"structured", false},
		{"hybrid", true},
	}
	for _, scenario := range scenarios {
		for _, method := range methods {
			cell, corpusSize := measureHybridCell(scenario.renamed, method.vector, perRule)
			cell.Scenario = scenario.label
			cell.Method = method.label
			result.CorpusSize = corpusSize
			result.Cells = append(result.Cells, cell)
		}
	}
	return result
}

// measureHybridCell builds the corpus and scores one (scenario, method) pair,
// returning the cell and the corpus size it was measured on.
func measureHybridCell(renamed, vectorEnabled bool, perRule int) (hybridCell, int) {
	repo := knowledge.NewInMemoryRepository()
	ctx := context.Background()
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	type probe struct {
		targetID int64
		query    knowledge.Query
	}
	probes := make(map[string]probe, len(hybridRules))

	for _, rule := range hybridRules {
		for i := 0; i < perRule; i++ {
			resource := fmt.Sprintf("api-%s-%02d", rulePrefix(rule.ID), i)
			stored, err := repo.Insert(ctx, knowledge.Entry{
				SourceDiagnosisID: int64(i + 1),
				RuleID:            rule.ID + legacyRuleSuffix,
				Severity:          "high",
				ResourceKind:      "Pod",
				ResourceNamespace: "prod",
				ResourceName:      resource,
				Summary:           rule.Symptom + " " + resource,
				RootCauses:        []string{rule.RootCause},
				Recommendations:   []string{"apply the documented remediation"},
				NotedAt:           base.Add(-time.Duration(i) * 24 * time.Hour),
			})
			if err != nil {
				continue // hermetic bench: an in-memory insert cannot fail
			}
			if i != 0 {
				continue // i=0 is the newest entry, i.e. the ground truth
			}
			queryRuleID := rule.ID + legacyRuleSuffix
			if renamed {
				queryRuleID = rule.ID
			}
			probes[rule.ID] = probe{
				targetID: stored.ID,
				query: knowledge.Query{
					RuleID:      queryRuleID,
					SummaryHint: rule.Symptom + " " + resource,
				},
			}
		}
	}

	cfg := knowledge.DefaultConfig()
	cfg.RerankEnabled = false
	cfg.VectorCandidateSize = 1000 // must exceed the corpus, or recency truncation hides the target
	retriever := knowledge.NewRetriever(repo, cfg)
	if vectorEnabled {
		retriever = retriever.WithEmbeddingProvider(knowledge.NewLexicalEmbeddingProvider(0))
	}

	var hit1, hit3, mrr float64
	queries := 0
	for _, rule := range hybridRules {
		p, ok := probes[rule.ID]
		if !ok {
			continue
		}
		queries++
		entries, err := retriever.Retrieve(ctx, p.query, nil)
		if err != nil {
			continue
		}
		rank := 0
		for i, e := range entries {
			if e.ID == p.targetID {
				rank = i + 1
				break
			}
		}
		switch {
		case rank == 1:
			hit1++
			hit3++
			mrr++
		case rank > 0 && rank <= 3:
			hit3++
			mrr += 1 / float64(rank)
		}
	}
	if queries == 0 {
		return hybridCell{}, len(hybridRules) * perRule
	}
	return hybridCell{
		Queries: queries,
		HitAt1:  hit1 / float64(queries),
		HitAt3:  hit3 / float64(queries),
		MRR:     mrr / float64(queries),
	}, len(hybridRules) * perRule
}

func printHybridTable(r *hybridReport) {
	fmt.Printf("aiopsbench retrieval-hybrid — recall across a rule rename (ablation)\n\n")
	fmt.Printf("%8s %7s %-10s %-11s %7s %7s %7s\n", "PER-RULE", "CORPUS", "SCENARIO", "METHOD", "HIT@1", "HIT@3", "MRR")
	for _, s := range r.Scales {
		for _, c := range s.Cells {
			fmt.Printf("%8d %7d %-10s %-11s %7.3f %7.3f %7.3f\n",
				s.EntriesPerRule, s.CorpusSize, c.Scenario, c.Method, c.HitAt1, c.HitAt3, c.MRR)
		}
	}
	fmt.Println("\nMethod: " + r.Method)
}
