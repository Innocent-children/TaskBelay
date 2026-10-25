package eval_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func taskActor(call string) ledger.Actor {
	return ledger.Actor{Agent: "codex", SessionID: "session", RoundID: "round1", CallID: call}
}
func userMessage(t *testing.T, s hook.Tasks, actor ledger.Actor, id, text string) ledger.Message {
	t.Helper()
	message, err := s.RecordUserMessage(t.Context(), event.Event{Kind: event.UserPrompt, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, MessageID: id, Message: text})
	if err != nil {
		t.Fatal(err)
	}
	return message
}
func startTask(t *testing.T, s hook.Tasks, actor ledger.Actor, input ledger.StartTaskInput) ledger.TaskRecord {
	t.Helper()
	task, err := s.Start(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	return task
}
func getTask(t *testing.T, s hook.Tasks, actor ledger.Actor, id string) ledger.TaskView {
	t.Helper()
	view, err := s.Get(t.Context(), actor, id)
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func saveRequirement(t *testing.T, s hook.Tasks, actor ledger.Actor, input ledger.RequirementInput) ledger.Requirement {
	t.Helper()
	req, err := s.SaveRequirement(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
func addNote(t *testing.T, s hook.Tasks, actor ledger.Actor, input ledger.NoteInput) ledger.Note {
	t.Helper()
	note, err := s.AddNote(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	return note
}
func taskCall(t *testing.T, s hook.Tasks, actor ledger.Actor, operation string, input any) ledger.Call {
	t.Helper()
	call, err := ledger.NewCall(s.Workspace.ID, actor, operation, input)
	if err != nil {
		t.Fatal(err)
	}
	return call
}
func transition(t *testing.T, s hook.Tasks, actor ledger.Actor, id string, state ledger.TaskState) ledger.TaskRecord {
	t.Helper()
	result, err := s.Store.TransitionTask(t.Context(), taskCall(t, s, actor, "transition", state), id, state, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTaskOriginUsesFirstMessageSnapshotAndExplicitOverride(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("start")
	write(t, s.Workspace.Root, "dirty.txt", "before request\n")
	m1 := userMessage(t, s, a, "prompt1", "第一条需求")
	write(t, s.Workspace.Root, "dirty.txt", "changed after first message\n")
	userMessage(t, s, a, "prompt2", "补充验收条件")
	task := startTask(t, s, a, ledger.StartTaskInput{Title: "A"})
	if task.StartMessageID != m1.ID || task.OriginObservationID != m1.ObservationID || task.OriginObservationID == task.ResumeObservationID {
		t.Fatalf("wrong origin: %+v", task)
	}
	initial := getTask(t, s, a, "")
	if len(initial.WorkspaceUpdates) != 1 || initial.WorkspaceUpdates[0].Path != "dirty.txt" {
		t.Fatal("changes between origin message and task start were lost")
	}
	origin, err := s.Store.Observation(t.Context(), task.OriginObservationID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := s.Workspace.ReadBlob(t.Context(), origin.Files["dirty.txt"].Blob)
	if err != nil || string(content) != "before request\n" {
		t.Fatalf("dirty origin: %q %v", content, err)
	}
	duplicate := userMessage(t, s, a, "prompt1", "第一条需求")
	if duplicate.ObservationID != m1.ObservationID {
		t.Fatal("retried message moved origin")
	}
	a.RoundID = "round2"
	a.CallID = "startB"
	m2 := userMessage(t, s, a, "prompt3", "另一件事 B")
	userMessage(t, s, a, "prompt4", "B 的补充")
	b := startTask(t, s, a, ledger.StartTaskInput{Title: "B"})
	if b.StartMessageID != m2.ID {
		t.Fatalf("new request used old message: %+v", b)
	}
	a.CallID = "startC"
	c := startTask(t, s, a, ledger.StartTaskInput{Title: "C", StartMessageID: m1.ID})
	if c.OriginTree != task.OriginTree {
		t.Fatal("explicit origin was ignored")
	}
	before := getTask(t, s, a, "")
	if _, err := s.Start(t.Context(), taskActor("start"), ledger.StartTaskInput{Title: "A"}); err != nil {
		t.Fatal(err)
	}
	after := getTask(t, s, a, "")
	if after.CurrentTaskID != c.ID || len(after.Candidates) != len(before.Candidates) {
		t.Fatal("retry created or switched a task")
	}
}

func TestWF05PauseResumeAcrossAgentsKeepsOriginAndWorkspaceChanges(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("startA")
	write(t, s.Workspace.Root, "work.txt", "start\n")
	m := userMessage(t, s, a, "a", "做 A，保留这个参数")
	taskA := startTask(t, s, a, ledger.StartTaskInput{Title: "A"})
	a.CallID = "r1"
	req := saveRequirement(t, s, a, ledger.RequirementInput{Content: "做 A", Acceptance: "结果 A", SourceID: m.ID, Quote: "做 A"})
	a.CallID = "keep"
	decision := addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "保留参数", SourceID: m.ID, Quote: "保留这个参数", Target: &ledger.DecisionTarget{Kind: "requirement", ID: req.ID, At: "flag:--mode", Effect: "retain", Scope: "task"}})
	a.RoundID = "round2"
	a.CallID = "startB"
	userMessage(t, s, a, "b", "先做 B")
	taskB := startTask(t, s, a, ledger.StartTaskInput{Title: "B"})
	e := edit("b-edit", "work.txt", "start\n", "B changes\n")
	if _, err := s.BeforeEdit(t.Context(), e, taskB.ID, "block"); err != nil {
		t.Fatal(err)
	}
	write(t, s.Workspace.Root, "work.txt", "B changes\n")
	if _, err := s.AfterEdit(t.Context(), e, taskB.ID); err != nil {
		t.Fatal(err)
	}
	a.CallID = "pendingB"
	transition(t, s, a, taskB.ID, ledger.TaskPending)
	a.CallID = "completeB"
	transition(t, s, a, taskB.ID, ledger.TaskCompleted)
	c := ledger.Actor{Agent: "claude", SessionID: "next-agent", RoundID: "resume", CallID: "resumeA"}
	userMessage(t, s, c, "continue", "继续 A")
	resumed := startTask(t, s, c, ledger.StartTaskInput{TaskID: taskA.ID})
	if resumed.OriginObservationID != taskA.OriginObservationID || resumed.ResumeObservationID == taskA.ResumeObservationID {
		t.Fatalf("resume changed origin or reused observation: %+v", resumed)
	}
	view := getTask(t, s, c, "")
	if len(view.EffectiveDecisions) != 1 || view.EffectiveDecisions[0].ID != decision.ID || view.EffectiveDecisions[0].Target.At != "flag:--mode" {
		t.Fatalf("decision scope lost: %+v", view.EffectiveDecisions)
	}
	found := false
	for _, change := range view.WorkspaceUpdates {
		if change.Path == "work.txt" && change.After.Blob != "" && change.Kind == "workspace_update" {
			found = true
		}
	}
	if !found {
		t.Fatal("B changes missing from A resume updates")
	}
	if string(read(t, filepath.Join(s.Workspace.Root, "work.txt"))) != "B changes\n" {
		t.Fatal("resume restored old code")
	}
	global := changes(t, s.Observer)
	agentEdits := 0
	for _, change := range global {
		if change.Kind == "agent_edit" {
			agentEdits++
			if change.ToolCallID != "b-edit" {
				t.Fatal("resume invented agent edit")
			}
		}
	}
	if agentEdits != 1 {
		t.Fatalf("original B attribution lost: %d", agentEdits)
	}
	c.CallID = "reopenB"
	reopened := startTask(t, s, c, ledger.StartTaskInput{TaskID: taskB.ID, Workflow: "semi"})
	if reopened.State != ledger.TaskActive || reopened.EndedAt != nil || reopened.Workflow != "semi" || reopened.OriginTree != taskB.OriginTree {
		t.Fatalf("reopen: %+v", reopened)
	}
}

func TestWF06CommitBranchStashAndAmendKeepTaskBoundaries(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	dir := s.Workspace.Root
	write(t, dir, "work.txt", "base\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	git(t, dir, "branch", "-M", "main")
	a := taskActor("main")
	userMessage(t, s, a, "main", "main task")
	mainTask := startTask(t, s, a, ledger.StartTaskInput{Title: "main"})
	write(t, dir, "work.txt", "main changes\n")
	before := getTask(t, s, a, "")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "work")
	git(t, dir, "commit", "--amend", "--no-edit")
	committed := getTask(t, s, a, "")
	if committed.Task.ID != mainTask.ID || committed.Task.OriginTree != mainTask.OriginTree || committed.Task.RecordRevision != before.Task.RecordRevision {
		t.Fatal("commit/amend reset task or invalidated unchanged content")
	}
	git(t, dir, "checkout", "-qb", "feature")
	a.RoundID = "feature"
	a.CallID = "feature"
	message := userMessage(t, s, a, "feature", "feature task")
	unknown := getTask(t, s, a, "")
	if unknown.CurrentTaskID != "" || len(unknown.Candidates) != 1 || unknown.Candidates[0].State != ledger.TaskPaused {
		t.Fatalf("unmatched branch: %+v", unknown)
	}
	feature := startTask(t, s, a, ledger.StartTaskInput{Title: "feature", StartMessageID: message.ID})
	write(t, dir, "work.txt", "stashed\n")
	getTask(t, s, a, "")
	git(t, dir, "stash", "push", "-qm", "fixture")
	stashed := getTask(t, s, a, "")
	git(t, dir, "stash", "pop", "--quiet")
	restored := getTask(t, s, a, "")
	if stashed.Task.ID != feature.ID || restored.Task.OriginTree != feature.OriginTree || len(restored.WorkspaceUpdates) <= len(stashed.WorkspaceUpdates) {
		t.Fatal("stash ended task or lost updates")
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "feature")
	getTask(t, s, a, "")
	git(t, dir, "checkout", "-q", "main")
	back := getTask(t, s, a, "")
	if back.CurrentTaskID != mainTask.ID || back.Task.OriginTree != mainTask.OriginTree {
		t.Fatalf("branch did not restore main: %+v", back.Task)
	}
	write(t, dir, "main.txt", "main upstream\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "main update")
	getTask(t, s, a, "")
	git(t, dir, "checkout", "-q", "feature")
	getTask(t, s, a, "")
	git(t, dir, "rebase", "main")
	rebased := getTask(t, s, a, "")
	if rebased.Task.ID != feature.ID || rebased.Task.OriginTree != feature.OriginTree {
		t.Fatal("rebase changed task identity")
	}
	upstream := t.TempDir()
	git(t, upstream, "clone", "-q", dir, ".")
	write(t, upstream, "pull.txt", "remote change\n")
	git(t, upstream, "add", ".")
	git(t, upstream, "commit", "-qm", "pull update")
	git(t, dir, "pull", "--ff-only", upstream, "feature")
	pulled := getTask(t, s, a, "")
	seen := map[string]bool{}
	for _, change := range pulled.WorkspaceUpdates {
		seen[change.Path] = true
	}
	if pulled.Task.ID != feature.ID || pulled.Task.OriginTree != feature.OriginTree || !seen["main.txt"] || !seen["pull.txt"] {
		t.Fatal("pull/rebase lost workspace updates")
	}
}

func TestBranchAmbiguityKeepsCandidatesForAgentChoice(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	dir := s.Workspace.Root
	write(t, dir, "a.txt", "base")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	git(t, dir, "branch", "-M", "main")
	a := taskActor("a")
	userMessage(t, s, a, "a", "任务 A")
	startTask(t, s, a, ledger.StartTaskInput{Title: "A"})
	a.CallID = "b"
	startTask(t, s, a, ledger.StartTaskInput{Title: "B"})
	git(t, dir, "checkout", "-qb", "other")
	getTask(t, s, a, "")
	git(t, dir, "checkout", "-q", "main")
	view := getTask(t, s, a, "")
	if view.CurrentTaskID != "" || len(view.Candidates) != 2 || view.Candidates[0].State != ledger.TaskPaused || view.Candidates[1].State != ledger.TaskPaused {
		t.Fatal("ambiguous branch guessed a task")
	}
}

func TestFirstRequirementAfterTaskEndUsesNewSessionMessage(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("old")
	userMessage(t, s, a, "old", "旧任务")
	old := startTask(t, s, a, ledger.StartTaskInput{Title: "old"})
	a.CallID = "pending"
	transition(t, s, a, old.ID, ledger.TaskPending)
	a.CallID = "complete"
	transition(t, s, a, old.ID, ledger.TaskCompleted)
	a.RoundID = "next"
	a.CallID = "first-requirement"
	first := userMessage(t, s, a, "next1", "新任务第一条")
	userMessage(t, s, a, "next2", "新任务补充")
	r := saveRequirement(t, s, a, ledger.RequirementInput{Content: "新目标", Acceptance: "新验收"})
	view := getTask(t, s, a, "")
	if r.TaskID == old.ID || view.Task.StartMessageID != first.ID || view.Task.OriginObservationID != first.ObservationID {
		t.Fatalf("old task messages became new origin: %+v", view.Task)
	}
}

func TestWF04LocationsMoveDeleteAndRetainHistory(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("task")
	write(t, s.Workspace.Root, "old.txt", "unique old content\n")
	write(t, s.Workspace.Root, "removed.go", "package p\nfunc Old() {}\n")
	m := userMessage(t, s, a, "request", "移动文件，删除旧方法")
	startTask(t, s, a, ledger.StartTaskInput{Title: "locations"})
	a.CallID = "r1"
	locations := []ledger.Location{{At: "old.txt", Purpose: "移动文档", Change: "move"}, {At: "removed.go:Old", Change: "delete"}, {At: "test:removed_test.go:TestOld/sub"}, {At: "flag:--mode"}, {At: "cmd:export"}, {At: "env:OUTPUT"}, {At: "route:GET /export"}, {At: "config:export.mode"}, {At: "dep:module/name"}}
	if err := os.Remove(filepath.Join(s.Workspace.Root, "removed.go")); err != nil {
		t.Fatal(err)
	}
	req := saveRequirement(t, s, a, ledger.RequirementInput{Content: "移动和删除", Acceptance: "旧方法不存在", SourceID: m.ID, Quote: "删除旧方法", ImplementedBy: &locations})
	if req.ImplementedBy[1].BeforeBlob == "" {
		t.Fatal("deleted source was lost")
	}
	old, err := s.Workspace.ReadBlob(t.Context(), req.ImplementedBy[1].BeforeBlob)
	if err != nil || string(old) != "package p\nfunc Old() {}\n" {
		t.Fatalf("deleted content=%q %v", old, err)
	}
	if err := os.Rename(filepath.Join(s.Workspace.Root, "old.txt"), filepath.Join(s.Workspace.Root, "new.txt")); err != nil {
		t.Fatal(err)
	}
	view := getTask(t, s, a, "")
	if view.Requirements[0].ImplementedBy[0].At != "new.txt" || view.History[0].ImplementedBy[0].At != "old.txt" {
		t.Fatal("move did not preserve current/history distinction")
	}
	write(t, s.Workspace.Root, "new.txt", "content changed after the observed move\n")
	view = getTask(t, s, a, "")
	if view.Requirements[0].ImplementedBy[0].At != "new.txt" || view.Requirements[0].ImplementedBy[0].Pending != "" {
		t.Fatal("later edit lost the observed move")
	}
	a.CallID = "update"
	updated := saveRequirement(t, s, a, ledger.RequirementInput{ID: req.ID, Content: req.Content, Acceptance: req.Acceptance, SourceID: m.ID, Quote: "删除旧方法", BaseRevision: &req.Revision, Status: "in_progress"})
	if len(updated.ImplementedBy) != 9 {
		t.Fatal("omitted positions were cleared")
	}
	a.CallID = "clear"
	empty := []ledger.Location{}
	cleared := saveRequirement(t, s, a, ledger.RequirementInput{ID: req.ID, Content: req.Content, Acceptance: req.Acceptance, BaseRevision: &updated.Revision, ImplementedBy: &empty})
	view = getTask(t, s, a, "")
	if len(cleared.ImplementedBy) != 0 || len(view.History) != 3 || len(view.History[0].ImplementedBy) != 9 {
		t.Fatal("position replacement overwrote history")
	}
}

func TestWF07RequirementsAndDecisionsExpireWithoutBroadExemptions(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("save")
	m := userMessage(t, s, a, "user", "可以保留该参数；本轮先不检查；只重跑一次。")
	r := saveRequirement(t, s, a, ledger.RequirementInput{Content: "支持参数", Acceptance: "参数可用", SourceID: m.ID, Quote: "可以"})
	view := getTask(t, s, a, "")
	taskID := view.CurrentTaskID
	if r.ID != "R1" || r.Kind != "sourced" || view.Task.StartMessageID != m.ID {
		t.Fatal("first requirement did not create sourced task")
	}
	a.CallID = "inferred"
	inferred := saveRequirement(t, s, a, ledger.RequirementInput{Content: "另加新功能", Acceptance: "可讨论"})
	if inferred.Kind != "inferred" || inferred.ID != "R2" || inferred.Citation != nil {
		t.Fatal("inference acquired user authorization")
	}
	a.CallID = "keep"
	addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "只保留参数", Quote: "保留该参数", SourceID: m.ID, Target: &ledger.DecisionTarget{Kind: "requirement", ID: r.ID, At: "flag:--format", Effect: "retain", Scope: "task"}})
	a.CallID = "pause"
	addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "本轮暂停", Quote: "本轮先不检查", SourceID: m.ID, Target: &ledger.DecisionTarget{Kind: "task", ID: taskID, Effect: "pause_checks", Scope: "round"}})
	a.CallID = "pending"
	transition(t, s, a, taskID, ledger.TaskPending)
	before := getTask(t, s, a, "")
	a.CallID = "plain"
	addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "实现原因的普通说明"})
	plain := getTask(t, s, a, "")
	if plain.Task.State != ledger.TaskPending || plain.Task.AuthorizationRevision != before.Task.AuthorizationRevision || plain.Task.RecordRevision != before.Task.RecordRevision {
		t.Fatal("plain note invalidated checks or exempted a rule")
	}
	a.CallID = "revoke"
	saveRequirement(t, s, a, ledger.RequirementInput{ID: r.ID, Content: r.Content, Acceptance: r.Acceptance, Status: "revoked", BaseRevision: &r.Revision, SourceID: m.ID, Quote: "可以"})
	revoked := getTask(t, s, a, "")
	if len(revoked.EffectiveDecisions) != 1 || revoked.EffectiveDecisions[0].Target.Effect != "pause_checks" || revoked.Task.State != ledger.TaskActive {
		t.Fatal("revoked requirement kept authorization")
	}
	a.RoundID = "round2"
	a.CallID = "rerun"
	userMessage(t, s, a, "next", "继续")
	expired := getTask(t, s, a, "")
	if len(expired.EffectiveDecisions) != 0 || expired.Task.AuthorizationRevision <= revoked.Task.AuthorizationRevision {
		t.Fatal("round decision did not expire")
	}
	verificationActor := a
	verificationActor.CallID = "original-verification"
	verification, err := s.Store.BeginVerification(t.Context(), ledger.VerificationRecord{WorkspaceID: s.Workspace.ID, TaskID: taskID, Actor: verificationActor, CommandKey: "command", RequestHash: "request", InputRevision: expired.Task.RecordRevision})
	if err != nil {
		t.Fatal(err)
	}
	grant := addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "怀疑上次运行受临时环境影响，重跑相同验证一次", Target: &ledger.DecisionTarget{Kind: "verification", ID: verification.ID, Effect: "rerun_once", Scope: "once"}})
	a.CallID = "consume"
	call := taskCall(t, s, a, "consume", map[string]string{"task": taskID, "verification": verification.ID})
	used, err := s.Store.ConsumeRerun(t.Context(), call, taskID, verification.ID)
	if err != nil || !used.Consumed || used.ID != grant.ID {
		t.Fatalf("consume=%+v %v", used, err)
	}
	if _, err := s.Store.ConsumeRerun(t.Context(), call, taskID, verification.ID); err != nil {
		t.Fatal("idempotent retry failed", err)
	}
	a.CallID = "again"
	if _, err := s.Store.ConsumeRerun(t.Context(), taskCall(t, s, a, "consume", verification.ID), taskID, verification.ID); err == nil {
		t.Fatal("authorization consumed twice")
	}
	a.CallID = "newGrant"
	addNote(t, s, a, ledger.NoteInput{Kind: "decision", Text: "再核验一次", Target: &ledger.DecisionTarget{Kind: "verification", ID: verification.ID, Effect: "rerun_once", Scope: "once"}})
	write(t, s.Workspace.Root, "new.txt", "new input")
	getTask(t, s, a, "")
	a.CallID = "changed"
	if _, err := s.Store.ConsumeRerun(t.Context(), taskCall(t, s, a, "consume", verification.ID), taskID, verification.ID); err == nil {
		t.Fatal("changed input reused old grant")
	}
}

