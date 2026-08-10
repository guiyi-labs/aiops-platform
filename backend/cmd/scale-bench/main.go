package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/scalebench"
	"k8s-aiops.local/backend/internal/scalefixture"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("scale-bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "versioned scale fixture config JSON")
	fixtureDir := flags.String("fixture", "", "verified scale fixture output directory")
	outputPath := flags.String("output", "", "benchmark report JSON path")
	warmup := flags.Int("warmup", 3, "warmup runs per operation")
	samples := flags.Int("samples", 30, "measured runs per operation")
	commit := flags.String("commit", "", "source commit for report metadata")
	timeout := flags.Duration("timeout", 10*time.Minute, "overall benchmark timeout")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *configPath == "" || *fixtureDir == "" || *outputPath == "" {
		fmt.Fprintln(stderr, "-config, -fixture and -output are required")
		return 2
	}
	config, err := scalefixture.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "scale benchmark config is invalid: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	manifest, err := scalefixture.Verify(ctx, *fixtureDir)
	if err != nil {
		fmt.Fprintf(stderr, "scale fixture verification failed: %v\n", err)
		return 1
	}
	loadStarted := time.Now()
	data, err := scalebench.Load(ctx, *fixtureDir, config, manifest)
	if err != nil {
		fmt.Fprintf(stderr, "scale fixture loading failed: %v\n", err)
		return 1
	}
	report, err := scalebench.Run(ctx, data, scalebench.RunConfig{
		Warmup: *warmup, Samples: *samples, Commit: resolveCommit(*commit),
		GeneratedAt: time.Now().UTC(), LoadDuration: time.Since(loadStarted),
	})
	if err != nil {
		fmt.Fprintf(stderr, "scale benchmark failed: %v\n", err)
		return 1
	}
	if err := scalebench.WriteReport(*outputPath, report); err != nil {
		fmt.Fprintf(stderr, "scale benchmark report writing failed: %v\n", err)
		return 1
	}
	failed := 0
	for _, invariant := range report.Invariants {
		if !invariant.Passed {
			failed++
		}
	}
	fmt.Fprintf(stdout, "report=%s operations=%d invariants_failed=%d go=%s cpus=%s\n", *outputPath, len(report.Operations), failed, runtime.Version(), strconv.Itoa(runtime.NumCPU()))
	if failed > 0 {
		return 1
	}
	return 0
}

func resolveCommit(explicit string) string {
	if value := strings.TrimSpace(explicit); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("GITHUB_SHA")); value != "" {
		return value
	}
	output, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return strings.TrimSpace(string(output))
	}
	return "unknown"
}
