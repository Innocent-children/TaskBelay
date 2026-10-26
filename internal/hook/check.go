package hook

import (
	"context"
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func (t Tasks) Check(ctx context.Context, actor ledger.Actor, options check.RunInput) (record ledger.CheckRecord, failure error) {
	ctx, trace := t.Store.StartDebug(ctx, t.Workspace.ID, nil, actor, "check", t.ReportError)
	returned := false
	defer func() {
		if !returned && failure == nil {
			failure = fmt.Errorf("检查未正常返回，过程记录不完整")
		}
		trace.Finish(ctx, record, failure)
	}()
	view, observation, err := t.observeTask(ctx, actor)
	if err != nil {
		return ledger.CheckRecord{}, err
	}
	record, err = check.Run(ctx, t.Store, t.Workspace, actor, view, observation, options)
	t.reportCheck(record)
	returned = true
	return record, err
}

func (t Tasks) reportCheck(record ledger.CheckRecord) {
	if t.ReportError == nil {
		return
	}
	if record.Static != nil {
		t.reportCheck(*record.Static)
	}
	for _, gap := range record.Missing[:min(5, len(record.Missing))] {
		t.ReportError(fmt.Errorf("%s 检查未完成（任务 %s，%s %s）：%s", record.Depth, record.Input.TaskID, gap.Operation, gap.At, gap.Reason))
	}
	if len(record.Missing) > 5 {
		t.ReportError(fmt.Errorf("%s 检查另有 %d 条未完成记录（任务 %s）；原因和数量见 taskbelay status", record.Depth, len(record.Missing)-5, record.Input.TaskID))
	}
}

func (t Tasks) Finish(ctx context.Context, actor ledger.Actor, input ledger.FinishInput) (ledger.FinishResult, error) {
	if input.Outcome == "" {
		input.Outcome = "complete"
	}
	if input.Outcome != "complete" && input.Outcome != "cancel" {
		return ledger.FinishResult{}, fmt.Errorf("outcome 只能为 complete 或 cancel")
	}
	call, err := ledger.NewCall(t.Workspace.ID, actor, "task_finish", input)
	if err != nil {
		return ledger.FinishResult{}, err
	}
	var result ledger.FinishResult
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
	if view.Task == nil {
		return result, fmt.Errorf("当前没有任务")
	}
	if input.Outcome == "cancel" {
		if input.SourceID == "" && input.Quote == "" {
			for i := len(view.Messages) - 1; i >= 0; i-- {
				message := view.Messages[i]
				if message.Actor.Agent == actor.Agent && message.Actor.SessionID == actor.SessionID && message.Actor.RoundID == actor.RoundID {
					input.SourceID, input.Quote = message.ID, message.Text
					break
				}
			}
		}
		return t.Store.CancelFinish(ctx, call, view.Task.ID, input.SourceID, input.Quote)
	}
	version, err := t.Store.CheckInput(ctx, t.Workspace.ID, view.Task.ID, actor, check.RulesVersion, check.CapabilitiesVersion)
	if err != nil {
		return result, err
	}
	return t.Store.RequestFinish(ctx, call, version)
}

func (t Tasks) stop(ctx context.Context, e event.Event) (event.Decision, error) {
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID}
	if err := t.Store.UnfinishedVerifications(ctx, t.Workspace.ID, actor); err != nil {
		return event.Decision{}, err
	}
	view, observation, err := t.observeEvent(ctx, e)
	if err != nil {
		return event.Decision{}, err
	}
	request, err := t.Store.FinishRequest(ctx, t.Workspace.ID, actor)
	ledger.DebugRecord(ctx, "delivery.request", request)
	if err != nil {
		return event.Decision{}, err
	}
	if request == nil || request.State != "pending" {
		ledger.DebugRecord(ctx, "delivery.ordinary_stop", nil)
		return t.ordinaryStop(ctx, actor, view, request)
	}
	if view.Task == nil || view.Task.ID != request.TaskID || view.Task.State != ledger.TaskPending {
		return event.Decision{AdditionalContext: "TaskBelay：交付请求已过期，请按当前任务和代码继续。"}, nil
	}
	input, err := t.Store.CheckInput(ctx, t.Workspace.ID, request.TaskID, actor, check.RulesVersion, check.CapabilitiesVersion)
	if err != nil {
		return event.Decision{}, err
	}
	var record ledger.CheckRecord
	ledger.DebugRecord(ctx, "delivery.input", input)
	if input.Version() != request.InputVersion {
		record = ledger.CheckRecord{Input: request.Input, InputVersion: request.InputVersion, Status: ledger.CheckExpired}
	} else {
		var cached *ledger.CheckRecord
		if request.Feedback {
			cached, err = t.Store.LatestCheck(ctx, request.TaskID, "static", input.Version())
			if err != nil {
				return event.Decision{}, err
			}
		}
		if cached != nil {
			ledger.DebugRecord(ctx, "delivery.cache", map[string]any{"depth": "static", "record": cached, "reason": "本次交付已反馈且输入未变化"})
			record = *cached
		} else {
			record, err = check.Run(ctx, t.Store, t.Workspace, actor, view, observation, check.RunInput{Depth: "static"})
			if err != nil {
				return event.Decision{}, err
			}
			t.reportCheck(record)
		}
	}
	var dynamic *ledger.CheckRecord
	if view.Settings.Values["analysis.tests"].Value == "on" && record.Status != ledger.CheckExpired {
		if request.Feedback {
			dynamic, err = t.Store.LatestCheck(ctx, request.TaskID, "tests", input.Version())
			if err != nil {
				return event.Decision{}, err
			}
			if dynamic != nil && (dynamic.Report.Dynamic == nil || dynamic.Report.Dynamic.Rules != testrun.DynamicVersion) {
				dynamic = nil
			}
		}
		if dynamic == nil {
			ledger.DebugRecord(ctx, "delivery.dynamic.start", nil)
			checked, err := check.Run(ctx, t.Store, t.Workspace, actor, view, observation, check.RunInput{Depth: "tests"})
			if err != nil {
				return event.Decision{}, err
			}
			dynamic = &checked
			t.reportCheck(checked)
		}
		dynamic.Static = nil
		if dynamic.Status == ledger.CheckExpired {
			record.Status = ledger.CheckExpired
		}
	}
	current, err := t.Workspace.Capture(ctx)
	record.Report.Verifications = view.Verifications
	currentTree := current.Tree
	if err != nil || !current.Complete() {
		currentTree = ""
	}
	delivery, err := t.Store.Deliver(ctx, t.Workspace.ID, actor, currentTree, record)
	ledger.DebugRecord(ctx, "delivery.result", map[string]any{"current_tree": currentTree, "delivery": delivery, "dynamic": dynamic, "completed": err == nil})
	if err != nil {
		return event.Decision{}, err
	}
	if delivery.State == "expired" {
		return event.Decision{AdditionalContext: "TaskBelay：检查期间输入已变化，旧结果仅保留为历史。请读取当前内容，完成后重新申请交付。"}, nil
	}
	text := check.Format(delivery.Check, 5)
	if dynamic != nil {
		text += "\n" + check.Format(*dynamic, 5)
	}
	if record.Status == ledger.CheckPartial || record.Status == ledger.CheckFailed || dynamic != nil && (dynamic.Status == ledger.CheckPartial || dynamic.Status == ledger.CheckFailed) {
		text = "TaskBelay 这次没有完成全部检查；已完成的事实和具体原因如下。\n" + text
	}
	decision := event.Decision{AdditionalContext: text, UserMessage: text}
	if len(delivery.Blocked) > 0 {
		decision.BlockStop = check.FormatIssues(delivery.Blocked, 0)
	}
	return decision, nil
}

