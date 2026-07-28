package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"k8s-aiops.local/backend/internal/identityreadiness"
)

func main() {
	os.Exit(run())
}

func run() int {
	policyPath := flag.String("policy-file", "", "approved identity readiness policy JSON file")
	discoveryPath := flag.String("discovery-file", "", "offline OIDC discovery JSON snapshot")
	jwksPath := flag.String("jwks-file", "", "offline OIDC JWKS JSON snapshot")
	flag.Parse()
	if flag.NArg() != 0 || *policyPath == "" || *discoveryPath == "" || *jwksPath == "" {
		fmt.Fprintln(os.Stderr, "policy-file, discovery-file and jwks-file are required")
		return 2
	}
	policy, err := identityreadiness.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "identity readiness policy is invalid")
		return 1
	}
	discovery, err := identityreadiness.LoadDiscovery(*discoveryPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "OIDC discovery snapshot is invalid")
		return 1
	}
	keys, err := identityreadiness.LoadJWKS(*jwksPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "OIDC JWKS snapshot is invalid")
		return 1
	}
	report := identityreadiness.Evaluate(policy, discovery, keys)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, "identity readiness report encoding failed")
		return 1
	}
	if !report.Ready {
		return 1
	}
	return 0
}
