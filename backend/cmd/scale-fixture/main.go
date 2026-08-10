package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"k8s-aiops.local/backend/internal/scalefixture"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scale-fixture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "versioned scale fixture config JSON")
	outputDir := flags.String("output", "", "directory for generated fixture artifacts")
	verifyDir := flags.String("verify", "", "verify an existing fixture output directory")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *verifyDir != "" {
		if *configPath != "" || *outputDir != "" || flags.NArg() != 0 {
			fmt.Fprintln(stderr, "-verify cannot be combined with -config, -output or positional arguments")
			return 2
		}
		manifest, err := scalefixture.Verify(context.Background(), *verifyDir)
		if err != nil {
			fmt.Fprintf(stderr, "scale fixture verification failed: %v\n", err)
			return 1
		}
		return encodeManifest(stdout, stderr, manifest)
	}
	if *configPath == "" || *outputDir == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "-config and -output are required")
		return 2
	}
	config, err := scalefixture.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "scale fixture config is invalid: %v\n", err)
		return 1
	}
	manifest, err := scalefixture.Generate(context.Background(), config, *outputDir)
	if err != nil {
		fmt.Fprintf(stderr, "scale fixture generation failed: %v\n", err)
		return 1
	}
	return encodeManifest(stdout, stderr, manifest)
}

func encodeManifest(stdout, stderr io.Writer, manifest scalefixture.Manifest) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		fmt.Fprintf(stderr, "scale fixture manifest encoding failed: %v\n", err)
		return 1
	}
	return 0
}
