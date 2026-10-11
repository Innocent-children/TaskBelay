package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func proposedFixture(t *testing.T) (*Service, View, string) {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git: %v %s", e, b)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "TaskBelay test")
	git("config", "user.email", "test@localhost")
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("base\n"), 0600)
	git("add", ".")
	git("commit", "-m", "base")
	db, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	s, _ := NewService(db, repository.NewGitObserver())
	v, e := s.Open(context.Background(), OpenRequest{Plan: testPlan(), OperationID: "open", Host: "cli", Repositories: []RepositoryInput{{Key: "primary", Path: root}}, Authorization: domain.Authorization{UserRef: "actual-test-user", Request: "Change file text", Goals: []domain.Goal{{ID: "goal", Statement: "Change the text", Checks: []string{"human"}}}, Limits: domain.DefaultLimits()}, Checks: []domain.Check{{ID: "human", Kind: "human", Rationale: "User checks requested text", Conditions: "Read current file"}}})
	if e != nil {
		t.Fatal(e)
	}
	return s, v, root
}
func code(e error) string {
	var v *domain.Error
	if errors.As(e, &v) {
		return v.Code
	}
	return ""
}
func TestObservationIsImmutableAndSamePathDriftPersists(t *testing.T) {
	s, v, root := fixture(t)
	original := v.Task.Repositories[0].Binding.ContentDigest
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("changed\n"), 0600)
	next, e := s.observe(context.Background(), v.Task)
	if e != nil {
		t.Fatal(e)
	}
	if v.Task.Repositories[0].Binding.ContentDigest != original || next.Repositories[0].Binding.ContentDigest == original {
		t.Fatal("observer mutated its input or missed content")
	}
	now, e := s.Inspect(context.Background(), v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := s.Store.Get(context.Background(), v.Task.TaskID)
	if e != nil || saved.ContentDigest() != now.ContentDigest || saved.Repositories[0].Binding.ContentDigest == original {
		t.Fatal("same-path change was not saved", e)
	}
}
func TestCompactionKeepsCostAndRejectsExpiredOperationReplay(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	first := CheckpointRequest{Identity: Identity{TaskID: v.Task.TaskID, OperationID: "first", Revision: v.Task.Revision}, HostMS: 7, WaitMS: 3, Tokens: 11}
	v, e := s.Checkpoint(ctx, first)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 280; i++ {
		task := v.Task
		expected := task.Revision
		task.Revision++
		e = s.Store.Save(ctx, task, expected, store.Operation{ID: "op-" + string(domain.Hash(i))[:12], Kind: "checkpoint", Digest: domain.Hash(i)}, store.Charge{HostMS: 1}, "", nil, "", "")
		if e != nil {
			t.Fatal(i, e)
		}
		v.Task, e = s.Store.Get(ctx, task.TaskID)
		if e != nil {
			t.Fatal(e)
		}
	}
	if v.Task.Usage.HostMS != 287 || v.Task.Usage.WaitMS != 3 || v.Task.Usage.Tokens != 11 {
		t.Fatal("compaction reset cost", v.Task.Usage)
	}
	_, e = s.Checkpoint(ctx, first)
	if code(e) != "REVISION_CONFLICT" {
		t.Fatal("expired identity was replayable", e)
	}
	if _, ok, e := s.Store.Operation(ctx, v.Task.TaskID, "open"); e != nil || !ok {
		t.Fatal("open identity was discarded")
	}
}
func TestUnknownMembersAndClaimsCannotFinishTask(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	_, e := s.RecordEvidence(ctx, EvidenceRequest{Identity: Identity{TaskID: v.Task.TaskID, OperationID: "fake-execution", Revision: v.Task.Revision}, CheckID: "human", Source: "core_execution", Status: "passed", Summary: "invented", ContentDigest: v.ContentDigest})
	if code(e) != "INVALID_ARGUMENT" {
		t.Fatal("fabricated execution accepted", e)
	}
	saved, _ := s.Store.Get(ctx, v.Task.TaskID)
	if saved.Revision != v.Task.Revision || saved.Usage.Runs != 0 {
		t.Fatal("rejected evidence changed task")
	}
	_, e = s.Finish(ctx, FinishRequest{Identity: Identity{TaskID: v.Task.TaskID, OperationID: "finish", Revision: v.Task.Revision}, Summary: "unverified"})
	if code(e) != "ACCEPTANCE_INCOMPLETE" {
		t.Fatal(e)
	}
}
func TestHumanObservationInvalidatesOnInputChange(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	v, e := s.RecordEvidence(ctx, EvidenceRequest{Identity: Identity{TaskID: v.Task.TaskID, OperationID: "human", Revision: v.Task.Revision}, CheckID: "human", Source: "human", Status: "passed", Summary: "I read the requested current text", UserRef: "actual-user-observation", ContentDigest: v.ContentDigest})
	if e != nil {
		t.Fatal(e)
	}
	if !v.Decision.Evidence[0].Valid {
		t.Fatal("actual human result unavailable")
	}
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("drift\n"), 0600)
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil || v.Decision.Evidence[0].Valid {
		t.Fatal("stale human acceptance retained", e)
	}
}
func TestWorkspaceClaimIsAtomicAndHistoryConflictCannotBeBlessed(t *testing.T) {
	s, v, root := fixture(t)
	ctx := context.Background()
	_, e := s.Open(ctx, OpenRequest{OperationID: "second", Host: "cli", Repositories: []RepositoryInput{{Key: "primary", Path: root}}, Authorization: v.Task.Authorization, Plan: v.Task.Plan, Checks: v.Task.Checks})
	if code(e) != "WORKSPACE_CLAIMED" {
		t.Fatal("duplicate claim allowed", e)
	}
	c := exec.Command("git", "-C", root, "switch", "-c", "other")
	if b, e := c.CombinedOutput(); e != nil {
		t.Fatal(e, string(b))
	}
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil || len(v.Task.Facts) == 0 || !v.Task.Facts[len(v.Task.Facts)-1].Blocking {
		t.Fatal("history conflict missed", e)
	}
	if v.Task.Repositories[0].Origin.TaskBranch != "main" {
		t.Fatal("conflicting branch became authority")
	}
}

