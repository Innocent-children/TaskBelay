package application

import (
	"context"
	"slices"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func selection(o domain.WorkspaceOrigin) repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: o.Mode, SourceType: o.SourceType, CarryChanges: o.CarryChanges, RemoteName: o.RemoteName, BaseBranch: o.BaseBranch, BaseCommit: o.BaseCommit, TaskBranch: o.TaskBranch, ProvisioningReceiptID: o.ProvisioningReceiptID}
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
	cost, e := s.SavedControlCost(ctx)
	if e != nil {
		return View{}, e
	}
	decision := domain.Decide(t, fresh, pending)
	issueDigest := domain.Digest("")
	if t.Issue != nil {
		issueDigest = domain.Hash(t.Issue)
	}
	return View{IssueDigest: issueDigest, Budget: t.Budget(decision.MissingChecks), Task: t, PlanDigest: t.PlanDigest(), AuthorizationDigest: t.Authorization.Digest(), ContentDigest: t.ContentDigest(), Decision: decision, Refreshed: refreshed, ControlCost: cost}, nil
}
