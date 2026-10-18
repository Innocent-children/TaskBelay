package domain

import "slices"

type Freshness struct {
	EvidenceID string `json:"evidence_id" description:"Existing evidence identity whose applicability was evaluated."`
	CheckID    string `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Valid      bool   `json:"valid" description:"Actual passing result applicable to the observed conditions."`
	Applicable bool   `json:"applicable" description:"此记录对本次已观察条件是否适用；保存状态读取不保证当前仍适用。"`
	Reason     string `json:"reason" description:"Specific reason this record can or cannot establish the current check result."`
}
type Gap struct {
	Code       string   `json:"code" description:"Stable unmet-condition category; use this instead of parsing message text."`
	Path       string   `json:"path" description:"Exact request field or saved object needing attention."`
	Subject    string   `json:"subject" description:"Affected goal, check, concern, path or execution identity."`
	Message    string   `json:"message" description:"Concrete reason this condition is not met."`
	Action     string   `json:"action" enum:"continue,review,self_correct,verify,stop,ask_user,recover_run" description:"Required work within existing authority; ask_user only for a real saved decision."`
	RelatedIDs []string `json:"related_ids" description:"Existing related goal, check or run identities; do not invent replacements."`
}
type Decision struct {
	Advice           string         `json:"advice" description:"下一类有意义的操作；stop也可能表示终态或未知运行，应结合gaps。"`
	Ready            bool           `json:"ready" description:"仅表示active Task当前满足完成前置条件；终态通常为false。"`
	Gaps             []Gap          `json:"gaps" description:"当前未满足条件的具体条目；与finish错误使用同一次计算。"`
	MissingChecks    []string       `json:"missing_checks" description:"当前还缺少适用结果的必需检查ID集合。"`
	Evidence         []Freshness    `json:"evidence" description:"Saved observations and execution records; claims do not satisfy checks."`
	PendingRuns      []string       `json:"pending_runs" description:"尚不能确定最终结果的原运行ID；不能因此启动另一份命令。"`
	AcceptedFailures []KnownFailure `json:"accepted_failures" description:"Applicable accepted existing regression failures; the original result remains failed."`
}

/**
 * Decide derives every completion gap from one observed Task. Semantic testimony
 * cannot remove a deterministic violation or manufacture a passing execution.
 */
func Decide(t Task, evidence []Freshness, pending []string) Decision {
	d := Decision{Advice: "continue", Gaps: []Gap{}, MissingChecks: []string{}, Evidence: evidence, PendingRuns: pending, AcceptedFailures: []KnownFailure{}}
	if t.Status != "active" {
		if t.Status == "done" {
			d.AcceptedFailures = slices.Clone(t.Exceptions)
		}
		d.Advice = "stop"
		return d
	}
	add := func(code, path, subject, message, action string, ids ...string) {
		d.Gaps = append(d.Gaps, Gap{code, path, subject, message, action, append([]string{}, ids...)})
	}
	needsCorrection := false
	for _, f := range t.Facts {
		if f.Blocking {
			needsCorrection = true
			add("scope_violation", f.Path, f.ID, f.Detail, "self_correct", f.ID)
		}
	}
	review := t.Assessment
	if review == nil {
		add("assessment_missing", "assessment", "assessment", "Review the actual changed paths and observed constraints.", "review")
	} else {
		for _, r := range t.Repositories {
			for _, e := range r.Binding.TaskSurface {
				p := t.QualifiedPath(r.Key, e.Path)
				if !slices.ContainsFunc(review.Paths, func(j PathJudgment) bool {
					return j.Path == p && len(j.GoalIDs) > 0 && j.Basis != "" && j.ScopeDigest == t.PathReviewDigest(j.Path, j.GoalIDs)
				}) {
					add("path_unmapped", "assessment.paths", p, "This changed path needs a current reason tied to an authorized goal: "+p, "review")
				}
			}
		}
		for _, c := range t.Authorization.Constraints {
			found := false
			for _, j := range review.Constraints {
				if j.ID == c.ID && j.ScopeDigest == t.ConstraintReviewDigest(c) {
					needsCorrection = needsCorrection || j.Verdict == "violation"
					found = j.Verdict == "satisfied" && j.Basis != "" && j.Uncertainty == ""
				}
			}
			if !found {
				add("constraint_unresolved", "assessment.constraints", c.ID, "A current supported conclusion is required for: "+c.Statement, "review", c.ID)
			}
		}
		for _, f := range t.Facts {
			if f.Blocking {
				continue
			}
			found := false
			for _, j := range review.Facts {
				if j.ID == f.ID && j.ScopeDigest == t.FactReviewDigest(f) {
					needsCorrection = needsCorrection || j.Verdict == "violation"
					found = j.Verdict == "satisfied" && j.Basis != "" && j.Uncertainty == ""
				}
			}
			if !found {
				add("fact_unreviewed", "assessment.facts", f.ID, f.Detail, "review", f.ID)
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
		completed := slices.ContainsFunc(t.Progress, func(p Progress) bool {
			return p.GoalID == g.ID && p.GoalDigest == g.RequirementDigest() && p.Status == "done"
		})
		done = done && completed
		if !completed {
			add("goal_not_done", "progress", g.ID, "Complete the authorized goal and record its actual result: "+g.Statement, "continue", g.ID)
		}
		for _, c := range t.RequiredChecks(g.ID) {
			if valid[c.ID] || slices.Contains(d.MissingChecks, c.ID) {
				continue
			}
			d.MissingChecks = append(d.MissingChecks, c.ID)
			code, reason := "check_missing", "No applicable passing result exists for this required check."
			ids := []string{c.ID}
			for _, f := range evidence {
				if f.CheckID == c.ID {
					code, reason = "check_stale", f.Reason
					if f.Applicable {
						code = "check_failed"
					}
					ids = append(ids, f.EvidenceID)
				}
			}
			add(code, "checks", c.ID, reason, "verify", ids...)
		}
	}
	if !t.ReviewComplete() {
		code, reason := "review_missing", "Submit assessment.review.summary after inspecting the actual diff, responsibilities and verification."
		if review != nil && review.Review != nil {
			code, reason = "review_stale", "The final review refers to a different plan or content; review the actual changes."
		}
		add(code, "assessment.review", "review", reason, "review")
	}
	for _, c := range t.ConcernStates(evidence) {
		if !c.Applicable {
			code := "concern_open"
			if c.Resolution != nil {
				code = "concern_stale"
			}
			add(code, "assessment.concerns", c.ID, c.Reason+": "+c.Summary, "self_correct", append(slices.Clone(c.GoalIDs), c.CheckIDs...)...)
			needsCorrection = true
		}
	}
	if needsCorrection {
		d.Advice = "self_correct"
	} else if done && len(d.MissingChecks) > 0 {
		d.Advice = "verify"
	} else if done && len(d.Gaps) > 0 {
		d.Advice = "review"
	}
	l, u := t.Authorization.Limits, t.Usage
	needsCommand := slices.ContainsFunc(t.Plan.Checks, func(c Check) bool { return c.Kind == "command" && slices.Contains(d.MissingChecks, c.ID) })
	if metric := u.UnavailableHostLimit(l); needsCommand && metric != "" {
		d.Advice = "stop"
		add("host_usage_unavailable", "usage.host."+metric, metric, "An explicit Host limit needs complete actual usage; report it or obtain a real budget change before a new execution.", "self_correct")
	}
	if len(d.MissingChecks) > 0 && (LimitReached(u.Runs, l.Runs) || LimitReached(u.ExecutionMS+u.UncertainMS+u.ReservedMS, l.ExecutionMS) || LimitReached(u.CoreMS, l.CoreMS) || LimitReached(u.HostMS+u.WaitMS, l.HostMS) || LimitReached(u.Tokens, l.Tokens)) {
		d.Advice = "stop"
		add("cost_limit", "authorization.limits", "budget", "An explicit cumulative limit was reached; preserve acceptance and obtain an actual budget decision if needed.", "stop")
	}
	if len(pending) > 0 {
		d.Advice = "stop"
		add("pending_execution", "run_id", pending[0], "Read and recover the original execution instead of starting another.", "recover_run", pending...)
	}
	if t.Issue != nil {
		d.Advice = "ask_user"
		add("user_decision", "issue", string(Hash(t.Issue)), t.Issue.Summary, "ask_user")
	}
	d.Ready = len(d.Gaps) == 0
	return d
}
func (t Task) QualifiedPath(key RepositoryKey, p string) string {
	if len(t.Repositories) == 1 {
		return p
	}
	return string(key) + "::" + p
}
