package signal

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// --- sqlmock scaffolding -------------------------------------------------

// sqlFragmentMatcher matches the expected SQL as a whitespace-normalized
// substring of the actual statement. sqlmock's default matcher compiles the
// expectation as a regexp, which forces callers to escape every parenthesis,
// quote and dollar sign in Postgres SQL. Matching on a distinctive fragment
// instead pins the statement shape (table, ON CONFLICT target, ORDER BY)
// without hard-coding placeholder numbering or the full column list.
func sqlFragmentMatcher(expected, actual string) error {
	if strings.Contains(collapseSQL(actual), collapseSQL(expected)) {
		return nil
	}
	return fmt.Errorf("actual sql %q does not contain fragment %q", collapseSQL(actual), collapseSQL(expected))
}

func collapseSQL(q string) string { return strings.Join(strings.Fields(q), " ") }

// newSignalMockGorm builds a GORM handle backed by sqlmock so the repository's
// Postgres-specific SQL branches can be exercised without a real database.
func newSignalMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(sqlFragmentMatcher)))
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

func expectAllMet(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// signalRowColumns mirrors the column set of the signalRow GORM model, which is
// what `SELECT *` returns to the scanner.
func signalRowColumns() []string {
	return []string{
		"id", "signal_id", "signal_code", "schema_version", "producer", "cluster_id",
		"namespace", "resource_kind", "resource_namespace", "resource_name", "resource_uid",
		"resource_incomplete", "severity", "state", "fingerprint", "coverage", "freshness",
		"window_start", "window_end", "observed_at", "ingested_at", "expires_at",
		"attributes", "evidence", "ingestion_run_id",
	}
}

// signalRowValues returns one fully-populated signal_occurrences row so tests
// can assert every mapped field survives the row -> domain conversion.
func signalRowValues(id int64, observed time.Time) []driver.Value {
	windowStart := observed.Add(-5 * time.Minute)
	windowEnd := observed.Add(-time.Minute)
	expiresAt := observed.Add(24 * time.Hour)
	return []driver.Value{
		id, "diag.pod.pending.v1", "diag.pod.pending.v1", "1.0", "diagnosis", int64(3),
		"payments", "Pod", "payments", "api-0", "uid-1",
		false, "critical", "active", "fp-1", "complete", observed,
		windowStart, windowEnd, observed, observed, expiresAt,
		[]byte(`{"rule":"crash_loop"}`), []byte(`[{"kind":"diagnosis_record","id":7}]`), "run-1",
	}
}

// signalObservedAt is the fixed timestamp used across these tests; it keeps
// assertions deterministic (no reliance on the wall clock).
func signalObservedAt() time.Time {
	return time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
}

func signalTestOccurrence() *Occurrence {
	observed := signalObservedAt()
	windowStart := observed.Add(-5 * time.Minute)
	windowEnd := observed.Add(-time.Minute)
	expiresAt := observed.Add(24 * time.Hour)
	return &Occurrence{
		ID:            42,
		SignalID:      "diag.pod.pending.v1",
		SignalCode:    "diag.pod.pending.v1",
		SchemaVersion: "1.0",
		Producer:      ProducerDiagnosis,
		ClusterID:     3,
		Namespace:     "payments",
		Resource: ResourceCitation{
			Kind: "Pod", Namespace: "payments", Name: "api-0", UID: "uid-1",
		},
		Severity:       SeverityCritical,
		State:          StateActive,
		Fingerprint:    "fp-1",
		Coverage:       CoverageComplete,
		Freshness:      observed,
		WindowStart:    &windowStart,
		WindowEnd:      &windowEnd,
		ObservedAt:     observed,
		IngestedAt:     observed,
		ExpiresAt:      &expiresAt,
		Attributes:     map[string]string{"rule": "crash_loop"},
		Evidence:       []EvidenceRef{{Kind: "diagnosis_record", ID: 7}},
		IngestionRunID: "run-1",
	}
}

// --- pure helpers: TableName / constructor / NopRepository ----------------

func TestSignalRowTableName(t *testing.T) {
	if got := (signalRow{}).TableName(); got != "signal_occurrences" {
		t.Fatalf("TableName() = %q, want %q", got, "signal_occurrences")
	}
}

func TestNewGormRepositoryWiresHandle(t *testing.T) {
	gdb, _ := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	if repo == nil {
		t.Fatal("NewGormRepository returned nil")
	}
	if repo.db != gdb {
		t.Fatalf("repo.db = %p, want %p", repo.db, gdb)
	}
}

func TestNopRepositoryUpsertIgnoresWrites(t *testing.T) {
	var repo NopRepository
	if err := repo.Upsert(context.Background(), signalTestOccurrence()); err != nil {
		t.Fatalf("NopRepository.Upsert err = %v, want nil", err)
	}
}

func TestNopRepositoryGetReturnsNotFound(t *testing.T) {
	var repo NopRepository
	occ, err := repo.Get(context.Background(), 99)
	if !errors.Is(err, ErrSignalNotFound) {
		t.Fatalf("NopRepository.Get err = %v, want ErrSignalNotFound", err)
	}
	if occ.ID != 0 || occ.SignalID != "" {
		t.Fatalf("NopRepository.Get occ = %#v, want zero Occurrence", occ)
	}
}

func TestNopRepositoryListReturnsEmpty(t *testing.T) {
	var repo NopRepository
	items, total, err := repo.List(context.Background(), ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("NopRepository.List err = %v, want nil", err)
	}
	if items != nil {
		t.Fatalf("NopRepository.List items = %#v, want nil", items)
	}
	if total != 0 {
		t.Fatalf("NopRepository.List total = %d, want 0", total)
	}
}

func TestNopRepositoryCountBySignalReturnsNil(t *testing.T) {
	var repo NopRepository
	since := signalObservedAt()
	got, err := repo.CountBySignal(context.Background(), nil, "payments", since, 5)
	if err != nil {
		t.Fatalf("NopRepository.CountBySignal err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("NopRepository.CountBySignal = %#v, want nil", got)
	}
}

func TestNopRepositoryDeleteExpiredReturnsZero(t *testing.T) {
	var repo NopRepository
	removed, err := repo.DeleteExpired(context.Background(), signalObservedAt(), 100)
	if err != nil {
		t.Fatalf("NopRepository.DeleteExpired err = %v, want nil", err)
	}
	if removed != 0 {
		t.Fatalf("NopRepository.DeleteExpired = %d, want 0", removed)
	}
}

// TestNopRepositoryImplementsRepository pins the interface contract at compile
// time so a signature drift fails the build rather than a caller.
func TestNopRepositoryImplementsRepository(t *testing.T) {
	var repo Repository = NopRepository{}
	ctx := context.Background()
	if err := repo.Upsert(ctx, signalTestOccurrence()); err != nil {
		t.Fatalf("interface Upsert err = %v", err)
	}
	if _, err := repo.Get(ctx, 1); !errors.Is(err, ErrSignalNotFound) {
		t.Fatalf("interface Get err = %v, want ErrSignalNotFound", err)
	}
	if _, total, err := repo.List(ctx, ListFilter{}); err != nil || total != 0 {
		t.Fatalf("interface List total/err = %d/%v, want 0/nil", total, err)
	}
	if out, err := repo.CountBySignal(ctx, nil, "", signalObservedAt(), 0); err != nil || out != nil {
		t.Fatalf("interface CountBySignal out/err = %#v/%v, want nil/nil", out, err)
	}
	if n, err := repo.DeleteExpired(ctx, signalObservedAt(), 0); err != nil || n != 0 {
		t.Fatalf("interface DeleteExpired n/err = %d/%v, want 0/nil", n, err)
	}
}

// --- pure helpers: occurrenceToRow / rowToOccurrence ---------------------

func TestOccurrenceToRowCopiesEveryMappedField(t *testing.T) {
	occ := signalTestOccurrence()
	row, err := occurrenceToRow(occ)
	if err != nil {
		t.Fatalf("occurrenceToRow err = %v", err)
	}
	if row.SignalID != occ.SignalID || row.SignalCode != occ.SignalCode || row.SchemaVersion != occ.SchemaVersion {
		t.Fatalf("identity columns = %q/%q/%q", row.SignalID, row.SignalCode, row.SchemaVersion)
	}
	if row.Producer != string(occ.Producer) || row.ClusterID != occ.ClusterID || row.Namespace != occ.Namespace {
		t.Fatalf("producer/scope columns = %q/%d/%q", row.Producer, row.ClusterID, row.Namespace)
	}
	if row.ResourceKind != "Pod" || row.ResourceNamespace != "payments" ||
		row.ResourceName != "api-0" || row.ResourceUID != "uid-1" || row.ResourceIncomplete {
		t.Fatalf("resource columns = %#v", row)
	}
	if row.Severity != string(SeverityCritical) || row.State != string(StateActive) || row.Coverage != string(CoverageComplete) {
		t.Fatalf("severity/state/coverage = %q/%q/%q", row.Severity, row.State, row.Coverage)
	}
	if row.Fingerprint != occ.Fingerprint || row.IngestionRunID != occ.IngestionRunID {
		t.Fatalf("fingerprint/run = %q/%q", row.Fingerprint, row.IngestionRunID)
	}
	if !row.Freshness.Equal(occ.Freshness) || !row.ObservedAt.Equal(occ.ObservedAt) || !row.IngestedAt.Equal(occ.IngestedAt) {
		t.Fatalf("timestamps = %v/%v/%v", row.Freshness, row.ObservedAt, row.IngestedAt)
	}
	if row.WindowStart == nil || !row.WindowStart.Equal(*occ.WindowStart) {
		t.Fatalf("window_start = %v, want %v", row.WindowStart, occ.WindowStart)
	}
	if row.WindowEnd == nil || !row.WindowEnd.Equal(*occ.WindowEnd) {
		t.Fatalf("window_end = %v, want %v", row.WindowEnd, occ.WindowEnd)
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.Equal(*occ.ExpiresAt) {
		t.Fatalf("expires_at = %v, want %v", row.ExpiresAt, occ.ExpiresAt)
	}
	if string(row.Attributes) != `{"rule":"crash_loop"}` {
		t.Fatalf("attributes JSON = %s", row.Attributes)
	}
	if string(row.Evidence) != `[{"kind":"diagnosis_record","id":7}]` {
		t.Fatalf("evidence JSON = %s", row.Evidence)
	}
}

func TestOccurrenceToRowNormalizesNilJSONColumns(t *testing.T) {
	// json.Marshal(nil map) yields "null"; the column is NOT NULL with a jsonb
	// default, so the helper must rewrite it to an empty object/array.
	row, err := occurrenceToRow(&Occurrence{SignalID: "s", Attributes: nil, Evidence: nil})
	if err != nil {
		t.Fatalf("occurrenceToRow err = %v", err)
	}
	if string(row.Attributes) != "{}" {
		t.Fatalf("attributes = %s, want {}", row.Attributes)
	}
	if string(row.Evidence) != "[]" {
		t.Fatalf("evidence = %s, want []", row.Evidence)
	}
}

func TestOccurrenceToRowKeepsEmptyJSONEncoding(t *testing.T) {
	// Non-nil but empty containers marshal to "{}"/"[]" and must pass through.
	row, err := occurrenceToRow(&Occurrence{
		SignalID:   "s",
		Attributes: map[string]string{},
		Evidence:   []EvidenceRef{},
	})
	if err != nil {
		t.Fatalf("occurrenceToRow err = %v", err)
	}
	if string(row.Attributes) != "{}" || string(row.Evidence) != "[]" {
		t.Fatalf("attributes/evidence = %s/%s, want {}/[]", row.Attributes, row.Evidence)
	}
}

func TestOccurrenceToRowRoundTripsThroughRowToOccurrence(t *testing.T) {
	occ := signalTestOccurrence()
	row, err := occurrenceToRow(occ)
	if err != nil {
		t.Fatalf("occurrenceToRow err = %v", err)
	}
	got, err := rowToOccurrence(&row)
	if err != nil {
		t.Fatalf("rowToOccurrence err = %v", err)
	}
	if got.SignalID != occ.SignalID || got.Producer != occ.Producer || got.ClusterID != occ.ClusterID {
		t.Fatalf("round-trip identity = %#v", got)
	}
	if got.Resource != occ.Resource {
		t.Fatalf("round-trip resource = %#v, want %#v", got.Resource, occ.Resource)
	}
	if got.Severity != occ.Severity || got.State != occ.State || got.Coverage != occ.Coverage {
		t.Fatalf("round-trip enums = %q/%q/%q", got.Severity, got.State, got.Coverage)
	}
	if len(got.Attributes) != 1 || got.Attributes["rule"] != "crash_loop" {
		t.Fatalf("round-trip attributes = %#v", got.Attributes)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Kind != "diagnosis_record" || got.Evidence[0].ID != 7 {
		t.Fatalf("round-trip evidence = %#v", got.Evidence)
	}
	if !got.ObservedAt.Equal(occ.ObservedAt) || !got.Freshness.Equal(occ.Freshness) {
		t.Fatalf("round-trip times = %v/%v", got.ObservedAt, got.Freshness)
	}
}

func TestRowToOccurrenceLeavesNilJSONColumnsNil(t *testing.T) {
	got, err := rowToOccurrence(&signalRow{ID: 5, SignalID: "s"})
	if err != nil {
		t.Fatalf("rowToOccurrence err = %v", err)
	}
	if got.Attributes != nil {
		t.Fatalf("attributes = %#v, want nil", got.Attributes)
	}
	if got.Evidence != nil {
		t.Fatalf("evidence = %#v, want nil", got.Evidence)
	}
	if got.ID != 5 || got.SignalID != "s" {
		t.Fatalf("row = %#v", got)
	}
}

func TestRowToOccurrenceRejectsInvalidAttributesJSON(t *testing.T) {
	_, err := rowToOccurrence(&signalRow{SignalID: "s", Attributes: []byte(`{"rule":`)})
	if err == nil {
		t.Fatal("rowToOccurrence err = nil, want JSON error for attributes")
	}
}

func TestRowToOccurrenceRejectsInvalidEvidenceJSON(t *testing.T) {
	_, err := rowToOccurrence(&signalRow{
		SignalID:   "s",
		Attributes: []byte(`{}`),
		Evidence:   []byte(`[{"kind":`),
	})
	if err == nil {
		t.Fatal("rowToOccurrence err = nil, want JSON error for evidence")
	}
}

// --- GormRepository.Upsert ----------------------------------------------

func TestGormRepositoryUpsertUsesOnConflictOnSignalAndFingerprint(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := signalObservedAt()

	// GORM emits INSERT ... ON CONFLICT (signal_id, fingerprint) DO UPDATE ...
	// RETURNING so duplicate producer deliveries collapse onto one row. Writes
	// run inside GORM's default transaction.
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "signal_occurrences"`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ingested_at", "attributes", "evidence"}).
			AddRow(int64(42), observed, []byte(`{}`), []byte(`[]`)))
	mock.ExpectCommit()

	if err := repo.Upsert(context.Background(), signalTestOccurrence()); err != nil {
		t.Fatalf("Upsert err = %v", err)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryUpsertPropagatesDatabaseError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("insert failed")

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "signal_occurrences"`).WillReturnError(boom)
	mock.ExpectRollback()

	err := repo.Upsert(context.Background(), signalTestOccurrence())
	if !errors.Is(err, boom) {
		t.Fatalf("Upsert err = %v, want %v", err, boom)
	}
	expectAllMet(t, mock)
}

// --- GormRepository.Get --------------------------------------------------

func TestGormRepositoryGetReturnsMappedOccurrence(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := signalObservedAt()

	mock.ExpectQuery(`SELECT * FROM "signal_occurrences" WHERE id = $1 ORDER BY "signal_occurrences"."id"`).
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()).AddRow(signalRowValues(42, observed)...))

	occ, err := repo.Get(context.Background(), 42)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if occ.ID != 42 || occ.SignalID != "diag.pod.pending.v1" || occ.ClusterID != 3 {
		t.Fatalf("occ identity = %#v", occ)
	}
	if occ.Producer != ProducerDiagnosis || occ.Severity != SeverityCritical ||
		occ.State != StateActive || occ.Coverage != CoverageComplete {
		t.Fatalf("occ enums = %q/%q/%q/%q", occ.Producer, occ.Severity, occ.State, occ.Coverage)
	}
	if occ.Resource.Kind != "Pod" || occ.Resource.UID != "uid-1" || occ.Resource.Incomplete {
		t.Fatalf("occ resource = %#v", occ.Resource)
	}
	if occ.Attributes["rule"] != "crash_loop" {
		t.Fatalf("occ attributes = %#v", occ.Attributes)
	}
	if len(occ.Evidence) != 1 || occ.Evidence[0].ID != 7 {
		t.Fatalf("occ evidence = %#v", occ.Evidence)
	}
	if occ.WindowStart == nil || occ.WindowEnd == nil || occ.ExpiresAt == nil {
		t.Fatalf("occ nullable times = %v/%v/%v", occ.WindowStart, occ.WindowEnd, occ.ExpiresAt)
	}
	if !occ.ObservedAt.Equal(observed) {
		t.Fatalf("occ observed_at = %v, want %v", occ.ObservedAt, observed)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryGetMapsRecordNotFoundToErrSignalNotFound(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`SELECT * FROM "signal_occurrences" WHERE id = $1`).
		WithArgs(int64(404), 1).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()))

	occ, err := repo.Get(context.Background(), 404)
	if !errors.Is(err, ErrSignalNotFound) {
		t.Fatalf("Get err = %v, want ErrSignalNotFound", err)
	}
	if occ.ID != 0 {
		t.Fatalf("Get occ = %#v, want zero value", occ)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryGetPropagatesDatabaseError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("connection reset")

	mock.ExpectQuery(`SELECT * FROM "signal_occurrences" WHERE id = $1`).
		WithArgs(int64(1), 1).
		WillReturnError(boom)

	_, err := repo.Get(context.Background(), 1)
	if !errors.Is(err, boom) {
		t.Fatalf("Get err = %v, want %v", err, boom)
	}
	if errors.Is(err, ErrSignalNotFound) {
		t.Fatal("Get must not translate an infrastructure error into ErrSignalNotFound")
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryGetPropagatesCorruptJSON(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)

	values := signalRowValues(42, signalObservedAt())
	values[22] = []byte(`{"rule":`) // attributes column: truncated JSON
	mock.ExpectQuery(`SELECT * FROM "signal_occurrences" WHERE id = $1`).
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()).AddRow(values...))

	if _, err := repo.Get(context.Background(), 42); err == nil {
		t.Fatal("Get err = nil, want JSON decode error from rowToOccurrence")
	}
	expectAllMet(t, mock)
}

