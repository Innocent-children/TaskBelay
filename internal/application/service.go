// Package application composes Core control, observations, execution and atomic persistence.
package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type Service struct {
	Store    *store.Store
	Observer *repository.GitObserver
}

func NewService(s *store.Store, o *repository.GitObserver) (*Service, error) {
	if s == nil || o == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &Service{s, o}, nil
}

type Identity struct {
	TaskID      string `json:"task_id"`
	OperationID string `json:"operation_id"`
	Revision    uint64 `json:"revision"`
}
type RepositoryInput struct {
	Key    domain.RepositoryKey `json:"key"`
	Path   string               `json:"repository_path"`
	Origin *OriginInput         `json:"workspace_origin,omitempty"`
}
type OpenRequest struct {
	OperationID   string               `json:"operation_id"`
	Host          domain.Host          `json:"host"`
	Repositories  []RepositoryInput    `json:"repositories"`
	Authorization domain.Authorization `json:"authorization"`
	Checks        []domain.Check       `json:"checks"`
	Plan          domain.Plan          `json:"plan"`
}
type View struct {
	Task                domain.Task        `json:"task"`
	PlanDigest          domain.Digest      `json:"plan_digest"`
	AuthorizationDigest domain.Digest      `json:"authorization_digest"`
	ContentDigest       domain.Digest      `json:"content_digest"`
	Decision            domain.Decision    `json:"decision"`
	Refreshed           bool               `json:"refreshed"`
	Replayed            bool               `json:"replayed"`
	ControlCost         store.ControlUsage `json:"control_cost"`
}

