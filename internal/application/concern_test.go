package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func reviewConcern(t *testing.T, s *Service, v View, op string, concerns ...domain.ConcernInput) View {
	t.Helper()
	next, err := s.Checkpoint(context.Background(), CheckpointRequest{
		Identity:   Identity{v.Task.TaskID, op, v.Task.Revision},
		Progress:   []ProgressInput{{GoalID: "goal", Status: "done", Summary: "Implementation complete"}},
		Assessment: &AssessmentInput{AuthorizationDigest: v.AuthorizationDigest, ContentDigest: v.ContentDigest, Summary: "Review actual fixture", Review: &ReviewInput{Summary: "Existing responsibility and validation fit the request"}, Concerns: concerns},
	})
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func observeText(t *testing.T, s *Service, v View) View {
	t.Helper()
	next, err := s.RecordEvidence(context.Background(), EvidenceRequest{Identity: Identity{v.Task.TaskID, "observe", v.Task.Revision}, CheckID: "human", Source: "human", Status: "passed", Summary: "Fixture user read the current text", UserRef: "fixture-observation", ContentDigest: v.ContentDigest})
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestConcernsSurviveOmissionAndRequireApplicableResolution(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	plan := v.Task.Plan
	plan.Checks[0].Inputs = []string{"file.txt"}
	plan.Checks[0].InputBasis = "The observation concerns only the requested file"
	v, err := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "inputs", v.Task.Revision}, Plan: &plan, Reason: "Declare actual observation dependency"})
	if err != nil {
		t.Fatal(err)
	}
	c := domain.ConcernInput{ID: "behavior", Summary: "Confirm the requested text", GoalIDs: []string{"goal"}, CheckIDs: []string{"human"}, Paths: []string{"file.txt"}}
	v = reviewConcern(t, s, v, "question", c)
	v = reviewConcern(t, s, v, "omit-question")
	if len(v.Task.Assessment.Concerns) != 1 || v.Decision.Ready {
		t.Fatal("omission erased question", v.Decision)
	}
	_, err = s.Finish(ctx, FinishRequest{Identity: Identity{v.Task.TaskID, "blocked-finish", v.Task.Revision}, Summary: "Attempt premature completion"})
	var failure *domain.Error
	if !errors.As(err, &failure) || len(failure.Gaps) == 0 || domain.Hash(failure.Gaps) != domain.Hash(v.Decision.Gaps) {
		t.Fatal("finish omitted current specific gaps", err)
	}
	v = observeText(t, s, v)
	c.Resolution = &domain.ConcernResolution{Kind: "verified", Basis: "Actual observation covers the question", EvidenceIDs: []string{v.Task.Evidence[0].ID}}
	v = reviewConcern(t, s, v, "resolve", c)
	if !v.Decision.Ready {
		t.Fatal(v.Decision)
	}
	os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("unrelated"), 0600)
	v, err = s.Inspect(ctx, v.Task.TaskID, true)
	if err != nil || !v.Task.ConcernStates(v.Decision.Evidence)[0].Applicable {
		t.Fatal("unrelated change invalidated concern", err)
	}
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("changed"), 0600)
	v, err = s.Inspect(ctx, v.Task.TaskID, true)
	if err != nil || v.Task.ConcernStates(v.Decision.Evidence)[0].Applicable || v.Decision.Ready {
		t.Fatal("related change retained resolution", err)
	}
}

func TestConcernCannotBeRelabelledOrResolvedByClaim(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	c := domain.ConcernInput{ID: "question", Summary: "Need an actual observation", GoalIDs: []string{"goal"}}
	v = reviewConcern(t, s, v, "question", c)
	v, err := s.RecordEvidence(ctx, EvidenceRequest{Identity: Identity{v.Task.TaskID, "claim", v.Task.Revision}, CheckID: "human", Source: "claim", Status: "passed", Summary: "Agent thinks it passes", ContentDigest: v.ContentDigest})
	if err != nil {
		t.Fatal(err)
	}
	c.Resolution = &domain.ConcernResolution{Kind: "verified", Basis: "Attempt to use a claim", EvidenceIDs: []string{v.Task.Evidence[0].ID}}
	for i := 0; i < 2; i++ {
		_, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "invalid", v.Task.Revision}, Assessment: &AssessmentInput{AuthorizationDigest: v.AuthorizationDigest, ContentDigest: v.ContentDigest, Summary: "Invalid change", Concerns: []domain.ConcernInput{c}}})
		if code(err) != "INVALID_ARGUMENT" {
			t.Fatal(err)
		}
		c.Resolution = nil
		c.Summary = "A different question"
	}
	saved, _ := s.Store.Get(ctx, v.Task.TaskID)
	if saved.Revision != v.Task.Revision || saved.Assessment.Concerns[0].Resolution != nil {
		t.Fatal("invalid submission changed question")
	}
}

func TestLimitationRemainsVisibleAndCannotWaiveRequiredCheck(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	c := domain.ConcernInput{ID: "print", Summary: "Printing was not tested", GoalIDs: []string{"goal"}, Resolution: &domain.ConcernResolution{Kind: "not_blocking", Basis: "The request covers saved text, not printing"}}
	v = reviewConcern(t, s, v, "review", c)
	if v.Decision.Ready || len(v.Decision.MissingChecks) != 1 {
		t.Fatal("limitation waived acceptance", v.Decision)
	}
	v = observeText(t, s, v)
	v, err := s.Finish(ctx, FinishRequest{Identity: Identity{v.Task.TaskID, "finish", v.Task.Revision}, Summary: "Text complete", Acceptance: []Acceptance{{GoalID: "goal", EvidenceIDs: []string{v.Task.Evidence[0].ID}, Basis: "Actual current observation"}}})
	if err != nil || len(v.Task.Limitations) != 1 {
		t.Fatal(err, v.Task.Limitations)
	}
	v, err = s.Inspect(ctx, v.Task.TaskID, false)
	if err != nil || len(v.Task.DeliveryLimitations(nil)) != 1 {
		t.Fatal("terminal limitation lost", err)
	}
}

func TestScopeChangedConcernRequiresSavedUserBoundaryChange(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	c := domain.ConcernInput{ID: "scope", Summary: "Settle the requested output", GoalIDs: []string{"goal"}}
	v = reviewConcern(t, s, v, "question", c)
	c.Resolution = &domain.ConcernResolution{Kind: "scope_changed", Basis: "User narrowed the output", UserRef: "invented"}
	_, err := v.Task.MergeConcerns([]domain.ConcernInput{c}, v.Decision.Evidence)
	if code(err) != "INVALID_ARGUMENT" {
		t.Fatal("invented decision accepted", err)
	}
	a := v.Task.Authorization
	a.Goals = []domain.Goal{{ID: "goal", Statement: "Only inspect original text"}}
	a.UserRef = "actual-new-user-request"
	v, err = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "scope", v.Task.Revision}, Authorization: &a, Reason: "Actual user narrowed the output"})
	if err != nil {
		t.Fatal(err)
	}
	c.Resolution.UserRef = a.UserRef
	v = reviewConcern(t, s, v, "resolve", c)
	if !v.Task.ConcernStates(v.Decision.Evidence)[0].Applicable {
		t.Fatal("saved scope resolution lost")
	}
}
