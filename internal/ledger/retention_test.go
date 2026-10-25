package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionKeepsLiveTasksAndReferencedSessionEvidence(t *testing.T) {
	s, dir, _, _, _ := taskFixture(t)
	now := time.Now().UTC().Add(90 * 24 * time.Hour)
	old := now.Add(-80 * 24 * time.Hour)
	encode := func(value any) string {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.Exec(query, args...); err != nil {
			t.Fatal(query, err)
		}
	}
	for i, state := range []TaskState{TaskActive, TaskPending, TaskPaused, TaskCompleted, TaskCanceled, TaskCompleted} {
		id := []string{"active", "pending", "paused", "recent", "boundary", "expired"}[i]
		obsID := int64(100 + i)
		obs := Observation{ID: obsID, WorkspaceID: "w", TaskID: id, Agent: "codex", SessionID: id, At: old, Full: true, Complete: true, Tree: "tree", SnapshotRef: "refs/taskbelay/" + id + "/o1"}
		exec("INSERT INTO observations(id,workspace_id,task_id,data) VALUES(?,?,?,?)", obsID, "w", id, encode(obs))
		task := TaskRecord{ID: id, WorkspaceID: "w", State: state, OriginObservationID: obsID, LastObservationID: obsID, CreatedAt: old, UpdatedAt: old, OriginRef: obs.SnapshotRef}
		if state == TaskCompleted || state == TaskCanceled {
			ended := now.Add(-time.Duration([]int{0, 0, 0, 29, 30, 31}[i]) * 24 * time.Hour)
			task.EndedAt = &ended
		}
		exec("INSERT INTO tasks(id,workspace_id,state,branch,data) VALUES(?,?,?,?,?)", id, "w", state, "", encode(task))
		exec("INSERT INTO task_sessions VALUES(?,?,?,?,?,?,?)", "w", "codex", id, "r", "", old.Format(time.RFC3339Nano), old.Format(time.RFC3339Nano))
		exec("INSERT INTO hook_events(workspace_id,agent,session_id,round_id,kind,tool_call_id,at,event,raw_input) VALUES(?,?,?,?,?,?,?,?,?)", "w", "codex", id, "r", "stop", "", old.Format(time.RFC3339Nano), "{}", "")
		exec("INSERT INTO mcp_contexts VALUES(?,?,?,?,?,?,?)", "w", "codex", id, "call", "r", "tool", old.Format(time.RFC3339Nano))
		exec("INSERT INTO task_calls VALUES(?,?,?,?,?,?,?,?)", "w", "codex", id, "call", "op", "hash", "{}", old.Format(time.RFC3339Nano))
		plan := PlanRecord{TaskID: id, Version: 1, Path: filepath.Join(dir, "plans", id, "v1.md"), Content: "saved plan", ObservationID: obsID}
		exec("INSERT INTO plans VALUES(?,?,?)", id, 1, encode(plan))
		v := VerificationRecord{ID: "v-" + id, WorkspaceID: "w", TaskID: id, Actor: Actor{Agent: "codex", SessionID: id, RoundID: "r"}, StartedAt: old}
		exec("INSERT INTO verifications(id,workspace_id,task_id,agent,session_id,round_id,call_id,command_key,denied,executed,state,data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", v.ID, "w", id, "codex", id, "r", "verify", "key", 0, 1, "passed", encode(v))
	}
	// 活跃任务引用了任务建立前的消息；该 session 的原始 hook 与身份必须一起保留。
	actor := Actor{Agent: "claude", SessionID: "source-session", RoundID: "r"}
	obs := Observation{ID: 40, WorkspaceID: "w", Agent: actor.Agent, SessionID: actor.SessionID, At: old}
	exec("INSERT INTO observations VALUES(?,?,?,?)", 40, "w", "", encode(obs))
	message := Message{ID: "m40", WorkspaceID: "w", Actor: actor, At: old, ObservationID: 40, Text: "保留范围"}
	exec("INSERT INTO messages(id,workspace_id,agent,session_id,round_id,event_id,observation_id,data) VALUES(?,?,?,?,?,?,?,?)", message.ID, "w", actor.Agent, actor.SessionID, "r", "prompt", 40, encode(message))
	source := Source{ID: message.ID, WorkspaceID: "w", ObservationID: 40, At: old, Kind: "user_message", Text: message.Text}
	exec("INSERT INTO sources VALUES(?,?,?,?)", source.ID, "w", "", encode(source))
	exec("INSERT INTO task_sessions VALUES(?,?,?,?,?,?,?)", "w", actor.Agent, actor.SessionID, "r", message.ID, old.Format(time.RFC3339Nano), old.Format(time.RFC3339Nano))
	req := Requirement{ID: "R1", TaskID: "active", Citation: &Citation{SourceID: message.ID, Quote: message.Text}, Author: actor}
	exec("INSERT INTO requirement_versions VALUES(?,?,?,?)", "active", "R1", 1, encode(req))
	if err := s.Prune(t.Context(), "w", now); err != nil {
		t.Fatal(err)
	}
	count := func(query string, want int) {
		t.Helper()
		var got int
		if err := s.db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatal(query, got, want, err)
		}
	}
	count("SELECT COUNT(*) FROM tasks", 5)
	count("SELECT COUNT(*) FROM tasks WHERE id='expired'", 0)
	count("SELECT COUNT(*) FROM observations WHERE id=105", 0)
	count("SELECT COUNT(*) FROM hook_events WHERE session_id='expired'", 0)
	count("SELECT COUNT(*) FROM mcp_contexts WHERE session_id='expired'", 0)
	count("SELECT COUNT(*) FROM task_calls WHERE session_id='expired'", 0)
	count("SELECT COUNT(*) FROM verifications WHERE task_id='expired'", 0)
	count("SELECT COUNT(*) FROM observations WHERE id=40", 1)
	count("SELECT COUNT(*) FROM messages WHERE id='m40'", 1)
	count("SELECT COUNT(*) FROM sources WHERE id='m40'", 1)
	count("SELECT COUNT(*) FROM task_sessions WHERE session_id='source-session'", 1)
	count("SELECT COUNT(*) FROM plans", 5)
	count("SELECT COUNT(*) FROM cleanup_items", 2)
	count("SELECT COUNT(*) FROM pragma_foreign_key_check", 0)
	if err := s.Prune(t.Context(), "w", now); err != nil {
		t.Fatal(err)
	}
	count("SELECT COUNT(*) FROM cleanup_items", 2)
	if err := s.Prune(t.Context(), "w", now.Add(2*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	count("SELECT COUNT(*) FROM tasks", 3)
	count("SELECT COUNT(*) FROM pragma_foreign_key_check", 0)
}

func TestPlanCleanupPreservesCommittedChangedAndOutsideFiles(t *testing.T) {
	s, dir, actor, obs, _ := taskFixture(t)
	input := StartTaskInput{Title: "plan"}
	task, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", input), "", obs.ID, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plans", task.ID, "v1.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	plan := PlanRecord{TaskID: task.ID, Version: 1, Path: path, Content: "owned"}
	if err := s.StagePlanCleanup(t.Context(), "w", plan); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := s.CleanupItems(t.Context(), "w", time.Now().Add(time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if err := s.CleanupPlan(t.Context(), items[0]); err == nil {
		t.Fatal("removed modified plan")
	}
	if data, _ := os.ReadFile(path); string(data) != "changed" {
		t.Fatal(string(data))
	}
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(plan)
	if _, err := s.db.Exec("INSERT INTO plans VALUES(?,?,?)", task.ID, 1, string(data)); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupPlan(t.Context(), items[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("committed plan removed", err)
	}
	if _, err := s.db.Exec("DELETE FROM plans"); err != nil {
		t.Fatal(err)
	}
	if err := s.StagePlanCleanup(t.Context(), "w", plan); err != nil {
		t.Fatal(err)
	}
	items, _ = s.CleanupItems(t.Context(), "w", time.Now().Add(time.Hour))
	if err := s.CleanupPlan(t.Context(), items[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("orphan survived", err)
	}
	if err := s.CleanupPlan(t.Context(), CleanupItem{Kind: "plan", Target: filepath.Join(t.TempDir(), "v1.md")}); err == nil {
		t.Fatal("accepted outside path")
	}
}

func TestRetentionCanceledTransactionKeepsRecords(t *testing.T) {
	s, _, _, _, _ := taskFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.Prune(ctx, "w", time.Now().Add(100*24*time.Hour)); err == nil {
		t.Fatal("canceled cleanup succeeded")
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestPlanPublicationRollbackCanBeCleanedAfterInterruption(t *testing.T) {
	s, dir, actor, obs, _ := taskFixture(t)
	input := StartTaskInput{Title: "plan", Workflow: "semi"}
	task, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", input), "", obs.ID, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := PlanRecord{TaskID: task.ID, Version: 1, Path: filepath.Join(dir, "plans", task.ID, "v1.md"), Content: "complete staged content"}
	if err := os.MkdirAll(filepath.Dir(plan.Path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.StagePlanCleanup(t.Context(), "w", plan); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_plan_call BEFORE INSERT ON task_calls WHEN NEW.operation='plan_save' BEGIN SELECT RAISE(ABORT,'simulated failure after publication'); END`); err != nil {
		t.Fatal(err)
	}
	actor.CallID = "save-plan"
	_, err = s.SavePlan(t.Context(), callFor(t, actor, "plan_save", PlanInput{}), task.ID, obs.ID, 0, plan, func() error { return os.WriteFile(plan.Path, []byte(plan.Content), 0600) })
	if err == nil {
		t.Fatal("missing write failure")
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM plans").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial plan published in ledger", count, err)
	}
	view, err := s.TaskView(t.Context(), "w", task.ID, actor)
	if err != nil || view.Task.PlanRevision != 0 {
		t.Fatal(view.Task, err)
	}
	if _, err := os.Stat(plan.Path); err != nil {
		t.Fatal("failure did not happen after file publication", err)
	}
	items, err := s.CleanupItems(t.Context(), "w", time.Now().Add(time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	if err := s.CleanupPlan(t.Context(), items[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plan.Path); !os.IsNotExist(err) {
		t.Fatal("orphan plan survived", err)
	}
}
