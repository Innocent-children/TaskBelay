package journeys

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func TestCurrentStorageBoundaryJourney(t *testing.T) {
	t.Run("fresh_current_format", func(t *testing.T) {
		root := t.TempDir()
		repoPath := filepath.Join(root, "repository")
		origin := initializeDedicatedJourneyWorktree(t, repoPath, "task/storage", "receipt-storage")
		dbPath := filepath.Join(root, "data", "taskbelay.db")
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			t.Fatal(err)
		}
		sqliteStore, err := store.Open(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sqliteStore.Close() })
		assertFreshCurrentSchema(t, dbPath)
		service, err := application.NewService(sqliteStore, repository.NewGitObserver())
		if err != nil {
			t.Fatal(err)
		}
		opened, err := service.OpenTask(context.Background(), application.OpenTaskRequest{RequestID: "fresh-storage-open", Host: domain.HostCodex, RepositoryPath: repoPath, WorkspaceOrigin: &origin, NewTask: &application.NewTaskInput{Request: "Prove the current storage format.", MethodProfile: domain.MethodPlain}})
		if err != nil {
			t.Fatal(err)
		}
		task := opened.Task
		applied, err := service.ApplyAction(context.Background(), journeyApplyRequest(task, "fresh-requirements", journeyPayload(t, task, "requirements_ready", "", requirementsJourneyResult("Fresh schema goal"))))
		if err != nil {
			t.Fatal(err)
		}
		task = applied.Task
		if task.CurrentNode != domain.NodeDesign || task.Revision != 2 {
			t.Fatal("fresh task did not advance to DESIGN")
		}
		var processID, definitionDigest string
		readOnly := openImmutableDatabase(t, dbPath)
		t.Cleanup(func() { _ = readOnly.Close() })
		if err := readOnly.QueryRow(`SELECT process_id,process_definition_digest FROM tasks WHERE task_id=?`, task.TaskID).Scan(&processID, &definitionDigest); err != nil {
			t.Fatal(err)
		}
		readOnly.Close()
		if processID != "standard-development" || definitionDigest != string(task.Process.DefinitionDigest) {
			t.Fatal("fresh task metadata mismatch")
		}
		events := databaseCount(t, dbPath, `SELECT COUNT(*) FROM task_events WHERE task_id=?`, task.TaskID)
		claims := databaseCount(t, dbPath, `SELECT COUNT(*) FROM repository_claims WHERE task_id=?`, task.TaskID)
		if err := sqliteStore.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := store.Open(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		loaded, err := reopened.LoadTask(context.Background(), task.TaskID)
		if err != nil || !reflect.DeepEqual(loaded, task) || databaseCount(t, dbPath, `SELECT COUNT(*) FROM task_events WHERE task_id=?`, task.TaskID) != events || databaseCount(t, dbPath, `SELECT COUNT(*) FROM repository_claims WHERE task_id=?`, task.TaskID) != claims {
			t.Fatalf("reopen err=%v loaded=%+v", err, loaded)
		}
	})
}

func assertFreshCurrentSchema(t *testing.T, dbPath string) {
	t.Helper()
	db := openImmutableDatabase(t, dbPath)
	defer db.Close()
	want := []string{"index:branch_rename_operations_pending_idx", "index:relocation_operations_task_idx", "index:relocation_operations_unresolved_task_idx", "index:repository_claims_task_idx", "index:tasks_node_idx", "index:tasks_origin_host_idx", "index:tasks_updated_at_idx", "table:action_operations", "table:branch_rename_operations", "table:relocation_operations", "table:repository_claims", "table:schema_metadata", "table:task_events", "table:tasks"}
	rows, err := db.Query(`SELECT type||':'||name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var value string
		if rows.Scan(&value) != nil {
			t.Fatal("schema scan")
		}
		got = append(got, value)
	}
	rows.Close()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema=%v", got)
	}
	for table, columns := range map[string][]string{"schema_metadata": {"version"}, "tasks": {"task_id", "origin_host", "process_id", "process_definition_digest", "current_node", "revision", "worktree_instance_digest", "snapshot", "created_at", "updated_at", "archived_at"}, "action_operations": {"task_id", "operation_id", "process_id", "process_definition_digest", "source_node", "expected_revision", "action_id", "action_kind", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "payload", "payload_digest", "prepared_at", "applied_revision"}, "branch_rename_operations": {"rename_id", "task_id", "request_id", "preparation", "resolved_revision", "resolution"}, "task_events": {"event_id", "task_id", "revision", "event_type", "source_node", "destination_node", "transition_id", "transition_reason", "action_id", "observed_binding_digest", "repository_delta_paths", "request_id", "payload_digest", "created_at"}, "repository_claims": {"worktree_instance_digest", "canonical_worktree_root", "task_id", "origin_host", "claimed_at"}, "relocation_operations": {"relocation_id", "task_id", "request_id", "source_binding_digest", "prepared_at", "resolved_revision"}} {
		columnRows, err := db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if err != nil {
			t.Fatal(err)
		}
		var actual []string
		for columnRows.Next() {
			var value string
			if columnRows.Scan(&value) != nil {
				t.Fatal("column scan")
			}
			actual = append(actual, value)
		}
		columnRows.Close()
		if !reflect.DeepEqual(actual, columns) {
			t.Fatalf("%s columns=%v", table, actual)
		}
	}
	var schemaVersion string
	if err := db.QueryRow(`SELECT version FROM schema_metadata`).Scan(&schemaVersion); err != nil || schemaVersion != store.DatabaseSchemaVersion {
		t.Fatalf("schema version=%q err=%v", schemaVersion, err)
	}
	var alterCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE UPPER(COALESCE(sql,'')) LIKE '%ALTER TABLE%'`).Scan(&alterCount); err != nil || alterCount != 0 {
		t.Fatalf("ALTER TABLE count=%d err=%v", alterCount, err)
	}
}

func openImmutableDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Clean(path)+"?mode=ro&immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}
