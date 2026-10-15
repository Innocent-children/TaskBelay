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
	CheckID       string        `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Source        string        `json:"source" description:"结果来源claim/human/core_execution；外部终端文本不能声明core_execution。"`
	Status        string        `json:"status" description:"Submitted observation status: passed, failed or not_run; claims cannot satisfy a check."`
	Summary       string        `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	UserRef       string        `json:"user_ref,omitempty" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	ContentDigest domain.Digest `json:"content_digest" description:"当前内容摘要。"`
}

func (s *Service) RecordEvidence(ctx context.Context, q EvidenceRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "evidence", q)
	if e != nil {
		return View{}, e
	}
	if replay {
		v, e := s.view(ctx, t, false)
		v.Replayed = true
		for _, record := range t.Evidence {
			if record.ID == "evidence-"+string(domain.Hash(q.OperationID))[:24] {
				value := record
				v.RecordedEvidence = &value
			}
		}
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
	v, e := s.save(ctx, t, o, store.Charge{})
	if e == nil {
		v.RecordedEvidence = &ev
	}
	return v, e
}

type AcceptFailureRequest struct {
	Identity
	CheckID              string   `json:"check_id" description:"当前已保存检查的稳定身份。"`
	FailedEvidenceID     string   `json:"failed_evidence_id" description:"Current actual failed regression result."`
	ComparisonEvidenceID string   `json:"comparison_evidence_id" description:"对应compares诊断检查的当前实际通过记录。"`
	Failures             []string `json:"failures" description:"用户接受的具体已有失败名称，不能笼统写所有失败。"`
	UserRef              string   `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Reason               string   `json:"reason" description:"操作/处理的实际原因，不能替代所需用户决定。"`
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
