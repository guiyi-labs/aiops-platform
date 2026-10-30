package migrations

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// embeddedMigrationCount 返回嵌入的 .up.sql 文件数。
//
// 测试里不能硬编码数量：之前的 `for range 50` 在新增第 51、52 个迁移后就一直失败，
// 而失败原因（sqlmock 期望已耗尽）与「迁移逻辑坏了」毫无关系，很容易被误判。
func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			count++
		}
	}
	return count
}

func TestApply_AlreadyApplied(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS schema_migrations`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	// 全部已应用的迁移各查一次；数量从嵌入 FS 取，新增迁移不会让测试失败。
	for range embeddedMigrationCount(t) {
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
	// 其余迁移都已应用；数量同样从嵌入 FS 取（原来写死 49）。
	for range embeddedMigrationCount(t) - 1 {
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
