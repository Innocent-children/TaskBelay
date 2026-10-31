package ledger

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

/**
 * 一次只准备一条正文，写入前重新核对原文。压缩占满准备时间时保存原文，
 * 总预算耗尽时不发布；失败和并发删除均不会留下半条引用。
 */
func (s *Store) Compact(ctx context.Context, workspace string) error {
	for _, category := range []string{"observations", "checks"} {
		for ctx.Err() == nil {
			deadline, ok := ctx.Deadline()
			if !ok {
				deadline = time.Now().Add(NormalWait)
			}
			if time.Until(deadline) <= 20*time.Millisecond {
				return nil
			}
			var id int64
			var original string
			err := s.read(ctx, func(ctx context.Context, q queryer) error {
				if category == "observations" {
					return q.QueryRowContext(ctx, `SELECT id,data FROM observations_v11 WHERE workspace_id=? AND files_set_id IS NULL AND json_extract(data,'$.Full')=1 AND json_extract(data,'$.Complete')=1 AND json_type(data,'$.Files')='object' ORDER BY id LIMIT 1`, workspace).Scan(&id, &original)
				}
				return q.QueryRowContext(ctx, `SELECT c.seq,c.data FROM checks_v11 c JOIN tasks_v11 t ON t.id=c.task_id WHERE t.workspace_id=? AND c.report_encoding='' ORDER BY c.seq LIMIT 1`, workspace).Scan(&id, &original)
			})
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			if err != nil {
				return err
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(original), &fields); err != nil {
				return fmt.Errorf("整理 %s %d: %w", category, id, err)
			}
			field := "report"
			if category == "observations" {
				field = "Files"
			}
			raw := fields[field]
			if len(raw) == 0 {
				return fmt.Errorf("整理 %s %d: 正文缺失", category, id)
			}
			if category == "observations" {
				var observation Observation
				if err := json.Unmarshal([]byte(original), &observation); err != nil {
					return err
				}
				raw, err = json.Marshal(observation.Files)
				if err != nil {
					return err
				}
			}
			prepareCtx, cancel := context.WithDeadline(ctx, deadline.Add(-20*time.Millisecond))
			body, prepareErr := prepareBody(prepareCtx, raw)
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if prepareErr != nil && !errors.Is(prepareErr, context.DeadlineExceeded) {
				return prepareErr
			}
			if prepareErr != nil {
				body.Encoding, body.Body = "json", raw
			}
			restored, err := body.decode(ctx)
			if err != nil || !bytes.Equal(restored, raw) {
				return fmt.Errorf("整理 %s %d 正文还原失败: %v", category, id, err)
			}
			delete(fields, field)
			metadata, err := json.Marshal(fields)
			if err != nil {
				return err
			}
			err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
				var current string
				query := `SELECT data FROM observations_v11 WHERE id=? AND files_set_id IS NULL`
				if category == "checks" {
					query = `SELECT data FROM checks_v11 WHERE seq=? AND report_encoding=''`
				}
				err := tx.QueryRowContext(ctx, query, id).Scan(&current)
				if errors.Is(err, sql.ErrNoRows) || err == nil && current != original {
					return nil
				}
				if err != nil {
					return err
				}
				if category == "observations" {
					filesID, err := saveFileSet(ctx, tx, &body)
					if err != nil {
						return err
					}
					_, err = tx.ExecContext(ctx, `UPDATE observations_v11 SET data=?,files_set_id=? WHERE id=?`, string(metadata), filesID, id)
					return err
				}
				_, err = tx.ExecContext(ctx, `UPDATE checks_v11 SET data=?,report_encoding=?,report_raw_bytes=?,report_sha256=?,report_body=? WHERE seq=?`, string(metadata), body.Encoding, body.RawBytes, body.SHA256, body.Body, id)
				return err
			})
			if err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}
