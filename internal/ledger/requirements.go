package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func latestRequirement(ctx context.Context, q queryer, task, id string) (Requirement, error) {
	var requirement Requirement
	var data string
	if err := q.QueryRowContext(ctx, "SELECT data FROM requirement_versions WHERE task_id=? AND id=? ORDER BY revision DESC LIMIT 1", task, id).Scan(&data); err != nil {
		return requirement, err
	}
	err := json.Unmarshal([]byte(data), &requirement)
	if err == nil {
		err = currentLocations(ctx, q, &requirement)
	}
	return requirement, err
}

func currentLocations(ctx context.Context, q queryer, requirement *Requirement) error {
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM requirement_locations WHERE task_id=? AND id=? AND revision=?", requirement.TaskID, requirement.ID, requirement.Revision).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// 不能复用历史记录的切片：omitempty 字段可能残留旧值，也会改写同一视图中的历史位置。
	var locations []Location
	if err := json.Unmarshal([]byte(data), &locations); err != nil {
		return err
	}
	requirement.ImplementedBy = locations
	return nil
}

/**
 * 当前定位是观察所得的派生事实，单独保存，不改写用户需求的历史修订。
 * 读取后若出现新观察或需求修订，放弃这次发布，避免覆盖并发的新定位。
 */
func (s *Store) SaveLocations(ctx context.Context, workspace string, observationID int64, requirements []Requirement) error {
	if len(requirements) == 0 {
		return nil
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := checkObservation(ctx, tx, workspace, observationID); err != nil {
			return err
		}
		return saveLocations(ctx, tx, workspace, observationID, requirements)
	})
}

func saveLocations(ctx context.Context, tx *sql.Tx, workspace string, observationID int64, requirements []Requirement) error {
	for _, requirement := range requirements {
		if _, err := loadTask(ctx, tx, workspace, requirement.TaskID); err != nil {
			return err
		}
		latest, err := latestRequirement(ctx, tx, requirement.TaskID, requirement.ID)
		if err != nil {
			return err
		}
		if latest.Revision != requirement.Revision {
			return &RequirementConflict{Latest: latest}
		}
		data, err := json.Marshal(requirement.ImplementedBy)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO requirement_locations VALUES (?,?,?,?,?)
ON CONFLICT(task_id,id,revision) DO UPDATE SET observation_id=excluded.observation_id,data=excluded.data`, requirement.TaskID, requirement.ID, requirement.Revision, observationID, string(data)); err != nil {
			return err
		}
	}
	return nil
}

func touchTask(task *TaskRecord) {
	task.Revision++
	task.UpdatedAt = time.Now().UTC()
	if task.State == TaskPending {
		task.State = TaskActive
	}
}

func (s *Store) SaveRequirement(ctx context.Context, call Call, current string, observationID int64, input RequirementInput) (Requirement, error) {
	var result Requirement
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if found, err := replay(ctx, tx, call, &result); found || err != nil {
			return err
		}
		if err := checkActor(ctx, tx, call); err != nil {
			return err
		}
		if err := checkCurrent(ctx, tx, call.WorkspaceID, current); err != nil {
			return err
		}
		observation, err := checkObservation(ctx, tx, call.WorkspaceID, observationID)
		if err != nil {
			return err
		}
		if strings.TrimSpace(input.Content) == "" || strings.TrimSpace(input.Acceptance) == "" {
			return fmt.Errorf("需求内容和验收标准不能为空")
		}
		var task TaskRecord
		if current == "" {
			if input.ID != "" {
				return fmt.Errorf("当前没有任务，不能更新既有需求")
			}
			task, err = switchTask(ctx, tx, call, "", observation, StartTaskInput{Title: input.Content}, nil)
		} else {
			task, err = loadTask(ctx, tx, call.WorkspaceID, current)
		}
		if err != nil {
			return err
		}
		var old Requirement
		if input.ID != "" {
			old, err = latestRequirement(ctx, tx, task.ID, input.ID)
			if err != nil {
				return err
			}
			if input.BaseRevision == nil || *input.BaseRevision != old.Revision {
				return &RequirementConflict{Latest: old}
			}
		} else if input.BaseRevision != nil && *input.BaseRevision != 0 {
			return fmt.Errorf("新需求不能指定已有修订")
		}
		proof, err := citation(ctx, tx, call.WorkspaceID, task.ID, input.SourceID, input.Quote)
		if err != nil {
			return err
		}
		status := input.Status
		if status == "" {
			status = old.Status
		}
		if status == "" {
			status = "pending"
		}
		switch status {
		case "pending", "in_progress", "completed", "revoked":
		default:
			return fmt.Errorf("未知需求状态 %q", status)
		}
		result = Requirement{ID: old.ID, TaskID: task.ID, Content: input.Content, Acceptance: input.Acceptance, Status: status, Kind: "inferred", Citation: proof, ImplementedBy: old.ImplementedBy, Revision: old.Revision + 1, Author: call.Actor, At: time.Now().UTC()}
		if proof != nil {
			result.Kind = "sourced"
		}
		if result.ID == "" {
			result.ID = fmt.Sprintf("R%d", task.NextRequirement)
			task.NextRequirement++
		}
		if input.ImplementedBy != nil {
			result.ImplementedBy = append([]Location{}, (*input.ImplementedBy)...)
		}
		for i := range result.ImplementedBy {
			location := &result.ImplementedBy[i]
			if strings.TrimSpace(location.At) == "" {
				return fmt.Errorf("实现位置不能为空")
			}
			switch location.Change {
			case "", "add", "modify", "delete", "move", "rename":
			default:
				return fmt.Errorf("未知位置变更类型 %q", location.Change)
			}
			location.Citation, err = citation(ctx, tx, call.WorkspaceID, task.ID, location.SourceID, location.Quote)
			if err != nil {
				return err
			}
		}
		if old.ID != "" && sameRequirement(old, result) {
			result = old
			return saveCall(ctx, tx, call, result)
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO requirement_versions VALUES (?,?,?,?)", task.ID, result.ID, result.Revision, string(data)); err != nil {
			return err
		}
		// 进度变化不撤回范围授权；目标、验收或来源改变时，旧目标授权不能沿用。
		if old.ID != "" && (result.Status == "revoked" || old.Content != result.Content || old.Acceptance != result.Acceptance || !sameCitation(old.Citation, result.Citation)) {
			changed, err := invalidateNotes(ctx, tx, task.ID, fmt.Sprintf("%s@%d", result.ID, result.Revision), func(note Note) bool {
				return note.Target != nil && note.Target.Kind == "requirement" && note.Target.ID == result.ID
			})
			if err != nil {
				return err
			}
			if changed {
				task.AuthorizationRevision++
			}
		}
		task.RequirementRevision++
		touchTask(&task)
		if err := saveTask(ctx, tx, task); err != nil {
			return err
		}
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}

func sameCitation(a, b *Citation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.SourceID == b.SourceID && a.Start == b.Start && a.End == b.End
}

func sameRequirement(a, b Requirement) bool {
	if a.Content != b.Content || a.Acceptance != b.Acceptance || a.Status != b.Status || a.Kind != b.Kind || !sameCitation(a.Citation, b.Citation) || len(a.ImplementedBy) != len(b.ImplementedBy) {
		return false
	}
	for i, left := range a.ImplementedBy {
		right := b.ImplementedBy[i]
		if left.At != right.At || left.Purpose != right.Purpose || left.Change != right.Change || !sameCitation(left.Citation, right.Citation) {
			return false
		}
	}
	return true
}
