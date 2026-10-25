package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

func cliHookInput(t *testing.T, dir, native string, extra map[string]any) []byte {
	t.Helper()
	in := map[string]any{"cwd": dir, "hook_event_name": native, "session_id": "s", "prompt_id": "p", "turn_id": "t", "source": "startup"}
	for key, value := range extra {
		in[key] = value
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHookNonRepoAndFailuresContinueWithoutDeny(t *testing.T) {
	base := t.TempDir()
	env := platform.Env{HomeDir: base, DataDir: filepath.Join(base, "data")}
	for _, a := range []string{"claude", "codex"} {
		var output bytes.Buffer
		if err := runHook(t.Context(), []string{a, "stop"}, env, bytes.NewReader(cliHookInput(t, base, "Stop", nil)), &output); err != nil || output.Len() != 0 {
			t.Fatal(output.String(), err)
		}
	}
	if _, err := os.Stat(env.DataDir); !os.IsNotExist(err) {
		t.Fatal("nonrepo hook created ledger or logs")
	}
	repo := filepath.Join(base, "repo")
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.DataDir, "taskbelay.db"), []byte("invalid sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"claude", "codex"} {
		var output bytes.Buffer
		if err := runHook(t.Context(), []string{a, "stop"}, env, bytes.NewReader(cliHookInput(t, repo, "Stop", nil)), &output); err != nil {
			t.Fatal(err)
		}
		var result map[string]string
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(output.String(), err)
		}
		if !strings.Contains(result["systemMessage"], "TaskBelay 这次没有完成检查") || !strings.Contains(result["systemMessage"], "打开账本") || result["decision"] != "" {
			t.Fatal(result)
		}
		for _, b := range output.Bytes() {
			if b > 127 {
				t.Fatal("non-ASCII response")
			}
		}
		output.Reset()
		if err := runHook(t.Context(), []string{a, "pre-tool-use"}, env, bytes.NewReader(cliHookInput(t, repo, "PreToolUse", map[string]any{"tool_name": "Bash", "tool_use_id": "c", "tool_input": map[string]string{"command": "test"}})), &output); err != nil || output.Len() != 0 {
			t.Fatal(output.String(), err)
		}
	}
	logs, err := filepath.Glob(filepath.Join(env.DataDir, "logs", "*"))
	if err != nil || len(logs) == 0 {
		t.Fatal("hook failure not logged", err)
	}
	var out bytes.Buffer
	if err := runHook(t.Context(), []string{"claude", "pre-tool-use"}, env, strings.NewReader("bad JSON"), &out); err != nil || out.Len() != 0 {
		t.Fatal(out.String(), err)
	}
}

