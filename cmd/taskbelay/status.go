package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

const (
	statusSessions = 10 // 最多列出几个 session
	statusMessages = 5  // 最多列出几条用户消息
	timeLayout     = "2006-01-02 15:04"
)

// runStatus 处理 taskbelay status：显示当前工作区记到的 session 和最近的用户消息。
func runStatus(env platform.Env, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errUsage
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	ws, err := findWorkspace(dir)
	if errors.Is(err, gitstate.ErrNotWorkspace) {
		return errors.New("当前目录不在 git 工作区里")
	}
	if err != nil {
		return err
	}

	l, err := openLedger(env)
	if err != nil {
		return err
	}
	defer l.Close()
	ctx := context.Background()

	fmt.Fprintf(stdout, "工作区：%s\n", ws.Path)
	wsID, found, err := l.FindWorkspace(ctx, ws.Key)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(stdout, "\n还没有记到这个工作区里的 session。")
		return nil
	}

	sessions, total, err := l.RecentSessions(ctx, wsID, statusSessions)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "\nsession（共 %d 个，最近活动的在前）：\n", total)
	for _, s := range sessions {
		fmt.Fprintf(stdout, "  %-6s  %s  开始于 %s，最后活动 %s\n",
			s.Agent, s.ID, s.StartedAt.Format(timeLayout), s.LastActiveAt.Format(timeLayout))
	}
	if total > len(sessions) {
		fmt.Fprintf(stdout, "  另有 %d 个更早的 session\n", total-len(sessions))
	}

	messages, err := l.RecentMessages(ctx, wsID, statusMessages)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		fmt.Fprintln(stdout, "\n还没有记到用户消息。")
		return nil
	}
	fmt.Fprintln(stdout, "\n最近的用户消息：")
	for _, m := range messages {
		fmt.Fprintf(stdout, "  m%d  %s  %s %s\n", m.ID, m.At.Format(timeLayout), m.Agent, m.Session)
		for _, line := range strings.Split(m.Text, "\n") {
			fmt.Fprintf(stdout, "    %s\n", line)
		}
	}
	return nil
}
