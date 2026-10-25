package eval_test

import (
	"errors"
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
	"github.com/Innocent-children/taskbelay/internal/plan"
)

func semiFixture(t *testing.T, request string) (hook.Tasks, ledger.TaskRecord, ledger.Message) {
	t.Helper()
	service := hook.Tasks{Observer: observer(t)}
	write(t, service.Workspace.Root, "main.go", "package main\nfunc main() {}\n")
	write(t, service.Workspace.Root, "go.mod", "module example.test/demo\n\ngo 1.26.0\n")
	message := userMessage(t, service, taskActor("message"), "m1", request)
	task := startTask(t, service, taskActor("start"), ledger.StartTaskInput{Title: "半自动", Workflow: "semi"})
	return service, task, message
}

func confirmPlan(t *testing.T, service hook.Tasks, input ledger.PlanInput, message ledger.Message, call string) ledger.PlanResult {
	t.Helper()
	saved, err := service.SavePlan(t.Context(), taskActor(call+"-save"), input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ConfirmPlan(t.Context(), taskActor(call+"-confirm"), ledger.PlanConfirmInput{Version: saved.Plan.Version, SourceID: message.ID, Quote: message.Text})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func prePlan(t *testing.T, service hook.Tasks, task, call, path string) event.Decision {
	t.Helper()
	e := edit(call, path, "", "draft\n")
	e.RoundID = "round1"
	e.Edits[0].WholeFile = true
	e.Edits[0].Before = nil
	decision, err := service.BeforeEdit(t.Context(), e, task, "block")
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func TestSA01ScopeBlocksShareRetrySessionAndRoundLimits(t *testing.T) {
	service := hook.Tasks{Observer: observer(t)}
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "workflow", "semi"); err != nil {
		t.Fatal(err)
	}
	if decision := prePlan(t, service, "", "one", "draft.txt"); !strings.Contains(decision.DenyTool, "S1") {
		t.Fatal(decision)
	}
	if decision := prePlan(t, service, "", "one", "draft.txt"); !strings.Contains(decision.DenyTool, "S1") {
		t.Fatal("retry lost original decision", decision)
	}
	if decision := prePlan(t, service, "", "two", "draft.txt"); decision.DenyTool != "" {
		t.Fatal("same issue blocked twice", decision)
	}
	for _, name := range []string{"second.txt", "third.txt"} {
		if decision := prePlan(t, service, "", name, name); decision.DenyTool == "" {
			t.Fatal("new issue not blocked", decision)
		}
	}
	if decision := prePlan(t, service, "", "fourth", "fourth.txt"); decision.DenyTool != "" {
		t.Fatal("round exceeded three blocks", decision)
	}
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	if decision := prePlan(t, service, "", "hint", "hint.txt"); decision.DenyTool != "" || !strings.Contains(decision.AdditionalContext, "S1 hint") {
		t.Fatal(decision)
	}
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Project, "enforcement", "block"); err != nil {
		t.Fatal(err)
	}
	e := edit("other-session", "draft.txt", "", "draft\n")
	e.SessionID, e.RoundID, e.Edits[0].Before, e.Edits[0].WholeFile = "other", "round1", nil, true
	if decision, err := service.BeforeEdit(t.Context(), e, "", "block"); err != nil || decision.DenyTool == "" {
		t.Fatal("another session inherited old block allowance", decision, err)
	}
}

func TestSA03WF09ConfirmedModuleAndDraftContinueWhileAdditionPending(t *testing.T) {
	service, task, message := semiFixture(t, "导出模块按这个做，直接改；分析时可写 notes/draft.md")
	confirmed := confirmPlan(t, service, firstPlan(), message, "v1")
	if decision := prePlan(t, service, task.ID, "companion-test", "tests/export_test.go"); decision.DenyTool != "" {
		t.Fatal("companion test required individual plan", decision)
	}
	if decision := prePlan(t, service, task.ID, "helper", "internal/export/helpers/value.go"); decision.DenyTool != "" {
		t.Fatal(decision)
	}
	if decision := prePlan(t, service, task.ID, "sibling", "internal/exporter/other.go"); decision.DenyTool == "" {
		t.Fatal("directory matched sibling", decision)
	}
	second := firstPlan()
	second.BaseRevision = confirmed.View.Revision
	second.Files = []ledger.PlanFile{{Path: "internal/log/", Change: "add", Note: "新增日志功能"}}
	if _, err := service.SavePlan(t.Context(), taskActor("v2-save"), second); err != nil {
		t.Fatal(err)
	}
	if decision := prePlan(t, service, task.ID, "old-scope", "internal/export/next.go"); decision.DenyTool != "" {
		t.Fatal("pending addition blocked effective range", decision)
	}
	if decision := prePlan(t, service, task.ID, "new-scope", "internal/log/log.go"); decision.DenyTool == "" {
		t.Fatal("pending range authorized", decision)
	}
	note := ledger.NoteInput{Kind: "decision", Text: "用户允许分析草稿", SourceID: message.ID, Quote: "分析时可写 notes/draft.md", Target: &ledger.DecisionTarget{Kind: "task", ID: task.ID, At: "notes/draft.md", Effect: "allow_edit", Scope: "task"}}
	addNote(t, service, taskActor("draft-grant"), note)
	if decision := prePlan(t, service, task.ID, "draft", "notes/draft.md"); decision.DenyTool != "" {
		t.Fatal(decision)
	}
	if decision := prePlan(t, service, task.ID, "ungranted-draft", "notes/other.md"); decision.DenyTool == "" {
		t.Fatal("file grant expanded to directory", decision)
	}
	note.Target.At = "./"
	if _, err := service.AddNote(t.Context(), taskActor("all-drafts"), note); err == nil {
		t.Fatal("draft grant expanded to workspace")
	}
}

func TestPlanEditRejectsStaleAuthorizationWithoutPublishing(t *testing.T) {
	service, task, message := semiFixture(t, "分析期间允许写 notes/draft.md")
	input, err := service.Store.PlanEditContext(t.Context(), service.Workspace.ID, taskActor("pre"))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := plan.BeforeEdit(service.Workspace, input, []string{"notes/draft.md"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	addNote(t, service, taskActor("grant"), ledger.NoteInput{Kind: "decision", Text: message.Text, SourceID: message.ID, Quote: message.Text, Target: &ledger.DecisionTarget{Kind: "task", ID: task.ID, At: "notes/draft.md", Effect: "allow_edit", Scope: "task"}})
	view, err := service.Store.ObservationState(t.Context(), service.Workspace.ID, []string{"notes/draft.md"})
	if err != nil {
		t.Fatal(err)
	}
	observation := ledger.Observation{WorkspaceID: service.Workspace.ID, TaskID: task.ID, At: time.Now(), Context: view.Context, RoundID: "round1"}
	record := ledger.EditRecord{WorkspaceID: service.Workspace.ID, Agent: "codex", SessionID: "session", ToolCallID: "stale-proof", PlanCheck: &proof}
	err = service.Store.SaveObservation(t.Context(), view, &observation, nil, &record)
	if err == nil || !strings.Contains(err.Error(), "方案授权已变化") {
		t.Fatal("stale scope published", err)
	}
	latest, err := service.Store.ObservationState(t.Context(), service.Workspace.ID, nil)
	if err != nil || latest.LatestID != view.LatestID || record.BeforeObservation != 0 || record.PlanCheck.Blocked {
		t.Fatal("failed transaction changed published state", latest, record, err)
	}
}

func TestS1AuthorizationInsideEditWindowRemainsUnknown(t *testing.T) {
	service, task, _ := semiFixture(t, "先分析")
	input := firstPlan()
	input.Files = []ledger.PlanFile{{Path: "new.go", Change: "add", Note: "实现辅助功能"}}
	if _, err := service.SavePlan(t.Context(), taskActor("save-window"), input); err != nil {
		t.Fatal(err)
	}
	prePlan(t, service, task.ID, "block-window", "new.go")
	e := edit("write-window", "new.go", "", "package main\nfunc helper() {}\n")
	e.RoundID = "round1"
	e.Edits[0].WholeFile, e.Edits[0].Before = true, nil
	if decision, err := service.BeforeEdit(t.Context(), e, task.ID, "block"); err != nil || decision.DenyTool != "" {
		t.Fatal(decision, err)
	}
	reply := userMessage(t, service, taskActor("window-message"), "window", "可以")
	if _, err := service.ConfirmPlan(t.Context(), taskActor("window-confirm"), ledger.PlanConfirmInput{Version: 1, SourceID: reply.ID, Quote: reply.Text}); err != nil {
		t.Fatal(err)
	}
	write(t, service.Workspace.Root, "new.go", *e.Edits[0].After)
	e.Kind, e.Result = event.ToolEnd, &event.ToolResult{State: event.Completed}
	if _, err := service.AfterEdit(t.Context(), e, task.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.Check(t.Context(), taskActor("window-check"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ledger.CheckPartial || !slices.ContainsFunc(result.Missing, func(gap ledger.CheckGap) bool { return gap.Operation == "S1" && gap.At == "new.go" }) {
		t.Fatal("window ordering invented", result)
	}
	for _, issue := range result.Issues {
		if issue.Rule == "S1" && issue.BlockOnDelivery {
			t.Fatal("unknown ordering blocked", issue)
		}
	}
}

func TestPlanRevocationRemovesEntryEvidenceAndDoesNotRestoreOldVersion(t *testing.T) {
	service, task, message := semiFixture(t, "方案按这个做，直接改")
	input := firstPlan()
	input.Files = []ledger.PlanFile{{Path: "main.go", Change: "modify", Note: "导出入口"}}
	confirmed := confirmPlan(t, service, input, message, "entry-plan")
	verifiedTaskEdit(t, service, task.ID, "entry-edit", "main.go", "package main\nfunc main() {}\n", "package main\nimport \"flag\"\nfunc main() { _ = flag.String(\"format\", \"csv\", \"format\") }\n")
	locations := []ledger.Location{{At: "flag:--format", Purpose: "选择导出格式"}}
	saveRequirement(t, service, taskActor("entry-req"), ledger.RequirementInput{Content: "选择导出格式", Acceptance: "支持 CSV", SourceID: confirmed.Plan.Confirmation.SourceID, Quote: "选择导出格式", ImplementedBy: &locations})
	note := ledger.NoteInput{Kind: "decision", Text: "保留入口", SourceID: confirmed.Plan.Confirmation.SourceID, Quote: "选择导出格式", Target: &ledger.DecisionTarget{Kind: "plan", ID: confirmed.Plan.ID, At: "flag:--format", Effect: "retain", Scope: "task"}}
	addNote(t, service, taskActor("entry-retain"), note)
	result, err := service.Check(t.Context(), taskActor("entry-check"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.Rule == "D3" }) {
		t.Fatal("confirmed entry not recognized", result.Issues)
	}
	reply := userMessage(t, service, taskActor("entry-revoke-message"), "entry-revoke", "撤回 format 入口授权")
	addNote(t, service, taskActor("entry-revoke"), ledger.NoteInput{Kind: "decision", Text: reply.Text, SourceID: reply.ID, Quote: reply.Text, Target: &ledger.DecisionTarget{Kind: "plan", ID: confirmed.Plan.ID, At: "flag:--format", Effect: "revoke"}})
	view := getTask(t, service, taskActor("entry-read"), task.ID)
	if len(view.Plans.Effective.Entries) != 0 || len(view.EffectiveDecisions) != 0 {
		t.Fatal("withdrawn plan still grants retain", view.Plans, view.EffectiveDecisions)
	}
	if _, err := service.AddNote(t.Context(), taskActor("stale-retain"), note); err == nil {
		t.Fatal("revoked source granted new retention")
	}
	result, err = service.Check(t.Context(), taskActor("entry-recheck"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "D3" && issue.BlockOnDelivery && issue.Status == "open"
	}) {
		t.Fatal("revoked source still authorizes entry", result.Issues)
	}
	second, err := service.SavePlan(t.Context(), taskActor("new-plan"), ledger.PlanInput{Content: "保留内部导出实现，不新增入口。", BaseRevision: view.Plans.Revision})
	if err != nil {
		t.Fatal(err)
	}
	approval := userMessage(t, service, taskActor("new-plan-message"), "approve-v2", "可以")
	if _, err := service.ConfirmPlan(t.Context(), taskActor("new-plan-confirm"), ledger.PlanConfirmInput{Version: 2, SourceID: approval.ID, Quote: approval.Text}); err != nil {
		t.Fatal(err)
	}
	revoke := userMessage(t, service, taskActor("whole-revoke-message"), "whole-revoke", "撤回整版方案")
	addNote(t, service, taskActor("whole-revoke"), ledger.NoteInput{Kind: "decision", Text: revoke.Text, SourceID: revoke.ID, Quote: revoke.Text, Target: &ledger.DecisionTarget{Kind: "plan", ID: second.Plan.ID, Effect: "revoke"}})
	view = getTask(t, service, taskActor("whole-read"), task.ID)
	if view.Plans.Effective != nil || len(view.Plans.Versions) != 2 {
		t.Fatal("whole revocation resurrected v1", view.Plans)
	}
}

func TestS1LateConfirmationKeepsActualWriteHistory(t *testing.T) {
	service, task, _ := semiFixture(t, "先分析")
	if decision := prePlan(t, service, task.ID, "first-block", "new.go"); decision.DenyTool == "" {
		t.Fatal(decision)
	}
	verifiedTaskEdit(t, service, task.ID, "second-attempt", "new.go", "", "package main\nfunc helper() {}\n")
	input := firstPlan()
	input.Files = []ledger.PlanFile{{Path: "new.go", Change: "add", Note: "实现已核对的辅助功能"}}
	saved, err := service.SavePlan(t.Context(), taskActor("save-late"), input)
	if err != nil {
		t.Fatal(err)
	}
	reply := userMessage(t, service, taskActor("late-message"), "late", "可以")
	confirmed, err := service.ConfirmPlan(t.Context(), taskActor("late-confirm"), ledger.PlanConfirmInput{Version: 1, SourceID: reply.ID, Quote: reply.Text})
	if err != nil || !confirmed.Plan.Confirmation.AuthorizedAt.After(saved.Plan.SubmittedAt) {
		t.Fatal(confirmed, err)
	}
	result, err := service.Check(t.Context(), taskActor("history"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "S1" && issue.At == "new.go" && issue.BlockOnDelivery && strings.Contains(strings.Join(issue.Facts, ""), "实际写入")
	}) {
		t.Fatal("late approval erased past write", result.Issues)
	}
	if slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.Rule == "S2" && issue.At == "new.go" }) {
		t.Fatal("confirmed current scope still rejected", result.Issues)
	}
	if result.Report.Plans.Effective == nil || !strings.Contains(check.Format(result, 0), confirmed.Plan.Path) {
		t.Fatal("report lacks effective plan")
	}
}

func TestSA02UserUpdatesAndAutoEditsDoNotNeedPlan(t *testing.T) {
	service, task, _ := semiFixture(t, "先分析")
	write(t, service.Workspace.Root, "user.go", "package main\nfunc user() {}\n")
	result, err := service.Check(t.Context(), taskActor("user-check"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.Rule == "S1" || issue.Rule == "S2" }) {
		t.Fatal("user edit treated as Agent", result.Issues)
	}
	startTask(t, service, taskActor("to-auto"), ledger.StartTaskInput{TaskID: task.ID, Workflow: "auto"})
	verifiedTaskEdit(t, service, task.ID, "auto-edit", "auto.go", "", "package main\nfunc automatic() {}\n")
	startTask(t, service, taskActor("to-semi"), ledger.StartTaskInput{TaskID: task.ID, Workflow: "semi"})
	result, err = service.Check(t.Context(), taskActor("switched-check"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool { return issue.Rule == "S1" || issue.Rule == "S2" }) {
		t.Fatal("mode change retroactively demanded approval", result.Issues)
	}
}

func TestS2RevokedScopeDependenciesAndExcludedFiles(t *testing.T) {
	service, task, message := semiFixture(t, "方案按这个做，直接改")
	input := firstPlan()
	input.Files = []ledger.PlanFile{{Path: "./", Change: "modify", Note: "实现导出及验证"}}
	confirmed := confirmPlan(t, service, input, message, "v1")
	verifiedTaskEdit(t, service, task.ID, "main", "main.go", "package main\nfunc main() {}\n", "package main\nfunc main() { println(1) }\n")
	verifiedTaskEdit(t, service, task.ID, "manifest", "go.mod", "module example.test/demo\n\ngo 1.26.0\n", "module example.test/demo\n\ngo 1.26.0\n\nrequire example.test/extra v1.0.0\n")
	verifiedTaskEdit(t, service, task.ID, "test", "extra_test.go", "", "package main\n")
	verifiedTaskEdit(t, service, task.ID, "lock", "go.sum", "", "example.test/extra v1.0.0 h1:fake\n")
	verifiedTaskEdit(t, service, task.ID, "generated", "generated.go", "", "// Code generated by test. DO NOT EDIT.\npackage main\n")
	result, err := service.Check(t.Context(), taskActor("deps"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "S2" && issue.At == "dep:example.test/extra" && issue.BlockOnDelivery
	}) {
		t.Fatal("unapproved dependency accepted", result.Issues)
	}
	reply := userMessage(t, service, taskActor("revoke-message"), "revoke", "撤回这版方案")
	addNote(t, service, taskActor("revoke"), ledger.NoteInput{Kind: "decision", Text: reply.Text, SourceID: reply.ID, Quote: reply.Text, Target: &ledger.DecisionTarget{Kind: "plan", ID: confirmed.Plan.ID, Effect: "revoke"}})
	view := getTask(t, service, taskActor("read"), task.ID)
	if view.Plans.Effective != nil || view.Plans.Revision != confirmed.View.Revision+1 {
		t.Fatal("revoke did not invalidate plan", view.Plans)
	}
	result, err = service.Check(t.Context(), taskActor("scope"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "S2" && issue.At == "main.go" && issue.BlockOnDelivery
	}) {
		t.Fatal("revoked range accepted", result.Issues)
	}
	for _, issue := range result.Issues {
		if issue.Rule == "S1" || issue.Rule == "S2" && (issue.At == "extra_test.go" || issue.At == "go.sum" || issue.At == "generated.go") {
			t.Fatal("past approval or excluded file rejected", issue)
		}
	}
	write(t, service.Workspace.Root, "main.go", "package main\nfunc main() { println(2) }\n")
	result, err = service.Check(t.Context(), taskActor("mixed"), check.RunInput{Depth: "static"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(result.Issues, func(issue ledger.Issue) bool {
		return issue.Rule == "S2" && issue.At == "main.go" && issue.BlockOnDelivery
	}) {
		t.Fatal("user replacement still attributed to Agent", result.Issues)
	}
	if _, err := service.Finish(t.Context(), taskActor("scope-finish"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	if decision := stopTask(t, service); !strings.Contains(decision.BlockStop, "S2") {
		t.Fatal("S2 did not enter delivery feedback", decision)
	}
	if decision := stopTask(t, service); decision.BlockStop != "" || !strings.Contains(decision.UserMessage, "S2") {
		t.Fatal("unchanged second Stop repeated block or hid scope fact", decision)
	}
}

func firstPlan() ledger.PlanInput {
	return ledger.PlanInput{Content: "原因：实现导出。\n成功标准：可导出 CSV。\n做法：沿用已有写出逻辑。\n步骤：接线并验证。", Files: []ledger.PlanFile{{Path: "internal/export/", Change: "modify", Note: "实现 CSV 导出"}}, Entries: []ledger.PlanEntry{{At: "flag:--format", Note: "选择导出格式"}}, Dependencies: []ledger.PlanDependency{}}
}

func TestSA05PlanFilesVersionsAndPendingAdditionsPreserveEffectiveScope(t *testing.T) {
	service, task, message := semiFixture(t, "先分析导出方案")
	input := firstPlan()
	saved, err := service.SavePlan(t.Context(), taskActor("save1"), input)
	if err != nil || saved.Plan == nil || saved.Plan.Version != 1 || saved.View.Effective != nil || saved.View.Pending == nil {
		t.Fatal(saved, err)
	}
	if !strings.HasPrefix(saved.Plan.Path, service.Store.DataDir()+string(filepath.Separator)) || strings.HasPrefix(saved.Plan.Path, service.Workspace.Root+string(filepath.Separator)) {
		t.Fatal("plan written outside data directory", saved.Plan.Path)
	}
	contents, err := os.ReadFile(saved.Plan.Path)
	if err != nil || !strings.Contains(string(contents), "涉及的改动") || !strings.Contains(string(contents), "internal/export/") {
		t.Fatal(string(contents), err)
	}
	if retried, err := service.SavePlan(t.Context(), taskActor("save1"), input); err != nil || retried.Plan.Version != 1 {
		t.Fatal("plan retry appended version", retried, err)
	}
	if _, err := service.ConfirmPlan(t.Context(), taskActor("early"), ledger.PlanConfirmInput{Version: 1, SourceID: message.ID, Quote: message.Text}); err == nil {
		t.Fatal("discussion before plan approved execution")
	}
	a := taskActor("reply")
	a.RoundID = "round2"
	reply := userMessage(t, service, a, "m2", "可以，但日志先不动")
	a.CallID = "confirm1"
	confirmed, err := service.ConfirmPlan(t.Context(), a, ledger.PlanConfirmInput{Version: 1, SourceID: reply.ID, Quote: "可以"})
	if err != nil || confirmed.View.Effective == nil || confirmed.Plan.Confirmation.Message.Text != reply.Text || confirmed.Plan.Confirmation.Method != "reply" {
		t.Fatal(confirmed, err)
	}
	if confirmed.Plan.Confirmation.AuthorizedAt.Before(saved.Plan.SubmittedAt) {
		t.Fatal("confirmation time rewritten")
	}
	second := ledger.PlanInput{Content: input.Content + "\n增加 JSON 格式。", Files: []ledger.PlanFile{{Path: "internal/json/", Change: "add", Note: "实现 JSON 写出"}}, BaseRevision: confirmed.View.Revision}
	a.CallID = "save2"
	added, err := service.SavePlan(t.Context(), a, second)
	if err != nil || added.Plan.Version != 2 || len(added.Plan.Files) != 2 || added.View.Effective.Version != 1 || added.View.Pending.Version != 2 {
		t.Fatal(added, err)
	}
	if len(added.Plan.Entries) != 1 || added.Plan.Entries[0].At != "flag:--format" {
		t.Fatal("confirmed entry silently dropped", added.Plan)
	}
	a.CallID = "stale"
	_, err = service.SavePlan(t.Context(), a, second)
	var conflict *ledger.PlanConflict
	if !errors.As(err, &conflict) || conflict.Latest.Pending.Version != 2 {
		t.Fatal("stale plan did not return latest", err)
	}
	view := getTask(t, service, a, task.ID)
	if len(view.Plans.Versions) != 2 || view.Task.PlanRevision != 3 {
		t.Fatal(view.Plans, view.Task)
	}
}

func TestSA06AttachedRequestAndNegativeReplies(t *testing.T) {
	service, _, message := semiFixture(t, "方案按 A 做，直接改")
	saved, err := service.SavePlan(t.Context(), taskActor("save"), firstPlan())
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := service.ConfirmPlan(t.Context(), taskActor("confirm"), ledger.PlanConfirmInput{Version: saved.Plan.Version, SourceID: message.ID, Quote: message.Text})
	if err != nil || confirmed.Plan.Confirmation.Method != "with_request" || !confirmed.Plan.Confirmation.AuthorizedAt.Equal(message.At) {
		t.Fatal(confirmed, err)
	}
	service, _, _ = semiFixture(t, "先分析")
	saved, err = service.SavePlan(t.Context(), taskActor("save"), firstPlan())
	if err != nil {
		t.Fatal(err)
	}
	a := taskActor("message2")
	a.RoundID = "round2"
	reply := userMessage(t, service, a, "m2", "继续分析，不要开始")
	a.CallID = "confirm"
	if _, err := service.ConfirmPlan(t.Context(), a, ledger.PlanConfirmInput{Version: 1, SourceID: reply.ID, Quote: reply.Text}); err == nil {
		t.Fatal("negative reply became confirmation")
	}
	view := getTask(t, service, a, "")
	if view.Plans.Effective != nil || view.Plans.Pending == nil {
		t.Fatal(view.Plans)
	}
}

func TestAutoPlanToolsDoNotCreateApprovalAndRetryKeepsOriginalMode(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	result, err := service.SavePlan(t.Context(), taskActor("auto-plan"), ledger.PlanInput{})
	if err != nil || result.Workflow != "auto" || result.Plan != nil {
		t.Fatal(result, err)
	}
	if _, err := os.Stat(filepath.Join(service.Store.DataDir(), "plans")); !os.IsNotExist(err) {
		t.Fatal("auto plan tool created files", err)
	}
	a := taskActor("switch")
	changed := startTask(t, service, a, ledger.StartTaskInput{TaskID: task.ID, Workflow: "semi"})
	if len(changed.WorkflowHistory) < 2 || changed.WorkflowHistory[len(changed.WorkflowHistory)-1].To != "semi" || changed.WorkflowHistory[len(changed.WorkflowHistory)-1].Tree == "" {
		t.Fatal("mode switch lost snapshot", changed)
	}
	result, err = service.SavePlan(t.Context(), taskActor("auto-plan"), ledger.PlanInput{})
	if err != nil || result.Workflow != "auto" || result.Plan != nil {
		t.Fatal("auto reply retry became a new approval", result, err)
	}
}
