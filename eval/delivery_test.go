package eval_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func deliveryFixture(t *testing.T) (hook.Tasks, ledger.TaskRecord, ledger.Message, string) {
	t.Helper()
	service := hook.Tasks{Observer: observer(t)}
	base := "package main\nfunc main() {}\n"
	write(t, service.Workspace.Root, "main.go", base)
	message := userMessage(t, service, taskActor("start"), "m1", "完成当前功能，可以保留参数")
	task := startTask(t, service, taskActor("start"), ledger.StartTaskInput{Title: "delivery"})
	return service, task, message, base
}

func verifiedTaskEdit(t *testing.T, service hook.Tasks, task, call, path, old, current string) {
	t.Helper()
	e := edit(call, path, old, current)
	e.RoundID = "round1"
	e.Edits[0].WholeFile = true
	if _, err := os.Stat(filepath.Join(service.Workspace.Root, path)); os.IsNotExist(err) && old == "" {
		e.Edits[0].Before = nil
	}
	decision, err := service.BeforeEdit(t.Context(), e, task, "block")
	if err != nil || decision.DenyTool != "" {
		t.Fatal(decision, err)
	}
	write(t, service.Workspace.Root, path, current)
	e.Kind = event.ToolEnd
	e.Result = &event.ToolResult{State: event.Completed}
	if _, err := service.AfterEdit(t.Context(), e, task); err != nil {
		t.Fatal(err)
	}
}

