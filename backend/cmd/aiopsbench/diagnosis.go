package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"k8s-aiops.local/backend/internal/diagnosisbench"
	"os"
	"time"
)

type Scenario = diagnosisbench.Scenario
type diagnosisReport = diagnosisbench.Report

func loadCorpus() (*diagnosisbench.Corpus, error) { return diagnosisbench.LoadCorpus() }
func evaluateTarget(s *Scenario, observedAt time.Time) (bool, error) {
	return diagnosisbench.EvaluateTarget(s, observedAt)
}
func evaluatePipeline(s *Scenario, observedAt time.Time) (string, error) {
	return diagnosisbench.EvaluatePipeline(s, observedAt)
}
func runDiagnosis(args []string) error {
	fs := flag.NewFlagSet("diagnosis", flag.ExitOnError)
	jsonOut := fs.String("json", "", "write the machine-readable report to this path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := diagnosisbench.Run()
	if err != nil {
		return err
	}
	printDiagnosisTable(&report)
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
