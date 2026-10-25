package eval_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

type knownVerificationScope struct{ environmentID string }

func (scope knownVerificationScope) VerificationScope(_ context.Context, _ gitstate.Workspace, _ shell.Script, tree string) (testrun.VerificationScope, error) {
	environment := scope.environmentID
	if environment == "" {
		environment = "fixture-environment-v1"
	}
	return testrun.VerificationScope{InputID: tree, ToolID: "fixture-tool-v1", EnvironmentID: environment, Complete: true, EnvironmentKnown: true, Suggestion: "按已核对的 fixture 依赖可只验证当前包"}, nil
}

func commandEvent(service hook.Tasks, call, text string) event.Event {
	a := taskActor(call)
	return event.Event{Kind: event.CommandUse, Agent: a.Agent, SessionID: a.SessionID, RoundID: a.RoundID, ToolCallID: a.CallID, WorkingDir: service.Workspace.Root, Command: &event.Command{Text: text, Shell: event.POSIX}}
}

func commandHook(t *testing.T, service hook.Tasks, e event.Event) event.Decision {
	t.Helper()
	d, err := service.Handle(t.Context(), e, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func completeCommand(t *testing.T, service hook.Tasks, e event.Event, state event.ToolState, code *int) event.Decision {
	t.Helper()
	e.Kind = event.ToolEnd
	e.Result = &event.ToolResult{State: state, ExitCode: code}
	return commandHook(t, service, e)
}

func TestOT09WF10VerificationBlocksOnceAndRetryDoesNotCountExecution(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	service.ScopeAnalyzer = knownVerificationScope{}
	first := commandEvent(service, "verify1", "go test ./...")
	if d := commandHook(t, service, first); d.DenyTool != "" || !strings.Contains(d.AdditionalContext, "V2") {
		t.Fatal(d)
	}
	code := 0
	completeCommand(t, service, first, event.Completed, &code)
	second := commandEvent(service, "verify2", "go  test ./...")
	d := commandHook(t, service, second)
	if !strings.Contains(d.DenyTool, "V1") {
		t.Fatal("matching proven verification did not block", d)
	}
	if repeated := commandHook(t, service, second); repeated.DenyTool != d.DenyTool {
		t.Fatal("pre-hook retry changed decision", repeated, d)
	}
	completeCommand(t, service, second, event.Completed, &code)
	third := commandEvent(service, "verify3", "go test ./...")
	if d := commandHook(t, service, third); d.DenyTool != "" {
		t.Fatal("same issue blocked twice", d)
	}
	completeCommand(t, service, third, event.Completed, &code)
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if err != nil || len(records) != 3 || !records[1].Denied || records[1].Executed {
		t.Fatal(records, err)
	}
	if records[0].State != "passed" || records[2].State != "passed" {
		t.Fatal(records)
	}
	if records[1].Issue.ID != records[2].Issue.ID {
		t.Fatal("tool call reset issue identity")
	}
	other := third
	other.SessionID = "other-session"
	other.ToolCallID = "different-agent-call"
	if d := commandHook(t, service, other); d.DenyTool != "" {
		t.Fatal("different session reused previous execution", d)
	}
}

func TestWF10RerunAuthorizationIsSpecificAndConsumedOnce(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	service.ScopeAnalyzer = knownVerificationScope{}
	first := commandEvent(service, "verify1", "go test ./...")
	commandHook(t, service, first)
	code := 0
	completeCommand(t, service, first, event.Completed, &code)
	records, _ := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	actor := taskActor("grant")
	note := addNote(t, service, actor, ledger.NoteInput{Kind: "decision", Text: "怀疑结果不稳定，重跑一次", Target: &ledger.DecisionTarget{Kind: "verification", ID: records[0].ID, Effect: "rerun_once", Scope: "once"}})
	wrong := commandEvent(service, "different", "go vet ./...")
	commandHook(t, service, wrong)
	view := getTask(t, service, actor, task.ID)
	for _, n := range view.Notes {
		if n.ID == note.ID && n.Consumed {
			t.Fatal("another command consumed grant")
		}
	}
	second := commandEvent(service, "verify2", "go test ./...")
	if d := commandHook(t, service, second); d.DenyTool != "" || !strings.Contains(d.AdditionalContext, note.ID) {
		t.Fatal(d)
	}
	commandHook(t, service, second)
	completeCommand(t, service, second, event.Completed, &code)
	view = getTask(t, service, actor, task.ID)
	found := false
	for _, n := range view.Notes {
		if n.ID == note.ID {
			found = n.Consumed
		}
	}
	if !found {
		t.Fatal("grant not consumed")
	}
	third := commandEvent(service, "verify3", "go test ./...")
	if d := commandHook(t, service, third); d.DenyTool == "" {
		t.Fatal("one grant allowed another run", d)
	}
	actor.CallID = "invalid-target"
	_, err := service.AddNote(t.Context(), actor, ledger.NoteInput{Kind: "decision", Text: "不能引用不存在的验证", Target: &ledger.DecisionTarget{Kind: "verification", ID: "missing", Effect: "rerun_once", Scope: "once"}})
	if err == nil {
		t.Fatal("missing verification target accepted")
	}
}

func TestOT13VerificationUnknownInputsDirectoriesAndHintModeAllow(t *testing.T) {
	for _, mode := range []string{"missing-runner", "unknown-result", "changed-content", "hint", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			service, task, _, _ := deliveryFixture(t)
			if mode != "missing-runner" {
				service.ScopeAnalyzer = knownVerificationScope{}
			}
			text := "go test ./..."
			if mode == "mixed" {
				text = "go test ./...; echo done"
			}
			first := commandEvent(service, "verify1", text)
			commandHook(t, service, first)
			code := 0
			if mode == "unknown-result" {
				completeCommand(t, service, first, event.Completed, nil)
			} else {
				completeCommand(t, service, first, event.Completed, &code)
			}
			if mode == "changed-content" {
				write(t, service.Workspace.Root, "main.go", "package main\nfunc main() { println(1) }\n")
			}
			if mode == "hint" {
				if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "enforcement", "hint"); err != nil {
					t.Fatal(err)
				}
			}
			if d := commandHook(t, service, commandEvent(service, "verify2", text)); d.DenyTool != "" {
				t.Fatal("uncertain verification blocked", d)
			}
			records, _ := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
			if mode == "unknown-result" && records[0].State != "unknown" {
				t.Fatal("empty output inferred success", records[0])
			}
		})
	}
	service, _, _, _ := deliveryFixture(t)
	service.ScopeAnalyzer = knownVerificationScope{}
	first := commandEvent(service, "one", "cd one && go test ./...")
	commandHook(t, service, first)
	code := 0
	completeCommand(t, service, first, event.Completed, &code)
	if d := commandHook(t, service, commandEvent(service, "two", "cd two && go test ./...")); d.DenyTool != "" {
		t.Fatal("different directories shared result", d)
	}
}