func (t Tasks) ordinaryStop(ctx context.Context, actor ledger.Actor, view ledger.TaskView, request *ledger.FinishRequest) (event.Decision, error) {
	var text []string
	if request != nil && request.State == "expired" {
		text = append(text, "TaskBelay：本轮交付请求已过期，完成当前修改后重新申请交付。")
	}
	if view.Task != nil {
		input, err := t.Store.CheckInput(ctx, t.Workspace.ID, view.Task.ID, actor, check.RulesVersion, check.CapabilitiesVersion)
		if err != nil {
			return event.Decision{}, err
		}
		cached, err := t.Store.LatestCheck(ctx, view.Task.ID, "static", input.Version())
		if err != nil {
			return event.Decision{}, err
		}
		if cached != nil {
			progress := ledger.WithDelivery(*cached, false)
			text = append(text, check.FormatIssues(progress.Issues, 5))
		}
	}
	active := false
	for _, req := range view.Requirements {
		active = active || req.Status != "revoked"
	}
	if !active {
		var since int64
		if view.Task != nil {
			since = view.Task.OriginObservationID
		} else {
			for _, message := range view.Messages {
				if message.Actor.Agent == actor.Agent && message.Actor.SessionID == actor.SessionID && message.Actor.RoundID == actor.RoundID {
					since = message.ObservationID
					break
				}
			}
		}
		changes, err := t.Store.Changes(ctx, t.Workspace.ID, since)
		if err != nil {
			return event.Decision{}, err
		}
		for _, change := range changes {
			if change.Kind == "agent_edit" {
				text = append(text, "TaskBelay：已有可核对的 Agent 编辑，尚未记录有效需求；请补记需求及实现位置。普通 Stop 未启动交付检查。")
				break
			}
		}
	}
	return event.Decision{AdditionalContext: strings.Join(text, "\n")}, nil
}
