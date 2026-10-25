package eval_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/agent"
	"github.com/Innocent-children/taskbelay/internal/agent/claude"
	"github.com/Innocent-children/taskbelay/internal/agent/codex"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

func handoff(t *testing.T, service hook.Tasks, adapter agent.Adapter, source, session string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"hook_event_name": "SessionStart", "session_id": session, "cwd": service.Workspace.Root, "source": source})
	if err != nil {
		t.Fatal(err)
	}
	e, err := adapter.ParseEvent("session-start", raw)
	if err != nil {
		t.Fatal(err)
	}
	d, err := service.Handle(t.Context(), e, raw)
	if err != nil || d.DenyTool != "" || d.BlockStop != "" {
		t.Fatal(d, err)
	}
	encoded, err := adapter.RenderDecision("session-start", d)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range encoded {
		if b > 127 {
			t.Fatal("native output is not ASCII")
		}
	}
	if summary.Tokens(d.AdditionalContext) > 2000 {
		t.Fatal("handoff exceeded budget")
	}
	return d.AdditionalContext
}

func TestWF05BidirectionalHandoffPreservesSilentUserEdits(t *testing.T) {
	for _, pair := range [][2]agent.Adapter{{claude.Adapter{}, codex.Adapter{}}, {codex.Adapter{}, claude.Adapter{}}} {
		t.Run(pair[0].Name()+" to "+pair[1].Name(), func(t *testing.T) {
			service := hook.Tasks{Observer: observer(t)}
			write(t, service.Workspace.Root, "export.go", "initial\n")
			a := ledger.Actor{Agent: pair[0].Name(), SessionID: "first", RoundID: "round", CallID: "start"}
			message := userMessage(t, service, a, "prompt", "完成导出流程")
			task := startTask(t, service, a, ledger.StartTaskInput{Title: "导出"})
			a.CallID = "requirement"
			req := saveRequirement(t, service, a, ledger.RequirementInput{Content: "完成导出", Acceptance: "输出正确", SourceID: message.ID, Quote: "完成导出流程"})
			a.CallID = "next"
			addNote(t, service, a, ledger.NoteInput{Kind: "next_step", Text: "重写错误处理"})
			write(t, service.Workspace.Root, "export.go", "user fixed error handling\n")
			git(t, service.Workspace.Root, "add", "export.go")
			git(t, service.Workspace.Root, "commit", "--quiet", "-m", "user fixes")
			text := handoff(t, service, pair[1], "startup", "second")
			for _, want := range []string{task.ID, "R1", "export.go", "笔记后工作区已有变化", "重写错误处理", "共享当前工作区", summary.Footer} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %s\n%s", want, text)
				}
			}
			if got := string(read(t, filepath.Join(service.Workspace.Root, "export.go"))); got != "user fixed error handling\n" {
				t.Fatal("handoff rewrote user content", got)
			}
			if text := handoff(t, service, pair[0], "resume", "first"); !strings.Contains(text, "共享当前工作区") {
				t.Fatal("first session did not learn about the second", text)
			}
			view := getTask(t, service, a, "")
			if view.Requirements[0].Revision != req.Revision || view.Requirements[0].Status != "pending" || view.Task.AuthorizationRevision != 0 {
				t.Fatal("workspace updates changed progress or authorization")
			}
			if text := handoff(t, service, pair[1], "resume", "second"); text != "" {
				t.Fatal("unchanged resume repeated context", text)
			}
			for _, source := range []string{"clear", "compact"} {
				if text := handoff(t, service, pair[1], source, "second"); !strings.Contains(text, "事前提示") {
					t.Fatal("fresh context lost guidance", source, text)
				}
			}
			if text := handoff(t, service, pair[1], "fork", "second"); text != "" {
				t.Fatal("same input fork repeated context", text)
			}
			if text := handoff(t, service, pair[1], "fork", "new-fork"); !strings.Contains(text, task.ID) {
				t.Fatal("new fork without prior delivery lost task", text)
			}
			if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "enforcement", "hint"); err != nil {
				t.Fatal(err)
			}
			if text := handoff(t, service, pair[1], "resume", "second"); !strings.Contains(text, "事前提示") || !strings.Contains(text, "只提示模式") {
				t.Fatal("changed settings did not refresh guidance", text)
			}
			git(t, service.Workspace.Root, "commit", "--quiet", "--allow-empty", "-m", "context only")
			if text := handoff(t, service, pair[1], "resume", "second"); text == "" || strings.Contains(text, "事前提示") {
				t.Fatal("HEAD change was ignored or repeated unchanged guidance", text)
			}
		})
	}
}

