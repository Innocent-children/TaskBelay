package ledger

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

func TestObservationCompareAndSwapAndAtomicEvidence(t *testing.T) {
	s := openTestStore(t)
	if err := s.RegisterWorkspace(t.Context(), "w", "p", "/repo"); err != nil {
		t.Fatal(err)
	}
	first, err := s.ObservationState(t.Context(), "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{WorkspaceID: "w", At: time.Now(), Files: map[string]gitstate.File{"a": {Blob: "before", Mode: "100644"}}}
	if err := s.SaveObservation(t.Context(), first, &observation, nil, nil); err != nil {
		t.Fatal(err)
	}
	stale := Observation{WorkspaceID: "w", Files: map[string]gitstate.File{"a": {Blob: "stale", Mode: "100644"}}}
	if err := s.SaveObservation(t.Context(), first, &stale, nil, nil); !errors.Is(err, ErrObservationConflict) {
		t.Fatalf("stale view=%v", err)
	}
	current, err := s.ObservationState(t.Context(), "w", []string{"a"})
	if err != nil {
		t.Fatal(err)
	}
	bad := Observation{WorkspaceID: "w", Files: map[string]gitstate.File{"a": {Blob: "after", Mode: "100644"}}}
	if err := s.SaveObservation(t.Context(), current, &bad, []FileChange{{Path: "a", Kind: "invented_author"}}, nil); err == nil {
		t.Fatal("invalid source published")
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM observations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("partial observation count=%d %v", count, err)
	}
	state, err := s.ObservationState(t.Context(), "w", nil)
	if err != nil || state.Files["a"].Blob != "before" || state.Revision != 1 {
		t.Fatalf("rollback failed %+v %v", state, err)
	}
}

func TestObservationReadsDoNotNeedWriteLock(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RegisterWorkspace(t.Context(), "w", "p", "/repo"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(t.Context(), dir, FastWait)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE workspaces SET branch='uncommitted' WHERE id='w'"); err != nil {
		t.Fatal(err)
	}
	state, err := other.ObservationState(t.Context(), "w", []string{})
	if err != nil || state.Context.Branch != "" {
		t.Fatalf("read under WAL writer %+v %v", state, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Millisecond)
	defer cancel()
	observation := Observation{WorkspaceID: "w", Files: map[string]gitstate.File{}}
	if err := other.SaveObservation(ctx, state, &observation, nil, nil); err == nil {
		t.Fatal("write ignored lock deadline")
	}
}

func TestMigrateV1PreservesProjectAndTaskSettings(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("CREATE TABLE workspaces(id TEXT PRIMARY KEY,project_id TEXT NOT NULL,path TEXT NOT NULL,current_task_id TEXT NOT NULL DEFAULT ''); CREATE TABLE settings(scope TEXT NOT NULL,scope_id TEXT NOT NULL,name TEXT NOT NULL,value TEXT NOT NULL,revision INTEGER NOT NULL,PRIMARY KEY(scope,scope_id,name)); PRAGMA user_version=1; INSERT INTO workspaces VALUES('/old/.git','/old/.git','/old','t1'); INSERT INTO settings VALUES('project','/old/.git','workflow','semi',3); INSERT INTO settings VALUES('task','t1','enforcement','hint',5);")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	w := gitstate.Workspace{ID: "stable-worktree", ProjectID: "stable-repo", Root: "/old", GitDir: "/old/.git", CommonDir: "/old/.git"}
	if err := s.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	settings := readSettings(t, s, w.ID)
	if settings.Values["workflow"].Value != "semi" || settings.Values["enforcement"].Value != "hint" || settings.Values["enforcement"].Revision != 5 {
		t.Fatalf("migration lost settings: %+v", settings)
	}
	w.Root = "/moved"
	w.GitDir = "/moved/.git"
	w.CommonDir = "/moved/.git"
	if err := s.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	if next := readSettings(t, s, w.ID); next.Revision != settings.Revision {
		t.Fatal("move changed effective setting revision")
	}
	if err := s.RegisterWorkspace(t.Context(), "child", w.ProjectID, "/child"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkWorkspace(t.Context(), w.ID, "t1", "child", "module"); err != nil {
		t.Fatal(err)
	}
	var child string
	if err := s.db.QueryRow("SELECT child_id FROM workspace_links WHERE parent_id=? AND task_id='t1'", w.ID).Scan(&child); err != nil || child != "child" {
		t.Fatalf("child link=%s %v", child, err)
	}
}
