package application

import (
	"context"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func TestRegressionExceptionCannotReplaceAcceptance(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	plan := v.Task.Plan
	regression := domain.Check{ID: "regression", Kind: "command", Purpose: "regression", GoalIDs: []string{"goal"}, Argv: []string{"git", "rev-parse", "--verify", "missing-fixture-ref"}, Conditions: "local repository", Rationale: "Check existing regression", Retry: "repeatable", TimeoutMS: 3000}
	plan.Checks = []domain.Check{regression}
	_, err := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "missing-acceptance", v.Task.Revision}, Plan: &plan, Reason: "Attempt to replace acceptance with regression"})
	if code(err) != "INVALID_ARGUMENT" {
		t.Fatal("goal without acceptance was saved", err)
	}
	saved, _ := s.Store.Get(ctx, v.Task.TaskID)
	if saved.Revision != v.Task.Revision {
		t.Fatal("rejected plan changed the task")
	}
	comparison := domain.Check{ID: "comparison", Kind: "command", Purpose: "diagnostic", GoalIDs: []string{"goal"}, Compares: "regression", Argv: []string{"git", "status", "--porcelain"}, Conditions: "local repository", Rationale: "Compare existing failure", Retry: "repeatable", TimeoutMS: 3000}
	plan.Checks = append(v.Task.Plan.Checks, regression, comparison)
	v, err = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "with-regression", v.Task.Revision}, Plan: &plan, Reason: "Retain actual acceptance and check the known regression"})
	if err != nil {
		t.Fatal(err)
	}
	run := func(id string) string {
		r, e := s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "run-" + id, v.Task.Revision}, CheckID: id, Rationale: "Run the fixture check"})
		if e != nil {
			t.Fatal(e)
		}
		v = r.View
		return r.Run.RunID
	}
	failed, compared := run("regression"), run("comparison")
	v, err = s.AcceptFailure(ctx, AcceptFailureRequest{Identity: Identity{v.Task.TaskID, "accept-old", v.Task.Revision}, CheckID: "regression", FailedEvidenceID: failed, ComparisonEvidenceID: compared, Failures: []string{"existing regression"}, UserRef: "fixture-user-accepts", Reason: "Fixture explicitly accepts this known issue"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "review", v.Task.Revision}, Progress: []domain.Progress{{GoalID: "goal", Status: "done", Summary: "Fixture implementation complete"}}, Assessment: &domain.Assessment{AuthorizationDigest: v.AuthorizationDigest, ContentDigest: v.ContentDigest, Summary: "Fixture review", Review: &domain.DesignReview{Summary: "Existing responsibility and relevant checks are appropriate"}}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision.Ready || len(v.Decision.MissingChecks) != 1 || v.Decision.MissingChecks[0] != "human" {
		t.Fatal("exception satisfied acceptance", v.Decision)
	}
	v, err = s.RecordEvidence(ctx, EvidenceRequest{Identity: Identity{v.Task.TaskID, "human-pass", v.Task.Revision}, CheckID: "human", Source: "human", Status: "passed", Summary: "Fixture user inspected the text", UserRef: "fixture-observation", ContentDigest: v.ContentDigest})
	if err != nil || !v.Decision.Ready {
		t.Fatal(err, v.Decision)
	}
	_, err = s.Finish(ctx, FinishRequest{Identity: Identity{v.Task.TaskID, "finish-with-failure", v.Task.Revision}, Summary: "Done", Acceptance: []Acceptance{{GoalID: "goal", EvidenceIDs: []string{failed}, Basis: "Must reject using regression as acceptance"}}})
	if code(err) != "INVALID_ARGUMENT" {
		t.Fatal("failed regression cited as acceptance", err)
	}
	for _, ev := range v.Task.Evidence {
		if ev.CheckID == "human" {
			v, err = s.Finish(ctx, FinishRequest{Identity: Identity{v.Task.TaskID, "finish", v.Task.Revision}, Summary: "Done with separately accepted regression", Acceptance: []Acceptance{{GoalID: "goal", EvidenceIDs: []string{ev.ID}, Basis: "Actual user acceptance"}}})
			if err != nil || v.Task.Status != "done" || len(v.Decision.AcceptedFailures) != 1 {
				t.Fatal(err)
			}
			saved, err := s.Inspect(ctx, v.Task.TaskID, false)
			if err != nil || len(saved.Decision.AcceptedFailures) != 1 || saved.Decision.AcceptedFailures[0].FailedEvidenceID != failed {
				t.Fatal("delivered regression exception disappeared from saved result", err)
			}

		}
	}
}

func TestUserDecisionIsBoundAndPreservesRestrictions(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	v, err := s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "ask", v.Task.Revision}, Issue: &domain.Issue{Kind: "authorization", Summary: "Retain the current public API?", Options: []string{"Retain", "Change"}}})
	if err != nil {
		t.Fatal(err)
	}
	q := ResolveIssueRequest{Identity: Identity{v.Task.TaskID, "answer", v.Task.Revision}, IssueDigest: v.IssueDigest, UserRef: "actual-user-answer", Decision: "Retain the current API"}
	before := v.Task.Authorization.Digest()
	result, err := s.ResolveIssue(ctx, q)
	if err != nil || result.Task.Issue != nil || result.Task.Authorization.Digest() != before {
		t.Fatal(err, result)
	}
	replay, err := s.ResolveIssue(ctx, q)
	if err != nil || !replay.Replayed || len(replay.Task.Changes) != len(result.Task.Changes) {
		t.Fatal("decision replay duplicated history", err)
	}
}
