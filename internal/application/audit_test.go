package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func commandProposal(t *testing.T, n int, timeout int64) (*Service, View, string) {
	t.Helper()
	s, v, root := fixture(t)
	a := v.Task.Authorization
	checks := []domain.Check{}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("check-%d", i)
		checks = append(checks, domain.Check{GoalIDs: []string{"goal"}, ID: id, Kind: "command", Argv: []string{"git", "diff", "--quiet"}, Conditions: "local fixture", Rationale: "verify no unintended diff", TimeoutMS: timeout, Retry: "repeatable"})
	}
	v, e := s.Revise(context.Background(), ReviseRequest{Identity: Identity{v.Task.TaskID, "commands", v.Task.Revision}, Authorization: ptr(a), Plan: ptr(planWithChecks(v.Task.Plan, checks)), Reason: "Actual fixture plan"})
	if e != nil {
		t.Fatal(e)
	}
	return s, v, root
}
func TestAuditBudgetDistinguishesAggregateRiskFromImpossibleReservation(t *testing.T) {
	s, v, _ := commandProposal(t, 9, 120000)
	if !v.Budget.Feasible || len(v.Budget.Warnings) == 0 || v.Budget.WorstCaseMS != 1080000 {
		t.Fatal("worst timeout estimates became a hard refusal", v.Budget)
	}
	s, v, _ = commandProposal(t, 1, 600001)
	_, e := s.Execute(context.Background(), RunRequest{Identity: Identity{v.Task.TaskID, "impossible", v.Task.Revision}, CheckID: "check-0", Rationale: "Try to reserve more than the actual user limit"})
	if code(e) != "COST_LIMIT" || v.Budget.Feasible {
		t.Fatal(e, v.Budget)
	}
	saved, _ := s.Store.Get(context.Background(), v.Task.TaskID)
	if saved.Usage.Runs != 0 {
		t.Fatal("unreservable command was executed")
	}
}
func TestAuditReservationAfterShortRunAndAtomicFourHourBudgetDecision(t *testing.T) {
	ctx := context.Background()
	s, v, _ := commandProposal(t, 1, 600000)
	task := v.Task
	task.Revision++
	if e := s.Store.Save(ctx, task, v.Task.Revision, store.Operation{ID: "fixture-short-cost", Kind: "checkpoint", Digest: domain.Hash("cost")}, store.Charge{ExecutionMS: 2500}, "", nil, "", ""); e != nil {
		t.Fatal(e)
	}
	v, _ = s.Inspect(ctx, task.TaskID, true)
	_, e := s.Execute(ctx, RunRequest{Identity: Identity{task.TaskID, "next-too-large", v.Task.Revision}, CheckID: "check-0", Rationale: "Establish current required result"})
	var de *domain.Error
	if !errors.As(e, &de) || de.Code != "COST_LIMIT" || de.Budget.Actual != 2500 || de.Budget.Reserved != 0 || de.Budget.Requested != 600000 || de.Budget.Remaining != 597500 {
		t.Fatal(e, de)
	}
	if e = s.Store.AddCost(ctx, task.TaskID, 888000, 0); e != nil {
		t.Fatal(e)
	}
	v, _ = s.Inspect(ctx, task.TaskID, false)
	q := BudgetRequest{Identity: Identity{task.TaskID, "actual-four-hour-decision", v.Task.Revision}, AuthorizationDigest: v.AuthorizationDigest, Budget: domain.BudgetDecision{Runs: 40, ExecutionMS: 14400000, WorkMS: 14400000, Tokens: 1000000}, UserRef: "explicit-four-hour-40-run-answer", Response: "Approve cumulative command and work time four hours and 40 runs"}
	v, e = s.UpdateBudget(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	if v.Task.Authorization.Limits.CoreMS != 14400000 || v.Task.Authorization.Limits.HostMS != 14400000 || v.Task.Usage.ExecutionMS != 2500 || v.Task.Usage.CoreMS != 888000 || v.Task.BudgetDecision.UserRef != q.UserRef {
		t.Fatal("budget decision lost a limit, source or cost", v.Task)
	}
	replay, e := s.UpdateBudget(ctx, q)
	if e != nil || !replay.Replayed || replay.Task.Revision != v.Task.Revision {
		t.Fatal("budget replay applied twice", e)
	}
	run, e := s.Execute(ctx, RunRequest{Identity: Identity{task.TaskID, "after-update", v.Task.Revision}, CheckID: "check-0", Rationale: "Verify after complete approved limit update"})
	if e != nil || run.Run.Status != "passed" {
		t.Fatal("stale Core cap still blocked", e)
	}
}
func TestRoutineCheckRevisionRetainsResultsButCannotExpandUserLimits(t *testing.T) {
	ctx := context.Background()
	s, v, _ := commandProposal(t, 1, 5000)
	run, e := s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "first", v.Task.Revision}, CheckID: "check-0", Rationale: "Verify clean fixture"})
	if e != nil {
		t.Fatal(e)
	}
	v = run.View
	checks := append([]domain.Check{}, v.Task.Plan.Checks...)
	checks[0].TimeoutMS = 10000
	checks[0].Rationale = "Allow ordinary environment variance"
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "method", v.Task.Revision}, Plan: ptr(planWithChecks(v.Task.Plan, checks)), Reason: "Adjust method without changing the user outcome"})
	if e != nil || !v.Decision.Evidence[0].Valid || v.Decision.Advice == "ask_user" {
		t.Fatal(e, v.Decision)
	}
	a := v.Task.Authorization
	a.Limits.ExecutionMS++
	_, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "expand", v.Task.Revision}, Authorization: &a, Reason: "Expansion has no new user decision"})
	if code(e) != "AUTHORIZATION_REQUIRED" {
		t.Fatal(e)
	}
	a.Limits.ExecutionMS = 0
	_, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "remove-limit", v.Task.Revision}, Authorization: &a, Reason: "Removing an explicit limit also expands it"})
	if code(e) != "AUTHORIZATION_REQUIRED" {
		t.Fatal(e)
	}
}
func TestNoImplicitBudgetAndPartialUserLimit(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	a := v.Task.Authorization
	a.UserRef = "user-without-budget"
	a.Limits = domain.Limits{}
	checks := []domain.Check{{GoalIDs: []string{"goal"}, ID: "command", Kind: "command", Argv: []string{"git", "status", "--porcelain"}, Conditions: "local", Rationale: "Inspect actual fixture", TimeoutMS: 1000}}
	v, e := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "no-budget", v.Task.Revision}, Authorization: &a, Plan: ptr(planWithChecks(v.Task.Plan, checks)), Reason: "This user set no cost limits"})
	if e != nil {
		t.Fatal(e)
	}
	for _, line := range v.Budget.Limits {
		if line.Limited {
			t.Fatal("invented user limit", line)
		}
	}
	if e = s.Store.AddCost(ctx, v.Task.TaskID, 700000, 0); e != nil {
		t.Fatal(e)
	}
	run, e := s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "over-old-default", v.Task.Revision}, CheckID: "command", Rationale: "Normal check remains available without a user cap"})
	if e != nil || run.Run.Status != "passed" {
		t.Fatal(e)
	}
	v = run.View
	a = v.Task.Authorization
	a.Limits.Runs = 1
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "run-limit", v.Task.Revision}, Authorization: &a, Reason: "Set a single execution limit"})
	if e != nil || v.Task.Usage.Runs != 1 || v.Task.Usage.CoreMS < 700000 {
		t.Fatal(e)
	}
	checks[0].Argv = []string{"git", "diff", "--quiet"}
	v, e = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "next-check", v.Task.Revision}, Plan: ptr(planWithChecks(v.Task.Plan, checks)), Reason: "Verify a different condition"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "capped", v.Task.Revision}, CheckID: "command", Rationale: "The actual user cap must still apply"})
	if code(e) != "COST_LIMIT" {
		t.Fatal(e)
	}
}
func TestAuditExactMultiRepositoryDependencyPermissionAndForbiddenWrite(t *testing.T) {
	ctx := context.Background()
	s, v, root := fixture(t)
	_, e := s.Cancel(ctx, CancelRequest{Identity: Identity{v.Task.TaskID, "release-fixture", v.Task.Revision}, Reason: "Prepare multi-repository fixture"})
	if e != nil {
		t.Fatal(e)
	}
	second, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	git := func(root string, args ...string) {
		t.Helper()
		if out, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
			t.Fatal(e, string(out))
		}
	}
	git(second, "init", "-b", "main")
	git(second, "config", "user.name", "test")
	git(second, "config", "user.email", "test@localhost")
	for _, path := range []string{root, second} {
		os.WriteFile(filepath.Join(path, "package.json"), []byte(`{"dependencies":{}}`), 0600)
		git(path, "add", ".")
		git(path, "commit", "-m", "manifest baseline")
	}
	a := v.Task.Authorization
	a.Dependencies = []domain.DependencyPolicy{{Path: "web::package.json", Allowed: []string{"lucide-react"}}, {Path: "backend::package.json", Allowed: []string{}}}
	a.ForbiddenPaths = []string{"web::secret.txt"}
	checks := append([]domain.Check{}, v.Task.Plan.Checks...)
	for i := range checks {
		checks[i].RepositoryKey = "web"
	}
	v, e = s.Open(ctx, OpenRequest{OperationID: "multi-dependency", Host: "cli", Repositories: []RepositoryInput{{Key: "web", Path: root}, {Key: "backend", Path: second}}, Authorization: a, Plan: planWithChecks(v.Task.Plan, checks)})
	if e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"lucide-react":"1.0.0"}}`), 0600)
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range v.Task.Facts {
		if f.Kind == "dependency_change" && f.Blocking {
			t.Fatal("approved web dependency refused", f)
		}
	}
	for _, section := range []string{"devDependencies", "optionalDependencies", "peerDependencies"} {
		contents := fmt.Sprintf(`{"%s":{"lucide-react":"1.0.0"}}`, section)
		os.WriteFile(filepath.Join(root, "package.json"), []byte(contents), 0600)
		v, e = s.Inspect(ctx, v.Task.TaskID, true)
		if e != nil {
			t.Fatal(e)
		}
		for _, f := range v.Task.Facts {
			if f.Kind == "dependency_change" && f.Blocking {
				t.Fatal("section prefix overrode approved package", section, f)
			}
		}
	}

	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"lucide-react":"1.0.0","unauthorized-package":"1.0.0"}}`), 0600)
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	unapproved := false
	for _, f := range v.Task.Facts {
		if f.Kind == "dependency_change" && f.Blocking && strings.Contains(f.Detail, "unauthorized-package") {
			unapproved = true
		}
	}
	if !unapproved {
		t.Fatal("normalizing a section granted a different package")
	}
	denied, e := s.PreWrite(ctx, root, []string{filepath.Join(root, "secret.txt")}, true)
	if e != nil || denied["decision"] != "deny" {
		t.Fatal("dependency policy granted forbidden write", e)
	}
	os.WriteFile(filepath.Join(second, "package.json"), []byte(`{"dependencies":{"lucide-react":"1.0.0"}}`), 0600)
	v, e = s.Inspect(ctx, v.Task.TaskID, true)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, f := range v.Task.Facts {
		if f.Kind == "dependency_change" && f.Blocking && f.Path == "backend::package.json" && strings.Contains(f.Detail, "explicit manifest restriction") {
			found = true
		}
	}
	if !found {
		t.Fatal("other repository borrowed web dependency permission")
	}
}

