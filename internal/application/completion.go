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

type ArchiveRequest struct {
	Identity
	Archived bool `json:"archived"`
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
