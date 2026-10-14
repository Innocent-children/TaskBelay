package application

import (
	"context"
	"os"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
)

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