func stopTask(t *testing.T, service hook.Tasks) event.Decision {
	t.Helper()
	a := taskActor("")
	decision, err := service.Handle(t.Context(), event.Event{Kind: event.Stop, Agent: a.Agent, SessionID: a.SessionID, RoundID: a.RoundID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func TestWF12WF13StopRequiresFinishAndFeedbackRetainsRoundRequest(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	current := base + "func unused() int { return 7 }\n"
	verifiedTaskEdit(t, service, task.ID, "edit", "main.go", base, current)
	ordinary := stopTask(t, service)
	if ordinary.BlockStop != "" || !strings.Contains(ordinary.AdditionalContext, "补记需求") {
		t.Fatal("ordinary stop ran delivery or missed requirement hint", ordinary)
	}
	if result, err := service.Store.LatestCheck(t.Context(), task.ID, "static", ""); err != nil || result != nil {
		t.Fatal("ordinary stop created a check", result, err)
	}
	finish, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{})
	if err != nil || finish.Static != nil || finish.Task.State != ledger.TaskPending {
		t.Fatal("finish ran analysis or lost request", finish, err)
	}
	first := stopTask(t, service)
	if !strings.Contains(first.BlockStop, "D1") {
		t.Fatal("first delivery failed to block proven unused function", first)
	}
	view := getTask(t, service, taskActor("read"), task.ID)
	if view.Task.State != ledger.TaskPending || len(view.Checks) != 1 || view.Checks[0].Status != ledger.CheckComplete {
		t.Fatal("feedback ended task or missing check", view.Task, view.Checks)
	}
	firstCheck := view.Checks[0].ID
	second := stopTask(t, service)
	if second.BlockStop != "" || !strings.Contains(second.UserMessage, "unused") {
		t.Fatal("unchanged second stop blocked or hid unresolved issue", second)
	}
	view = getTask(t, service, taskActor("read"), task.ID)
	if view.Task.State != ledger.TaskCompleted || view.CurrentTaskID != "" || view.Checks[0].ID != firstCheck {
		t.Fatal("unchanged check was rerun or task stayed pending", view.Task, view.Checks)
	}
}

func TestWF15EditingAfterFinishExpiresRequestAndMixedUpdateOnlyHints(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	current := base + "func unused() int {\n return 7\n}\n"
	verifiedTaskEdit(t, service, task.ID, "edit", "main.go", base, current)
	if _, err := service.Finish(t.Context(), taskActor("finish1"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	changed := strings.ReplaceAll(current, "return 7", "return 9")
	write(t, service.Workspace.Root, "main.go", changed)
	decision := stopTask(t, service)
	if decision.BlockStop != "" || !strings.Contains(decision.AdditionalContext, "过期") {
		t.Fatal("silent update did not expire finish", decision)
	}
	view := getTask(t, service, taskActor("view"), "")
	if view.Task.State != ledger.TaskActive {
		t.Fatal("changed task remained pending")
	}
	result, err := service.Check(t.Context(), taskActor("check"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "D1" && issue.At == "main.go:unused" && issue.Level == "hint"
	}) {
		t.Fatal("mixed declaration not kept as hint", result)
	}
	if _, err := service.Finish(t.Context(), taskActor("finish2"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	decision = stopTask(t, service)
	if decision.BlockStop != "" || !strings.Contains(decision.UserMessage, "工作区更新") {
		t.Fatal(decision)
	}
}

func TestIssueIdentityMovesWithoutReblockingSession(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	current := base + "func unused() int { return 7 }\n"
	verifiedTaskEdit(t, service, task.ID, "edit1", "main.go", base, current)
	if _, err := service.Finish(t.Context(), taskActor("finish1"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	if decision := stopTask(t, service); decision.BlockStop == "" {
		t.Fatal("missing first block")
	}
	one, err := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if err != nil {
		t.Fatal(err)
	}
	var original string
	for _, issue := range one.Issues {
		if issue.Rule == "D1" {
			original = issue.ID
		}
	}
	next := strings.ReplaceAll(current, "unused", "renamed")
	verifiedTaskEdit(t, service, task.ID, "edit2", "main.go", current, next)
	if _, err := service.Finish(t.Context(), taskActor("finish2"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	if decision := stopTask(t, service); decision.BlockStop != "" {
		t.Fatal("rename reset per-session block", decision)
	}
	two, _ := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if !slices.ContainsFunc(two.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "D1" && issue.ID == original && issue.At == "main.go:renamed"
	}) {
		t.Fatal("rename reset identity", two.Issues)
	}
}

func TestExactParameterDecisionDoesNotRetainWholeFunction(t *testing.T) {
	service, task, message, base := deliveryFixture(t)
	current := base + "func unused(sep rune) int { return 7 }\n"
	verifiedTaskEdit(t, service, task.ID, "edit", "main.go", base, current)
	result, err := service.Check(t.Context(), taskActor("check1"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	var parameter ledger.Issue
	for _, issue := range result.Issues {
		if issue.Rule == "D5" {
			parameter = issue
		}
	}
	if parameter.ID == "" {
		t.Fatal("missing parameter issue", result.Issues)
	}
	addNote(t, service, taskActor("retain"), ledger.NoteInput{Kind: "decision", Text: "保留参数", SourceID: message.ID, Quote: "可以保留参数", Target: &ledger.DecisionTarget{Kind: "issue", ID: parameter.ID, At: parameter.At, Effect: "retain", Scope: "task"}})
	result, err = service.Check(t.Context(), taskActor("check2"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.ID == parameter.ID && issue.Status == "retained" }) || !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.Rule == "D1" && issue.Status == "open" }) {
		t.Fatal("parameter decision leaked to function or failed", result.Issues)
	}
}

func TestCheckCacheRefreshAndCancellationKeepEvidence(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	one, err := service.Check(t.Context(), taskActor("check1"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := service.Check(t.Context(), taskActor("check2"), check.RunInput{Depth: "static"})
	if err != nil || two.ID != one.ID {
		t.Fatal("unchanged complete input not cached", one, two, err)
	}
	three, err := service.Check(t.Context(), taskActor("check3"), check.RunInput{Depth: "static", Refresh: true})
	if err != nil || three.ID == one.ID {
		t.Fatal("refresh did not rerun", three, err)
	}
	actor := taskActor("cancel")
	actor.RoundID = "cancel-round"
	message := userMessage(t, service, actor, "cancel-message", "这个任务不做了，放弃")
	result, err := service.Finish(t.Context(), actor, ledger.FinishInput{Outcome: "cancel"})
	if err != nil || result.Task.ID != task.ID || result.Task.State != ledger.TaskCanceled || result.Task.Cancellation == nil || result.Task.Cancellation.SourceID != message.ID {
		t.Fatal("cancel lost user evidence", result, err)
	}
	replay, err := service.Finish(t.Context(), actor, ledger.FinishInput{Outcome: "cancel"})
	if err != nil || replay.Task.ID != result.Task.ID {
		t.Fatal("cancel replay failed", replay, err)
	}
}

func TestStaticCapabilityUpgradeRechecksAndKeepsHistory(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	actor := taskActor("prior-capabilities")
	input, err := service.Store.CheckInput(t.Context(), service.Workspace.ID, task.ID, actor, check.RulesVersion, "gotreesitter-0.56.0-facts-tests-v2")
	if err != nil {
		t.Fatal(err)
	}
	previous, err := service.Store.SaveCheck(t.Context(), service.Workspace.ID, actor, input.Tree, ledger.CheckRecord{Input: input, Depth: "static", Status: ledger.CheckComplete, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Check(t.Context(), taskActor("current-capabilities"), check.RunInput{Depth: "static"})
	if err != nil || current.ID == previous.ID || current.Status != ledger.CheckComplete || current.Input.Capabilities != check.CapabilitiesVersion {
		t.Fatal("the previous capability result was reused", previous, current, err)
	}
	retained, err := service.Store.LatestCheck(t.Context(), task.ID, "static", previous.InputVersion)
	if err != nil || retained == nil || retained.ID != previous.ID {
		t.Fatal("the previous result was not preserved", retained, err)
	}
	repeated, err := service.Check(t.Context(), taskActor("repeat-current-capabilities"), check.RunInput{Depth: "static"})
	if err != nil || repeated.ID != current.ID {
		t.Fatal("unchanged current capabilities were not cached", current, repeated, err)
	}
}

func TestPreDeliveryCheckIsHintAndReusedWhenFinishRequested(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	current := base + "func unused() int { return 7 }\n"
	verifiedTaskEdit(t, service, task.ID, "edit", "main.go", base, current)
	record, err := service.Check(t.Context(), taskActor("check"), check.RunInput{Depth: "static"})
	if err != nil || record.Level == "block" {
		t.Fatal("pre-delivery check claimed blocking conclusion", record, err)
	}
	if _, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	if decision := stopTask(t, service); !strings.Contains(decision.BlockStop, "D1") {
		t.Fatal("eligible cached result not applied at delivery", decision)
	}
	latest, _ := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if latest.ID != record.ID {
		t.Fatal("finish reran unchanged complete analysis")
	}
}

func TestPartialResultDoesNotRerunOnSecondFeedbackStop(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	write(t, service.Workspace.Root, "not-parsed.txt", "not parsed\n")
	current := "package main\nimport \"flag\"\nfunc main() { _ = flag.String(\"extra\", \"x\", \"help\") }\n"
	verifiedTaskEdit(t, service, task.ID, "edit", "main.go", base, current)
	if _, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	if decision := stopTask(t, service); !strings.Contains(decision.BlockStop, "D3") {
		t.Fatal("missing first entry feedback", decision)
	}
	one, _ := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if one.Status != ledger.CheckPartial {
		t.Fatal("fixture did not retain missing scope", one)
	}
	if decision := stopTask(t, service); decision.BlockStop != "" {
		t.Fatal(decision)
	}
	two, _ := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if one.ID != two.ID {
		t.Fatal("same feedback request reran partial analysis")
	}
}
