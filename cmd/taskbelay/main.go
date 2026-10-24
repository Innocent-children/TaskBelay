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

	"github.com/Innocent-children/taskbelay/internal/platform"
)

const usage = `用法：
  taskbelay config get <设置项>
  taskbelay config set <设置项> <值>

设置项：
  workflow      工作模式：auto（全自动，默认）或 semi（半自动）
  enforcement   拦截强度：block（拦截一次，默认）或 hint（只提示）`

// errUsage 表示命令行参数不对，此时打印用法。
var errUsage = errors.New("参数不对")

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}

	dataDir, err := platform.DataDir()
	if err != nil {
		fmt.Fprintln(stderr, "taskbelay:", err)
		return 1
	}
	logger, closeLog, err := openLog(filepath.Join(dataDir, "logs"), time.Now())
	if err != nil {
		fmt.Fprintln(stderr, "taskbelay:", err)
		return 1
	}
	defer closeLog()
	slog.SetDefault(logger)

	switch args[0] {
	case "config":
		err = runConfig(dataDir, args[1:], stdout)
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