func TestV3OnlyCountsExecutedCallsAndUpdatesForNewFailure(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	code := 0
	for i := 1; i <= 9; i++ {
		e := commandEvent(service, fmt.Sprintf("verify%d", i), "go test ./...")
		commandHook(t, service, e)
		state := event.Completed
		if i == 9 {
			code = 1
			state = event.Failed
		}
		d := completeCommand(t, service, e, state, &code)
		if strings.Contains(d.AdditionalContext, "V3") != (i == 7 || i == 9) {
			t.Fatalf("round count feedback %d: %+v", i, d)
		}
		if retry := completeCommand(t, service, e, state, &code); strings.Contains(retry.AdditionalContext, "V3") {
			t.Fatal("post retry repeated count feedback", retry)
		}
	}
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if err != nil || len(records) != 9 {
		t.Fatal(records, err)
	}
	for _, record := range records {
		if !record.Executed {
			t.Fatal("actual run omitted", record)
		}
	}
}

func TestVerificationPostWithoutPreAndRunningCompletion(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	e := commandEvent(service, "missing-pre", "go test ./...")
	completeCommand(t, service, e, event.Running, nil)
	records, _ := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if len(records) != 1 || records[0].State != "running" || records[0].Tree != "" || records[0].Scope.Complete || records[0].RequestDirectory != e.WorkingDir {
		t.Fatal(records)
	}
	stopTask(t, service)
	records, _ = service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if records[0].State != "unknown" || !records[0].EndedAt.IsZero() {
		t.Fatal("stop inferred a finished background process", records)
	}
	code := 0
	completeCommand(t, service, e, event.Completed, &code)
	records, _ = service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if records[0].State != "passed" || records[0].EndedAt.IsZero() {
		t.Fatal(records)
	}
}

