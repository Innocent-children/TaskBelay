package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestStatusShowsCurrentCandidatesUpdatesSettingsAndRecordedError(t *testing.T) {
	dir := t.TempDir()
	env := platform.Env{DataDir: t.TempDir()}
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	w, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ledger.Open(t.Context(), env.DataDir, ledger.NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: w}}
	actor := ledger.Actor{Agent: "claude", SessionID: "s", RoundID: "r1", CallID: "start-a"}
	for i, title := range []string{"原任务", "当前任务"} {
		actor.RoundID, actor.CallID = title, title
		_, err := service.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, MessageID: title, Message: title})
		if err != nil {
			t.Fatal(err)
		}
		input := ledger.StartTaskInput{Title: title}
		if i == 1 {
			input.Workflow = "semi"
		}
		if _, err := service.Start(t.Context(), actor, input); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "用户更新.txt"), []byte("current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hookID, err := store.BeginHook(t.Context(), w.ID, event.Event{Agent: actor.Agent, SessionID: actor.SessionID, Kind: event.Stop}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishHook(t.Context(), hookID, event.Decision{}, errors.New("测试接入错误")); err != nil {
		t.Fatal(err)
	}
	before, err := store.SummaryState(t.Context(), w.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(t.Context(), []string{"status"}, env, dir, &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"当前任务", "原任务", "用户更新.txt", "workflow = semi（任务", "尚未检查", "测试接入错误", "task_get"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("status missing %s\n%s", want, output.String())
		}
	}
	after, err := store.SummaryState(t.Context(), w.ID, actor)
	if err != nil || after != before {
		t.Fatal("status consumed Agent delivery state", after, err)
	}
	sessions, err := store.ActiveSessions(t.Context(), w.ID, time.Now().UTC())
	if err != nil || len(sessions) != 1 {
		t.Fatal("status invented an Agent session", sessions, err)
	}
	output.Reset()
	if err := run(t.Context(), []string{"check", "--refresh"}, env, dir, &output); err != nil || !strings.Contains(output.String(), "输入=") || !strings.Contains(output.String(), "D2") {
		t.Fatal("CLI check missing fixed result", output.String(), err)
	}
	output.Reset()
	if err := run(t.Context(), []string{"status"}, env, dir, &output); err != nil || !strings.Contains(output.String(), "static: partial") || !strings.Contains(output.String(), "未运行：动态测试") {
		t.Fatal("status omitted full saved check", output.String(), err)
	}
}

type closedOutput struct{}

func (closedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestHookOutputFailureCanRetryAndStatusOutsideGitCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	env := platform.Env{DataDir: filepath.Join(t.TempDir(), "not-created")}
	var output bytes.Buffer
	if err := run(t.Context(), []string{"status"}, env, dir, &output); !errors.Is(err, gitstate.ErrNotWorkspace) {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.DataDir); !os.IsNotExist(err) {
		t.Fatal("status outside git created ledger", err)
	}
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	input := cliHookInput(t, dir, "SessionStart", map[string]any{"source": "resume"})
	if err := runHook(t.Context(), []string{"codex", "session-start"}, env, bytes.NewReader(input), closedOutput{}); err != nil {
		t.Fatal("hook did not fail open", err)
	}
	if err := runHook(t.Context(), []string{"codex", "session-start"}, env, bytes.NewReader(input), &output); err != nil || output.Len() == 0 {
		t.Fatal("failed output was suppressed", err)
	}
	output.Reset()
	if err := runHook(t.Context(), []string{"codex", "session-start"}, env, bytes.NewReader(input), &output); err != nil || output.Len() != 0 {
		t.Fatal("delivered output repeated", err)
	}
}
