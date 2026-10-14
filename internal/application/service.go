package application

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type Service struct {
	Store    *store.Store
	Observer *repository.GitObserver
}

func NewService(s *store.Store, o *repository.GitObserver) (*Service, error) {
	if s == nil || o == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &Service{s, o}, nil
}

type Identity struct {
	TaskID      string `json:"task_id"`
	OperationID string `json:"operation_id" description:"Stable unique identity for this logical operation. Keep it after response loss; read saved state before retrying."`
	Revision    uint64 `json:"revision" description:"Current revision from the last task response."`
}

type View struct {
	Task                domain.Task         `json:"task"`
	PlanDigest          domain.Digest       `json:"plan_digest"`
	AuthorizationDigest domain.Digest       `json:"authorization_digest"`
	ContentDigest       domain.Digest       `json:"content_digest"`
	Decision            domain.Decision     `json:"decision"`
	IssueDigest         domain.Digest       `json:"issue_digest,omitempty"`
	Refreshed           bool                `json:"refreshed"`
	Replayed            bool                `json:"replayed"`
	ControlCost         store.ControlUsage  `json:"control_cost"`
	Budget              domain.BudgetReport `json:"budget"`
}

func (s *Service) begin(ctx context.Context, q Identity, kind string, payload any) (domain.Task, store.Operation, bool, error) {
	if !domain.ID(q.TaskID).IsValid() || !domain.ID(q.OperationID).IsValid() || q.Revision == 0 {
		return domain.Task{}, store.Operation{}, false, domain.Invalid("identity", "requires task_id, operation_id and current revision")
	}
	o := store.Operation{ID: q.OperationID, Kind: kind, Digest: domain.Hash(payload)}
	old, ok, e := s.Store.Operation(ctx, q.TaskID, q.OperationID)
	if e != nil {
		return domain.Task{}, o, false, e
	}
	t, e := s.Store.Get(ctx, q.TaskID)
	if e != nil {
		return t, o, false, e
	}
	if ok {
		if old.Kind != kind || old.Digest != o.Digest {
			return t, o, false, domain.Fail("OPERATION_CONFLICT", "operation_id", "saved identity belongs to a different operation")
		}
		return t, old, true, nil
	}
	if t.Revision != q.Revision {
		return t, o, false, domain.Fail("REVISION_CONFLICT", "revision", "read the latest task and submit a new operation identity")
	}
	if t.Status != "active" && kind != "archive" {
		return t, o, false, domain.Fail("TASK_TERMINAL", "task_id", "terminal tasks cannot be changed")
	}
	return t, o, false, nil
}

func (s *Service) save(ctx context.Context, t domain.Task, o store.Operation, c store.Charge) (View, error) {
	if e := s.boundEvidence(ctx, &t); e != nil {
		return View{}, e
	}
	expected := t.Revision
	t.Revision++
	t.UpdatedAt = time.Now().UTC()
	if e := s.Store.Save(ctx, t, expected, o, c, "", nil, "", ""); e != nil {
		return View{}, e
	}
	if e := s.compactReceipts(ctx); e != nil {
		return View{}, domain.Fail("RESOURCE_CLEANUP_FAILED", "", "task was saved; retired receipt cleanup failed, so read saved state before any retry")
	}
	t, e := s.Store.Get(ctx, t.TaskID)
	if e != nil {
		return View{}, e
	}
	return s.view(ctx, t, true)
}

func requireExecutionAuthority(t domain.Task) error {
	if t.Issue != nil && t.Issue.Kind == "authorization" {
		return domain.Fail("AUTHORIZATION_PENDING", "issue", "resolve the actual user authorization question before further writes or executions")
	}
	return nil
}
