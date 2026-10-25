package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func finishRequest(ctx context.Context, q queryer, workspace string, actor Actor) (*FinishRequest, error) {
	var data, state string
	err := q.QueryRowContext(ctx, "SELECT state,data FROM finish_requests WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=?", workspace, actor.Agent, actor.SessionID, actor.RoundID).Scan(&state, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var request FinishRequest
	if err := json.Unmarshal([]byte(data), &request); err != nil {
		return nil, err
	}
	request.State = state
	return &request, nil
}

func (s *Store) FinishRequest(ctx context.Context, workspace string, actor Actor) (*FinishRequest, error) {
	var request *FinishRequest
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		request, err = finishRequest(ctx, q, workspace, actor)
		return err
	})
	return request, err
}

func saveFinishRequest(ctx context.Context, tx *sql.Tx, workspace string, request FinishRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO finish_requests VALUES(?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,agent,session_id,round_id) DO UPDATE SET task_id=excluded.task_id,state=excluded.state,data=excluded.data`, workspace, request.Actor.Agent, request.Actor.SessionID, request.Actor.RoundID, request.TaskID, request.State, string(data))
	return err
}

func expireFinishRequests(ctx context.Context, tx *sql.Tx, workspace, task string) error {
	_, err := tx.ExecContext(ctx, "UPDATE finish_requests SET state='expired' WHERE workspace_id=? AND task_id=? AND state='pending'", workspace, task)
	return err
}

func (s *Store) RequestFinish(ctx context.Context, call Call, expected CheckInput) (FinishResult, error) {
	var result FinishResult
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if found, err := replay(ctx, tx, call, &result); found || err != nil {
			return err
		}
		if err := checkActor(ctx, tx, call); err != nil {
			return err
		}
		if err := checkCurrent(ctx, tx, call.WorkspaceID, expected.TaskID); err != nil {
			return err
		}
		input, err := checkInput(ctx, tx, call.WorkspaceID, expected.TaskID, call.Actor, expected.Rules, expected.Capabilities)
		if err != nil {
			return err
		}
		if input.Version() != expected.Version() {
			return ErrCheckInputChanged
		}
		result.Task, err = loadTask(ctx, tx, call.WorkspaceID, expected.TaskID)
		if err != nil {
			return err
		}
		if result.Task.State != TaskActive && result.Task.State != TaskPending {
			return fmt.Errorf("只有进行中任务可以请求交付")
		}
		request, err := finishRequest(ctx, tx, call.WorkspaceID, call.Actor)
		if err != nil {
			return err
		}
		if request == nil || request.State != "pending" || request.InputVersion != input.Version() {
			request = &FinishRequest{TaskID: input.TaskID, Actor: call.Actor, Input: input, InputVersion: input.Version(), State: "pending", At: time.Now().UTC()}
		}
		result.Request = request
		if err := saveFinishRequest(ctx, tx, call.WorkspaceID, *request); err != nil {
			return err
		}
		result.Task.State, result.Task.UpdatedAt = TaskPending, request.At
		result.Task.Revision++
		if err := saveTask(ctx, tx, result.Task); err != nil {
			return err
		}
		result.Static, err = latestCheck(ctx, tx, input.TaskID, "static", input.Version(), true)
		if err != nil {
			return err
		}
		result.Dynamic, err = latestCheck(ctx, tx, input.TaskID, "tests", input.Version(), true)
		if err != nil {
			return err
		}
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}

type Delivery struct {
	State   string      `json:"state"`
	Check   CheckRecord `json:"check"`
	Blocked []Issue     `json:"blocked,omitempty"`
}

/**
 * 只提交已保存结果的反馈和状态。重复 Stop 不再次扣拦截次数，输入变化后
 * 保留历史结果并退回进行中，完成任务不要求提交代码。
 */
func (s *Store) Deliver(ctx context.Context, workspace string, actor Actor, currentTree string, record CheckRecord) (Delivery, error) {
	result := Delivery{Check: record}
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		request, err := finishRequest(ctx, tx, workspace, actor)
		if err != nil {
			return err
		}
		if request == nil {
			return fmt.Errorf("本轮没有交付请求")
		}
		result.State = request.State
		if request.State != "pending" {
			return nil
		}
		task, err := loadTask(ctx, tx, workspace, request.TaskID)
		if err != nil {
			return err
		}
		input, err := checkInput(ctx, tx, workspace, task.ID, actor, record.Input.Rules, record.Input.Capabilities)
		if err != nil {
			return err
		}
		id, err := currentID(ctx, tx, workspace)
		if err != nil {
			return err
		}
		if id != task.ID || task.State != TaskPending || record.Status == CheckExpired || record.InputVersion != request.InputVersion || input.Version() != request.InputVersion || currentTree != input.Tree {
			request.State, result.State = "expired", "expired"
			if task.State == TaskPending {
				task.State = TaskActive
				task.Revision++
				if err := saveTask(ctx, tx, task); err != nil {
					return err
				}
			}
			return saveFinishRequest(ctx, tx, workspace, *request)
		}
		settings, err := effectiveSettings(ctx, tx, workspace, task.ID, false)
		if err != nil {
			return err
		}
		record = WithDelivery(record, settings.Values["enforcement"].Value == "block")
		result.Check = record
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE checks SET data=? WHERE seq=? AND task_id=? AND input_version=?", string(data), record.ID, task.ID, record.InputVersion); err != nil {
			return err
		}
		if !request.Feedback && settings.Values["enforcement"].Value == "block" {
			result.Blocked, err = blockIssues(ctx, tx, workspace, actor, record.Issues)
			if err != nil {
				return err
			}
		}
		request.CheckID, request.Feedback = record.ID, true
		if len(result.Blocked) == 0 {
			now := time.Now().UTC()
			request.State, result.State = "completed", "completed"
			task.State, task.EndedAt, task.UpdatedAt = TaskCompleted, &now, now
			task.Revision++
			if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(seq),0) FROM messages WHERE workspace_id=?", workspace).Scan(&task.EndMessageSeq); err != nil {
				return err
			}
			if err := saveTask(ctx, tx, task); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "UPDATE workspaces SET current_task_id='' WHERE id=? AND current_task_id=?", workspace, task.ID); err != nil {
				return err
			}
		}
		return saveFinishRequest(ctx, tx, workspace, *request)
	})
	return result, err
}
