package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const retentionSchema = `
ALTER TABLE task_calls ADD COLUMN at TEXT NOT NULL DEFAULT '';
UPDATE task_calls SET at=COALESCE((SELECT activity_at FROM task_sessions s
 WHERE s.workspace_id=task_calls.workspace_id AND s.agent=task_calls.agent AND s.session_id=task_calls.session_id),strftime('%Y-%m-%dT%H:%M:%fZ','now'));
ALTER TABLE check_feedback ADD COLUMN at TEXT NOT NULL DEFAULT '';
CREATE TABLE cleanup_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 kind TEXT NOT NULL, target TEXT NOT NULL, expected TEXT NOT NULL,
 ready_at TEXT NOT NULL, error TEXT NOT NULL DEFAULT '',
 UNIQUE(workspace_id,kind,target,expected)
);
CREATE INDEX cleanup_ready ON cleanup_items(workspace_id,ready_at,id);
CREATE INDEX hooks_retention ON hook_events(workspace_id,at);
PRAGMA user_version=10;
`

type CleanupItem struct {
	ID                            int64
	Kind, Target, Expected, Error string
}

func contentHash(content string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(content))) }

func queueCleanup(ctx context.Context, tx *sql.Tx, workspace, kind, target, expected string, ready time.Time) error {
	if target == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO cleanup_items(workspace_id,kind,target,expected,ready_at) VALUES(?,?,?,?,?)
ON CONFLICT(workspace_id,kind,target,expected) DO UPDATE SET ready_at=MIN(ready_at,excluded.ready_at)`, workspace, kind, target, expected, ready.UTC().Format(time.RFC3339Nano))
	return err
}

/**
 * 文件发布与 SQLite 提交不能组成同一个原子操作。先留下清理意图，发布成功
 * 随业务事务撤销；宽限时间长于普通 MCP 预算，避免清理尚未进入事务的写入。
 */
func (s *Store) StagePlanCleanup(ctx context.Context, workspace string, plan PlanRecord) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return queueCleanup(ctx, tx, workspace, "plan", plan.Path, contentHash(plan.Content), time.Now().Add(5*time.Minute))
	})
}

/**
 * 每次只清理少量结束任务和旧观察。引用集合与删除在同一短事务中核对，
 * 不在写锁内调用 git 或扫描工作区；预算不足则整批回滚，下次入口再处理。
 */
func (s *Store) Prune(ctx context.Context, workspace string, now time.Time) error {
	cutoff := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, statement := range []string{
			`DROP TABLE IF EXISTS temp.prune_tasks`, `DROP TABLE IF EXISTS temp.keep_data`, `DROP TABLE IF EXISTS temp.keep_sources`,
			`DROP TABLE IF EXISTS temp.keep_observations`, `DROP TABLE IF EXISTS temp.keep_sessions`, `DROP TABLE IF EXISTS temp.keep_verifications`, `DROP TABLE IF EXISTS temp.prune_observations`,
			`CREATE TEMP TABLE prune_tasks(id TEXT PRIMARY KEY)`, `CREATE TEMP TABLE keep_data(data TEXT)`,
			`CREATE TEMP TABLE keep_sources(id TEXT PRIMARY KEY)`, `CREATE TEMP TABLE keep_observations(id INTEGER PRIMARY KEY)`,
			`CREATE TEMP TABLE keep_sessions(id TEXT PRIMARY KEY)`, `CREATE TEMP TABLE keep_verifications(id TEXT PRIMARY KEY)`, `CREATE TEMP TABLE prune_observations(id INTEGER PRIMARY KEY)`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prune_tasks SELECT id FROM tasks WHERE workspace_id=? AND state IN ('completed','canceled')
AND julianday(json_extract(data,'$.ended_at')) < julianday(?) ORDER BY seq LIMIT 8`, workspace, cutoff); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT data FROM plans WHERE task_id IN (SELECT id FROM prune_tasks)`)
		if err != nil {
			return err
		}
		var plans []PlanRecord
		for rows.Next() {
			var raw string
			var plan PlanRecord
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &plan); err != nil {
				rows.Close()
				return err
			}
			plans = append(plans, plan)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, plan := range plans {
			expected, err := filepath.Abs(filepath.Join(s.dataDir, "plans", plan.TaskID, fmt.Sprintf("v%d.md", plan.Version)))
			if err != nil || expected != plan.Path {
				return fmt.Errorf("方案文件路径与账本归属不符: %s", plan.ID)
			}
			if err := queueCleanup(ctx, tx, workspace, "plan", plan.Path, contentHash(plan.Content), now); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`DELETE FROM issue_blocks WHERE issue_id IN (SELECT json_extract(data,'$.id') FROM issues WHERE task_id IN (SELECT id FROM prune_tasks))`,
			`DELETE FROM settings WHERE scope='task' AND scope_id IN (SELECT id FROM prune_tasks)`,
			`UPDATE workspaces SET current_task_id='' WHERE current_task_id IN (SELECT id FROM prune_tasks)`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		for _, table := range []string{"requirement_locations", "requirement_versions", "notes", "task_updates", "checks", "finish_requests", "issues", "plans", "workspace_links"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id IN (SELECT id FROM prune_tasks)"); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id IN (SELECT id FROM prune_tasks)`); err != nil {
			return err
		}
		// 仍保留任务的全部修订都是引用根，不只保护最后一版。
		if _, err := tx.ExecContext(ctx, `INSERT INTO keep_data SELECT data FROM tasks WHERE workspace_id=?`, workspace); err != nil {
			return err
		}
		for _, table := range []string{"requirement_versions", "requirement_locations", "notes", "plans"} {
			if _, err := tx.ExecContext(ctx, "INSERT INTO keep_data SELECT data FROM "+table+" WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id=?)", workspace); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sources
WITH RECURSIVE refs(id) AS (
 SELECT value FROM keep_data,json_tree(keep_data.data) WHERE key IN ('source_id','start_message_id','request_message_id') AND type='text'
 UNION SELECT id FROM sources WHERE workspace_id=? AND task_id IN (SELECT id FROM tasks)
 UNION SELECT json_extract(s.data,'$.confirmation.source_id') FROM sources s JOIN refs r ON s.id=r.id WHERE s.workspace_id=?
) SELECT id FROM refs WHERE id IS NOT NULL AND id!=''`, workspace, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO keep_data SELECT data FROM sources WHERE id IN (SELECT id FROM keep_sources)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_observations SELECT value FROM keep_data,json_tree(keep_data.data)
WHERE key IN ('observation_id','origin_observation_id','resume_observation_id','last_observation_id') AND type='integer' AND value>0`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_observations SELECT id FROM observations WHERE workspace_id=? AND task_id IN (SELECT id FROM tasks)`, workspace); err != nil {
			return err
		}
		for _, table := range []string{"task_updates", "requirement_locations"} {
			if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM "+table+" WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id=?)", workspace); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_observations SELECT json_extract(data,'$.BeforeObservation') FROM file_changes WHERE workspace_id=? AND observation_id IN (SELECT id FROM observations WHERE task_id IN (SELECT id FROM tasks))`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sessions SELECT value FROM keep_data,json_tree(keep_data.data) WHERE key='session_id' AND type='text'`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sessions SELECT json_extract(data,'$.SessionID') FROM observations WHERE id IN (SELECT id FROM keep_observations) AND json_extract(data,'$.SessionID')!=''`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sessions SELECT session_id FROM messages WHERE workspace_id=? AND id IN (SELECT id FROM keep_sources)`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sessions SELECT session_id FROM verifications WHERE workspace_id=? AND task_id IN (SELECT id FROM tasks)`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_verifications
WITH RECURSIVE kept(id) AS (
 SELECT id FROM verifications WHERE workspace_id=? AND (task_id IN (SELECT id FROM tasks) OR NOT julianday(json_extract(data,'$.started_at'))<julianday(?) OR session_id IN (SELECT id FROM keep_sessions))
 UNION SELECT json_extract(v.data,'$.previous_id') FROM verifications v JOIN kept k ON v.id=k.id WHERE v.workspace_id=?
) SELECT id FROM kept WHERE id IS NOT NULL AND id!=''`, workspace, cutoff, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO keep_sessions SELECT session_id FROM verifications WHERE workspace_id=? AND id IN (SELECT id FROM keep_verifications)`, workspace); err != nil {
			return err
		}
		for _, item := range []struct{ table, at string }{
			{"hook_events", "at"}, {"mcp_contexts", "at"}, {"task_calls", "at"}, {"check_feedback", "at"}, {"messages", "json_extract(data,'$.at')"},
		} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+item.table+" WHERE workspace_id=? AND julianday("+item.at+")<julianday(?) AND session_id NOT IN (SELECT id FROM keep_sessions)", workspace, cutoff); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM verifications WHERE workspace_id=? AND id NOT IN (SELECT id FROM keep_verifications)`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE workspace_id=? AND julianday(json_extract(data,'$.at'))<julianday(?)
AND task_id NOT IN (SELECT id FROM tasks) AND id NOT IN (SELECT id FROM keep_sources) AND id NOT IN (SELECT id FROM messages)`, workspace, cutoff); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM edits WHERE workspace_id=? AND session_id NOT IN (SELECT id FROM keep_sessions)
AND before_observation IN (SELECT id FROM observations WHERE julianday(json_extract(data,'$.At'))<julianday(?))`, workspace, cutoff); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM session_summaries WHERE workspace_id=? AND session_id NOT IN (SELECT id FROM keep_sessions)
AND (workspace_id,agent,session_id) IN (SELECT workspace_id,agent,session_id FROM task_sessions WHERE julianday(activity_at)<julianday(?))`, workspace, cutoff); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_sessions WHERE workspace_id=? AND julianday(activity_at)<julianday(?) AND session_id NOT IN (SELECT id FROM keep_sessions)`, workspace, cutoff); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM check_feedback WHERE workspace_id=? AND at='' AND id NOT IN (SELECT feedback_id FROM issue_blocks)
AND (workspace_id,agent,session_id) NOT IN (SELECT workspace_id,agent,session_id FROM task_sessions)`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM verification_rounds WHERE workspace_id=? AND session_id NOT IN (SELECT id FROM keep_sessions)
AND (workspace_id,agent,session_id,round_id) NOT IN (SELECT workspace_id,agent,session_id,round_id FROM verifications)
AND (workspace_id,agent,session_id,round_id) NOT IN (SELECT workspace_id,agent,session_id,round_id FROM task_sessions)`, workspace); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_changes WHERE workspace_id=? AND julianday(json_extract(data,'$.At'))<julianday(?)
AND observation_id NOT IN (SELECT id FROM observations WHERE task_id IN (SELECT id FROM tasks))`, workspace, cutoff); err != nil {
			return err
		}
		// 边界观察的内容仍可回读，不沿历史文件变化无限保留更早的观察链。
		for _, statement := range []string{
			`INSERT OR IGNORE INTO keep_observations SELECT latest_observation FROM workspaces WHERE id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM observed_files WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM messages WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT json_extract(data,'$.observation_id') FROM sources WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM session_summaries WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT before_observation FROM edits WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT after_observation FROM edits WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM file_changes WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT json_extract(data,'$.BeforeObservation') FROM file_changes WHERE workspace_id=?`,
			`INSERT OR IGNORE INTO keep_observations SELECT observation_id FROM task_updates WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id=?)`,
		} {
			if _, err := tx.ExecContext(ctx, statement, workspace); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prune_observations SELECT id FROM observations WHERE workspace_id=? AND julianday(json_extract(data,'$.At'))<julianday(?)
AND id NOT IN (SELECT id FROM keep_observations) ORDER BY id LIMIT 256`, workspace, cutoff); err != nil {
			return err
		}
		rows, err = tx.QueryContext(ctx, `SELECT data FROM observations WHERE id IN (SELECT id FROM prune_observations)`)
		if err != nil {
			return err
		}
		var observations []Observation
		for rows.Next() {
			var raw string
			var observation Observation
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &observation); err != nil {
				rows.Close()
				return err
			}
			observations = append(observations, observation)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, observation := range observations {
			for _, ref := range []string{observation.SnapshotRef, observation.EvidenceRef} {
				if err := queueCleanup(ctx, tx, workspace, "ref", ref, "", now); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM observations WHERE id IN (SELECT id FROM prune_observations)`)
		return err
	})
}

