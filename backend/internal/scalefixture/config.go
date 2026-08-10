package scalefixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"
)

const (
	SchemaVersion          = "aiops.scale-fixture/v1"
	DefaultDatasetVersion  = "m96-v1"
	DefaultClusterID       = int64(1)
	DefaultSeed            = uint64(20260810)
	DefaultNodeCount       = 500
	DefaultNamespaceCount  = 100
	DefaultPodCount        = 50000
	DefaultEventCount      = 100000
	DefaultPodsPerWorkload = 10
	DefaultHistoryPoints   = 6
)

var datasetVersionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,31}$`)

type Config struct {
	SchemaVersion   string    `json:"schema_version"`
	DatasetVersion  string    `json:"dataset_version"`
	Seed            uint64    `json:"seed"`
	ClusterID       int64     `json:"cluster_id"`
	ObservedAt      time.Time `json:"observed_at"`
	NodeCount       int       `json:"node_count"`
	NamespaceCount  int       `json:"namespace_count"`
	PodCount        int       `json:"pod_count"`
	EventCount      int       `json:"event_count"`
	PodsPerWorkload int       `json:"pods_per_workload"`
	HistoryPoints   int       `json:"history_points"`
}

type Counts struct {
	Nodes          int64 `json:"nodes"`
	Workloads      int64 `json:"workloads"`
	Pods           int64 `json:"pods"`
	Events         int64 `json:"events"`
	HistorySamples int64 `json:"history_samples"`
}

type TopologyCoverage struct {
	NodesReferenced int64 `json:"nodes_referenced"`
	OwnsEdges       int64 `json:"owns_edges"`
	SelectsEdges    int64 `json:"selects_edges"`
	RunsOnEdges     int64 `json:"runs_on_edges"`
	RoutesToEdges   int64 `json:"routes_to_edges"`
}

type WorkloadCoverage struct {
	Namespaces  int64 `json:"namespaces"`
	Deployments int64 `json:"deployments"`
	ReplicaSets int64 `json:"replica_sets"`
	Services    int64 `json:"services"`
	Ingresses   int64 `json:"ingresses"`
	Pods        int64 `json:"pods"`
}

type SearchCoverage struct {
	Pods        int64    `json:"pods"`
	Deployments int64    `json:"deployments"`
	Services    int64    `json:"services"`
	Ingresses   int64    `json:"ingresses"`
	Total       int64    `json:"total"`
	Terms       []string `json:"terms"`
}

type HistoryCoverage struct {
	NodeSeries      int64 `json:"node_series"`
	PodSeries       int64 `json:"pod_series"`
	PointsPerSeries int64 `json:"points_per_series"`
	Samples         int64 `json:"samples"`
}

type Coverage struct {
	Topology  TopologyCoverage `json:"topology"`
	Workloads WorkloadCoverage `json:"workloads"`
	Search    SearchCoverage   `json:"search"`
	History   HistoryCoverage  `json:"history"`
}

type Summary struct {
	Counts   Counts   `json:"counts"`
	Coverage Coverage `json:"coverage"`
}

var (
	ErrOutputExists    = errors.New("scale fixture output already exists")
	ErrInvalidManifest = errors.New("scale fixture manifest is invalid")
)

func DefaultConfig() Config {
	return Config{
		SchemaVersion: SchemaVersion, DatasetVersion: DefaultDatasetVersion,
		Seed: DefaultSeed, ClusterID: DefaultClusterID,
		ObservedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		NodeCount:  DefaultNodeCount, NamespaceCount: DefaultNamespaceCount,
		PodCount: DefaultPodCount, EventCount: DefaultEventCount,
		PodsPerWorkload: DefaultPodsPerWorkload, HistoryPoints: DefaultHistoryPoints,
	}
}

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	var config Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode scale fixture config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	config.ObservedAt = config.ObservedAt.UTC()
	return config, nil
}

func (c Config) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version must be %q", SchemaVersion)
	}
	if !datasetVersionPattern.MatchString(c.DatasetVersion) {
		return errors.New("dataset_version is invalid")
	}
	if c.Seed == 0 || c.ClusterID < 1 || c.ObservedAt.IsZero() {
		return errors.New("seed, cluster_id and observed_at are required")
	}
	if c.NodeCount < 1 || c.NodeCount > 10000 || c.NamespaceCount < 1 || c.NamespaceCount > 1000 {
		return errors.New("node_count or namespace_count is outside the supported range")
	}
	if c.PodCount < 1 || c.PodCount > 1000000 || c.EventCount < 1 || c.EventCount > 2000000 {
		return errors.New("pod_count or event_count is outside the supported range")
	}
	if c.PodsPerWorkload < 1 || c.PodsPerWorkload > 1000 || c.HistoryPoints < 1 || c.HistoryPoints > 1440 {
		return errors.New("pods_per_workload or history_points is outside the supported range")
	}
	if c.PodCount%c.NamespaceCount != 0 || c.PodCount%c.PodsPerWorkload != 0 {
		return errors.New("pod_count must divide evenly into namespaces and workloads")
	}
	if c.EventCount != c.PodCount*2 {
		return errors.New("event_count must be exactly two events per pod")
	}
	return nil
}

func (c Config) WorkloadCount() int { return c.PodCount / c.PodsPerWorkload }

func (c Config) Summary() Summary {
	workloads := int64(c.WorkloadCount())
	pods := int64(c.PodCount)
	nodes := int64(c.NodeCount)
	historySeries := nodes*2 + pods*2
	return Summary{
		Counts: Counts{
			Nodes: nodes, Workloads: workloads, Pods: pods, Events: int64(c.EventCount),
			HistorySamples: historySeries * int64(c.HistoryPoints),
		},
		Coverage: Coverage{
			Topology: TopologyCoverage{
				NodesReferenced: nodes, OwnsEdges: pods, SelectsEdges: pods,
				RunsOnEdges: pods, RoutesToEdges: workloads,
			},
			Workloads: WorkloadCoverage{
				Namespaces: int64(c.NamespaceCount), Deployments: workloads,
				ReplicaSets: workloads, Services: workloads, Ingresses: workloads, Pods: pods,
			},
			Search: SearchCoverage{
				Pods: pods, Deployments: workloads, Services: workloads, Ingresses: workloads,
				Total: pods + workloads*3, Terms: searchTerms(c),
			},
			History: HistoryCoverage{
				NodeSeries: nodes * 2, PodSeries: pods * 2,
				PointsPerSeries: int64(c.HistoryPoints), Samples: historySeries * int64(c.HistoryPoints),
			},
		},
	}
}

func (c Config) Hash() (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func searchTerms(c Config) []string {
	terms := make([]string, 0, 4)
	for _, prefix := range workloadPrefixes {
		if c.WorkloadCount() == 0 {
			break
		}
		terms = append(terms, prefix)
	}
	return terms
}
