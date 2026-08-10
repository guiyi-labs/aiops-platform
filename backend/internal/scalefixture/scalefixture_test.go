package scalefixture

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		SchemaVersion: SchemaVersion, DatasetVersion: "test-v1", Seed: 7, ClusterID: 1,
		ObservedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		NodeCount:  4, NamespaceCount: 2, PodCount: 40, EventCount: 80,
		PodsPerWorkload: 5, HistoryPoints: 2,
	}
}

func TestDefaultSummaryMatchesM96ScaleContract(t *testing.T) {
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	summary := config.Summary()
	if summary.Counts.Nodes != 500 || summary.Counts.Pods != 50000 || summary.Counts.Events != 100000 {
		t.Fatalf("counts = %#v", summary.Counts)
	}
	if summary.Counts.Workloads != 5000 || summary.Counts.HistorySamples != 606000 {
		t.Fatalf("derived counts = %#v", summary.Counts)
	}
	if summary.Coverage.Topology.NodesReferenced != 500 || summary.Coverage.Search.Total != 65000 || summary.Coverage.History.Samples != 606000 {
		t.Fatalf("coverage = %#v", summary.Coverage)
	}
	configFromFile, err := LoadConfig(filepath.Join("..", "..", "testdata", "scale", "m96-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if configFromFile != config {
		t.Fatalf("config file = %#v, default = %#v", configFromFile, config)
	}
}

func TestConfigValidationRejectsBrokenMappings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Config)
	}{
		{name: "schema", change: func(c *Config) { c.SchemaVersion = "v0" }},
		{name: "dataset version", change: func(c *Config) { c.DatasetVersion = "INVALID" }},
		{name: "zero seed", change: func(c *Config) { c.Seed = 0 }},
		{name: "uneven namespace", change: func(c *Config) { c.PodCount = 41 }},
		{name: "event mapping", change: func(c *Config) { c.EventCount = 79 }},
		{name: "history bound", change: func(c *Config) { c.HistoryPoints = 1441 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := testConfig()
			test.change(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestRecordsProvideWorkloadTopologySearchAndHistoryMappings(t *testing.T) {
	config := testConfig()
	workload := Workload(config, 0)
	if workload.Deployment.Metadata.Namespace != "fleet-000" || workload.ReplicaSet.Metadata.OwnerReferences[0].UID != workload.Deployment.Metadata.UID {
		t.Fatalf("workload owner mapping = %#v", workload)
	}
	pod := Pod(config, 0)
	if pod.Metadata.OwnerReferences[0].UID != workload.ReplicaSet.Metadata.UID || pod.Spec.NodeName != "node-000" || pod.Metadata.Labels["fixture.aiops.dev/search"] != "api" {
		t.Fatalf("pod mapping = %#v", pod)
	}
	event := Event(config, 1)
	if event.InvolvedObject.UID != pod.Metadata.UID || event.InvolvedObject.Kind != "Pod" {
		t.Fatalf("event mapping = %#v", event)
	}
	history := History(config, 0)
	if history.ResourceKind != "Node" || history.MetricName != "cpu" || history.Input().Window != time.Minute {
		t.Fatalf("node history = %#v", history)
	}
	podHistory := History(config, config.NodeCount*2*config.HistoryPoints)
	if podHistory.ResourceKind != "Pod" || podHistory.ResourceNamespace == "" || podHistory.ContainerName != "app" {
		t.Fatalf("pod history = %#v", podHistory)
	}
	if Event(config, 1).Type != "Warning" && Pod(config, 0).Status.Phase == "Pending" {
		t.Fatal("degraded pod event mapping is inconsistent")
	}
}

func TestGenerateAndVerifyIsDeterministic(t *testing.T) {
	config := testConfig()
	first := filepath.Join(t.TempDir(), "fixture")
	second := filepath.Join(t.TempDir(), "fixture")
	firstManifest, err := Generate(context.Background(), config, first)
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, err := Generate(context.Background(), config, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstManifest.DatasetSHA256 != secondManifest.DatasetSHA256 {
		t.Fatalf("dataset hashes differ: %s != %s", firstManifest.DatasetSHA256, secondManifest.DatasetSHA256)
	}
	if firstManifest.ConfigSHA256 != secondManifest.ConfigSHA256 {
		t.Fatalf("config hashes differ: %s != %s", firstManifest.ConfigSHA256, secondManifest.ConfigSHA256)
	}
	if _, err := Verify(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(context.Background(), config, first); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("second Generate() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(first, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Artifacts) != 5 || manifest.Summary.Counts.HistorySamples != 176 {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestGenerateHonorsCancellationAndVerifyDetectsExtraFiles(t *testing.T) {
	config := testConfig()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "canceled")
	if _, err := Generate(canceled, config, output); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Generate() error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("canceled output exists: %v", err)
	}

	valid := filepath.Join(t.TempDir(), "valid")
	if _, err := Generate(context.Background(), config, valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(valid, "unexpected.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(context.Background(), valid); err == nil {
		t.Fatal("Verify() error = nil for extra file")
	}
}
