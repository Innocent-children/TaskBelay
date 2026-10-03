package store

import (
	"context"
	"database/sql"
	"strings"
)

const DatabaseSchemaVersion = "0.8.0"

// Only the immediately deployed layout is readable before this bounded upgrade.
const priorDatabaseSchemaVersion = "0.7.0"
const priorSchemaStatementCount = 12

var currentSchemaStatements = []string{
	`CREATE TABLE schema_metadata (version TEXT PRIMARY KEY)`,
	`CREATE TABLE tasks (task_id TEXT PRIMARY KEY, origin_host TEXT NOT NULL, process_id TEXT NOT NULL, process_definition_digest TEXT NOT NULL, current_node TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision >= 1), worktree_instance_digest TEXT NOT NULL, snapshot BLOB NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, archived_at TEXT)`,
	`CREATE INDEX tasks_node_idx ON tasks (current_node)`,
	`CREATE INDEX tasks_origin_host_idx ON tasks (origin_host)`,
	`CREATE INDEX tasks_updated_at_idx ON tasks (updated_at)`,
	`CREATE TABLE action_operations (task_id TEXT PRIMARY KEY, operation_id TEXT NOT NULL UNIQUE, process_id TEXT NOT NULL, process_definition_digest TEXT NOT NULL, source_node TEXT NOT NULL, expected_revision INTEGER NOT NULL CHECK (expected_revision >= 1), action_id TEXT NOT NULL, action_kind TEXT NOT NULL, repository_binding_digest TEXT NOT NULL, issuance_identity_digest TEXT NOT NULL, issuance_history_digest TEXT NOT NULL, issuance_content_digest TEXT NOT NULL, payload BLOB NOT NULL, payload_digest TEXT NOT NULL, prepared_at TEXT NOT NULL, applied_revision INTEGER CHECK (applied_revision IS NULL OR applied_revision = expected_revision + 1), FOREIGN KEY (task_id) REFERENCES tasks (task_id) ON DELETE RESTRICT)`,
	`CREATE TABLE task_events (event_id TEXT PRIMARY KEY, task_id TEXT NOT NULL, revision INTEGER NOT NULL CHECK (revision >= 1), event_type TEXT NOT NULL, source_node TEXT NOT NULL, destination_node TEXT NOT NULL, transition_id TEXT, transition_reason TEXT, action_id TEXT, observed_binding_digest TEXT, repository_delta_paths BLOB NOT NULL, request_id TEXT NOT NULL, payload_digest TEXT NOT NULL, created_at TEXT NOT NULL, UNIQUE (task_id, revision), UNIQUE (task_id, request_id), FOREIGN KEY (task_id) REFERENCES tasks (task_id) ON DELETE RESTRICT)`,
	`CREATE TABLE repository_claims (worktree_instance_digest TEXT PRIMARY KEY, canonical_worktree_root TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL, origin_host TEXT NOT NULL, claimed_at TEXT NOT NULL, FOREIGN KEY (task_id) REFERENCES tasks (task_id) ON DELETE RESTRICT)`,
	`CREATE INDEX repository_claims_task_idx ON repository_claims (task_id)`,
	`CREATE TABLE relocation_operations (relocation_id TEXT PRIMARY KEY, task_id TEXT NOT NULL, request_id TEXT NOT NULL UNIQUE, source_binding_digest TEXT NOT NULL, prepared_at TEXT NOT NULL, resolved_revision INTEGER, FOREIGN KEY (task_id) REFERENCES tasks (task_id) ON DELETE RESTRICT)`,
	`CREATE INDEX relocation_operations_task_idx ON relocation_operations (task_id)`,
	`CREATE UNIQUE INDEX relocation_operations_unresolved_task_idx ON relocation_operations (task_id) WHERE resolved_revision IS NULL`,
	`CREATE TABLE branch_rename_operations (rename_id TEXT PRIMARY KEY, task_id TEXT NOT NULL, request_id TEXT NOT NULL UNIQUE, preparation BLOB NOT NULL, resolved_revision INTEGER, resolution TEXT, FOREIGN KEY (task_id) REFERENCES tasks (task_id) ON DELETE RESTRICT)`,
	`CREATE UNIQUE INDEX branch_rename_operations_pending_idx ON branch_rename_operations (task_id) WHERE resolved_revision IS NULL`,
}

var currentSchemaObjects = []struct {
	name           string
	kind           string
	statementIndex int
}{
	{"schema_metadata", "table", 0},
	{"tasks", "table", 1},
	{"tasks_node_idx", "index", 2},
	{"tasks_origin_host_idx", "index", 3},
	{"tasks_updated_at_idx", "index", 4},
	{"action_operations", "table", 5},
	{"task_events", "table", 6},
	{"repository_claims", "table", 7},
	{"repository_claims_task_idx", "index", 8},
	{"relocation_operations", "table", 9},
	{"relocation_operations_task_idx", "index", 10},
	{"relocation_operations_unresolved_task_idx", "index", 11},
	{"branch_rename_operations", "table", 12},
	{"branch_rename_operations_pending_idx", "index", 13},
}

