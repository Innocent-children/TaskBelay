package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/distribution"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/terminal"
)

var version = "dev"

func versionText() string {
	v := version
	if v == "dev" {
		if build, ok := debug.ReadBuildInfo(); ok && build.Main.Version != "" && build.Main.Version != "(devel)" {
			v = build.Main.Version
		}
	}
	return "TaskBelay " + v + " (" + runtime.GOOS + "/" + runtime.GOARCH + ")"
}

func runDoctor(ctx context.Context, env platform.Env, output io.Writer) error {
	fmt.Fprintln(output, versionText())
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "程序：", exe)
	fmt.Fprintln(output, "数据：", env.DataDir)
	if _, err := distribution.ManagedPath(env, exe); err != nil {
		fmt.Fprintln(output, "程序维护：", err)
	} else {
		fmt.Fprintln(output, "程序维护：安装脚本管理，可通过菜单更新和卸载")
	}
	if path, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(output, "git：未找到，需要 git 2.5 或更高")
	} else {
		fmt.Fprintln(output, "git：", path)
	}
	var problems []error
	for _, name := range []string{"claude", "codex"} {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		state, err := agent.Inspect(checkCtx, env, name, nil)
		cancel()
		command := state.Command
		if command == "" {
			command = "未找到命令"
		}
		fmt.Fprintf(output, "\n%s：%s\n", name, command)
		if err != nil {
			fmt.Fprintln(output, "接入状态未完成核对：", err)
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
			continue
		}
		mcp := "未注册"
		if state.MCP {
			mcp = "已注册"
		} else if state.ForeignMCP {
			mcp = "同名注册属于其他程序"
		}
		fmt.Fprintf(output, "TaskBelay hook：%d；受管理 skill：%d；MCP：%s\n", state.Hooks, state.Skills, mcp)
	}
	fmt.Fprintln(output, "\n接入配置不代表 hook 已实际运行；Codex 仍需在 /hooks 中确认信任。")
	return errors.Join(problems...)
}

func runUpdate(ctx context.Context, env platform.Env, output io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "正在下载并校验最新发布包……")
	message, err := distribution.Update(ctx, env, exe)
	fmt.Fprint(output, message)
	return err
}

func runUninstallAll(ctx context.Context, args []string, env platform.Env, output io.Writer) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "--yes") {
		return fmt.Errorf("用法: taskbelay uninstall --all [--yes]")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := distribution.ManagedPath(env, exe); err != nil {
		return err
	}
	if len(args) == 0 {
		menu, err := terminal.Open(os.Stdin, output)
		if err != nil {
			return fmt.Errorf("卸载需要终端确认；无交互脚本请显式使用 --yes: %w", err)
		}
		defer menu.Close()
		confirmed, err := menu.Confirm(ctx, "移除两个 Agent 的接入并卸载 TaskBelay？账本保留。")
		if err != nil || !confirmed {
			return err
		}
	}
	// 先核对两个 Agent，诊断失败时不开始部分卸载。
	var installed []string
	for _, name := range []string{"claude", "codex"} {
		checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		state, err := agent.Inspect(checkCtx, env, name, nil)
		cancel()
		if err != nil {
			return fmt.Errorf("无法核对 %s 接入，已保留程序: %w", name, err)
		}
		if state.Hooks > 0 || state.Skills > 0 || state.MCP {
			installed = append(installed, name)
		}
	}
	for _, name := range installed {
		if err := runInstall([]string{"uninstall", name}, env, output); err != nil {
			return fmt.Errorf("%s 接入移除未完成，已保留程序: %w", name, err)
		}
	}
	message, err := distribution.Uninstall(env, exe)
	fmt.Fprint(output, message)
	return err
}
