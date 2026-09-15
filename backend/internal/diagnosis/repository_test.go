package diagnosis

// Covers the Postgres-backed GormRepository: the pure decodeStoredRecord
// projection plus every raw-SQL method (Save/List/Get/Transition/Assign/
// AddFeedback/Summary/ListByClusters/StatsByClusters) on both the happy path
// and the error path. The repository uses Postgres-only SQL (JSONB casts,
// FILTER (WHERE ...), FOR UPDATE OF), so sqlmock is the only offline option.
// Expectations are matched with loose (unanchored) regexp fragments so the
// tests assert behaviour instead of freezing the exact statement text.

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newDiagnosisMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
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

var recordColumns = []string{
	"id", "cluster_id", "rule_id", "severity", "resource_kind", "resource_namespace",
	"resource_name", "resource_uid", "status", "summary", "root_causes", "recommendations",
	"assignee_id", "assignee_name", "observed_at", "created_at", "updated_at", "sla_due_at",
	"resolved_at", "overdue",
}

// expectGetRecord wires the five statements Get issues: the record row, the
// evidence scan and the three workflow scans.
func expectGetRecord(mock sqlmock.Sqlmock, id int64, status string, resolvedAt any) {
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			id, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", status,
			"node is not ready", `["kubelet down"]`, `["restart kubelet"]`,
			1, "ops", observed, observed, observed, observed.Add(4*time.Hour), resolvedAt, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}).
			AddRow("node_condition", "node.status.conditions", `{"type":"Ready","status":"False"}`))
	mock.ExpectQuery(`FROM diagnosis_activities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_status", "to_status", "comment", "created_at"}).
			AddRow(3, 1, "ops", "open", status, "triaged", observed))
	mock.ExpectQuery(`FROM diagnosis_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "verdict", "comment", "created_at"}).
			AddRow(4, 2, "sre", "accurate", "spot on", observed))
	mock.ExpectQuery(`FROM diagnosis_assignments`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_assignee_user_id", "from_assignee_name", "to_assignee_user_id", "to_assignee_name", "comment", "created_at"}).
			AddRow(5, 1, "ops", 2, "sre", 1, "ops", "handover", observed))
}

// --- decodeStoredRecord (pure) ---

func TestDecodeStoredRecordFullProjection(t *testing.T) {
	resolved := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	item := storedRecord{
		ID: 11, ClusterID: 7, RuleID: RuleCrashLoopBackOff, Severity: "high",
		ResourceKind: "Pod", ResourceNamespace: "demo", ResourceName: "api-0", ResourceUID: "uid-1",
		Status: "resolved", Summary: "crash loop",
		RootCauses:      `["image_pull_backoff","bad probe"]`,
		Recommendations: `["check registry"]`,
		AssigneeID:      sql.NullInt64{Int64: 5, Valid: true},
		AssigneeName:    sql.NullString{String: "ops", Valid: true},
		ObservedAt:      observed, CreatedAt: observed, UpdatedAt: observed,
		SLADueAt: observed.Add(4 * time.Hour), ResolvedAt: sql.NullTime{Time: resolved, Valid: true},
		Overdue: true,
	}
	got := decodeStoredRecord(item)
	if got.ID != 11 || got.ClusterID != 7 || got.RuleID != RuleCrashLoopBackOff || got.Severity != "high" {
		t.Fatalf("identity fields = %#v", got)
	}
	if got.Resource != (ResourceRef{Kind: "Pod", Namespace: "demo", Name: "api-0", UID: "uid-1"}) {
		t.Fatalf("resource = %#v", got.Resource)
	}
	if len(got.RootCauses) != 2 || got.RootCauses[0] != "image_pull_backoff" || got.RootCauses[1] != "bad probe" {
		t.Fatalf("root causes = %#v", got.RootCauses)
	}
	if len(got.Recommendations) != 1 || got.Recommendations[0] != "check registry" {
		t.Fatalf("recommendations = %#v", got.Recommendations)
	}
	if got.Assignee == nil || got.Assignee.ID != 5 || got.Assignee.Name != "ops" {
		t.Fatalf("assignee = %#v", got.Assignee)
	}
	if got.ResolvedAt == nil || !got.ResolvedAt.Equal(resolved) {
		t.Fatalf("resolved at = %#v", got.ResolvedAt)
	}
	if !got.Overdue || !got.SLADueAt.Equal(observed.Add(4*time.Hour)) {
		t.Fatalf("overdue = %v, sla = %v", got.Overdue, got.SLADueAt)
	}
}

