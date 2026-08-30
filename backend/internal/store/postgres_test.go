package store

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/gorm"
)

func TestOpenPostgres_InvalidURL(t *testing.T) {
	_, err := OpenPostgres("postgres://bad:bad@127.0.0.1:1/bad?sslmode=disable")
	if err == nil {
		t.Fatal("expected error for invalid postgres URL")
	}
}

func TestPostgres_PingAndClose(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	p := &Postgres{db: db, gormDB: &gorm.DB{}}
	mock.ExpectPing()
	if err := p.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	mock.ExpectClose()
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgres_GORMAndSQL(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()
	p := &Postgres{db: db, gormDB: &gorm.DB{}}
	if p.GORM() == nil {
		t.Fatal("GORM() is nil")
	}
	if p.SQL() == nil {
		t.Fatal("SQL() is nil")
	}
}
