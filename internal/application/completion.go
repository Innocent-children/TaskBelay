package application

import (
	"context"
	"slices"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type Acceptance = domain.Acceptance

type FinishRequest struct {
	Identity
	Summary    string       `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Acceptance []Acceptance `json:"acceptance" description:"Per-goal conclusions citing every applicable passed acceptance check; regression exceptions cannot replace them."`
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
		return View{}, &domain.Error{Code: "ACCEPTANCE_INCOMPLETE", Message: "resolve the listed current completion gaps", Gaps: v.Decision.Gaps, Current: currentState(t)}
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
		required := t.AcceptanceChecks(g.ID)
		covered := map[string]bool{}
		for _, id := range a.EvidenceIDs {
			found := false
			for _, f := range v.Decision.Evidence {
				if f.EvidenceID != id || !f.Valid {
					continue
				}
				for _, ev := range t.Evidence {
					if ev.ID == id && ev.Status == "passed" && ev.Source != "claim" && slices.ContainsFunc(required, func(c domain.Check) bool { return c.ID == ev.CheckID }) {
						covered[ev.CheckID], found = true, true
					}
				}
			}
			if !found {
				return View{}, domain.Invalid("acceptance.evidence_ids", "cite applicable passing acceptance checks for this goal; regression exceptions are not acceptance")
			}
		}
		for _, c := range required {
			if !covered[c.ID] {
				return View{}, domain.Invalid("acceptance.evidence_ids", "every acceptance check needs an applicable passing result")
			}
		}
	}
	t.Exceptions = v.Decision.AcceptedFailures
	t.Limitations = t.DeliveryLimitations(v.Decision.Evidence)
	t.Status = "done"
	t.Acceptance = q.Acceptance
	t.Summary = q.Summary
	return s.save(ctx, t, o, store.Charge{})
}

type CancelRequest struct {
	Identity
	Reason string `json:"reason" description:"操作/处理的实际原因，不能替代所需用户决定。"`
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

type ArchiveRequest struct {
	Identity
	Archived bool `json:"archived" description:"终态记录是否从默认列表隐藏；不删除、不重开Task。"`
}

func (s *Service) Archive(ctx context.Context, q ArchiveRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "archive", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	if t.Status == "active" {
		return View{}, domain.Fail("ARCHIVE_NOT_ALLOWED", "task_id", "finish or cancel the task before archiving")
	}
	t.Archived = q.Archived
	return s.save(ctx, t, o, store.Charge{})
}
