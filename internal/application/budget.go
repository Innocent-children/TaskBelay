package application

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type BudgetRequest struct {
	Identity
	AuthorizationDigest domain.Digest         `json:"authorization_digest"`
	Budget              domain.BudgetDecision `json:"budget"`
	UserRef             string                `json:"user_ref"`
	Response            string                `json:"response"`
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
	if q.AuthorizationDigest != t.Authorization.Digest() {
		return View{}, domain.Fail("AUTHORIZATION_CHANGED", "authorization_digest", "read the current task before recording its budget decision")
	}
	if e = domain.Text("user_ref", q.UserRef); e != nil {
		return View{}, e
	}
	if e = domain.Text("response", q.Response); e != nil {
		return View{}, e
	}
	previous := t.Authorization.Digest()
	a := t.Authorization
	a.Limits = q.Budget.Limits()
	if e = a.Validate(); e != nil {
		return View{}, e
	}
	if a.Limits == t.Authorization.Limits {
		return View{}, domain.Fail("BUDGET_UNCHANGED", "budget", "reuse the existing budget decision")
	}
	t.Authorization = a
	t.RecordChange("budget", q.Response, q.UserRef)
	t.BudgetDecision = &domain.BudgetApproval{DecisionDigest: domain.Hash([]any{previous, q.Budget}), UserRef: q.UserRef, Response: q.Response, ConfirmedAt: time.Now().UTC()}
	return s.save(ctx, t, o, store.Charge{})
}