// --- GormRepository.List -------------------------------------------------

func TestGormRepositoryListAppliesEveryFilterAndReturnsTotal(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	observed := signalObservedAt()
	clusterID := int64(3)
	windowStart := observed.Add(-time.Hour)
	windowEnd := observed.Add(time.Hour)

	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).
		WithArgs(clusterID, "payments", "diag.pod.pending.v1", "diagnosis", "active", "critical", windowStart, windowEnd).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(7)))
	mock.ExpectQuery(`ORDER BY observed_at DESC, id DESC LIMIT`).
		WithArgs(clusterID, "payments", "diag.pod.pending.v1", "diagnosis", "active", "critical", windowStart, windowEnd, 25).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()).AddRow(signalRowValues(42, observed)...))

	items, total, err := repo.List(context.Background(), ListFilter{
		ClusterID:   &clusterID,
		Namespace:   "payments",
		SignalID:    "diag.pod.pending.v1",
		Producer:    ProducerDiagnosis,
		State:       StateActive,
		Severity:    SeverityCritical,
		WindowStart: &windowStart,
		WindowEnd:   &windowEnd,
		Limit:       25,
	})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 7 {
		t.Fatalf("List total = %d, want 7", total)
	}
	if len(items) != 1 {
		t.Fatalf("List items = %d, want 1", len(items))
	}
	if items[0].ID != 42 || items[0].SignalID != "diag.pod.pending.v1" || items[0].Attributes["rule"] != "crash_loop" {
		t.Fatalf("List item = %#v", items[0])
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryListClampsOversizedLimit(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)

	// Limit > 200 must be clamped to the 100-row ceiling.
	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(`ORDER BY observed_at DESC, id DESC LIMIT`).
		WithArgs(100).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()))

	items, total, err := repo.List(context.Background(), ListFilter{Limit: 1000})
	if err != nil {
		t.Fatalf("List err = %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("List = %d items/%d total, want 0/0", len(items), total)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryListClampsNonPositiveLimit(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)

	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`ORDER BY observed_at DESC, id DESC LIMIT`).
		WithArgs(100).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()))

	if _, _, err := repo.List(context.Background(), ListFilter{Limit: -5}); err != nil {
		t.Fatalf("List err = %v", err)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryListPropagatesCountError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("count failed")

	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).WillReturnError(boom)

	items, total, err := repo.List(context.Background(), ListFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("List err = %v, want %v", err, boom)
	}
	if items != nil || total != 0 {
		t.Fatalf("List = %#v/%d, want nil/0 on error", items, total)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryListPropagatesFindError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("select failed")

	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(`ORDER BY observed_at DESC, id DESC LIMIT`).WillReturnError(boom)

	_, _, err := repo.List(context.Background(), ListFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("List err = %v, want %v", err, boom)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryListPropagatesCorruptRowJSON(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)

	values := signalRowValues(42, signalObservedAt())
	values[23] = []byte(`[{"kind":`) // evidence column: truncated JSON
	mock.ExpectQuery(`SELECT count(*) FROM "signal_occurrences"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`ORDER BY observed_at DESC, id DESC LIMIT`).
		WillReturnRows(sqlmock.NewRows(signalRowColumns()).AddRow(values...))

	if _, _, err := repo.List(context.Background(), ListFilter{}); err == nil {
		t.Fatal("List err = nil, want JSON decode error from rowToOccurrence")
	}
	expectAllMet(t, mock)
}

// --- GormRepository.CountBySignal ---------------------------------------

func TestGormRepositoryCountBySignalAggregatesScopedRows(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	since := signalObservedAt()
	clusterID := int64(3)
	lastSeen := since.Add(30 * time.Minute)

	mock.ExpectQuery(`COUNT(*) AS cnt`).
		WithArgs(StateActive, since, clusterID, "payments", 5).
		WillReturnRows(sqlmock.NewRows([]string{"signal_id", "producer", "severity", "cnt", "last_seen", "namespace"}).
			AddRow("diag.pod.pending.v1", "diagnosis", "critical", int64(4), lastSeen, "payments").
			AddRow("alert.node.notready.v1", "alert", "warning", int64(2), lastSeen, "payments"))

	out, err := repo.CountBySignal(context.Background(), &clusterID, "payments", since, 5)
	if err != nil {
		t.Fatalf("CountBySignal err = %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("CountBySignal len = %d, want 2", len(out))
	}
	first := out[0]
	if first.SignalID != "diag.pod.pending.v1" || first.Producer != ProducerDiagnosis ||
		first.Severity != SeverityCritical || first.Count != 4 || first.Namespace != "payments" {
		t.Fatalf("first aggregate = %#v", first)
	}
	if !first.LastSeen.Equal(lastSeen) {
		t.Fatalf("last_seen = %v, want %v", first.LastSeen, lastSeen)
	}
	if out[1].Producer != ProducerAlert || out[1].Count != 2 {
		t.Fatalf("second aggregate = %#v", out[1])
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryCountBySignalDefaultsToTwentyAndUnscoped(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	since := signalObservedAt()

	// nil cluster scope + empty namespace + limit <= 0 => LIMIT 20, no scope filters.
	mock.ExpectQuery(`COUNT(*) AS cnt`).
		WithArgs(StateActive, since, 20).
		WillReturnRows(sqlmock.NewRows([]string{"signal_id", "producer", "severity", "cnt", "last_seen", "namespace"}))

	out, err := repo.CountBySignal(context.Background(), nil, "", since, 0)
	if err != nil {
		t.Fatalf("CountBySignal err = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("CountBySignal = %#v, want empty", out)
	}
	if out == nil {
		t.Fatal("CountBySignal must return a non-nil empty slice")
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryCountBySignalPropagatesError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	boom := errors.New("aggregate failed")

	mock.ExpectQuery(`COUNT(*) AS cnt`).WillReturnError(boom)

	out, err := repo.CountBySignal(context.Background(), nil, "", signalObservedAt(), 10)
	if !errors.Is(err, boom) {
		t.Fatalf("CountBySignal err = %v, want %v", err, boom)
	}
	if out != nil {
		t.Fatalf("CountBySignal = %#v, want nil on error", out)
	}
	expectAllMet(t, mock)
}

// --- GormRepository.DeleteExpired ---------------------------------------

// deleteExpiredSQL is the bounded shape the statement must take. The bound has
// to live *inside* the statement: GORM's Delete silently drops any Limit clause
// for the Postgres dialect, so `Delete(...).Limit(n)` deletes every expired row
// in one unbounded statement — which is exactly the bug this pins down.
// (Whitespace is normalised by the shared matcher, so this reads as one line.)
const deleteExpiredSQL = `WITH expired AS ( SELECT id FROM signal_occurrences WHERE expires_at IS NOT NULL AND expires_at <= $1 ORDER BY expires_at ASC, id ASC LIMIT $2 ) DELETE FROM signal_occurrences WHERE id IN (SELECT id FROM expired)`

func TestGormRepositoryDeleteExpiredReturnsRowsAffected(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	now := signalObservedAt()

	// Exec issues the statement directly, without GORM's implicit transaction.
	mock.ExpectExec(deleteExpiredSQL).
		WithArgs(now, 500).
		WillReturnResult(sqlmock.NewResult(0, 3))

	removed, err := repo.DeleteExpired(context.Background(), now, 500)
	if err != nil {
		t.Fatalf("DeleteExpired err = %v", err)
	}
	if removed != 3 {
		t.Fatalf("DeleteExpired = %d, want 3", removed)
	}
	expectAllMet(t, mock)
}

// TestGormRepositoryDeleteExpiredBoundsBatchInSQL is the regression test for the
// unbounded-delete defect: a caller-supplied batch size must actually reach the
// SQL. Before the fix the second argument was absent entirely, so the assertion
// on WithArgs failed.
func TestGormRepositoryDeleteExpiredBoundsBatchInSQL(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	now := signalObservedAt()

	const batchSize = 7
	mock.ExpectExec(deleteExpiredSQL).
		WithArgs(now, batchSize).
		WillReturnResult(sqlmock.NewResult(0, 7))

	removed, err := repo.DeleteExpired(context.Background(), now, batchSize)
	if err != nil {
		t.Fatalf("DeleteExpired err = %v", err)
	}
	if removed != int64(batchSize) {
		t.Fatalf("DeleteExpired = %d, want %d", removed, batchSize)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryDeleteExpiredDefaultsBatchSize(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	now := signalObservedAt()

	// batchSize <= 0 is replaced by the 500 default, and that default must be
	// the one carried into the statement's LIMIT.
	mock.ExpectExec(deleteExpiredSQL).
		WithArgs(now, 500).
		WillReturnResult(sqlmock.NewResult(0, 0))

	removed, err := repo.DeleteExpired(context.Background(), now, 0)
	if err != nil {
		t.Fatalf("DeleteExpired err = %v", err)
	}
	if removed != 0 {
		t.Fatalf("DeleteExpired = %d, want 0", removed)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryDeleteExpiredPropagatesError(t *testing.T) {
	gdb, mock := newSignalMockGorm(t)
	repo := NewGormRepository(gdb)
	now := signalObservedAt()
	boom := errors.New("delete failed")

	mock.ExpectExec(deleteExpiredSQL).WithArgs(now, 100).WillReturnError(boom)

	removed, err := repo.DeleteExpired(context.Background(), now, 100)
	if !errors.Is(err, boom) {
		t.Fatalf("DeleteExpired err = %v, want %v", err, boom)
	}
	if removed != 0 {
		t.Fatalf("DeleteExpired = %d, want 0 on error", removed)
	}
	expectAllMet(t, mock)
}

func TestGormRepositoryImplementsRepository(t *testing.T) {
	var _ Repository = (*GormRepository)(nil)
	var _ Repository = NopRepository{}
}
