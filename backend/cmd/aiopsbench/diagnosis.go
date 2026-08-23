package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	diagnosis "k8s-aiops.local/backend/internal/diagnosis"
	k8sgateway "k8s-aiops.local/backend/internal/kubernetes"
	"k8s-aiops.local/backend/internal/metricshistory"
)

//go:embed testdata/diagnosis-corpus.json
var corpusRaw []byte

// Scenario is one labeled evaluation point for exactly one target rule.
type Scenario struct {
	ID             string                                `json:"id"`
	TargetRule     string                                `json:"target_rule"`
	Expected       bool                                  `json:"expected"`
	PipelineExpect string                                `json:"pipeline_expect,omitempty"`
	Kind           string                                `json:"kind"`
	Note           string                                `json:"note,omitempty"`
	Pod            k8sgateway.Pod                        `json:"pod"`
	Service        k8sgateway.ServiceResource            `json:"service"`
	Endpoints      k8sgateway.Endpoints                  `json:"endpoints"`
	Node           k8sgateway.Node                       `json:"node"`
	Deployment     k8sgateway.Deployment                 `json:"deployment"`
	Claim          k8sgateway.PersistentVolumeClaim      `json:"claim"`
	HPA            k8sgateway.HorizontalPodAutoscaler    `json:"hpa"`
	Ingress        k8sgateway.Ingress                    `json:"ingress"`
	ServicesMap    map[string]k8sgateway.ServiceResource `json:"services"`
	EndpointsMap   map[string]k8sgateway.Endpoints       `json:"endpoints_map"`
	Evaluation     metricshistory.EvaluationResponse     `json:"evaluation"`
	Events         []k8sgateway.Event                    `json:"events"`
}

type corpusFile struct {
	Version     string     `json:"version"`
	Description string     `json:"description"`
	ObservedAt  time.Time  `json:"observed_at"`
	Scenarios   []Scenario `json:"scenarios"`
}

const benchmarkClusterID int64 = 7

func loadCorpus() (*corpusFile, error) {
	var c corpusFile
	if err := json.Unmarshal(corpusRaw, &c); err != nil {
		return nil, fmt.Errorf("parse diagnosis corpus: %w", err)
	}
	if len(c.Scenarios) == 0 {
		return nil, fmt.Errorf("diagnosis corpus contains no scenarios")
	}
	return &c, nil
}

// evaluateTarget runs only the target rule of the scenario. The corpus holds
// resource-shaped inputs, so cross-rule evaluation is out of scope; the
// priority chain is measured separately via pipeline_expect.
func evaluateTarget(s *Scenario, observedAt time.Time) (matched bool, err error) {
	switch s.TargetRule {
	case "pod.image_pull_backoff.v1":
		_, matched = diagnosis.EvaluateImagePullBackOff(benchmarkClusterID, s.Pod, s.Events, observedAt)
	case "pod.crash_loop_backoff.v1":
		_, matched = diagnosis.EvaluateCrashLoopBackOff(benchmarkClusterID, s.Pod, s.Events, observedAt)
	case "pod.oom_killed.v1":
		_, matched = diagnosis.EvaluatePodOOMKilled(benchmarkClusterID, s.Pod, s.Events, observedAt)
	case "pod.pending.v1":
		_, matched = diagnosis.EvaluatePodPending(benchmarkClusterID, s.Pod, s.Events, observedAt)
	case "service.no_ready_endpoints.v1":
		_, matched = diagnosis.EvaluateServiceNoEndpoints(benchmarkClusterID, s.Service, s.Endpoints, observedAt)
	case "node.not_ready.v1":
		_, matched = diagnosis.EvaluateNodeNotReady(benchmarkClusterID, s.Node, observedAt)
	case "node.pressure.v1":
		_, matched = diagnosis.EvaluateNodePressure(benchmarkClusterID, s.Node, observedAt)
	case "deployment.replicas_unavailable.v1":
		_, matched = diagnosis.EvaluateDeploymentReplicasUnavailable(benchmarkClusterID, s.Deployment, observedAt)
	case "persistentvolumeclaim.pending.v1":
		_, matched = diagnosis.EvaluatePersistentVolumeClaimPending(benchmarkClusterID, s.Claim, s.Events, observedAt)
	case "horizontalpodautoscaler.saturated.v1":
		_, matched = diagnosis.EvaluateHorizontalPodAutoscalerSaturated(benchmarkClusterID, s.HPA, observedAt)
	case "ingress.backend_unavailable.v1":
		states := make(map[string]diagnosis.IngressBackendState, len(s.ServicesMap))
		for name, svc := range s.ServicesMap {
			states[name] = diagnosis.IngressBackendState{Service: svc, Endpoints: s.EndpointsMap[name]}
		}
		_, matched = diagnosis.EvaluateIngressBackendUnavailable(benchmarkClusterID, s.Ingress, diagnosis.IngressServiceRoutes(s.Ingress), states, observedAt)
	case "node.metric_sustained_breach.v1":
		_, matched = diagnosis.EvaluateSustainedMetricBreach(benchmarkClusterID, s.Evaluation, observedAt)
	default:
		return false, fmt.Errorf("scenario %s: unknown target rule %q", s.ID, s.TargetRule)
	}
	return matched, nil
}

