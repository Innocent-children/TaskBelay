package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func readPlans(ctx context.Context, q queryer, task TaskRecord) (PlanView, error) {
	result := PlanView{Revision: task.PlanRevision}
	rows, err := q.QueryContext(ctx, "SELECT data FROM plans WHERE task_id=? ORDER BY version", task.ID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var data string
		var plan PlanRecord
		if err := rows.Scan(&data); err != nil {
			return result, err
		}
		if err := json.Unmarshal([]byte(data), &plan); err != nil {
			return result, err
		}
		result.Versions = append(result.Versions, plan)
		if plan.Confirmation != nil {
			// 最新的完整方案被撤回时，旧版历史不能重新授予同一范围。
			result.Effective = effectivePlan(plan)
			result.Pending = nil
		} else {
			result.Pending = effectivePlan(plan)
		}
	}
	if result.Pending != nil && result.Effective != nil && result.Pending.Version < result.Effective.Version {
		result.Pending = nil
	}
	return result, rows.Err()
}

func revokePlan(ctx context.Context, tx *sql.Tx, task *TaskRecord, target DecisionTarget, proof Citation) error {
	source, err := loadSource(ctx, tx, task.WorkspaceID, proof.SourceID)
	if err != nil {
		return err
	}
	if source.Kind != "user_message" {
		return fmt.Errorf("撤回方案需要用户消息来源")
	}
	view, err := readPlans(ctx, tx, *task)
	if err != nil {
		return err
	}
	for _, plan := range view.Versions {
		if plan.ID != target.ID {
			continue
		}
		found := target.At == ""
		for _, file := range plan.Files {
			found = found || file.Path == target.At
		}
		for _, entry := range plan.Entries {
			found = found || entry.At == target.At
		}
		for _, dep := range plan.Dependencies {
			found = found || "dep:"+dep.Module == target.At
		}
		if !found {
			return fmt.Errorf("撤回范围须对应该方案中列出的完整范围、入口或 dep:模块名")
		}
		plan.Revocations = append(plan.Revocations, PlanRevocation{At: target.At, Citation: proof, Time: source.At})
		if err := savePlanRow(ctx, tx, plan); err != nil {
			return err
		}
		if plan.Confirmation != nil {
			_, err := invalidateNotes(ctx, tx, task.ID, "plan_revoke", func(note Note) bool {
				return note.Citation != nil && note.Citation.SourceID == plan.Confirmation.SourceID && (target.At == "" || note.Target != nil && revokedPosition(target.At, note.Target.At))
			})
			if err != nil {
				return err
			}
		}
		task.PlanRevision++
		task.AuthorizationRevision++
		touchTask(task)
		return nil
	}
	return fmt.Errorf("方案目标不存在")
}

func revokedPosition(scope, at string) bool {
	return scope == "" || scope == at || strings.HasSuffix(scope, "/") && strings.HasPrefix(at, scope) || !strings.Contains(scope, ":") && strings.HasPrefix(at, scope+":")
}

func effectivePlan(plan PlanRecord) *PlanRecord {
	plan.Files = append([]PlanFile(nil), plan.Files...)
	plan.Entries = append([]PlanEntry(nil), plan.Entries...)
	plan.Dependencies = append([]PlanDependency(nil), plan.Dependencies...)
	for _, revocation := range plan.Revocations {
		if revocation.At == "" {
			return nil
		}
		files := plan.Files[:0]
		for _, file := range plan.Files {
			if file.Path != revocation.At {
				files = append(files, file)
			}
		}
		plan.Files = files
		entries := plan.Entries[:0]
		for _, entry := range plan.Entries {
			if entry.At != revocation.At {
				entries = append(entries, entry)
			}
		}
		plan.Entries = entries
		deps := plan.Dependencies[:0]
		for _, dep := range plan.Dependencies {
			if "dep:"+dep.Module != revocation.At {
				deps = append(deps, dep)
			}
		}
		plan.Dependencies = deps
	}
	return &plan
}

