package application

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func testPlan() domain.Plan {
	return domain.Plan{Approach: "Edit the existing text file directly; no new abstraction is needed.", VerificationBasis: "Read the requested text and review the actual diff.", Checks: []domain.Check{{ID: "human", Kind: "human", GoalIDs: []string{"goal"}, Rationale: "User checks requested text", Conditions: "Read current file"}}}
}
func ptr[T any](v T) *T { return &v }
func TestUserRequestStartsWorkAndMethodChangesDoNotAskAgain(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	before, e := s.PreWrite(ctx, root, []string{filepath.Join(root, "file.txt")}, true)
	if e != nil || before["decision"] != "allow" {
		t.Fatal("existing request did not authorize work", before, e)
	}
	v, e = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "start", v.Task.Revision}, Progress: []domain.Progress{{GoalID: "goal", Status: "doing", Summary: "Editing the requested text"}}})
	if e != nil {
		t.Fatal(e)
	}
	plan := v.Task.Plan
	plan.Approach = "Update the existing formatter after locating the cause"
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "revise-method", v.Task.Revision}, Plan: &plan, Reason: "Investigation found the responsible formatter"})
	if e != nil || v.Decision.Advice == "ask_user" || len(v.Task.Progress) != 1 {
		t.Fatal("method revision lost authority or progress", e)
	}
	before, e = s.PreWrite(ctx, root, []string{filepath.Join(root, "file.txt")}, true)
	if e != nil || before["decision"] != "allow" {
		t.Fatal(before, e)
	}
	a := v.Task.Authorization
	a.Request = "Also change the public API"
	_, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "expand", v.Task.Revision}, Authorization: &a, Reason: "New request needs an actual user decision"})
	if code(e) != "AUTHORIZATION_REQUIRED" {
		t.Fatal(e)
	}
	a.UserRef = "actual-user-followup"
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "user-expands", v.Task.Revision}, Authorization: &a, Reason: "The user requested the additional API work"})
	if e != nil || v.Task.Authorization.UserRef != a.UserRef {
		t.Fatal(e)
	}
}
func TestExplicitAllowedScopeDiffersFromPredictedPaths(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	a := v.Task.Authorization
	a.UserRef = "user-restricts-paths"
	a.AllowedPaths = []string{"file.txt"}
	v.Task.Plan.ExpectedPaths = []string{"different-prediction.txt"}
	v, e := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "scope", v.Task.Revision}, Authorization: ptr(a), Plan: ptr(v.Task.Plan), Reason: "User explicitly restricts edits to file.txt"})
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct{ path, want string }{{"file.txt", "allow"}, {"extra.txt", "deny"}} {
		r, e := s.PreWrite(ctx, root, []string{filepath.Join(root, c.path)}, true)
		if e != nil || r["decision"] != c.want {
			t.Fatal(c, r, e)
		}
	}
	os.WriteFile(filepath.Join(root, "extra.txt"), []byte("external edit"), 0600)
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range v.Task.Facts {
		found = found || f.Kind == "unauthorized_path" && f.Blocking
	}
	if !found {
		t.Fatal("external write outside allowlist was not detected")
	}
}

