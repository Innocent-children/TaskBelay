package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"
)

func readNotes(ctx context.Context, q queryer, task string) ([]Note, error) {
	rows, err := q.QueryContext(ctx, "SELECT data,active,consumed,invalidated_by FROM notes_v11 WHERE task_id=? ORDER BY seq", task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		var note Note
		var data string
		var active, consumed bool
		var invalid string
		if err := rows.Scan(&data, &active, &consumed, &invalid); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &note); err != nil {
			return nil, err
		}
		note.Active, note.Consumed, note.InvalidatedBy = active, consumed, invalid
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func invalidateNotes(ctx context.Context, tx *sql.Tx, task, reason string, match func(Note) bool) (bool, error) {
	notes, err := readNotes(ctx, tx, task)
	if err != nil {
		return false, err
	}
	changed := false
	for _, note := range notes {
		if !note.Active || note.Consumed || !match(note) {
			continue
		}
		if _, err := tx.ExecContext(ctx, "UPDATE notes_v11 SET active=0,invalidated_by=? WHERE id=?", reason, note.ID); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func (s *Store) AddNote(ctx context.Context, call Call, current string, observationID int64, input NoteInput) (Note, error) {
	var result Note
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if found, err := replay(ctx, tx, call, &result); found || err != nil {
			return err
		}
		if err := checkActor(ctx, tx, call); err != nil {
			return err
		}
		if current == "" {
			return fmt.Errorf("当前没有任务")
		}
		if err := checkCurrent(ctx, tx, call.WorkspaceID, current); err != nil {
			return err
		}
		if _, err := checkObservation(ctx, tx, call.WorkspaceID, observationID); err != nil {
			return err
		}
		if input.Kind != "decision" && input.Kind != "next_step" {
			return fmt.Errorf("笔记种类只能是 decision 或 next_step")
		}
		if strings.TrimSpace(input.Text) == "" {
			return fmt.Errorf("笔记内容不能为空")
		}
		task, err := loadTask(ctx, tx, call.WorkspaceID, current)
		if err != nil {
			return err
		}
		settings, err := effectiveSettings(ctx, tx, call.WorkspaceID, current, false)
		if err != nil {
			return err
		}
		task.SettingsRevision, task.Workflow = settings.Revision, settings.Values["workflow"].Value
		proof, err := citation(ctx, tx, call.WorkspaceID, current, input.SourceID, input.Quote)
		if err != nil {
			return err
		}
		result = Note{TaskID: current, Kind: input.Kind, Text: input.Text, Target: input.Target, Citation: proof, Author: call.Actor, At: time.Now().UTC(), ObservationID: observationID, InputRevision: task.RecordRevision, RequirementRevision: task.RequirementRevision, PlanRevision: task.PlanRevision, SettingsRevision: task.SettingsRevision}
		if target := input.Target; target != nil {
			if target.ID == "" {
				return fmt.Errorf("决定目标缺少编号")
			}
			switch target.Kind {
			case "task":
				if target.ID != current {
					return fmt.Errorf("决定只能指向当前任务")
				}
			case "requirement":
				requirement, err := latestRequirement(ctx, tx, current, target.ID)
				if err != nil {
					return fmt.Errorf("需求目标不存在: %w", err)
				}
				if requirement.Status == "revoked" && target.Effect == "retain" {
					return fmt.Errorf("需求已撤销，须先核对新的有效需求")
				}
			case "verification":
				var count int
				if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM verifications_v11 WHERE id=? AND workspace_id=? AND task_id=? AND denied=0", target.ID, call.WorkspaceID, current).Scan(&count); err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("验证目标不存在、未允许运行或不属于当前任务")
				}
			case "plan":
				plans, err := readPlans(ctx, tx, task)
				if err != nil {
					return err
				}
				found := false
				for _, plan := range plans.Versions {
					found = found || plan.ID == target.ID
				}
				if !found {
					return fmt.Errorf("方案目标不存在或不属于当前任务")
				}
			case "issue":
			default:
				return fmt.Errorf("未知决定目标")
			}
			if target.Effect != "" && input.Kind != "decision" {
				return fmt.Errorf("下一步笔记不能授予决定权限")
			}
			switch target.Effect {
			case "":
			case "allow_edit":
				if proof == nil || target.Kind != "task" || target.Scope != "task" || target.At == "" {
					return fmt.Errorf("分析草稿授权需要用户消息、当前任务、具体文件或目录及任务期限")
				}
				source, err := loadSource(ctx, tx, call.WorkspaceID, proof.SourceID)
				if err != nil {
					return err
				}
				clean := path.Clean(strings.ReplaceAll(target.At, "\\", "/"))
				if source.Kind != "user_message" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.ContainsAny(clean, ":\x00") {
					return fmt.Errorf("分析草稿授权须来自用户消息且不能覆盖整个工作区或工作区外路径")
				}
				copy := *target
				copy.At = clean
				if strings.HasSuffix(target.At, "/") || strings.HasSuffix(target.At, "\\") {
					copy.At += "/"
				}
				result.Target = &copy
				result.Active = true
			case "retain":
				if proof == nil || target.At == "" || target.Kind == "task" || target.Kind == "verification" || target.Scope != "task" {
					return fmt.Errorf("保留决定需要来源、具体位置及任务期限")
				}
				result.Active = true
			case "pause_checks":
				if proof == nil || target.Kind != "task" || (target.Scope != "round" && target.Scope != "task") {
					return fmt.Errorf("暂停检查需要用户来源、当前任务和 round 或 task 期限")
				}
				result.Active = true
			case "rerun_once":
				if target.Kind != "verification" || target.Scope != "once" {
					return fmt.Errorf("重跑授权必须指向一次验证，期限为 once")
				}
				result.Active = true
			case "revoke":
				if proof == nil {
					return fmt.Errorf("撤回决定需要用户来源")
				}
				if target.Kind == "plan" {
					if err := revokePlan(ctx, tx, &task, *target, *proof); err != nil {
						return err
					}
				}
				changed, err := invalidateNotes(ctx, tx, current, "user_revoke", func(note Note) bool {
					return note.Target != nil && note.Target.Kind == target.Kind && note.Target.ID == target.ID && note.Target.At == target.At && (target.Scope == "" || note.Target.Scope == target.Scope)
				})
				if err != nil {
					return err
				}
				if changed {
					task.AuthorizationRevision++
					touchTask(&task)
				}
			default:
				return fmt.Errorf("未知决定作用")
			}
			if result.Active && proof != nil {
				source, err := loadSource(ctx, tx, call.WorkspaceID, proof.SourceID)
				if err != nil {
					return err
				}
				if source.Kind != "user_message" && source.Kind != "confirmed_plan" {
					return fmt.Errorf("用户决定不能由项目约定代替")
				}
				if source.Kind == "confirmed_plan" {
					plans, err := readPlans(ctx, tx, task)
					if err != nil {
						return err
					}
					for _, plan := range plans.Versions {
						if plan.Confirmation == nil || plan.Confirmation.SourceID != source.ID {
							continue
						}
						for _, revoked := range plan.Revocations {
							if revokedPosition(revoked.At, target.At) {
								return fmt.Errorf("决定引用的方案范围已撤回，请核对新的有效用户来源")
							}
						}
					}
				}
			}
		}
		insert, err := tx.ExecContext(ctx, "INSERT INTO notes_v11(task_id,active,data) VALUES (?,?,?)", current, result.Active, "")
		if err != nil {
			return err
		}
		seq, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		result.ID = fmt.Sprintf("n%d", seq)
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE notes_v11 SET id=?,data=? WHERE seq=?", result.ID, string(data), seq); err != nil {
			return err
		}
		changedDecision := result.Active
		if changedDecision {
			notes, err := readNotes(ctx, tx, current)
			if err != nil {
				return err
			}
			for _, old := range effectiveDecisions(notes, task, settings, call.Actor) {
				if old.ID != result.ID && decisionKey(old) == decisionKey(result) {
					changedDecision = false
					break
				}
			}
		}
		if changedDecision {
			task.AuthorizationRevision++
			touchTask(&task)
		} else {
			task.Revision++
			task.UpdatedAt = result.At
		}
		if err := saveTask(ctx, tx, task); err != nil {
			return err
		}
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}

func expireRoundDecisions(ctx context.Context, tx *sql.Tx, message Message) error {
	tasks, err := taskList(ctx, tx, message.WorkspaceID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		changed, err := invalidateNotes(ctx, tx, task.ID, "round:"+message.Actor.RoundID, func(note Note) bool {
			return note.Target != nil && note.Target.Scope == "round" && note.Author.Agent == message.Actor.Agent && note.Author.SessionID == message.Actor.SessionID && note.Author.RoundID != message.Actor.RoundID
		})
		if err != nil {
			return err
		}
		if changed {
			task.AuthorizationRevision++
			touchTask(&task)
			if err := saveTask(ctx, tx, task); err != nil {
				return err
			}
		}
	}
	return nil
}

/**
 * 相同验证编号代表同一命令及环境输入，由验证模块核对；同时核对任务输入版本。
 * 工具重试返回同一次消费结果，不再消耗另一条授权。
 */
func (s *Store) ConsumeRerun(ctx context.Context, call Call, taskID, verification string) (Note, error) {
	var result Note
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if found, err := replay(ctx, tx, call, &result); found || err != nil {
			return err
		}
		if err := checkActor(ctx, tx, call); err != nil {
			return err
		}
		if err := checkCurrent(ctx, tx, call.WorkspaceID, taskID); err != nil {
			return err
		}
		task, err := loadTask(ctx, tx, call.WorkspaceID, taskID)
		if err != nil {
			return err
		}
		settings, err := effectiveSettings(ctx, tx, call.WorkspaceID, taskID, false)
		if err != nil {
			return err
		}
		notes, err := readNotes(ctx, tx, taskID)
		if err != nil {
			return err
		}
		for _, note := range notes {
			if !note.Active || note.Consumed || note.Target == nil || note.Target.Effect != "rerun_once" || note.Target.ID != verification {
				continue
			}
			if note.InputRevision != task.RecordRevision || note.RequirementRevision != task.RequirementRevision || note.PlanRevision != task.PlanRevision || note.SettingsRevision != settings.Revision {
				continue
			}
			note.Active, note.Consumed = false, true
			if _, err := tx.ExecContext(ctx, "UPDATE notes_v11 SET active=0,consumed=1 WHERE id=?", note.ID); err != nil {
				return err
			}
			task.AuthorizationRevision++
			touchTask(&task)
			if err := saveTask(ctx, tx, task); err != nil {
				return err
			}
			result = note
			return saveCall(ctx, tx, call, result)
		}
		return fmt.Errorf("没有适用于当前输入和该次验证的未使用授权")
	})
	return result, err
}
