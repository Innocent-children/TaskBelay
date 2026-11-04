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
CREATE TABLE analysis_indexes_v12 (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL, tree TEXT NOT NULL,
 variant TEXT NOT NULL, parent_id TEXT NOT NULL, data TEXT NOT NULL,
 UNIQUE(project_id,tree,variant)
);
CREATE INDEX analysis_index_project_v12 ON analysis_indexes_v12(project_id);
ALTER TABLE observations_v12 ADD COLUMN paths_indexed INTEGER NOT NULL DEFAULT 0;
CREATE INDEX observations_full_v12 ON observations_v12(workspace_id,id) WHERE json_extract(data,'$.Full')=1 AND json_extract(data,'$.Complete')=1;
CREATE INDEX observed_files_observation_v12 ON observed_files_v12(workspace_id,observation_id);
CREATE TABLE observation_paths_v12 (
 workspace_id TEXT NOT NULL, path TEXT NOT NULL, observation_id INTEGER NOT NULL,
 blob TEXT NOT NULL, mode TEXT NOT NULL,
 PRIMARY KEY(workspace_id,path,observation_id)
);
INSERT INTO observation_paths_v12(workspace_id,path,observation_id,blob,mode)
SELECT workspace_id,path,observation_id,blob,mode FROM observed_files_v12;
CREATE TABLE analysis_index_rows_v12 (
 index_id TEXT NOT NULL, kind TEXT NOT NULL, key TEXT NOT NULL,
 member TEXT NOT NULL, data TEXT NOT NULL,
 PRIMARY KEY(index_id,kind,key,member)
);
CREATE VIRTUAL TABLE analysis_text_v12 USING fts5(index_id UNINDEXED,path UNINDEXED,body,tokenize='trigram');
CREATE TABLE analysis_text_paths_v12 (
 row_id INTEGER PRIMARY KEY, index_id TEXT NOT NULL, path TEXT NOT NULL,
 UNIQUE(index_id,path)
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