func selection(o domain.WorkspaceOrigin) repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: o.Mode, SourceType: o.SourceType, CarryChanges: o.CarryChanges, RemoteName: o.RemoteName, BaseBranch: o.BaseBranch, BaseCommit: o.BaseCommit, TaskBranch: o.TaskBranch, ProvisioningReceiptID: o.ProvisioningReceiptID}
}
func (s *Service) Open(ctx context.Context, q OpenRequest) (View, error) {
	if !domain.ID(q.OperationID).IsValid() {
		return View{}, domain.Invalid("operation_id", "requires a stable canonical operation identity")
	}
	if !q.Host.IsValid() {
		return View{}, domain.Invalid("host", "must be codex, deepseek, claude, zcode or cli")
	}
	if len(q.Repositories) == 0 || len(q.Repositories) > 8 {
		return View{}, domain.Invalid("repositories", "requires 1..8 explicitly selected repositories")
	}
	id := "task-" + string(domain.Hash(q.OperationID))[:24]
	digest := domain.Hash(q)
	if o, ok, e := s.Store.Operation(ctx, id, q.OperationID); e != nil {
		return View{}, e
	} else if ok {
		if o.Digest != digest {
			return View{}, domain.Fail("OPERATION_CONFLICT", "operation_id", "operation identity was used for different content")
		}
		t, e := s.Store.Get(ctx, id)
		if e != nil {
			return View{}, e
		}
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	if q.Authorization.Limits == (domain.Limits{}) {
		q.Authorization.Limits = domain.DefaultLimits()
	}
	if e := q.Authorization.Validate(); e != nil {
		return View{}, e
	}
	now := time.Now().UTC()
	t := domain.Task{TaskID: id, Host: q.Host, Status: "active", Revision: 1, Authorization: q.Authorization, Plan: q.Plan, Checks: normalizeChecks(q.Checks), Repositories: []domain.RepositoryScopeEntry{}, Progress: []domain.Progress{}, Evidence: []domain.Evidence{}, Attempts: []domain.Attempt{}, Facts: []domain.Fact{}, Risks: []string{}, CreatedAt: now, UpdatedAt: now}
	for i, input := range q.Repositories {
		if input.Key == "" && i == 0 {
			input.Key = "primary"
		}
		b, e := s.Observer.Observe(ctx, input.Path)
		if e != nil {
			return View{}, domain.Fail("OBSERVATION_UNAVAILABLE", "repositories", "could not observe an existing Git worktree: "+e.Error())
		}
		if b.CurrentBranch == nil {
			return View{}, domain.Fail("WORKSPACE_CONFLICT", "repositories", "a task requires a bound branch")
		}
		o := input.Origin
		if o == nil {
			o = &OriginInput{Mode: domain.WorkspaceModeCurrentBranch, SourceType: "local", CarryChanges: true, BaseBranch: *b.CurrentBranch, BaseCommit: b.CurrentHead, TaskBranch: *b.CurrentBranch, ProvisioningReceiptID: domain.ID(q.OperationID)}
		}
		origin, b, e := s.Observer.ObserveWorkspace(ctx, input.Path, o.selection(), nil)
		if e != nil {
			return View{}, domain.Fail("OBSERVATION_UNAVAILABLE", "repositories", "workspace origin could not be verified: "+e.Error())
		}
		t.Repositories = append(t.Repositories, domain.RepositoryScopeEntry{Key: input.Key, Origin: origin, Binding: b})
	}
	t.ProposedContentDigest = t.ContentDigest()
	if e := t.Validate(); e != nil {
		return View{}, e
	}
	facts, e := repository.ScopeFacts(ctx, t)
	if e != nil {
		return View{}, e
	}
	t.Facts = facts
	if e = s.Store.Create(ctx, t, store.Operation{ID: q.OperationID, Kind: "open", Digest: digest}); e != nil {
		return View{}, e
	}
	return s.view(ctx, t, true)
}
func normalizeChecks(cs []domain.Check) []domain.Check {
	cs = slices.Clone(cs)
	for i := range cs {
		if cs[i].RepositoryKey == "" {
			cs[i].RepositoryKey = "primary"
		}
		if cs[i].Purpose == "" {
			cs[i].Purpose = "acceptance"
		}
		if cs[i].Cwd == "" {
			cs[i].Cwd = "."
		}
		if len(cs[i].Inputs) == 0 {
			cs[i].Inputs = []string{"."}
		}
		if cs[i].Kind == "command" && cs[i].TimeoutMS == 0 {
			cs[i].TimeoutMS = 60000
		}
		if cs[i].Kind == "command" && cs[i].Retry == "" {
			cs[i].Retry = "requires_user"
		}
	}
	return cs
}
func (s *Service) observe(ctx context.Context, t domain.Task) (domain.Task, error) {
	t.Repositories = slices.Clone(t.Repositories)
	start := time.Now()
	defer func() { s.Store.AddObservation(context.WithoutCancel(ctx), t.TaskID, time.Since(start).Milliseconds()) }()
	conflicts := []domain.Fact{}
	for i, r := range t.Repositories {
		o, b, e := s.Observer.ObserveWorkspace(ctx, r.Origin.CanonicalWorktreeRoot, selection(r.Origin), &r.Binding)
		if e != nil {
			return t, domain.Fail("OBSERVATION_UNAVAILABLE", "repositories", "current workspace observation failed: "+e.Error())
		}
		if b.WorktreeInstanceDigest != r.Binding.WorktreeInstanceDigest || o.WorktreeGitDirDigest != r.Origin.WorktreeGitDirDigest || b.CurrentBranch == nil || *b.CurrentBranch != r.Origin.TaskBranch || !b.BaseCommitAncestor || b.HistoryRelation != domain.RepositoryHistoryExact && b.HistoryRelation != domain.RepositoryHistoryLinearAdvance {
			conflicts = append(conflicts, domain.Fact{ID: "workspace:" + string(r.Key), Kind: "workspace_conflict", Path: string(r.Key), Detail: "Restore the original workspace instance, branch and linear history before proceeding.", Blocking: true})
			continue
		}
		t.Repositories[i].Binding = b
	}
	facts, e := repository.ScopeFacts(ctx, t)
	if e != nil {
		return t, e
	}
	t.Facts = append(facts, conflicts...)
	return t, nil
}
func (s *Service) Inspect(ctx context.Context, id string, refresh bool) (View, error) {
	t, e := s.Store.Get(ctx, id)
	if e != nil {
		return View{}, e
	}
	if refresh && t.Status == "active" {
		next, e := s.observe(ctx, t)
		if e != nil {
			return View{}, e
		}
		changed := domain.Hash(t.Facts) != domain.Hash(next.Facts)
		for i, r := range t.Repositories {
			changed = changed || r.Binding.BindingDigest != next.Repositories[i].Binding.BindingDigest
		}
		if changed {
			expected := t.Revision
			next.Revision++
			next.UpdatedAt = time.Now().UTC()
			if e = s.Store.Save(ctx, next, expected, store.Operation{}, store.Charge{}, "", nil, "", ""); e != nil {
				return View{}, e
			}
			t = next
		}
	}
	t, e = s.Store.Get(ctx, id)
	if e != nil {
		return View{}, e
	}
	return s.view(ctx, t, refresh && t.Status == "active")
}
func (s *Service) view(ctx context.Context, t domain.Task, refreshed bool) (View, error) {
	pending, e := s.Store.Pending(ctx, t.TaskID)
	if e != nil {
		return View{}, e
	}
	fresh := []domain.Freshness{}
	probes := map[string]execution.Fingerprint{}
	probeErrors := map[string]error{}
	for _, v := range t.Evidence {
		f := domain.Freshness{EvidenceID: v.ID, CheckID: v.CheckID, Reason: "current evidence"}
		if !refreshed {
			f.Reason = "saved evidence; refresh actual conditions to assess applicability"
			fresh = append(fresh, f)
			continue
		}
		c, ok := t.Check(v.CheckID)
		if !ok {
			f.Reason = "check removed"
		} else if v.Source == "claim" {
			f.Reason = "agent claim does not prove execution or acceptance"
		} else if v.CheckDigest != t.CheckDigest(c) {
			f.Reason = "check, covered requirement or relevant constraint changed"
		} else {
			probe, seen := probes[c.ID]
			er := probeErrors[c.ID]
			if !seen && er == nil {
				probe, er = execution.Observe(ctx, t, c)
				if er != nil {
					probeErrors[c.ID] = er
				} else {
					probes[c.ID] = probe
				}
			}
			if er != nil {
				f.Reason = "applicability could not be observed"
			} else if probe.Inputs != v.InputsDigest {
				f.Reason = "declared code inputs changed"
			} else if probe.Environment != v.EnvironmentDigest {
				f.Reason = "declared environment or executable changed"
			} else {
				f.Applicable = true
				f.Valid = v.Status == "passed"
				if !f.Valid {
					f.Reason = "check did not pass"
				}
			}
		}
		fresh = append(fresh, f)
	}
	cost, e := s.ControlCost(ctx)
	if e != nil {
		return View{}, e
	}
	return View{Task: t, PlanDigest: t.PlanDigest(), AuthorizationDigest: t.Authorization.Digest(), ContentDigest: t.ContentDigest(), Decision: domain.Decide(t, fresh, pending), Refreshed: refreshed, ControlCost: cost}, nil
}

func (s *Service) ControlCost(ctx context.Context) (store.ControlUsage, error) {
	return s.Store.ControlCost(ctx, func(pid int, id string) bool {
		return execution.OwnerAlive(execution.Run{OwnerPID: pid, OwnerIdentity: id})
	})
}
func (s *Service) begin(ctx context.Context, q Identity, kind string, payload any) (domain.Task, store.Operation, bool, error) {
	if !domain.ID(q.TaskID).IsValid() || !domain.ID(q.OperationID).IsValid() || q.Revision == 0 {
		return domain.Task{}, store.Operation{}, false, domain.Invalid("identity", "requires task_id, operation_id and current revision")
	}
	o := store.Operation{ID: q.OperationID, Kind: kind, Digest: domain.Hash(payload)}
	old, ok, e := s.Store.Operation(ctx, q.TaskID, q.OperationID)
	if e != nil {
		return domain.Task{}, o, false, e
	}
	t, e := s.Store.Get(ctx, q.TaskID)
	if e != nil {
		return t, o, false, e
	}
	if ok {
		if old.Kind != kind || old.Digest != o.Digest {
			return t, o, false, domain.Fail("OPERATION_CONFLICT", "operation_id", "saved identity belongs to a different operation")
		}
		return t, old, true, nil
	}
	if t.Revision != q.Revision {
		return t, o, false, domain.Fail("REVISION_CONFLICT", "revision", "read the latest task and submit a new operation identity")
	}
	if t.Status != "active" && kind != "archive" {
		return t, o, false, domain.Fail("TASK_TERMINAL", "task_id", "terminal tasks cannot be changed")
	}
	return t, o, false, nil
}
func (s *Service) save(ctx context.Context, t domain.Task, o store.Operation, c store.Charge) (View, error) {
	if e := s.boundEvidence(ctx, &t); e != nil {
		return View{}, e
	}
	expected := t.Revision
	t.Revision++
	t.UpdatedAt = time.Now().UTC()
	if e := s.Store.Save(ctx, t, expected, o, c, "", nil, "", ""); e != nil {
		return View{}, e
	}
	if e := s.compactReceipts(ctx); e != nil {
		return View{}, domain.Fail("RESOURCE_CLEANUP_FAILED", "", "task was saved; retired receipt cleanup failed, so read saved state before any retry")
	}
	t, e := s.Store.Get(ctx, t.TaskID)
	if e != nil {
		return View{}, e
	}
	return s.view(ctx, t, true)
}

type CheckpointRequest struct {
	Identity
	Assessment    *domain.Assessment `json:"assessment,omitempty"`
	Progress      []domain.Progress  `json:"progress,omitempty"`
	Issue         *domain.Issue      `json:"issue,omitempty"`
	ClearIssue    bool               `json:"clear_issue,omitempty"`
	ResolutionRef string             `json:"resolution_ref,omitempty"`
	HostMS        int64              `json:"host_ms,omitempty"`
	WaitMS        int64              `json:"wait_ms,omitempty"`
	Tokens        int64              `json:"tokens,omitempty"`
}

func (s *Service) Checkpoint(ctx context.Context, q CheckpointRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "checkpoint", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
	}
	if q.HostMS < 0 || q.WaitMS < 0 || q.Tokens < 0 {
		return View{}, domain.Invalid("cost", "reported cost deltas must be nonnegative")
	}
	if len(q.Progress) > 0 {
		if e = requireExecutionAuthority(t); e != nil {
			return View{}, e
		}
	}
	for _, p := range q.Progress {
		g, ok := t.Goal(p.GoalID)
		if !ok || !slices.Contains([]string{"planned", "doing", "done"}, p.Status) || domain.Text("progress.summary", p.Summary) != nil {
			return View{}, domain.Invalid("progress", "must reference an existing goal, status planned/doing/done and a concrete summary")
		}
		p.GoalDigest = domain.Hash(g)
		found := false
		for i, old := range t.Progress {
			if old.GoalID == p.GoalID {
				t.Progress[i] = p
				found = true
			}
		}
		if !found {
			t.Progress = append(t.Progress, p)
		}
	}
	if q.Assessment != nil {
		if e = validateAssessment(t, *q.Assessment); e != nil {
			return View{}, e
		}
		t.Assessment = t.MergeAssessment(*q.Assessment)
	}
	if q.Issue != nil {
		if !slices.Contains([]string{"authorization", "tradeoff"}, q.Issue.Kind) || domain.Text("issue.summary", q.Issue.Summary) != nil || len(q.Issue.Options) < 2 || len(q.Issue.Options) > 8 {
			return View{}, domain.Invalid("issue", "only an unresolved authorization or important tradeoff with concrete options belongs to the user")
		}
		t.Issue = q.Issue
	}
	if q.ClearIssue {
		if domain.Text("resolution_ref", q.ResolutionRef) != nil {
			return View{}, domain.Invalid("resolution_ref", "a real user decision is required to resolve a saved question")
		}
		t.Issue = nil
	}
	return s.save(ctx, t, o, store.Charge{HostMS: q.HostMS, WaitMS: q.WaitMS, Tokens: q.Tokens})
}
func validateAssessment(t domain.Task, a domain.Assessment) error {
	if a.AuthorizationDigest != t.Authorization.Digest() || a.ContentDigest != t.ContentDigest() {
		return domain.Fail("ASSESSMENT_STALE", "assessment", "review must bind the current authorization_digest and content_digest")
	}
	if e := domain.Text("assessment.summary", a.Summary); e != nil {
		return e
	}
	if a.Review != nil {
		for _, f := range []struct{ name, value string }{{"root_cause", a.Review.RootCause}, {"responsibilities", a.Review.Responsibilities}, {"proportionality", a.Review.Proportionality}, {"verification", a.Review.Verification}} {
			if e := domain.Text("assessment.review."+f.name, f.value); e != nil {
				return e
			}
		}
	}
	seen := map[string]bool{}
	for _, p := range a.Paths {
		work, workOK := t.Work(p.WorkID)
		if !workOK || domain.Text("assessment.paths.necessity", p.Necessity) != nil {
			return domain.Invalid("assessment.paths.work_id", "bind the change and its necessity to approved work")
		}
		if domain.ScopedPath(p.Path) != nil || seen[p.Path] || len(p.GoalIDs) == 0 || domain.Text("assessment.paths.basis", p.Basis) != nil {
			return domain.Invalid("assessment.paths", "requires unique changed paths, goal references and concrete reasons")
		}
		seen[p.Path] = true
		actual := false
		for _, r := range t.Repositories {
			for _, v := range r.Binding.TaskSurface {
				actual = actual || p.Path == t.QualifiedPath(r.Key, v.Path)
			}
		}
		if !actual {
			return domain.Invalid("assessment.paths.path", "must name an actually observed changed path")
		}
		if e := domain.IDs("assessment.paths.goal_ids", p.GoalIDs); e != nil {
			return e
		}
		for _, id := range p.GoalIDs {
			if _, ok := t.Goal(id); !ok || !slices.Contains(work.GoalIDs, id) {
				return domain.Invalid("assessment.paths.goal_ids", "must name goals served by the planned work")
			}
		}
	}
	for kind, js := range map[string][]domain.Judgment{"constraints": a.Constraints, "facts": a.Facts} {
		seen := map[string]bool{}
		for _, j := range js {
			exists := false
			if kind == "constraints" {
				for _, v := range t.Authorization.Constraints {
					exists = exists || v.ID == j.ID
				}
			} else {
				for _, v := range t.Facts {
					exists = exists || v.ID == j.ID
				}
			}
			if !exists || seen[j.ID] || !slices.Contains([]string{"satisfied", "violation", "uncertain"}, j.Verdict) || domain.Text("assessment."+kind+".basis", j.Basis) != nil || j.Verdict == "uncertain" && j.Uncertainty == "" {
				return domain.Invalid("assessment."+kind, "requires unique observed IDs, verdict satisfied/violation/uncertain, basis and explicit uncertainty")
			}
			seen[j.ID] = true
		}
	}
	return nil
}