func TestDecodeStoredRecordOmitsOptionalFields(t *testing.T) {
	// Invalid JSON and NULL columns must degrade to nil instead of panicking.
	got := decodeStoredRecord(storedRecord{ID: 1, RootCauses: "", Recommendations: "{not json"})
	if got.ResolvedAt != nil {
		t.Fatalf("resolved at = %#v, want nil", got.ResolvedAt)
	}
	if got.Assignee != nil {
		t.Fatalf("assignee = %#v, want nil", got.Assignee)
	}
	if got.RootCauses != nil || got.Recommendations != nil {
		t.Fatalf("root causes = %#v, recommendations = %#v", got.RootCauses, got.Recommendations)
	}
	if got.Overdue {
		t.Fatal("overdue should default to false")
	}
}

// --- Save ---

func TestGormRepositorySaveInsertsRecordAndEvidence(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	created := observed.Add(time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO diagnosis_records`).
		WithArgs(int64(7), RuleNodeNotReady, "critical", "Node", "", "worker-1", "uid-1", "open",
			"node down", sqlmock.AnyArg(), sqlmock.AnyArg(), observed, observed.Add(time.Hour)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(42, created, created))
	mock.ExpectExec(`INSERT INTO diagnosis_evidence`).
		WithArgs(int64(42), "node_condition", "node.status.conditions", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	record := &Record{
		ClusterID: 7, RuleID: RuleNodeNotReady, Severity: "critical", Status: "open",
		Resource: ResourceRef{Kind: "Node", Name: "worker-1", UID: "uid-1"},
		Summary:  "node down", ObservedAt: observed,
		RootCauses: []string{"kubelet down"},
		Evidence:   []Evidence{{Type: "node_condition", Source: "node.status.conditions", Content: map[string]any{"status": "False"}}},
	}
	if err := repo.Save(context.Background(), record); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if record.ID != 42 {
		t.Fatalf("id = %d, want 42", record.ID)
	}
	if !record.CreatedAt.Equal(created) || !record.UpdatedAt.Equal(created) {
		t.Fatalf("timestamps = %v / %v", record.CreatedAt, record.UpdatedAt)
	}
	// Save must derive the SLA deadline when the caller left it unset.
	if want := SLADeadline("critical", observed); !record.SLADueAt.Equal(want) {
		t.Fatalf("sla due = %v, want %v", record.SLADueAt, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositorySaveKeepsExplicitSLADeadline(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	explicit := observed.Add(30 * time.Minute)

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO diagnosis_records`).
		WithArgs(int64(1), RulePodPending, "high", "Pod", "demo", "api", "", "open",
			"pending", sqlmock.AnyArg(), sqlmock.AnyArg(), observed, explicit).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(9, observed, observed))
	mock.ExpectCommit()

	record := &Record{
		ClusterID: 1, RuleID: RulePodPending, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Pod", Namespace: "demo", Name: "api"},
		Summary:  "pending", ObservedAt: observed, SLADueAt: explicit,
	}
	if err := repo.Save(context.Background(), record); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !record.SLADueAt.Equal(explicit) {
		t.Fatalf("sla due = %v, want %v", record.SLADueAt, explicit)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositorySaveInsertErrorRollsBack(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("insert exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO diagnosis_records`).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Save(context.Background(), &Record{
		ClusterID: 7, RuleID: RuleNodeNotReady, Severity: "high",
		Resource: ResourceRef{Kind: "Node", Name: "worker-1"}, ObservedAt: observed,
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositorySaveEvidenceErrorRollsBack(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("evidence exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).AddRow(42, observed, observed))
	mock.ExpectExec(`INSERT INTO diagnosis_evidence`).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Save(context.Background(), &Record{
		ClusterID: 7, RuleID: RuleNodeNotReady, Severity: "high", Status: "open",
		Resource: ResourceRef{Kind: "Node", Name: "worker-1"}, ObservedAt: observed,
		Evidence: []Evidence{{Type: "event", Source: "node.events", Content: map[string]any{"reason": "x"}}},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- List ---

func TestGormRepositoryListAppliesEveryFilter(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	updatedAfter := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	overdue := true

	mock.ExpectQuery(regexp.QuoteMeta(`d.cluster_id = $1 AND d.status = $2 AND d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW() AND d.observed_at >= $3 AND d.updated_at > $4 ORDER BY d.created_at DESC, d.id DESC LIMIT $5`)).
		WithArgs(int64(7), "open", since, updatedAfter, 5).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			42, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `["kubelet down"]`, `["restart kubelet"]`, nil, nil,
			since, since, since, since.Add(time.Hour), nil, true))

	items, err := repo.List(context.Background(), ListFilter{
		ClusterID: 7, Status: "open", Overdue: &overdue,
		Since: &since, UpdatedAfter: &updatedAfter, Limit: 5,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].ID != 42 || items[0].RuleID != RuleNodeNotReady || !items[0].Overdue {
		t.Fatalf("item = %#v", items[0])
	}
	if len(items[0].RootCauses) != 1 || items[0].RootCauses[0] != "kubelet down" {
		t.Fatalf("root causes = %#v", items[0].RootCauses)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListNotOverdueFilter(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	overdue := false

	mock.ExpectQuery(regexp.QuoteMeta(`NOT (d.status IN ('open', 'confirmed') AND d.sla_due_at < NOW()) ORDER BY d.created_at DESC, d.id DESC LIMIT $1`)).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows(recordColumns))

	items, err := repo.List(context.Background(), ListFilter{Overdue: &overdue, Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListPropagatesQueryError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("list exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id`).WillReturnError(boom)

	if _, err := repo.List(context.Background(), ListFilter{Limit: 5}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Get ---

func TestGormRepositoryGetLoadsEvidenceAndWorkflow(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	expectGetRecord(mock, 42, "resolved", time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC))

	record, err := repo.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if record.ID != 42 || record.Status != "resolved" || record.RuleID != RuleNodeNotReady {
		t.Fatalf("record = %#v", record)
	}
	if record.Assignee == nil || record.Assignee.ID != 1 {
		t.Fatalf("assignee = %#v", record.Assignee)
	}
	if record.ResolvedAt == nil {
		t.Fatal("resolved_at was not decoded")
	}
	if len(record.Evidence) != 1 || record.Evidence[0].Type != "node_condition" {
		t.Fatalf("evidence = %#v", record.Evidence)
	}
	if record.Evidence[0].Content["status"] != "False" {
		t.Fatalf("evidence content = %#v", record.Evidence[0].Content)
	}
	if len(record.Activities) != 1 || record.Activities[0].ToStatus != "resolved" || record.Activities[0].Actor.Name != "ops" {
		t.Fatalf("activities = %#v", record.Activities)
	}
	if len(record.Feedback) != 1 || record.Feedback[0].Verdict != "accurate" {
		t.Fatalf("feedback = %#v", record.Feedback)
	}
	if len(record.Assignments) != 1 {
		t.Fatalf("assignments = %#v", record.Assignments)
	}
	assignment := record.Assignments[0]
	if assignment.FromAssignee == nil || assignment.FromAssignee.ID != 2 || assignment.FromAssignee.Name != "sre" {
		t.Fatalf("from assignee = %#v", assignment.FromAssignee)
	}
	if assignment.ToAssignee.ID != 1 || assignment.ToAssignee.Name != "ops" {
		t.Fatalf("to assignee = %#v", assignment.ToAssignee)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetWithoutPreviousAssignee(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}))
	mock.ExpectQuery(`FROM diagnosis_activities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_status", "to_status", "comment", "created_at"}))
	mock.ExpectQuery(`FROM diagnosis_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "verdict", "comment", "created_at"}))
	// A first-ever assignment row has no from_assignee_* values.
	mock.ExpectQuery(`FROM diagnosis_assignments`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_assignee_user_id", "from_assignee_name", "to_assignee_user_id", "to_assignee_name", "comment", "created_at"}).
			AddRow(5, 1, "ops", nil, "", 2, "sre", "", observed))

	record, err := repo.Get(context.Background(), 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(record.Assignments) != 1 || record.Assignments[0].FromAssignee != nil {
		t.Fatalf("assignments = %#v", record.Assignments)
	}
	if record.ResolvedAt != nil || record.Assignee != nil {
		t.Fatalf("optional fields = %#v / %#v", record.ResolvedAt, record.Assignee)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetNotFound(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns))

	if _, err := repo.Get(context.Background(), 404); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetPropagatesRowError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("row exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).WillReturnError(boom)

	if _, err := repo.Get(context.Background(), 42); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetEvidenceDecodeError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}).
			AddRow("node_condition", "node.status.conditions", `{"broken"`))

	_, err := repo.Get(context.Background(), 7)
	if err == nil || !regexp.MustCompile(`decode diagnosis evidence`).MatchString(err.Error()) {
		t.Fatalf("err = %v, want evidence decode failure", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetEvidenceQueryError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("evidence query exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).WillReturnError(boom)

	if _, err := repo.Get(context.Background(), 7); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetWorkflowQueryError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("activities exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}))
	mock.ExpectQuery(`FROM diagnosis_activities`).WillReturnError(boom)

	if _, err := repo.Get(context.Background(), 7); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetFeedbackQueryError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("feedback exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}))
	mock.ExpectQuery(`FROM diagnosis_activities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_status", "to_status", "comment", "created_at"}))
	mock.ExpectQuery(`FROM diagnosis_feedback`).WillReturnError(boom)

	if _, err := repo.Get(context.Background(), 7); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryGetAssignmentsQueryError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	boom := errors.New("assignments exploded")

	mock.ExpectQuery(`SELECT d.id, d.cluster_id, d.rule_id`).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			7, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))
	mock.ExpectQuery(`SELECT evidence_type AS type, source, content::text AS content`).
		WillReturnRows(sqlmock.NewRows([]string{"type", "source", "content"}))
	mock.ExpectQuery(`FROM diagnosis_activities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "from_status", "to_status", "comment", "created_at"}))
	mock.ExpectQuery(`FROM diagnosis_feedback`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor_user_id", "actor_name", "verdict", "comment", "created_at"}))
	mock.ExpectQuery(`FROM diagnosis_assignments`).WillReturnError(boom)

	if _, err := repo.Get(context.Background(), 7); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Transition ---

func TestGormRepositoryTransitionClaimsUnassignedRecord(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	actor := ActorRef{ID: 1, Name: "ops"}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", nil))
	mock.ExpectExec(`UPDATE diagnosis_records SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	// Unassigned records get an implicit claim row before the activity row.
	mock.ExpectExec(`INSERT INTO diagnosis_assignments`).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_activities`).WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()
	expectGetRecord(mock, 42, "confirmed", nil)

	record, err := repo.Transition(context.Background(), 42, "confirmed", actor, "triaged")
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if record.ID != 42 || record.Status != "confirmed" {
		t.Fatalf("record = %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionKeepsExistingAssignee(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", int64(9)))
	mock.ExpectExec(`UPDATE diagnosis_records SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_activities`).WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectCommit()
	expectGetRecord(mock, 42, "dismissed", nil)

	record, err := repo.Transition(context.Background(), 42, "dismissed", ActorRef{ID: 1, Name: "ops"}, "")
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if record.Status != "dismissed" {
		t.Fatalf("status = %q", record.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionRejectsInvalidTarget(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", nil))
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "resolved", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionMissingRecord(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}))
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 404, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionUpdateError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("update exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", int64(1)))
	mock.ExpectExec(`UPDATE diagnosis_records SET status`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionClaimInsertError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("claim insert exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", nil))
	mock.ExpectExec(`UPDATE diagnosis_records SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_assignments`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionActivityInsertError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("activity insert exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("open", int64(1)))
	mock.ExpectExec(`UPDATE diagnosis_records SET status`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_activities`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionLookupError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("lookup exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryTransitionUnknownCurrentStatus(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	// An unrecognised stored status has no legal transition at all.
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT status, assigned_to_user_id FROM diagnosis_records`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "assigned_to_user_id"}).AddRow("archived", int64(1)))
	mock.ExpectRollback()

	if _, err := repo.Transition(context.Background(), 42, "confirmed", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("err = %v, want ErrInvalidTransition", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Assign ---

func TestGormRepositoryAssignRecordsPreviousOwner(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}).AddRow(int64(1), "ops"))
	mock.ExpectExec(`UPDATE diagnosis_records SET assigned_to_user_id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_assignments`).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectCommit()
	expectGetRecord(mock, 42, "open", nil)

	record, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, "handover")
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if record.ID != 42 {
		t.Fatalf("record = %#v", record)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignFromUnassigned(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}).AddRow(nil, ""))
	mock.ExpectExec(`UPDATE diagnosis_records SET assigned_to_user_id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_assignments`).WillReturnResult(sqlmock.NewResult(6, 1))
	mock.ExpectCommit()
	expectGetRecord(mock, 42, "open", nil)

	if _, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignRejectsSameAssignee(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}).AddRow(int64(2), "sre"))
	mock.ExpectRollback()

	if _, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrAlreadyAssigned) {
		t.Fatalf("err = %v, want ErrAlreadyAssigned", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignMissingRecord(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}))
	mock.ExpectRollback()

	if _, err := repo.Assign(context.Background(), 404, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignUpdateError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("assign update exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}).AddRow(nil, ""))
	mock.ExpectExec(`UPDATE diagnosis_records SET assigned_to_user_id`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignLookupError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("assign lookup exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAssignAssignmentInsertError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("assignment insert exploded")

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT d.assigned_to_user_id, COALESCE`).
		WillReturnRows(sqlmock.NewRows([]string{"assigned_to_user_id", "coalesce"}).AddRow(nil, ""))
	mock.ExpectExec(`UPDATE diagnosis_records SET assigned_to_user_id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO diagnosis_assignments`).WillReturnError(boom)
	mock.ExpectRollback()

	if _, err := repo.Assign(context.Background(), 42, ActorRef{ID: 2, Name: "sre"}, ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- AddFeedback ---

func TestGormRepositoryAddFeedbackInsertsRow(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectExec(`INSERT INTO diagnosis_feedback`).WillReturnResult(sqlmock.NewResult(11, 1))
	expectGetRecord(mock, 42, "open", nil)

	record, err := repo.AddFeedback(context.Background(), 42, "accurate", ActorRef{ID: 1, Name: "ops"}, "spot on")
	if err != nil {
		t.Fatalf("AddFeedback: %v", err)
	}
	if len(record.Feedback) != 1 || record.Feedback[0].Verdict != "accurate" {
		t.Fatalf("feedback = %#v", record.Feedback)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAddFeedbackMissingRecord(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	// The INSERT ... SELECT matches no record, so zero rows are affected.
	mock.ExpectExec(`INSERT INTO diagnosis_feedback`).WillReturnResult(sqlmock.NewResult(0, 0))

	if _, err := repo.AddFeedback(context.Background(), 404, "accurate", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("err = %v, want ErrRecordNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryAddFeedbackPropagatesExecError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("feedback exploded")

	mock.ExpectExec(`INSERT INTO diagnosis_feedback`).WillReturnError(boom)

	if _, err := repo.AddFeedback(context.Background(), 42, "accurate", ActorRef{ID: 1, Name: "ops"}, ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- Summary ---

func TestGormRepositorySummaryAggregatesAndListsRecent(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT COUNT\(\*\) AS total`).
		WillReturnRows(sqlmock.NewRows([]string{"total", "open", "confirmed", "resolved", "dismissed", "overdue"}).
			AddRow(9, 3, 2, 3, 1, 2))
	mock.ExpectQuery(regexp.QuoteMeta(`ORDER BY d.updated_at DESC, d.id DESC LIMIT $1`)).
		WithArgs(5).
		WillReturnRows(sqlmock.NewRows(recordColumns).AddRow(
			42, 7, RuleNodeNotReady, "high", "Node", "", "worker-1", "uid-1", "open",
			"node down", `[]`, `[]`, nil, nil, observed, observed, observed, observed, nil, false))

	summary, err := repo.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Total != 9 || summary.Open != 3 || summary.Confirmed != 2 || summary.Resolved != 3 || summary.Dismissed != 1 || summary.Overdue != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.Recent) != 1 || summary.Recent[0].ID != 42 {
		t.Fatalf("recent = %#v", summary.Recent)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositorySummaryPropagatesErrors(t *testing.T) {
	boom := errors.New("summary exploded")

	t.Run("count query", func(t *testing.T) {
		gdb, mock := newDiagnosisMockGorm(t)
		mock.ExpectQuery(`SELECT COUNT\(\*\) AS total`).WillReturnError(boom)
		if _, err := NewGormRepository(gdb).Summary(context.Background()); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("recent list query", func(t *testing.T) {
		gdb, mock := newDiagnosisMockGorm(t)
		mock.ExpectQuery(`SELECT COUNT\(\*\) AS total`).
			WillReturnRows(sqlmock.NewRows([]string{"total", "open", "confirmed", "resolved", "dismissed", "overdue"}).
				AddRow(0, 0, 0, 0, 0, 0))
		mock.ExpectQuery(`SELECT d.id, d.cluster_id`).WillReturnError(boom)
		if _, err := NewGormRepository(gdb).Summary(context.Background()); !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})
}

// --- ListByClusters ---

func TestGormRepositoryListByClustersEmptyScopeReturnsNoRows(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	rows, err := repo.ListByClusters(context.Background(), nil, "", "", 10)
	if err != nil {
		t.Fatalf("ListByClusters: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %#v", rows)
	}
	// Fail-closed: an empty scope must not touch the database at all.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListByClustersAppliesFilters(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`AND d.status = $3 AND d.severity = $4 ORDER BY d.observed_at DESC, d.id DESC LIMIT $5`)).
		WithArgs(int64(7), int64(8), "open", "high", 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cluster_id", "rule_id", "severity", "resource_kind",
			"resource_name", "resource_namespace", "status", "summary", "observed_at", "resolved_at"}).
			AddRow(42, 7, RuleNodeNotReady, "high", "Node", "worker-1", "", "open", "node down", observed, nil))

	rows, err := repo.ListByClusters(context.Background(), []int64{7, 8}, "open", "high", 20)
	if err != nil {
		t.Fatalf("ListByClusters: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	got := rows[0]
	if got.ID != 42 || got.ClusterID != 7 || got.RuleID != RuleNodeNotReady || got.ResourceKind != "Node" || got.Status != "open" {
		t.Fatalf("row = %#v", got)
	}
	if got.ResolvedAt != nil {
		t.Fatalf("resolved at = %#v", got.ResolvedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListByClustersWithoutFilters(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`d.cluster_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cluster_id"}))

	rows, err := repo.ListByClusters(context.Background(), []int64{3}, "", "", 5)
	if err != nil {
		t.Fatalf("ListByClusters: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %#v", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryListByClustersError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	boom := errors.New("clusters exploded")

	mock.ExpectQuery(`d.cluster_id IN`).WillReturnError(boom)

	if _, err := NewGormRepository(gdb).ListByClusters(context.Background(), []int64{3}, "", "", 5); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// --- StatsByClusters ---

func TestGormRepositoryStatsByClustersEmptyScopeIsZeroed(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)

	stats, err := NewGormRepository(gdb).StatsByClusters(context.Background(), []int64{})
	if err != nil {
		t.Fatalf("StatsByClusters: %v", err)
	}
	if stats.Total != 0 || len(stats.ByStatus) != 0 || len(stats.BySeverity) != 0 || len(stats.ByCluster) != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	if stats.ByStatus == nil || stats.BySeverity == nil || stats.ByCluster == nil {
		t.Fatalf("maps/slices must be non-nil: %#v", stats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryStatsByClustersAggregatesGroups(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)

	mock.ExpectQuery(`SELECT status, severity, cluster_id, COUNT\(\*\) AS count`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "severity", "cluster_id", "count"}).
			AddRow("open", "high", int64(8), 2).
			AddRow("resolved", "critical", int64(7), 1).
			AddRow("open", "critical", int64(8), 3))

	stats, err := NewGormRepository(gdb).StatsByClusters(context.Background(), []int64{7, 8})
	if err != nil {
		t.Fatalf("StatsByClusters: %v", err)
	}
	if stats.Total != 6 {
		t.Fatalf("total = %d, want 6", stats.Total)
	}
	if stats.ByStatus["open"] != 5 || stats.ByStatus["resolved"] != 1 {
		t.Fatalf("by status = %#v", stats.ByStatus)
	}
	if stats.BySeverity["high"] != 2 || stats.BySeverity["critical"] != 4 {
		t.Fatalf("by severity = %#v", stats.BySeverity)
	}
	if len(stats.ByCluster) != 2 {
		t.Fatalf("by cluster = %#v", stats.ByCluster)
	}
	// Grouping must be emitted in ascending cluster order for stable output.
	if stats.ByCluster[0].ClusterID != 7 || stats.ByCluster[0].Count != 1 {
		t.Fatalf("first cluster = %#v", stats.ByCluster[0])
	}
	if stats.ByCluster[1].ClusterID != 8 || stats.ByCluster[1].Count != 5 {
		t.Fatalf("second cluster = %#v", stats.ByCluster[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGormRepositoryStatsByClustersError(t *testing.T) {
	gdb, mock := newDiagnosisMockGorm(t)
	boom := errors.New("stats exploded")

	mock.ExpectQuery(`SELECT status, severity, cluster_id`).WillReturnError(boom)

	if _, err := NewGormRepository(gdb).StatsByClusters(context.Background(), []int64{7}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