func TestWF14RequirementConflictReturnsLatestAndKeepsBothRevisions(t *testing.T) {
	s := hook.Tasks{Observer: observer(t)}
	a := taskActor("first")
	userMessage(t, s, a, "request", "需求")
	r := saveRequirement(t, s, a, ledger.RequirementInput{Content: "目标", Acceptance: "原验收"})
	b := ledger.Actor{Agent: "claude", SessionID: "other", RoundID: "other-round", CallID: "edit-b"}
	userMessage(t, s, b, "other", "补充验收")
	a.CallID = "edit-a"
	first := saveRequirement(t, s, a, ledger.RequirementInput{ID: r.ID, Content: r.Content, Acceptance: "A 的验收", BaseRevision: &r.Revision})
	_, err := s.SaveRequirement(t.Context(), b, ledger.RequirementInput{ID: r.ID, Content: r.Content, Acceptance: "B 的验收", BaseRevision: &r.Revision})
	var conflict *ledger.RequirementConflict
	if !errors.As(err, &conflict) || conflict.Latest.Revision != first.Revision || conflict.Latest.Acceptance != "A 的验收" {
		t.Fatalf("missing latest conflict: %v", err)
	}
	b.CallID = "merge"
	merged := saveRequirement(t, s, b, ledger.RequirementInput{ID: r.ID, Content: r.Content, Acceptance: "A 的验收；B 的验收", BaseRevision: &first.Revision})
	view := getTask(t, s, b, "")
	if merged.Revision != 3 || len(view.History) != 3 || view.History[1].Acceptance != "A 的验收" {
		t.Fatal("merge lost successful changes")
	}
}
