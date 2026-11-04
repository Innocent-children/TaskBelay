package ledger

import (
	"context"
	"encoding/json"
)

func (s *Store) CommandContext(ctx context.Context, workspace string) (TaskRecord, Observation, error) {
	var task TaskRecord
	var observation Observation
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		id, err := currentID(ctx, q, workspace)
		if err != nil || id == "" {
			return err
		}
		task, err = loadTask(ctx, q, workspace, id)
		if err != nil {
			return err
		}
		var raw string
		if err = q.QueryRowContext(ctx, "SELECT json_remove(data,'$.Files') FROM observations_v12 WHERE workspace_id=? AND id=?", workspace, task.LastObservationID).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(raw), &observation); err != nil {
			return err
		}
		// 正文在插入前编码，观察编号以数据库主键为准。
		observation.ID = task.LastObservationID
		return nil
	})
	return task, observation, err
}

func (s *Store) TaskChanges(ctx context.Context, workspace, task string, through int64) ([]FileChange, error) {
	var result []FileChange
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT c.data FROM file_changes_v12 c JOIN observations_v12 o ON o.id=c.observation_id WHERE o.workspace_id=? AND o.task_id=? AND o.id<=? ORDER BY c.id`, workspace, task, through)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw string
			var change FileChange
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(raw), &change); err != nil {
				return err
			}
			result = append(result, change)
		}
		return rows.Err()
	})
	return result, err
}