func TestVerificationPostKeepsOriginalDirectoryAfterCommandChangesCWD(t *testing.T) {
	for _, tc := range []struct {
		agent string
		shell event.Shell
		text  string
	}{
		{"claude", event.POSIX, "cd .. && python -m pytest -q 2>&1 | tail -3"},
		{"codex", event.POSIX, "cd .. && python -m pytest -q 2>&1 | tail -3"},
		{"claude", event.PowerShell, "Set-Location ..; python -m pytest -q"},
	} {
		t.Run(string(tc.agent)+"/"+string(tc.shell), func(t *testing.T) {
			service, task, _, _ := deliveryFixture(t)
			write(t, service.Workspace.Root, "src/module.py", "value = 1\n")
			before := commandEvent(service, "cwd-change", tc.text)
			before.Agent, before.Command.Shell = tc.agent, tc.shell
			before.WorkingDir = filepath.Join(service.Workspace.Root, "src")
			commandHook(t, service, before)
			started, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
			if err != nil || len(started) != 1 {
				t.Fatal(started, err)
			}
			after := before
			after.WorkingDir = service.Workspace.Root
			zero := 0
			completeCommand(t, service, after, event.Completed, &zero)
			completeCommand(t, service, after, event.Completed, &zero)
			finished, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
			if err != nil || len(finished) != 1 || !finished[0].Executed || finished[0].State != "passed" {
				t.Fatal(finished, err)
			}
			if finished[0].RequestDirectory != before.WorkingDir || finished[0].RequestHash != started[0].RequestHash || finished[0].CommandKey != started[0].CommandKey || !reflect.DeepEqual(finished[0].Command, started[0].Command) {
				t.Fatal("post cwd changed the original command input", started[0], finished[0])
			}
		})
	}
}

func TestVerificationDirectoryChangeDoesNotAcceptDifferentRequest(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	write(t, service.Workspace.Root, "src/module.py", "value = 1\n")
	before := commandEvent(service, "identity", "cd .. && python -m pytest -q")
	before.WorkingDir = filepath.Join(service.Workspace.Root, "src")
	commandHook(t, service, before)
	wrongDirectory := before
	wrongDirectory.WorkingDir = service.Workspace.Root
	if _, err := service.BeforeCommand(t.Context(), wrongDirectory); err == nil {
		t.Fatal("pre replay accepted a different request directory")
	}
	for _, command := range []event.Command{
		{Text: "cd .. && python -m pytest -x", Shell: event.POSIX},
		{Text: before.Command.Text, Shell: event.PowerShell},
	} {
		after := before
		after.WorkingDir, after.Command = service.Workspace.Root, &command
		zero := 0
		after.Result = &event.ToolResult{State: event.Completed, ExitCode: &zero}
		if _, err := service.AfterCommand(t.Context(), after); err == nil {
			t.Fatal("post event accepted changed command or shell", command)
		}
	}
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if err != nil || len(records) != 1 || records[0].Executed || records[0].State != "unknown" {
		t.Fatal("identity mismatch changed execution result", records, err)
	}
}