func TestHookProcessHelper(t *testing.T) {
	dataDir := os.Getenv("TASKBELAY_HOOK_TEST_DATA")
	if dataDir == "" {
		return
	}
	err := run(context.Background(), []string{"hook", os.Getenv("TASKBELAY_HOOK_TEST_AGENT"), os.Getenv("TASKBELAY_HOOK_TEST_EVENT")}, platform.Env{DataDir: dataDir}, "", os.Stdout)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func TestNativeHookCLIProcessesShareLedger(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, agent := range []string{"claude", "codex"} {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestHookProcessHelper$")
		cmd.Env = append(os.Environ(), "TASKBELAY_HOOK_TEST_DATA="+dataDir, "TASKBELAY_HOOK_TEST_AGENT="+agent, "TASKBELAY_HOOK_TEST_EVENT=user-prompt-submit")
		cmd.Stdin = bytes.NewReader(cliHookInput(t, dir, "UserPromptSubmit", map[string]any{"prompt": "保持完整\n用户消息。"}))
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("hook process: %v %s", err, output)
		}
		var response struct {
			Specific struct {
				Name string `json:"hookEventName"`
				Text string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal(output, &response); err != nil || response.Specific.Name != "UserPromptSubmit" || !strings.Contains(response.Specific.Text, "task_get") {
			t.Fatalf("missing handoff context: %+v %v", response, err)
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var messages, events int
	if err := db.QueryRow("SELECT count(*) FROM messages").Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM hook_events WHERE state='recorded'").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if messages != 2 || events != 2 {
		t.Fatalf("messages=%d events=%d", messages, events)
	}
	// 同一个原生消息重发不会新增来源；两种 Agent 的 session 相同也不能混为一条。
	var output bytes.Buffer
	if err := runHook(t.Context(), []string{"claude", "user-prompt-submit"}, platform.Env{DataDir: dataDir}, bytes.NewReader(cliHookInput(t, dir, "UserPromptSubmit", map[string]any{"prompt": "保持完整\n用户消息。"})), &output); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM messages").Scan(&messages); err != nil || messages != 2 {
		t.Fatal(messages, err)
	}
}

func TestFastHookDeadlineContinuesOnCancellation(t *testing.T) {
	dir := t.TempDir()
	env := platform.Env{DataDir: filepath.Join(t.TempDir(), "data")}
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	input := cliHookInput(t, dir, "PreToolUse", map[string]any{"tool_name": "Edit", "tool_use_id": "c", "tool_input": map[string]string{"file_path": "a", "old_string": "old", "new_string": "new"}})
	if err := runHook(ctx, []string{"claude", "pre-tool-use"}, env, bytes.NewReader(input), &output); err != nil || output.Len() != 0 {
		t.Fatal(output.String(), err)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "taskbelay.db")); !os.IsNotExist(err) {
		t.Fatal("canceled hook opened ledger")
	}
	logs, err := filepath.Glob(filepath.Join(env.DataDir, "logs", "*.log"))
	if err != nil || len(logs) != 1 {
		t.Fatal("missing canceled hook diagnostic", logs, err)
	}
	data, err := os.ReadFile(logs[0])
	if err != nil || !bytes.Contains(data, []byte("定位工作区")) || !bytes.Contains(data, []byte("context canceled")) {
		t.Fatal("canceled hook lost phase or cause", string(data), err)
	}
}

type slowHookInput struct {
	io.Reader
	delay time.Duration
}

func (r *slowHookInput) Read(p []byte) (int, error) {
	if r.delay > 0 {
		time.Sleep(r.delay)
		r.delay = 0
	}
	return r.Reader.Read(p)
}

func TestEditHookBudgetAndNonVerificationDeadline(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		for _, tc := range []struct {
			name     string
			delay    time.Duration
			command  bool
			recorded int
		}{
			{name: "edit-after-50ms", delay: 100 * time.Millisecond, recorded: 1},
			{name: "edit-after-300ms", delay: 350 * time.Millisecond},
			{name: "command-after-50ms", delay: 100 * time.Millisecond, command: true},
		} {
			t.Run(agent+"/"+tc.name, func(t *testing.T) {
				dir, dataDir := t.TempDir(), t.TempDir()
				if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
				if err := os.WriteFile(filepath.Join(dir, "a"), []byte("old\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				store, err := ledger.Open(t.Context(), dataDir, ledger.NormalWait)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				tool := "Edit"
				toolInput := map[string]string{"file_path": "a", "old_string": "old", "new_string": "new"}
				if agent == "codex" {
					tool = "apply_patch"
					toolInput = map[string]string{"command": "*** Begin Patch\n*** Update File: a\n@@\n-old\n+new\n*** End Patch"}
				}
				if tc.command {
					tool, toolInput = "Bash", map[string]string{"command": "echo ok"}
				}
				input := cliHookInput(t, dir, "PreToolUse", map[string]any{"tool_name": tool, "tool_use_id": "c", "tool_input": toolInput})
				reader := &slowHookInput{Reader: bytes.NewReader(input), delay: tc.delay}
				var output bytes.Buffer
				if err := runHook(t.Context(), []string{agent, "pre-tool-use"}, platform.Env{DataDir: dataDir}, reader, &output); err != nil || output.Len() != 0 {
					t.Fatal(output.String(), err)
				}
				db, err := sql.Open("sqlite", filepath.Join(dataDir, "taskbelay.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var total, recorded int
				if err := db.QueryRow("SELECT count(*), count(CASE WHEN state='recorded' THEN 1 END) FROM hook_events").Scan(&total, &recorded); err != nil {
					t.Fatal(err)
				}
				if total != tc.recorded || recorded != tc.recorded {
					t.Fatalf("hook events: total=%d recorded=%d want=%d", total, recorded, tc.recorded)
				}
				if tc.recorded == 0 {
					logs, err := filepath.Glob(filepath.Join(dataDir, "logs", "*.log"))
					if err != nil || len(logs) != 1 {
						t.Fatal("missing deadline diagnostic", logs, err)
					}
					data, err := os.ReadFile(logs[0])
					if err != nil || !bytes.Contains(data, []byte("context deadline exceeded")) {
						t.Fatal("deadline failure lost its cause", string(data), err)
					}
				}
			})
		}
	}
}

func TestStopDeliveryUsesStaticBudgetBeyondMessageDeadline(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	w, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ledger.Open(t.Context(), dataDir, ledger.NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	service := hook.Tasks{Observer: hook.Observer{Store: store, Workspace: w}}
	actor := ledger.Actor{Agent: "claude", SessionID: "s", RoundID: "p", CallID: "start"}
	if _, err := service.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, MessageID: "m", Message: "交付当前任务"}); err != nil {
		t.Fatal(err)
	}
	task, err := service.Start(t.Context(), actor, ledger.StartTaskInput{Title: "delivery budget"})
	if err != nil {
		t.Fatal(err)
	}
	actor.CallID = "finish"
	if _, err := service.Finish(t.Context(), actor, ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	reader := &slowHookInput{Reader: bytes.NewReader(cliHookInput(t, dir, "Stop", nil)), delay: event.MessageBudget + 100*time.Millisecond}
	var output bytes.Buffer
	if err := runHook(t.Context(), []string{"claude", "stop"}, platform.Env{DataDir: dataDir}, reader, &output); err != nil {
		t.Fatal(err)
	}
	view, err := store.TaskView(t.Context(), w.ID, task.ID, actor)
	if err != nil || view.Task.State != ledger.TaskCompleted || len(view.Checks) != 1 {
		t.Fatal("Stop was cut off at the ordinary message deadline", output.String(), view.Task, err)
	}
}
