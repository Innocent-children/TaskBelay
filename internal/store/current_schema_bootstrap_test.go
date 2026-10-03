package store

import (
	"context"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func TestFreshCurrentSchemaBootstrapIsDirectAndExact(t *testing.T) {
	path := dbPath(t)
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	db := openRaw(t, path)
	defer db.Close()
	if err := verifyCurrentSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := db.QueryRow(`SELECT version FROM schema_metadata`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != DatabaseSchemaVersion {
		t.Fatalf("database version=%q", version)
	}
	if version != "0.8.0" {
		t.Fatalf("database schema identity=%q", version)
	}
	var claimIndexes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='repository_claims_task_idx'`).Scan(&claimIndexes); err != nil || claimIndexes != 1 {
		t.Fatalf("claim task index count=%d err=%v", claimIndexes, err)
	}
}

func TestRelocationSchemaRetainsHistoryAndLimitsPendingOperation(t *testing.T) {
	path := dbPath(t)
	opened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	task := testGraphTask(t)
	if err := opened.CommitTask(context.Background(), testMutation(t, task)); err != nil {
		t.Fatal(err)
	}
	preparedAt := formatTime(task.CreatedAt)
	for _, operation := range []struct {
		relocationID domain.ID
		requestID    domain.ID
		revision     uint64
	}{
		{"relocation-one", "relocation-request-one", 1},
		{"relocation-two", "relocation-request-two", 1},
	} {
		_, err := opened.db.Exec(
			`INSERT INTO relocation_operations(relocation_id,task_id,request_id,source_binding_digest,prepared_at,resolved_revision) VALUES(?,?,?,?,?,?)`,
			operation.relocationID,
			task.TaskID,
			operation.requestID,
			task.Repository.BindingDigest,
			preparedAt,
			operation.revision,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer opened.Close()
	if _, err := opened.db.Exec(
		`INSERT INTO relocation_operations(relocation_id,task_id,request_id,source_binding_digest,prepared_at,resolved_revision) VALUES(?,?,?,?,?,NULL)`,
		"relocation-pending-one",
		task.TaskID,
		"relocation-pending-request-one",
		task.Repository.BindingDigest,
		preparedAt,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.db.Exec(
		`INSERT INTO relocation_operations(relocation_id,task_id,request_id,source_binding_digest,prepared_at,resolved_revision) VALUES(?,?,?,?,?,NULL)`,
		"relocation-pending-two",
		task.TaskID,
		"relocation-pending-request-two",
		task.Repository.BindingDigest,
		preparedAt,
	); err == nil {
		t.Fatal("second unresolved relocation accepted for one Task")
	}
}

func TestBootstrapFailureLeavesNoPartialSchema(t *testing.T) {
	path := dbPath(t)
	db := openRaw(t, path)
	original := currentSchemaStatements
	currentSchemaStatements = append(append([]string(nil), original...), `CREATE TABLE tasks (`)
	defer func() { currentSchemaStatements = original }()
	if err := bootstrapCurrentSchema(context.Background(), db); err == nil {
		t.Fatal("invalid bootstrap succeeded")
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial tables=%d", count)
	}
}
func TestPartialSchemaIsRejectedWithoutCompletion(t *testing.T) {
	path := dbPath(t)
	db := openRaw(t, path)
	if _, err := db.Exec(`CREATE TABLE unexpected_schema(marker TEXT PRIMARY KEY);INSERT INTO unexpected_schema VALUES('unsupported')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before := databaseManifest(t, path)
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("partial schema accepted")
	}
	assertDatabaseManifestUnchanged(t, path, before)
}