// evaluatePipeline replays the deterministic evaluatePod priority chain
// (oom -> image pull -> crash loop -> pending) and returns the selected
// rule id, or "" when no rule matched.
func evaluatePipeline(s *Scenario, observedAt time.Time) (string, error) {
	if s.Kind != "Pod" {
		return "", fmt.Errorf("scenario %s: pipeline_expect is only valid for Pod scenarios", s.ID)
	}
	pod, events := s.Pod, s.Events
	if r, ok := diagnosis.EvaluatePodOOMKilled(benchmarkClusterID, pod, events, observedAt); ok {
		return r.RuleID, nil
	}
	if r, ok := diagnosis.EvaluateImagePullBackOff(benchmarkClusterID, pod, events, observedAt); ok {
		return r.RuleID, nil
	}
	if r, ok := diagnosis.EvaluateCrashLoopBackOff(benchmarkClusterID, pod, events, observedAt); ok {
		return r.RuleID, nil
	}
	if r, ok := diagnosis.EvaluatePodPending(benchmarkClusterID, pod, events, observedAt); ok {
		return r.RuleID, nil
	}
	return "", nil
}

// RuleConfusion is the binary classification outcome set of one rule.
type RuleConfusion struct {
	RuleID    string `json:"rule_id"`
	Scenarios int    `json:"scenarios"`
	TP        int    `json:"true_positives"`
	FN        int    `json:"false_negatives"`
	FP        int    `json:"false_positives"`
	TN        int    `json:"true_negatives"`

	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

func (r *RuleConfusion) finalize() {
	denomP := r.TP + r.FP
	denomR := r.TP + r.FN
	if denomP > 0 {
		r.Precision = float64(r.TP) / float64(denomP)
	}
	if denomR > 0 {
		r.Recall = float64(r.TP) / float64(denomR)
	}
	if r.Precision+r.Recall > 0 {
		r.F1 = 2 * r.Precision * r.Recall / (r.Precision + r.Recall)
	}
}

type pipelineOutcome struct {
	Total       int      `json:"total_pod_scenarios"`
	CorrectTop1 int      `json:"correct_top1"`
	Accuracy    float64  `json:"top1_accuracy"`
	Mismatches  []string `json:"mismatches,omitempty"`
}

type diagnosisReport struct {
	Tool           string          `json:"tool"`
	CorpusVersion  string          `json:"corpus_version"`
	ObservedAt     time.Time       `json:"observed_at"`
	GeneratedAt    time.Time       `json:"generated_at"`
	TotalScenarios int             `json:"total_scenarios"`
	PerRule        []RuleConfusion `json:"per_rule"`
	MicroF1        float64         `json:"micro_f1"`
	MacroF1        float64         `json:"macro_f1"`
	LabelAccuracy  float64         `json:"label_accuracy"`
	Pipeline       pipelineOutcome `json:"pipeline"`
}

func runDiagnosis(args []string) error {
	fs := flag.NewFlagSet("diagnosis", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the machine-readable report to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	corpus, err := loadCorpus()
	if err != nil {
		return err
	}
	observed := corpus.ObservedAt
	if observed.IsZero() {
		observed = time.Date(2026, 8, 23, 8, 30, 0, 0, time.UTC)
	}

	byRule := map[string]*RuleConfusion{}
	var micro struct{ tp, fp, fn, tn int }
	correct := 0
	pipeline := pipelineOutcome{}

	for i := range corpus.Scenarios {
		s := &corpus.Scenarios[i]
		matched, err := evaluateTarget(s, observed)
		if err != nil {
			return err
		}
		rc := byRule[s.TargetRule]
		if rc == nil {
			rc = &RuleConfusion{RuleID: s.TargetRule}
			byRule[s.TargetRule] = rc
		}
		rc.Scenarios++
		switch {
		case s.Expected && matched:
			rc.TP++
			micro.tp++
			correct++
		case s.Expected && !matched:
			rc.FN++
			micro.fn++
		case !s.Expected && matched:
			rc.FP++
			micro.fp++
		default:
			rc.TN++
			micro.tn++
			correct++
		}

		if s.Kind == "Pod" {
			got, err := evaluatePipeline(s, observed)
			if err != nil {
				return err
			}
			pipeline.Total++
			if got == s.PipelineExpect {
				pipeline.CorrectTop1++
			} else {
				pipeline.Mismatches = append(pipeline.Mismatches,
					fmt.Sprintf("%s: want %q got %q", s.ID, s.PipelineExpect, got))
			}
		}
	}

	rules := make([]string, 0, len(byRule))
	for id := range byRule {
		rules = append(rules, id)
	}
	sort.Strings(rules)

	report := diagnosisReport{
		Tool: "aiopsbench diagnosis", CorpusVersion: corpus.Version,
		ObservedAt: observed, GeneratedAt: time.Now().UTC(),
		TotalScenarios: len(corpus.Scenarios),
	}
	macroSum := 0.0
	for _, id := range rules {
		rc := byRule[id]
		rc.finalize()
		report.PerRule = append(report.PerRule, *rc)
		macroSum += rc.F1
	}
	if len(rules) > 0 {
		report.MacroF1 = macroSum / float64(len(rules))
	}
	microDenom := 2*micro.tp + micro.fp + micro.fn
	if microDenom > 0 {
		report.MicroF1 = 2 * float64(micro.tp) / float64(microDenom)
	}
	report.LabelAccuracy = float64(correct) / float64(len(corpus.Scenarios))
	if pipeline.Total > 0 {
		pipeline.Accuracy = float64(pipeline.CorrectTop1) / float64(pipeline.Total)
	}
	report.Pipeline = pipeline

	printDiagnosisTable(&report)

	if *jsonOut != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
		fmt.Printf("report written to %s\n", *jsonOut)
	}
	return nil
}

func printDiagnosisTable(r *diagnosisReport) {
	fmt.Printf("aiopsbench diagnosis — corpus v%s, %d scenarios\n\n", r.CorpusVersion, r.TotalScenarios)
	fmt.Printf("%-42s %4s %4s %4s %4s %8s %8s %8s\n", "RULE", "TP", "FN", "FP", "TN", "PREC", "RECALL", "F1")
	for _, rc := range r.PerRule {
		fmt.Printf("%-42s %4d %4d %4d %4d %8.3f %8.3f %8.3f\n",
			rc.RuleID, rc.TP, rc.FN, rc.FP, rc.TN, rc.Precision, rc.Recall, rc.F1)
	}
	fmt.Printf("\nmicro F1=%.3f  macro F1=%.3f  label accuracy=%.3f (%d/%d)\n",
		r.MicroF1, r.MacroF1, r.LabelAccuracy, int(r.LabelAccuracy*float64(r.TotalScenarios)), r.TotalScenarios)
	fmt.Printf("pipeline top-1 accuracy=%.3f (%d/%d)\n",
		r.Pipeline.Accuracy, r.Pipeline.CorrectTop1, r.Pipeline.Total)
	for _, m := range r.Pipeline.Mismatches {
		fmt.Printf("  pipeline mismatch: %s\n", m)
	}
}