type ReviseRequest struct {
	Identity
	Plan          domain.Plan          `json:"plan"`
	Authorization domain.Authorization `json:"authorization"`
	Checks        []domain.Check       `json:"checks"`
	Reason        string               `json:"reason"`
}

func (s *Service) Revise(ctx context.Context, q ReviseRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "revise", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	if e = domain.Text("reason", q.Reason); e != nil {
		return View{}, e
	}
	if e = q.Authorization.Validate(); e != nil {
		return View{}, e
	}
	previousPlan := t.PlanDigest()
	t.Authorization = q.Authorization
	t.Plan = q.Plan
	t.Checks = normalizeChecks(q.Checks)
	if t.PlanDigest() != previousPlan {
		t.Approval = nil
	}
	progress := []domain.Progress{}
	for _, p := range t.Progress {
		g, ok := t.Goal(p.GoalID)
		if ok && p.GoalDigest == domain.Hash(g) {
			progress = append(progress, p)
		}
	}
	t.Progress = progress
	t.Evidence = retainedEvidence(t)
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
	}
	if t.Approval == nil {
		t.ProposedContentDigest = t.ContentDigest()
	}
	return s.save(ctx, t, o, store.Charge{})
}

func retainedEvidence(t domain.Task) []domain.Evidence {
	result := []domain.Evidence{}
	for _, v := range t.Evidence {
		c, ok := t.Check(v.CheckID)
		if ok && (v.Source == "claim" || v.Source == "human" && c.Kind == "human" || v.Source == "core_execution" && c.Kind == "command") {
			result = append(result, v)
		}
	}
	return result
}

