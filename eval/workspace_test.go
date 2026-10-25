package eval_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	all := []string{"-C", dir, "-c", "user.name=TaskBelay test", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(t.TempDir(), "no-hooks")}
	command := exec.CommandContext(t.Context(), "git", append(all, args...)...)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func observer(t *testing.T) hook.Observer {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "项目 space")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "--quiet")
	w, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ledger.Open(t.Context(), t.TempDir(), ledger.NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err := s.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	return hook.Observer{Store: s, Workspace: w}
}

func observe(t *testing.T, o hook.Observer, task string) ledger.Observation {
	t.Helper()
	result, err := o.Observe(t.Context(), event.Event{Kind: event.UserPrompt, Agent: "codex", SessionID: "session"}, task)
	if err != nil || !result.Complete {
		t.Fatalf("observation=%+v err=%v", result, err)
	}
	return result
}

func edit(call, path, before, after string) event.Event {
	return event.Event{Kind: event.EditUse, Agent: "codex", SessionID: "session", ToolCallID: call, Edits: []event.FileEdit{{Path: path, Before: &before, After: &after}}}
}

func before(t *testing.T, o hook.Observer, e event.Event, mode string) event.Decision {
	t.Helper()
	d, err := o.BeforeEdit(t.Context(), e, "", mode)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func after(t *testing.T, o hook.Observer, e event.Event) ledger.Observation {
	t.Helper()
	result, err := o.AfterEdit(t.Context(), e, "")
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func changes(t *testing.T, o hook.Observer) []ledger.FileChange {
	t.Helper()
	result, err := o.Store.Changes(t.Context(), o.Workspace.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWF01CumulativeSnapshotsPreserveRealIndex(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "main.txt", "base\n")
	write(t, dir, ".gitignore", "private\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "--quiet", "-m", "initial")
	write(t, dir, "main.txt", "staged before task\n")
	git(t, dir, "add", "main.txt")
	write(t, dir, "main.txt", "dirty before task\n")
	write(t, dir, "未跟踪 space.txt", "starting file\n")
	write(t, dir, "private", "ignored\n")
	index := git(t, dir, "rev-parse", "--git-path", "index")
	if !filepath.IsAbs(index) {
		index = filepath.Join(dir, index)
	}
	indexBytes := read(t, index)
	info, _ := os.Stat(index)
	origin := observe(t, o, "t1")
	afterInfo, _ := os.Stat(index)
	if !bytes.Equal(indexBytes, read(t, index)) || info.ModTime() != afterInfo.ModTime() {
		t.Fatal("snapshot changed the real index")
	}
	if _, ok := origin.Files["private"]; ok {
		t.Fatal("ignored file captured")
	}
	if _, ok := origin.Files["未跟踪 space.txt"]; !ok {
		t.Fatal("untracked file missing")
	}
	for n := 1; n <= 3; n++ {
		write(t, dir, "main.txt", fmt.Sprintf("commit %d\n", n))
		git(t, dir, "add", "main.txt")
		git(t, dir, "commit", "--quiet", "-m", fmt.Sprintf("part %d", n))
		observe(t, o, "t1")
	}
	git(t, dir, "commit", "--amend", "--no-edit", "--quiet")
	write(t, dir, "new.txt", "staged\n")
	git(t, dir, "add", "new.txt")
	write(t, dir, "new.txt", "working\n")
	if err := os.Rename(filepath.Join(dir, "未跟踪 space.txt"), filepath.Join(dir, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	indexBytes = read(t, index)
	current := observe(t, o, "t1")
	if !bytes.Equal(indexBytes, read(t, index)) {
		t.Fatal("partial staging lost")
	}
	diff, err := o.Workspace.Diff(t.Context(), origin.Tree, current.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff) != 3 {
		t.Fatalf("cumulative diff=%+v", diff)
	}
	renamed := false
	for _, c := range diff {
		if c.Path == "renamed.txt" && c.UniqueRename {
			renamed = true
		}
	}
	if !renamed {
		t.Fatalf("unique rename missing: %+v", diff)
	}
	if git(t, dir, "rev-parse", origin.SnapshotRef+"^{tree}") != origin.Tree {
		t.Fatal("origin moved across commits")
	}
	write(t, dir, "main.txt", "dirty before task\n")
	if err := os.Remove(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "renamed.txt"), filepath.Join(dir, "未跟踪 space.txt")); err != nil {
		t.Fatal(err)
	}
	reverted := observe(t, o, "t1")
	diff, err = o.Workspace.Diff(t.Context(), origin.Tree, reverted.Tree)
	if err != nil || len(diff) != 0 {
		t.Fatalf("net diff after return to origin=%+v err=%v", diff, err)
	}
	if len(changes(t, o)) < 5 {
		t.Fatal("net-zero diff erased history")
	}
	for _, c := range changes(t, o) {
		if c.Kind != "workspace_update" {
			t.Fatalf("commit author inferred: %+v", c)
		}
	}
}

func TestWF02AdjacentEditPreservesUserBehavior(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	source := "package sample\nimport \"fmt\"\nfunc Pick(values []int, index int) (string,error) {\n mode := \"plain\"\n if index >= len(values) { return \"\", fmt.Errorf(\"invalid\") }\n return fmt.Sprintf(\"%s:%d\", mode, values[index]), nil\n}\n"
	write(t, dir, "go.mod", "module example.invalid/fixture\ngo 1.26.0\n")
	write(t, dir, "pick.go", source)
	observe(t, o, "t1")
	user := strings.ReplaceAll(source, "values", "data")
	user = strings.Replace(user, "index >= len(data)", "index < 0 || index >= len(data)", 1)
	user = strings.Replace(user, "fmt.Errorf(\"invalid\")", "fmt.Errorf(\"index %d outside [0,%d)\", index, len(data))", 1)
	write(t, dir, "pick.go", user)
	testSource := "package sample\nimport (\"testing\"; \"strings\")\nfunc TestUserBoundaries(t *testing.T) {\n for _,i := range []int{-1,1,2} { if _,err:=Pick([]int{7},i); err==nil || !strings.Contains(err.Error(),\"outside\") { t.Fatalf(\"invalid index %d: %v\",i,err) } }\n if _,err:=Pick(nil,0); err==nil { t.Fatal(\"empty input\") }; if got,err:=Pick([]int{7},0); got!=\"friendly:7\" || err!=nil { t.Fatalf(\"%s %v\",got,err) }\n}\n"
	write(t, dir, "pick_test.go", testSource)
	e := edit("adjacent", "pick.go", "mode := \"plain\"", "mode := \"friendly\"")
	started := time.Now()
	d := before(t, o, e, "block")
	t.Logf("one-file pre-edit observation: %s", time.Since(started))
	if d.DenyTool != "" || !strings.Contains(d.AdditionalContext, "工作区更新") {
		t.Fatalf("adjacent decision=%+v", d)
	}
	record, err := o.Store.Edit(t.Context(), o.Workspace.ID, e.Agent, e.SessionID, e.ToolCallID)
	if err != nil {
		t.Fatal(err)
	}
	pre, err := o.Store.Observation(t.Context(), record.BeforeObservation)
	if err != nil {
		t.Fatal(err)
	}
	if pre.Full || len(pre.Files) != 1 {
		t.Fatalf("pre-edit scanned beyond targets: %+v", pre)
	}
	write(t, dir, "pick.go", strings.Replace(user, "mode := \"plain\"", "mode := \"friendly\"", 1))
	after(t, o, e)
	observe(t, o, "t1")
	agent, workspace := 0, 0
	for _, c := range changes(t, o) {
		if c.Kind == "agent_edit" {
			agent++
		} else {
			workspace++
		}
	}
	if agent != 1 || workspace < 2 {
		t.Fatalf("evidence agent=%d workspace=%d: %+v", agent, workspace, changes(t, o))
	}
	if string(read(t, filepath.Join(dir, "pick_test.go"))) != testSource {
		t.Fatal("user assertions changed")
	}
	command := exec.CommandContext(t.Context(), "go", "test", "./...")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("user behavior regressed: %v\n%s", err, out)
	}
}

func TestWF03StaleEditsRequireFreshContextEveryTime(t *testing.T) {
	for _, scenario := range []struct{ name, old, current string }{
		{"error", "return nil", "return err"},
		{"name", "values", "data"},
		{"boundary", "n < 0", "n <= 0"},
		{"assertion", "got != 1", "got != 2"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			o := observer(t)
			dir := o.Workspace.Root
			write(t, dir, "file.txt", scenario.old+"\n")
			observe(t, o, "t1")
			write(t, dir, "file.txt", scenario.current+"\n")
			for i := 0; i < 2; i++ {
				d := before(t, o, edit(fmt.Sprintf("stale%d", i), "file.txt", scenario.old, "overwritten"), "block")
				if d.DenyTool == "" || !strings.Contains(d.AdditionalContext, scenario.current) {
					t.Fatalf("stale decision=%+v", d)
				}
			}
			d := before(t, o, edit("hint", "file.txt", scenario.old, "overwritten"), "hint")
			if d.DenyTool != "" || !strings.Contains(d.AdditionalContext, scenario.current) {
				t.Fatalf("hint=%+v", d)
			}
			if before(t, o, edit("stale0", "file.txt", scenario.old, "overwritten"), "hint").DenyTool != "" {
				t.Fatal("retry ignored current hint setting")
			}
			observe(t, o, "t1")
			old := scenario.old + "\n"
			whole := event.Event{Agent: "codex", SessionID: "session", ToolCallID: "whole", Edits: []event.FileEdit{{Path: "file.txt", After: &old, WholeFile: true}}}
			if before(t, o, whole, "block").DenyTool == "" {
				t.Fatal("unverifiable whole-file overwrite allowed")
			}
			if got := string(read(t, filepath.Join(dir, "file.txt"))); got != scenario.current+"\n" {
				t.Fatalf("user content overwritten: %q", got)
			}
			fresh := edit("fresh", "file.txt", scenario.current, scenario.current+" // updated")
			if before(t, o, fresh, "block").DenyTool != "" {
				t.Fatal("fresh current-context edit rejected")
			}
			write(t, dir, "file.txt", scenario.current+" // updated\n")
			after(t, o, fresh)
		})
	}
}

func TestMixedEditsIdempotenceAndShellRemainDistinct(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	initial := "func example() {\n first := 1\n keep := 2\n user := 3\n last := 4\n}\n"
	write(t, dir, "sample.go", initial)
	origin := observe(t, o, "")
	if !strings.Contains(origin.SnapshotRef, "/session-") {
		t.Fatalf("unbound origin not retained: %s", origin.SnapshotRef)
	}
	e := edit("mixed", "sample.go", "first := 1", "first := 10")
	d := before(t, o, e, "block")
	if d.DenyTool != "" {
		t.Fatal(d)
	}
	pre, err := o.Store.ObservationState(t.Context(), o.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	before(t, o, e, "block")
	duplicate, err := o.Store.ObservationState(t.Context(), o.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.LatestID != pre.LatestID {
		t.Fatal("duplicate pre-event appended evidence")
	}
	actual := strings.Replace(initial, "first := 1", "first := 10", 1)
	actual = strings.Replace(actual, "user := 3", "user := 30", 1)
	write(t, dir, "sample.go", actual)
	result := after(t, o, e)
	again := after(t, o, e)
	if again.ID != result.ID {
		t.Fatal("duplicate post-event appended evidence")
	}
	recorded := changes(t, o)
	if len(recorded) != 2 || recorded[0].Kind != "agent_edit" || recorded[1].Kind != "workspace_update" {
		t.Fatalf("mixed evidence=%+v", recorded)
	}
	if recorded[0].Fragments[0].AfterContentID != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(" first := 10\n"))) || recorded[1].Fragments[0].AfterContentID != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(" user := 30\n"))) {
		t.Fatalf("wrong attribution: %+v", recorded)
	}
	state, err := o.Store.ObservationState(t.Context(), o.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	observe(t, o, "t1")
	unchanged, err := o.Store.ObservationState(t.Context(), o.Workspace.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != unchanged.Revision || len(changes(t, o)) != 2 {
		t.Fatal("full observation relabeled verified edits or invalidated unchanged input")
	}
	write(t, dir, "shell.txt", "unverifiable command output\n")
	observe(t, o, "t1")
	recorded = changes(t, o)
	if recorded[len(recorded)-1].Kind != "workspace_update" {
		t.Fatal("shell output attributed to agent")
	}
}

func TestCaptureCancellationReportsIncompleteAndKeepsIndex(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "large.bin", strings.Repeat("x", 4<<20))
	git(t, dir, "add", ".")
	index := filepath.Join(o.Workspace.GitDir, "index")
	original := read(t, index)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Millisecond))
	defer cancel()
	snapshot, err := o.Workspace.Capture(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || snapshot.Complete() || len(snapshot.Incomplete) == 0 {
		t.Fatalf("deadline result=%+v err=%v", snapshot, err)
	}
	if !bytes.Equal(original, read(t, index)) {
		t.Fatal("timeout changed index")
	}
}

func TestWF17MoveWorktreeSymlinkAndActualCaseRules(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "Original.txt", "content\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "--quiet", "-m", "initial")
	origin := observe(t, o, "t1")
	if err := o.Store.SetSetting(t.Context(), o.Workspace.ID, ledger.Project, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	beforeSettings, err := o.Store.Settings(t.Context(), o.Workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(filepath.Dir(dir), "已移动 space")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	w, err := gitstate.Discover(t.Context(), moved)
	if err != nil {
		t.Fatal(err)
	}
	if w.ID != o.Workspace.ID || w.ProjectID != o.Workspace.ProjectID || w.Root == dir {
		t.Fatalf("identity changed after move: %+v %+v", o.Workspace, w)
	}
	if err := o.Store.RegisterGitWorkspace(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	o.Workspace = w
	settings, err := o.Store.Settings(t.Context(), w.ID)
	if err != nil || settings.Revision != beforeSettings.Revision {
		t.Fatalf("settings lost on move: %+v %v", settings, err)
	}
	if git(t, moved, "rev-parse", origin.SnapshotRef+"^{tree}") != origin.Tree {
		t.Fatal("origin lost on move")
	}
	worktree := filepath.Join(filepath.Dir(moved), "worktree 中文")
	git(t, moved, "worktree", "add", "--quiet", "-b", "other", worktree)
	child, err := gitstate.Discover(t.Context(), worktree)
	if err != nil {
		t.Fatal(err)
	}
	if child.ID == w.ID || child.ProjectID != w.ProjectID {
		t.Fatalf("worktree identity: %+v %+v", w, child)
	}
	if err := o.Store.RegisterGitWorkspace(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	childSettings, err := o.Store.Settings(t.Context(), child.ID)
	if err != nil || childSettings.Values["enforcement"].Value != "hint" {
		t.Fatalf("project settings not shared: %+v %v", childSettings, err)
	}
	parentIndex := read(t, filepath.Join(w.GitDir, "index"))
	childIndex := read(t, filepath.Join(child.GitDir, "index"))
	write(t, worktree, "Original.txt", "worktree dirty\n")
	childSnapshot, err := child.Capture(t.Context())
	if err != nil || !childSnapshot.Complete() {
		t.Fatalf("worktree capture %+v %v", childSnapshot, err)
	}
	if !bytes.Equal(parentIndex, read(t, filepath.Join(w.GitDir, "index"))) || !bytes.Equal(childIndex, read(t, filepath.Join(child.GitDir, "index"))) {
		t.Fatal("worktree capture altered an index")
	}
	movedWorktree := filepath.Join(filepath.Dir(worktree), "moved worktree")
	git(t, moved, "worktree", "move", worktree, movedWorktree)
	childAfterMove, err := gitstate.Discover(t.Context(), movedWorktree)
	if err != nil || childAfterMove.ID != child.ID || childAfterMove.ProjectID != child.ProjectID {
		t.Fatalf("moved worktree identity %+v %v", childAfterMove, err)
	}
	rule := platform.DirectoryCaseRule(w.Root)
	keyA, err := w.TargetKey("NewDirectory/NewFile.txt")
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := w.TargetKey("newdirectory/newfile.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rule == platform.CaseInsensitive {
		if keyA != keyB {
			t.Fatal("new case aliases not compared using parent rule")
		}
		rel, err := w.TargetPath("original.TXT")
		if err != nil || rel != "Original.txt" {
			t.Fatalf("case-insensitive target %q %v", rel, err)
		}
	} else if rule == platform.CaseSensitive {
		if keyA == keyB {
			t.Fatal("new case-sensitive paths collapsed")
		}
		write(t, moved, "original.txt", "distinct\n")
		a, err := w.TargetPath("Original.txt")
		if err != nil {
			t.Fatal(err)
		}
		b, err := w.TargetPath("original.txt")
		if err != nil || a == b {
			t.Fatalf("case-sensitive names collapsed: %s %s %v", a, b, err)
		}
	} else {
		t.Log("filesystem does not expose a known case rule; paths remain distinct")
	}
	if err := os.Symlink(moved, filepath.Join(filepath.Dir(moved), "repo alias")); err == nil {
		alias, err := gitstate.Discover(t.Context(), filepath.Join(filepath.Dir(moved), "repo alias"))
		if err != nil || alias.ID != w.ID {
			t.Fatalf("alias identity %+v %v", alias, err)
		}
	}
}

func TestNestedRepositoriesAndSubmodulesKeepBoundaries(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "root.txt", "root")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "--quiet", "-m", "root")
	external := t.TempDir()
	git(t, external, "init", "--quiet")
	write(t, external, "child.txt", "child")
	git(t, external, "add", ".")
	git(t, external, "commit", "--quiet", "-m", "child")
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", external, "module space")
	nested := filepath.Join(dir, "nested 中文")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, nested, "init", "--quiet")
	write(t, nested, "inside.txt", "inside")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "--quiet", "-m", "nested")
	observation := observe(t, o, "t1")
	if len(observation.Boundaries) != 2 {
		t.Fatalf("boundaries=%+v", observation.Boundaries)
	}
	for _, boundary := range observation.Boundaries {
		if boundary.Version == "" || boundary.WorkspaceID == "" || boundary.WorkspaceID == o.Workspace.ID {
			t.Fatalf("invalid boundary %+v", boundary)
		}
		child, err := o.LinkBoundary(t.Context(), "t1", boundary.Path)
		if err != nil || child.ID != boundary.WorkspaceID {
			t.Fatalf("child link %+v %v", child, err)
		}
		for path := range observation.Files {
			if strings.HasPrefix(path, boundary.Path+"/") {
				t.Fatalf("nested file expanded into parent: %s", path)
			}
		}
	}
	if _, err := o.Workspace.TargetPath("module space/child.txt"); err == nil {
		t.Fatal("submodule edit used parent observation history")
	}
}

func TestRawCRLFAndPatchContextAgreeWithFullObservation(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	git(t, dir, "config", "core.autocrlf", "true")
	write(t, dir, "nested/file.txt", "before\r\nkeep\r\n")
	observe(t, o, "t1")
	e := event.Event{Agent: "codex", SessionID: "session", ToolCallID: "patch", WorkingDir: filepath.Join(dir, "nested"), Edits: []event.FileEdit{{Path: "file.txt", Patch: "@@ -1,2 +1,2 @@\n-before\r\n+after\r\n keep\r\n"}}}
	d := before(t, o, e, "block")
	if d.DenyTool != "" || d.AdditionalContext != "" {
		t.Fatalf("line conversion fabricated conflict: %+v", d)
	}
	write(t, dir, "nested/file.txt", "after\r\nkeep\r\n")
	after(t, o, e)
	observe(t, o, "t1")
	all := changes(t, o)
	if len(all) != 1 || all[0].Kind != "agent_edit" {
		t.Fatalf("raw/canonical mismatch generated updates: %+v", all)
	}
}

func TestInterveningObservationAndOwnFailureDoNotBlameOrBlock(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "file.txt", "before\n")
	observe(t, o, "t1")
	e := edit("intervening", "file.txt", "before", "after")
	before(t, o, e, "block")
	write(t, dir, "file.txt", "after\n")
	observe(t, o, "t1")
	after(t, o, e)
	for _, c := range changes(t, o) {
		if c.Kind == "agent_edit" {
			t.Fatalf("already unexplained changes relabeled: %+v", c)
		}
	}
	if err := o.Store.Close(); err != nil {
		t.Fatal(err)
	}
	d, err := o.BeforeEdit(t.Context(), edit("closed", "file.txt", "after", "next"), "t1", "block")
	if err == nil || d.DenyTool != "" {
		t.Fatalf("own failure blocked normal tool: %+v %v", d, err)
	}
}

