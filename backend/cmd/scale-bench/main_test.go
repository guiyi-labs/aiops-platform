package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/scalefixture"
)

func TestRunRequiresPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestResolveCommitPrecedence(t *testing.T) {
	t.Setenv("GITHUB_SHA", "environment-commit")
	if got := resolveCommit("explicit-commit"); got != "explicit-commit" {
		t.Fatalf("explicit commit = %q", got)
	}
	if got := resolveCommit(""); got != "environment-commit" {
		t.Fatalf("environment commit = %q", got)
	}
	if err := os.Unsetenv("GITHUB_SHA"); err != nil {
		t.Fatal(err)
	}
}

func TestRunWritesSmallReport(t *testing.T) {
	config := scalefixture.Config{
		SchemaVersion: scalefixture.SchemaVersion, DatasetVersion: "cli-bench-v1", Seed: 23, ClusterID: 1,
		ObservedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), NodeCount: 2,
		NamespaceCount: 1, PodCount: 4, EventCount: 8, PodsPerWorkload: 2, HistoryPoints: 2,
	}
	root := t.TempDir()
	fixtureDir := filepath.Join(root, "fixture")
	if _, err := scalefixture.Generate(context.Background(), config, fixtureDir); err != nil {
		t.Fatal(err)
	}
	configData, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, "report.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"-config", configPath, "-fixture", fixtureDir, "-output", reportPath, "-warmup", "0", "-samples", "3", "-commit", "test"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "invariants_failed=0") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatal(err)
	}
}
