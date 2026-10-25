package ledger

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

func checkFixture(t *testing.T) (*Store, Actor, CheckInput, Observation, Message) {
	t.Helper()
	s, _, actor, obs, message := taskFixture(t)
	start := StartTaskInput{Title: "check task"}
	task, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", start), "", obs.ID, start, nil)
	if err != nil {
		t.Fatal(err)
	}
	input, err := s.CheckInput(t.Context(), "w", task.ID, actor, "rules1", "tools1")
	if err != nil {
		t.Fatal(err)
	}
	return s, actor, input, obs, message
}

func TestIssueRelocationPrefersMostSpecificDeclaration(t *testing.T) {
	moves := map[string]string{"a.go:Outer": "b.go:Renamed", "a.go:Outer.run": "c.go:Worker.execute"}
	for _, test := range []struct{ from, to string }{
		{"a.go:Outer", "b.go:Renamed"},
		{"a.go:Outer.run", "c.go:Worker.execute"},
		{"a.go:Outer.run.value", "c.go:Worker.execute.value"},
		{"a.go:OuterExtra.value", "a.go:OuterExtra.value"},
	} {
		if got := relocated(test.from, moves); got != test.to {
			t.Fatalf("%s moved to %s, want %s", test.from, got, test.to)
		}
	}
}