type RunRequest struct {
	Identity
	CheckID        string `json:"check_id"`
	Rationale      string `json:"rationale"`
	PreviousRunID  string `json:"previous_run_id,omitempty"`
	NewInformation string `json:"new_information,omitempty"`
	FullSuiteBasis string `json:"full_suite_basis,omitempty"`
}
type RunResult struct {
	View View          `json:"view"`
	Run  execution.Run `json:"run"`
}

func (s *Service) Execute(ctx context.Context, q RunRequest) (RunResult, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "run", q)
	if e != nil {
		return RunResult{}, e
	}
	if replay {
		var run execution.Run
		if e = s.Store.Run(ctx, o.RunID, &run); e != nil {
			return RunResult{}, e
		}
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return RunResult{v, run}, e
	}
	if e = requireExecutionAuthority(t); e != nil {
		return RunResult{}, e
	}
	if e = domain.Text("rationale", q.Rationale); e != nil {
		return RunResult{}, e
	}
	t, e = s.observe(ctx, t)
	if e != nil {
		return RunResult{}, e
	}
	for _, f := range t.Facts {
		if f.Blocking {
			return RunResult{}, domain.Fail("SCOPE_VIOLATION", "scope", f.Detail)
		}
	}
	pending, e := s.Store.Pending(ctx, t.TaskID)
	if e != nil {
		return RunResult{}, e
	}
	if len(pending) > 0 {
		return RunResult{}, domain.Fail("EXECUTION_PENDING", "run_id", "resolve the recorded execution before starting another")
	}
	c, ok := t.Check(q.CheckID)
	if !ok || c.Kind != "command" {
		return RunResult{}, domain.Invalid("check_id", "requires an existing command check")
	}
	if c.FullSuite && domain.Text("full_suite_basis", q.FullSuiteBasis) != nil {
		return RunResult{}, domain.Invalid("full_suite_basis", "explain the current gap that targeted checks cannot cover; remaining budget is not a reason")
	}
	if e := execution.CheckCommand(c.Argv); e != nil {
		return RunResult{}, e
	}
	u, l := t.Usage, t.Authorization.Limits
	for _, limit := range []struct {
		path          string
		used, maximum int64
	}{
		{"core_ms", u.CoreMS, l.CoreMS}, {"host_ms", u.HostMS + u.WaitMS, l.HostMS}, {"tokens", u.Tokens, l.Tokens},
	} {
		if limit.used >= limit.maximum {
			return RunResult{}, &domain.Error{Code: "COST_LIMIT", Message: "cumulative control or Host cost limit reached; no process was started", Details: []domain.ContractViolation{{Path: "authorization.limits." + limit.path, Rule: "quantity", Message: "retain acceptance requirements and obtain a real authorized limit change"}}, Budget: &domain.BudgetFailure{Used: limit.used, Requested: 0, Limit: limit.maximum}}
		}
	}
	f, e := execution.Observe(ctx, t, c)
	if e != nil {
		return RunResult{}, e
	}
	pid, identity, e := execution.Owner()
	if e != nil {
		return RunResult{}, e
	}
	r := execution.Run{RunID: "run-" + string(domain.Hash([]string{t.TaskID, q.OperationID}))[:24], TaskID: t.TaskID, CheckID: c.ID, CheckDigest: t.CheckDigest(c), RequirementDigest: t.CheckRequirementDigest(c), Check: c, Before: f, Status: "running", OwnerPID: pid, OwnerIdentity: identity, StartedAt: time.Now().UTC(), Rationale: q.Rationale, NewInformation: q.NewInformation, FullSuiteBasis: q.FullSuiteBasis}
	var previous execution.Run
	found, e := s.Store.LatestRun(ctx, t.TaskID, r.InputKey(), &previous)
	if e != nil {
		return RunResult{}, e
	}
	if slices.ContainsFunc(t.Evidence, func(ev domain.Evidence) bool {
		return ev.CheckID == c.ID && ev.Source == "core_execution" && ev.Status == "passed" && ev.CheckDigest == r.CheckDigest && ev.InputsDigest == f.Inputs && ev.EnvironmentDigest == f.Environment
	}) {
		return RunResult{}, domain.Fail("CHECK_ALREADY_SATISFIED", "check_id", "this command already passed for the same inputs and conditions; reuse its evidence")
	}
	if found && previous.Status != "passed" && (q.PreviousRunID != previous.RunID || domain.Text("new_information", q.NewInformation) != nil || q.NewInformation == previous.NewInformation) {
		return RunResult{}, domain.Fail("RETRY_BASIS_REQUIRED", "new_information", "read the previous run and explain new diagnostic information before repeating unchanged conditions")
	}
	count, e := s.Store.Failure(ctx, t.TaskID, r.InputKey())
	if e != nil {
		return RunResult{}, e
	}
	if count >= 3 {
		return RunResult{}, domain.Fail("REPEATED_FAILURE", "check_id", "three failed executions have the same check and observed inputs; change the code, conditions or verification approach before retrying")
	}
	o.RunID = r.RunID
	expected := t.Revision
	t.Revision++
	t.UpdatedAt = time.Now().UTC()
	if e = s.Store.Save(ctx, t, expected, o, store.Charge{Runs: 1, ReserveMS: c.TimeoutMS, InputKey: r.InputKey()}, r.RunID, r, r.Status, c.ID); e != nil {
		return RunResult{}, e
	}
	// Reservation is durable before any child process is started.
	reserved := r
	r = execution.Perform(ctx, t, r, func(ctx context.Context, started execution.Run) error {
		return s.Store.RecordStartedRun(ctx, r.RunID, reserved, started)
	})
	if e = execution.WriteReceipt(s.Store.Directory, r); e != nil {
		return RunResult{}, domain.Fail("EXECUTION_UNCERTAIN", "run_id", "process returned, but its recovery receipt could not be saved; inspect the reserved run")
	}
	return s.finalize(context.WithoutCancel(ctx), r, store.Operation{})
}
func (s *Service) finalize(ctx context.Context, r execution.Run, operation store.Operation) (RunResult, error) {
	for attempt := 0; attempt < 5; attempt++ {
		var saved execution.Run
		if e := s.Store.Run(ctx, r.RunID, &saved); e != nil {
			return RunResult{}, e
		}
		t, e := s.Store.Get(ctx, r.TaskID)
		if e != nil {
			return RunResult{}, e
		}
		if saved.Status != "running" && saved.Status != "uncertain" {
			if operation.ID != "" {
				v, e := s.save(ctx, t, operation, store.Charge{})
				return RunResult{v, saved}, e
			}
			v, e := s.view(ctx, t, false)
			return RunResult{v, saved}, e
		}
		t, e = s.observe(ctx, t)
		if e != nil {
			return RunResult{}, e
		}
		source := "core_execution"
		exit := "unavailable"
		if r.ExitCode != nil {
			exit = fmt.Sprint(*r.ExitCode)
		}
		summary := fmt.Sprintf("actual exit %s; %s; %d ms", exit, r.Status, r.DurationMS)
		ev := domain.Evidence{ID: r.RunID, CheckID: r.CheckID, Source: source, Status: r.Status, CheckDigest: r.CheckDigest, InputsDigest: r.Before.Inputs, EnvironmentDigest: r.Before.Environment, Summary: summary, RecordedAt: time.Now().UTC()}
		if current, ok := t.Check(r.CheckID); ok && current.Kind == "command" {
			t.Evidence = upsertEvidence(t.Evidence, ev)
		}
		t.Attempts = append(t.Attempts, domain.Attempt{RunID: r.RunID, CheckID: r.CheckID, Status: r.Status, Summary: summary, At: time.Now().UTC()})
		if len(t.Attempts) > 64 {
			t.Attempts = t.Attempts[len(t.Attempts)-64:]
		}
		if e := s.boundEvidence(ctx, &t); e != nil {
			return RunResult{}, e
		}
		expected := t.Revision
		t.Revision++
		t.UpdatedAt = time.Now().UTC()
		charge := store.Charge{ReleaseMS: r.Check.TimeoutMS, ExecutionMS: r.DurationMS, InputKey: r.InputKey()}
		if r.Status != "passed" {
			charge.FailureSignature = r.InputKey()
		}
		if r.Status == "interrupted_unknown" {
			charge.ExecutionMS = 0
			charge.UncertainMS = r.Check.TimeoutMS
		}
		if e = s.Store.Save(ctx, t, expected, operation, charge, r.RunID, r, r.Status, r.CheckID); e != nil {
			var failure *domain.Error
			if errors.As(e, &failure) && failure.Code == "REVISION_CONFLICT" {
				continue
			}
			return RunResult{}, e
		}
		if e = s.compactReceipts(ctx); e != nil {
			return RunResult{}, domain.Fail("RESOURCE_CLEANUP_FAILED", "", "execution was saved; retired receipt cleanup failed, so read the saved run")
		}
		t, e = s.Store.Get(ctx, t.TaskID)
		if e != nil {
			return RunResult{}, e
		}
		v, e := s.view(ctx, t, true)
		return RunResult{v, r}, e
	}
	return RunResult{}, domain.Fail("REVISION_CONFLICT", "run_id", "execution receipt is retained; recover it after concurrent changes settle")
}
func upsertEvidence(xs []domain.Evidence, e domain.Evidence) []domain.Evidence {
	out := []domain.Evidence{}
	for _, v := range xs {
		if v.ID != e.ID && (v.CheckID != e.CheckID || v.Source != e.Source || v.CheckDigest != e.CheckDigest || v.InputsDigest != e.InputsDigest || v.EnvironmentDigest != e.EnvironmentDigest) {
			out = append(out, v)
		}
	}
	return append(out, e)
}

