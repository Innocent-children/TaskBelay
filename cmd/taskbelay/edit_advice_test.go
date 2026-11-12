package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func TestEditAdviceSessionRecord(t *testing.T) {
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, repo := t.Context(), t.TempDir()
	env := platform.Env{DataDir: t.TempDir()}
	require(exec.Command("git", "init", "-q", repo).Run())
	w, err := gitstate.Discover(ctx, repo)
	require(err)
	store, err := ledger.Open(ctx, env.DataDir, ledger.NormalWait)
	require(err)
	defer store.Close()
	require(store.RegisterGitWorkspace(ctx, w))
	service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: w}}
	actor := ledger.Actor{Agent: "codex", SessionID: "session-001", RoundID: "round-001", CallID: "start"}
	_, err = service.RecordUserMessage(ctx, event.Event{Kind: event.UserPrompt, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, MessageID: "prompt", Message: "调整标题"})
	require(err)
	task, err := service.Start(ctx, actor, ledger.StartTaskInput{Title: "调整标题"})
	require(err)
	// 编辑预览只读取已发布的起点索引。
	require(runCheck(ctx, nil, env, repo, io.Discard))
	require(store.SetSetting(ctx, w.ID, ledger.Task, "debug", "on"))

	input := fmt.Sprintf(`{"cwd":%q,"session_id":"session-001","turn_id":"round-001","hook_event_name":"PreToolUse","tool_name":"apply_patch","tool_use_id":"edit-001","tool_input":{"command":"*** Begin Patch\n*** Add File: extra.txt\n+extra\n*** End Patch"}}`, repo)
	edit := func() string {
		t.Helper()
		var stdout bytes.Buffer
		require(runHook(ctx, []string{"codex", "pre-tool-use"}, env, strings.NewReader(input), &stdout))
		return strings.TrimSpace(stdout.String())
	}
	var output struct {
		HookSpecificOutput struct{ AdditionalContext string }
	}
	require(json.Unmarshal([]byte(edit()), &output))
	advice := output.HookSpecificOutput.AdditionalContext
	if !strings.Contains(advice, "新文件：extra.txt") {
		t.Fatalf("未输出预期的设计提示：%q", advice)
	}
	if replay := edit(); replay != "" {
		t.Fatalf("重放输出：%s", replay)
	}
	read := func(pattern string) string {
		t.Helper()
		files, err := filepath.Glob(filepath.Join(env.DataDir, pattern))
		require(err)
		if len(files) != 1 {
			t.Fatalf("%s 匹配文件数 %d", pattern, len(files))
		}
		data, err := os.ReadFile(files[0])
		require(err)
		return string(data)
	}
	records := read("session-records/*/codex-*.md")
	source := "candidate:" + units.Hash("external", "file:extra.txt")
	if strings.Count(records, advice) != 1 || !strings.Contains(records, source+"；轮次：round-001；工具调用：edit-001") {
		t.Fatalf("提示与触发操作的记录不匹配：%s", records)
	}
	trace := read("debug/*/" + task.ID + ".jsonl")
	var outputs int
	for line := range strings.SplitSeq(trace, "\n") {
		if strings.Contains(line, `"phase":"hook.output"`) && strings.Contains(line, `"tool_call_id":"edit-001"`) && strings.Contains(line, `"written":true`) {
			if outputs == 0 && !strings.Contains(line, advice) {
				t.Fatalf("debug 与首次输出不符：%s", line)
			}
			outputs++
		}
	}
	if outputs != 2 {
		t.Fatalf("首次输出和重放应分别成功，debug 中找到 %d 次", outputs)
	}
	t.Logf("任务 %s，调用 edit-001，首次输出与会话记录一致；重放输出为空。\n%s\n%s", task.ID, advice, source)
}
