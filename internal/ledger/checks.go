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

var ErrCheckInputChanged = errors.New("检查输入已变化，请读取当前任务后继续")

func effectiveDecisions(notes []Note, task TaskRecord, settings EffectiveSettings, actor Actor) []Note {
	var result []Note
	for _, note := range notes {
		if !note.Active || note.Consumed || note.Target == nil {
			continue
		}
		if note.Target.Scope == "round" && (actor.Agent != note.Author.Agent || actor.SessionID != note.Author.SessionID || actor.RoundID != note.Author.RoundID) {
			continue
		}
		if note.Target.Effect == "rerun_once" && (note.InputRevision != task.RecordRevision || note.RequirementRevision != task.RequirementRevision || note.PlanRevision != task.PlanRevision || note.SettingsRevision != settings.Revision) {
			continue
		}
		result = append(result, note)
	}
	return result
}

func checkInput(ctx context.Context, q queryer, workspace, id string, actor Actor, rules, capabilities string) (CheckInput, error) {
	task, err := loadTask(ctx, q, workspace, id)
	if err != nil {
		return CheckInput{}, err
	}
	observation, err := loadObservation(ctx, q, workspace, task.LastObservationID)
	if err != nil {
		return CheckInput{}, err
	}
	if !observation.Full || !observation.Complete || observation.Tree == "" {
		return CheckInput{}, fmt.Errorf("检查需要完整工作区快照")
	}
	settings, err := effectiveSettings(ctx, q, workspace, id, false)
	if err != nil {
		return CheckInput{}, err
	}
	notes, err := readNotes(ctx, q, id)
	if err != nil {
		return CheckInput{}, err
	}
	return NewCheckInput(task, observation.Tree, settings, effectiveDecisions(notes, task, settings, actor), rules, capabilities), nil
}

func (s *Store) CheckInput(ctx context.Context, workspace, id string, actor Actor, rules, capabilities string) (CheckInput, error) {
	var input CheckInput
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		input, err = checkInput(ctx, q, workspace, id, actor, rules, capabilities)
		return err
	})
	return input, err
}

func latestCheck(ctx context.Context, q queryer, task, depth, version string, excludeExpired bool) (*CheckRecord, error) {
	query := "SELECT seq,status,data FROM checks WHERE task_id=? AND depth=?"
	args := []any{task, depth}
	if version != "" {
		query += " AND input_version=?"
		args = append(args, version)
	}
	if excludeExpired {
		query += " AND status!='expired'"
	}
	query += " ORDER BY seq DESC LIMIT 1"
	var result CheckRecord
	var data string
	var id int64
	var status CheckStatus
	err := q.QueryRowContext(ctx, query, args...).Scan(&id, &status, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		return nil, err
	}
	result.ID, result.Status = id, status
	return &result, nil
}

func (s *Store) LatestCheck(ctx context.Context, task, depth, version string) (*CheckRecord, error) {
	var result *CheckRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = latestCheck(ctx, q, task, depth, version, version != "")
		return err
	})
	return result, err
}

func (s *Store) PreviousCheck(ctx context.Context, task, depth string) (*CheckRecord, error) {
	var result *CheckRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = latestCheck(ctx, q, task, depth, "", true)
		return err
	})
	return result, err
}

func viewChecks(ctx context.Context, q queryer, view *TaskView, actor Actor) error {
	var labels []string
	for _, depth := range []string{"static", "tests"} {
		record, err := latestCheck(ctx, q, view.Task.ID, depth, "", false)
		if err != nil {
			return err
		}
		if record == nil {
			continue
		}
		input, err := checkInput(ctx, q, view.Task.WorkspaceID, view.Task.ID, actor, record.Input.Rules, record.Input.Capabilities)
		if err != nil {
			return err
		}
		if input.Version() != record.InputVersion {
			record.Status = CheckExpired
			record.Level = "hint"
			for i := range record.Issues {
				record.Issues[i].Level = "hint"
			}
		}
		view.Checks = append(view.Checks, *record)
		labels = append(labels, depth+": "+string(record.Status))
	}
	if len(labels) > 0 {
		view.LatestCheck = strings.Join(labels, "; ")
	}
	var err error
	view.Issues, err = readIssues(ctx, q, view.Task.ID)
	return err
}

/**
 * currentTree 由调用方在分析后重新采集；事务内再核对账本输入和任务状态。
 * 不持有工作区锁，当前结果必须同时满足这两次核对，过期结果只保存历史。
 */
func (s *Store) SaveCheck(ctx context.Context, workspace string, actor Actor, currentTree string, result CheckRecord) (CheckRecord, error) {
	if result.Depth != "static" && result.Depth != "tests" {
		return result, fmt.Errorf("未知检查深度")
	}
	if result.Status != CheckComplete && result.Status != CheckPartial && result.Status != CheckFailed {
		return result, fmt.Errorf("未知检查完成状态")
	}
	result.InputVersion = result.Input.Version()
	result.ComputedStatus = result.Status
	result.FinishedAt = time.Now().UTC()
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		input, err := checkInput(ctx, tx, workspace, result.Input.TaskID, actor, result.Input.Rules, result.Input.Capabilities)
		if err != nil {
			return err
		}
		id, err := currentID(ctx, tx, workspace)
		if err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, workspace, result.Input.TaskID)
		if err != nil {
			return err
		}
		if id != task.ID || (task.State != TaskActive && task.State != TaskPending) || input.Version() != result.InputVersion || currentTree != result.Input.Tree {
			result.Status = CheckExpired
		} else if err := reconcileIssues(ctx, tx, &result); err != nil {
			return err
		}
		if result.Status != CheckExpired {
			if err := saveLocations(ctx, tx, workspace, task.LastObservationID, result.Locations); err != nil {
				return err
			}
		}
		result.Level = HighestLevel(result.Issues)
		if result.Status == CheckExpired {
			result.Level = "hint"
			for i := range result.Issues {
				result.Issues[i].Level = "hint"
			}
			if err := expireFinishRequests(ctx, tx, workspace, task.ID); err != nil {
				return err
			}
			if task.State == TaskPending {
				task.State = TaskActive
				task.Revision++
				if err := saveTask(ctx, tx, task); err != nil {
					return err
				}
			}
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		insert, err := tx.ExecContext(ctx, "INSERT INTO checks(task_id,depth,input_version,status,data) VALUES(?,?,?,?,?)", task.ID, result.Depth, result.InputVersion, result.Status, string(data))
		if err != nil {
			return err
		}
		result.ID, err = insert.LastInsertId()
		return err
	})
	return result, err
}
