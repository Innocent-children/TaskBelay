package ledger

import (
	"context"
	"database/sql"
	"fmt"
)

const schemaVersion = 11

var businessTables = []string{
	"workspaces", "settings", "observations", "observed_files", "file_changes", "edits", "workspace_links",
	"tasks", "messages", "task_sessions", "sources", "requirement_versions", "requirement_locations", "notes", "task_calls", "task_updates",
	"hook_events", "mcp_contexts", "session_summaries", "checks", "finish_requests", "issues", "issue_blocks", "check_feedback",
	"verifications", "verification_rounds", "plans", "cleanup_items",
}

func ensureSchema(ctx context.Context, q queryer) error {
	var version int
	if err := q.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("账本版本已变为 %d，当前程序只支持 %d；请重新启动 Agent session，必要时更新 TaskBelay", version, schemaVersion)
	}
	return nil
}

func (s *Store) CheckSchema(ctx context.Context) error {
	return s.read(ctx, func(context.Context, queryer) error { return nil })
}

func migrateV11(ctx context.Context, tx *sql.Tx) error {
	// 旧进程即使已通过启动检查，也不能再用旧 SQL 读写新格式。
	for _, table := range businessTables {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" RENAME TO "+table+"_v11"); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `
CREATE TABLE file_sets_v11 (
 id TEXT PRIMARY KEY, format INTEGER NOT NULL, encoding TEXT NOT NULL,
 raw_bytes INTEGER NOT NULL, sha256 TEXT NOT NULL, body BLOB NOT NULL,
 UNIQUE(format,sha256)
);
ALTER TABLE observations_v11 ADD COLUMN files_set_id TEXT REFERENCES file_sets_v11(id);
CREATE INDEX observations_file_set ON observations_v11(files_set_id);
ALTER TABLE checks_v11 ADD COLUMN report_encoding TEXT NOT NULL DEFAULT '';
ALTER TABLE checks_v11 ADD COLUMN report_raw_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE checks_v11 ADD COLUMN report_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE checks_v11 ADD COLUMN report_body BLOB;
CREATE INDEX observations_compact ON observations_v11(workspace_id,id) WHERE files_set_id IS NULL AND json_extract(data,'$.Full')=1 AND json_extract(data,'$.Complete')=1 AND json_type(data,'$.Files')='object';
CREATE INDEX checks_compact ON checks_v11(task_id,seq) WHERE report_encoding='';
CREATE INDEX tasks_expiry ON tasks_v11(workspace_id,state,julianday(json_extract(data,'$.ended_at')));
CREATE INDEX observations_expiry ON observations_v11(workspace_id,julianday(json_extract(data,'$.At')));
CREATE INDEX observations_task ON observations_v11(task_id,id);
CREATE INDEX observations_session ON observations_v11(workspace_id,json_extract(data,'$.Agent'),json_extract(data,'$.SessionID'),task_id);
CREATE INDEX observations_snapshot_ref ON observations_v11(json_extract(data,'$.SnapshotRef'));
CREATE INDEX observations_evidence_ref ON observations_v11(json_extract(data,'$.EvidenceRef'));
CREATE INDEX tasks_origin_ref ON tasks_v11(json_extract(data,'$.origin_ref'));
CREATE INDEX tasks_origin_observation ON tasks_v11(json_extract(data,'$.origin_observation_id'));
CREATE INDEX tasks_resume_observation ON tasks_v11(json_extract(data,'$.resume_observation_id'));
CREATE INDEX tasks_last_observation ON tasks_v11(json_extract(data,'$.last_observation_id'));
CREATE INDEX tasks_start_message ON tasks_v11(json_extract(data,'$.start_message_id'));
CREATE INDEX tasks_cancel_source ON tasks_v11(json_extract(data,'$.cancellation.source_id'));
CREATE INDEX files_observation ON observed_files_v11(observation_id);
CREATE INDEX changes_observation ON file_changes_v11(observation_id);
CREATE INDEX changes_before ON file_changes_v11(json_extract(data,'$.BeforeObservation'));
CREATE INDEX changes_expiry ON file_changes_v11(workspace_id,julianday(json_extract(data,'$.At')));
CREATE INDEX edits_before ON edits_v11(before_observation);
CREATE INDEX edits_after ON edits_v11(after_observation);
CREATE INDEX links_task ON workspace_links_v11(task_id);
CREATE INDEX sources_task ON sources_v11(task_id);
CREATE INDEX sources_expiry ON sources_v11(workspace_id,julianday(json_extract(data,'$.at')));
CREATE INDEX sources_confirmation ON sources_v11(json_extract(data,'$.confirmation.source_id'));
CREATE INDEX sources_observation ON sources_v11(json_extract(data,'$.observation_id'));
CREATE INDEX messages_expiry ON messages_v11(workspace_id,julianday(json_extract(data,'$.at')));
CREATE INDEX messages_observation ON messages_v11(observation_id);
CREATE INDEX requirements_citation ON requirement_versions_v11(json_extract(data,'$.citation.source_id'));
CREATE INDEX requirements_author ON requirement_versions_v11(json_extract(data,'$.author.agent'),json_extract(data,'$.author.session_id'));
CREATE INDEX locations_observation ON requirement_locations_v11(observation_id);
CREATE INDEX updates_observation ON task_updates_v11(observation_id);
CREATE INDEX notes_citation ON notes_v11(json_extract(data,'$.citation.source_id'));
CREATE INDEX notes_observation ON notes_v11(json_extract(data,'$.observation_id'));
CREATE INDEX notes_author ON notes_v11(json_extract(data,'$.author.agent'),json_extract(data,'$.author.session_id'));
CREATE INDEX plans_path ON plans_v11(json_extract(data,'$.path'));
CREATE INDEX plans_request ON plans_v11(json_extract(data,'$.request_message_id'));
CREATE INDEX plans_confirmation ON plans_v11(json_extract(data,'$.confirmation.citation.source_id'));
CREATE INDEX plans_confirmation_observation ON plans_v11(json_extract(data,'$.confirmation.message.observation_id'));
CREATE INDEX plans_observation ON plans_v11(json_extract(data,'$.observation_id'));
CREATE INDEX plans_author ON plans_v11(json_extract(data,'$.author.agent'),json_extract(data,'$.author.session_id'));
CREATE INDEX sessions_expiry ON task_sessions_v11(workspace_id,julianday(activity_at));
CREATE INDEX summaries_expiry ON session_summaries_v11(workspace_id,julianday(delivered_at));
CREATE INDEX summaries_observation ON session_summaries_v11(observation_id);
CREATE INDEX hooks_expiry ON hook_events_v11(workspace_id,julianday(at));
CREATE INDEX mcp_expiry ON mcp_contexts_v11(workspace_id,julianday(at));
CREATE INDEX calls_expiry ON task_calls_v11(workspace_id,julianday(at));
CREATE INDEX feedback_expiry ON check_feedback_v11(workspace_id,julianday(at));
CREATE INDEX blocks_feedback ON issue_blocks_v11(feedback_id);
CREATE INDEX finish_task ON finish_requests_v11(task_id);
CREATE INDEX verifications_expiry ON verifications_v11(workspace_id,julianday(json_extract(data,'$.started_at')));
CREATE INDEX verifications_previous_id ON verifications_v11(json_extract(data,'$.previous_id'));
CREATE INDEX verifications_round ON verifications_v11(workspace_id,agent,session_id,round_id);
CREATE INDEX cleanup_expiry ON cleanup_items_v11(workspace_id,julianday(ready_at));
PRAGMA user_version=11;
`)
	return err
}
