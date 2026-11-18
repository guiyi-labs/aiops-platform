package diagnosisbench

import (
	"encoding/json"
	"k8s-aiops.local/backend/internal/diagnosis"
	"reflect"
	"testing"
)

func TestFixedReportCoversEngineAndPreservesInputs(t *testing.T) {
	report, err := Run()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalScenarios != 38 || len(report.Outcomes) != 38 {
		t.Fatalf("unexpected corpus size: %d/%d", report.TotalScenarios, len(report.Outcomes))
	}
	roster := map[string]bool{}
	tp, tn, fp, fn := 0, 0, 0, 0
	for _, rule := range report.PerRule {
		roster[rule.RuleID] = true
		if rule.Scenarios != rule.TP+rule.TN+rule.FP+rule.FN {
			t.Fatalf("broken confusion counts: %+v", rule)
		}
		tp += rule.TP
		tn += rule.TN
		fp += rule.FP
		fn += rule.FN
	}
	if len(roster) != len(diagnosis.RuleIDs()) {
		t.Fatalf("corpus covers %d of %d rules", len(roster), len(diagnosis.RuleIDs()))
	}
	for _, id := range diagnosis.RuleIDs() {
		if !roster[id] {
			t.Fatalf("missing rule %s", id)
		}
	}
	if tp != 20 || tn != 18 || fp != 0 || fn != 0 {
		t.Fatalf("unexpected aggregate %d %d %d %d", tp, tn, fp, fn)
	}
	for _, outcome := range report.Outcomes {
		var source Scenario
		if err := json.Unmarshal(outcome.Input, &source); err != nil {
			t.Fatal(err)
		}
		if source.ID != outcome.ID || source.Expected != outcome.Expected || source.TargetRule != outcome.RuleID {
			t.Fatalf("source labels lost: %+v", outcome)
		}
		if !outcome.Passed || (outcome.Kind == "Pod" && !outcome.PipelinePassed) {
			t.Fatalf("regression: %+v", outcome)
		}
		if outcome.Matched && (outcome.Record == nil || len(outcome.Record.Evidence) == 0) {
			t.Fatalf("positive output lost evidence: %s", outcome.ID)
		}
		if !outcome.Matched && outcome.Record != nil {
			t.Fatalf("unmatched scenario fabricated a record: %s", outcome.ID)
		}
	}
	if report.MicroF1 != 1 || report.MacroF1 != 1 || report.LabelAccuracy != 1 {
		t.Fatalf("metrics inconsistent: %+v", report)
	}
	second, err := Run()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Outcomes, second.Outcomes) || !reflect.DeepEqual(report.PerRule, second.PerRule) {
		t.Fatal("fixed corpus output is not deterministic")
	}
}

func TestChangedInputIsEvaluatedRatherThanReusingLabel(t *testing.T) {
	corpus, err := LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	scenario := corpus.Scenarios[0]
	scenario.Pod.Status.ContainerStatuses = nil
	scenario.Events = nil
	matched, err := EvaluateTarget(&scenario, corpus.ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	if matched || !scenario.Expected {
		t.Fatal("engine failed to distinguish changed input from original positive label")
	}
}
