package migrations

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApply_AlreadyApplied(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS schema_migrations`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// All 50 embedded migrations already applied; isApplied returns true each time.
	for range 50 {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`)).
			WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply already-applied: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApply_AppliesOneMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS schema_migrations`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// First migration not yet applied -> applyOne path exercised.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectExec(`.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	// Remaining 49 already applied.
	for range 49 {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`)).
			WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	}

	if err := Apply(ctx, db); err != nil {
		t.Fatalf("Apply with one pending: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIsApplied_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`)).
		WithArgs("000001_init_schema.up.sql").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	got, err := isApplied(context.Background(), db, "000001_init_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("expected true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIsApplied_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`)).
		WithArgs("foo.up.sql").
		WillReturnError(sqlmock.ErrCancelled)
	if _, err := isApplied(context.Background(), db, "foo.up.sql"); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyOne_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin().WillReturnError(sqlmock.ErrCancelled)
	if err := applyOne(context.Background(), db, "x.up.sql", "SELECT 1"); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyOne_CommitError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectExec(`.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO schema_migrations`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(sqlmock.ErrCancelled)
	if err := applyOne(context.Background(), db, "x.up.sql", "SELECT 1"); err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
