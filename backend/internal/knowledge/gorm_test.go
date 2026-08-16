package knowledge

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// newMockGorm builds a GORM handle backed by sqlmock so the repository's raw
// SQL branches can be exercised without a real Postgres.
func newMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open with sqlmock: %v", err)
	}
	return gdb, mock
}

func TestGormRepositoryInsertNewEntry(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	ctx := context.Background()
	noted := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

	// Dedup lookup returns no existing row.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM knowledge_entries
		WHERE rule_id = $1 AND resource_kind = $2 AND resource_name = $3 AND root_causes->>0 = $4
		ORDER BY noted_at DESC LIMIT 1`)).
		WithArgs("crash_loop", "Deployment", "api", "image_pull_backoff").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// INSERT ... RETURNING id
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO knowledge_entries
			(source_diagnosis_id, rule_id, severity, resource_kind, resource_namespace,
			 resource_name, summary, root_causes, recommendations, noted_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		RETURNING id`)).
		WithArgs(int64(7), "crash_loop", "high", "Deployment", "payments", "api",
			"crash summary", sqlmock.AnyArg(), sqlmock.AnyArg(), noted).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	entry := Entry{
		SourceDiagnosisID: 7, RuleID: "crash_loop", Severity: "high",
		ResourceKind: "Deployment", ResourceNamespace: "payments", ResourceName: "api",
		Summary: "crash summary", RootCauses: []string{"image_pull_backoff"},
		Recommendations: []string{"check registry"}, NotedAt: noted,
	}
	got, err := repo.Insert(ctx, entry)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got.ID != 42 {
		t.Fatalf("id = %d, want 42", got.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryInsertUpdatesExisting(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	ctx := context.Background()
	noted := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

	// Dedup lookup finds an existing row.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM knowledge_entries
		WHERE rule_id = $1 AND resource_kind = $2 AND resource_name = $3 AND root_causes->>0 = $4
		ORDER BY noted_at DESC LIMIT 1`)).
		WithArgs("crash_loop", "Deployment", "api", "image_pull_backoff").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	// UPDATE ... RETURNING id
	mock.ExpectQuery(regexp.QuoteMeta(`UPDATE knowledge_entries
			SET source_diagnosis_id = $1, severity = $2, summary = $3, root_causes = $4,
			    recommendations = $5, noted_at = $6, created_at = NOW()
			WHERE id = $7 RETURNING id`)).
		WithArgs(int64(99), "critical", "new summary", sqlmock.AnyArg(), sqlmock.AnyArg(), noted, int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))

	entry := Entry{
		SourceDiagnosisID: 99, RuleID: "crash_loop", Severity: "critical",
		ResourceKind: "Deployment", ResourceName: "api", Summary: "new summary",
		RootCauses: []string{"image_pull_backoff"}, NotedAt: noted,
	}
	got, err := repo.Insert(ctx, entry)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got.ID != 9 {
		t.Fatalf("id = %d, want 9", got.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListByFilter(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	ctx := context.Background()
	noted := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, source_diagnosis_id, rule_id, severity, resource_kind,
	                 resource_namespace, resource_name, summary, root_causes,
	                 recommendations, noted_at
	          FROM knowledge_entries WHERE 1 = 1 AND rule_id = $1 AND severity IN ('high', 'critical') ORDER BY noted_at DESC LIMIT $2`)).
		WithArgs("crash_loop", 5).
		WillReturnRows(sqlmock.NewRows([]string{"id", "source_diagnosis_id", "rule_id", "severity", "resource_kind",
			"resource_namespace", "resource_name", "summary", "root_causes", "recommendations", "noted_at"}).
			AddRow(42, 7, "crash_loop", "high", "Deployment", "payments", "api",
				"crash", `["image_pull_backoff"]`, `["check registry"]`, noted))

	resp, err := repo.ListByFilter(ctx, Filter{RuleID: "crash_loop", MinSeverity: string(SeverityHigh), Limit: 5})
	if err != nil {
		t.Fatalf("ListByFilter: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	got := resp.Items[0]
	if got.ID != 42 || got.RuleID != "crash_loop" || got.Summary != "crash" {
		t.Fatalf("item = %#v", got)
	}
	if len(got.RootCauses) != 1 || got.RootCauses[0] != "image_pull_backoff" {
		t.Fatalf("root causes = %#v", got.RootCauses)
	}
	if len(got.Recommendations) != 1 || got.Recommendations[0] != "check registry" {
		t.Fatalf("recommendations = %#v", got.Recommendations)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryCount(t *testing.T) {
	gdb, mock := newMockGorm(t)
	repo := NewGormRepository(gdb)
	ctx := context.Background()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM knowledge_entries`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMustParseRerankResult(t *testing.T) {
	parsed := MustParseRerankResult([]byte(`{"picked_indices":[0,2,1]}`))
	if len(parsed.PickedIndices) != 3 || parsed.PickedIndices[0] != 0 || parsed.PickedIndices[2] != 1 {
		t.Fatalf("parsed = %#v", parsed)
	}
	empty := MustParseRerankResult([]byte(`{}`))
	if len(empty.PickedIndices) != 0 {
		t.Fatalf("empty parsed = %#v", empty)
	}
}