func TestWF16OverBudgetRequirementsStillShowSilentUpdates(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	a := taskActor("start")
	userMessage(t, service, a, "prompt", "继续完成需求")
	task := startTask(t, service, a, ledger.StartTaskInput{Title: "大量需求"})
	for i := range 12 {
		a.CallID = fmt.Sprintf("r%d", i)
		saveRequirement(t, service, a, ledger.RequirementInput{Content: strings.Repeat("只实现当前要求", 100), Acceptance: "保持范围"})
	}
	handoff(t, service, codex.Adapter{}, "startup", a.SessionID)
	write(t, service.Workspace.Root, "current.go", "user version\n")
	text := handoff(t, service, codex.Adapter{}, "resume", a.SessionID)
	if summary.Tokens(text) > 2000 || !strings.Contains(text, "共 12 条") || !strings.Contains(text, "省略") || !strings.Contains(text, "current.go") || !strings.Contains(text, task.ID) || !strings.HasSuffix(text, summary.Footer) {
		t.Fatal(text)
	}
}

func TestNewMessagesDoNotRepeatDeliveredUnchangedContext(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	e := event.Event{Kind: event.UserPrompt, Agent: "claude", SessionID: "s", RoundID: "r1", MessageID: "m1", Message: "开始"}
	first, err := service.Handle(t.Context(), e, nil)
	if err != nil || first.AdditionalContext == "" {
		t.Fatal(first, err)
	}
	e.RoundID, e.MessageID, e.Message = "r2", "m2", "进度如何"
	second, err := service.Handle(t.Context(), e, nil)
	if err != nil || second.AdditionalContext != "" {
		t.Fatal("new message repeated unchanged context", second, err)
	}
	write(t, service.Workspace.Root, "silent.txt", "external\n")
	e.RoundID, e.MessageID = "r3", "m3"
	third, err := service.Handle(t.Context(), e, nil)
	if err != nil || !strings.Contains(third.AdditionalContext, "silent.txt") {
		t.Fatal(third, err)
	}
	view := getTask(t, service, ledger.Actor{}, "")
	if view.Task != nil || len(view.Messages) != 3 {
		t.Fatal("summary started a task or lost a message")
	}
}

func TestFailedOutputIsNotMarkedAsDelivered(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	e := event.Event{Kind: event.SessionStart, Agent: "codex", SessionID: "s", StartMode: event.Resume}
	err := service.HandleOutput(t.Context(), e, nil, func(event.Decision) error { return errors.New("output closed") })
	if err == nil {
		t.Fatal("lost output error")
	}
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID}
	state, err := service.Store.SummaryState(t.Context(), service.Workspace.ID, actor)
	if err != nil || state.InputVersion != "" {
		t.Fatal("undelivered output was acknowledged", state, err)
	}
	if text := handoff(t, service, codex.Adapter{}, "resume", "s"); text == "" {
		t.Fatal("failed output was not retried")
	}
	if text := handoff(t, service, codex.Adapter{}, "resume", "s"); text != "" {
		t.Fatal("delivered output repeated")
	}
}

