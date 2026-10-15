package application

import (
	"context"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type ReviseRequest struct {
	Identity
	Plan          *domain.Plan          `json:"plan,omitempty" description:"Replace the complete implementation plan, including checks, forecasts and review hints; omission retains it."`
	Authorization *domain.Authorization `json:"authorization,omitempty" description:"Replace the user request and boundaries only when needed. Changed outcomes/restrictions or increased caps need an actual new user_ref."`
	Reason        string                `json:"reason" description:"操作/处理的实际原因，不能替代所需用户决定。"`
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
	if q.Authorization == nil && q.Plan == nil {
		return View{}, domain.Invalid("revision", "provide authorization or plan to replace")
	}
	if q.Authorization != nil {
		if e = q.Authorization.Validate(); e != nil {
			return View{}, e
		}
		if q.Authorization.RequiresDecision(t.Authorization) && q.Authorization.UserRef == t.Authorization.UserRef {
			return View{}, domain.Fail("AUTHORIZATION_REQUIRED", "authorization.user_ref", "changed outcomes, explicit restrictions or increased limits require the actual new user decision; implementation and check changes can reuse the current request")
		}
		t.Authorization = *q.Authorization
	}
	if q.Plan != nil {
		t.Plan = normalizePlan(*q.Plan)
	}
	kind, userRef := "plan", ""
	if q.Authorization != nil {
		kind, userRef = "authorization", q.Authorization.UserRef
	}
	t.RecordChange(kind, q.Reason, userRef)
	progress := []domain.Progress{}
	for _, p := range t.Progress {
		g, ok := t.Goal(p.GoalID)
		if ok && p.GoalDigest == g.RequirementDigest() {
			progress = append(progress, p)
		}
	}
	t.Progress = progress
	t.Evidence = retainedEvidence(t)
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
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
