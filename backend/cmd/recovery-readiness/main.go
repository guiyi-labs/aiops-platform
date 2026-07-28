package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s-aiops.local/backend/internal/recoveryreadiness"
)

func main() {
	os.Exit(run())
}

func run() int {
	policyPath := flag.String("policy-file", "", "approved recovery readiness policy JSON file")
	evidencePath := flag.String("logical-restore-evidence", "", "sanitized logical restore evidence JSON file")
	flag.Parse()
	if flag.NArg() != 0 || *policyPath == "" || *evidencePath == "" {
		fmt.Fprintln(os.Stderr, "policy-file and logical-restore-evidence are required")
		return 2
	}
	policy, err := recoveryreadiness.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "recovery readiness policy is invalid")
		return 1
	}
	evidence, err := recoveryreadiness.LoadEvidence(*evidencePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logical restore evidence is invalid")
		return 1
	}
	report := recoveryreadiness.Evaluate(policy, evidence, time.Now().UTC())
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "recovery readiness report encoding failed")
		return 1
	}
	if !report.ReadyForPITRHAImplementation {
		return 1
	}
	return 0
}