func TestPostEditReportsOnlyObservedWorkspaceUpdates(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	write(t, service.Workspace.Root, "a.txt", "old\nkeep\n")
	actor := taskActor("start")
	userMessage(t, service, actor, "prompt", "改第一行")
	startTask(t, service, actor, ledger.StartTaskInput{Title: "编辑"})
	handoff(t, service, claude.Adapter{}, "startup", "claude-session")
	input := map[string]any{"file_path": "a.txt", "old_string": "old", "new_string": "new"}
	e, raw := nativeEvent(t, service.Observer, claude.Adapter{}, "pre-tool-use", "mixed", "Edit", input)
	if _, err := service.Handle(t.Context(), e, raw); err != nil {
		t.Fatal(err)
	}
	write(t, service.Workspace.Root, "a.txt", "new\nuser changed second line\n")
	e, raw = nativeEvent(t, service.Observer, claude.Adapter{}, "post-tool-use", "mixed", "Edit", input)
	d, err := service.Handle(t.Context(), e, raw)
	if err != nil || !strings.Contains(d.AdditionalContext, "a.txt") || !strings.Contains(d.AdditionalContext, "本次只观察目标文件") {
		t.Fatal(d, err)
	}
	input = map[string]any{"file_path": "a.txt", "old_string": "new", "new_string": "agent"}
	e, raw = nativeEvent(t, service.Observer, claude.Adapter{}, "pre-tool-use", "owned", "Edit", input)
	if _, err := service.Handle(t.Context(), e, raw); err != nil {
		t.Fatal(err)
	}
	write(t, service.Workspace.Root, "a.txt", "agent\nuser changed second line\n")
	e, raw = nativeEvent(t, service.Observer, claude.Adapter{}, "post-tool-use", "owned", "Edit", input)
	d, err = service.Handle(t.Context(), e, raw)
	if err != nil || d.AdditionalContext != "" {
		t.Fatal("verified Agent edit repeated handoff", d, err)
	}
}

func TestTaskRestoreReopenAndBranchSelectionUseCurrentCode(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	write(t, service.Workspace.Root, "a.txt", "original\n")
	git(t, service.Workspace.Root, "add", "a.txt")
	git(t, service.Workspace.Root, "commit", "--quiet", "-m", "origin")
	a := taskActor("a")
	userMessage(t, service, a, "m1", "任务 A")
	first := startTask(t, service, a, ledger.StartTaskInput{Title: "A"})
	a.CallID = "next"
	addNote(t, service, a, ledger.NoteInput{Kind: "next_step", Text: "完成 A"})
	branch := first.Context.Branch
	git(t, service.Workspace.Root, "checkout", "--quiet", "-b", "other")
	if text := handoff(t, service, codex.Adapter{}, "resume", a.SessionID); !strings.Contains(text, "当前没有任务") || !strings.Contains(text, first.ID) {
		t.Fatal("missing candidates after branch switch", text)
	}
	a.RoundID, a.CallID = "b", "b"
	userMessage(t, service, a, "m2", "任务 B")
	startTask(t, service, a, ledger.StartTaskInput{Title: "B"})
	git(t, service.Workspace.Root, "checkout", "--quiet", branch)
	write(t, service.Workspace.Root, "a.txt", "current while paused\n")
	text := handoff(t, service, codex.Adapter{}, "resume", a.SessionID)
	if !strings.Contains(text, "任务 "+first.ID) || !strings.Contains(text, "a.txt") || !strings.Contains(text, "笔记后工作区已有变化") {
		t.Fatal("branch task restoration did not show current changes", text)
	}
	a.CallID = "pending"
	transition(t, service, a, first.ID, ledger.TaskPending)
	a.CallID = "complete"
	transition(t, service, a, first.ID, ledger.TaskCompleted)
	a.RoundID, a.CallID = "reopen", "reopen"
	userMessage(t, service, a, "m3", "继续 A")
	write(t, service.Workspace.Root, "a.txt", "current after completion\n")
	reopened := startTask(t, service, a, ledger.StartTaskInput{TaskID: first.ID})
	text, err := service.TaskSummary(t.Context(), a, first.ID)
	if err != nil || reopened.OriginTree != first.OriginTree || !strings.Contains(text, "a.txt") || !strings.Contains(text, "进行中") {
		t.Fatal(text, err)
	}
	if got := string(read(t, filepath.Join(service.Workspace.Root, "a.txt"))); got != "current after completion\n" {
		t.Fatal("restoration rewrote files")
	}
}
