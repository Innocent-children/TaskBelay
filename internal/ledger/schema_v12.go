package ledger

import (
	"context"
	"database/sql"
)

func migrateV12(ctx context.Context, tx *sql.Tx) error {
	// 一次切换全部业务表，使已启动进程的 SQL 与新格式隔离。
	for _, table := range append(append([]string(nil), businessTables...), "file_sets") {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+"_v11 RENAME TO "+table+"_v12"); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `
CREATE TABLE analysis_cache_v12 (
 kind TEXT NOT NULL, cache_key TEXT NOT NULL, encoding TEXT NOT NULL,
 raw_bytes INTEGER NOT NULL, sha256 TEXT NOT NULL, body BLOB NOT NULL,
 PRIMARY KEY(kind,cache_key)
);
CREATE TABLE analysis_records_v12 (
 task_id TEXT NOT NULL REFERENCES tasks_v12(id) ON DELETE CASCADE,
 input_version TEXT NOT NULL, kind TEXT NOT NULL, state TEXT NOT NULL, data TEXT NOT NULL,
 PRIMARY KEY(task_id,input_version,kind)
);
CREATE TABLE feedback_labels_v12 (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, issue_id TEXT NOT NULL,
 data TEXT NOT NULL
);
CREATE INDEX feedback_project_v12 ON feedback_labels_v12(project_id,id);
CREATE TABLE issue_feedback_v12 (
 task_id TEXT NOT NULL REFERENCES tasks_v12(id) ON DELETE CASCADE,
 issue_id TEXT NOT NULL, data TEXT NOT NULL,
 PRIMARY KEY(task_id,issue_id)
);
CREATE TABLE analysis_weights_v12 (
 project_id TEXT PRIMARY KEY, data TEXT NOT NULL
);
CREATE TABLE dynamic_jobs_v12 (
 task_id TEXT PRIMARY KEY REFERENCES tasks_v12(id) ON DELETE CASCADE,
 input_version TEXT NOT NULL, owner TEXT NOT NULL, expires_at TEXT NOT NULL, state TEXT NOT NULL
);
CREATE INDEX verification_cross_session_v12 ON verifications_v12(workspace_id,command_key,seq);
PRAGMA user_version=12;
`)
	return err
}
