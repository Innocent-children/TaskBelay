package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// runHook 处理 taskbelay hook <agent> <事件>。TaskBelay 自己出了任何问题都正常退出、
// 不输出内容，让 Agent 照常工作，错误写进日志。不在 git 工作区里时直接退出。
func runHook(env platform.Env, args []string, stdin io.Reader, stdout io.Writer) {
	logger, closeLog, err := openLog(logDir(env), time.Now())
	if err != nil {
		return // 日志也写不了，只能安静地退出
	}
	defer closeLog()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("hook 崩溃", "args", args, "panic", fmt.Sprint(r))
		}
	}()
	if err := handleHook(env, args, stdin, stdout); err != nil {
		logger.Error("hook 失败", "args", args, "err", err)
	}
}

// handleHook 把一次 hook 调用串起来：适配器把输入转成事件，hook 给出决定，
// 适配器再把决定写成这个 Agent 要的输出。
func handleHook(env platform.Env, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("参数应该是 <agent> <事件>")
	}
	a, ok := agent.Lookup(args[0])
	if !ok {
		return fmt.Errorf("没有 %s 这个 Agent", args[0])
	}
	input, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	ev, err := a.ParseEvent(args[1], input)
	if err != nil {
		return err
	}
	ws, err := findWorkspace(ev.Dir)
	if errors.Is(err, gitstate.ErrNotWorkspace) {
		return nil
	}
	if err != nil {
		return err
	}

	l, err := openLedger(env)
	if err != nil {
		return err
	}
	defer l.Close()
	d, err := hook.Handle(context.Background(), l, ws, ev, time.Now())
	if err != nil {
		return err
	}
	out, err := a.RenderDecision(args[1], d)
	if err != nil {
		return err
	}
	if len(out) > 0 {
		_, err = stdout.Write(out)
	}
	return err
}
