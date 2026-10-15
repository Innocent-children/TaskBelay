package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type RunRequest struct {
	Identity
	CheckID        string `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Rationale      string `json:"rationale" description:"这项/这次检查如何覆盖具体目标或补充当前缺口。"`
	PreviousRunID  string `json:"previous_run_id,omitempty" description:"Original failed run under unchanged conditions; read it before another execution."`
	NewInformation string `json:"new_information,omitempty" description:"New actual diagnostic information for an unchanged failed-check retry."`
	FullSuiteBasis string `json:"full_suite_basis,omitempty" description:"Required for a full suite: the actual gap targeted checks cannot cover."`
}

type RunResult struct {
	View View          `json:"view" description:"原生执行/恢复携带的最新TaskState，不是旧请求快照。"`
	Run  execution.Run `json:"run" description:"本次实际或恢复运行的摘要，必须检查status。"`
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
			return RunResult{}, domain.Fail("SCOPE_VIOLATION", f.Path, f.Detail)
		}
	}
	pending, e := s.Store.Pending(ctx, t.TaskID)
	if e != nil {
		return RunResult{}, e
	}
	if len(pending) > 0 {
		return RunResult{}, &domain.Error{Code: "EXECUTION_PENDING", Message: "resolve the recorded execution before starting another", ExistingResult: &domain.ResultReference{RunID: pending[0]}, Outcome: "not_applied", Execution: "not_started"}
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
	f, e := execution.Observe(ctx, t, c)
	if e != nil {
		return RunResult{}, e
	}
	pid, identity, e := execution.Owner()
	if e != nil {
		return RunResult{}, e
	}
	r := execution.Run{RunID: "run-" + string(domain.Hash([]string{t.TaskID, q.OperationID}))[:24], TaskID: t.TaskID, OperationID: q.OperationID, CheckID: c.ID, CheckDigest: t.CheckDigest(c), RequirementDigest: t.CheckRequirementDigest(c), Check: c, Before: f, Status: "running", OwnerPID: pid, OwnerIdentity: identity, StartedAt: time.Now().UTC(), Rationale: q.Rationale, NewInformation: q.NewInformation, FullSuiteBasis: q.FullSuiteBasis}
	var previous execution.Run
	found, e := s.Store.LatestRun(ctx, t.TaskID, r.InputKey(), &previous)
	if e != nil {
		return RunResult{}, e
	}
	for _, ev := range t.Evidence {
		if ev.CheckID == c.ID && ev.Source == "core_execution" && ev.Status == "passed" && ev.CheckDigest == r.CheckDigest && ev.InputsDigest == f.Inputs && ev.EnvironmentDigest == f.Environment {
			return RunResult{}, &domain.Error{Code: "CHECK_ALREADY_SATISFIED", Message: "reuse the existing passing result for these inputs and conditions", ExistingResult: &domain.ResultReference{RunID: ev.ID, EvidenceID: ev.ID, CheckID: c.ID}, Execution: "not_started"}
		}
	}
	u, l := t.Usage, t.Authorization.Limits
	for _, limit := range []struct {
		path          string
		used, maximum int64
	}{
		{"core_ms", u.CoreMS, l.CoreMS}, {"host_ms", u.HostMS + u.WaitMS, l.HostMS}, {"tokens", u.Tokens, l.Tokens},
	} {
		if domain.LimitReached(limit.used, limit.maximum) {
			return RunResult{}, &domain.Error{Code: "COST_LIMIT", Message: "cumulative control or Host cost limit reached; no process was started", Details: []domain.ContractViolation{{Path: "authorization.limits." + limit.path, Rule: "quantity", Message: "retain acceptance requirements and obtain a real authorized limit change"}}, Budget: domain.NewBudgetLine(limit.path, limit.used, 0, 0, limit.maximum).Failure(0)}
		}
	}
	if metric := t.Usage.UnavailableHostLimit(t.Authorization.Limits); metric != "" {
		return RunResult{}, &domain.Error{Code: "HOST_USAGE_UNAVAILABLE", Message: "an explicit Host limit requires complete reported usage; no command was started", Details: []domain.ContractViolation{{Path: "usage.host." + metric, Rule: "requirement", Message: "provide actual usage or obtain a real budget change; do not estimate missing usage"}}, Execution: "not_started"}
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
		return RunResult{}, &domain.Error{Code: "EXECUTION_UNCERTAIN", Message: "process returned, but its recovery receipt could not be saved; inspect the reserved run", ExistingResult: &domain.ResultReference{RunID: r.RunID, CheckID: r.CheckID}, Outcome: "applied", Execution: "unknown"}
	}
	return s.finalize(context.WithoutCancel(ctx), r, store.Operation{})
}

func (s *Service) finalize(ctx context.Context, r execution.Run, operation store.Operation) (result RunResult, err error) {
	// Execute already saved its operation before starting the process. Recovery
	// may still need to commit a separate operation, but always retains this run.
	applied := operation.ID == ""
	defer func() {
		if err == nil {
			return
		}
		var cause *domain.Error
		if !errors.As(err, &cause) {
			cause = &domain.Error{Code: "EXECUTION_UNCERTAIN", Message: "the original execution record or receipt is retained; read and recover that run before another command"}
		}
		copy := *cause
		if applied {
			copy.Outcome = "applied"
		} else if copy.Outcome == "" {
			copy.Outcome = "unknown"
		}
		copy.ExistingResult = &domain.ResultReference{RunID: r.RunID, CheckID: r.CheckID}
		if copy.Execution == "" {
			copy.Execution = "unknown"
			if r.ExitCode != nil {
				copy.Execution = "started"
			}
		}
		err = &copy
	}()
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
		applied = true
		if e = s.compactReceipts(ctx); e != nil {
			return RunResult{}, &domain.Error{Code: "RESOURCE_CLEANUP_FAILED", Message: "execution was saved; retired receipt cleanup failed, so read the saved run", Outcome: "applied", Execution: "started", ExistingResult: &domain.ResultReference{RunID: r.RunID, EvidenceID: r.RunID, CheckID: r.CheckID}, Current: currentState(t)}
		}
		t, e = s.Store.Get(ctx, t.TaskID)
		if e != nil {
			return RunResult{}, e
		}
		v, e := s.view(ctx, t, true)
		return RunResult{v, r}, e
	}
	return RunResult{}, &domain.Error{Code: "REVISION_CONFLICT", Message: "execution receipt is retained; recover it after concurrent changes settle", Outcome: "unknown", Execution: "started", ExistingResult: &domain.ResultReference{RunID: r.RunID, CheckID: r.CheckID}}
}
