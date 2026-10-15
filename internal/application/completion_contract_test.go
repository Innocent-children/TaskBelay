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
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "review", v.Task.Revision}, Progress: []ProgressInput{{GoalID: "goal", Status: "done", Summary: "Fixture implementation complete"}}, Assessment: &AssessmentInput{AuthorizationDigest: v.AuthorizationDigest, ContentDigest: v.ContentDigest, Summary: "Fixture review", Review: &ReviewInput{Summary: "Existing responsibility and relevant checks are appropriate"}}})
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

func TestCheckpointPreservesUnresolvedIssueUntilUserAnswers(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	issue := &domain.Issue{Kind: "authorization", Summary: "May this task change the public API?", Options: []string{"Retain", "Change"}}
	v, err := s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "ask", v.Task.Revision}, Issue: issue})
	if err != nil {
		t.Fatal(err)
	}
	for _, next := range []struct {
		name  string
		issue *domain.Issue
	}{{"omitted", nil}, {"unchanged", issue}} {
		v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, next.name, v.Task.Revision}, Issue: next.issue})
		if err != nil || domain.Hash(v.Task.Issue) != domain.Hash(issue) || v.Decision.Advice != "ask_user" {
			t.Fatal("checkpoint lost the unanswered question", err)
		}
	}
	for _, changed := range []domain.Issue{
		{Kind: "tradeoff", Summary: issue.Summary, Options: issue.Options},
		{Kind: issue.Kind, Summary: "Which label should be used?", Options: issue.Options},
		{Kind: issue.Kind, Summary: issue.Summary, Options: []string{"Different choice", "Change"}},
	} {
		_, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "replace", v.Task.Revision}, Issue: &changed})
		if code(err) != "ISSUE_CHANGED" {
			t.Fatal("unanswered question was replaced", err)
		}
		saved, err := s.Store.Get(ctx, v.Task.TaskID)
		if err != nil || saved.Revision != v.Task.Revision || domain.Hash(saved.Issue) != domain.Hash(issue) || code(requireExecutionAuthority(saved)) != "AUTHORIZATION_PENDING" {
			t.Fatal("rejected replacement changed the saved question or authority", err)
		}
		if _, exists, err := s.Store.Operation(ctx, v.Task.TaskID, "replace"); err != nil || exists {
			t.Fatal("rejected replacement recorded an operation", err)
		}
	}
	v, err = s.ResolveIssue(ctx, ResolveIssueRequest{Identity: Identity{v.Task.TaskID, "answer", v.Task.Revision}, IssueDigest: v.IssueDigest, UserRef: "actual-user-answer", Decision: "Retain the public API"})
	if err != nil {
		t.Fatal(err)
	}
	next := &domain.Issue{Kind: "tradeoff", Summary: "Which label should be used?", Options: []string{"First label", "Second label"}}
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "next-question", v.Task.Revision}, Issue: next})
	if err != nil || domain.Hash(v.Task.Issue) != domain.Hash(next) {
		t.Fatal("could not record a new question after the actual answer", err)
	}
	if last := v.Task.Changes[len(v.Task.Changes)-1]; last.Kind != "user_decision" || last.UserRef != "actual-user-answer" {
		t.Fatal("original user answer was not retained", last)
	}
}