func TestAuditRemoteBaselineRetainedAndForgedFreshnessRefused(t *testing.T) {
	ctx := context.Background()
	s, v, root := fixture(t)
	git := func(args ...string) string {
		t.Helper()
		out, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if e != nil {
			t.Fatal(e, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, e := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	git("remote", "add", "origin", remote)
	git("push", "-u", "origin", "main")
	git("fetch", "--no-tags", "origin")
	head := git("rev-parse", "HEAD")
	fetchedAt := time.Now().UTC()
	f := domain.RemoteFreshness{FetchedAt: &fetchedAt, RemoteName: "origin", RemoteRef: "refs/heads/main", Head: head, Upstream: "refs/remotes/origin/main", RemoteHead: head, State: "current", Fetched: true, CheckedAt: time.Now().UTC(), Basis: "actual fixture fetch and comparison"}
	baseline := *v.Task.Repositories[0].RemoteBaseline
	v, e := s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "remote-recheck", v.Task.Revision}, RepositoryFreshness: []domain.RepositoryFreshness{{Key: "primary", Observation: f}}})
	if e != nil {
		t.Fatal(e)
	}
	if v.Task.Repositories[0].RemoteBaseline.Head != baseline.Head || v.Task.Repositories[0].RemoteBaseline.State != "not_checked" || v.Task.Repositories[0].RemoteLatest.State != "current" {
		t.Fatal("later fetch erased baseline")
	}
	f.RemoteHead = strings.Repeat("0", 40)
	_, e = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "forged-fetch", v.Task.Revision}, RepositoryFreshness: []domain.RepositoryFreshness{{Key: "primary", Observation: f}}})
	if code(e) != "REMOTE_OBSERVATION_STALE" {
		t.Fatal("unverified remote commit accepted", e)
	}
}