type EvidenceRequest struct {
	Identity
	CheckID       string        `json:"check_id"`
	Source        string        `json:"source"`
	Status        string        `json:"status"`
	Summary       string        `json:"summary"`
	UserRef       string        `json:"user_ref,omitempty"`
	ContentDigest domain.Digest `json:"content_digest"`
}

func (s *Service) RecordEvidence(ctx context.Context, q EvidenceRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "evidence", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
	}
	c, ok := t.Check(q.CheckID)
	if !ok {
		return View{}, domain.Invalid("check_id", "unknown check")
	}
	if e = requireExecutionAuthority(t); e != nil {
		return View{}, e
	}
	if !slices.Contains([]string{"claim", "human"}, q.Source) || !slices.Contains([]string{"passed", "failed", "not_run"}, q.Status) {
		return View{}, domain.Invalid("evidence", "only claim or human evidence and passed/failed/not_run can be submitted; command execution is Core-owned")
	}
	if q.Source == "human" && (c.Kind != "human" || domain.Text("user_ref", q.UserRef) != nil) {
		return View{}, domain.Invalid("user_ref", "human evidence requires a human check and an actual user observation reference")
	}
	if e = domain.Text("summary", q.Summary); e != nil {
		return View{}, e
	}
	if q.ContentDigest != t.ContentDigest() {
		return View{}, domain.Fail("EVIDENCE_STALE", "content_digest", "report must bind the observed current contents")
	}
	f, e := execution.Observe(ctx, t, c)
	if e != nil {
		return View{}, e
	}
	ev := domain.Evidence{ID: "evidence-" + string(domain.Hash(q.OperationID))[:24], CheckID: c.ID, Source: q.Source, Status: q.Status, Summary: q.Summary, UserRef: q.UserRef, CheckDigest: t.CheckDigest(c), InputsDigest: f.Inputs, EnvironmentDigest: f.Environment, RecordedAt: time.Now().UTC()}
	t.Evidence = upsertEvidence(t.Evidence, ev)
	return s.save(ctx, t, o, store.Charge{})
}

