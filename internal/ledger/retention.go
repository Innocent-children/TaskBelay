package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
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
	WorkspaceID                   string
}

func contentHash(content string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(content))) }

func queueCleanup(ctx context.Context, tx *sql.Tx, workspace, kind, target, expected string, ready time.Time) error {
	if target == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO cleanup_items_v11(workspace_id,kind,target,expected,ready_at) VALUES(?,?,?,?,?)
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

func (s *Store) CleanupItems(ctx context.Context, workspace string, now time.Time) ([]CleanupItem, error) {
	var items []CleanupItem
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT id,kind,target,expected,error FROM cleanup_items_v11 WHERE workspace_id=? AND julianday(ready_at)<=julianday(?) ORDER BY id LIMIT 32`, workspace, now.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item CleanupItem
			item.WorkspaceID = workspace
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
			_, err := tx.ExecContext(ctx, `UPDATE cleanup_items_v11 SET error=?,ready_at=? WHERE id=?`, failure.Error(), time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano), id)
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM cleanup_items_v11 WHERE id=?`, id)
		return err
	})
}

func (s *Store) CleanupStatus(ctx context.Context, workspace string) (string, error) {
	var output strings.Builder
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var count int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM cleanup_items_v11 WHERE workspace_id=?", workspace).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		fmt.Fprintf(&output, "保留期清理：%d 项待处理。\n", count)
		rows, err := q.QueryContext(ctx, "SELECT kind,target,error FROM cleanup_items_v11 WHERE workspace_id=? AND error!='' ORDER BY id LIMIT 8", workspace)
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
		return q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM observations_v11 WHERE json_extract(data,'$.SnapshotRef')=? OR json_extract(data,'$.EvidenceRef')=?) OR EXISTS(SELECT 1 FROM tasks_v11 WHERE json_extract(data,'$.origin_ref')=?)`, ref, ref, ref).Scan(&used)
	})
	return used, err
}
