package ledger

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
)

var ErrVerificationChanged = errors.New("验证输入或上一条验证已变化，请按当前状态运行")

func verificationByCall(ctx context.Context, q queryer, workspace string, actor Actor) (*VerificationRecord, error) {
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM verifications_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", workspace, actor.Agent, actor.SessionID, actor.CallID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record VerificationRecord
	err = json.Unmarshal([]byte(data), &record)
	return &record, err
}

func (s *Store) VerificationByCall(ctx context.Context, workspace string, actor Actor) (*VerificationRecord, error) {
	var result *VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = verificationByCall(ctx, q, workspace, actor)
		return err
	})
	return result, err
}

func previousVerification(ctx context.Context, q queryer, workspace string, actor Actor, key string) (*VerificationRecord, error) {
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM verifications_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND command_key=? AND denied=0 ORDER BY seq DESC LIMIT 1", workspace, actor.Agent, actor.SessionID, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record VerificationRecord
	err = json.Unmarshal([]byte(data), &record)
	return &record, err
}

func (s *Store) PreviousVerification(ctx context.Context, workspace string, actor Actor, key string) (*VerificationRecord, error) {
	var result *VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = previousVerification(ctx, q, workspace, actor, key)
		return err
	})
	return result, err
}

func saveVerification(ctx context.Context, tx *sql.Tx, record VerificationRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO verifications_v11(id,workspace_id,task_id,agent,session_id,round_id,call_id,command_key,denied,executed,state,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET denied=excluded.denied,executed=excluded.executed,state=excluded.state,data=excluded.data`, record.ID, record.WorkspaceID, record.TaskID, record.Actor.Agent, record.Actor.SessionID, record.Actor.RoundID, record.Actor.CallID, record.CommandKey, record.Denied, record.Executed, record.State, string(data))
	return err
}

/**
 * 分析在事务外完成；这里只核对前置事实、消费一次授权并保存同一工具调用的决定。
 * 并发到来的新验证不能绕过上一条仍在运行或状态未知的验证。
 */
func (s *Store) BeginVerification(ctx context.Context, record VerificationRecord) (VerificationRecord, error) {
	var result VerificationRecord
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		prior, err := verificationByCall(ctx, tx, record.WorkspaceID, record.Actor)
		if err != nil {
			return err
		}
		if prior != nil {
			if prior.RequestHash != record.RequestHash {
				return fmt.Errorf("验证工具调用编号与原请求不一致")
			}
			result = *prior
			return nil
		}
		if record.Actor.Agent == "" || record.Actor.SessionID == "" || record.Actor.CallID == "" {
			return fmt.Errorf("验证记录缺少工具调用身份")
		}
		previous, err := previousVerification(ctx, tx, record.WorkspaceID, record.Actor, record.CommandKey)
		if err != nil {
			return err
		}
		previousID := ""
		if previous != nil {
			previousID = previous.ID
		}
		if previousID != record.PreviousID {
			return ErrVerificationChanged
		}
		current, err := currentID(ctx, tx, record.WorkspaceID)
		if err != nil {
			return err
		}
		if current != record.TaskID {
			return ErrVerificationChanged
		}
		settings, err := effectiveSettings(ctx, tx, record.WorkspaceID, record.TaskID, false)
		if err != nil {
			return err
		}
		if record.TaskID != "" {
			task, err := loadTask(ctx, tx, record.WorkspaceID, record.TaskID)
			if err != nil {
				return err
			}
			if task.RecordRevision != record.InputRevision {
				return ErrVerificationChanged
			}
			if record.PreviousID != "" && !record.PostOnly {
				note, err := consumeVerificationRerun(ctx, tx, &task, settings, record.PreviousID)
				if err != nil {
					return err
				}
				if note != nil {
					record.RerunNoteID, record.UserRequested = note.ID, note.Citation != nil
				}
			}
			if record.Issue != nil {
				check := CheckRecord{Input: CheckInput{TaskID: record.TaskID}, Issues: []Issue{*record.Issue}, Evaluated: []string{CheckScope("V1", record.CommandKey)}}
				if err := reconcileIssues(ctx, tx, &check); err != nil {
					return err
				}
				record.Issue = &check.Issues[0]
				if record.RerunNoteID != "" {
					record.Issue.Level = "hint"
					record.Issue.HandledBy = append(record.Issue.HandledBy, record.RerunNoteID)
				}
				if settings.Values["enforcement"].Value == "hint" {
					record.Issue.Level = "hint"
				}
				if err := saveIssue(ctx, tx, record.TaskID, *record.Issue); err != nil {
					return err
				}
				blocked, err := blockIssues(ctx, tx, record.WorkspaceID, record.Actor, []Issue{*record.Issue})
				if err != nil {
					return err
				}
				if len(blocked) > 0 {
					record.Denied = true
					record.Issue = &blocked[0]
				}
			}
		}
		record.ID, record.State = "v-"+rand.Text(), "unknown"
		if record.Denied || record.PostOnly {
			record.State = "unknown"
		} else {
			record.StartedAt = time.Now().UTC()
		}
		if err := saveVerification(ctx, tx, record); err != nil {
			return err
		}
		result = record
		return nil
	})
	return result, err
}

/**
 * 本轮停止时，只有启动结果而没有最终退出信息的命令保持未知。
 * 后续可核对的完成事件仍可补写；这里不补造结束时间，也不增加执行次数。
 */
func (s *Store) UnfinishedVerifications(ctx context.Context, workspace string, actor Actor) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT data FROM verifications_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? AND state='running'", workspace, actor.Agent, actor.SessionID, actor.RoundID)
		if err != nil {
			return err
		}
		var records []VerificationRecord
		for rows.Next() {
			var data string
			var record VerificationRecord
			if err := rows.Scan(&data); err != nil {
				rows.Close()
				return err
			}
			if err := json.Unmarshal([]byte(data), &record); err != nil {
				rows.Close()
				return err
			}
			record.State = "unknown"
			records = append(records, record)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, record := range records {
			if err := saveVerification(ctx, tx, record); err != nil {
				return err
			}
		}
		return nil
	})
}

func consumeVerificationRerun(ctx context.Context, tx *sql.Tx, task *TaskRecord, settings EffectiveSettings, previous string) (*Note, error) {
	notes, err := readNotes(ctx, tx, task.ID)
	if err != nil {
		return nil, err
	}
	for _, note := range notes {
		if !note.Active || note.Consumed || note.Target == nil || note.Target.Effect != "rerun_once" || note.Target.ID != previous {
			continue
		}
		if note.InputRevision != task.RecordRevision || note.RequirementRevision != task.RequirementRevision || note.PlanRevision != task.PlanRevision || note.SettingsRevision != settings.Revision {
			continue
		}
		note.Active, note.Consumed = false, true
		if _, err := tx.ExecContext(ctx, "UPDATE notes_v11 SET active=0,consumed=1 WHERE id=?", note.ID); err != nil {
			return nil, err
		}
		task.AuthorizationRevision++
		touchTask(task)
		if err := saveTask(ctx, tx, *task); err != nil {
			return nil, err
		}
		return &note, nil
	}
	return nil, nil
}

func (s *Store) EndVerification(ctx context.Context, workspace string, actor Actor, tool *event.ToolResult) (VerificationEnd, error) {
	var result VerificationEnd
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		record, err := verificationByCall(ctx, tx, workspace, actor)
		if err != nil {
			return err
		}
		if record == nil {
			return sql.ErrNoRows
		}
		result.Record = *record
		if record.Denied {
			return nil
		}
		if record.Executed && (record.State == "passed" || record.State == "failed" || record.State == "interrupted") {
			return nil
		}
		beforeExecuted, beforeState := record.Executed, record.State
		record.Executed, record.State = true, VerificationState(tool)
		if tool != nil {
			record.ExitCode = tool.ExitCode
		}
		if record.State != "running" {
			record.EndedAt = time.Now().UTC()
		}
		if err := saveVerification(ctx, tx, *record); err != nil {
			return err
		}
		result.Record = *record
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM verifications_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? AND executed=1 AND denied=0", workspace, actor.Agent, actor.SessionID, record.Actor.RoundID).Scan(&result.Count); err != nil {
			return err
		}
		if result.Count < 7 || beforeExecuted && beforeState == record.State {
			return nil
		}
		var notified bool
		err = tx.QueryRowContext(ctx, "SELECT notified FROM verification_rounds_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=?", workspace, actor.Agent, actor.SessionID, record.Actor.RoundID).Scan(&notified)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		result.Notify = !notified || record.State == "failed" || record.UserRequested
		if result.Notify {
			_, err = tx.ExecContext(ctx, `INSERT INTO verification_rounds_v11 VALUES(?,?,?,?,1) ON CONFLICT(workspace_id,agent,session_id,round_id) DO UPDATE SET notified=1`, workspace, actor.Agent, actor.SessionID, record.Actor.RoundID)
		}
		return err
	})
	return result, err
}

func readVerifications(ctx context.Context, q queryer, workspace, task string) ([]VerificationRecord, error) {
	rows, err := q.QueryContext(ctx, "SELECT data FROM verifications_v11 WHERE workspace_id=? AND (?='' OR task_id=?) ORDER BY seq", workspace, task, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []VerificationRecord
	for rows.Next() {
		var data string
		var record VerificationRecord
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *Store) Verifications(ctx context.Context, workspace, task string) ([]VerificationRecord, error) {
	var result []VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		result, err = readVerifications(ctx, q, workspace, task)
		return err
	})
	return result, err
}
