package store

import (
	"context"
	"database/sql"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedSchemaIsReadOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	db.Exec("CREATE TABLE old_task(cursor TEXT)")
	db.Close()
	before, _ := os.ReadFile(p)
	_, e = Open(context.Background(), p)
	failure, ok := e.(*domain.Error)
	if !ok || failure.Code != "SCHEMA_UNSUPPORTED" {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("old user database modified")
	}
	if _, e = os.Stat(p + "-wal"); !os.IsNotExist(e) {
		t.Fatal("write sidecar created for unsupported schema")
	}
}
func TestAlteredCurrentSchemaIsRejectedBeforeWritableOpen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "current.db")
	s, e := Open(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	db, _ := sql.Open("sqlite", p)
	db.Exec("CREATE TABLE unexpected(value TEXT)")
	db.Close()
	before, _ := os.ReadFile(p)
	_, e = Open(context.Background(), p)
	if e == nil {
		t.Fatal("changed layout accepted")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("unsupported layout modified")
	}
}
