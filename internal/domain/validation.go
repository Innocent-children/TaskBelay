package domain

import "slices"

func (t Task) ValidateRecords() error {
	if b := t.BudgetDecision; b != nil {
		if !b.DecisionDigest.IsValid() || Text("budget_decision.user_ref", b.UserRef) != nil || Text("budget_decision.response", b.Response) != nil || validateUTC(b.ConfirmedAt) != nil {
			return Invalid("budget_decision", "requires the actual budget decision and reference")
		}
	}
	if t.Archived && t.Status == "active" {
		return Invalid("archived", "only terminal tasks may be archived")
	}

	if len(t.Changes) > 64 {
		return Invalid("changes", "at most 64 recent decisions")
	}
	for _, c := range t.Changes {
		if c.Revision == 0 || c.Revision > t.Revision || Text("changes.summary", c.Summary) != nil || validateUTC(c.At) != nil || !slices.Contains([]string{"authorization", "plan", "budget", "user_decision", "accepted_failure"}, c.Kind) {
			return Invalid("changes", "invalid saved change")
		}
	}

	if len(t.Exceptions) > 64 {
		return Invalid("exceptions", "at most 64 current known-failure decisions")
	}
	for _, x := range t.Exceptions {
		if !ID(x.CheckID).IsValid() || !ID(x.FailedEvidenceID).IsValid() || !ID(x.ComparisonEvidenceID).IsValid() || len(x.Failures) == 0 || len(x.Failures) > 64 || Text("exceptions.user_ref", x.UserRef) != nil || Text("exceptions.reason", x.Reason) != nil || validateUTC(x.AcceptedAt) != nil {
			return Invalid("exceptions", "invalid user decision or evidence binding")
		}
		for _, f := range x.Failures {
			if Text("exceptions.failures", f) != nil {
				return Invalid("exceptions.failures", "name each accepted existing failure")
			}
		}
	}
	if validateUTC(t.CreatedAt) != nil || validateUTC(t.UpdatedAt) != nil || t.UpdatedAt.Before(t.CreatedAt) {
		return Invalid("task.timestamps", "invalid creation or update time")
	}
	seen := map[string]bool{}
	for _, p := range t.Progress {
		if _, ok := t.Goal(p.GoalID); !ok || seen[p.GoalID] || !p.GoalDigest.IsValid() || !slices.Contains([]string{"planned", "doing", "done"}, p.Status) || Text("progress.summary", p.Summary) != nil {
			return Invalid("progress", "invalid current progress record")
		}
		seen[p.GoalID] = true
	}
	seen = map[string]bool{}
	for _, e := range t.Evidence {
		c, ok := t.Check(e.CheckID)
		if !ok || !ID(e.ID).IsValid() || seen[e.ID] || !slices.Contains([]string{"core_execution", "claim", "human"}, e.Source) || !slices.Contains([]string{"passed", "failed", "not_run", "timeout", "interrupted", "interrupted_unknown", "invalidated", "output_limit"}, e.Status) || !e.CheckDigest.IsValid() || !e.InputsDigest.IsValid() || !e.EnvironmentDigest.IsValid() || validateUTC(e.RecordedAt) != nil || Text("evidence.summary", e.Summary) != nil {
			return Invalid("evidence", "invalid provenance or applicability record")
		}
		if e.Source == "human" && (c.Kind != "human" || Text("evidence.user_ref", e.UserRef) != nil) {
			return Invalid("evidence", "human record requires an actual observation reference")
		}
		if e.Source == "core_execution" && c.Kind != "command" {
			return Invalid("evidence", "only a command has Core execution provenance")
		}
		seen[e.ID] = true
	}
	if t.Assessment != nil {
		if !t.Assessment.AuthorizationDigest.IsValid() || !t.Assessment.ContentDigest.IsValid() || Text("assessment.summary", t.Assessment.Summary) != nil {
			return Invalid("assessment", "invalid saved semantic assessment")
		}
		if len(t.Assessment.Paths) > MaxFingerprintPaths*8 || len(t.Assessment.Constraints) > 64 || len(t.Assessment.Facts) > MaxFingerprintPaths*8 {
			return Invalid("assessment", "semantic review exceeds its bound")
		}
		for _, p := range t.Assessment.Paths {
			if !p.ScopeDigest.IsValid() {
				return Invalid("assessment.paths.scope_digest", "requires a Core-bound semantic scope")
			}
		}
		for _, js := range [][]Judgment{t.Assessment.Constraints, t.Assessment.Facts} {
			for _, j := range js {
				if !j.ScopeDigest.IsValid() {
					return Invalid("assessment.scope_digest", "requires a Core-bound semantic scope")
				}
			}
		}
	}
	if t.Status == "done" && len(t.Acceptance) != len(t.Authorization.Goals) {
		return Invalid("acceptance", "terminal completion requires saved conclusions for every goal")
	}
	return nil
}
