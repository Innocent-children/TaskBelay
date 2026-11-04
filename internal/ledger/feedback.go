package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type IssueFeedback struct {
	Issue         Issue
	Tree          string
	ObservationID int64
	At            time.Time
}

/**
 * RecordFeedback 只保存最终输出中出现的问题；同一发生记录的重复展示不重置时间和内容。
 */
func (s *Store) RecordFeedback(ctx context.Context, workspace, task, output string) error {
	if task == "" || !strings.Contains(output, "i-") {
		return nil
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		t, err := loadTask(ctx, tx, workspace, task)
		if err != nil {
			return err
		}
		var tree string
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(json_extract(data,'$.Tree'),'') FROM observations_v12 WHERE id=? AND workspace_id=?", t.LastObservationID, workspace).Scan(&tree); err != nil {
			return err
		}
		if tree == "" {
			return nil
		}
		issues, err := readIssues(ctx, tx, task)
		if err != nil {
			return err
		}
		for _, issue := range issues {
			if issue.ID == "" || len(issue.ContentSignature) == 0 || (issue.Status != "open" && issue.Status != "retained") || !strings.Contains(output, issue.ID) {
				continue
			}
			raw, err := json.Marshal(IssueFeedback{Issue: issue, Tree: tree, ObservationID: t.LastObservationID, At: time.Now().UTC()})
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO issue_feedback_v12 VALUES(?,?,?) ON CONFLICT(task_id,issue_id) DO NOTHING", task, issue.ID, string(raw)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) IssueFeedback(ctx context.Context, task string) ([]IssueFeedback, error) {
	var out []IssueFeedback
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT data FROM issue_feedback_v12 WHERE task_id=? ORDER BY issue_id", task)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			var value IssueFeedback
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(raw), &value); err != nil {
				return err
			}
			out = append(out, value)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Store) RemoveCalibration(ctx context.Context, id string) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM feedback_labels_v12 WHERE id=?", id)
		return err
	})
}
