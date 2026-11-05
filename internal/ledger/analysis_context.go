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

/**
 * 有有效需求时只读取匹配所需的记录；没有有效需求时沿用完整任务视图，
 * 使原话回退包含历史引用展开的消息，避免另写一套来源递归规则。
 */
func (s *Store) EditAnalysisContext(ctx context.Context, workspace string) (TaskView, error) {
	var view TaskView
	hasRequirement := false
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		view.CurrentTaskID, err = currentID(ctx, q, workspace)
		if err != nil || view.CurrentTaskID == "" {
			return err
		}
		task, err := loadTask(ctx, q, workspace, view.CurrentTaskID)
		if err != nil {
			return err
		}
		view.Task = &task
		view.Plans, err = readPlans(ctx, q, task)
		if err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `SELECT r.data FROM requirement_versions_v12 r
JOIN (SELECT id,MAX(revision) AS revision,MIN(rowid) AS first_row
 FROM requirement_versions_v12 WHERE task_id=? GROUP BY id) latest
ON r.task_id=? AND r.id=latest.id AND r.revision=latest.revision ORDER BY latest.first_row`, task.ID, task.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw string
			var requirement Requirement
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(raw), &requirement); err != nil {
				rows.Close()
				return err
			}
			view.Requirements = append(view.Requirements, requirement)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for i := range view.Requirements {
			if err := currentLocations(ctx, q, &view.Requirements[i]); err != nil {
				return err
			}
			hasRequirement = hasRequirement || view.Requirements[i].Status != "revoked"
		}
		return nil
	})
	if err != nil || view.Task == nil || hasRequirement {
		return view, err
	}
	return s.TaskView(ctx, workspace, view.Task.ID, Actor{})
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