func (view PlanView) At(at time.Time) *PlanRecord {
	var result *PlanRecord
	for _, plan := range view.Versions {
		if plan.Confirmation == nil || plan.Confirmation.AuthorizedAt.After(at) {
			continue
		}
		revocations := []PlanRevocation{}
		for _, revoke := range plan.Revocations {
			if !revoke.Time.After(at) {
				revocations = append(revocations, revoke)
			}
		}
		plan.Revocations = revocations
		result = effectivePlan(plan)
	}
	return result
}

func savePlanRow(ctx context.Context, tx *sql.Tx, plan PlanRecord) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO plans VALUES(?,?,?) ON CONFLICT(task_id,version) DO UPDATE SET data=excluded.data`, plan.TaskID, plan.Version, string(data))
	return err
}

/**
 * 正文在事务外写完。publish 只执行同目录的文件发布；版本、来源和调用记录
 * 在同一短事务提交，文件发布失败时不会出现可见的半成品方案。
 */
func (s *Store) SavePlan(ctx context.Context, call Call, taskID string, observationID int64, base int64, plan PlanRecord, publish func() error) (PlanResult, error) {
	var result PlanResult
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
		result.Workflow = settings.Values["workflow"].Value
		view, err := readPlans(ctx, tx, task)
		if err != nil {
			return err
		}
		if result.Workflow != "semi" {
			result.View = view
			result.Message = "全自动模式，继续按有效需求开发，无需方案确认。"
			return saveCall(ctx, tx, call, result)
		}
		if base != task.PlanRevision {
			return &PlanConflict{Latest: view}
		}
		if _, err := checkObservation(ctx, tx, call.WorkspaceID, observationID); err != nil {
			return err
		}
		version := int64(1)
		if len(view.Versions) > 0 {
			version = view.Versions[len(view.Versions)-1].Version + 1
		}
		if plan.Version != version || plan.TaskID != taskID || plan.Path == "" || strings.TrimSpace(plan.Content) == "" {
			return fmt.Errorf("方案文件或版本不完整")
		}
		plan.Author, plan.SubmittedAt, plan.ObservationID = call.Actor, time.Now().UTC(), observationID
		var messageID string
		err = tx.QueryRowContext(ctx, "SELECT id FROM messages WHERE workspace_id=? AND agent=? AND session_id=? AND round_id=? ORDER BY seq DESC LIMIT 1", call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID, call.Actor.RoundID).Scan(&messageID)
		if err != nil {
			return err
		}
		plan.RequestMessageID = messageID
		task.PlanRevision++
		plan.Revision = task.PlanRevision
		touchTask(&task)
		if err := savePlanRow(ctx, tx, plan); err != nil {
			return err
		}
		if err := saveTask(ctx, tx, task); err != nil {
			return err
		}
		if publish == nil {
			return fmt.Errorf("方案文件尚未准备")
		}
		if err := publish(); err != nil {
			return err
		}
		result.Plan = &plan
		result.View, err = readPlans(ctx, tx, task)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM cleanup_items WHERE kind='plan' AND target=? AND expected=?", plan.Path, contentHash(plan.Content)); err != nil {
			return err
		}
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}

func (s *Store) ConfirmPlan(ctx context.Context, call Call, taskID string, input PlanConfirmInput) (PlanResult, error) {
	var result PlanResult
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
		result.Workflow = settings.Values["workflow"].Value
		view, err := readPlans(ctx, tx, task)
		if err != nil {
			return err
		}
		result.View = view
		if result.Workflow != "semi" {
			result.Message = "全自动模式，继续按有效需求开发，无需方案确认。"
			return saveCall(ctx, tx, call, result)
		}
		var plan *PlanRecord
		for i := range view.Versions {
			if view.Versions[i].Version == input.Version {
				copy := view.Versions[i]
				plan = &copy
				break
			}
		}
		if plan == nil {
			return fmt.Errorf("方案版本不存在")
		}
		if effectivePlan(*plan) == nil {
			return fmt.Errorf("该版方案已撤回，请依据当前范围重新提交")
		}
		if plan.Confirmation != nil {
			result.Plan = plan
			return saveCall(ctx, tx, call, result)
		}
		proof, err := citation(ctx, tx, call.WorkspaceID, taskID, input.SourceID, input.Quote)
		if err != nil {
			return err
		}
		if proof == nil {
			return fmt.Errorf("方案确认需要用户消息及原话引用")
		}
		source, err := loadSource(ctx, tx, call.WorkspaceID, proof.SourceID)
		if err != nil {
			return err
		}
		if source.Kind != "user_message" {
			return fmt.Errorf("方案确认必须来自用户消息")
		}
		var data string
		if err := tx.QueryRowContext(ctx, "SELECT data FROM messages WHERE workspace_id=? AND id=?", call.WorkspaceID, input.SourceID).Scan(&data); err != nil {
			return err
		}
		var message Message
		if err := json.Unmarshal([]byte(data), &message); err != nil {
			return err
		}
		method := "reply"
		if !message.At.After(plan.SubmittedAt) {
			if message.ID != plan.RequestMessageID || message.Actor.Agent != plan.Author.Agent || message.Actor.SessionID != plan.Author.SessionID || message.Actor.RoundID != plan.Author.RoundID {
				return fmt.Errorf("确认消息早于该方案，且不是提交方案时的本次请求")
			}
			// 随请求的例外只接受明确执行措辞；语义和附带限制仍由 Agent 核对完整原话。
			if !explicitPlanExecution(message.Text) || !explicitPlanExecution(input.Quote) {
				return fmt.Errorf("提交前的消息没有可核对的随请求执行授权")
			}
			method = "with_request"
		}
		if negativePlanConfirmation(message.Text) {
			return fmt.Errorf("消息含有不确认或继续分析的限制，请保留待确认状态")
		}
		task.PlanRevision++
		task.AuthorizationRevision++
		touchTask(&task)
		confirmed := PlanConfirmation{Citation: *proof, Message: message, Method: method, AuthorizedAt: message.At, RecordedAt: time.Now().UTC(), Author: call.Actor, SourceID: "confirmed-" + plan.ID, Revision: task.PlanRevision}
		plan.Confirmation = &confirmed
		if err := putSource(ctx, tx, Source{ID: confirmed.SourceID, WorkspaceID: call.WorkspaceID, TaskID: taskID, Kind: "confirmed_plan", Text: plan.Content, Path: plan.Path, Version: plan.Version, ObservationID: plan.ObservationID, Confirmation: proof, At: confirmed.AuthorizedAt}); err != nil {
			return err
		}
		if err := savePlanRow(ctx, tx, *plan); err != nil {
			return err
		}
		if err := saveTask(ctx, tx, task); err != nil {
			return err
		}
		result.Plan = plan
		result.View, err = readPlans(ctx, tx, task)
		if err != nil {
			return err
		}
		result.Message = "已保留完整确认消息及其中的附带要求；按原话调整方案或需求，不能只采用肯定部分。"
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}

func explicitPlanExecution(text string) bool {
	for _, phrase := range []string{"直接改", "直接做", "直接实现", "开始开发", "开始实现", "按这个做", "按方案做", "go ahead", "implement it", "proceed with"} {
		if strings.Contains(strings.ToLower(text), phrase) {
			return true
		}
	}
	return false
}

func negativePlanConfirmation(text string) bool {
	for _, phrase := range []string{"不确认方案", "不同意方案", "不能开始", "先不要开始", "先不要改", "不要开始", "暂不确认", "继续分析", "仅分析", "只分析", "先讨论", "还没确认", "do not implement", "don't implement", "not approved", "keep analyzing"} {
		if strings.Contains(strings.ToLower(text), phrase) {
			return true
		}
	}
	trimmed := strings.Trim(strings.TrimSpace(text), "。.!！")
	if trimmed == "不确认" || trimmed == "不同意" || trimmed == "不可以" {
		return true
	}
	return false
}

func (s *Store) SaveAutoPlan(ctx context.Context, call Call, taskID string, result PlanResult) (PlanResult, error) {
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
		settings, err := effectiveSettings(ctx, tx, call.WorkspaceID, taskID, false)
		if err != nil {
			return err
		}
		if settings.Values["workflow"].Value != "auto" {
			return fmt.Errorf("工作模式已变化，请重新读取当前任务")
		}
		return saveCall(ctx, tx, call, result)
	})
	return result, err
}
