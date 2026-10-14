package application

import (
	"context"
	"slices"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func upsertEvidence(xs []domain.Evidence, e domain.Evidence) []domain.Evidence {
	out := []domain.Evidence{}
	for _, v := range xs {
		if v.ID != e.ID && (v.CheckID != e.CheckID || v.Source != e.Source || v.CheckDigest != e.CheckDigest || v.InputsDigest != e.InputsDigest || v.EnvironmentDigest != e.EnvironmentDigest) {
			out = append(out, v)
		}
	}
	return append(out, e)
}

type EvidenceRequest struct {
	Identity
	CheckID       string        `json:"check_id"`
	Source        string        `json:"source"`
	Status        string        `json:"status"`
	Summary       string        `json:"summary"`
	UserRef       string        `json:"user_ref,omitempty"`
	ContentDigest domain.Digest `json:"content_digest"`
}

func (s *Service) RecordEvidence(ctx context.Context, q EvidenceRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "evidence", q)
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
	c, ok := t.Check(q.CheckID)
	if !ok {
		return View{}, domain.Invalid("check_id", "unknown check")
	}
	if e = requireExecutionAuthority(t); e != nil {
		return View{}, e
	}
	if !slices.Contains([]string{"claim", "human"}, q.Source) || !slices.Contains([]string{"passed", "failed", "not_run"}, q.Status) {
		return View{}, domain.Invalid("evidence", "only claim or human evidence and passed/failed/not_run can be submitted; command execution is Core-owned")
	}
	if q.Source == "human" && (c.Kind != "human" || domain.Text("user_ref", q.UserRef) != nil) {
		return View{}, domain.Invalid("user_ref", "human evidence requires a human check and an actual user observation reference")
	}
	if e = domain.Text("summary", q.Summary); e != nil {
		return View{}, e
	}
	if q.ContentDigest != t.ContentDigest() {
		return View{}, domain.Fail("EVIDENCE_STALE", "content_digest", "report must bind the observed current contents")
	}
	f, e := execution.Observe(ctx, t, c)
	if e != nil {
		return View{}, e
	}
	ev := domain.Evidence{ID: "evidence-" + string(domain.Hash(q.OperationID))[:24], CheckID: c.ID, Source: q.Source, Status: q.Status, Summary: q.Summary, UserRef: q.UserRef, CheckDigest: t.CheckDigest(c), InputsDigest: f.Inputs, EnvironmentDigest: f.Environment, RecordedAt: time.Now().UTC()}
	t.Evidence = upsertEvidence(t.Evidence, ev)
	return s.save(ctx, t, o, store.Charge{})
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
	t.RecordChange("accepted_failure", q.Reason, q.UserRef)
	return s.save(ctx, t, o, store.Charge{})
}
