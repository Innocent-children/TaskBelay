package application

import (
	"context"
	"slices"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type RepositoryInput struct {
	Key             domain.RepositoryKey    `json:"key"`
	Path            string                  `json:"repository_path"`
	Origin          *OriginInput            `json:"workspace_origin,omitempty"`
	RemoteFreshness *domain.RemoteFreshness `json:"remote_freshness,omitempty"`
}

type OpenRequest struct {
	OperationID   string               `json:"operation_id" description:"Stable unique identity for this logical operation. Keep it after response loss; read saved state before retrying."`
	Host          domain.Host          `json:"host"`
	Repositories  []RepositoryInput    `json:"repositories"`
	Authorization domain.Authorization `json:"authorization"`
	Plan          domain.Plan          `json:"plan"`
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
	if e := q.Authorization.Validate(); e != nil {
		return View{}, e
	}
	now := time.Now().UTC()
	t := domain.Task{TaskID: id, Host: q.Host, Status: "active", Revision: 1, Authorization: q.Authorization, Plan: normalizePlan(q.Plan), Repositories: []domain.RepositoryScopeEntry{}, Progress: []domain.Progress{}, Evidence: []domain.Evidence{}, Attempts: []domain.Attempt{}, Facts: []domain.Fact{}, Risks: []string{}, CreatedAt: now, UpdatedAt: now}
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
		f, e := s.Observer.VerifyRemoteFreshness(ctx, origin.CanonicalWorktreeRoot, b, origin, input.RemoteFreshness)
		if e != nil {
			return View{}, e
		}
		t.Repositories = append(t.Repositories, domain.RepositoryScopeEntry{Key: input.Key, Origin: origin, Binding: b, RemoteBaseline: &f, RemoteLatest: &f})
	}
	t.RecordChange("authorization", q.Authorization.Request, q.Authorization.UserRef)
	t.Changes[0].Revision = t.Revision
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

func normalizePlan(p domain.Plan) domain.Plan {
	p.Checks = normalizeChecks(p.Checks)
	return p
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
