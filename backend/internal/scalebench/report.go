package scalebench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s-aiops.local/backend/internal/apiquery"
	"k8s-aiops.local/backend/internal/globalsearch"
	"k8s-aiops.local/backend/internal/metricshistory"
	"k8s-aiops.local/backend/internal/scalefixture"
)

const ReportSchemaVersion = "aiops.scale-benchmark/v1"

type RunConfig struct {
	Warmup       int
	Samples      int
	Commit       string
	GeneratedAt  time.Time
	LoadDuration time.Duration
}

type Environment struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	GoVersion  string `json:"go_version"`
	CPUs       int    `json:"cpus"`
	GOMAXPROCS int    `json:"gomaxprocs"`
	Commit     string `json:"commit"`
}

type Fixture struct {
	SchemaVersion  string               `json:"schema_version"`
	DatasetVersion string               `json:"dataset_version"`
	ConfigSHA256   string               `json:"config_sha256"`
	DatasetSHA256  string               `json:"dataset_sha256"`
	Summary        scalefixture.Summary `json:"summary"`
}

type DurationStats struct {
	Samples int     `json:"samples"`
	MinMS   float64 `json:"min_ms"`
	MeanMS  float64 `json:"mean_ms"`
	P50MS   float64 `json:"p50_ms"`
	P95MS   float64 `json:"p95_ms"`
	P99MS   float64 `json:"p99_ms"`
	MaxMS   float64 `json:"max_ms"`
}

type Operation struct {
	Name                string        `json:"name"`
	Stats               DurationStats `json:"stats"`
	ObservedItems       int64         `json:"observed_items"`
	BoundedLimit        int           `json:"bounded_limit,omitempty"`
	IterationsPerSample int           `json:"iterations_per_sample"`
}

type RuntimeObservation struct {
	HeapBytesBefore  uint64 `json:"heap_bytes_before"`
	HeapBytesAfter   uint64 `json:"heap_bytes_after"`
	PeakHeapBytes    uint64 `json:"peak_heap_bytes"`
	GoroutinesBefore int    `json:"goroutines_before"`
	GoroutinesAfter  int    `json:"goroutines_after"`
	PeakGoroutines   int    `json:"peak_goroutines"`
	Samples          int    `json:"samples"`
}

type Invariant struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Observed string `json:"observed"`
}

