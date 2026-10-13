package application

import (
	"context"
	"slices"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func requireExecutionAuthority(t domain.Task) error {
	if t.Issue != nil && t.Issue.Kind == "authorization" {
		return domain.Fail("AUTHORIZATION_PENDING", "issue", "resolve the actual user authorization question before further writes or executions")
	}
	return nil
}

type AcceptFailureRequest struct {
	Identity
	CheckID              string   `json:"check_id"`
	FailedEvidenceID     string   `json:"failed_evidence_id"`
	ComparisonEvidenceID string   `json:"comparison_evidence_id"`
	Failures             []string `json:"failures"`
	UserRef              string   `json:"user_ref"`
	Reason               string   `json:"reason"`
}

func (s *Service) AcceptFailure(ctx context.Context, q AcceptFailureRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "accept_failure", q)
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
	if e = domain.Text("user_ref", q.UserRef); e != nil {
		return View{}, e
	}
	if e = domain.Text("reason", q.Reason); e != nil {
		return View{}, e
	}
	if len(q.Failures) == 0 || len(q.Failures) > 64 {
		return View{}, domain.Invalid("failures", "name 1..64 explicitly accepted existing failures")
	}
	for _, f := range q.Failures {
		if e = domain.Text("failures", f); e != nil {
			return View{}, e
		}
	}
	t, e = s.observe(ctx, t)
	if e != nil {
		return View{}, e
	}
	v, e := s.view(ctx, t, true)
	if e != nil {
		return View{}, e
	}
	x := domain.KnownFailure{CheckID: q.CheckID, FailedEvidenceID: q.FailedEvidenceID, ComparisonEvidenceID: q.ComparisonEvidenceID, Failures: q.Failures, UserRef: q.UserRef, Reason: q.Reason, AcceptedAt: time.Now().UTC()}
	t.Exceptions = []domain.KnownFailure{x}
	if len(t.AcceptedFailures(v.Decision.Evidence)) != 1 {
		return View{}, domain.Fail("FAILURE_ACCEPTANCE_NOT_ALLOWED", "failed_evidence_id", "requires a current failed regression execution and a current passed comparison; acceptance checks cannot be waived")
	}
	t.Exceptions = slices.DeleteFunc(v.Task.Exceptions, func(old domain.KnownFailure) bool { return old.CheckID == x.CheckID })
	t.Exceptions = append(t.Exceptions, x)
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
