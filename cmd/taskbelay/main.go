// taskbelay 是 TaskBelay 的命令行入口。
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

const usage = `用法：
  taskbelay install claude|codex      接入 Agent
  taskbelay uninstall claude|codex    移除接入，任务账本里的数据保留
  taskbelay status                    显示当前工作区记到的 session 和最近的用户消息
  taskbelay config get <设置项>       显示一项设置当前的值
  taskbelay config set <设置项> <值>  修改一项设置
  taskbelay hook <agent> <事件>       由 Agent 调用，处理一次 hook

设置项：
  workflow      工作模式：auto（全自动，默认）或 semi（半自动）
  enforcement   拦截强度：block（拦截一次，默认）或 hint（只提示）`

// errUsage 表示命令行参数不对，此时打印用法。
var errUsage = errors.New("参数不对")

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	env, err := platform.CurrentEnv()
	if err != nil {
		if args[0] == "hook" {
			return 0 // hook 出错时也正常退出，见 runHook
		}
		fmt.Fprintln(stderr, "taskbelay:", err)
		return 1
	}
	if args[0] == "hook" {
		runHook(env, args[1:], stdin, stdout)
		return 0
	}

	logger, closeLog, err := openLog(logDir(env), time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "taskbelay:", err)
		return 1
	}
	defer closeLog()
	slog.SetDefault(logger)

	switch args[0] {
	case "install":
		err = runInstall(env, args[1:], stdout, true)
	case "uninstall":
		err = runInstall(env, args[1:], stdout, false)
	case "status":
		err = runStatus(env, args[1:], stdout)
	case "config":
		err = runConfig(env, args[1:], stdout)
	default:
		err = errUsage
	}

	switch {
	case errors.Is(err, errUsage):
		fmt.Fprintln(stderr, usage)
		return 2
	case err != nil:
		slog.Error("命令失败", "args", args, "err", err)
		fmt.Fprintln(stderr, "taskbelay:", err)
		return 1
	}
	return 0
}

func logDir(env platform.Env) string {
	return filepath.Join(env.DataDir(), "logs")
}

func openLedger(env platform.Env) (*ledger.Ledger, error) {
	return ledger.Open(filepath.Join(env.DataDir(), "taskbelay.db"))
}
