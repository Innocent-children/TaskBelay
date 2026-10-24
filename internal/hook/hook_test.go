package hook

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

// 每个事件都更新 session 的最后活动时间；用户消息记下原文、轮次和到达时的工作区状态。
func TestHandleRecordsSessionAndPrompt(t *testing.T) {
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init：%v %s", err, out)
	}
	root, err := gitstate.Root(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(filepath.Join(t.TempDir(), "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	ws := ledger.Workspace{Path: root, Key: root}

	t0 := time.UnixMilli(1_000_000)
	t1 := t0.Add(time.Minute)
	base := event.Event{Agent: "codex", Session: "s1", Dir: root}
	start := base
	start.Kind = event.SessionStart
	if _, err := Handle(ctx, l, ws, start, t0); err != nil {
		t.Fatal(err)
	}
	prompt := base
	prompt.Kind, prompt.Turn, prompt.Prompt = event.UserPrompt, "turn-1", "给 export 命令加一个导出 CSV 的选项\n第二行"
	if _, err := Handle(ctx, l, ws, prompt, t1); err != nil {
		t.Fatal(err)
	}

	wsID, _, err := l.FindWorkspace(ctx, ws.Key)
	if err != nil {
		t.Fatal(err)
	}
	sessions, _, err := l.RecentSessions(ctx, wsID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || !sessions[0].StartedAt.Equal(t0) || !sessions[0].LastActiveAt.Equal(t1) {
		t.Errorf("session 记录不对：%+v", sessions)
	}
	messages, err := l.RecentMessages(ctx, wsID, 5)
	if err != nil {
		t.Fatal(err)
	}
	state, err := gitstate.State(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("应有 1 条消息，得到 %d 条", len(messages))
	}
	m := messages[0]
	if m.Text != prompt.Prompt || m.Turn != "turn-1" || m.State != state || !m.At.Equal(t1) {
		t.Errorf("消息记录不对：%+v，工作区状态应为 %s", m, state)
	}
}