func TestAuditReviewBoundSourceUpstreamAndRemoteOnlyOrigin(t *testing.T) {
	for _, remoteOnly := range []bool{false, true} {
		t.Run(fmt.Sprint(remoteOnly), func(t *testing.T) {
			ctx := context.Background()
			s, v, root := fixture(t)
			_, err := s.Cancel(ctx, CancelRequest{Identity: Identity{v.Task.TaskID, "retire-fixture", v.Task.Revision}, Reason: "Prepare the separately selected native fixture workspace"})
			if err != nil {
				t.Fatal(err)
			}
			git := func(dir string, args ...string) string {
				t.Helper()
				out, e := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
				if e != nil {
					t.Fatal(e, string(out))
				}
				return strings.TrimSpace(string(out))
			}
			remote := filepath.Join(t.TempDir(), "remote.git")
			out, e := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput()
			if e != nil {
				t.Fatal(e, string(out))
			}
			git(root, "remote", "add", "origin", remote)
			git(root, "push", "-u", "origin", "main")
			head := git(root, "rev-parse", "HEAD")
			branch, work := "main", root
			origin := &OriginInput{Mode: domain.WorkspaceModeNewBranch, SourceType: "local", CarryChanges: false, BaseBranch: "main", BaseCommit: head, TaskBranch: "repair", ProvisioningReceiptID: "review-fixture-origin"}
			if remoteOnly {
				git(root, "push", "origin", "HEAD:refs/heads/release")
				git(root, "fetch", "origin")
				work = filepath.Join(t.TempDir(), "repair-worktree")
				git(root, "worktree", "add", "--detach", work, "origin/release")
				git(work, "switch", "--no-track", "-c", "repair")
				branch = "release"
				origin.Mode = domain.WorkspaceModeDedicatedWorktree
				origin.SourceType = "remote"
				origin.RemoteName = "origin"
				origin.BaseBranch = branch
				if exec.Command("git", "-C", work, "show-ref", "--verify", "refs/heads/release").Run() == nil {
					t.Fatal("fixture unexpectedly has local release")
				}
			} else {
				git(root, "switch", "-c", "repair")
				git(root, "push", "-u", "origin", "repair")
			}
			git(work, "fetch", "--no-tags", "origin")
			fetched := time.Now().UTC()
			f := domain.RemoteFreshness{Head: head, Upstream: "refs/remotes/origin/" + branch, RemoteName: "origin", RemoteRef: "refs/heads/" + branch, RemoteHead: head, State: "current", Fetched: true, FetchedAt: &fetched, CheckedAt: time.Now().UTC(), Basis: "Actual fetch of the explicitly bound original source"}
			opened, e := s.Open(ctx, OpenRequest{OperationID: "review-source-open", Host: v.Task.Host, Repositories: []RepositoryInput{{Key: "primary", Path: work, Origin: origin, RemoteFreshness: &f}}, Authorization: v.Task.Authorization, Plan: v.Task.Plan})
			if e != nil {
				t.Fatal("correct bound origin refused", e)
			}
			_, e = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{opened.Task.TaskID, "review-source-recheck", opened.Task.Revision}, RepositoryFreshness: []domain.RepositoryFreshness{{Key: "primary", Observation: f}}})
			if e != nil {
				t.Fatal("correct source checkpoint refused", e)
			}
			wrong := f
			wrong.Upstream = "refs/remotes/origin/repair"
			wrong.RemoteRef = "refs/heads/repair"
			_, e = s.Observer.VerifyRemoteFreshness(ctx, work, opened.Task.Repositories[0].Binding, opened.Task.Repositories[0].Origin, &wrong)
			if code(e) != "REMOTE_OBSERVATION_STALE" {
				t.Fatal("task upstream impersonated source", e)
			}
			if !remoteOnly {
				git(work, "branch", "--set-upstream-to=origin/repair", "main")
				_, e = s.Observer.VerifyRemoteFreshness(ctx, work, opened.Task.Repositories[0].Binding, opened.Task.Repositories[0].Origin, &f)
				if code(e) != "REMOTE_OBSERVATION_STALE" {
					t.Fatal("changed bound source upstream accepted", e)
				}
			}
		})
	}
}

func TestAuditReviewDeniedOwnerQueryKeepsPendingMetering(t *testing.T) {
	s, _, _ := fixture(t)
	ctx := context.Background()
	if e := s.Store.BeginControlCall(ctx, "owner-query-fixture", os.Getpid(), "retained-owner"); e != nil {
		t.Fatal(e)
	}
	cause := &os.PathError{Op: "fork/exec", Path: "/bin/ps", Err: os.ErrPermission}
	_, e := s.controlCost(ctx, func(execution.Run) (bool, error) { return false, cause })
	var meter *ControlMeterError
	if !errors.As(e, &meter) || meter.Stage != "process_identity" || !errors.Is(e, os.ErrPermission) {
		t.Fatal(e)
	}
	retained, e := s.Store.ControlCost(ctx, nil)
	if e != nil || retained.Pending != 1 || retained.UnknownMS != 0 {
		t.Fatal("denial settled an owner as dead", retained, e)
	}
}