func (s *Store) CleanupItems(ctx context.Context, workspace string, now time.Time) ([]CleanupItem, error) {
	var items []CleanupItem
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT id,kind,target,expected,error FROM cleanup_items WHERE workspace_id=? AND julianday(ready_at)<=julianday(?) ORDER BY id LIMIT 32`, workspace, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item CleanupItem
			if err := rows.Scan(&item.ID, &item.Kind, &item.Target, &item.Expected, &item.Error); err != nil {
				return err
			}
			items = append(items, item)
		}
		return rows.Err()
	})
	return items, err
}

func (s *Store) FinishCleanup(ctx context.Context, id int64, failure error) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if failure != nil {
			_, err := tx.ExecContext(ctx, `UPDATE cleanup_items SET error=?,ready_at=? WHERE id=?`, failure.Error(), time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), id)
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM cleanup_items WHERE id=?`, id)
		return err
	})
}

func (s *Store) CleanupStatus(ctx context.Context, workspace string) (string, error) {
	var output strings.Builder
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var count int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM cleanup_items WHERE workspace_id=?", workspace).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		fmt.Fprintf(&output, "保留期清理：%d 项待处理。\n", count)
		rows, err := q.QueryContext(ctx, "SELECT kind,target,error FROM cleanup_items WHERE workspace_id=? AND error!='' ORDER BY id LIMIT 8", workspace)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kind, target, reason string
			if err := rows.Scan(&kind, &target, &reason); err != nil {
				return err
			}
			fmt.Fprintf(&output, "  %s %s：%s\n", kind, target, reason)
		}
		return rows.Err()
	})
	return output.String(), err
}

func (s *Store) RefInUse(ctx context.Context, ref string) (bool, error) {
	var used bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM observations WHERE json_extract(data,'$.SnapshotRef')=? OR json_extract(data,'$.EvidenceRef')=?) OR EXISTS(SELECT 1 FROM tasks WHERE json_extract(data,'$.origin_ref')=?)`, ref, ref, ref).Scan(&used)
	})
	return used, err
}
