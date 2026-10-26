package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/terminal"
)

const usage = `TaskBelay：记录需求和开发事实，帮助 Agent 完成当前任务。

用法：
  taskbelay                         打开交互菜单
  taskbelay menu
  taskbelay --version
  taskbelay doctor
  taskbelay update
  taskbelay uninstall --all [--yes]
  taskbelay install <claude|codex>
  taskbelay uninstall <claude|codex>
  taskbelay hook <claude|codex> <事件>
  taskbelay mcp
  taskbelay status
  taskbelay hits --since 14d
  taskbelay check [--tests] [--refresh]
  taskbelay config get <设置项>
  taskbelay config set <设置项> <值> [--project | --task]

设置项：
  workflow                 auto（默认）或 semi
  enforcement              block（默认）或 hint
  analysis.tests           off（默认）或 on
  debug                    off（默认）或 on，详细过程写入数据目录 debug/日期/任务编号.jsonl
  test.<语言>.command      测试命令或前缀，默认使用 Runner 的方式
  test.<语言>.prepare      临时副本的准备命令，默认无
  test.<语言>.parallel     1 至 4，默认 1

语言：go、python、javascript、typescript、java、rust。
默认保存用户设置；--project 保存项目设置；--task 保存当前任务设置。
get 显示当前工作区实际生效的值、来源和设置修订。
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	env, err := platform.Current()
	if err == nil {
		var dir string
		dir, err = os.Getwd()
		if err == nil {
			err = run(ctx, os.Args[1:], env, dir, os.Stdout)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "taskbelay:", err)
		if env.DataDir != "" {
			if logErr := logError(env, err); logErr != nil {
				fmt.Fprintln(os.Stderr, "无法写入 TaskBelay 日志:", logErr)
			}
		}
		if len(os.Args) > 1 && os.Args[1] == "hook" {
			if len(os.Args) == 4 && os.Args[3] == "stop" {
				if adapter, findErr := adapters.Find(os.Args[2]); findErr == nil {
					data, _ := adapter.RenderDecision("stop", event.Decision{UserMessage: "TaskBelay 这次没有完成检查：" + err.Error()})
					_, _ = os.Stdout.Write(append(data, '\n'))
				}
			}
			return
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, env platform.Env, dir string, output io.Writer) error {
	if len(args) == 0 && terminal.Available(os.Stdin, output) {
		return runMenu(ctx, env, dir, output)
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help")) {
		_, err := io.WriteString(output, usage)
		return err
	}
	if args[0] != "config" {
		if len(args) == 1 {
			switch args[0] {
			case "menu":
				return runMenu(ctx, env, dir, output)
			case "--version":
				_, err := fmt.Fprintln(output, versionText())
				return err
			case "doctor":
				return runDoctor(ctx, env, output)
			case "update":
				return runUpdate(ctx, env, output)
			}
		}
		if len(args) >= 2 && args[0] == "uninstall" && args[1] == "--all" {
			return runUninstallAll(ctx, args[2:], env, output)
		}
		if args[0] == "hook" {
			return runHook(ctx, args[1:], env, os.Stdin, output)
		}
		if args[0] == "install" || args[0] == "uninstall" {
			return runInstall(args, env, output)
		}
		if args[0] == "mcp" && len(args) == 1 {
			return runMCP(ctx, env, dir, os.Stdin, output)
		}
		if args[0] == "status" && len(args) == 1 {
			return runStatus(ctx, env, dir, output)
		}
		if args[0] == "hits" {
			return runHits(ctx, args[1:], env, dir, output)
		}
		if args[0] == "check" {
			return runCheck(ctx, args[1:], env, dir, output)
		}
		return fmt.Errorf("未知命令 %q；使用 taskbelay --help 查看用法", args[0])
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		_, err := io.WriteString(output, usage)
		return err
	}
	if len(args) < 3 {
		return fmt.Errorf("用法: taskbelay config get <设置项> 或 config set <设置项> <值> [--project | --task]")
	}
	action := args[1]
	var name, value string
	scope := ledger.User
	switch action {
	case "get":
		if len(args) != 3 {
			return fmt.Errorf("用法: taskbelay config get <设置项>")
		}
		name = args[2]
		if _, err := ledger.DefaultValue(name); err != nil {
			return err
		}
	case "set":
		var err error
		name, value, scope, err = parseSet(args[2:])
		if err != nil {
			return err
		}
		if err := ledger.ValidateSetting(name, value); err != nil {
			return err
		}
	default:
		return fmt.Errorf("未知 config 操作 %q", action)
	}

	ctx, cancel := context.WithTimeout(ctx, event.MCPBudget)
	defer cancel()
	var workspace gitstate.Workspace
	if action == "get" || scope != ledger.User {
		var err error
		workspace, err = gitstate.Discover(ctx, dir)
		if err != nil && !(action == "get" && errors.Is(err, gitstate.ErrNotWorkspace)) {
			return err
		}
	}
	store, err := ledger.Open(ctx, env.DataDir, ledger.NormalWait)
	if err != nil {
		return err
	}
	defer store.Close()
	if workspace.GitDir != "" {
		if err := store.RegisterGitWorkspace(ctx, workspace); err != nil {
			return err
		}
	}
	if action == "set" {
		if err := store.SetSetting(ctx, workspace.ID, scope, name, value); err != nil {
			return err
		}
		_, err := fmt.Fprintf(output, "已保存 %s = %s（%s设置）\n", name, displayValue(name, value), scopeName(scope))
		return err
	}
	settings, err := store.Settings(ctx, workspace.ID)
	if err != nil {
		return err
	}
	setting := settings.Values[name]
	source := scopeName(setting.Source)
	if setting.ScopeID != "" {
		source += " (" + setting.ScopeID + ")"
	}
	_, err = fmt.Fprintf(output, "%s = %s\n来源: %s\n设置修订: %s\n", name, displayValue(name, setting.Value), source, settings.Revision)
	return err
}

func parseSet(args []string) (string, string, ledger.Scope, error) {
	scope := ledger.User
	var positional []string
	for _, arg := range args {
		// 第二个位置参数是设置原文，即使恰好是 --task 也不当作选项。
		if len(positional) != 1 && (arg == "--project" || arg == "--task") {
			if scope != ledger.User {
				return "", "", "", fmt.Errorf("--project 和 --task 只能选择其中一个且不能重复")
			}
			scope = ledger.Project
			if arg == "--task" {
				scope = ledger.Task
			}
			continue
		}
		positional = append(positional, arg)
	}
	if len(positional) != 2 {
		return "", "", "", fmt.Errorf("用法: taskbelay config set <设置项> <值> [--project | --task]；命令值请用引号包裹")
	}
	return positional[0], positional[1], scope, nil
}

func scopeName(scope ledger.Scope) string {
	switch scope {
	case ledger.User:
		return "用户"
	case ledger.Project:
		return "项目"
	case ledger.Task:
		return "任务"
	default:
		return "内置默认"
	}
}

func displayValue(name, value string) string {
	if value == "" {
		if strings.HasSuffix(name, ".command") {
			return "\"\"（Runner 默认命令）"
		}
		return "\"\"（无）"
	}
	if strings.HasSuffix(name, ".command") || strings.HasSuffix(name, ".prepare") {
		return strconv.Quote(value)
	}
	return value
}
