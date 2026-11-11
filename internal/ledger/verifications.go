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

// recentVerifications 是判断两次相同验证之间有无其他操作时回看的记录数上限。
const recentVerifications = 256

var ErrVerificationChanged = errors.New("验证输入或上一条验证已变化，请按当前状态运行")

func verificationByCall(ctx context.Context, q queryer, workspace string, actor Actor) (*VerificationRecord, error) {
	var data string
	err := q.QueryRowContext(ctx, "SELECT data FROM verifications_v12 WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", workspace, actor.Agent, actor.SessionID, actor.CallID).Scan(&data)
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
	err := q.QueryRowContext(ctx, "SELECT data FROM verifications_v12 WHERE workspace_id=? AND command_key=? AND denied=0 AND json_extract(data,'$.algorithm') IS NOT NULL ORDER BY seq DESC LIMIT 1", workspace, key).Scan(&data)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO verifications_v12(id,workspace_id,task_id,agent,session_id,round_id,call_id,command_key,denied,executed,state,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET denied=excluded.denied,executed=excluded.executed,state=excluded.state,data=excluded.data`, record.ID, record.WorkspaceID, record.TaskID, record.Actor.Agent, record.Actor.SessionID, record.Actor.RoundID, record.Actor.CallID, record.CommandKey, record.Denied, record.Executed, record.State, string(data))
	return err
}

/**
 * 分析在事务外完成；这里只核对分析所用的前置信息、标记已使用的一次性授权，并保存同一工具调用的决定。
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
				check := CheckRecord{Input: CheckInput{TaskID: record.TaskID, Algorithm: record.Algorithm}, Issues: []Issue{*record.Issue}, Evaluated: []string{CheckScope("validation_repeat", record.CommandKey)}}
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
			record.State = "running"
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
		rows, err := tx.QueryContext(ctx, "SELECT data FROM verifications_v12 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? AND state='running'", workspace, actor.Agent, actor.SessionID, actor.RoundID)
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
		if _, err := tx.ExecContext(ctx, "UPDATE notes_v12 SET active=0,consumed=1 WHERE id=?", note.ID); err != nil {
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
		beforeExecuted, beforeState, beforeExit := record.Executed, record.State, record.ExitCode
		record.Executed, record.State = true, VerificationState(tool)
		if tool != nil {
			record.ExitCode = tool.ExitCode
			record.Output = tool.Output
		}
		result.Changed = !beforeExecuted || beforeState != record.State ||
			(beforeExit == nil) != (record.ExitCode == nil) ||
			beforeExit != nil && record.ExitCode != nil && *beforeExit != *record.ExitCode
		if record.State != "running" {
			record.EndedAt = time.Now().UTC()
		}
		if err := saveVerification(ctx, tx, *record); err != nil {
			return err
		}
		result.Record = *record
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM verifications_v12 WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? AND executed=1 AND denied=0", workspace, actor.Agent, actor.SessionID, record.Actor.RoundID).Scan(&result.Count); err != nil {
			return err
		}
		return nil
	})
	return result, err
}

func readVerifications(ctx context.Context, q queryer, workspace, task string) ([]VerificationRecord, error) {
	rows, err := q.QueryContext(ctx, "SELECT data FROM verifications_v12 WHERE workspace_id=? AND (?='' OR task_id=?) ORDER BY seq", workspace, task, task)
	if err != nil {
		return nil, err
	}
	return scanVerifications(rows)
}

func scanVerifications(rows *sql.Rows) ([]VerificationRecord, error) {
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

// IndexVerifications 只加载本版分析可能采用的运行。退出时间和只读命令仍由 runSamples 核对。
func (s *Store) IndexVerifications(ctx context.Context, workspace, task, algorithm string) ([]VerificationRecord, error) {
	var result []VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT data FROM verifications_v12
WHERE workspace_id=? AND task_id=? AND executed=1 AND json_extract(data,'$.algorithm')=?
 AND json_extract(data,'$.exit_code') IS NOT NULL ORDER BY seq`, workspace, task, algorithm)
		if err != nil {
			return err
		}
		result, err = scanVerifications(rows)
		return err
	})
	return result, err
}

/**
 * VerificationHistory 返回判断一条命令是否重复所需的记录：同一命令的全部运行，
 * 以及最近一次同命令运行之后的近期操作。多取一条判定间隔是否读全，输出在选中记录后再取。
 */
func (s *Store) VerificationHistory(ctx context.Context, workspace, key string) ([]VerificationRecord, bool, error) {
	var result []VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT json_remove(data,'$.output','$.issue') FROM (
 SELECT seq,data FROM verifications_v12 WHERE workspace_id=?1 AND command_key=?2
 UNION ALL
 SELECT seq,data FROM (SELECT seq,data FROM verifications_v12 WHERE workspace_id=?1 AND command_key<>?2 AND seq>COALESCE((SELECT MAX(seq) FROM verifications_v12 WHERE workspace_id=?1 AND command_key=?2 AND executed=1),0) ORDER BY seq DESC LIMIT ?3)
) ORDER BY seq`, workspace, key, recentVerifications+1)
		if err != nil {
			return err
		}
		result, err = scanVerifications(rows)
		return err
	})
	other := 0
	for _, r := range result {
		if r.CommandKey != key {
			other++
		}
	}
	return result, other > recentVerifications, err
}

/**
 * VerificationOutcomes 也返回已放行但尚无结果的记录，以免跨命令提示引用更早的成功。
 */
func (s *Store) VerificationOutcomes(ctx context.Context, workspace, algorithm string) ([]VerificationRecord, error) {
	var result []VerificationRecord
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT json_remove(data,'$.output','$.issue') FROM verifications_v12
WHERE workspace_id=? AND denied=0 AND json_extract(data,'$.algorithm')=? ORDER BY seq`, workspace, algorithm)
		if err != nil {
			return err
		}
		result, err = scanVerifications(rows)
		return err
	})
	return result, err
}

func (s *Store) VerificationOutput(ctx context.Context, id string) (string, error) {
	var output sql.NullString
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, "SELECT json_extract(data,'$.output') FROM verifications_v12 WHERE id=?", id).Scan(&output)
	})
	return output.String, err
}
