package hook

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

/**
 * Handler 只处理统一事件；普通 Stop 保存观察，只有本轮交付请求才进入检查。
 */
func (t Tasks) Handle(ctx context.Context, e event.Event, raw []byte) (event.Decision, error) {
	var decision event.Decision
	err := t.HandleOutput(ctx, e, raw, func(d event.Decision) error {
		decision = d
		return nil
	})
	return decision, err
}

/**
 * 输出成功后才记录摘要交付；渲染或写 stdout 失败时，下次事件仍可补发。
 * 具体 Agent 的编码和输出由 CLI 回调完成。
 */
func (t Tasks) HandleOutput(ctx context.Context, e event.Event, raw []byte, output func(event.Decision) error) error {
	if e.Kind == event.MCPUse {
		// MCP 前置事件只登记身份，不把工具的业务参数重复保存到 hook 日志。
		raw = nil
	}
	id, err := t.Store.BeginHook(ctx, t.Workspace.ID, e, raw)
	if err != nil {
		return fmt.Errorf("登记 hook 开始: %w", err)
	}
	var before int64
	if e.Kind == event.ToolEnd {
		state, readErr := t.Store.ObservationState(ctx, t.Workspace.ID, []string{})
		err, before = readErr, state.LatestID
		if err != nil {
			err = fmt.Errorf("读取后置事件观察: %w", err)
		}
	}
	var d event.Decision
	if err == nil {
		d, err = t.dispatch(ctx, e)
		if err != nil {
			err = fmt.Errorf("处理 %s 事件: %w", e.Kind, err)
		}
	}
	var delivery *summaryDelivery
	needsSummary := e.Kind == event.SessionStart || e.Kind == event.UserPrompt
	if err == nil && e.Kind == event.ToolEnd {
		var changes []ledger.FileChange
		changes, err = t.Store.Changes(ctx, t.Workspace.ID, before)
		if err != nil {
			err = fmt.Errorf("读取后置工作区更新: %w", err)
		}
		for _, change := range changes {
			if change.Kind == "workspace_update" {
				needsSummary = true
				break
			}
		}
	}
	if err == nil && needsSummary {
		var text string
		text, delivery, err = t.prepareSummary(ctx, e)
		if err != nil {
			err = fmt.Errorf("准备接续摘要: %w", err)
		}
		d.AdditionalContext = strings.TrimSpace(d.AdditionalContext + "\n" + text)
	}
	if err == nil {
		err = output(d)
		if err != nil {
			err = fmt.Errorf("输出 hook 决定: %w", err)
		}
	}
	if err == nil && delivery != nil {
		err = t.Store.SaveSummaryState(ctx, t.Workspace.ID, ledger.Actor{Agent: e.Agent, SessionID: e.SessionID}, delivery.view, delivery.state)
		if err != nil {
			err = fmt.Errorf("保存接续摘要交付状态: %w", err)
		}
	}
	finishErr := t.Store.FinishHook(ctx, id, d, err)
	if finishErr != nil {
		finishErr = fmt.Errorf("登记 hook 结束: %w", finishErr)
	}
	return errors.Join(err, finishErr)
}

func (t Tasks) dispatch(ctx context.Context, e event.Event) (event.Decision, error) {
	var d event.Decision
	if e.Kind == event.MCPUse {
		return d, t.Store.BindMCPContext(ctx, t.Workspace.ID, ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID, CallID: e.ToolCallID}, e.MCPTool)
	}
	task, err := t.Store.CurrentTaskID(ctx, t.Workspace.ID)
	if err != nil {
		return d, err
	}
	switch e.Kind {
	case event.UserPrompt:
		_, err = t.RecordUserMessage(ctx, e)
	case event.SessionStart:
		_, _, err = t.observeEvent(ctx, e)
	case event.Stop:
		d, err = t.stop(ctx, e)
	case event.EditUse:
		var settings ledger.EffectiveSettings
		settings, err = t.Store.Settings(ctx, t.Workspace.ID)
		if err == nil {
			d, err = t.BeforeEdit(ctx, e, task, settings.Values["enforcement"].Value)
		}
	case event.CommandUse:
		d, err = t.BeforeCommand(ctx, e)
	case event.ToolEnd:
		if e.Command != nil {
			d, err = t.AfterCommand(ctx, e)
		}
		if err != nil {
			break
		}
		if len(e.Edits) > 0 {
			_, err = t.AfterEdit(ctx, e, task)
			if errors.Is(err, sql.ErrNoRows) {
				_, err = t.Observe(ctx, e, task)
			}
		} else {
			_, err = t.Observe(ctx, e, task)
		}
	default:
		err = fmt.Errorf("不支持统一事件 %q", e.Kind)
	}
	return d, err
}
