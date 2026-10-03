package store

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"reflect"
	"strings"
	"testing"
)

func TestCurrentSchemaUpgradePreservesNonemptyTaskAndPendingOperation(t *testing.T) {
	ctx := context.Background()
	path := dbPath(t)
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CommitTask(ctx, testMutation(t, testGraphTask(t))); err != nil {
		t.Fatal(err)
	}
	task, err := database.LoadTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	commit := storeTestActionCommit(t, task)
	if err := database.StageActionOperation(ctx, task, commit); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`DROP TABLE branch_rename_operations; UPDATE schema_metadata SET version='0.7.0'`); err != nil {
		t.Fatal(err)
	}
	database.Close()
	before := databaseManifest(t, path)
	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	read, err := reader.LoadTask(ctx, task.TaskID)
	if err != nil || !reflect.DeepEqual(read, task) {
		t.Fatalf("read-only prior Task: %v", err)
	}
	reader.Close()
	assertDatabaseManifestUnchanged(t, path, before)
	database, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	loaded, err := database.LoadTask(ctx, task.TaskID)
	if err != nil || !reflect.DeepEqual(loaded, task) {
		t.Fatalf("upgrade rewrote Task: %v", err)
	}
	op, found, err := database.LoadActionOperation(ctx, task.TaskID)
	if err != nil || !found || op.AppliedRevision != nil || !op.Commit.Equal(commit) {
		t.Fatalf("upgrade lost pending operation: %v", err)
	}
	if err := database.StageActionOperation(ctx, task, commit); err != nil {
		t.Fatalf("upgraded pending operation cannot be continued: %v", err)
	}
	if err := verifyCurrentSchema(ctx, database.db); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentSchemaUpgradeFailureRollsBackSchemaAndRecords(t *testing.T) {
	ctx := context.Background()
	path := dbPath(t)
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CommitTask(ctx, testMutation(t, testGraphTask(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := database.db.Exec(`DROP TABLE branch_rename_operations; UPDATE schema_metadata SET version='0.7.0'`); err != nil {
		t.Fatal(err)
	}
	database.Close()
	before := databaseManifest(t, path)
	raw := openRaw(t, path)
	original := currentSchemaStatements
	currentSchemaStatements = append(append([]string(nil), original...), `CREATE TABLE broken (`)
	err = bootstrapCurrentSchema(ctx, raw)
	currentSchemaStatements = original
	raw.Close()
	if err == nil {
		t.Fatal("injected upgrade failure succeeded")
	}
	assertDatabaseManifestUnchanged(t, path, before)
}

func TestBaselineHistoryLargeSnapshotRestartAndAppendOnlyTransaction(t *testing.T) {
	ctx := context.Background()
	path := dbPath(t)
	database, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	task := testGraphTask(t)
	for i := 1; i <= 600; i++ {
		task.BaselineHistory = append(task.BaselineHistory, domain.BaselineReference{Kind: domain.BaselineRequirements, Revision: uint32(i), Digest: task.Process.DefinitionDigest, Summary: strings.Repeat("r", domain.MaxEvidenceSummaryBytes), CreatedAt: task.CreatedAt})
	}
	initial := testMutation(t, task)
	task = initial.Task
	encoded, err := encodeTask(task)
	if err != nil || len(encoded) <= domain.MaxPersistedTaskSnapshotBytes {
		t.Fatalf("large fixture bytes=%d err=%v", len(encoded), err)
	}
	if err := database.CommitTask(ctx, initial); err != nil {
		t.Fatal(err)
	}
	database.Close()
	database, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	saved, err := database.LoadTask(ctx, task.TaskID)
	if err != nil || !reflect.DeepEqual(saved, task) {
		t.Fatalf("restart lost full history: %v", err)
	}
	commit := storeTestActionCommit(t, saved)
	if err := database.StageActionOperation(ctx, saved, commit); err != nil {
		t.Fatalf("large stage rejected: %v", err)
	}
	// These remain valid standalone chains, but cannot replace existing saved references.
	for _, damage := range []string{"rewrite", "truncate", "reorder"} {
		next := saved
		next.BaselineHistory = append([]domain.BaselineReference(nil), saved.BaselineHistory...)
		switch damage {
		case "rewrite":
			next.BaselineHistory[0].Summary = "Changed"
		case "truncate":
			next.BaselineHistory = next.BaselineHistory[:599]
		case "reorder":
			next.BaselineHistory[0], next.BaselineHistory[1] = next.BaselineHistory[1], next.BaselineHistory[0]
		}
		tx, err := database.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = validateBaselineHistoryMutation(ctx, tx, TaskMutation{ExpectedRevision: saved.Revision, Task: next})
		tx.Rollback()
		if err == nil {
			t.Fatalf("%s accepted", damage)
		}
	}
	loaded, err := database.LoadTask(ctx, task.TaskID)
	if err != nil || !reflect.DeepEqual(loaded, saved) {
		t.Fatal("failed mutation changed history")
	}
}
