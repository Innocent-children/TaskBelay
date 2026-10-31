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

func loadTask(ctx context.Context, q queryer, workspace, id string) (TaskRecord, error) {
	var task TaskRecord
	var data string
	if err := q.QueryRowContext(ctx, "SELECT data FROM tasks_v11 WHERE workspace_id=? AND id=?", workspace, id).Scan(&data); err != nil {
		return task, err
	}
	err := json.Unmarshal([]byte(data), &task)
	return task, err
}

func saveTask(ctx context.Context, tx *sql.Tx, task TaskRecord) error {
	if task.State != TaskPending {
		if err := expireFinishRequests(ctx, tx, task.WorkspaceID, task.ID); err != nil {
			return err
		}
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE tasks_v11 SET state=?,branch=?,data=? WHERE id=? AND workspace_id=?", task.State, task.Context.Branch, string(data), task.ID, task.WorkspaceID)
	return err
}

func currentID(ctx context.Context, q queryer, workspace string) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, "SELECT current_task_id FROM workspaces_v11 WHERE id=?", workspace).Scan(&id)
	return id, err
}

func (s *Store) CurrentTaskID(ctx context.Context, workspace string) (string, error) {
	var id string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		id, err = currentID(ctx, q, workspace)
		return err
	})
	return id, err
}

func checkCurrent(ctx context.Context, q queryer, workspace, expected string) error {
	id, err := currentID(ctx, q, workspace)
	if err != nil {
		return err
	}
	if id != expected {
		return &TaskConflict{CurrentTaskID: id}
	}
	return nil
}

func checkObservation(ctx context.Context, q queryer, workspace string, id int64) (Observation, error) {
	var latest int64
	if err := q.QueryRowContext(ctx, "SELECT latest_observation FROM workspaces_v11 WHERE id=?", workspace).Scan(&latest); err != nil {
		return Observation{}, err
	}
	if latest != id {
		return Observation{}, ErrObservationConflict
	}
	observation, err := loadObservation(ctx, q, workspace, id)
	if err != nil {
		return observation, err
	}
	if !observation.Full || !observation.Complete || observation.Tree == "" || observation.SnapshotRef == "" {
		return observation, fmt.Errorf("工作区观察不完整，不能建立或恢复任务")
	}
	return observation, nil
}

func checkActor(ctx context.Context, q queryer, call Call) error {
	if err := call.Actor.Validate(); err != nil {
		return err
	}
	if call.Actor.CallID == "" || call.InputHash == "" || call.Operation == "" {
		return fmt.Errorf("缺少工具调用身份")
	}
	var round string
	if err := q.QueryRowContext(ctx, "SELECT round_id FROM task_sessions_v11 WHERE workspace_id=? AND agent=? AND session_id=?", call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID).Scan(&round); err != nil {
		return fmt.Errorf("当前 session 尚无可核对的消息来源: %w", err)
	}
	if round != call.Actor.RoundID {
		return fmt.Errorf("请求轮次不是该 session 最近记录的轮次")
	}
	return nil
}

func originMessage(ctx context.Context, q queryer, call Call, current, explicit string) (Message, error) {
	var data string
	var err error
	if explicit != "" {
		err = q.QueryRowContext(ctx, "SELECT data FROM messages_v11 WHERE workspace_id=? AND id=?", call.WorkspaceID, explicit).Scan(&data)
	} else if current != "" {
		err = q.QueryRowContext(ctx, "SELECT data FROM messages_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? ORDER BY seq LIMIT 1", call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID, call.Actor.RoundID).Scan(&data)
	} else {
		err = q.QueryRowContext(ctx, `SELECT data FROM messages_v11 WHERE workspace_id=? AND agent=? AND session_id=?
AND seq>(SELECT COALESCE(MAX(json_extract(data,'$.end_message_seq')),0) FROM tasks_v11 WHERE workspace_id=?) ORDER BY seq LIMIT 1`, call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID, call.WorkspaceID).Scan(&data)
	}
	var message Message
	if err != nil {
		return message, fmt.Errorf("没有可用的起点消息，请核对消息记录或 start_message_id: %w", err)
	}
	err = json.Unmarshal([]byte(data), &message)
	return message, err
}

func pauseTask(ctx context.Context, tx *sql.Tx, workspace, id string, now time.Time) error {
	if id == "" {
		return nil
	}
	task, err := loadTask(ctx, tx, workspace, id)
	if err != nil {
		return err
	}
	task.State, task.PausedAt, task.UpdatedAt = TaskPaused, &now, now
	task.Revision++
	return saveTask(ctx, tx, task)
}

/**
 * switchTask 只修改账本；快照和差异必须在进入写事务前完成。
 */
