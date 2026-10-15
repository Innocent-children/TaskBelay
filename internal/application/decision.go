package application

import (
	"context"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type ResolveIssueRequest struct {
	Identity
	IssueDigest domain.Digest `json:"issue_digest" description:"Current issue digest, copied when recording the real user answer."`
	UserRef     string        `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Decision    string        `json:"decision" description:"Actual user answer to the current issue. This clears that issue but does not revise authorization."`
}

/**
 * Record an answer to the exact displayed question. The answer does not itself
 * change user restrictions; any changed authorization must still pass Revise.
 */
func (s *Service) ResolveIssue(ctx context.Context, q ResolveIssueRequest) (View, error) {
	t, o, replay, err := s.begin(ctx, q.Identity, "resolve_issue", q)
	if err != nil {
		return View{}, err
	}
	if replay {
		v, err := s.view(ctx, t, false)
		v.Replayed = true
		return v, err
	}
	if t.Issue == nil || domain.Hash(t.Issue) != q.IssueDigest {
		return View{}, domain.Fail("ISSUE_CHANGED", "issue_digest", "read the current question before recording a user decision")
	}
	if err = domain.Text("user_ref", q.UserRef); err != nil {
		return View{}, err
	}
	if err = domain.Text("decision", q.Decision); err != nil {
		return View{}, err
	}
	t.RecordChange("user_decision", t.Issue.Summary+": "+q.Decision, q.UserRef)
	t.Issue = nil
	return s.save(ctx, t, o, store.Charge{})
}
