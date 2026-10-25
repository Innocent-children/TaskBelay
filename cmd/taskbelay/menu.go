package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/terminal"
)

func runMenu(ctx context.Context, env platform.Env, dir string, output io.Writer) error {
	menu, err := terminal.Open(os.Stdin, output)
	if err != nil {
		return err
	}
	defer menu.Close()
	items := []string{"安装 / 修复 Agent 接入", "移除 Agent 接入", "常用设置", "当前任务与检查", "本机状态与接入诊断", "更新 TaskBelay", "卸载 TaskBelay（保留账本）", "退出"}
	for {
		choice, err := menu.Select(ctx, versionText()+" · 管理菜单", items, false)
		if errors.Is(err, terminal.ErrBack) || errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		switch choice[0] {
		case 0, 1:
			action := "install"
			if choice[0] == 1 {
				action = "uninstall"
			}
			err = menuAgents(ctx, menu, action, env, output)
		case 2:
			err = menuSettings(ctx, menu, env, dir, output)
		case 3:
			err = menuTask(ctx, menu, env, dir, output)
		case 4:
			err = runDoctor(ctx, env, output)
		case 5:
			var ok bool
			ok, err = menu.Confirm(ctx, "下载最新发布包并更新程序？完成后退出菜单。")
			if err == nil && ok {
				err = runUpdate(ctx, env, output)
				if err == nil {
					return nil
				}
			} else if err == nil {
				continue
			}
		case 6:
			var ok bool
			ok, err = menu.Confirm(ctx, "移除两个 Agent 的接入并卸载 TaskBelay？账本保留。")
			if err == nil && ok {
				err = runUninstallAll(ctx, []string{"--yes"}, env, output)
				if err == nil {
					return nil
				}
			} else if err == nil {
				continue
			}
		case 7:
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if errors.Is(err, terminal.ErrBack) {
			continue
		}
		if err != nil {
			fmt.Fprintln(output, "操作未完成：", err)
		}
		if err := menu.Pause(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
	}
}

func menuAgents(ctx context.Context, menu *terminal.Menu, action string, env platform.Env, output io.Writer) error {
	title := "选择要安装 / 修复接入的 Agent"
	if action == "uninstall" {
		title = "选择要移除接入的 Agent（保留账本）"
	}
	selected, err := menu.Select(ctx, title, []string{"Claude Code", "Codex"}, true)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return terminal.ErrBack
	}
	var problems []error
	for _, index := range selected {
		name := []string{"claude", "codex"}[index]
		fmt.Fprintln(output, "\n处理 "+name+"……")
		if err := runInstall([]string{action, name}, env, output); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(problems...)
}

func menuSettings(ctx context.Context, menu *terminal.Menu, env platform.Env, dir string, output io.Writer) error {
	choice, err := menu.Select(ctx, "常用设置", []string{"查看有效设置及来源", "工作模式", "拦截强度", "动态测试检查"}, false)
	if err != nil {
		return err
	}
	keys := []string{"workflow", "enforcement", "analysis.tests"}
	if choice[0] == 0 {
		for _, name := range keys {
			if err := run(ctx, []string{"config", "get", name}, env, dir, output); err != nil {
				return err
			}
		}
		return nil
	}
	key := keys[choice[0]-1]
	scope, err := menu.Select(ctx, "设置生效范围", []string{"用户默认", "当前项目", "当前任务"}, false)
	if err != nil {
		return err
	}
	values := [][]string{{"auto", "semi"}, {"block", "hint"}, {"off", "on"}}[choice[0]-1]
	labels := [][]string{{"全自动：直接开发", "半自动：先确认方案"}, {"同一问题有限拦截", "只提示"}, {"关闭", "开启：允许在临时副本运行测试"}}[choice[0]-1]
	value, err := menu.Select(ctx, "选择 "+key, labels, false)
	if err != nil {
		return err
	}
	if key == "analysis.tests" && value[0] == 1 {
		ok, err := menu.Confirm(ctx, "确认开启动态测试检查？会运行项目的测试和准备命令。")
		if err != nil {
			return err
		}
		if !ok {
			return terminal.ErrBack
		}
	}
	args := []string{"config", "set", key, values[value[0]]}
	if scope[0] > 0 {
		args = append(args, []string{"", "--project", "--task"}[scope[0]])
	}
	if err := run(ctx, args, env, dir, output); err != nil {
		return err
	}
	return run(ctx, []string{"config", "get", key}, env, dir, output)
}

func menuTask(ctx context.Context, menu *terminal.Menu, env platform.Env, dir string, output io.Writer) error {
	choice, err := menu.Select(ctx, "当前工作区的任务与检查", []string{"查看任务状态和最近检查", "运行静态检查", "运行一次动态测试检查"}, false)
	if err != nil {
		return err
	}
	if choice[0] == 0 {
		return runStatus(ctx, env, dir, output)
	}
	var args []string
	if choice[0] == 2 {
		ok, err := menu.Confirm(ctx, "本次检查将运行项目测试和准备命令，是否继续？")
		if err != nil {
			return err
		}
		if !ok {
			return terminal.ErrBack
		}
		args = []string{"--tests"}
	}
	return runCheck(ctx, args, env, dir, output)
}
