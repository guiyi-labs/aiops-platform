package scalefixture

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

func Verify(ctx context.Context, outputDir string) (Manifest, error) {
	manifestPath := filepath.Join(outputDir, "manifest.json")
	file, err := os.Open(manifestPath)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&manifest)
	closeErr := file.Close()
	if err != nil {
		return Manifest{}, fmt.Errorf("decode scale fixture manifest: %w", err)
	}
	if closeErr != nil {
		return Manifest{}, closeErr
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	if err := validateDirectory(outputDir, manifest); err != nil {
		return Manifest{}, err
	}
	for _, artifact := range manifest.Artifacts {
		if err := verifyArtifact(ctx, outputDir, artifact); err != nil {
			return Manifest{}, err
		}
	}
	if got := datasetHash(manifest.Artifacts); got != manifest.DatasetSHA256 {
		return Manifest{}, fmt.Errorf("dataset_sha256 = %s, want %s", got, manifest.DatasetSHA256)
	}
	return manifest, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != SchemaVersion || manifest.DatasetVersion == "" || manifest.Seed == 0 || manifest.ClusterID < 1 || manifest.ObservedAt == "" {
		return ErrInvalidManifest
	}
	config := Config{
		SchemaVersion: SchemaVersion, DatasetVersion: manifest.DatasetVersion,
		Seed: manifest.Seed, ClusterID: manifest.ClusterID,
		ObservedAt: mustParseTime(manifest.ObservedAt),
		NodeCount:  int(manifest.Summary.Counts.Nodes), NamespaceCount: int(manifest.Summary.Coverage.Workloads.Namespaces),
		PodCount: int(manifest.Summary.Counts.Pods), EventCount: int(manifest.Summary.Counts.Events),
		PodsPerWorkload: int(manifest.Summary.Coverage.Workloads.Pods / manifest.Summary.Counts.Workloads),
		HistoryPoints:   int(manifest.Summary.Coverage.History.PointsPerSeries),
	}
	if err := config.Validate(); err != nil {
		return fmt.Errorf("%w: config: %v", ErrInvalidManifest, err)
	}
	expected := config.Summary()
	if !sameSummary(manifest.Summary, expected) {
		return fmt.Errorf("%w: summary does not match config", ErrInvalidManifest)
	}
	hash, err := config.Hash()
	if err != nil || hash != manifest.ConfigSHA256 {
		return fmt.Errorf("%w: config_sha256 does not match config", ErrInvalidManifest)
	}
	if len(manifest.Artifacts) != len(artifactSpecs(config)) || manifest.DatasetSHA256 == "" {
		return ErrInvalidManifest
	}
	names := make(map[string]bool, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		if names[artifact.Name] || artifact.Records < 1 || artifact.SHA256 == "" || artifact.Compression != "gzip" {
			return ErrInvalidManifest
		}
		names[artifact.Name] = true
	}
	for _, spec := range artifactSpecs(config) {
		if !names[spec.name] {
			return ErrInvalidManifest
		}
	}
	return nil
}

func sameSummary(actual, expected Summary) bool {
	actualBytes, _ := json.Marshal(actual)
	expectedBytes, _ := json.Marshal(expected)
	return string(actualBytes) == string(expectedBytes)
}

func validateDirectory(outputDir string, manifest Manifest) error {
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return err
	}
	expected := map[string]bool{"manifest.json": true}
	for _, artifact := range manifest.Artifacts {
		expected[artifact.Name] = true
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return fmt.Errorf("unexpected scale fixture output %q", entry.Name())
		}
	}
	return nil
}

func verifyArtifact(ctx context.Context, outputDir string, artifact Artifact) error {
	path := filepath.Join(outputDir, filepath.Base(artifact.Name))
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if info.Size() != artifact.CompressedBytes {
		_ = file.Close()
		return fmt.Errorf("%s compressed_bytes = %d, want %d", artifact.Name, info.Size(), artifact.CompressedBytes)
	}
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("open %s: %w", artifact.Name, err)
	}
	hasher := sha256.New()
	reader := bufio.NewReaderSize(gzipReader, 1<<20)
	var records, bytes int64
	for {
		select {
		case <-ctx.Done():
			_ = gzipReader.Close()
			_ = file.Close()
			return ctx.Err()
		default:
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && readErr == io.EOF {
			break
		}
		if readErr == bufio.ErrBufferFull {
			_ = gzipReader.Close()
			_ = file.Close()
			return fmt.Errorf("%s contains an oversized record", artifact.Name)
		}
		if len(line) == 0 || line[len(line)-1] != '\n' {
			_ = gzipReader.Close()
			_ = file.Close()
			return fmt.Errorf("%s contains a record without a newline", artifact.Name)
		}
		if !json.Valid(line[:len(line)-1]) {
			_ = gzipReader.Close()
			_ = file.Close()
			return fmt.Errorf("%s contains invalid JSON", artifact.Name)
		}
		if _, err := hasher.Write(line); err != nil {
			_ = gzipReader.Close()
			_ = file.Close()
			return err
		}
		records++
		bytes += int64(len(line))
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			_ = gzipReader.Close()
			_ = file.Close()
			return fmt.Errorf("read %s: %w", artifact.Name, readErr)
		}
	}
	closeErr := gzipReader.Close()
	fileErr := file.Close()
	if closeErr != nil {
		return closeErr
	}
	if fileErr != nil {
		return fileErr
	}
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if records != artifact.Records || bytes != artifact.UncompressedBytes || actualHash != artifact.SHA256 {
		return fmt.Errorf("%s metadata mismatch: records=%d bytes=%d sha256=%s", artifact.Name, records, bytes, actualHash)
	}
	return nil
}

func sortedArtifactNames(manifest Manifest) []string {
	names := make([]string, 0, len(manifest.Artifacts))
	for _, artifact := range manifest.Artifacts {
		names = append(names, artifact.Name)
	}
	sort.Strings(names)
	return names
}