var currentColumns = map[string][]string{
	"branch_rename_operations": {"rename_id", "task_id", "request_id", "preparation", "resolved_revision", "resolution"},
	"schema_metadata":          {"version"},
	"tasks":                    {"task_id", "origin_host", "process_id", "process_definition_digest", "current_node", "revision", "worktree_instance_digest", "snapshot", "created_at", "updated_at", "archived_at"},
	"action_operations":        {"task_id", "operation_id", "process_id", "process_definition_digest", "source_node", "expected_revision", "action_id", "action_kind", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "payload", "payload_digest", "prepared_at", "applied_revision"},
	"task_events":              {"event_id", "task_id", "revision", "event_type", "source_node", "destination_node", "transition_id", "transition_reason", "action_id", "observed_binding_digest", "repository_delta_paths", "request_id", "payload_digest", "created_at"},
	"repository_claims":        {"worktree_instance_digest", "canonical_worktree_root", "task_id", "origin_host", "claimed_at"},
	"relocation_operations":    {"relocation_id", "task_id", "request_id", "source_binding_digest", "prepared_at", "resolved_revision"},
}

func bootstrapCurrentSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ErrStorageUnavailable
	}
	defer tx.Rollback()
	has, err := hasUserTables(ctx, tx)
	if err != nil {
		return ErrStorageUnavailable
	}
	if has {
		if err := verifyReadableSchema(ctx, tx); err != nil {
			return err
		}
		var version string
		if err := tx.QueryRowContext(ctx, `SELECT version FROM schema_metadata`).Scan(&version); err != nil {
			return ErrSchemaUnsupported
		}
		if version == DatabaseSchemaVersion {
			return nil
		}
		// Validate all nonempty Task/operation/claim/event records before adding
		// storage. The transaction either retains 0.7.0 intact or commits 0.8.0.
		if err := preflightSnapshot(ctx, tx); err != nil {
			return err
		}
		for _, statement := range currentSchemaStatements[priorSchemaStatementCount:] {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return ErrStorageUnavailable
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE schema_metadata SET version=? WHERE version=?`, DatabaseSchemaVersion, priorDatabaseSchemaVersion); err != nil {
			return ErrStorageUnavailable
		}
		if err := verifyCurrentSchema(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	for _, statement := range currentSchemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return ErrStorageUnavailable
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_metadata(version) VALUES (?)`, DatabaseSchemaVersion); err != nil {
		return ErrStorageUnavailable
	}
	if err := verifyCurrentSchema(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func hasUserTables(ctx context.Context, q queryer) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&n)
	return n != 0, err
}

func verifyCurrentSchema(ctx context.Context, q queryer) error {
	return verifySchemaLayout(ctx, q, false)
}

func verifyReadableSchema(ctx context.Context, q queryer) error {
	return verifySchemaLayout(ctx, q, true)
}

func verifySchemaLayout(ctx context.Context, q queryer, allowPrior bool) error {
	var version string
	var versionRows int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(version),'') FROM schema_metadata`).Scan(&versionRows, &version); err != nil || versionRows != 1 {
		return ErrSchemaUnsupported
	}
	prior := allowPrior && version == priorDatabaseSchemaVersion
	if version != DatabaseSchemaVersion && !prior {
		return ErrSchemaUnsupported
	}
	expected := make(map[string]string, len(currentSchemaObjects))
	for _, object := range currentSchemaObjects {
		if prior && object.statementIndex >= priorSchemaStatementCount {
			continue
		}
		if object.statementIndex >= len(currentSchemaStatements) {
			return ErrSchemaUnsupported
		}
		expected[object.kind+"\x00"+object.name] = normalizeSchemaSQL(currentSchemaStatements[object.statementIndex])
	}
	rows, err := q.QueryContext(ctx, `SELECT type,name,sql FROM sqlite_master WHERE type IN ('table','index') AND name NOT LIKE 'sqlite_%' ORDER BY type,name`)
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var kind, name, statement string
		if rows.Scan(&kind, &name, &statement) != nil {
			return ErrSchemaUnsupported
		}
		key := kind + "\x00" + name
		if seen[key] || normalizeSchemaSQL(statement) != expected[key] {
			return ErrSchemaUnsupported
		}
		seen[key] = true
	}
	if rows.Err() != nil || len(seen) != len(expected) {
		return ErrSchemaUnsupported
	}
	for table, expectedNames := range currentColumns {
		if _, included := expected["table\x00"+table]; !included {
			continue
		}
		columnRows, err := q.QueryContext(ctx, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if err != nil {
			return ErrSchemaUnsupported
		}
		var actual []string
		for columnRows.Next() {
			var name string
			if columnRows.Scan(&name) != nil {
				columnRows.Close()
				return ErrSchemaUnsupported
			}
			actual = append(actual, name)
		}
		if columnRows.Err() != nil || columnRows.Close() != nil || strings.Join(actual, "\x00") != strings.Join(expectedNames, "\x00") {
			return ErrSchemaUnsupported
		}
	}
	return nil
}

func normalizeSchemaSQL(statement string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(statement)), " "))
}
