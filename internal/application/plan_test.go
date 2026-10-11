package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func testPlan() domain.Plan {
	return domain.Plan{
		Understanding: "Change only the requested text.", RootCause: "The current text does not express the requested behavior.",
		Approach: "Edit the existing file directly.", SimplerAlternative: "A separate abstraction or configuration has no current consumer.",
		VerificationBasis: "A user observation checks the current text without an unrelated test suite.", Completion: "The requested text is present with no unrelated changes.",
		Work: []domain.PlannedWork{{ID: "edit", GoalIDs: []string{"goal"}, Change: "Update requested text", Owner: "Existing text file", Necessity: "The goal cannot be delivered without this edit"}},
	}
}
func fixture(t *testing.T) (*Service, View, string) {
	t.Helper()
	s, v, root := proposedFixture(t)
	v, e := s.ConfirmPlan(context.Background(), ConfirmPlanRequest{Identity: Identity{v.Task.TaskID, "confirm", v.Task.Revision}, PlanDigest: v.PlanDigest, UserRef: "fixture-user-answer", Response: "I understand and approve this exact plan"})
	if e != nil {
		t.Fatal(e)
	}
	return s, v, root
}
func TestPlanConfirmationGuardsImplementationAndIsInvalidatedByRevision(t *testing.T) {
	s, v, root := proposedFixture(t)
	ctx := context.Background()
	before, e := s.PreWrite(ctx, root, []string{filepath.Join(root, "file.txt")}, true)
	if e != nil || before["decision"] != "deny" {
		t.Fatal("unconfirmed write allowed", before, e)
	}
	_, e = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "start-too-soon", v.Task.Revision}, Progress: []domain.Progress{{GoalID: "goal", Status: "doing", Summary: "editing"}}})
	if code(e) != "PLAN_CONFIRMATION_REQUIRED" {
		t.Fatal(e)
	}
	v, e = s.ConfirmPlan(ctx, ConfirmPlanRequest{Identity: Identity{v.Task.TaskID, "confirm", v.Task.Revision}, PlanDigest: v.PlanDigest, UserRef: "fixture-real-answer", Response: "Approved understanding and plan"})
	if e != nil {
		t.Fatal(e)
	}
	old := v.PlanDigest
	after, e := s.PreWrite(ctx, root, []string{filepath.Join(root, "file.txt")}, true)
	if e != nil || after["decision"] != "allow" {
		t.Fatal(after, e)
	}
	plan := v.Task.Plan
	plan.Approach = "A different approved approach must first be shown"
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "revise", v.Task.Revision}, Authorization: v.Task.Authorization, Checks: v.Task.Checks, Plan: plan, Reason: "Actual material design change"})
	if e != nil {
		t.Fatal(e)
	}
	if v.Task.Approval != nil || v.Decision.Advice != "ask_user" {
		t.Fatal("revision retained old approval")
	}
	_, e = s.ConfirmPlan(ctx, ConfirmPlanRequest{Identity: Identity{v.Task.TaskID, "stale-confirm", v.Task.Revision}, PlanDigest: old, UserRef: "old-answer", Response: "Old answer"})
	if code(e) != "PLAN_CHANGED" {
		t.Fatal("stale confirmation accepted", e)
	}
}
func TestExplicitAllowedScopeDiffersFromPredictedPaths(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	a := v.Task.Authorization
	a.AllowedPaths = []string{"file.txt"}
	a.ExpectedPaths = []string{"different-prediction.txt"}
	v, e := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "scope", v.Task.Revision}, Authorization: a, Plan: v.Task.Plan, Checks: v.Task.Checks, Reason: "User explicitly restricts edits to file.txt"})
	if e != nil {
		t.Fatal(e)
	}
	v, e = s.ConfirmPlan(ctx, ConfirmPlanRequest{Identity: Identity{v.Task.TaskID, "scope-confirm", v.Task.Revision}, PlanDigest: v.PlanDigest, UserRef: "scope-answer", Response: "Only file.txt may change"})
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