func TestEvidenceCompactionKeepsApplicableResultAndCost(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	c := v.Task.Checks[0]
	f, e := execution.Observe(ctx, v.Task, c)
	if e != nil {
		t.Fatal(e)
	}
	current := domain.Evidence{ID: "evidence-current", CheckID: c.ID, Source: "human", Status: "passed", CheckDigest: v.Task.CheckDigest(c), InputsDigest: f.Inputs, EnvironmentDigest: f.Environment, UserRef: "fixture-user-observation", Summary: "Fixture observation of the actual current input", RecordedAt: time.Now().UTC()}
	task := v.Task
	task.Evidence = []domain.Evidence{current}
	for i := 0; i < 210; i++ {
		claim := current
		claim.ID = fmt.Sprintf("claim-%d", i)
		claim.Source = "claim"
		claim.InputsDigest = domain.Hash(i)
		task.Evidence = append(task.Evidence, claim)
	}
	before := task.Usage
	if e = s.boundEvidence(ctx, &task); e != nil {
		t.Fatal(e)
	}
	if len(task.Evidence) != 192 || task.Usage != before {
		t.Fatal("bounded retention changed cost", len(task.Evidence))
	}
	found := false
	for _, record := range task.Evidence {
		found = found || record.ID == current.ID
	}
	if !found {
		t.Fatal("oldest applicable result was lost to newer claims")
	}
	if _, e = s.save(ctx, task, store.Operation{ID: "bounded-evidence", Kind: "checkpoint", Digest: domain.Hash("bounded-evidence")}, store.Charge{}); e != nil {
		t.Fatal("retained snapshot cannot persist", e)
	}
}

func TestIndependentRevisionRetainsAnUnresolvedUserDecision(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	v, e := s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "question", v.Task.Revision}, Issue: &domain.Issue{Kind: "tradeoff", Summary: "A real user must choose the API behavior", Options: []string{"Retain API", "Authorize API change"}}})
	if e != nil {
		t.Fatal(e)
	}
	authorization := v.Task.Authorization
	authorization.UserRef = "fixture-user-independent-revision"
	authorization.Request += "; also describe the result"
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "independent-revision", v.Task.Revision}, Authorization: authorization, Plan: v.Task.Plan, Checks: v.Task.Checks, Reason: "This request does not answer the saved API question"})
	if e != nil || v.Task.Issue == nil || v.Decision.Advice != "ask_user" {
		t.Fatal("revision fabricated a user decision", e)
	}
}
