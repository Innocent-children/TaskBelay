package application

import (
	"context"
	"slices"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type CheckpointRequest struct {
	Identity
	RepositoryFreshness []domain.RepositoryFreshness `json:"repository_freshness,omitempty"`
	Assessment          *domain.Assessment           `json:"assessment,omitempty"`
	Progress            []domain.Progress            `json:"progress,omitempty"`
	Issue               *domain.Issue                `json:"issue,omitempty"`
	HostMS              int64                        `json:"host_ms,omitempty"`
	WaitMS              int64                        `json:"wait_ms,omitempty"`
	Tokens              int64                        `json:"tokens,omitempty"`
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
	if q.HostMS < 0 || q.WaitMS < 0 || q.Tokens < 0 {
		return View{}, domain.Invalid("cost", "reported cost deltas must be nonnegative")
	}
	if len(q.Progress) > 0 {
		if e = requireExecutionAuthority(t); e != nil {
			return View{}, e
		}
	}
	for _, p := range q.Progress {
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
		if e = validateAssessment(t, *q.Assessment); e != nil {
			return View{}, e
		}
		t.Assessment = t.MergeAssessment(*q.Assessment)
	}
	if q.Issue != nil {
		if !slices.Contains([]string{"authorization", "tradeoff"}, q.Issue.Kind) || domain.Text("issue.summary", q.Issue.Summary) != nil || len(q.Issue.Options) < 2 || len(q.Issue.Options) > 8 {
			return View{}, domain.Invalid("issue", "only an unresolved authorization or important tradeoff with concrete options belongs to the user")
		}
		t.Issue = q.Issue
	}
	return s.save(ctx, t, o, store.Charge{HostMS: q.HostMS, WaitMS: q.WaitMS, Tokens: q.Tokens})
}

func validateAssessment(t domain.Task, a domain.Assessment) error {
	if a.AuthorizationDigest != t.Authorization.Digest() || a.ContentDigest != t.ContentDigest() {
		return domain.Fail("ASSESSMENT_STALE", "assessment", "review must bind the current authorization_digest and content_digest")
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
			seen[j.ID] = true
		}
	}
	return nil
}
