package ledger

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func TestHookActivityDoesNotRegressMessageRoundAndRetainsUnknownResult(t *testing.T) {
	s, _, actor, _, message := taskFixture(t)
	e := event.Event{Kind: event.ToolEnd, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: "old-round", ToolCallID: "late", Result: &event.ToolResult{State: event.Unknown, Output: "exited with code 0"}}
	id, err := s.BeginHook(t.Context(), "w", e, []byte(`{"original":"payload"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateSession(t.Context(), "w", actor); err != nil {
		t.Fatal("late hook overwrote round", err)
	}
	if err := s.FinishHook(t.Context(), id, event.Decision{}, nil); err != nil {
		t.Fatal(err)
	}
	var encoded, raw, state, source string
	if err := s.db.QueryRow("SELECT event,raw_input,state FROM hook_events WHERE id=?", id).Scan(&encoded, &raw, &state); err != nil {
		t.Fatal(err)
	}
	var saved event.Event
	if err := json.Unmarshal([]byte(encoded), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Result.ExitCode != nil || saved.Result.State != event.Unknown || raw != `{"original":"payload"}` || state != "recorded" {
		t.Fatal(saved, raw, state)
	}
	if err := s.db.QueryRow("SELECT last_message_id FROM task_sessions WHERE workspace_id='w'").Scan(&source); err != nil || source != message.ID {
		t.Fatal(source, err)
	}
}

func TestMigrateV3PreservesSessionAndTaskRecords(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT '');
CREATE TABLE settings(scope TEXT,scope_id TEXT,name TEXT,value TEXT,revision INTEGER,PRIMARY KEY(scope,scope_id,name));` + observationSchema + taskSchema + `
INSERT INTO workspaces(id,project_id,path,current_task_id) VALUES('w','p','/fixture','t1');
INSERT INTO task_sessions VALUES('w','codex','s','r','m1','start','active');
INSERT INTO tasks VALUES(1,'t1','w','active','branch','{}');`)
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
	if err := s.ValidateSession(t.Context(), "w", Actor{Agent: "codex", SessionID: "s", RoundID: "r"}); err != nil {
		t.Fatal(err)
	}
	id, err := s.CurrentTaskID(t.Context(), "w")
	if err != nil || id != "t1" {
		t.Fatal(id, err)
	}
	if _, err := s.BeginHook(t.Context(), "w", event.Event{Agent: "codex", SessionID: "s", Kind: event.SessionStart}, nil); err != nil {
		t.Fatal(err)
	}
}