func TestPreWriteObservesBranchAndHistoryBeforeAllowingChanges(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	git := func(args ...string) {
		t.Helper()
		if output, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git: %v %s", e, output)
		}
	}
	prewrite := func(want string) {
		t.Helper()
		result, e := s.PreWrite(ctx, root, []string{filepath.Join(root, "file.txt")}, true)
		if e != nil || result["decision"] != want {
			t.Fatal("write decision", result, e)
		}
	}
	git("switch", "-c", "other")
	prewrite("deny")
	if _, e := s.Inspect(ctx, v.Task.TaskID, true); e != nil {
		t.Fatal(e)
	}
	prewrite("deny")
	git("switch", "main")
	prewrite("allow")
	git("commit", "--allow-empty", "-m", "linear progress")
	if _, e := s.Inspect(ctx, v.Task.TaskID, true); e != nil {
		t.Fatal(e)
	}
	prewrite("allow")
	git("reset", "--hard", "HEAD~1")
	prewrite("deny")
}
func TestTerminalTaskPagesArchiveAndRestore(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	v, e := s.Cancel(ctx, CancelRequest{Identity: Identity{v.Task.TaskID, "cancel", v.Task.Revision}, Reason: "Finish fixture"})
	if e != nil {
		t.Fatal(e)
	}
	// Populate terminal fixtures to test the public paging boundary without 201 Git observations.
	for i := 0; i < 205; i++ {
		task := v.Task
		task.TaskID = fmt.Sprintf("task-page-%03d", i)
		task.Revision = 1
		task.Repositories = nil
		// Create requires the current valid task scope; terminal claims are removed by Save.
		task.Repositories = v.Task.Repositories
		if e = s.Store.Create(ctx, task, store.Operation{ID: fmt.Sprintf("open-%d", i), Kind: "open", Digest: domain.Hash(i)}); e != nil {
			t.Fatal(e)
		}
		task.Revision++
		if e = s.Store.Save(ctx, task, 1, store.Operation{}, store.Charge{}, "", nil, "", ""); e != nil {
			t.Fatal(e)
		}
	}
	page, e := s.Store.List(ctx, store.ListQuery{Page: 11})
	if e != nil || len(page.Items) != 6 || page.HasNext {
		t.Fatal("older tasks unreachable", len(page.Items), page.HasNext, e)
	}
	v, e = s.Archive(ctx, ArchiveRequest{Identity: Identity{v.Task.TaskID, "archive", v.Task.Revision}, Archived: true})
	if e != nil {
		t.Fatal(e)
	}
	page, e = s.Store.List(ctx, store.ListQuery{Page: 11})
	if e != nil || len(page.Items) != 5 {
		t.Fatal("archive did not hide task", e)
	}
	page, e = s.Store.List(ctx, store.ListQuery{Page: 11, IncludeArchived: true})
	if e != nil || len(page.Items) != 6 {
		t.Fatal("archived task cannot be found", e)
	}
	v, e = s.Archive(ctx, ArchiveRequest{Identity: Identity{v.Task.TaskID, "restore", v.Task.Revision}, Archived: false})
	if e != nil || v.Task.Status != "cancelled" {
		t.Fatal("archive changed outcome", e)
	}
}

func TestDependencyChangesWithoutAnExplicitPolicyRequireReviewInsteadOfApproval(t *testing.T) {
	s, v, root := fixture(t)
	if e := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"example-library":"1.0.0"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	v, e := s.Inspect(context.Background(), v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, fact := range v.Task.Facts {
		if fact.Kind == "dependency_change" {
			found = true
			if fact.Blocking {
				t.Fatal("an unstated dependency restriction was invented", fact)
			}
		}
		if fact.Kind == "unexpected_path" {
			t.Fatal("an empty forecast flagged every edit", fact)
		}
	}
	if !found || v.Decision.Advice == "ask_user" || v.Decision.Ready {
		t.Fatal("dependency change must still be reviewed", v.Decision)
	}
}

func planWithChecks(p domain.Plan, checks []domain.Check) domain.Plan { p.Checks = checks; return p }

func TestTaskSearchAndHostFilterBeforePagination(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	for i, host := range []domain.Host{"codex", "cli", "codex"} {
		var err error
		v, err = s.Cancel(ctx, CancelRequest{Identity: Identity{v.Task.TaskID, fmt.Sprintf("cancel-filter-%d", i), v.Task.Revision}, Reason: "Prepare isolated filter fixture"})
		if err != nil {
			t.Fatal(err)
		}
		authorization := v.Task.Authorization
		authorization.Request = fmt.Sprintf("Needle_%% fixture %d", i)
		v, err = s.Open(ctx, OpenRequest{OperationID: fmt.Sprintf("filter-open-%d", i), Host: host, Repositories: []RepositoryInput{{Key: "primary", Path: root}}, Authorization: authorization, Plan: v.Task.Plan})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Store.List(ctx, store.ListQuery{Search: "needle_%", Host: "codex", PageSize: 1})
	if err != nil || len(page.Items) != 1 || !page.HasNext || page.Items[0].Authorization.Request != "Needle_% fixture 2" {
		t.Fatal(page, err)
	}
	page, err = s.Store.List(ctx, store.ListQuery{Search: "needle_%", Host: "codex", PageSize: 1, Page: 2})
	if err != nil || len(page.Items) != 1 || page.HasNext || page.Items[0].Authorization.Request != "Needle_% fixture 0" {
		t.Fatal(page, err)
	}
	page, err = s.Store.List(ctx, store.ListQuery{Search: "needleZZ", Host: "codex"})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("search treated literal characters as SQL wildcards", page, err)
	}
}
