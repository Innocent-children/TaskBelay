package hook

import (
	"context"
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/plan"
)

func (t Tasks) SavePlan(ctx context.Context, actor ledger.Actor, input ledger.PlanInput) (ledger.PlanResult, error) {
	call, err := ledger.NewCall(t.Workspace.ID, actor, "plan_save", input)
	if err != nil {
		return ledger.PlanResult{}, err
	}
	var result ledger.PlanResult
	if found, err := t.Store.Replay(ctx, call, &result); found || err != nil {
		return result, err
	}
	if err := t.Store.ValidateSession(ctx, t.Workspace.ID, actor); err != nil {
		return result, err
	}
	view, observation, err := t.observeTask(ctx, actor)
	if err != nil {
		return result, err
	}
	if view.Settings.Values["workflow"].Value != "semi" {
		return t.Store.SaveAutoPlan(ctx, call, view.CurrentTaskID, autoPlan(view))
	}
	if view.Task == nil {
		return result, fmt.Errorf("保存方案前先建立任务")
	}
	if input.BaseRevision != view.Plans.Revision {
		return result, &ledger.PlanConflict{Latest: view.Plans}
	}
	record, err := plan.Prepare(view, input, t.Store.DataDir())
	if err != nil {
		return result, err
	}
	if err := t.Store.StagePlanCleanup(ctx, t.Workspace.ID, record); err != nil {
		return result, err
	}
	publish, cleanup, err := plan.Stage(record)
	if err != nil {
		return result, err
	}
	defer cleanup()
	return t.Store.SavePlan(ctx, call, view.Task.ID, observation.ID, input.BaseRevision, record, publish)
}

func (t Tasks) ConfirmPlan(ctx context.Context, actor ledger.Actor, input ledger.PlanConfirmInput) (ledger.PlanResult, error) {
	call, err := ledger.NewCall(t.Workspace.ID, actor, "plan_confirm", input)
	if err != nil {
		return ledger.PlanResult{}, err
	}
	var result ledger.PlanResult
	if found, err := t.Store.Replay(ctx, call, &result); found || err != nil {
		return result, err
	}
	if err := t.Store.ValidateSession(ctx, t.Workspace.ID, actor); err != nil {
		return result, err
	}
	view, _, err := t.observeTask(ctx, actor)
	if err != nil {
		return result, err
	}
	if view.Settings.Values["workflow"].Value != "semi" {
		return t.Store.SaveAutoPlan(ctx, call, view.CurrentTaskID, autoPlan(view))
	}
	if view.Task == nil {
		return result, fmt.Errorf("当前没有可确认方案的任务")
	}
	return t.Store.ConfirmPlan(ctx, call, view.Task.ID, input)
}

func autoPlan(view ledger.TaskView) ledger.PlanResult {
	var requirements []ledger.Requirement
	for _, requirement := range view.Requirements {
		if requirement.Status != "revoked" {
			requirements = append(requirements, requirement)
		}
	}
	return ledger.PlanResult{Workflow: view.Settings.Values["workflow"].Value, View: view.Plans, Requirements: requirements, Message: "全自动模式，继续按有效需求开发，无需方案确认。"}
}
