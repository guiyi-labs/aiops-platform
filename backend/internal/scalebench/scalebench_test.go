package scalebench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/scalefixture"
)

func loadTestData(t *testing.T) (*Data, string) {
	t.Helper()
	config := scalefixture.Config{
		SchemaVersion: scalefixture.SchemaVersion, DatasetVersion: "bench-v1", Seed: 17, ClusterID: 1,
		ObservedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), NodeCount: 4,
		NamespaceCount: 2, PodCount: 40, EventCount: 80, PodsPerWorkload: 5, HistoryPoints: 2,
	}
	directory := filepath.Join(t.TempDir(), "fixture")
	manifest, err := scalefixture.Generate(context.Background(), config, directory)
	if err != nil {
		t.Fatal(err)
	}
	data, err := Load(context.Background(), directory, config, manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data, directory
}

func TestLoadAndFixtureOperations(t *testing.T) {
	data, _ := loadTestData(t)
	if len(data.Nodes) != 4 || len(data.Pods) != 40 || len(data.Events) != 80 || len(data.Namespaces) != 2 {
		t.Fatalf("loaded counts nodes=%d pods=%d events=%d namespaces=%d", len(data.Nodes), len(data.Pods), len(data.Events), len(data.Namespaces))
	}
	edges, err := data.DeriveTopology(context.Background())
	if err != nil || edges != 128 {
		t.Fatalf("DeriveTopology() = %d, %v", edges, err)
	}
	page, err := data.PagePods(context.Background(), data.Namespaces[0], apiquery.ListQuery{Limit: 7})
	if err != nil || len(page.Items) != 7 || page.Total != 20 || page.Remaining != 13 {
		t.Fatalf("PagePods() = %#v, %v", page, err)
	}
	search, err := data.Search(context.Background(), "api")
	if err != nil || len(search.Items) > 100 {
		t.Fatalf("Search() = %#v, %v", search, err)
	}
	stream, err := data.StreamPods(context.Background(), data.Namespaces[0], 4)
	if err != nil || stream.Records != 20 || stream.MaxQueue > 4 {
		t.Fatalf("StreamPods() = %#v, %v", stream, err)
	}
}

func TestOperationsHonorCancellation(t *testing.T) {
	data, _ := loadTestData(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := data.DeriveTopology(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("DeriveTopology() error = %v", err)
	}
	if _, err := data.PageEvents(ctx, data.Namespaces[0], apiquery.ListQuery{Limit: 10}); !errors.Is(err, context.Canceled) {
		t.Fatalf("PageEvents() error = %v", err)
	}
}

func TestRunProducesStructuredReport(t *testing.T) {
	data, _ := loadTestData(t)
	report, err := Run(context.Background(), data, RunConfig{Samples: 3, Commit: "test-commit", LoadDuration: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchemaVersion || report.Mode != "report" || len(report.Operations) != 8 || report.Environment.Commit != "test-commit" {
		t.Fatalf("report = %#v", report)
	}
	for _, invariant := range report.Invariants {
		if !invariant.Passed {
			t.Fatalf("invariant = %#v", invariant)
		}
	}
	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	dataBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(dataBytes, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Operations[0].Stats.Samples != 3 {
		t.Fatalf("decoded report = %#v", decoded)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "report.md")); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	if _, err := Run(context.Background(), nil, RunConfig{}); err == nil {
		t.Fatal("Run() error = nil")
	}
	if err := WriteReport("", Report{}); err == nil {
		t.Fatal("WriteReport() error = nil")
	}
}