func switchTask(ctx context.Context, tx *sql.Tx, call Call, current string, observation Observation, input StartTaskInput, updates []FileChange) (TaskRecord, error) {
	var task TaskRecord
	now := time.Now().UTC()
	if input.Workflow != "" {
		if err := ValidateSetting("workflow", input.Workflow); err != nil {
			return task, err
		}
	}
	if input.TaskID != "" {
		if input.Title != "" || input.StartMessageID != "" {
			return task, fmt.Errorf("恢复任务不能替换标题或原始起点")
		}
		var err error
		task, err = loadTask(ctx, tx, call.WorkspaceID, input.TaskID)
		if err != nil {
			return task, err
		}
		if task.State == TaskCanceled {
			return task, fmt.Errorf("已中止任务不能恢复，请开始新任务")
		}
	} else {
		if strings.TrimSpace(input.Title) == "" {
			return task, fmt.Errorf("新任务需要标题")
		}
		message, err := originMessage(ctx, tx, call, current, input.StartMessageID)
		if err != nil {
			return task, err
		}
		origin, err := loadObservation(ctx, tx, call.WorkspaceID, message.ObservationID)
		if err != nil {
			return task, err
		}
		if !origin.Full || !origin.Complete || origin.Tree == "" || origin.SnapshotRef == "" {
			return task, fmt.Errorf("起点消息的快照不完整，不能用当前代码替代")
		}
		insert, err := tx.ExecContext(ctx, "INSERT INTO tasks_v11(workspace_id,state,branch,data) VALUES (?,?,?,?)", call.WorkspaceID, TaskActive, observation.Context.Branch, "")
		if err != nil {
			return task, err
		}
		seq, err := insert.LastInsertId()
		if err != nil {
			return task, err
		}
		task = TaskRecord{ID: fmt.Sprintf("t%d", seq), WorkspaceID: call.WorkspaceID, Title: input.Title, State: TaskActive, StartMessageID: message.ID, OriginObservationID: origin.ID, OriginTree: origin.Tree, OriginRef: origin.SnapshotRef, NextRequirement: 1, CreatedAt: now}
		updates, err = changesBeforeStart(ctx, tx, call.WorkspaceID, origin.ID, observation.ID)
		if err != nil {
			return task, err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE tasks_v11 SET id=? WHERE seq=?", task.ID, seq); err != nil {
			return task, err
		}
	}
	if current != task.ID {
		if err := pauseTask(ctx, tx, call.WorkspaceID, current, now); err != nil {
			return task, err
		}
		task.State, task.PausedAt, task.EndedAt = TaskActive, nil, nil
		if len(updates) > 0 {
			data, err := json.Marshal(updates)
			if err != nil {
				return task, err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO task_updates_v11 VALUES (?,?,?)", task.ID, observation.ID, string(data)); err != nil {
				return task, err
			}
		}
	}
	task.Context, task.UpdatedAt = observation.Context, now
	task.ResumeObservationID, task.LastObservationID = observation.ID, observation.ID
	task.Revision++
	if err := tx.QueryRowContext(ctx, "SELECT record_revision FROM workspaces_v11 WHERE id=?", call.WorkspaceID).Scan(&task.RecordRevision); err != nil {
		return task, err
	}
	if input.Workflow != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings_v11 VALUES ('task',?,'workflow',?,1)
ON CONFLICT(scope,scope_id,name) DO UPDATE SET value=excluded.value,revision=settings_v11.revision+1 WHERE settings_v11.value!=excluded.value`, task.ID, input.Workflow); err != nil {
			return task, err
		}
	}
	settings, err := effectiveSettings(ctx, tx, call.WorkspaceID, task.ID, false)
	if err != nil {
		return task, err
	}
	if task.SettingsRevision != "" && task.SettingsRevision != settings.Revision && task.State == TaskPending {
		task.State = TaskActive
	}
	if task.Workflow != settings.Values["workflow"].Value {
		task.WorkflowHistory = append(task.WorkflowHistory, WorkflowChange{From: task.Workflow, To: settings.Values["workflow"].Value, At: now, ObservationID: observation.ID, Tree: observation.Tree})
	}
	task.Workflow, task.SettingsRevision = settings.Values["workflow"].Value, settings.Revision
	if err := saveTask(ctx, tx, task); err != nil {
		return task, err
	}
	_, err = tx.ExecContext(ctx, "UPDATE workspaces_v11 SET current_task_id=? WHERE id=?", task.ID, call.WorkspaceID)
	return task, err
}

/**
 * 起点消息可能早于任务建立。把这段时间已经观察到的变化带入新任务，
 * 保留原记录及其作者证据；新任务只把开始时已有的内容视为工作区更新。
 */
func changesBeforeStart(ctx context.Context, q queryer, workspace string, origin, current int64) ([]FileChange, error) {
	rows, err := q.QueryContext(ctx, "SELECT data FROM file_changes_v11 WHERE workspace_id=? AND observation_id>? AND observation_id<=? ORDER BY id", workspace, origin, current)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []FileChange
	for rows.Next() {
		var data string
		var change FileChange
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &change); err != nil {
			return nil, err
		}
		change.Kind, change.ToolCallID = "workspace_update", ""
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

func (s *Store) StartTask(ctx context.Context, call Call, current string, observationID int64, input StartTaskInput, updates []FileChange) (TaskRecord, error) {
	var task TaskRecord
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if found, err := replay(ctx, tx, call, &task); found || err != nil {
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
		task, err = switchTask(ctx, tx, call, current, observation, input, updates)
		if err != nil {
			return err
		}
		return saveCall(ctx, tx, call, task)
	})
	return task, err
}

/**
 * 维护任务状态约束，取消必须带可核对的用户来源。
 * 普通提交、观察和 session 结束不会调用本方法结束任务。
 */
func (s *Store) TransitionTask(ctx context.Context, call Call, id string, state TaskState, sourceID, quote string) (TaskRecord, error) {
	result, err := s.transition(ctx, call, id, state, sourceID, quote, false)
	return result.Task, err
}

func (s *Store) CancelFinish(ctx context.Context, call Call, id, sourceID, quote string) (FinishResult, error) {
	return s.transition(ctx, call, id, TaskCanceled, sourceID, quote, true)
}

func (s *Store) transition(ctx context.Context, call Call, id string, state TaskState, sourceID, quote string, finish bool) (FinishResult, error) {
	var task TaskRecord
	var result FinishResult
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if finish {
			if found, err := replay(ctx, tx, call, &result); found || err != nil {
				task = result.Task
				return err
			}
		} else {
			if found, err := replay(ctx, tx, call, &task); found || err != nil {
				return err
			}
		}
		if err := checkActor(ctx, tx, call); err != nil {
			return err
		}
		if err := checkCurrent(ctx, tx, call.WorkspaceID, id); err != nil {
			return err
		}
		var err error
		task, err = loadTask(ctx, tx, call.WorkspaceID, id)
		if err != nil {
			return err
		}
		switch state {
		case TaskPending:
			if task.State != TaskActive {
				return fmt.Errorf("只有进行中任务可以请求交付")
			}
		case TaskCompleted:
			if task.State != TaskPending {
				return fmt.Errorf("只有待交付任务可以完成")
			}
		case TaskCanceled:
			proof, err := citation(ctx, tx, call.WorkspaceID, id, sourceID, quote)
			if err != nil {
				return err
			}
			if proof == nil {
				return fmt.Errorf("中止任务需要用户明确放弃的来源")
			}
			source, err := loadSource(ctx, tx, call.WorkspaceID, proof.SourceID)
			if err != nil {
				return err
			}
			if source.Kind != "user_message" {
				return fmt.Errorf("中止须引用用户消息")
			}
			task.Cancellation = proof
		default:
			return fmt.Errorf("此入口不支持该状态转换")
		}
		now := time.Now().UTC()
		task.State, task.UpdatedAt = state, now
		task.Revision++
		if state == TaskCompleted || state == TaskCanceled {
			task.EndedAt = &now
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM messages_v11 WHERE workspace_id=?", call.WorkspaceID).Scan(&task.EndMessageSeq); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE workspaces_v11 SET current_task_id='' WHERE id=?", call.WorkspaceID); err != nil {
				return err
			}
		}
		if err := saveTask(ctx, tx, task); err != nil {
			return err
		}
		if finish {
			return saveCall(ctx, tx, call, FinishResult{Task: task})
		}
		return saveCall(ctx, tx, call, task)
	})
	return FinishResult{Task: task}, err
}

func syncTaskObservation(ctx context.Context, tx *sql.Tx, observation Observation, delta int64) error {
	if observation.TaskID == "" {
		return nil
	}
	id, err := currentID(ctx, tx, observation.WorkspaceID)
	if err != nil {
		return err
	}
	if id != observation.TaskID {
		return nil
	}
	task, err := loadTask(ctx, tx, observation.WorkspaceID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// 分支变化由恢复事务处理，先保留离开分支时的进度与关联。
	if task.Context.Branch != observation.Context.Branch {
		return nil
	}
	task.Context = observation.Context
	if observation.Full && observation.Complete {
		task.LastObservationID = observation.ID
	}
	if delta > 0 {
		task.RecordRevision += delta
		task.Revision++
		task.UpdatedAt = observation.At
		if task.State == TaskPending {
			task.State = TaskActive
		}
	}
	return saveTask(ctx, tx, task)
}
