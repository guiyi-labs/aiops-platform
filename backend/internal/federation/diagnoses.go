package federation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"k8s-aiops.local/backend/internal/diagnosis"
)

// FederationDiagnosisRepository is the minimal diagnosis read surface used by
// the cross-cluster diagnosis aggregation. It is the platform-side view: the
// diagnosis_records table is already populated centrally (per-cluster via the
// diagnosis resolution hook), so P2d aggregates from it directly.
type FederationDiagnosisRepository interface {
	ListByClusters(ctx context.Context, clusters []int64, status, severity string, limit int) ([]diagnosis.FederationDiagnosisRow, error)
	StatsByClusters(ctx context.Context, clusters []int64) (diagnosis.FederationDiagnosisStats, error)
}

// FederationDiagnosis is the API projection for the cross-cluster fleet view.
type FederationDiagnosis struct {
	ID                int64      `json:"id"`
	ClusterID         int64      `json:"cluster_id"`
	ClusterName       string     `json:"cluster_name,omitempty"`
	RuleID            string     `json:"rule_id"`
	Severity          string     `json:"severity"`
	ResourceKind      string     `json:"resource_kind"`
	ResourceName      string     `json:"resource_name"`
	ResourceNamespace string     `json:"resource_namespace,omitempty"`
	Status            string     `json:"status"`
	Summary           string     `json:"summary"`
	ObservedAt        time.Time  `json:"observed_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
}

// FederationDiagnosisStats aggregates cross-cluster diagnosis counts.
type FederationDiagnosisStats struct {
	Total      int64                   `json:"total"`
	ByStatus   map[string]int64        `json:"by_status"`
	BySeverity map[string]int64        `json:"by_severity"`
	ByCluster  []ClusterDiagnosisCount `json:"by_cluster"`
}

// ClusterDiagnosisCount is the per-cluster contribution to the stats.
type ClusterDiagnosisCount struct {
	ClusterID   int64  `json:"cluster_id"`
	ClusterName string `json:"cluster_name,omitempty"`
	Count       int64  `json:"count"`
}

// FederationDiagnosisQuery bounds the cross-cluster diagnosis aggregation.
type FederationDiagnosisQuery struct {
	Clusters []int64
	Status   string
	Severity string
	Limit    int
}

const MaxDiagnosisLimit = 200
const DefaultDiagnosisLimit = 50

const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
	SeverityInfo     = "info"
)

func isValidSeverity(s string) bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo:
		return true
	}
	return false
}

func isValidStatus(s string) bool {
	switch s {
	case "open", "confirmed", "resolved", "dismissed":
		return true
	}
	return false
}

func (s *Service) ListDiagnoses(ctx context.Context, q FederationDiagnosisQuery) ([]FederationDiagnosis, error) {
	if s.diagnosisRepo == nil {
		return []FederationDiagnosis{}, nil
	}
	if q.Clusters != nil && len(q.Clusters) == 0 {
		return []FederationDiagnosis{}, nil
	}
	if q.Status != "" && !isValidStatus(q.Status) {
		return nil, fmt.Errorf("invalid status %q", q.Status)
	}
	if q.Severity != "" && !isValidSeverity(q.Severity) {
		return nil, fmt.Errorf("invalid severity %q", q.Severity)
	}
	limit := q.Limit
	if limit <= 0 || limit > MaxDiagnosisLimit {
		limit = DefaultDiagnosisLimit
	}
	rows, err := s.diagnosisRepo.ListByClusters(ctx, q.Clusters, q.Status, q.Severity, limit)
	if err != nil {
		return nil, err
	}
	items := make([]FederationDiagnosis, 0, len(rows))
	for _, row := range rows {
		items = append(items, FederationDiagnosis{
			ID: row.ID, ClusterID: row.ClusterID, RuleID: row.RuleID, Severity: row.Severity,
			ResourceKind: row.ResourceKind, ResourceName: row.ResourceName, ResourceNamespace: row.ResourceNamespace,
			Status: row.Status, Summary: row.Summary, ObservedAt: row.ObservedAt, ResolvedAt: row.ResolvedAt,
		})
	}
	s.enrichClusterNames(items)
	return items, nil
}

func (s *Service) DiagnosesStats(ctx context.Context, clusters []int64) (FederationDiagnosisStats, error) {
	if s.diagnosisRepo == nil {
		return FederationDiagnosisStats{}, nil
	}
	if clusters != nil && len(clusters) == 0 {
		return FederationDiagnosisStats{Total: 0, ByStatus: map[string]int64{}, BySeverity: map[string]int64{}, ByCluster: []ClusterDiagnosisCount{}}, nil
	}
	raw, err := s.diagnosisRepo.StatsByClusters(ctx, clusters)
	if err != nil {
		return FederationDiagnosisStats{}, err
	}
	stats := FederationDiagnosisStats{Total: raw.Total, ByStatus: raw.ByStatus, BySeverity: raw.BySeverity, ByCluster: make([]ClusterDiagnosisCount, 0, len(raw.ByCluster))}
	for _, cc := range raw.ByCluster {
		stats.ByCluster = append(stats.ByCluster, ClusterDiagnosisCount{ClusterID: cc.ClusterID, Count: cc.Count})
	}
	s.enrichClusterStats(&stats)
	return stats, nil
}

func (s *Service) enrichClusterNames(items []FederationDiagnosis) {
	if s.repo == nil {
		return
	}
	clusters, err := s.repo.ListClusters(context.Background())
	if err != nil {
		return
	}
	names := make(map[int64]string, len(clusters))
	for _, c := range clusters {
		names[c.ID] = c.Name
	}
	for i := range items {
		if name, ok := names[items[i].ClusterID]; ok {
			items[i].ClusterName = name
		}
	}
}

func (s *Service) enrichClusterStats(stats *FederationDiagnosisStats) {
	if s.repo == nil {
		return
	}
	clusters, err := s.repo.ListClusters(context.Background())
	if err != nil {
		return
	}
	names := make(map[int64]string, len(clusters))
	for _, c := range clusters {
		names[c.ID] = c.Name
	}
	for i := range stats.ByCluster {
		if name, ok := names[stats.ByCluster[i].ClusterID]; ok {
			stats.ByCluster[i].ClusterName = name
		}
	}
}

func safeInList(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}

var _ = safeInList
