package application

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type BudgetRequest struct {
	Identity
	AuthorizationDigest domain.Digest         `json:"authorization_digest" description:"当前授权摘要。"`
	Budget              domain.BudgetDecision `json:"budget" description:"Current reservation feasibility and explicit limits; timing categories overlap."`
	UserRef             string                `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Response            string                `json:"response" description:"场景的完整返回值。"`
}

func (s *Service) UpdateBudget(ctx context.Context, q BudgetRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "update_budget", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		v.ChangedLimits = &t.Authorization.Limits
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
	v, e := s.save(ctx, t, o, store.Charge{})
	if e == nil {
		v.ChangedLimits = &v.Task.Authorization.Limits
	}
	return v, e
}
