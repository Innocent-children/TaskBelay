package ledger

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMCPContextKeepsNativeIdentityAndRejectsConflictingReuse(t *testing.T) {
	s, _, actor, _, _ := taskFixture(t)
	actor.CallID = "call"
	if err := s.BindMCPContext(t.Context(), "w", actor, "task_start"); err != nil {
		t.Fatal(err)
	}
	if err := s.BindMCPContext(t.Context(), "w", actor, "task_start"); err != nil {
		t.Fatal(err)
	}
	changed := actor
	changed.RoundID = "other"
	if err := s.BindMCPContext(t.Context(), "w", changed, "task_start"); err == nil {
		t.Fatal("rebound round")
	}
	if err := s.BindMCPContext(t.Context(), "w", actor, "note_add"); err == nil {
		t.Fatal("rebound tool")
	}
	lookup := actor
	lookup.RoundID = ""
	got, err := s.MCPContext(t.Context(), "w", lookup, "task_start")
	if err != nil || got != actor {
		t.Fatal(got, err)
	}
	if _, err := s.MCPContext(t.Context(), "other-workspace", lookup, "task_start"); err == nil {
		t.Fatal("crossed workspace identity")
	}
	for _, field := range []string{"agent", "session", "call"} {
		other := lookup
		switch field {
		case "agent":
			other.Agent = "claude"
		case "session":
			other.SessionID = "other"
		case "call":
			other.CallID = "other"
		}
		if _, err := s.MCPContext(t.Context(), "w", other, "task_start"); err == nil {
			t.Fatal("crossed identity", field)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM mcp_contexts").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestMigrateV4PreservesHookRecords(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT '');
CREATE TABLE settings(scope TEXT,scope_id TEXT,name TEXT,value TEXT,revision INTEGER,PRIMARY KEY(scope,scope_id,name));` + observationSchema + taskSchema + hookSchema + `
INSERT INTO workspaces(id,project_id,path) VALUES('w','p','/fixture');
INSERT INTO hook_events(workspace_id,agent,session_id,round_id,kind,tool_call_id,at,event,raw_input) VALUES('w','claude','s','r','stop','','at','{}','original');`)
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
	var raw string
	if err := s.db.QueryRow("SELECT raw_input FROM hook_events").Scan(&raw); err != nil || raw != "original" {
		t.Fatal(raw, err)
	}
	if err := s.BindMCPContext(t.Context(), "w", Actor{Agent: "claude", SessionID: "s", RoundID: "r", CallID: "c"}, "task_start"); err != nil {
		t.Fatal(err)
	}
}
