package application

import (
	"context"
	"errors"
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
	TaskID      string `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	OperationID string `json:"operation_id" description:"Stable unique identity for this logical operation. Keep it after response loss; read saved state before retrying."`
	Revision    uint64 `json:"revision" description:"Current revision from the last task response."`
}

type View struct {
	Task                domain.Task         `json:"task" description:"Internal saved Task; consumers use explicit public projections."`
	PlanDigest          domain.Digest       `json:"plan_digest" description:"Core-generated current plan/authorization digest; read-only."`
	AuthorizationDigest domain.Digest       `json:"authorization_digest" description:"当前授权摘要。"`
	ContentDigest       domain.Digest       `json:"content_digest" description:"当前内容摘要。"`
	Decision            domain.Decision     `json:"decision" description:"当前事实推导的控制结果，不是另一份持久化工作流。"`
	IssueDigest         domain.Digest       `json:"issue_digest,omitempty" description:"Current issue digest, copied when recording the real user answer."`
	Refreshed           bool                `json:"refreshed" description:"是否刚核对实际条件；false只返回保存内容。"`
	Replayed            bool                `json:"replayed" description:"是否回读既有操作身份的结果；true没有再次执行副作用。"`
	ControlCost         store.ControlUsage  `json:"control_cost" description:"当前数据目录跨Task的Core计量，不能与单Task重复相加。"`
	Budget              domain.BudgetReport `json:"budget" description:"Current reservation feasibility and explicit limits; timing categories overlap."`
	RecordedEvidence    *domain.Evidence    `json:"-"`
	ChangedLimits       *domain.Limits      `json:"-"`
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
		return t, o, false, &domain.Error{Code: "REVISION_CONFLICT", Message: "read the latest task and submit a new operation identity", Details: []domain.ContractViolation{{Path: "revision", Rule: "requirement", Message: "request revision differs from the saved task"}}, Current: currentState(t)}
	}
	if t.Status != "active" && kind != "archive" {
		return t, o, false, domain.Fail("TASK_TERMINAL", "task_id", "terminal tasks cannot be changed")
	}
	return t, o, false, nil
}

func currentState(t domain.Task) *domain.CurrentState {
	paths := []string{}
	for _, r := range t.Repositories {
		for _, p := range r.Binding.TaskSurface {
			paths = append(paths, t.QualifiedPath(r.Key, p.Path))
		}
	}
	return &domain.CurrentState{TaskID: t.TaskID, Revision: t.Revision, AuthorizationDigest: t.Authorization.Digest(), ContentDigest: t.ContentDigest(), ChangedPaths: paths}
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
		return View{}, &domain.Error{Code: "RESOURCE_CLEANUP_FAILED", Message: "task was saved; retired receipt cleanup failed, so read saved state before any retry", Outcome: "applied", Current: currentState(t)}
	}
	saved, e := s.Store.Get(ctx, t.TaskID)
	if e != nil {
		return View{}, appliedReadFailure(t, e)
	}
	v, e := s.view(ctx, saved, true)
	if e != nil {
		return View{}, appliedReadFailure(saved, e)
	}
	return v, nil
}

func requireExecutionAuthority(t domain.Task) error {
	if t.Issue != nil && t.Issue.Kind == "authorization" {
		return domain.Fail("AUTHORIZATION_PENDING", "issue", "resolve the actual user authorization question before further writes or executions")
	}
	return nil
}

func appliedReadFailure(t domain.Task, err error) error {
	var original *domain.Error
	if errors.As(err, &original) {
		copy := *original
		copy.Outcome = "applied"
		copy.Current = currentState(t)
		return &copy
	}
	return &domain.Error{Code: "POST_COMMIT_READ_FAILED", Message: "operation was saved but its state/view could not be read; inspect the original Task before another operation", Outcome: "applied", Current: currentState(t)}
}
