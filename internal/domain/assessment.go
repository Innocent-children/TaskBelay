package domain

import "slices"

/**
 * semanticSurface binds reasoning to relevant current content, not a global epoch.
 */
func (t Task) semanticSurface(scopes []string) Digest {
	entries := []any{}
	for _, r := range t.Repositories {
		for _, e := range r.Binding.TaskSurface {
			p := t.QualifiedPath(r.Key, e.Path)
			included := len(scopes) == 0
			for _, scope := range scopes {
				included = included || MatchPath(scope, p)
			}
			if !included {
				continue
			}
			var staged any
			if e.IndexContentDigest != e.BaseContentDigest && e.IndexContentDigest != e.WorktreeContentDigest {
				staged = []any{e.IndexMode, e.IndexContentDigest}
			}
			entries = append(entries, []any{p, e.BaseMode, e.BaseContentDigest, e.WorktreeMode, e.WorktreeContentDigest, staged})
		}
	}
	return Hash(entries)
}
func (t Task) PathReviewDigest(p string, ids []string) Digest {
	goals := []Goal{}
	for _, id := range ids {
		g, ok := t.Goal(id)
		if !ok {
			return ""
		}
		goals = append(goals, g)
	}
	return Hash([]any{p, goals, t.semanticSurface([]string{p})})
}
func (t Task) ConstraintReviewDigest(c Constraint) Digest {
	return Hash([]any{c, t.semanticSurface(c.Paths)})
}
func (t Task) FactReviewDigest(f Fact) Digest {
	scopes := []string{}
	if f.Kind != "change_magnitude" {
		scopes = []string{f.Path}
	}
	return Hash([]any{f, t.semanticSurface(scopes)})
}

/**
 * MergeAssessment stamps submitted fragments from observed facts. Host-provided
 * scope digests cannot bless retained reasoning or override deterministic rules.
 */
func (t Task) MergeAssessment(a Assessment) *Assessment {
	if a.Review != nil {
		r := *a.Review
		r.PlanDigest, r.ContentDigest = t.PlanDigest(), t.ContentDigest()
		a.Review = &r
	}
	old := Assessment{}
	if t.Assessment != nil {
		old = *t.Assessment
	}
	if a.Review == nil && old.Review != nil {
		a.Review = old.Review
	}
	paths := []PathJudgment{}
	for _, r := range t.Repositories {
		for _, e := range r.Binding.TaskSurface {
			p := t.QualifiedPath(r.Key, e.Path)
			for _, j := range old.Paths {
				if j.Path == p && j.ScopeDigest == t.PathReviewDigest(j.Path, j.GoalIDs) {
					paths = append(paths, j)
				}
			}
		}
	}
	for _, j := range a.Paths {
		j.ScopeDigest = t.PathReviewDigest(j.Path, j.GoalIDs)
		paths = slices.DeleteFunc(paths, func(v PathJudgment) bool { return v.Path == j.Path })
		paths = append(paths, j)
	}
	merge := func(prior, submitted []Judgment, current map[string]Digest) []Judgment {
		result := []Judgment{}
		for _, j := range prior {
			if digest, ok := current[j.ID]; ok && j.ScopeDigest == digest {
				result = append(result, j)
			}
		}
		for _, j := range submitted {
			j.ScopeDigest = current[j.ID]
			result = slices.DeleteFunc(result, func(v Judgment) bool { return v.ID == j.ID })
			result = append(result, j)
		}
		return result
	}
	constraints, facts := map[string]Digest{}, map[string]Digest{}
	for _, c := range t.Authorization.Constraints {
		constraints[c.ID] = t.ConstraintReviewDigest(c)
	}
	for _, f := range t.Facts {
		facts[f.ID] = t.FactReviewDigest(f)
	}
	a.Paths = paths
	a.Constraints = merge(old.Constraints, a.Constraints, constraints)
	a.Facts = merge(old.Facts, a.Facts, facts)
	return &a
}