type RecoverRequest struct {
	Identity
	RunID           string `json:"run_id"`
	ResolutionBasis string `json:"resolution_basis"`
	UserRef         string `json:"user_ref"`
}

func (s *Service) Recover(ctx context.Context, q RecoverRequest) (RunResult, error) {
	t, operation, replay, e := s.begin(ctx, q.Identity, "recover", q)
	if e != nil {
		return RunResult{}, e
	}
	if !domain.ID(q.RunID).IsValid() {
		return RunResult{}, domain.Invalid("run_id", "requires a canonical run identity")
	}
	var r execution.Run
	if e := s.Store.Run(ctx, q.RunID, &r); e != nil {
		return RunResult{}, e
	}
	if r.TaskID != q.TaskID {
		return RunResult{}, domain.Invalid("run_id", "run belongs to another task")
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return RunResult{v, r}, e
	}
	operation.RunID = r.RunID
	if r.Status != "running" && r.Status != "uncertain" {
		v, e := s.save(ctx, t, operation, store.Charge{})
		return RunResult{v, r}, e
	}
	complete, e := execution.ReadReceipt(s.Store.Directory, r)
	if e == nil {
		return s.finalize(ctx, complete, operation)
	}
	if !os.IsNotExist(e) {
		return RunResult{}, e
	}
	if execution.OwnerAlive(r) {
		return RunResult{}, domain.Fail("EXECUTION_RUNNING", "run_id", "original execution owner is still alive; wait for its result instead of replaying it")
	}
	active, e := execution.ProcessActive(ctx, r.Process)
	if e != nil {
		return RunResult{}, domain.Fail("EXECUTION_UNCERTAIN", "run_id", "could not determine whether the recorded command is still running; retain the pending execution")
	}
	if active {
		return RunResult{}, domain.Fail("EXECUTION_RUNNING", "run_id", "the original command or its process group is still running; wait for it to stop before recovering")
	}
	if domain.Text("resolution_basis", q.ResolutionBasis) != nil || r.Check.Retry != "repeatable" && domain.Text("user_ref", q.UserRef) != nil {
		return RunResult{}, domain.Fail("EXECUTION_UNCERTAIN", "resolution_basis", "owner stopped without a receipt; inspect side effects and record a real user decision for a non-repeatable check")
	}
	r.Status = "interrupted_unknown"
	r.Reconciliation = q.ResolutionBasis
	r.ResolutionUserRef = q.UserRef
	r = finishUnknown(r)
	if e = execution.WriteReceipt(s.Store.Directory, r); e != nil {
		return RunResult{}, e
	}
	return s.finalize(ctx, r, operation)
}
func finishUnknown(r execution.Run) execution.Run {
	now := time.Now().UTC()
	r.FinishedAt = &now
	r.DurationMS = 0
	return r
}

