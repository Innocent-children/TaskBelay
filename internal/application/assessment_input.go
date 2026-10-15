package application

import "github.com/Innocent-children/taskbelay/internal/domain"

/**
 * Submissions contain Host-owned judgments only. Applicability digests and
 * execution provenance are written by Core, never accepted from a caller.
 */
type AssessmentInput struct {
	AuthorizationDigest domain.Digest         `json:"authorization_digest" description:"Current authorization digest from get_task/open_task; re-read after a boundary change."`
	ContentDigest       domain.Digest         `json:"content_digest" description:"Current observed content digest. Review changed files before replacing a stale digest."`
	Summary             string                `json:"summary" description:"Concrete explanation of the observed changes and current reasoning."`
	Paths               []PathJudgmentInput   `json:"paths,omitempty" description:"Only new or affected changed paths. Omission retains applicable saved judgments."`
	Constraints         []JudgmentInput       `json:"constraints,omitempty" description:"Judgments of actual authorization constraint IDs."`
	Facts               []JudgmentInput       `json:"facts,omitempty" description:"Judgments of observed fact IDs, not IDs from decision.gaps."`
	Review              *ReviewInput          `json:"review,omitempty" description:"Final diff/design review; Core binds it to the current plan and content."`
	Concerns            []domain.ConcernInput `json:"concerns,omitempty" description:"New questions or resolutions by stable ID. Omitted records and an empty array preserve existing concerns."`
}
type ReviewInput struct {
	Summary string `json:"summary" description:"Explain resolved causes, responsibilities, complexity and actual verification. Record unresolved questions separately in concerns."`
}
type PathJudgmentInput struct {
	Path    string   `json:"path" description:"Actually observed changed path; use repository_key::path for multiple repositories."`
	GoalIDs []string `json:"goal_ids" description:"Existing authorized goals that require this change."`
	Basis   string   `json:"basis" description:"Concrete reason the change serves these goals."`
}
type JudgmentInput struct {
	ID          string `json:"id" description:"Existing constraint or observed fact ID, never a derived decision gap ID."`
	Verdict     string `json:"verdict" enum:"satisfied,violation,uncertain" description:"Supported semantic judgment; uncertain cannot satisfy completion."`
	Basis       string `json:"basis" description:"Observed code, result or actual user information supporting this judgment."`
	Uncertainty string `json:"uncertainty" description:"Required nonempty for uncertain; satisfied requires an empty string."`
}
type ProgressInput struct {
	GoalID   string `json:"goal_id" description:"An existing current authorized goal."`
	Status   string `json:"status" enum:"planned,doing,done" description:"Implementation progress. done is not proof that acceptance checks passed."`
	Summary  string `json:"summary" description:"Actual work completed or remaining for this goal."`
	NextStep string `json:"next_step,omitempty" description:"Next useful action; empty when no implementation step remains."`
}

func (a AssessmentInput) assessment() domain.Assessment {
	result := domain.Assessment{AuthorizationDigest: a.AuthorizationDigest, ContentDigest: a.ContentDigest, Summary: a.Summary}
	for _, p := range a.Paths {
		result.Paths = append(result.Paths, domain.PathJudgment{Path: p.Path, GoalIDs: p.GoalIDs, Basis: p.Basis})
	}
	for _, pair := range []struct {
		input  []JudgmentInput
		target *[]domain.Judgment
	}{{a.Constraints, &result.Constraints}, {a.Facts, &result.Facts}} {
		for _, j := range pair.input {
			*pair.target = append(*pair.target, domain.Judgment{ID: j.ID, Verdict: j.Verdict, Basis: j.Basis, Uncertainty: j.Uncertainty})
		}
	}
	if a.Review != nil {
		result.Review = &domain.DesignReview{Summary: a.Review.Summary}
	}
	return result
}
