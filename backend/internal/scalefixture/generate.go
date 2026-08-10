package scalefixture

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Artifact struct {
	Name              string `json:"name"`
	Records           int64  `json:"records"`
	SHA256            string `json:"sha256"`
	UncompressedBytes int64  `json:"uncompressed_bytes"`
	CompressedBytes   int64  `json:"compressed_bytes"`
	Compression       string `json:"compression"`
}

type Manifest struct {
	SchemaVersion  string     `json:"schema_version"`
	DatasetVersion string     `json:"dataset_version"`
	Seed           uint64     `json:"seed"`
	ClusterID      int64      `json:"cluster_id"`
	ObservedAt     string     `json:"observed_at"`
	ConfigSHA256   string     `json:"config_sha256"`
	Summary        Summary    `json:"summary"`
	Artifacts      []Artifact `json:"artifacts"`
	DatasetSHA256  string     `json:"dataset_sha256"`
}

type artifactSpec struct {
	name  string
	count int64
	value func(Config, int) any
}

func Generate(ctx context.Context, config Config, outputDir string) (Manifest, error) {
	if err := config.Validate(); err != nil {
		return Manifest{}, err
	}
	config.ObservedAt = config.ObservedAt.UTC()
	if strings.TrimSpace(outputDir) == "" {
		return Manifest{}, errors.New("output directory is required")
	}
	outputDir = filepath.Clean(outputDir)
	if _, err := os.Stat(outputDir); err == nil {
		return Manifest{}, ErrOutputExists
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o755); err != nil {
		return Manifest{}, err
	}
	if err := os.Mkdir(outputDir, 0o755); err != nil {
		if os.IsExist(err) {
			return Manifest{}, ErrOutputExists
		}
		return Manifest{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(outputDir)
		}
	}()

	configHash, err := config.Hash()
	if err != nil {
		return Manifest{}, fmt.Errorf("hash scale fixture config: %w", err)
	}
	artifacts := make([]Artifact, 0, len(artifactSpecs(config)))
	for _, spec := range artifactSpecs(config) {
		artifact, err := writeArtifact(ctx, outputDir, spec, config)
		if err != nil {
			return Manifest{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion, DatasetVersion: config.DatasetVersion,
		Seed: config.Seed, ClusterID: config.ClusterID,
		ObservedAt: config.ObservedAt.Format(timeRFC3339Nano), ConfigSHA256: configHash,
		Summary: config.Summary(), Artifacts: artifacts,
	}
	manifest.DatasetSHA256 = datasetHash(artifacts)
	if err := writeManifest(outputDir, manifest); err != nil {
		return Manifest{}, err
	}
	complete = true
	return manifest, nil
}

func artifactSpecs(config Config) []artifactSpec {
	return []artifactSpec{
		{name: "nodes.ndjson.gz", count: int64(config.NodeCount), value: func(c Config, index int) any { return Node(c, index) }},
		{name: "workloads.ndjson.gz", count: int64(config.WorkloadCount()), value: func(c Config, index int) any { return Workload(c, index) }},
		{name: "pods.ndjson.gz", count: int64(config.PodCount), value: func(c Config, index int) any { return Pod(c, index) }},
		{name: "events.ndjson.gz", count: int64(config.EventCount), value: func(c Config, index int) any { return Event(c, index) }},
		{name: "history.ndjson.gz", count: config.Summary().Counts.HistorySamples, value: func(c Config, index int) any { return History(c, index) }},
	}
}

func writeArtifact(ctx context.Context, outputDir string, spec artifactSpec, config Config) (Artifact, error) {
	path := filepath.Join(outputDir, spec.name)
	file, err := os.Create(path)
	if err != nil {
		return Artifact{}, err
	}
	removeOnError := true
	defer func() {
		if removeOnError {
			_ = os.Remove(path)
		}
	}()
	gzipWriter, err := gzip.NewWriterLevel(file, gzip.BestSpeed)
	if err != nil {
		_ = file.Close()
		return Artifact{}, err
	}
	gzipWriter.Header.ModTime = zeroTime
	gzipWriter.Header.Name = ""
	hasher := sha256.New()
	writer := &countingWriter{writer: io.MultiWriter(gzipWriter, hasher)}
	encoder := json.NewEncoder(writer)
	for index := int64(0); index < spec.count; index++ {
		if index%256 == 0 {
			select {
			case <-ctx.Done():
				_ = gzipWriter.Close()
				_ = file.Close()
				return Artifact{}, ctx.Err()
			default:
			}
		}
		if err := encoder.Encode(spec.value(config, int(index))); err != nil {
			_ = gzipWriter.Close()
			_ = file.Close()
			return Artifact{}, fmt.Errorf("write %s: %w", spec.name, err)
		}
	}
	if err := gzipWriter.Close(); err != nil {
		_ = file.Close()
		return Artifact{}, fmt.Errorf("close %s: %w", spec.name, err)
	}
	if err := file.Close(); err != nil {
		return Artifact{}, fmt.Errorf("close %s: %w", spec.name, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Artifact{}, err
	}
	removeOnError = false
	return Artifact{
		Name: spec.name, Records: writer.records, SHA256: hex.EncodeToString(hasher.Sum(nil)),
		UncompressedBytes: writer.bytes, CompressedBytes: info.Size(), Compression: "gzip",
	}, nil
}

func writeManifest(outputDir string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(outputDir, "manifest.json"), data, 0o644)
}

func datasetHash(artifacts []Artifact) string {
	ordered := append([]Artifact(nil), artifacts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	hasher := sha256.New()
	for _, artifact := range ordered {
		_, _ = io.WriteString(hasher, artifact.Name)
		_, _ = hasher.Write([]byte{0})
		_, _ = io.WriteString(hasher, artifact.SHA256)
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

type countingWriter struct {
	writer  io.Writer
	bytes   int64
	records int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	written, err := w.writer.Write(data)
	w.bytes += int64(written)
	w.records += int64(bytes.Count(data[:written], []byte{'\n'}))
	return written, err
}

const (
	timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
)

var zeroTime = time.Unix(0, 0).UTC()

func mustParseTime(value string) (result time.Time) {
	result, _ = time.Parse(time.RFC3339, value)
	return result
}