type Acceptance = domain.Acceptance
type FinishRequest struct {
	Identity
	Summary    string       `json:"summary"`
	Acceptance []Acceptance `json:"acceptance"`
	Risks      []string     `json:"risks"`
}

func (s *Service) Finish(ctx context.Context, q FinishRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "finish", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
	}
	v, e := s.view(ctx, t, true)
	if e != nil {
		return View{}, e
	}
	if !v.Decision.Ready {
		return View{}, domain.Fail("ACCEPTANCE_INCOMPLETE", "acceptance", "current scope review, completed goals, applicable passing evidence and resolved executions are all required")
	}
	if e = domain.Text("summary", q.Summary); e != nil {
		return View{}, e
	}
	if len(q.Acceptance) != len(t.Authorization.Goals) {
		return View{}, domain.Invalid("acceptance", "must cover every current goal exactly once")
	}
	seen := map[string]bool{}
	for _, a := range q.Acceptance {
		g, ok := t.Goal(a.GoalID)
		if !ok || seen[a.GoalID] || domain.Text("acceptance.basis", a.Basis) != nil {
			return View{}, domain.Invalid("acceptance", "requires unique current goals and concrete acceptance conclusions")
		}
		seen[a.GoalID] = true
		covered := map[string]bool{}
		for _, id := range a.EvidenceIDs {
			found := false
			for _, f := range v.Decision.Evidence {
				accepted := false
				for _, x := range v.Decision.AcceptedFailures {
					accepted = accepted || x.FailedEvidenceID == id
				}
				if f.EvidenceID == id && (f.Valid || accepted) {
					for _, ev := range t.Evidence {
						if ev.ID == id && slices.Contains(g.Checks, ev.CheckID) {
							covered[ev.CheckID] = true
							found = true
						}
					}
				}
			}
			if !found {
				return View{}, domain.Invalid("acceptance.evidence_ids", "must cite applicable passing evidence for this goal")
			}
		}
		for _, id := range g.Checks {
			if !covered[id] {
				return View{}, domain.Invalid("acceptance.evidence_ids", "each required check needs applicable passing evidence")
			}
		}
	}
	t.Status = "done"
	t.Acceptance = q.Acceptance
	t.Summary = q.Summary
	t.Risks = q.Risks
	return s.save(ctx, t, o, store.Charge{})
}

