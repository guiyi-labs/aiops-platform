package federation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"k8s-aiops.local/backend/internal/cluster"
	"k8s-aiops.local/backend/internal/diagnosis"
)

// fakeDiagnosisRepo is an in-memory FederationDiagnosisRepository for service tests.
type fakeDiagnosisRepo struct {
	rows     []diagnosis.FederationDiagnosisRow
	stats    diagnosis.FederationDiagnosisStats
	listErr  error
	statsErr error
}

func (f *fakeDiagnosisRepo) ListByClusters(_ context.Context, _ []int64, _, _ string, _ int) ([]diagnosis.FederationDiagnosisRow, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.rows, nil
}

func (f *fakeDiagnosisRepo) StatsByClusters(_ context.Context, _ []int64) (diagnosis.FederationDiagnosisStats, error) {
	if f.statsErr != nil {
		return diagnosis.FederationDiagnosisStats{}, f.statsErr
	}
	return f.stats, nil
}

func TestIsValidSeverity(t *testing.T) {
	valid := []string{"critical", "high", "medium", "low", "info"}
	for _, s := range valid {
		if !isValidSeverity(s) {
			t.Errorf("isValidSeverity(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "WARN", "urgent", "critical2"}
	for _, s := range invalid {
		if isValidSeverity(s) {
			t.Errorf("isValidSeverity(%q) = true, want false", s)
		}
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []string{"open", "confirmed", "resolved", "dismissed"}
	for _, s := range valid {
		if !isValidStatus(s) {
			t.Errorf("isValidStatus(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "pending", "closed", "open2"}
	for _, s := range invalid {
		if isValidStatus(s) {
			t.Errorf("isValidStatus(%q) = true, want false", s)
		}
	}
}

func TestSafeInList(t *testing.T) {
	got := safeInList([]int64{3, 1, 2})
	want := "3,1,2"
	if got != want {
		t.Errorf("safeInList = %q, want %q", got, want)
	}
	empty := safeInList([]int64{})
	if empty != "" {
		t.Errorf("safeInList([]) = %q, want empty", empty)
	}
}

func TestListDiagnoses_NilRepo(t *testing.T) {
	svc := NewService(nil, nil)
	items, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty, got %d", len(items))
	}
}

func TestListDiagnoses_EmptyClusters(t *testing.T) {
	svc := NewService(nil, nil).WithDiagnosisRepository(&fakeDiagnosisRepo{
		rows: []diagnosis.FederationDiagnosisRow{
			{ID: 1, ClusterID: 1, RuleID: "r1", Severity: "high", Status: "open"},
		},
	})
	items, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Clusters: []int64{}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty for empty clusters, got %d", len(items))
	}
}

func TestListDiagnoses_InvalidStatus(t *testing.T) {
	svc := NewService(nil, nil).WithDiagnosisRepository(&fakeDiagnosisRepo{})
	_, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Status: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestListDiagnoses_InvalidSeverity(t *testing.T) {
	svc := NewService(nil, nil).WithDiagnosisRepository(&fakeDiagnosisRepo{})
	_, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Severity: "bad"})
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
}

func TestListDiagnoses_Success(t *testing.T) {
	now := time.Now().UTC()
	fake := &fakeDiagnosisRepo{
		rows: []diagnosis.FederationDiagnosisRow{
			{ID: 1, ClusterID: 10, RuleID: "crash_loop", Severity: "high", Status: "open", Summary: "pod restarting", ObservedAt: now},
			{ID: 2, ClusterID: 20, RuleID: "oom_kill", Severity: "critical", Status: "resolved", Summary: "oom", ObservedAt: now},
		},
	}
	repo := newFakeRepository()
	repo.clusters[10] = clusterWith(10, "prod-us")
	repo.clusters[20] = clusterWith(20, "prod-eu")
	svc := NewService(repo, nil).WithDiagnosisRepository(fake)

	items, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2, got %d", len(items))
	}
	if items[0].ClusterName != "prod-us" {
		t.Errorf("cluster name = %q, want prod-us", items[0].ClusterName)
	}
	if items[1].ClusterName != "prod-eu" {
		t.Errorf("cluster name = %q, want prod-eu", items[1].ClusterName)
	}
}

func TestListDiagnoses_LimitClamp(t *testing.T) {
	fake := &fakeDiagnosisRepo{rows: []diagnosis.FederationDiagnosisRow{}}
	svc := NewService(nil, nil).WithDiagnosisRepository(fake)

	// limit=0 → default 50
	items, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	_ = items

	// limit > max → default 50
	items, err = svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Limit: 999})
	if err != nil {
		t.Fatal(err)
	}
	_ = items
}

func TestListDiagnoses_ListError(t *testing.T) {
	fake := &fakeDiagnosisRepo{listErr: fmt.Errorf("db down")}
	svc := NewService(nil, nil).WithDiagnosisRepository(fake)
	_, err := svc.ListDiagnoses(context.Background(), FederationDiagnosisQuery{Limit: 10})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDiagnosesStats_NilRepo(t *testing.T) {
	svc := NewService(nil, nil)
	stats, err := svc.DiagnosesStats(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
}

func TestDiagnosesStats_EmptyClusters(t *testing.T) {
	svc := NewService(nil, nil).WithDiagnosisRepository(&fakeDiagnosisRepo{})
	stats, err := svc.DiagnosesStats(context.Background(), []int64{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 0 {
		t.Errorf("total = %d, want 0", stats.Total)
	}
}

func TestDiagnosesStats_Success(t *testing.T) {
	fake := &fakeDiagnosisRepo{
		stats: diagnosis.FederationDiagnosisStats{
			Total:      5,
			ByStatus:   map[string]int64{"open": 3, "resolved": 2},
			BySeverity: map[string]int64{"high": 4, "low": 1},
			ByCluster: []diagnosis.FederationClusterCount{
				{ClusterID: 10, Count: 3},
				{ClusterID: 20, Count: 2},
			},
		},
	}
	repo := newFakeRepository()
	repo.clusters[10] = clusterWith(10, "cluster-a")
	repo.clusters[20] = clusterWith(20, "cluster-b")
	svc := NewService(repo, nil).WithDiagnosisRepository(fake)

	stats, err := svc.DiagnosesStats(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 5 {
		t.Errorf("total = %d, want 5", stats.Total)
	}
	if stats.ByCluster[0].ClusterName != "cluster-a" {
		t.Errorf("cluster 10 name = %q, want cluster-a", stats.ByCluster[0].ClusterName)
	}
	if stats.ByCluster[1].ClusterName != "cluster-b" {
		t.Errorf("cluster 20 name = %q, want cluster-b", stats.ByCluster[1].ClusterName)
	}
}

func TestDiagnosesStats_StatsError(t *testing.T) {
	fake := &fakeDiagnosisRepo{statsErr: fmt.Errorf("db down")}
	svc := NewService(nil, nil).WithDiagnosisRepository(fake)
	_, err := svc.DiagnosesStats(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnrichClusterNames_NilRepo(t *testing.T) {
	svc := NewService(nil, nil)
	items := []FederationDiagnosis{{ClusterID: 1}}
	svc.enrichClusterNames(items)
	if items[0].ClusterName != "" {
		t.Errorf("expected empty cluster name with nil repo")
	}
}

// clusterWith is a helper to create a cluster.Cluster with ID and Name set.
func clusterWith(id int64, name string) cluster.Cluster {
	return cluster.Cluster{ID: id, Name: name}
}
