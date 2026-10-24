// Package hook 决定每个事件该做什么。它只认识统一的事件和决定，不知道事件来自哪个 Agent。
package hook

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

// Handle 处理工作区 ws 里的一个事件，返回决定。每个事件都更新 session 的记录和最后活动时间；
// 用户消息还要记下原文和此刻的工作区状态，作为原话引用的来源和起点快照的候选。
func Handle(ctx context.Context, l *ledger.Ledger, ws ledger.Workspace, ev event.Event, now time.Time) (event.Decision, error) {
	wsID, err := l.TouchSession(ctx, ws, ev.Agent, ev.Session, now)
	if err != nil {
		return event.Decision{}, err
	}
	if ev.Kind == event.UserPrompt {
		state, err := gitstate.State(ws.Path)
		if err != nil {
			return event.Decision{}, err
		}
		if _, err := l.AddMessage(ctx, wsID, ledger.Message{
			Agent: ev.Agent, Session: ev.Session, Turn: ev.Turn, Text: ev.Prompt, State: state, At: now,
		}); err != nil {
			return event.Decision{}, err
		}
	}
	return event.Decision{}, nil
}