type CancelRequest struct {
	Identity
	Reason string `json:"reason"`
}

func (s *Service) Cancel(ctx context.Context, q CancelRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "cancel", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	if domain.Text("reason", q.Reason) != nil {
		return View{}, domain.Invalid("reason", "cancellation requires a concrete reason")
	}
	pending, e := s.Store.Pending(ctx, t.TaskID)
	if e != nil {
		return View{}, e
	}
	if len(pending) > 0 {
		return View{}, domain.Fail("EXECUTION_PENDING", "run_id", "resolve the execution before releasing this workspace")
	}
	t.Status = "cancelled"
	t.Summary = q.Reason
	return s.save(ctx, t, o, store.Charge{})
}

// PreWrite gates supported structured paths. It cannot inspect future semantics or shell writes.
func (s *Service) PreWrite(ctx context.Context, root string, paths []string, complete bool) (map[string]any, error) {
	canonical, instance, e := s.Observer.IdentifyWorkspace(ctx, root)
	if e != nil {
		if errors.Is(e, repository.ErrNotGitRepository) {
			return map[string]any{"decision": "allow", "coverage": "no_active_task"}, nil
		}
		return nil, e
	}
	id, e := s.Store.Claim(ctx, canonical)
	if e != nil {
		return nil, e
	}
	if id == "" {
		return map[string]any{"decision": "allow", "coverage": "no_active_task"}, nil
	}
	t, e := s.Store.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	deny := func(reason string) map[string]any {
		return map[string]any{"decision": "deny", "reason": reason, "task_id": id, "task_revision": t.Revision, "coverage": "structured_paths"}
	}
	for _, r := range t.Repositories {
		if r.Origin.CanonicalWorktreeRoot == canonical && r.Binding.WorktreeInstanceDigest != instance {
			return deny("Restore the original bound workspace instance before writing."), nil
		}
	}
	if e := requireExecutionAuthority(t); e != nil {
		return deny(e.Error()), nil
	}
	observed, e := s.observe(ctx, t)
	if e != nil {
		return deny("The bound workspace could not be observed before writing."), nil
	}
	for _, fact := range observed.Facts {
		if fact.Kind == "workspace_conflict" {
			return deny(fact.Detail), nil
		}
	}
	if !complete || len(paths) == 0 {
		return deny("Target paths were not fully parsed; inspect the tool operation before writing."), nil
	}
	for _, p := range paths {
		relative := ""
		for _, r := range t.Repositories {
			if r.Origin.CanonicalWorktreeRoot == canonical {
				relative = strings.TrimPrefix(p, canonical+string(os.PathSeparator))
				relative = strings.ReplaceAll(relative, "\\", "/")
				relative = t.QualifiedPath(r.Key, relative)
			}
		}
		if domain.ScopedPath(relative) != nil {
			return deny("Write target is outside the bound workspace."), nil
		}
		if !t.Authorization.AllowsPath(relative) {
			return deny("This path is outside the approved write scope."), nil
		}
		for _, pin := range t.Authorization.Pins {
			if pin.Path == relative {
				return deny("This content is pinned by the approved plan."), nil
			}
		}
	}
	return map[string]any{"decision": "allow", "task_id": id, "coverage": "structured_paths", "semantic_review": "required_at_checkpoint"}, nil
}
