package ledger

import (
	"context"
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "数据 ? #"), NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func mustSet(t *testing.T, store *Store, workspace string, scope Scope, name, value string) {
	t.Helper()
	if err := store.SetSetting(t.Context(), workspace, scope, name, value); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, store *Store, workspace string) EffectiveSettings {
	t.Helper()
	settings, err := store.Settings(t.Context(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestSettingsScopesAndRevisions(t *testing.T) {
	store := openTestStore(t)
	for _, workspace := range []struct{ id, project string }{{"w1", "p1"}, {"w2", "p1"}, {"w3", "p2"}} {
		if err := store.RegisterWorkspace(t.Context(), workspace.id, workspace.project, "/"+workspace.id); err != nil {
			t.Fatal(err)
		}
	}
	settings := readSettings(t, store, "w1")
	for name, want := range map[string]string{"workflow": "auto", "enforcement": "block", "analysis.tests": "off", "test.go.parallel": "1", "test.python.command": "", "test.java.prepare": ""} {
		if got := settings.Values[name]; got.Value != want || got.Source != Default {
			t.Fatalf("default %s = %+v, want %q", name, got, want)
		}
	}
	mustSet(t, store, "", User, "workflow", "semi")
	mustSet(t, store, "w1", Project, "workflow", "auto")
	for _, id := range []string{"w1", "w2"} {
		if got := readSettings(t, store, id).Values["workflow"]; got.Value != "auto" || got.Source != Project || got.ScopeID != "p1" {
			t.Fatalf("shared project %s = %+v", id, got)
		}
	}
	if got := readSettings(t, store, "w3").Values["workflow"]; got.Value != "semi" || got.Source != User {
		t.Fatalf("other project = %+v", got)
	}
	// 这里只提供后续任务阶段保存的上下文，不在配置测试里实现任务生命周期。
	if _, err := store.db.ExecContext(t.Context(), "UPDATE workspaces SET current_task_id='t1' WHERE id='w1'"); err != nil {
		t.Fatal(err)
	}
	mustSet(t, store, "w1", Task, "workflow", "semi")
	before := readSettings(t, store, "w1")
	if got := before.Values["workflow"]; got.Source != Task || got.ScopeID != "t1" || got.Value != "semi" {
		t.Fatalf("task setting = %+v", got)
	}
	if got := readSettings(t, store, "w2").Values["workflow"]; got.Source != Project || got.Value != "auto" {
		t.Fatalf("task leaked to another worktree: %+v", got)
	}
	mustSet(t, store, "w1", Task, "workflow", "semi")
	mustSet(t, store, "", User, "workflow", "auto")
	if err := store.RegisterWorkspace(t.Context(), "w1", "p1", "/moved/w1"); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, store, "w1"); got.Revision != before.Revision {
		t.Fatal("no-op, shadowed setting or path update changed the effective revision")
	}
	mustSet(t, store, "w1", Task, "workflow", "auto")
	after := readSettings(t, store, "w1")
	if after.Revision == before.Revision || after.Values["workflow"].Revision != before.Values["workflow"].Revision+1 {
		t.Fatal("an effective change did not advance the revision")
	}
}

func TestInvalidSettingsDoNotWrite(t *testing.T) {
	store := openTestStore(t)
	if err := store.RegisterWorkspace(t.Context(), "w1", "p1", "/w1"); err != nil {
		t.Fatal(err)
	}
	before := readSettings(t, store, "w1")
	for _, test := range []struct {
		workspace string
		scope     Scope
		name      string
		value     string
	}{
		{"", User, "workflow", "invalid"},
		{"", User, "enforcement", "allow"},
		{"", User, "analysis.tests", "true"},
		{"", User, "test.go.parallel", "0"},
		{"", User, "test.go.parallel", "5"},
		{"", User, "test.go.parallel", "1.5"},
		{"", User, "test.kotlin.command", "test"},
		{"", User, "unknown", "x"},
		{"", Project, "workflow", "semi"},
		{"w1", Task, "workflow", "semi"},
		{"w1", Default, "workflow", "semi"},
	} {
		if err := store.SetSetting(t.Context(), test.workspace, test.scope, test.name, test.value); err == nil {
			t.Errorf("accepted invalid setting: %+v", test)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := store.SetSetting(ctx, "", User, "workflow", "semi"); err == nil {
		t.Fatal("accepted canceled write")
	}
	if got := readSettings(t, store, "w1"); got.Revision != before.Revision {
		t.Fatal("failed write changed settings")
	}
	command := "python -m pytest -k 'two  spaces'\n# project prefix"
	mustSet(t, store, "", User, "test.python.command", command)
	mustSet(t, store, "", User, "test.python.parallel", "4")
	if got := readSettings(t, store, "w1").Values["test.python.command"].Value; got != command {
		t.Fatalf("command text changed: %q", got)
	}
}