type Report struct {
	SchemaVersion string             `json:"schema_version"`
	Mode          string             `json:"mode"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Environment   Environment        `json:"environment"`
	Fixture       Fixture            `json:"fixture"`
	Warmup        int                `json:"warmup"`
	Samples       int                `json:"samples"`
	Load          DurationStats      `json:"load"`
	Operations    []Operation        `json:"operations"`
	Runtime       RuntimeObservation `json:"runtime"`
	Invariants    []Invariant        `json:"invariants"`
}

type benchmarkOperation struct {
	name       string
	limit      int
	iterations int
	run        func(context.Context) (int64, error)
}

func Run(ctx context.Context, data *Data, config RunConfig) (Report, error) {
	if data == nil || config.Warmup < 0 || config.Samples < 3 || config.Samples > 200 {
		return Report{}, errors.New("scale benchmark configuration is invalid")
	}
	if config.GeneratedAt.IsZero() {
		config.GeneratedAt = time.Now().UTC()
	}
	if config.Commit == "" {
		config.Commit = "unknown"
	}
	targetNode := scalefixture.Node(data.Config, 0)
	targetPod := scalefixture.Pod(data.Config, 0)
	operations := []benchmarkOperation{
		{name: "topology_derive_all_namespaces", run: func(ctx context.Context) (int64, error) {
			count, err := data.DeriveTopology(ctx)
			return int64(count), err
		}},
		{name: "global_search_api", limit: 100, run: func(ctx context.Context) (int64, error) {
			response, err := data.Search(ctx, "api")
			return int64(response.Total), err
		}},
		{name: "pods_paginate_all", limit: 100, run: func(ctx context.Context) (int64, error) {
			return paginatePods(ctx, data)
		}},
		{name: "events_paginate_all", limit: 100, run: func(ctx context.Context) (int64, error) {
			return paginateEvents(ctx, data)
		}},
		{name: "history_query_node_cpu", limit: data.Config.HistoryPoints, iterations: 1000, run: func(ctx context.Context) (int64, error) {
			items, err := data.QueryHistory(ctx, "Node", "", targetNode.Metadata.Name, "", "cpu", data.Config.HistoryPoints)
			return int64(len(items)), err
		}},
		{name: "history_query_pod_cpu", limit: data.Config.HistoryPoints, iterations: 1000, run: func(ctx context.Context) (int64, error) {
			items, err := data.QueryHistory(ctx, "Pod", targetPod.Metadata.Namespace, targetPod.Metadata.Name, "app", "cpu", data.Config.HistoryPoints)
			return int64(len(items)), err
		}},
		{name: "history_evaluate_pod_cpu", limit: data.Config.HistoryPoints, iterations: 1000, run: func(ctx context.Context) (int64, error) {
			items, err := data.QueryHistory(ctx, "Pod", targetPod.Metadata.Namespace, targetPod.Metadata.Name, "app", "cpu", data.Config.HistoryPoints)
			if err != nil {
				return 0, err
			}
			if len(items) == 0 {
				return 0, errors.New("history query returned no points")
			}
			points := make([]metricshistory.Point, 0, len(items))
			for _, item := range items {
				points = append(points, metricshistory.Point{Value: item.Value, SourceTimestamp: item.SourceTimestamp, WindowMilliseconds: item.WindowMilliseconds, CollectedAt: item.SourceTimestamp.Add(time.Second)})
			}
			series := metricshistory.SeriesResponse{
				Series: metricshistory.Series{ClusterID: data.Config.ClusterID, ResourceKind: metricshistory.ResourcePod, ResourceNamespace: targetPod.Metadata.Namespace, ResourceName: targetPod.Metadata.Name, ContainerName: "app", MetricName: metricshistory.MetricCPU, Unit: metricshistory.UnitNanocores},
				From:   items[0].SourceTimestamp, To: items[len(items)-1].SourceTimestamp.Add(time.Minute), Points: points,
				Coverage: metricshistory.QueryCoverage{Collections: len(points), Succeeded: len(points), Points: len(points)}, Limits: metricshistory.QueryLimits{MaxWindowSeconds: 24 * 60 * 60, MaxPoints: 1440},
			}
			_, err = metricshistory.EvaluateWindow(series, metricshistory.EvaluationRule{Operator: metricshistory.OperatorGreaterThanOrEqual, Threshold: 250_000_000, ForSeconds: 60, MinimumPoints: 2})
			return int64(len(points)), err
		}},
		{name: "pod_stream_backpressure", limit: 8, iterations: 10, run: func(ctx context.Context) (int64, error) {
			result, err := data.StreamPods(ctx, data.Namespaces[0], 8)
			return result.Records, err
		}},
	}
	beforeMemory := readHeapBytes()
	beforeGoroutines := runtime.NumGoroutine()
	sampler := newRuntimeSampler()
	sampler.start()
	operationReports := make([]Operation, 0, len(operations))
	for _, operation := range operations {
		report, err := runOperation(ctx, config, operation)
		if err != nil {
			sampler.stop()
			return Report{}, fmt.Errorf("%s: %w", operation.name, err)
		}
		operationReports = append(operationReports, report)
	}
	sampler.stop()
	runtime.GC()
	afterMemory := readHeapBytes()
	invariants, finalGoroutines, err := probeInvariants(data, beforeGoroutines)
	if err != nil {
		return Report{}, err
	}
	return Report{
		SchemaVersion: ReportSchemaVersion, Mode: "fail-closed", GeneratedAt: config.GeneratedAt.UTC(),
		Environment: Environment{
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(),
			CPUs: runtime.NumCPU(), GOMAXPROCS: runtime.GOMAXPROCS(0), Commit: config.Commit,
		},
		Fixture: Fixture{
			SchemaVersion: data.Manifest.SchemaVersion, DatasetVersion: data.Manifest.DatasetVersion,
			ConfigSHA256: data.Manifest.ConfigSHA256, DatasetSHA256: data.Manifest.DatasetSHA256,
			Summary: data.Manifest.Summary,
		},
		Warmup: config.Warmup, Samples: config.Samples, Load: durationStats([]time.Duration{config.LoadDuration}),
		Operations: operationReports,
		Runtime: RuntimeObservation{
			HeapBytesBefore: beforeMemory, HeapBytesAfter: afterMemory, PeakHeapBytes: sampler.peakHeap,
			GoroutinesBefore: beforeGoroutines, GoroutinesAfter: finalGoroutines,
			PeakGoroutines: sampler.peakGoroutines, Samples: sampler.samples,
		},
		Invariants: invariants,
	}, nil
}

func runOperation(ctx context.Context, config RunConfig, operation benchmarkOperation) (Operation, error) {
	if operation.iterations < 1 {
		operation.iterations = 1
	}
	for index := 0; index < config.Warmup; index++ {
		if _, err := executeBatch(ctx, operation); err != nil {
			return Operation{}, err
		}
	}
	durations := make([]time.Duration, 0, config.Samples)
	var observed int64
	for index := 0; index < config.Samples; index++ {
		started := time.Now()
		items, err := executeBatch(ctx, operation)
		if err != nil {
			return Operation{}, err
		}
		durations = append(durations, time.Since(started)/time.Duration(operation.iterations))
		if index == 0 {
			observed = items
		} else if items != observed {
			return Operation{}, fmt.Errorf("observed item count changed from %d to %d", observed, items)
		}
	}
	return Operation{Name: operation.name, Stats: durationStats(durations), ObservedItems: observed, BoundedLimit: operation.limit, IterationsPerSample: operation.iterations}, nil
}

func executeBatch(ctx context.Context, operation benchmarkOperation) (int64, error) {
	var observed int64
	for index := 0; index < operation.iterations; index++ {
		items, err := operation.run(ctx)
		if err != nil {
			return 0, err
		}
		if index == 0 {
			observed = items
		} else if items != observed {
			return 0, fmt.Errorf("batch item count changed from %d to %d", observed, items)
		}
	}
	return observed, nil
}

func paginatePods(ctx context.Context, data *Data) (int64, error) {
	var total int64
	for _, namespace := range data.Namespaces {
		for page := 1; ; page++ {
			query := apiquery.ListQuery{Page: page, Limit: 100, Offset: (page - 1) * 100, SortBy: "name", Ascending: true}
			response, err := data.PagePods(ctx, namespace, query)
			if err != nil {
				return 0, err
			}
			if len(response.Items) > 100 {
				return 0, fmt.Errorf("pod page exceeded limit")
			}
			total += int64(len(response.Items))
			if response.Remaining == 0 {
				break
			}
		}
	}
	return total, nil
}

func paginateEvents(ctx context.Context, data *Data) (int64, error) {
	var total int64
	for _, namespace := range data.Namespaces {
		for page := 1; ; page++ {
			query := apiquery.ListQuery{Page: page, Limit: 100, Offset: (page - 1) * 100, SortBy: "name", Ascending: true}
			response, err := data.PageEvents(ctx, namespace, query)
			if err != nil {
				return 0, err
			}
			if len(response.Items) > 100 {
				return 0, fmt.Errorf("event page exceeded limit")
			}
			total += int64(len(response.Items))
			if response.Remaining == 0 {
				break
			}
		}
	}
	return total, nil
}

func probeInvariants(data *Data, goroutinesBefore int) ([]Invariant, int, error) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, cancelErr := data.PagePods(canceled, data.Namespaces[0], apiquery.ListQuery{Limit: 100})
	timedOut, timeoutCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer timeoutCancel()
	_, timeoutErr := data.DeriveTopology(timedOut)
	searchTimeoutResponse, searchTimeoutErr := data.Search(timedOut, "api")
	stream, err := data.StreamPods(context.Background(), data.Namespaces[0], 8)
	if err != nil {
		return nil, 0, err
	}
	canceledStream, canceledStreamCancel := context.WithCancel(context.Background())
	canceledStreamCancel()
	_, streamCancelErr := data.StreamPods(canceledStream, data.Namespaces[0], 8)
	finalGoroutines := runtime.NumGoroutine()
	searchTimeoutReported := searchTimeoutErr == nil && len(searchTimeoutResponse.Failures) > 0
	for _, failure := range searchTimeoutResponse.Failures {
		if failure.Code != globalsearch.FailureTimeout {
			searchTimeoutReported = false
		}
	}
	return []Invariant{
		{Name: "fixture_counts_match_manifest", Passed: int64(len(data.Nodes)) == data.Manifest.Summary.Counts.Nodes && int64(len(data.Pods)) == data.Manifest.Summary.Counts.Pods && int64(len(data.Events)) == data.Manifest.Summary.Counts.Events, Observed: fmt.Sprintf("nodes=%d pods=%d events=%d", len(data.Nodes), len(data.Pods), len(data.Events))},
		{Name: "cancellation_observed", Passed: errors.Is(cancelErr, context.Canceled), Observed: errorName(cancelErr)},
		{Name: "timeout_observed", Passed: errors.Is(timeoutErr, context.DeadlineExceeded), Observed: errorName(timeoutErr)},
		{Name: "search_timeout_reported", Passed: searchTimeoutReported, Observed: fmt.Sprintf("error=%s failures=%d", errorName(searchTimeoutErr), len(searchTimeoutResponse.Failures))},
		{Name: "backpressure_queue_bounded", Passed: stream.MaxQueue <= stream.QueueCapacity, Observed: fmt.Sprintf("max_queue=%d capacity=%d records=%d", stream.MaxQueue, stream.QueueCapacity, stream.Records)},
		{Name: "backpressure_cancellation_observed", Passed: errors.Is(streamCancelErr, context.Canceled), Observed: errorName(streamCancelErr)},
		{Name: "goroutines_return_to_baseline", Passed: finalGoroutines == goroutinesBefore, Observed: fmt.Sprintf("before=%d after=%d", goroutinesBefore, finalGoroutines)},
	}, finalGoroutines, nil
}

func durationStats(values []time.Duration) DurationStats {
	if len(values) == 0 {
		return DurationStats{}
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total time.Duration
	for _, value := range ordered {
		total += value
	}
	return DurationStats{
		Samples: len(ordered), MinMS: milliseconds(ordered[0]),
		MeanMS: milliseconds(total / time.Duration(len(ordered))),
		P50MS:  milliseconds(percentile(ordered, 50)), P95MS: milliseconds(percentile(ordered, 95)),
		P99MS: milliseconds(percentile(ordered, 99)), MaxMS: milliseconds(ordered[len(ordered)-1]),
	}
}

func percentile(values []time.Duration, percent int) time.Duration {
	index := int(math.Ceil(float64(percent)/100*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func milliseconds(value time.Duration) float64 {
	return math.Round(float64(value)/float64(time.Millisecond)*1000000) / 1000000
}

func errorName(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

func readHeapBytes() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapInuse + stats.StackInuse
}

type runtimeSampler struct {
	mu             sync.Mutex
	stopChannel    chan struct{}
	done           chan struct{}
	peakHeap       uint64
	peakGoroutines int
	samples        int
}

func newRuntimeSampler() *runtimeSampler {
	return &runtimeSampler{stopChannel: make(chan struct{}), done: make(chan struct{})}
}

func (s *runtimeSampler) start() {
	s.sample()
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopChannel:
				return
			case <-ticker.C:
				s.sample()
			}
		}
	}()
}

func (s *runtimeSampler) sample() {
	heap := readHeapBytes()
	goroutines := runtime.NumGoroutine()
	s.mu.Lock()
	if heap > s.peakHeap {
		s.peakHeap = heap
	}
	if goroutines > s.peakGoroutines {
		s.peakGoroutines = goroutines
	}
	s.samples++
	s.mu.Unlock()
}

func (s *runtimeSampler) stop() {
	close(s.stopChannel)
	<-s.done
	s.sample()
}

func WriteReport(path string, report Report) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("report path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	return os.WriteFile(markdownPath, []byte(markdownReport(report)), 0o644)
}

func markdownReport(report Report) string {
	lines := []string{
		"# M96 backend scale benchmark report", "",
		fmt.Sprintf("- Generated: %s", report.GeneratedAt.Format(time.RFC3339)),
		fmt.Sprintf("- Commit: `%s`", report.Environment.Commit),
		fmt.Sprintf("- Runtime: %s/%s, %s, CPUs=%d, GOMAXPROCS=%d", report.Environment.GOOS, report.Environment.GOARCH, report.Environment.GoVersion, report.Environment.CPUs, report.Environment.GOMAXPROCS),
		fmt.Sprintf("- Fixture: `%s` / `%s`", report.Fixture.DatasetVersion, report.Fixture.DatasetSHA256),
		fmt.Sprintf("- Mode: %s; warmup=%d; samples=%d", report.Mode, report.Warmup, report.Samples),
		"", "| Operation | Items | Limit | Iterations/sample | P50 ms | P95 ms | P99 ms | Max ms |", "|---|---:|---:|---:|---:|---:|---:|---:|",
	}
	for _, operation := range report.Operations {
		lines = append(lines, fmt.Sprintf("| %s | %d | %d | %d | %.6f | %.6f | %.6f | %.6f |", operation.Name, operation.ObservedItems, operation.BoundedLimit, operation.IterationsPerSample, operation.Stats.P50MS, operation.Stats.P95MS, operation.Stats.P99MS, operation.Stats.MaxMS))
	}
	lines = append(lines, "", "## Runtime", "", fmt.Sprintf("- Heap: before=%d, peak=%d, after=%d bytes", report.Runtime.HeapBytesBefore, report.Runtime.PeakHeapBytes, report.Runtime.HeapBytesAfter), fmt.Sprintf("- Goroutines: before=%d, peak=%d, after=%d", report.Runtime.GoroutinesBefore, report.Runtime.PeakGoroutines, report.Runtime.GoroutinesAfter), "", "## Invariants", "")
	for _, invariant := range report.Invariants {
		lines = append(lines, fmt.Sprintf("- %s: passed=%t (%s)", invariant.Name, invariant.Passed, invariant.Observed))
	}
	lines = append(lines, "", "This report is a fail-closed production gate. Threshold violations block CI (M109).", "")
	return strings.Join(lines, "\n")
}
