package domain

import (
	"slices"
	"time"
)

type Plan struct {
	Approach          string `json:"approach"`
	VerificationBasis string `json:"verification_basis"`
}
type BudgetApproval struct {
	DecisionDigest Digest    `json:"decision_digest"`
	UserRef        string    `json:"user_ref"`
	Response       string    `json:"response"`
	ConfirmedAt    time.Time `json:"confirmed_at"`
}
type DesignReview struct {
	Summary       string `json:"summary"`
	Uncertainty   string `json:"uncertainty"`
	PlanDigest    Digest `json:"plan_digest,omitempty"`
	ContentDigest Digest `json:"content_digest,omitempty"`
}
type KnownFailure struct {
	CheckID              string    `json:"check_id"`
	FailedEvidenceID     string    `json:"failed_evidence_id"`
	ComparisonEvidenceID string    `json:"comparison_evidence_id"`
	Failures             []string  `json:"failures"`
	UserRef              string    `json:"user_ref"`
	Reason               string    `json:"reason"`
	AcceptedAt           time.Time `json:"accepted_at"`
}

func (t Task) PlanDigest() Digest {
	return Hash([]any{t.Authorization.Digest(), t.Plan, t.Checks})
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
	if e := Text("plan.approach", t.Plan.Approach); e != nil {
		return e
	}
	if e := Text("plan.verification_basis", t.Plan.VerificationBasis); e != nil {
		return e
	}
	if b := t.BudgetDecision; b != nil {
		if !b.DecisionDigest.IsValid() || Text("budget_decision.user_ref", b.UserRef) != nil || Text("budget_decision.response", b.Response) != nil || validateUTC(b.ConfirmedAt) != nil {
			return Invalid("budget_decision", "requires the actual budget decision and reference")
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
	return r.PlanDigest == t.PlanDigest() && r.ContentDigest == t.ContentDigest() && r.Uncertainty == "" && r.Summary != ""
}
func (t Task) AcceptedFailures(fresh []Freshness) []KnownFailure {
	accepted := []KnownFailure{}
	for _, x := range t.Exceptions {
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
