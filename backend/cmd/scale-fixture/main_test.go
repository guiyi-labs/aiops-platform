package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s-aiops.local/backend/internal/scalefixture"
)

func TestRunRequiresGenerationArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "required") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRejectsMixedVerifyArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-verify", "fixture", "-output", "other"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunGeneratesAndVerifiesFixture(t *testing.T) {
	config := scalefixture.Config{
		SchemaVersion: scalefixture.SchemaVersion, DatasetVersion: "cli-v1", Seed: 11, ClusterID: 1,
		ObservedAt: scalefixture.DefaultConfig().ObservedAt, NodeCount: 2, NamespaceCount: 1,
		PodCount: 4, EventCount: 8, PodsPerWorkload: 2, HistoryPoints: 1,
	}
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	configData, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, configData, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "fixture")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-config", configPath, "-output", output}, &stdout, &stderr); code != 0 {
		t.Fatalf("generate code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"-verify", output}, &stdout, &stderr); code != 0 {
		t.Fatalf("verify code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dataset_version": "cli-v1"`) {
		t.Fatalf("manifest output = %q", stdout.String())
	}
}
