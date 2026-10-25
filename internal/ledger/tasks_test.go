package ledger

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

func TestQuoteWhitespaceOffsetsAndFullSource(t *testing.T) {
	source := Source{ID: "m1", Text: "  可以\n\t保留　 参数，但不要增加接口。\n"}
	for _, quote := range []string{"可以", "  可以 保留 参数  ", "不要增加接口"} {
		proof, err := MatchQuote(source, quote)
		if err != nil {
			t.Fatal(err)
		}
		if proof.Start < 0 || proof.End > len(source.Text) || proof.End <= proof.Start {
			t.Fatalf("bad byte range: %+v", proof)
		}
		if _, err := MatchQuote(Source{ID: source.ID, Text: source.Text[proof.Start:proof.End]}, quote); err != nil {
			t.Fatal("quote does not point back to original", err)
		}
	}
	for _, quote := range []string{"可以增加接口", "保留参数", "可以，保留", " ", "CAN"} {
		if _, err := MatchQuote(source, quote); err == nil {
			t.Fatalf("loose match accepted %q", quote)
		}
	}
	if source.Text != "  可以\n\t保留　 参数，但不要增加接口。\n" {
		t.Fatal("full source was rewritten")
	}
}

func taskFixture(t *testing.T) (*Store, string, Actor, Observation, Message) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.RegisterWorkspace(t.Context(), "w", "p", "/fixture"); err != nil {
		t.Fatal(err)
	}
	actor := Actor{Agent: "codex", SessionID: "s", RoundID: "r", CallID: "c"}
	view, err := s.ObservationState(t.Context(), "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	obs := Observation{WorkspaceID: "w", Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, Full: true, Complete: true, Tree: "tree", SnapshotRef: "refs/taskbelay/session/o1", Files: map[string]gitstate.File{}, At: time.Now().UTC()}
	if err := s.SaveObservation(t.Context(), view, &obs, nil, nil); err != nil {
		t.Fatal(err)
	}
	m, err := s.RecordMessage(t.Context(), Message{WorkspaceID: "w", Actor: actor, EventID: "prompt", Text: "确认需求，保留参数", ObservationID: obs.ID})
	if err != nil {
		t.Fatal(err)
	}
	return s, dir, actor, obs, m
}

func callFor(t *testing.T, actor Actor, operation string, input any) Call {
	t.Helper()
	call, err := NewCall("w", actor, operation, input)
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func TestFirstRequirementFailureAndTaskSwitchAreAtomic(t *testing.T) {
	s, _, actor, obs, m := taskFixture(t)
	bad := RequirementInput{Content: "目标", Acceptance: "验收", SourceID: m.ID, Quote: "并不存在的引用"}
	if _, err := s.SaveRequirement(t.Context(), callFor(t, actor, "requirement_save", bad), "", obs.ID, bad); err == nil {
		t.Fatal("accepted invalid citation")
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial task created %d %v", count, err)
	}
	input := StartTaskInput{Title: "A"}
	call := callFor(t, actor, "task_start", input)
	a, err := s.StartTask(t.Context(), call, "", obs.ID, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	actor.CallID = "bad-switch"
	badStart := StartTaskInput{TaskID: "missing"}
	if _, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", badStart), a.ID, obs.ID, badStart, nil); err == nil {
		t.Fatal("resumed nonexistent task")
	}
	view, err := s.TaskView(t.Context(), "w", "", actor)
	if err != nil || view.CurrentTaskID != a.ID || view.Task.State != TaskActive {
		t.Fatalf("failed switch paused A: %+v %v", view, err)
	}
	actor.CallID = "taskB"
	bInput := StartTaskInput{Title: "B"}
	b, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", bInput), a.ID, obs.ID, bInput, nil)
	if err != nil {
		t.Fatal(err)
	}
	if replayed, err := s.StartTask(t.Context(), call, "", obs.ID, input, nil); err != nil || replayed.ID != a.ID {
		t.Fatalf("retry=%+v %v", replayed, err)
	}
	current, err := s.CurrentTaskID(t.Context(), "w")
	if err != nil || current != b.ID {
		t.Fatal("retry switched current task")
	}
	actor.CallID = "stale"
	cInput := StartTaskInput{Title: "C"}
	if _, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", cInput), a.ID, obs.ID, cInput, nil); err == nil {
		t.Fatal("stale current overwrote B")
	}
	changed := call
	changed.InputHash = "different"
	if _, err := s.StartTask(t.Context(), changed, b.ID, obs.ID, input, nil); !errors.Is(err, ErrCallConflict) {
		t.Fatalf("changed retry=%v", err)
	}
}

