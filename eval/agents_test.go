package eval_test

import (
	"encoding/json"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func nativeEvent(t *testing.T, o hook.Observer, a agent.Adapter, hookName, call, tool string, input any) (event.Event, []byte) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"hook_event_name": hookjson.Events[hookName], "session_id": a.Name() + "-session", "prompt_id": "prompt", "turn_id": "turn", "cwd": o.Workspace.Root, "source": "startup", "prompt": "完成当前需求\n保持用户修改。", "tool_use_id": call, "tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	e, err := a.ParseEvent(hookName, data)
	if err != nil {
		t.Fatal(err)
	}
	return e, data
}

func TestAgentHooksShareLedgerAndCheckRealEditResults(t *testing.T) {
	o := observer(t)
	service := hook.Tasks{Observer: o}
	write(t, o.Workspace.Root, "a.txt", "old\nkeep\n")
	for _, a := range []agent.Adapter{claude.Adapter{}, codex.Adapter{}} {
		for _, kind := range []string{"session-start", "user-prompt-submit"} {
			e, raw := nativeEvent(t, o, a, kind, "", "", nil)
			if _, err := service.Handle(t.Context(), e, raw); err != nil {
				t.Fatal(err)
			}
		}
	}
	view, err := o.Store.TaskView(t.Context(), o.Workspace.ID, "", ledger.Actor{})
	if err != nil || len(view.Messages) != 2 {
		t.Fatalf("messages=%d err=%v", len(view.Messages), err)
	}
	for _, m := range view.Messages {
		if m.Text != "完成当前需求\n保持用户修改。" {
			t.Fatal(m)
		}
	}
	input := map[string]any{"file_path": "a.txt", "old_string": "old", "new_string": "claude"}
	e, raw := nativeEvent(t, o, claude.Adapter{}, "pre-tool-use", "edit-1", "Edit", input)
	if d, err := service.Handle(t.Context(), e, raw); err != nil || d.DenyTool != "" {
		t.Fatal(d, err)
	}
	write(t, o.Workspace.Root, "a.txt", "claude\nkeep\n")
	e, raw = nativeEvent(t, o, claude.Adapter{}, "post-tool-use", "edit-1", "Edit", input)
	if _, err := service.Handle(t.Context(), e, raw); err != nil {
		t.Fatal(err)
	}
	input = map[string]any{"command": "*** Begin Patch\n*** Update File: a.txt\n@@\n-claude\n+codex\n keep\n*** End Patch"}
	e, raw = nativeEvent(t, o, codex.Adapter{}, "pre-tool-use", "edit-2", "apply_patch", input)
	if d, err := service.Handle(t.Context(), e, raw); err != nil || d.DenyTool != "" {
		t.Fatal(d, err)
	}
	write(t, o.Workspace.Root, "a.txt", "codex\nkeep\n")
	e, raw = nativeEvent(t, o, codex.Adapter{}, "post-tool-use", "edit-2", "apply_patch", input)
	if _, err := service.Handle(t.Context(), e, raw); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range changes(t, o) {
		if c.Kind == "agent_edit" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("agent edits=%d", count)
	}
	// 外部修改与旧上下文相交，两种 Agent 都只能根据当前文件重新组织请求。
	write(t, o.Workspace.Root, "a.txt", "user\nkeep\n")
	for i, a := range []agent.Adapter{claude.Adapter{}, codex.Adapter{}} {
		tool := "Edit"
		input := map[string]any{"file_path": "a.txt", "old_string": "codex", "new_string": "stale"}
		if i == 1 {
			tool = "apply_patch"
			input = map[string]any{"command": "*** Begin Patch\n*** Update File: a.txt\n@@\n-codex\n+stale\n keep\n*** End Patch"}
		}
		e, raw := nativeEvent(t, o, a, "pre-tool-use", "stale", tool, input)
		d, err := service.Handle(t.Context(), e, raw)
		if err != nil || d.DenyTool == "" {
			t.Fatal(d, err)
		}
	}
	if err := o.Store.SetSetting(t.Context(), o.Workspace.ID, ledger.Project, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	e, raw = nativeEvent(t, o, claude.Adapter{}, "pre-tool-use", "hint", "Edit", map[string]any{"file_path": "a.txt", "old_string": "codex", "new_string": "stale"})
	if d, err := service.Handle(t.Context(), e, raw); err != nil || d.DenyTool != "" || d.AdditionalContext == "" {
		t.Fatal(d, err)
	}
	// 普通 Stop 保存观察，不提交交付、不运行检查。
	e, raw = nativeEvent(t, o, codex.Adapter{}, "stop", "", "", nil)
	if d, err := service.Handle(t.Context(), e, raw); err != nil || d.BlockStop != "" {
		t.Fatal(d, err)
	}
	state, err := o.Store.ObservationState(t.Context(), o.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	obs, err := o.Store.Observation(t.Context(), state.LatestID)
	if err != nil || obs.Trigger != "stop" {
		t.Fatal(obs, err)
	}
	view, err = o.Store.TaskView(t.Context(), o.Workspace.ID, "", ledger.Actor{})
	if err != nil || view.LatestCheck != "not_checked" {
		t.Fatal(view.LatestCheck, err)
	}
}

func TestAgentUnknownAndFailedEditsStayWorkspaceUpdates(t *testing.T) {
	for _, kind := range []string{"no pre", "notebook", "delete", "shell", "failed edit"} {
		t.Run(kind, func(t *testing.T) {
			o := observer(t)
			service := hook.Tasks{Observer: o}
			write(t, o.Workspace.Root, "a.txt", "before\n")
			observe(t, o, "")
			var a agent.Adapter = claude.Adapter{}
			tool := "Edit"
			input := map[string]any{"file_path": "a.txt", "old_string": "before", "new_string": "after"}
			post := "post-tool-use"
			switch kind {
			case "notebook":
				tool = "NotebookEdit"
				input = map[string]any{"notebook_path": "a.txt", "new_source": "after"}
			case "delete":
				a = codex.Adapter{}
				tool = "apply_patch"
				input = map[string]any{"command": "*** Begin Patch\n*** Delete File: a.txt\n*** End Patch"}
			case "shell":
				tool = "Bash"
				input = map[string]any{"command": "printf after > a.txt"}
			case "failed edit":
				post = "post-tool-use-failure"
			}
			if kind != "no pre" {
				e, raw := nativeEvent(t, o, a, "pre-tool-use", "call", tool, input)
				if _, err := service.Handle(t.Context(), e, raw); err != nil {
					t.Fatal(err)
				}
			}
			// 失败编辑之后的实际内容不等于请求；不能把外部写入归给 Agent。
			write(t, o.Workspace.Root, "a.txt", "external\n")
			e, raw := nativeEvent(t, o, a, post, "call", tool, input)
			if _, err := service.Handle(t.Context(), e, raw); err != nil {
				t.Fatal(err)
			}
			cs := changes(t, o)
			if len(cs) == 0 {
				t.Fatal("lost workspace update")
			}
			for _, c := range cs {
				if c.Kind != "workspace_update" {
					t.Fatal(c)
				}
			}
		})
	}
}

func TestCodexContextPatchAnchorsEndOfFileAndLineEndings(t *testing.T) {
	for _, tt := range []struct {
		name, before, patch, after string
		deny                       bool
	}{
		{"implicit first hunk", "old\n", "-old\n+new", "new\n", false},
		{"no trailing newline", "old", "@@\n-old\n+new", "new\n", false},
		{"mixed line endings", "head\r\nold\ntail\r\n", "@@\n head\n-old\n+new\n tail", "head\r\nnew\r\ntail\r\n", false},
		{"anchor selects later occurrence", "old\nsection\nold\n", "@@ section\n-old\n+new", "old\nsection\nnew\n", false},
		{"end selects final occurrence", "old\nkeep\nold\n", "@@\n-old\n+new\n*** End of File", "old\nkeep\nnew\n", false},
		{"append no context", "old\n", "@@\n+new", "old\nnew\n", false},
		{"ambiguous context", "old\nold\n", "@@\n-old\n+new", "", true},
		{"end context moved", "old\nnewer\n", "@@\n-old\n+new\n*** End of File", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := observer(t)
			service := hook.Tasks{Observer: o}
			write(t, o.Workspace.Root, "a.txt", tt.before)
			observe(t, o, "")
			input := map[string]any{"command": "*** Begin Patch\n*** Update File: a.txt\n" + tt.patch + "\n*** End Patch"}
			e, raw := nativeEvent(t, o, codex.Adapter{}, "pre-tool-use", "patch", "apply_patch", input)
			d, err := service.Handle(t.Context(), e, raw)
			if err != nil || (d.DenyTool != "") != tt.deny {
				t.Fatal(d, err)
			}
			if tt.deny {
				return
			}
			write(t, o.Workspace.Root, "a.txt", tt.after)
			e, raw = nativeEvent(t, o, codex.Adapter{}, "post-tool-use", "patch", "apply_patch", input)
			if _, err := service.Handle(t.Context(), e, raw); err != nil {
				t.Fatal(err)
			}
			cs := changes(t, o)
			if len(cs) != 1 || cs[0].Kind != "agent_edit" {
				t.Fatalf("changes=%+v", cs)
			}
		})
	}
}
