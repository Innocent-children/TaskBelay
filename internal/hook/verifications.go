package hook

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func CommandScript(e event.Event) shell.Script {
	if e.Command == nil {
		return shell.Script{}
	}
	return shell.Parse(e.Command.Text, e.WorkingDir, e.Command.Shell, map[event.Shell]shell.Parser{event.POSIX: posix.Parser{}, event.PowerShell: pwsh.Parser{}})
}

func (t Tasks) BeforeCommand(ctx context.Context, e event.Event) (event.Decision, error) {
	script := CommandScript(e)
	if !script.HasVerification() {
		return event.Decision{}, nil
	}
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID, CallID: e.ToolCallID}
	requestHash := fmt.Sprintf("%x", sha256.Sum256([]byte(string(e.Command.Shell)+"\x00"+e.WorkingDir+"\x00"+e.Command.Text)))
	existing, err := t.Store.VerificationByCall(ctx, t.Workspace.ID, actor)
	if err != nil {
		return event.Decision{}, err
	}
	if existing != nil {
		if existing.RequestHash != requestHash {
			return event.Decision{}, fmt.Errorf("验证调用身份与原请求不一致")
		}
		return verificationDecision(*existing), nil
	}
	view, observation, err := t.observeEvent(ctx, e)
	if err != nil {
		return event.Decision{}, err
	}
	record := ledger.VerificationRecord{WorkspaceID: t.Workspace.ID, TaskID: view.CurrentTaskID, Actor: actor, RequestHash: requestHash, RequestDirectory: e.WorkingDir, Command: script, CommandKey: script.CommandKey(), Tree: observation.Tree}
	record.EnvironmentID = record.Command.HashEnvironment(os.Environ())
	if view.Task != nil {
		record.InputRevision = view.Task.RecordRevision
	}
	record.Scope.Reason = "该命令的 Runner 输入范围与外部环境尚未核对"
	analyzer := t.ScopeAnalyzer
	if analyzer == nil && view.Task != nil {
		analyzer = testrun.Scope{OriginTree: view.Task.OriginTree}
	}
	if analyzer != nil {
		record.Scope, err = analyzer.VerificationScope(ctx, t.Workspace, script, record.Tree)
		if err != nil {
			record.Scope.Complete = false
			record.Scope.Reason = "Runner 范围核对未完成"
		}
	}
	if e.Command.Background {
		for i := range record.Command.Segments {
			record.Command.Segments[i].Background = true
		}
	}
	previous, err := t.Store.PreviousVerification(ctx, t.Workspace.ID, actor, record.CommandKey)
	if err != nil {
		return event.Decision{}, err
	}
	if previous != nil {
		record.PreviousID = previous.ID
	}
	if record.Scope.Complete {
		current, captureErr := t.Workspace.Capture(ctx)
		if captureErr != nil || !current.Complete() || current.Tree != record.Tree {
			record.Scope.Complete = false
			record.Scope.Reason = "验证前内容发生变化或无法再次核对"
		}
	}
	record.Issue = check.Verification(record, previous)
	record, err = t.Store.BeginVerification(ctx, record)
	if err != nil {
		return event.Decision{}, err
	}
	return verificationDecision(record), nil
}

func verificationDecision(record ledger.VerificationRecord) event.Decision {
	text := check.FormatVerification(record)
	decision := event.Decision{AdditionalContext: text}
	if record.Denied {
		decision.DenyTool = text
	}
	return decision
}

func (t Tasks) AfterCommand(ctx context.Context, e event.Event) (event.Decision, error) {
	if e.Command == nil || !CommandScript(e).HasVerification() {
		return event.Decision{}, nil
	}
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID, CallID: e.ToolCallID}
	previousCall, err := t.Store.VerificationByCall(ctx, t.Workspace.ID, actor)
	if err != nil {
		return event.Decision{}, err
	}
	requestDirectory := e.WorkingDir
	if previousCall != nil && previousCall.RequestDirectory != "" {
		// shell 可以在执行中改变 cwd；请求身份仍以最初收到的目录核对。
		requestDirectory = previousCall.RequestDirectory
	}
	requestHash := fmt.Sprintf("%x", sha256.Sum256([]byte(string(e.Command.Shell)+"\x00"+requestDirectory+"\x00"+e.Command.Text)))
	if previousCall != nil && previousCall.RequestHash != requestHash {
		return event.Decision{}, fmt.Errorf("验证后置事件与原请求不一致")
	}
	result, err := t.Store.EndVerification(ctx, t.Workspace.ID, actor, e.Result)
	if errors.Is(err, sql.ErrNoRows) {
		// 前置预算不足时仍保存实际后置事实；不能倒推已缺失的命令前输入。
		script := CommandScript(e)
		view, viewErr := t.Store.TaskView(ctx, t.Workspace.ID, "", actor)
		if viewErr != nil {
			return event.Decision{}, viewErr
		}
		record := ledger.VerificationRecord{WorkspaceID: t.Workspace.ID, TaskID: view.CurrentTaskID, Actor: actor, Command: script, CommandKey: script.CommandKey(), RequestHash: requestHash, RequestDirectory: requestDirectory, PostOnly: true}
		record.EnvironmentID = record.Command.HashEnvironment(os.Environ())
		record.Scope.Reason = "没有可核对的前置记录，命令前输入未知"
		if view.Task != nil {
			record.InputRevision = view.Task.RecordRevision
		}
		previous, lookupErr := t.Store.PreviousVerification(ctx, t.Workspace.ID, actor, record.CommandKey)
		if lookupErr != nil {
			return event.Decision{}, lookupErr
		}
		if previous != nil {
			record.PreviousID = previous.ID
		}
		if _, err = t.Store.BeginVerification(ctx, record); err != nil {
			return event.Decision{}, err
		}
		result, err = t.Store.EndVerification(ctx, t.Workspace.ID, actor, e.Result)
	}
	if err != nil {
		return event.Decision{}, err
	}
	if !result.Notify {
		return event.Decision{}, nil
	}
	return event.Decision{AdditionalContext: fmt.Sprintf("[V3 提示] 本轮实际运行 %d 次验证；最近一条 %s：%s（%s）。可把相关改动合在一起验证；新的失败或用户要求仍按实际需要处理。", result.Count, result.Record.ID, result.Record.Command.Display, result.Record.State)}, nil
}
