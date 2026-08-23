// Command aiopsbench runs offline, deterministic quality benchmarks for the
// AIOps diagnosis engine and the knowledge (case memory) retriever.
//
// Subcommands:
//
//	diagnosis  replay the labeled scenario corpus (testdata/diagnosis-corpus.json)
//	           through the compiled-in rules and report per-rule precision /
//	           recall / F1 plus the deterministic pipeline top-1 accuracy.
//	retrieval  measure structured-phase retrieval quality (Hit@k, MRR) of the
//	           knowledge case memory across corpus scales.
//
// Both modes are hermetic: no cluster, database or AI provider is contacted.
// Labels in the corpus are anchored to reviewed unit tests and rule
// specifications; because this tool replays them through the same exported
// functions used in production, any behavior change that breaks a label is a
// signal to review the rule or relabel deliberately.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "diagnosis":
		if err := runDiagnosis(os.Args[2:]); err != nil {
			fail(err)
		}
	case "retrieval":
		if err := runRetrieval(os.Args[2:]); err != nil {
			fail(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "aiopsbench: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "aiopsbench: %v\n", err)
	os.Exit(1)
}

func usage() {
	fmt.Fprint(os.Stderr, `aiopsbench — offline quality benchmarks for the AIOps platform

Usage:
  aiopsbench diagnosis [-json <out.json>]
      Replay the labeled diagnosis corpus and print per-rule P/R/F1,
      micro/macro aggregates and the pipeline top-1 accuracy.

  aiopsbench retrieval [-shortlist <n>] [-max <n>] [-json <out.json>]
      Measure structured-phase retrieval quality (Hit@k, MRR) across
      synthetic corpus scales using the production retriever semantics.
`)
}
