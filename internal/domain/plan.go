package domain

import (
	"slices"
)

type Plan struct {
	Approach          string   `json:"approach" description:"当前实现方法与职责选择；普通调整沿用已有授权。"`
	VerificationBasis string   `json:"verification_basis" description:"为什么这些检查足以覆盖当前改动及其主要失败路径，以及未覆盖范围。"`
	ExpectedPaths     []string `json:"expected_paths,omitempty" description:"Optional predicted paths, not a user allowlist or permission."`
	ReviewAfterLines  int      `json:"review_after_lines,omitempty" description:"Optional nonnegative review hint; never a new workflow or permission."`
	Checks            []Check  `json:"checks" description:"Full current check definitions. Every goal needs an acceptance check; relevant regression checks are also required, diagnostics only inform reasoning."`
}
type DesignReview struct {
	Summary       string `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	PlanDigest    Digest `json:"plan_digest,omitempty" description:"Core-generated current plan/authorization digest; read-only."`
	ContentDigest Digest `json:"content_digest,omitempty" description:"当前内容摘要。"`
}

func (t Task) PlanDigest() Digest {
	return Hash([]any{t.Authorization.Digest(), t.Plan})
}

/**
 * The plan owns the only goal-to-check relation. Diagnostics inform reasoning;
 * acceptance and regression checks must be resolved before delivery.
 */
func (t Task) RequiredChecks(goalID string) []Check {
	checks := []Check{}
	for _, c := range t.Plan.Checks {
		if c.Purpose != "diagnostic" && slices.Contains(c.GoalIDs, goalID) {
			checks = append(checks, c)
		}
	}
	return checks
}
func (t Task) AcceptanceChecks(goalID string) []Check {
	return slices.DeleteFunc(t.RequiredChecks(goalID), func(c Check) bool { return c.Purpose != "acceptance" })
}

func (t Task) ValidatePlan() error {
	if e := Paths("plan.expected_paths", t.Plan.ExpectedPaths); e != nil {
		return e
	}
	if t.Plan.ReviewAfterLines < 0 {
		return Invalid("plan.review_after_lines", "must be nonnegative")
	}
	if e := Text("plan.approach", t.Plan.Approach); e != nil {
		return e
	}
	if e := Text("plan.verification_basis", t.Plan.VerificationBasis); e != nil {
		return e
	}
	return nil
}
func (t Task) ReviewComplete() bool {
	if t.Assessment == nil || t.Assessment.Review == nil {
		return false
	}
	r := t.Assessment.Review
	return r.PlanDigest == t.PlanDigest() && r.ContentDigest == t.ContentDigest() && r.Summary != ""
}

/**
 * Labels and bookkeeping cannot erase repeated execution history.
 */
func (t Task) CheckRequirementDigest(c Check) Digest {
	statements := []string{}
	for _, id := range c.GoalIDs {
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
