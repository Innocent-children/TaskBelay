package domain

import "slices"

type Freshness struct {
	EvidenceID string `json:"evidence_id"`
	CheckID    string `json:"check_id"`
	Valid      bool   `json:"valid"`
	Applicable bool   `json:"applicable"`
	Reason     string `json:"reason"`
}
type Decision struct {
	Advice           string         `json:"advice"`
	Ready            bool           `json:"ready"`
	Findings         []Fact         `json:"findings"`
	MissingChecks    []string       `json:"missing_checks"`
	Evidence         []Freshness    `json:"evidence"`
	PendingRuns      []string       `json:"pending_runs"`
	AcceptedFailures []KnownFailure `json:"accepted_failures"`
}

// Decide uses observations and provenance; it never grades semantic truth.
func Decide(t Task, evidence []Freshness, pending []string) Decision {
	d := Decision{Advice: "continue", Findings: append([]Fact{}, t.Facts...), MissingChecks: []string{}, Evidence: evidence, PendingRuns: pending, AcceptedFailures: []KnownFailure{}}
	if t.Status != "active" {
		d.Advice = "stop"
		return d
	}
	review := t.Assessment
	add := func(id, kind, detail string) {
		d.Findings = append(d.Findings, Fact{ID: id, Kind: kind, Detail: detail, Blocking: true})
	}
	if review == nil {
		add("semantic-review", "semantic", "Review current changes against the authorized outcome and constraints; cite evidence and uncertainty.")
	} else {
		for _, r := range t.Repositories {
			for _, e := range r.Binding.TaskSurface {
				p := t.QualifiedPath(r.Key, e.Path)
				found := false
				for _, j := range review.Paths {
					if j.Path == p && len(j.GoalIDs) > 0 && j.Basis != "" && j.ScopeDigest == t.PathReviewDigest(j.Path, j.GoalIDs, j.WorkID) {
						found = true
					}
				}
				if !found {
					add("unmapped:"+p, "semantic", "Change has no reason tied to an authorized goal: "+p)
				}
			}
		}
		for _, c := range t.Authorization.Constraints {
			found := false
			for _, j := range review.Constraints {
				if j.ID == c.ID && j.Verdict == "satisfied" && j.Basis != "" && j.Uncertainty == "" && j.ScopeDigest == t.ConstraintReviewDigest(c) {
					found = true
				}
			}
			if !found {
				add("constraint:"+c.ID, "semantic", "Constraint needs a supported, certain conclusion or correction: "+c.Statement)
			}
		}
		for _, f := range t.Facts {
			if !f.Blocking {
				found := false
				for _, j := range review.Facts {
					if j.ID == f.ID && j.Verdict == "satisfied" && j.Basis != "" && j.Uncertainty == "" && j.ScopeDigest == t.FactReviewDigest(f) {
						found = true
					}
				}
				if !found {
					add("unreviewed:"+f.ID, "semantic", "Review this observed scope change: "+f.Detail)
				}
			}
		}
	}
	valid := map[string]bool{}
	for _, f := range evidence {
		if f.Valid {
			for _, e := range t.Evidence {
				if e.ID == f.EvidenceID && e.Status == "passed" && e.Source != "claim" {
					valid[e.CheckID] = true
				}
			}
		}
	}
	d.AcceptedFailures = t.AcceptedFailures(evidence)
	for _, x := range d.AcceptedFailures {
		valid[x.CheckID] = true
	}
	done := true
	for _, g := range t.Authorization.Goals {
		completed := false
		for _, p := range t.Progress {
			if p.GoalID == g.ID && p.GoalDigest == Hash(g) && p.Status == "done" {
				completed = true
			}
		}
		done = done && completed
		for _, c := range g.Checks {
			if !valid[c] && !slices.Contains(d.MissingChecks, c) {
				d.MissingChecks = append(d.MissingChecks, c)
			}
		}
	}
	blocking := false
	if !t.ReviewComplete() {
		add("design-review", "semantic", "Review root cause, responsibility, complexity and verification against the current plan and changes before delivery.")
	}
	for _, f := range d.Findings {
		blocking = blocking || f.Blocking
	}
	if blocking {
		d.Advice = "self_correct"
	} else if done && len(d.MissingChecks) > 0 {
		d.Advice = "verify"
	}
	l, u := t.Authorization.Limits, t.Usage
	if u.Runs >= l.Runs || u.ExecutionMS+u.UncertainMS+u.ReservedMS >= l.ExecutionMS || u.CoreMS >= l.CoreMS || u.HostMS+u.WaitMS >= l.HostMS || u.Tokens >= l.Tokens {
		if len(d.MissingChecks) > 0 {
			d.Advice = "stop"
			add("cost-limit", "budget", "Cumulative limit reached. Preserve acceptance requirements; correct the approach or request an authorized limit change.")
		}
	}
	if len(pending) > 0 {
		d.Advice = "stop"
		add("pending-execution", "recovery", "Read the recorded execution; do not replay an operation with an uncertain result.")
	}
	if t.Issue != nil {
		d.Advice = "ask_user"
		add("user-decision", "authorization", t.Issue.Summary)
	}
	if !t.PlanApproved() {
		d.Advice = "ask_user"
		if t.ContentDigest() != t.ProposedContentDigest {
			add("unapproved-changes", "authorization", "Changes were observed before plan confirmation; restore the proposed contents or disclose them in a revised plan.")
		}
		add("plan-confirmation", "authorization", "Show the complete understanding and plan, then record the user's confirmation of this exact plan before implementation or checks.")
	}
	d.Ready = t.PlanApproved() && done && !blocking && len(d.MissingChecks) == 0 && len(pending) == 0 && t.Issue == nil
	return d
}
func (t Task) QualifiedPath(key RepositoryKey, p string) string {
	if len(t.Repositories) == 1 {
		return p
	}
	return string(key) + "::" + p
}
