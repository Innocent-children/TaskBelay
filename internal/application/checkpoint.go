package application

import (
	"context"
	"slices"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type CheckpointRequest struct {
	Identity
	RepositoryFreshness []domain.RepositoryFreshness `json:"repository_freshness,omitempty" description:"Optional [{key,observation}] copied from the workspace helper; Core verifies it."`
	Assessment          *AssessmentInput             `json:"assessment,omitempty" description:"公开审查对象，提交新增/受影响片段。"`
	Progress            []ProgressInput              `json:"progress,omitempty" description:"按goal_id保存的当前实现进展，不是验证结果。"`
	Issue               *domain.Issue                `json:"issue,omitempty" description:"Actual unresolved user authorization/tradeoff question; null in checkpoint does not clear it."`
	HostUsage           *domain.HostUsageReport      `json:"host_usage,omitempty" description:"能力处说明依赖Host报告；请求中为有来源与范围的累计采样。"`
}

func (s *Service) Checkpoint(ctx context.Context, q CheckpointRequest) (View, error) {
	t, o, replay, e := s.begin(ctx, q.Identity, "checkpoint", q)
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
	seenFreshness := map[domain.RepositoryKey]bool{}
	for _, reported := range q.RepositoryFreshness {
		if seenFreshness[reported.Key] {
			return View{}, domain.Invalid("repository_freshness", "duplicate repository key")
		}
		seenFreshness[reported.Key] = true
		found := false
		for i, r := range t.Repositories {
			if r.Key == reported.Key {
				f, e := s.Observer.VerifyRemoteFreshness(ctx, r.Origin.CanonicalWorktreeRoot, r.Binding, r.Origin, &reported.Observation)
				if e != nil {
					return View{}, e
				}
				t.Repositories[i].RemoteLatest = &f
				found = true
			}
		}
		if !found {
			return View{}, domain.Invalid("repository_freshness.key", "repository must be bound to this Task")
		}
	}
	if q.HostUsage != nil {
		if e = q.HostUsage.Validate(); e != nil {
			return View{}, e
		}
	}
	if len(q.Progress) > 0 {
		if e = requireExecutionAuthority(t); e != nil {
			return View{}, e
		}
	}
	for _, input := range q.Progress {
		p := domain.Progress{GoalID: input.GoalID, Status: input.Status, Summary: input.Summary, NextStep: input.NextStep}
		g, ok := t.Goal(p.GoalID)
		if !ok || !slices.Contains([]string{"planned", "doing", "done"}, p.Status) || domain.Text("progress.summary", p.Summary) != nil {
			return View{}, domain.Invalid("progress", "must reference an existing goal, status planned/doing/done and a concrete summary")
		}
		p.GoalDigest = g.RequirementDigest()
		found := false
		for i, old := range t.Progress {
			if old.GoalID == p.GoalID {
				t.Progress[i] = p
				found = true
			}
		}
		if !found {
			t.Progress = append(t.Progress, p)
		}
	}
	if q.Assessment != nil {
		a := q.Assessment.assessment()
		if e = validateAssessment(t, a); e != nil {
			return View{}, e
		}
		v, err := s.view(ctx, t, true)
		if err != nil {
			return View{}, err
		}
		a.Concerns, e = t.MergeConcerns(q.Assessment.Concerns, v.Decision.Evidence)
		if e != nil {
			return View{}, e
		}
		if len(q.Assessment.Concerns) > 0 {
			t.RecordChange("concern", q.Assessment.Summary, "")
		}
		t.Assessment = t.MergeAssessment(a)
	}
	if q.Issue != nil {
		if !slices.Contains([]string{"authorization", "tradeoff"}, q.Issue.Kind) || domain.Text("issue.summary", q.Issue.Summary) != nil || len(q.Issue.Options) < 2 || len(q.Issue.Options) > 8 {
			return View{}, domain.Invalid("issue", "only an unresolved authorization or important tradeoff with concrete options belongs to the user")
		}
		t.Issue = q.Issue
	}
	return s.save(ctx, t, o, store.Charge{HostUsage: q.HostUsage})
}

func validateAssessment(t domain.Task, a domain.Assessment) error {
	if a.AuthorizationDigest != t.Authorization.Digest() || a.ContentDigest != t.ContentDigest() {
		return &domain.Error{Code: "ASSESSMENT_STALE", Message: "review must bind current authorization and content; inspect the changed paths before correcting digests", Details: []domain.ContractViolation{{Path: "assessment", Rule: "requirement", Message: "use the current digests after reviewing actual changes"}}, Current: currentState(t)}
	}
	if e := domain.Text("assessment.summary", a.Summary); e != nil {
		return e
	}
	if a.Review != nil {
		if e := domain.Text("assessment.review.summary", a.Review.Summary); e != nil {
			return e
		}
	}
	seen := map[string]bool{}
	for _, p := range a.Paths {
		if domain.ScopedPath(p.Path) != nil || seen[p.Path] || len(p.GoalIDs) == 0 || domain.Text("assessment.paths.basis", p.Basis) != nil {
			return domain.Invalid("assessment.paths", "requires unique changed paths, goal references and concrete reasons")
		}
		seen[p.Path] = true
		actual := false
		for _, r := range t.Repositories {
			for _, v := range r.Binding.TaskSurface {
				actual = actual || p.Path == t.QualifiedPath(r.Key, v.Path)
			}
		}
		if !actual {
			return domain.Invalid("assessment.paths.path", "must name an actually observed changed path")
		}
		if e := domain.IDs("assessment.paths.goal_ids", p.GoalIDs); e != nil {
			return e
		}
		for _, id := range p.GoalIDs {
			if _, ok := t.Goal(id); !ok {
				return domain.Invalid("assessment.paths.goal_ids", "must name an authorized goal")
			}
		}
	}
	for kind, js := range map[string][]domain.Judgment{"constraints": a.Constraints, "facts": a.Facts} {
		seen := map[string]bool{}
		for _, j := range js {
			exists := false
			if kind == "constraints" {
				for _, v := range t.Authorization.Constraints {
					exists = exists || v.ID == j.ID
				}
			} else {
				for _, v := range t.Facts {
					exists = exists || v.ID == j.ID
				}
			}
			if !exists || seen[j.ID] || !slices.Contains([]string{"satisfied", "violation", "uncertain"}, j.Verdict) || domain.Text("assessment."+kind+".basis", j.Basis) != nil || j.Verdict == "uncertain" && j.Uncertainty == "" {
				return domain.Invalid("assessment."+kind, "requires unique observed IDs, verdict satisfied/violation/uncertain, basis and explicit uncertainty")
			}
			if j.Verdict == "satisfied" && j.Uncertainty != "" {
				return domain.Invalid("assessment."+kind+"."+j.ID+".uncertainty", "satisfied requires a supported conclusion without unresolved uncertainty; use uncertain or record a concern")
			}
			seen[j.ID] = true
		}
	}
	return nil
}
