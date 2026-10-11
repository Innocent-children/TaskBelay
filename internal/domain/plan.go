package domain

import (
	"slices"
	"time"
)

// Plan is the work actually shown to the user. It is not a process cursor.
type Plan struct {
	Understanding      string        `json:"understanding"`
	RootCause          string        `json:"root_cause"`
	Approach           string        `json:"approach"`
	SimplerAlternative string        `json:"simpler_alternative"`
	VerificationBasis  string        `json:"verification_basis"`
	Completion         string        `json:"completion"`
	Work               []PlannedWork `json:"work"`
}
type PlannedWork struct {
	ID        string   `json:"id"`
	GoalIDs   []string `json:"goal_ids"`
	Change    string   `json:"change"`
	Owner     string   `json:"owner"`
	Necessity string   `json:"necessity"`
}
type PlanApproval struct {
	PlanDigest  Digest    `json:"plan_digest"`
	UserRef     string    `json:"user_ref"`
	Response    string    `json:"response"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
type DesignReview struct {
	RootCause        string `json:"root_cause"`
	Responsibilities string `json:"responsibilities"`
	Proportionality  string `json:"proportionality"`
	Verification     string `json:"verification"`
	WouldChooseAgain bool   `json:"would_choose_again"`
	Uncertainty      string `json:"uncertainty"`
	PlanDigest       Digest `json:"plan_digest,omitempty"`
	ContentDigest    Digest `json:"content_digest,omitempty"`
}
type KnownFailure struct {
	CheckID              string    `json:"check_id"`
	FailedEvidenceID     string    `json:"failed_evidence_id"`
	ComparisonEvidenceID string    `json:"comparison_evidence_id"`
	Failures             []string  `json:"failures"`
	UserRef              string    `json:"user_ref"`
	Reason               string    `json:"reason"`
	PlanDigest           Digest    `json:"plan_digest"`
	ContentDigest        Digest    `json:"content_digest"`
	AcceptedAt           time.Time `json:"accepted_at"`
}

func (t Task) PlanDigest() Digest {
	repositories := []any{}
	for _, r := range t.Repositories {
		repositories = append(repositories, []any{r.Key, r.Origin})
	}
	return Hash(struct {
		Authorization   Authorization
		Plan            Plan
		Checks          []Check
		Repositories    []any
		ProposedContent Digest
	}{t.Authorization, t.Plan, t.Checks, repositories, t.ProposedContentDigest})
}
func (t Task) PlanApproved() bool {
	return t.Approval != nil && t.Approval.PlanDigest == t.PlanDigest()
}
func (t Task) Work(id string) (PlannedWork, bool) {
	for _, w := range t.Plan.Work {
		if w.ID == id {
			return w, true
		}
	}
	return PlannedWork{}, false
}
func (t Task) CheckGoals(c Check) []string {
	ids := slices.Clone(c.GoalIDs)
	for _, g := range t.Authorization.Goals {
		if slices.Contains(g.Checks, c.ID) && !slices.Contains(ids, g.ID) {
			ids = append(ids, g.ID)
		}
	}
	return ids
}

// A forecast never grants permission. Explicit allowed paths and exclusions do.
func (a Authorization) AllowsPath(path string) bool {
	allowed := len(a.AllowedPaths) == 0
	for _, scope := range a.AllowedPaths {
		allowed = allowed || MatchPath(scope, path)
	}
	for _, scope := range a.ForbiddenPaths {
		if MatchPath(scope, path) {
			return false
		}
	}
	return allowed
}
func (t Task) ValidatePlan() error {
	if !t.ProposedContentDigest.IsValid() {
		return Invalid("proposed_content_digest", "requires observed contents when the plan was proposed")
	}
	p := t.Plan
	for _, field := range []struct{ name, value string }{
		{"understanding", p.Understanding}, {"root_cause", p.RootCause}, {"approach", p.Approach},
		{"simpler_alternative", p.SimplerAlternative}, {"verification_basis", p.VerificationBasis}, {"completion", p.Completion},
	} {
		if e := Text("plan."+field.name, field.value); e != nil {
			return e
		}
	}
	if len(p.Work) == 0 || len(p.Work) > 64 {
		return Invalid("plan.work", "requires 1..64 necessary work items")
	}
	ids, covered := []string{}, map[string]bool{}
	for _, w := range p.Work {
		ids = append(ids, w.ID)
		if len(w.GoalIDs) == 0 {
			return Invalid("plan.work.goal_ids", "every work item must serve a goal")
		}
		if e := IDs("plan.work.goal_ids", w.GoalIDs); e != nil {
			return e
		}
		for _, id := range w.GoalIDs {
			if _, ok := t.Goal(id); !ok {
				return Invalid("plan.work.goal_ids", "goal must exist")
			}
			covered[id] = true
		}
		for _, f := range []struct{ name, value string }{{"change", w.Change}, {"owner", w.Owner}, {"necessity", w.Necessity}} {
			if e := Text("plan.work."+f.name, f.value); e != nil {
				return e
			}
		}
	}
	if e := IDs("plan.work.id", ids); e != nil {
		return e
	}
	for _, g := range t.Authorization.Goals {
		if !covered[g.ID] {
			return Invalid("plan.work", "every goal needs planned work")
		}
	}
	if t.Approval != nil {
		if !t.PlanApproved() || Text("approval.user_ref", t.Approval.UserRef) != nil || Text("approval.response", t.Approval.Response) != nil || validateUTC(t.Approval.ConfirmedAt) != nil {
			return Invalid("approval", "requires actual user confirmation of the exact current plan")
		}
	}
	if t.Archived && t.Status == "active" {
		return Invalid("archived", "only terminal tasks may be archived")
	}
	return nil
}
func (t Task) ReviewComplete() bool {
	if t.Assessment == nil || t.Assessment.Review == nil {
		return false
	}
	r := t.Assessment.Review
	return r.PlanDigest == t.PlanDigest() && r.ContentDigest == t.ContentDigest() && r.WouldChooseAgain && r.Uncertainty == "" && r.RootCause != "" && r.Responsibilities != "" && r.Proportionality != "" && r.Verification != ""
}
func (t Task) AcceptedFailures(fresh []Freshness) []KnownFailure {
	accepted := []KnownFailure{}
	for _, x := range t.Exceptions {
		if x.PlanDigest != t.PlanDigest() || x.ContentDigest != t.ContentDigest() {
			continue
		}
		c, ok := t.Check(x.CheckID)
		if !ok || c.Purpose != "regression" {
			continue
		}
		failed, comparison := false, false
		for _, f := range fresh {
			for _, e := range t.Evidence {
				if e.ID != f.EvidenceID || e.Source != "core_execution" {
					continue
				}
				if e.ID == x.FailedEvidenceID && e.CheckID == c.ID && e.Status == "failed" && f.Applicable {
					failed = true
				}
				if e.ID == x.ComparisonEvidenceID && f.Valid {
					if cc, ok := t.Check(e.CheckID); ok && cc.Compares == c.ID {
						comparison = true
					}
				}
			}
		}
		if failed && comparison {
			accepted = append(accepted, x)
		}
	}
	return accepted
}

// Labels and bookkeeping cannot erase repeated execution history.
func (t Task) CheckRequirementDigest(c Check) Digest {
	statements := []string{}
	for _, id := range t.CheckGoals(c) {
		if g, ok := t.Goal(id); ok {
			statements = append(statements, "goal:"+g.Statement)
		}
	}
	for _, v := range t.Authorization.Constraints {
		if len(c.ConstraintIDs) == 0 || slices.Contains(c.ConstraintIDs, v.ID) {
			statements = append(statements, "constraint:"+v.Kind+":"+v.Statement)
		}
	}
	slices.Sort(statements)
	return Hash(slices.Compact(statements))
}
