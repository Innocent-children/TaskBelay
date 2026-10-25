package agent_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
)

func payload(t *testing.T, native, tool string, input any, extra map[string]any) []byte {
	t.Helper()
	in := map[string]any{"session_id": "s", "prompt_id": "p", "turn_id": "t", "cwd": "/项目 space", "hook_event_name": native, "tool_use_id": "call", "tool_name": tool, "tool_input": input}
	for key, value := range extra {
		in[key] = value
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestNativeToolNormalizationAndResultEvidence(t *testing.T) {
	tests := []struct {
		name       string
		adapter    agent.Adapter
		hook, tool string
		input      any
		extra      map[string]any
		kind       event.Kind
		state      event.ToolState
		code       *int
		edit       bool
	}{
		{"claude edit", claude.Adapter{}, "pre-tool-use", "Edit", map[string]any{"file_path": "a", "old_string": "old", "new_string": "new", "replace_all": true}, nil, event.EditUse, "", nil, true},
		{"claude write", claude.Adapter{}, "pre-tool-use", "Write", map[string]any{"file_path": "a", "content": ""}, nil, event.EditUse, "", nil, true},
		{"claude notebook", claude.Adapter{}, "pre-tool-use", "NotebookEdit", map[string]any{"notebook_path": "a.ipynb", "new_source": "print(1)", "cell_id": "a", "edit_mode": "replace"}, nil, event.EditUse, "", nil, true},
		{"claude powershell", claude.Adapter{}, "pre-tool-use", "PowerShell", map[string]any{"command": "Get-Item ."}, nil, event.CommandUse, "", nil, false},
		{"claude success", claude.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "go test ./..."}, map[string]any{"tool_response": map[string]any{"stdout": "ok", "stderr": "", "interrupted": false}}, event.ToolEnd, event.Completed, new(0), false},
		{"claude failure", claude.Adapter{}, "post-tool-use-failure", "PowerShell", map[string]any{"command": "exit 17"}, map[string]any{"error": "Exit code 17\nnot ok"}, event.ToolEnd, event.Failed, new(17), false},
		{"claude interrupted", claude.Adapter{}, "post-tool-use-failure", "Bash", map[string]any{"command": "sleep 30"}, map[string]any{"error": "interrupted", "is_interrupt": true}, event.ToolEnd, event.Interrupted, nil, false},
		{"claude failed start", claude.Adapter{}, "post-tool-use-failure", "Bash", map[string]any{"command": "bad"}, map[string]any{"error": "cannot start: 1"}, event.ToolEnd, event.Failed, nil, false},
		{"claude background", claude.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test"}, map[string]any{"tool_response": map[string]any{"backgroundTaskId": "job"}}, event.ToolEnd, event.Running, nil, false},
		{"claude requested background", claude.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test", "run_in_background": true}, map[string]any{"tool_response": map[string]any{"stdout": "started", "interrupted": false}}, event.ToolEnd, event.Running, nil, false},
		{"claude unknown response", claude.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test"}, map[string]any{"tool_response": "done"}, event.ToolEnd, event.Unknown, nil, false},
		{"claude malformed status", claude.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test"}, map[string]any{"tool_response": map[string]any{"stdout": "ok", "interrupted": false, "exit_code": "unknown"}}, event.ToolEnd, event.Unknown, nil, false},
		{"codex exec", codex.Adapter{}, "pre-tool-use", "exec_command", map[string]any{"cmd": "go test ./...", "workdir": "nested"}, nil, event.CommandUse, "", nil, false},
		{"codex plain output", codex.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "go test ./..."}, map[string]any{"tool_response": "Process exited with code 0\nok"}, event.ToolEnd, event.Unknown, nil, false},
		{"codex running", codex.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test"}, map[string]any{"tool_response": map[string]any{"session_id": 23, "exit_code": nil}}, event.ToolEnd, event.Running, nil, false},
		{"codex complete", codex.Adapter{}, "post-tool-use", "Bash", map[string]any{"command": "test"}, map[string]any{"tool_response": map[string]any{"exit_code": 5}}, event.ToolEnd, event.Failed, new(5), false},
		{"codex patch", codex.Adapter{}, "pre-tool-use", "apply_patch", map[string]any{"command": "*** Begin Patch\n*** Update File: a\n@@\n-old\n+new\n*** End Patch"}, nil, event.EditUse, "", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := tt.adapter.ParseEvent(tt.hook, payload(t, hookjson.Events[tt.hook], tt.tool, tt.input, tt.extra))
			if err != nil {
				t.Fatal(err)
			}
			if e.Kind != tt.kind || e.SessionID != "s" || e.ToolCallID != "call" || len(e.Edits) > 0 != tt.edit {
				t.Fatalf("event=%+v", e)
			}
			round := "p"
			if tt.adapter.Name() == "codex" {
				round = "t"
			}
			if e.RoundID != round {
				t.Fatal(e.RoundID)
			}
			if tt.state != "" {
				if e.Result == nil || e.Result.State != tt.state {
					t.Fatalf("result=%+v", e.Result)
				}
				if (e.Result.ExitCode == nil) != (tt.code == nil) || tt.code != nil && *e.Result.ExitCode != *tt.code {
					t.Fatalf("exit=%v", e.Result.ExitCode)
				}
			}
			if tt.tool == "NotebookEdit" && e.Edits[0].Unverifiable == "" {
				t.Fatal("notebook falsely verifiable")
			}
			if tt.tool == "PowerShell" && e.Command.Shell != event.PowerShell {
				t.Fatal(e.Command)
			}
		})
	}
}

func TestNativeLifecycleAndInvalidInput(t *testing.T) {
	for _, a := range []agent.Adapter{claude.Adapter{}, codex.Adapter{}} {
		for _, mode := range []string{"startup", "resume", "clear", "compact", "fork"} {
			e, err := a.ParseEvent("session-start", payload(t, "SessionStart", "", nil, map[string]any{"source": mode}))
			if err != nil || e.Kind != event.SessionStart {
				t.Fatal(e, err)
			}
		}
		e, err := a.ParseEvent("user-prompt-submit", payload(t, "UserPromptSubmit", "", nil, map[string]any{"prompt": "完整\n原文\t😀"}))
		if err != nil || e.Message != "完整\n原文\t😀" || e.MessageID != e.RoundID {
			t.Fatal(e, err)
		}
		for _, data := range [][]byte{[]byte(`null`), []byte(`{}`), []byte(`{} {}`), payload(t, "Stop", "", nil, nil)} {
			if _, err := a.ParseEvent("pre-tool-use", data); err == nil {
				t.Fatal("invalid input accepted")
			}
		}
	}
	if _, err := (codex.Adapter{}).ParseEvent("post-tool-use-failure", payload(t, "PostToolUseFailure", "Bash", map[string]string{"command": "false"}, nil)); err == nil {
		t.Fatal("invented event")
	}
}

func TestDecisionsAreASCIIAndPreserveAgentPermissions(t *testing.T) {
	for _, a := range []agent.Adapter{claude.Adapter{}, codex.Adapter{}} {
		for hook := range hookjson.Events {
			if hook == "post-tool-use-failure" && a.Name() == "codex" {
				continue
			}
			d := event.Decision{AdditionalContext: "当前内容😀", UserMessage: "提示"}
			if hook == "pre-tool-use" {
				d.DenyTool = "重新读取"
			}
			if hook == "stop" {
				d.BlockStop = "补充需求"
			}
			data, err := a.RenderDecision(hook, d)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range data {
				if b > 127 {
					t.Fatalf("non-ASCII: %s", data)
				}
			}
			if bytes.Contains(data, []byte(`"allow"`)) || bytes.Contains(data, []byte(`"continue"`)) {
				t.Fatal(string(data))
			}
			var out map[string]any
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatal(err)
			}
			if hook != "stop" && out["hookSpecificOutput"].(map[string]any)["additionalContext"] != "当前内容😀" {
				t.Fatal(out)
			}
			if hook == "stop" && out["hookSpecificOutput"] != nil {
				t.Fatal("unsupported stop context")
			}
		}
		if _, err := a.RenderDecision("post-tool-use", event.Decision{DenyTool: "bad"}); err == nil {
			t.Fatal("post denied original tool")
		}
	}
}

func TestCodexMCPUsesHookSessionAndNativeCallIdentity(t *testing.T) {
	meta := map[string]any{"sessionId": "runtime-session", "threadId": "different-thread", "callId": "native-call", "x-codex-turn-metadata": map[string]any{"session_id": "runtime-session", "thread_id": "different-thread", "turn_id": "turn", "codex_version": "0.163.0"}}
	e, found, err := codex.MCPIdentity(meta)
	if err != nil || !found || e.SessionID != "runtime-session" || e.RoundID != "turn" || e.ToolCallID != "native-call" {
		t.Fatal(e, found, err)
	}
	meta["sessionId"] = "different-session"
	if _, _, err := codex.MCPIdentity(meta); err == nil {
		t.Fatal("conflicting session accepted")
	}
	delete(meta, "sessionId")
	delete(meta, "callId")
	if _, _, err := codex.MCPIdentity(meta); err == nil {
		t.Fatal("missing native call accepted")
	}
	if _, found, err := codex.MCPIdentity(map[string]any{"threadId": "thread"}); err != nil || found {
		t.Fatal("guessed native identity")
	}
}

func TestClaudeMCPPreHookOnlyBindsNativeIdentity(t *testing.T) {
	a := claude.Adapter{}
	e, err := a.ParseEvent("pre-tool-use", payload(t, "PreToolUse", "mcp__taskbelay__task_start", map[string]string{"title": "task"}, nil))
	if err != nil || e.Kind != event.MCPUse || e.RoundID != "p" || e.MCPTool != "task_start" || e.Command != nil || len(e.Edits) != 0 {
		t.Fatal(e, err)
	}
	if _, err := a.ParseEvent("pre-tool-use", payload(t, "PreToolUse", "mcp__taskbelay__task_start", nil, map[string]any{"prompt_id": ""})); err == nil {
		t.Fatal("missing round accepted")
	}
	if _, err := a.ParseEvent("pre-tool-use", payload(t, "PreToolUse", "mcp__other__task_start", nil, nil)); err == nil {
		t.Fatal("matched unrelated MCP")
	}
	identity, found, err := claude.MCPIdentity(map[string]any{"claudecode/toolUseId": "native", "progressToken": 99}, "session")
	if err != nil || !found || identity.SessionID != "session" || identity.ToolCallID != "native" || identity.RoundID != "" {
		t.Fatal(identity, found, err)
	}
}