func saveCheckFixture(t *testing.T, s *Store, actor Actor, input CheckInput, issues []Issue) CheckRecord {
	t.Helper()
	record, err := s.SaveCheck(t.Context(), "w", actor, input.Tree, CheckRecord{Input: input, Depth: "static", Status: CheckComplete, Issues: issues, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func requestFinishFixture(t *testing.T, s *Store, actor Actor, input CheckInput) FinishResult {
	t.Helper()
	actor.CallID = "finish-" + actor.CallID
	result, err := s.RequestFinish(t.Context(), callFor(t, actor, "task_finish", FinishInput{}), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCheckInputIgnoresObservationNotesAndSettingProvenance(t *testing.T) {
	s, actor, input, obs, _ := checkFixture(t)
	requestFinishFixture(t, s, actor, input)
	before, err := s.ObservationState(t.Context(), "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	obs.TaskID, obs.At = input.TaskID, obs.At.Add(time.Minute)
	if err := s.SaveObservation(t.Context(), before, &obs, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(t.Context(), "w", Project, "analysis.tests", "off"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(t.Context(), "w", Project, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	actor.CallID = "plain-note"
	note := NoteInput{Kind: "next_step", Text: "普通进度"}
	if _, err := s.AddNote(t.Context(), callFor(t, actor, "note_add", note), input.TaskID, obs.ID, note); err != nil {
		t.Fatal(err)
	}
	got, err := s.CheckInput(t.Context(), "w", input.TaskID, actor, input.Rules, input.Capabilities)
	if err != nil || got.Version() != input.Version() {
		t.Fatalf("non-input activity changed check input: %+v %v", got, err)
	}
	view, err := s.TaskView(t.Context(), "w", input.TaskID, actor)
	if err != nil || view.Task.State != TaskPending {
		t.Fatal("ordinary note expired finish", err)
	}
	if err := s.SetSetting(t.Context(), "w", Project, "analysis.tests", "on"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.CheckInput(t.Context(), "w", input.TaskID, actor, input.Rules, input.Capabilities)
	if got.Version() == input.Version() {
		t.Fatal("analysis settings change reused old input")
	}
}

func TestCheckInputDeduplicatesEffectiveDecision(t *testing.T) {
	s, actor, input, obs, message := checkFixture(t)
	note := NoteInput{Kind: "decision", Text: "保留参数", SourceID: message.ID, Quote: "保留参数", Target: &DecisionTarget{Kind: "issue", ID: "specific", At: "a.go:write.sep", Effect: "retain", Scope: "task"}}
	var first CheckInput
	for i, call := range []string{"one", "two"} {
		actor.CallID = call
		if _, err := s.AddNote(t.Context(), callFor(t, actor, "note_add", note), input.TaskID, obs.ID, note); err != nil {
			t.Fatal(err)
		}
		got, err := s.CheckInput(t.Context(), "w", input.TaskID, actor, input.Rules, input.Capabilities)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = got
		} else if first.Version() != got.Version() {
			t.Fatal("repeating effective decision changed input")
		}
	}
	if first.Version() == input.Version() {
		t.Fatal("new user decision did not change input")
	}
}

func TestObservingAbsentEditTargetDoesNotInvalidateCheck(t *testing.T) {
	s, actor, input, obs, _ := checkFixture(t)
	requestFinishFixture(t, s, actor, input)
	before, err := s.ObservationState(t.Context(), "w", []string{"new.go"})
	if err != nil {
		t.Fatal(err)
	}
	obs.TaskID, obs.Full, obs.Tree = input.TaskID, false, ""
	obs.Files = map[string]gitstate.File{"new.go": {}}
	if err := s.SaveObservation(t.Context(), before, &obs, nil, nil); err != nil {
		t.Fatal(err)
	}
	current, err := s.CheckInput(t.Context(), "w", input.TaskID, actor, input.Rules, input.Capabilities)
	if err != nil || current.Version() != input.Version() {
		t.Fatal("unchanged absent target increased input revision", current, err)
	}
	request, _ := s.FinishRequest(t.Context(), "w", actor)
	if request.State != "pending" {
		t.Fatal("read-only target observation expired request")
	}
}

func TestChecksKeepStaticDynamicSeparateAndExpireConcurrentResult(t *testing.T) {
	s, actor, input, obs, _ := checkFixture(t)
	static := saveCheckFixture(t, s, actor, input, nil)
	dynamic, err := s.SaveCheck(t.Context(), "w", actor, input.Tree, CheckRecord{Input: input, Depth: "tests", Status: CheckPartial, Missing: []CheckGap{{Operation: "tests", Reason: "工具未安装"}}})
	if err != nil || dynamic.Status != CheckPartial {
		t.Fatal(dynamic, err)
	}
	finish := requestFinishFixture(t, s, actor, input)
	if finish.Static == nil || finish.Static.ID != static.ID || finish.Dynamic == nil || finish.Dynamic.ID != dynamic.ID {
		t.Fatal("finish did not return existing independent checks", finish)
	}
	actor.CallID = "new-requirement"
	req := RequirementInput{Content: "新内容", Acceptance: "新验收"}
	if _, err := s.SaveRequirement(t.Context(), callFor(t, actor, "requirement_save", req), input.TaskID, obs.ID, req); err != nil {
		t.Fatal(err)
	}
	stale := saveCheckFixture(t, s, actor, input, []Issue{{Rule: "D1", Level: "block", At: "a.go:unused", FactKey: "unused"}})
	if stale.Status != CheckExpired || stale.Level == "block" {
		t.Fatal("published stale blocker", stale)
	}
	issues, err := s.Issues(t.Context(), input.TaskID)
	if err != nil || len(issues) != 0 {
		t.Fatal("stale result changed issue history", issues, err)
	}
	request, _ := s.FinishRequest(t.Context(), "w", actor)
	if request == nil || request.State != "expired" {
		t.Fatal("input mutation kept delivery request", request)
	}
}

func TestDeliveryRetainsRequestOnceThenCompletesAndHintNeverBlocks(t *testing.T) {
	for _, enforcement := range []string{"block", "hint"} {
		t.Run(enforcement, func(t *testing.T) {
			s, actor, input, _, _ := checkFixture(t)
			if err := s.SetSetting(t.Context(), "w", Project, "enforcement", enforcement); err != nil {
				t.Fatal(err)
			}
			record := saveCheckFixture(t, s, actor, input, []Issue{{Rule: "D1", Level: "block", At: "a.go:unused", FactKey: "unused"}})
			requestFinishFixture(t, s, actor, input)
			first, err := s.Deliver(t.Context(), "w", actor, input.Tree, record)
			if err != nil {
				t.Fatal(err)
			}
			if enforcement == "block" && (first.State != "pending" || len(first.Blocked) != 1) {
				t.Fatal(first)
			}
			if enforcement == "hint" && (first.State != "completed" || len(first.Blocked) != 0) {
				t.Fatal(first)
			}
			second, err := s.Deliver(t.Context(), "w", actor, input.Tree, record)
			if err != nil || second.State != "completed" || len(second.Blocked) != 0 {
				t.Fatal(second, err)
			}
			view, _ := s.TaskView(t.Context(), "w", input.TaskID, actor)
			if view.Task.State != TaskCompleted || view.CurrentTaskID != "" {
				t.Fatal("delivery did not complete current task")
			}
		})
	}
}

func TestCurrentTreeMismatchExpiresDeliveryWithoutBlocking(t *testing.T) {
	s, actor, input, _, _ := checkFixture(t)
	record := saveCheckFixture(t, s, actor, input, []Issue{{Rule: "D1", Level: "block", At: "a.go:unused", FactKey: "unused"}})
	requestFinishFixture(t, s, actor, input)
	result, err := s.Deliver(t.Context(), "w", actor, "changed-on-disk", record)
	if err != nil || result.State != "expired" || len(result.Blocked) != 0 {
		t.Fatal(result, err)
	}
	view, _ := s.TaskView(t.Context(), "w", input.TaskID, actor)
	if view.Task.State != TaskActive {
		t.Fatal("stale delivery did not return to active")
	}
}

func TestIssueIdentitySurvivesMoveAndReintroducedOccurrence(t *testing.T) {
	s, actor, input, _, _ := checkFixture(t)
	issue := Issue{Rule: "D4", Level: "hint", At: "old.go:I", FactKey: "implementations:1"}
	one := saveCheckFixture(t, s, actor, input, []Issue{issue})
	issue.At = "new.go:I"
	two, err := s.SaveCheck(t.Context(), "w", actor, input.Tree, CheckRecord{Input: input, Depth: "static", Status: CheckComplete, Issues: []Issue{issue}, Relocations: map[string]string{"old.go:I": "new.go:I"}})
	if err != nil || one.Issues[0].ID != two.Issues[0].ID {
		t.Fatal("move reset issue", two, err)
	}
	if _, err := s.SaveCheck(t.Context(), "w", actor, input.Tree, CheckRecord{Input: input, Depth: "static", Status: CheckComplete, Evaluated: []string{CheckScope("D4", issue.At)}}); err != nil {
		t.Fatal(err)
	}
	three := saveCheckFixture(t, s, actor, input, []Issue{issue})
	if three.Issues[0].Occurrence != 2 || three.Issues[0].ID == one.Issues[0].ID {
		t.Fatal("reintroduced issue reused previous occurrence", three)
	}
}

func TestBlockLimitsPersistAcrossIssuesAndAgents(t *testing.T) {
	s, actor, input, _, _ := checkFixture(t)
	for i := 0; i < 5; i++ {
		issue := Issue{Rule: "D1", Level: "block", At: string(rune('a'+i)) + ".go:f", FactKey: "unused"}
		record := saveCheckFixture(t, s, actor, input, []Issue{issue})
		var blocked []Issue
		err := s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var err error
			blocked, err = blockIssues(ctx, tx, "w", actor, record.Issues)
			return err
		})
		if err != nil || (len(blocked) == 1) != (i < 3) {
			t.Fatal(i, blocked, err)
		}
	}
	issues, _ := s.Issues(t.Context(), input.TaskID)
	actor.RoundID = "another-round"
	var blocked []Issue
	err := s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		blocked, err = blockIssues(ctx, tx, "w", actor, issues[:1])
		return err
	})
	if err != nil || len(blocked) != 0 {
		t.Fatal("same issue blocked twice in session", blocked, err)
	}
	actor.Agent = "claude"
	err = s.write(t.Context(), func(ctx context.Context, tx *sql.Tx) error {
		var err error
		blocked, err = blockIssues(ctx, tx, "w", actor, issues[:1])
		return err
	})
	if err != nil || len(blocked) != 1 {
		t.Fatal("agent identity crossed", blocked, err)
	}
}

func TestMigrateV6PreservesSummary(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT '');
CREATE TABLE settings(scope TEXT,scope_id TEXT,name TEXT,value TEXT,revision INTEGER,PRIMARY KEY(scope,scope_id,name));` + observationSchema + taskSchema + hookSchema + mcpContextSchema + summarySchema + `
INSERT INTO workspaces(id,project_id,path) VALUES('w','p','/fixture');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.SummaryState(t.Context(), "w", Actor{Agent: "claude", SessionID: "s"}); err != nil {
		t.Fatal(err)
	}
	if issues, err := s.Issues(t.Context(), "missing"); err != nil || len(issues) != 0 {
		t.Fatal(issues, err)
	}
}