func TestTwoConnectionsRevisionCASAndReplay(t *testing.T) {
	s, dir, actor, obs, _ := taskFixture(t)
	input := RequirementInput{Content: "目标", Acceptance: "原验收"}
	r, err := s.SaveRequirement(t.Context(), callFor(t, actor, "requirement_save", input), "", obs.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	errorsByWriter := make([]error, 2)
	results := make([]Requirement, 2)
	for i, store := range []*Store{s, other} {
		wg.Go(func() {
			a := actor
			a.CallID = []string{"writer-a", "writer-b"}[i]
			update := RequirementInput{ID: r.ID, Content: r.Content, Acceptance: []string{"A", "B"}[i], BaseRevision: &r.Revision}
			call, err := NewCall("w", a, "requirement_save", update)
			if err != nil {
				errorsByWriter[i] = err
				return
			}
			results[i], errorsByWriter[i] = store.SaveRequirement(t.Context(), call, r.TaskID, obs.ID, update)
		})
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errorsByWriter {
		var conflict *RequirementConflict
		if err == nil {
			successes++
		} else if errors.As(err, &conflict) && conflict.Latest.Revision == 2 {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	view, err := s.TaskView(t.Context(), "w", r.TaskID, actor)
	if err != nil || len(view.History) != 2 {
		t.Fatalf("history=%+v %v", view.History, err)
	}
}

func TestSourceKindsAndCancellationRequireUserEvidence(t *testing.T) {
	s, _, actor, obs, m := taskFixture(t)
	input := StartTaskInput{Title: "task"}
	task, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", input), "", obs.ID, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := MatchQuote(Source{ID: m.ID, Text: m.Text}, "确认")
	if err != nil {
		t.Fatal(err)
	}
	plan := Source{ID: "plan1", WorkspaceID: "w", TaskID: task.ID, Kind: "confirmed_plan", Text: "已确认方案的完整正文", Version: 1, Confirmation: &proof}
	if err := s.RecordSource(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	changed := plan
	changed.Text = "另一份正文"
	if err := s.RecordSource(t.Context(), changed); err == nil {
		t.Fatal("source overwritten")
	}
	fake := Source{ID: "forged", WorkspaceID: "w", Kind: "user_message", Text: "用户同意"}
	if err := s.RecordSource(t.Context(), fake); err == nil {
		t.Fatal("Agent forged user message")
	}
	actor.CallID = "no-proof"
	if _, err := s.TransitionTask(t.Context(), callFor(t, actor, "cancel", "no-proof"), task.ID, TaskCanceled, "", ""); err == nil {
		t.Fatal("canceled without user source")
	}
	actor.CallID = "plan-proof"
	if _, err := s.TransitionTask(t.Context(), callFor(t, actor, "cancel", "plan"), task.ID, TaskCanceled, plan.ID, "完整正文"); err == nil {
		t.Fatal("canceled via plan rather than explicit user message")
	}
	// 引用匹配不做语义推断，取消意图由上游 Agent 根据完整消息确认。
	actor.CallID = "user-proof"
	canceled, err := s.TransitionTask(t.Context(), callFor(t, actor, "cancel", "user"), task.ID, TaskCanceled, m.ID, "确认")
	if err != nil || canceled.Cancellation == nil {
		t.Fatalf("cancellation=%+v %v", canceled, err)
	}
	actor.CallID = "reopen"
	reopen := StartTaskInput{TaskID: task.ID}
	if _, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", reopen), "", obs.ID, reopen, nil); err == nil {
		t.Fatal("reopened canceled task")
	}
}

func TestMigrateV2PreservesObservationsAndSettings(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT '');
CREATE TABLE settings(scope TEXT NOT NULL,scope_id TEXT NOT NULL,name TEXT NOT NULL,value TEXT NOT NULL,revision INTEGER NOT NULL,PRIMARY KEY(scope,scope_id,name));` + observationSchema + `
INSERT INTO workspaces(id,project_id,path) VALUES('w','p','/repo');
INSERT INTO observations(id,workspace_id,task_id,data) VALUES(7,'w','','{"Tree":"kept"}');
INSERT INTO settings VALUES('project','p','workflow','semi',4);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	observation, err := s.Observation(t.Context(), 7)
	if err != nil || observation.Tree != "kept" {
		t.Fatal("migration lost observations", err)
	}
	settings, err := s.Settings(t.Context(), "w")
	if err != nil || settings.Values["workflow"].Revision != 4 {
		t.Fatal("migration lost settings", err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 10 {
		t.Fatalf("schema=%d %v", version, err)
	}
}

func TestCanceledWriteDoesNotLeaveTask(t *testing.T) {
	s, _, actor, obs, _ := taskFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	input := StartTaskInput{Title: "must not persist"}
	if _, err := s.StartTask(ctx, callFor(t, actor, "task_start", input), "", obs.ID, input, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
	id, err := s.CurrentTaskID(t.Context(), "w")
	if err != nil || id != "" {
		t.Fatal("canceled write published task")
	}
}

func TestUnchangedRequirementKeepsDeliveryAndInputRevisions(t *testing.T) {
	s, _, actor, obs, _ := taskFixture(t)
	input := RequirementInput{Content: "目标", Acceptance: "验收"}
	r, err := s.SaveRequirement(t.Context(), callFor(t, actor, "requirement_save", input), "", obs.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	actor.CallID = "pending"
	pending, err := s.TransitionTask(t.Context(), callFor(t, actor, "transition", TaskPending), r.TaskID, TaskPending, "", "")
	if err != nil {
		t.Fatal(err)
	}
	input.ID, input.BaseRevision = r.ID, &r.Revision
	actor.CallID = "same-content-new-call"
	saved, err := s.SaveRequirement(t.Context(), callFor(t, actor, "requirement_save", input), r.TaskID, obs.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.TaskView(t.Context(), "w", r.TaskID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != r.Revision || len(view.History) != 1 || view.Task.RequirementRevision != pending.RequirementRevision || view.Task.State != TaskPending {
		t.Fatal("unchanged requirement invalidated delivery")
	}
}
