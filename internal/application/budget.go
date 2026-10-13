package application

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
	"time"
)

// UpdateBudget applies the complete approved decision once; it never infers approval from progress.
type BudgetRequest struct {
	Identity
	PlanDigest domain.Digest         `json:"plan_digest"`
	Budget     domain.BudgetDecision `json:"budget"`
	UserRef    string                `json:"user_ref"`
	Response   string                `json:"response"`
}

func (s *Service) UpdateBudget(ctx context.Context, q BudgetRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "update_budget", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		return v, e
	}
	if e = requireExecutionAuthority(t); e != nil {
		return View{}, e
	}
	if q.PlanDigest != t.PlanDigest() {
		return View{}, domain.Fail("PLAN_CHANGED", "plan_digest", "read the current proposal before recording its budget decision")
	}
	if e = domain.Text("user_ref", q.UserRef); e != nil {
		return View{}, e
	}
	if e = domain.Text("response", q.Response); e != nil {
		return View{}, e
	}
	previous := t.PlanDigest()
	a := t.Authorization
	a.Limits = q.Budget.Limits()
	if e = a.Validate(); e != nil {
		return View{}, e
	}
	if a.Limits == t.Authorization.Limits {
		return View{}, domain.Fail("BUDGET_UNCHANGED", "budget", "reuse the existing budget decision")
	}
	t.Authorization = a
	// The original plan decision remains attributable; only the complete budget ceiling changes.
	approval := *t.Approval
	approval.Limits = a.Limits
	approval.PlanDigest = t.PlanDigest()
	approval.BudgetDecision = &domain.BudgetApproval{DecisionDigest: domain.Hash([]any{previous, q.Budget}), UserRef: q.UserRef, Response: q.Response, ConfirmedAt: time.Now().UTC()}
	t.Approval = &approval
	return s.save(ctx, t, o, store.Charge{})
}
