package ledger

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSummaryStatePersistsWithoutChangingTaskAndRejectsStaleInput(t *testing.T) {
	s, dir, actor, observation, _ := taskFixture(t)
	input := StartTaskInput{Title: "task"}
	task, err := s.StartTask(t.Context(), callFor(t, actor, "task_start", input), "", observation.ID, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.TaskView(t.Context(), "w", "", actor)
	if err != nil {
		t.Fatal(err)
	}
	state := SummaryState{InputVersion: "v1", SettingsRevision: view.Settings.Revision, GuidanceVersion: "g1", ObservationID: observation.ID}
	if err := s.SaveSummaryState(t.Context(), "w", actor, view, state); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := reader.SummaryState(t.Context(), "w", actor)
	if err != nil || got != state {
		t.Fatal(got, err)
	}
	after, err := s.TaskView(t.Context(), "w", "", actor)
	if err != nil || after.Task.Revision != task.Revision || after.Task.RecordRevision != task.RecordRevision {
		t.Fatal("summary changed task revisions", err)
	}
	other := actor
	other.Agent = "claude"
	if got, err := s.SummaryState(t.Context(), "w", other); err != nil || got.InputVersion != "" {
		t.Fatal("Agent identity crossed", got, err)
	}
	other = actor
	other.SessionID = "another"
	if got, err := s.SummaryState(t.Context(), "w", other); err != nil || got.InputVersion != "" {
		t.Fatal("session identity crossed", got, err)
	}
	if got, err := s.SummaryState(t.Context(), "another-workspace", actor); err != nil || got.InputVersion != "" {
		t.Fatal("workspace identity crossed", got, err)
	}
	if err := s.SetSetting(t.Context(), "w", Project, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	state.InputVersion = "stale"
	if err := s.SaveSummaryState(t.Context(), "w", actor, view, state); err == nil {
		t.Fatal("stale settings acknowledged")
	}
	got, err = s.SummaryState(t.Context(), "w", actor)
	if err != nil || got.InputVersion != "v1" {
		t.Fatal(got, err)
	}
}

func TestActiveSessionsUseAgentWorkspaceAndTenMinuteWindow(t *testing.T) {
	s, _, actor, _, _ := taskFixture(t)
	if err := s.RegisterWorkspace(t.Context(), "other", "p", "/other"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		workspace string
		actor     Actor
		age       time.Duration
	}{
		{"w", actor, 9 * time.Minute},
		{"w", Actor{Agent: "claude", SessionID: actor.SessionID}, 10 * time.Minute},
		{"w", Actor{Agent: "codex", SessionID: "idle"}, 11 * time.Minute},
		{"other", Actor{Agent: "claude", SessionID: "external"}, time.Minute},
	} {
		if err := s.TouchSession(t.Context(), item.workspace, item.actor); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("UPDATE task_sessions SET activity_at=? WHERE workspace_id=? AND agent=? AND session_id=?", now.Add(-item.age).Format(time.RFC3339Nano), item.workspace, item.actor.Agent, item.actor.SessionID); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.ActiveSessions(t.Context(), "w", now)
	if err != nil || len(active) != 2 || active[0].Agent != "claude" || active[1].Agent != "codex" {
		t.Fatal(active, err)
	}
	active, err = s.ActiveSessions(t.Context(), "w", now.Add(time.Nanosecond))
	if err != nil || len(active) != 1 || active[0].Agent != "codex" {
		t.Fatal(active, err)
	}
	if err := s.TouchSession(t.Context(), "w", Actor{Agent: actor.Agent, SessionID: actor.SessionID, RoundID: "late-other-round"}); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateSession(t.Context(), "w", actor); err != nil {
		t.Fatal("activity replaced message round", err)
	}
}

func TestMigrateV5PreservesNativeMCPBindings(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT '');
CREATE TABLE settings(scope TEXT,scope_id TEXT,name TEXT,value TEXT,revision INTEGER,PRIMARY KEY(scope,scope_id,name));` + observationSchema + taskSchema + hookSchema + mcpContextSchema + `
INSERT INTO workspaces(id,project_id,path) VALUES('w','p','/fixture');
INSERT INTO task_sessions VALUES('w','claude','s','r','m1','at','at');
INSERT INTO mcp_contexts VALUES('w','claude','s','call','r','task_start','at');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	actor, err := s.MCPContext(t.Context(), "w", Actor{Agent: "claude", SessionID: "s", CallID: "call"}, "task_start")
	if err != nil || actor.RoundID != "r" {
		t.Fatal(actor, err)
	}
	state, err := s.SummaryState(t.Context(), "w", actor)
	if err != nil || state.InputVersion != "" {
		t.Fatal(state, err)
	}
}
