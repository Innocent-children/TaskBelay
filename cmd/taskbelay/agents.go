package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

var adapters = agent.Registry{"claude": claude.Adapter{}, "codex": codex.Adapter{}}

func runInstall(args []string, env platform.Env, output io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("用法: taskbelay %s <claude|codex>", args[0])
	}
	a, err := adapters.Find(args[1])
	if err != nil {
		return err
	}
	var message string
	if args[0] == "uninstall" {
		message, err = a.Uninstall(env)
	} else {
		var exe string
		exe, err = os.Executable()
		if err != nil {
			return err
		}
		exe, err = filepath.Abs(exe)
		if err != nil {
			return err
		}
		message, err = a.Install(env, exe)
	}
	_, writeErr := io.WriteString(output, message)
	return errors.Join(err, writeErr)
}

func runHook(ctx context.Context, args []string, env platform.Env, input io.Reader, output io.Writer) error {
	var adapter agent.Adapter
	var hookName string
	var workspaceID string
	if len(args) == 2 {
		hookName = args[1]
	}
	d, err := func() (event.Decision, error) {
		if len(args) != 2 {
			return event.Decision{}, fmt.Errorf("用法: taskbelay hook <claude|codex> <事件>")
		}
		var err error
		adapter, err = adapters.Find(args[0])
		if err != nil {
			return event.Decision{}, err
		}
		budget, wait := event.MessageBudget, ledger.NormalWait
		started := time.Now()
		if hookName == "pre-tool-use" {
			budget, wait = event.FastBudget, ledger.FastWait
		} else if hookName == "stop" {
			budget = event.StaticBudget + event.TestsBudget
		}
		data, err := io.ReadAll(input)
		if err != nil {
			return event.Decision{}, fmt.Errorf("读取 hook 输入: %w", err)
		}
		e, err := adapter.ParseEvent(hookName, data)
		if err != nil {
			return event.Decision{}, fmt.Errorf("解析 hook 事件: %w", err)
		}
		if e.Kind == event.EditUse {
			budget = event.EditBudget
		} else if e.Kind == event.CommandUse && hook.CommandScript(e).HasVerification() {
			budget, wait = event.MessageBudget, ledger.NormalWait
		}
		ctx, cancel := context.WithDeadline(ctx, started.Add(budget))
		defer cancel()
		workspace, err := gitstate.Discover(ctx, e.WorkingDir)
		if errors.Is(err, gitstate.ErrNotWorkspace) {
			return event.Decision{}, nil
		}
		if err != nil {
			return event.Decision{}, fmt.Errorf("定位工作区: %w", err)
		}
		store, err := ledger.Open(ctx, env.DataDir, wait)
		if err != nil {
			return event.Decision{}, fmt.Errorf("打开账本: %w", err)
		}
		defer store.Close()
		if err := store.RegisterGitWorkspace(ctx, workspace); err != nil {
			return event.Decision{}, fmt.Errorf("登记工作区: %w", err)
		}
		workspaceID = workspace.ID
		service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: workspace}, ReportError: func(cause error) { _ = logWorkspaceError(env, workspace.ID, cause) }}
		err = service.HandleOutput(ctx, e, data, func(d event.Decision) error {
			encoded, err := adapter.RenderDecision(hookName, d)
			if err == nil && len(encoded) > 0 {
				_, err = output.Write(append(encoded, '\n'))
			}
			return err
		})
		if err == nil && e.Kind == event.SessionStart {
			maintain(ctx, service, env, event.FastBudget)
		}
		return event.Decision{}, err
	}()
	if err != nil {
		_ = logWorkspaceError(env, workspaceID, fmt.Errorf("hook %s: %w", hookName, err))
		d = event.Decision{}
		if hookName == "stop" {
			d.UserMessage = "TaskBelay 这次没有完成检查：" + err.Error()
		}
	}
	if adapter != nil {
		data, renderErr := adapter.RenderDecision(hookName, d)
		if renderErr != nil {
			_ = logError(env, renderErr)
		} else if len(data) > 0 {
			if _, err := output.Write(append(data, '\n')); err != nil {
				_ = logError(env, err)
			}
		}
	}
	// 自身失败不能覆盖 Agent 的原权限判断，也不能使原工具失败。
	return nil
}
