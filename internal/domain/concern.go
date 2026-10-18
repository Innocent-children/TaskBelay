package domain

import (
	"slices"
	"strings"
)

/**
 * Concerns retain engineering questions independently of final review prose.
 * Resolutions bind the relevant goals, checks and contents; they never waive a
 * deterministic constraint, required check or unresolved execution.
 */
type ConcernInput struct {
	ID         string             `json:"id" description:"Stable concern identity. Omitted concerns remain saved; reuse this ID to resolve the same question."`
	Summary    string             `json:"summary" description:"The original engineering question. Its definition and dependencies cannot be silently replaced."`
	GoalIDs    []string           `json:"goal_ids" description:"Current goals affected by this concern; at least one."`
	CheckIDs   []string           `json:"check_ids,omitempty" description:"Known relevant checks. Resolution evidence must cover every named check; omission permits later relevant evidence."`
	Paths      []string           `json:"paths,omitempty" description:"Relevant qualified paths. Omission conservatively binds all task changes."`
	Resolution *ConcernResolution `json:"resolution,omitempty" description:"Null or omission leaves this concern open. Supply an actual supported resolution; omission of the entire concern retains its previous resolution."`
}
type ConcernResolution struct {
	Kind        string   `json:"kind" enum:"verified,not_blocking,scope_changed" description:"verified requires actual passing evidence; not_blocking records a delivery limitation; scope_changed references a saved user boundary change."`
	Basis       string   `json:"basis" description:"Concrete reasoning explaining how the original concern was addressed."`
	EvidenceIDs []string `json:"evidence_ids,omitempty" description:"Required only for verified. Existing applicable non-claim passing results covering the concern's goals and checks."`
	UserRef     string   `json:"user_ref,omitempty" description:"Required only for scope_changed. Must equal the saved current authorization reference after a real boundary change."`
}
type Concern struct {
	ConcernInput
	OpenedAuthorizationDigest Digest `json:"opened_authorization_digest" description:"Original user boundary when Core saved this concern."`
	ResolutionScopeDigest     Digest `json:"resolution_scope_digest,omitempty" description:"Core binding of concern resolution to relevant goals, checks and contents."`
}
type ConcernState struct {
	ConcernInput
	Applicable bool   `json:"applicable" description:"此记录对本次已观察条件是否适用；保存状态读取不保证当前仍适用。"`
	Reason     string `json:"reason" description:"Why the saved resolution currently applies or needs to be revisited."`
}
type Limitation struct {
	ID      string `json:"id" description:"Stable ID of the concern resolved as not_blocking."`
	Summary string `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Basis   string `json:"basis" description:"与实际观察、当前目标或检查关联的具体判断理由。"`
}

func (t Task) ConcernDigest(c Concern) Digest {
	goals := []any{}
	checks := []any{}
	for _, id := range c.GoalIDs {
		g, exists := t.Goal(id)
		goals = append(goals, []any{id, exists, g})
	}
	ids := slices.Clone(c.CheckIDs)
	if c.Resolution != nil {
		for _, id := range c.Resolution.EvidenceIDs {
			for _, ev := range t.Evidence {
				if ev.ID == id && !slices.Contains(ids, ev.CheckID) {
					ids = append(ids, ev.CheckID)
				}
			}
		}
	}
	slices.Sort(ids)
	for _, id := range ids {
		check, exists := t.Check(id)
		checks = append(checks, []any{id, exists, t.CheckDigest(check)})
	}
	var boundary any
	if c.Resolution != nil && c.Resolution.Kind == "scope_changed" {
		boundary = []any{t.Authorization.BoundaryDigest(), t.Authorization.UserRef}
	}
	return Hash([]any{goals, checks, t.Authorization.Constraints, t.semanticSurface(c.Paths), boundary})
}

func (t Task) ConcernApplicable(c Concern, freshness []Freshness) (bool, string) {
	if c.Resolution == nil {
		return false, "concern has no resolution"
	}
	if c.ResolutionScopeDigest != t.ConcernDigest(c) {
		return false, "related goals, checks or contents changed; review the original concern"
	}
	if c.Resolution.Kind == "scope_changed" {
		if c.Resolution.UserRef != t.Authorization.UserRef || c.OpenedAuthorizationDigest == t.Authorization.BoundaryDigest() {
			return false, "resolution does not reference a saved change to the original user boundary"
		}
	}
	if c.Resolution.Kind == "verified" {
		coveredChecks, coveredGoals := map[string]bool{}, map[string]bool{}
		for _, id := range c.Resolution.EvidenceIDs {
			valid := t.Status != "active"
			for _, f := range freshness {
				valid = valid || f.EvidenceID == id && f.Valid
			}
			found := false
			for _, ev := range t.Evidence {
				if ev.ID != id || ev.Status != "passed" || ev.Source == "claim" || !valid {
					continue
				}
				check, ok := t.Check(ev.CheckID)
				if !ok {
					continue
				}
				for _, goal := range check.GoalIDs {
					if slices.Contains(c.GoalIDs, goal) {
						found, coveredGoals[goal] = true, true
					}
				}
				coveredChecks[check.ID] = true
			}
			if !found {
				return false, "resolution evidence is missing, stale, failed or unrelated to this concern"
			}
		}
		for _, id := range c.GoalIDs {
			if !coveredGoals[id] {
				return false, "resolution evidence does not cover every affected goal"
			}
		}
		for _, id := range c.CheckIDs {
			if _, current := t.Check(id); current && !coveredChecks[id] {
				return false, "resolution evidence does not cover every declared check"
			}
		}
	}
	return true, "resolution applies to the observed concern scope"
}

func (t Task) ConcernStates(freshness []Freshness) []ConcernState {
	states := []ConcernState{}
	if t.Assessment != nil {
		for _, c := range t.Assessment.Concerns {
			ok, reason := t.ConcernApplicable(c, freshness)
			states = append(states, ConcernState{ConcernInput: c.ConcernInput, Applicable: ok, Reason: reason})
		}
	}
	return states
}

func (t Task) DeliveryLimitations(freshness []Freshness) []Limitation {
	if t.Status != "active" {
		return slices.Clone(t.Limitations)
	}
	result := []Limitation{}
	for _, c := range t.ConcernStates(freshness) {
		if c.Applicable && c.Resolution.Kind == "not_blocking" {
			result = append(result, Limitation{ID: c.ID, Summary: c.Summary, Basis: c.Resolution.Basis})
		}
	}
	return result
}

/**
 * MergeConcerns preserves omitted records and immutable question definitions.
 * Only a supported resolution can change whether an existing question is open.
 */
func (t Task) MergeConcerns(submitted []ConcernInput, freshness []Freshness) ([]Concern, error) {
	result := []Concern{}
	if t.Assessment != nil {
		result = slices.Clone(t.Assessment.Concerns)
	}
	seen := map[string]bool{}
	for _, q := range submitted {
		if !ID(q.ID).IsValid() || seen[q.ID] || Text("concerns.summary", q.Summary) != nil || len(q.GoalIDs) == 0 {
			return nil, Invalid("assessment.concerns", "requires unique stable IDs, a concrete question and affected goals")
		}
		seen[q.ID] = true
		if err := IDs("assessment.concerns.goal_ids", q.GoalIDs); err != nil {
			return nil, err
		}
		if err := IDs("assessment.concerns.check_ids", q.CheckIDs); err != nil {
			return nil, err
		}
		if err := Paths("assessment.concerns.paths", q.Paths); err != nil {
			return nil, err
		}
		for _, p := range q.Paths {
			key, _, qualified := strings.Cut(p, "::")
			if qualified {
				if _, ok := t.Repository(RepositoryKey(key)); !ok {
					return nil, Invalid("assessment.concerns.paths", "qualified paths must name a bound repository")
				}
			} else if len(t.Repositories) > 1 {
				return nil, Invalid("assessment.concerns.paths", "use repository_key::path for a multi-repository concern")
			}
		}
		at := slices.IndexFunc(result, func(c Concern) bool { return c.ID == q.ID })
		c := Concern{ConcernInput: q, OpenedAuthorizationDigest: t.Authorization.BoundaryDigest()}
		if at >= 0 {
			previous, next := result[at].ConcernInput, q
			previous.Resolution, next.Resolution = nil, nil
			if Hash(previous) != Hash(next) {
				return nil, Invalid("assessment.concerns."+q.ID, "retain the original question and dependencies; resolve it instead of relabelling it")
			}
			c.OpenedAuthorizationDigest = result[at].OpenedAuthorizationDigest
		} else {
			for _, id := range q.GoalIDs {
				if _, ok := t.Goal(id); !ok {
					return nil, Invalid("assessment.concerns.goal_ids", "new concerns must name current authorized goals")
				}
			}
			for _, id := range q.CheckIDs {
				check, ok := t.Check(id)
				if !ok || !slices.ContainsFunc(check.GoalIDs, func(id string) bool { return slices.Contains(q.GoalIDs, id) }) {
					return nil, Invalid("assessment.concerns.check_ids", "checks must exist and serve an affected goal")
				}
			}
		}
		if r := q.Resolution; r != nil {
			if Text("assessment.concerns.resolution.basis", r.Basis) != nil || !slices.Contains([]string{"verified", "not_blocking", "scope_changed"}, r.Kind) {
				return nil, Invalid("assessment.concerns.resolution", "requires verified, not_blocking or scope_changed with concrete reasoning")
			}
			if err := IDs("assessment.concerns.resolution.evidence_ids", r.EvidenceIDs); err != nil {
				return nil, err
			}
			if (r.Kind == "verified") != (len(r.EvidenceIDs) > 0) || (r.Kind == "scope_changed") != (r.UserRef != "") {
				return nil, Invalid("assessment.concerns.resolution", "verified requires only evidence_ids; scope_changed requires only the saved user_ref")
			}
			if r.Kind != "scope_changed" && slices.ContainsFunc(q.GoalIDs, func(id string) bool { _, ok := t.Goal(id); return !ok }) {
				return nil, Invalid("assessment.concerns.resolution", "removed goals require a resolution referencing the saved scope change")
			}
			c.ResolutionScopeDigest = t.ConcernDigest(c)
			if ok, reason := t.ConcernApplicable(c, freshness); !ok {
				return nil, Invalid("assessment.concerns."+q.ID+".resolution", reason)
			}
		}
		if at < 0 {
			result = append(result, c)
		} else {
			result[at] = c
		}
	}
	return result, nil
}