func TestAdjacentMixedChangesAndUnverifiablePatch(t *testing.T) {
	o := observer(t)
	dir := o.Workspace.Root
	write(t, dir, "file.txt", "agent: before\nuser: before\n")
	observe(t, o, "t1")
	e := edit("adjacent-mixed", "file.txt", "agent: before", "agent: after")
	before(t, o, e, "block")
	write(t, dir, "file.txt", "agent: after\nuser: after\n")
	after(t, o, e)
	all := changes(t, o)
	if len(all) != 2 || all[0].Kind != "agent_edit" || all[0].Fragments[0].BeforeStart != 1 || all[1].Kind != "workspace_update" || all[1].Fragments[0].BeforeStart != 2 {
		t.Fatalf("adjacent changes conflated: %+v", all)
	}
	bad := event.Event{Agent: "codex", SessionID: "session", ToolCallID: "unknown-patch", Edits: []event.FileEdit{{Path: "file.txt", Patch: "not a normalized patch"}}}
	d := before(t, o, bad, "block")
	if d.DenyTool != "" || !strings.Contains(d.AdditionalContext, "无法核对") {
		t.Fatalf("unknown syntax blocked as if proven stale: %+v", d)
	}
	write(t, dir, "file.txt", "unknown change\n")
	after(t, o, bad)
	all = changes(t, o)
	if all[len(all)-1].Kind != "workspace_update" {
		t.Fatal("unknown patch attributed to agent")
	}
}