func TestVerificationLegacyRequestDoesNotInferMissingDirectory(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	before := commandEvent(service, "legacy", "cd src && go test ./...")
	script := hook.CommandScript(before)
	_, err := service.Store.BeginVerification(t.Context(), ledger.VerificationRecord{
		WorkspaceID: service.Workspace.ID, TaskID: task.ID, Actor: taskActor("legacy"),
		InputRevision: task.RecordRevision, Command: script, CommandKey: script.CommandKey(),
		RequestHash: fmt.Sprintf("%x", sha256.Sum256([]byte(string(before.Command.Shell)+"\x00"+before.WorkingDir+"\x00"+before.Command.Text))),
	})
	if err != nil {
		t.Fatal(err)
	}
	after := before
	after.WorkingDir = filepath.Join(service.Workspace.Root, "src")
	zero := 0
	after.Result = &event.ToolResult{State: event.Completed, ExitCode: &zero}
	if _, err := service.AfterCommand(t.Context(), after); err == nil {
		t.Fatal("legacy record inferred an unavailable request directory")
	}
	after.WorkingDir = before.WorkingDir
	if _, err := service.AfterCommand(t.Context(), after); err != nil {
		t.Fatal("legacy request with original directory failed", err)
	}
}

func TestCargoToolchainVerificationRecordsKeepIdentityAndUnknownScope(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	code := 0
	commands := []string{"cargo +1.97.0 test --locked", "cargo +1.97.0 test --locked", "cargo +1.99.1 test --locked"}
	for i, text := range commands {
		e := commandEvent(service, fmt.Sprintf("cargo-%d", i), text)
		if d := commandHook(t, service, e); d.DenyTool != "" {
			t.Fatal("unverified Cargo scope blocked", d)
		}
		completeCommand(t, service, e, event.Completed, &code)
	}
	postOnly := commandEvent(service, "cargo-post-only", "cargo +1.97.0 check")
	completeCommand(t, service, postOnly, event.Completed, &code)
	other := commandEvent(service, "cargo-fmt", "cargo +1.97.0 fmt")
	commandHook(t, service, other)
	completeCommand(t, service, other, event.Completed, &code)
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if err != nil || len(records) != 4 {
		t.Fatalf("Cargo verification records: count=%d error=%v", len(records), err)
	}
	for i, record := range records {
		if !record.Executed || record.State != "passed" || record.Scope.Complete || record.Denied {
			t.Fatalf("record %d did not preserve execution and unknown scope: %+v", i, record)
		}
	}
	if records[0].CommandKey != records[1].CommandKey || records[0].CommandKey == records[2].CommandKey || records[1].PreviousID != records[0].ID {
		t.Fatal("Cargo toolchain identity or previous execution was lost")
	}
	if records[0].Command.Segments[0].Args[1] != "+1.97.0" || records[2].Command.Segments[0].Args[1] != "+1.99.1" {
		t.Fatal("recorded toolchain selection was removed")
	}
	if !records[3].PostOnly || records[3].Tree != "" {
		t.Fatal("post-only execution inferred missing inputs", records[3])
	}
}

func TestCargoToolchainBuildNeverReusesEvenWithKnownScope(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	service.ScopeAnalyzer = knownVerificationScope{}
	code := 0
	for i := 0; i < 2; i++ {
		e := commandEvent(service, fmt.Sprintf("build-%d", i), "cargo +1.97.0 build")
		if d := commandHook(t, service, e); d.DenyTool != "" {
			t.Fatal("build with outputs was blocked", d)
		}
		completeCommand(t, service, e, event.Completed, &code)
	}
	records, err := service.Store.Verifications(t.Context(), service.Workspace.ID, task.ID)
	if err != nil || len(records) != 2 {
		t.Fatalf("build records: count=%d error=%v", len(records), err)
	}
	if !records[1].Executed || !records[1].Scope.Complete || records[1].Issue == nil || records[1].Issue.Level != "hint" {
		t.Fatal("known scope bypassed build side-effect guard", records[1])
	}
}
