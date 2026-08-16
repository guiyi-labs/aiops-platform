// Package main implements the aiops CLI — a thin, dependency-light front end
// over the platform's deterministic diagnosis rules and case library.
//
// It reuses the compiled-in rules from internal/diagnosis (Evaluate* pure
// functions), the kubeconfig parser from internal/cluster and the case data
// model from internal/knowledge. It never requires the platform server or a
// database: without a kubeconfig it runs against built-in demo data, and the
// cases command degrades to a built-in rule catalog when no server is given.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is the semantic version reported by --version. It is kept in sync
// with the release tag.
const version = "0.1.0"

const usageText = `aiops — Kubernetes fault diagnosis with case memory

Usage:
  aiops diagnose [--kubeconfig PATH] [--namespace NS] [--pod NAME] [-o text|json]
      Run deterministic rule-based diagnosis against a cluster (or built-in
      demo data when no kubeconfig is available). Exit code: 0 clean, 1
      findings, 2 error.
  aiops cases [--query TERM] [--severity LEVEL] [--limit N] [--server URL] [-o text|json]
      Search the historical case library. Without --server the command
      degrades to a built-in rule catalog. Exit code: 0 results, 1 no
      results, 2 error.
  aiops --version
      Print the aiops version.

Global behavior:
  Output is English. -o json emits machine-readable JSON for scripting.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run dispatches subcommands and returns the process exit code. It is
// separated from main so tests can drive it without spawning a process.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	switch args[0] {
	case "diagnose":
		return runDiagnose(args[1:], stdout, stderr)
	case "cases":
		return runCases(args[1:], stdout, stderr)
	case "--version", "-version", "version":
		fmt.Fprintf(stdout, "aiops %s\n", version)
		return 0
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "aiops: unknown command %q\n\n%s", args[0], usageText)
		return 2
	}
}
