package domain

import (
	"slices"
	"time"
)

/**
 * BudgetDecision expresses one actual decision, synchronizing overlapping work caps.
 */
type BudgetDecision struct {
	Runs        int64 `json:"runs" description:"Explicit user cap on actual command runs; zero removes this dimension cap. It does not reset recorded usage."`
	ExecutionMS int64 `json:"execution_ms" description:"Explicit user cap on cumulative command milliseconds; zero removes this dimension cap. It does not reset recorded usage."`
	WorkMS      int64 `json:"work_ms" description:"One actual cap applied to both Core and Host time; overlapping categories are not added."`
	Tokens      int64 `json:"tokens" description:"Explicit user cap on cumulative Host tokens; zero removes this dimension cap. It does not reset recorded usage."`
}

func (b BudgetDecision) Limits() Limits {
	return Limits{Runs: b.Runs, ExecutionMS: b.ExecutionMS, CoreMS: b.WorkMS, HostMS: b.WorkMS, Tokens: b.Tokens}
}
func (l Limits) Within(ceiling Limits) bool {
	within := func(value, bound int64) bool { return bound == 0 || value > 0 && value <= bound }
	return within(l.Runs, ceiling.Runs) && within(l.ExecutionMS, ceiling.ExecutionMS) && within(l.CoreMS, ceiling.CoreMS) && within(l.HostMS, ceiling.HostMS) && within(l.Tokens, ceiling.Tokens)
}
func LimitReached(used, limit int64) bool { return limit > 0 && used >= limit }

type BudgetLine struct {
	Name      string `json:"name" description:"Usage dimension: runs, execution_ms, core_ms, host_ms or tokens."`
	Limited   bool   `json:"limited" description:"Whether this dimension has a positive explicit cap."`
	Actual    *int64 `json:"actual" description:"Measured or reported amount; null means unavailable, not zero."`
	Reserved  int64  `json:"reserved" description:"Amount reserved but not yet settled."`
	Uncertain int64  `json:"uncertain" description:"Conservative amount attributed to unresolved work in this dimension; not a boolean."`
	Limit     int64  `json:"limit" description:"Explicit cumulative cap; zero means unset."`
	Remaining *int64 `json:"remaining" description:"Remaining explicit capacity; null means unlimited or unknown, not exhausted."`
	Coverage  string `json:"coverage" description:"measured for Core/process counts; unreported, partial or reported for Host-derived metrics."`
}

func NewBudgetLine(name string, actual, reserved, uncertain, limit int64) BudgetLine {
	remaining := max(int64(0), limit-actual-reserved-uncertain)
	line := BudgetLine{Name: name, Limited: limit > 0, Actual: &actual, Reserved: reserved, Uncertain: uncertain, Limit: limit, Coverage: "measured"}
	if limit > 0 {
		line.Remaining = &remaining
	}
	return line
}
func (b BudgetLine) Failure(requested int64) *BudgetFailure {
	actual, remaining := int64(0), int64(0)
	if b.Actual != nil {
		actual = *b.Actual
	}
	if b.Remaining != nil {
		remaining = *b.Remaining
	}
	return &BudgetFailure{Used: actual + b.Reserved + b.Uncertain, Requested: requested, Limit: b.Limit, Actual: actual, Reserved: b.Reserved, Uncertain: b.Uncertain, Remaining: remaining}
}

type CheckBudget struct {
	CheckID   string `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Required  bool   `json:"required" description:"Whether this check still lacks a required applicable result."`
	TimeoutMS int64  `json:"timeout_ms" description:"执行超时上限毫秒，同时用于预算预留；缺省60000。"`
	Startable bool   `json:"startable" description:"Limits can reserve the check; authority, reuse and recovery still apply."`
	Reason    string `json:"reason" description:"操作/处理的实际原因，不能替代所需用户决定。"`
}
type BudgetReport struct {
	Limits         []BudgetLine  `json:"limits" description:"各维度实际用户累计上限；省略/0不设限，不能自动制造默认额度。"`
	Checks         []CheckBudget `json:"checks" description:"默认返回检查摘要；完整定义在context.plan.checks。"`
	Feasible       bool          `json:"feasible" description:"Whether explicit limits can currently reserve required command checks."`
	RequiredRuns   int64         `json:"required_runs" description:"Missing required command count, not automatic permission to execute."`
	WorstCaseMS    int64         `json:"worst_case_ms" description:"Sum of missing required timeouts; planning bound rather than actual usage."`
	RetryReserveMS int64         `json:"retry_reserve_ms" description:"Largest required timeout considered for one retry; not measured time."`
	Warnings       []string      `json:"warnings" description:"Specific budget/reporting limitations, not extra approval requirements."`
}

func (t Task) Budget(missing []string) BudgetReport {
	l, u := t.Authorization.Limits, t.Usage
	report := BudgetReport{Feasible: true, Checks: []CheckBudget{}, Warnings: []string{}, Limits: []BudgetLine{
		NewBudgetLine("runs", u.Runs, 0, 0, l.Runs), NewBudgetLine("execution_ms", u.ExecutionMS, u.ReservedMS, u.UncertainMS, l.ExecutionMS),
		NewBudgetLine("core_ms", u.CoreMS, 0, 0, l.CoreMS), NewBudgetLine("host_ms", u.HostMS+u.WaitMS, 0, 0, l.HostMS), NewBudgetLine("tokens", u.Tokens, 0, 0, l.Tokens),
	}}
	for i := 3; i <= 4; i++ {
		report.Limits[i].Coverage = u.Host.Coverage
		if u.Host.Coverage == "" {
			report.Limits[i].Coverage = "unreported"
		}
		if u.Host.Coverage != "reported" {
			report.Limits[i].Remaining = nil
		}
	}
	if u.Host.ActiveMS == nil || u.Host.WaitMS == nil {
		report.Limits[3].Actual = nil
	}
	if u.Host.Tokens == nil {
		report.Limits[4].Actual = nil
	}
	for _, c := range t.Plan.Checks {
		if c.Kind != "command" {
			continue
		}
		required := slices.Contains(missing, c.ID)
		item := CheckBudget{CheckID: c.ID, Required: required, TimeoutMS: c.TimeoutMS, Startable: true, Reason: "timeout can be reserved; acceptance and retry guards still apply"}
		if required {
			report.RequiredRuns++
			report.WorstCaseMS += c.TimeoutMS
			report.RetryReserveMS = max(report.RetryReserveMS, c.TimeoutMS)
		}
		if LimitReached(u.Runs, l.Runs) || l.ExecutionMS > 0 && c.TimeoutMS > *report.Limits[1].Remaining || LimitReached(u.CoreMS, l.CoreMS) || LimitReached(u.HostMS+u.WaitMS, l.HostMS) || LimitReached(u.Tokens, l.Tokens) || u.UnavailableHostLimit(l) != "" {
			item.Startable = false
			item.Reason = "an effective limit cannot reserve this check; inspect all limits"
			if required {
				report.Feasible = false
			}
		}
		report.Checks = append(report.Checks, item)
	}
	if l.Runs > 0 && report.RequiredRuns > *report.Limits[0].Remaining {
		report.Feasible = false
		report.Warnings = append(report.Warnings, "Remaining runs cannot cover the required command checks.")
	}
	if l.ExecutionMS > 0 && report.WorstCaseMS+report.RetryReserveMS > *report.Limits[1].Remaining {
		report.Warnings = append(report.Warnings, "Aggregate timeout estimate including one retry exceeds remaining execution time. This is a planning risk, not measured cost or a hard rejection; choose realistic timeouts and bounded retries.")
	}
	if l.CoreMS > 0 && (l.CoreMS < l.ExecutionMS || l.CoreMS < l.HostMS) {
		report.Warnings = append(report.Warnings, "Core wall time overlaps command and Host work; its lower independent cap may become the bottleneck. Review one complete work-time decision and all effective limits.")
	}
	if metric := u.UnavailableHostLimit(l); metric != "" {
		report.Warnings = append(report.Warnings, "The explicit "+metric+" cap needs actual Host usage; new executions cannot claim this limit is enforced while reporting is incomplete.")
	}
	return report
}

type BudgetApproval struct {
	DecisionDigest Digest    `json:"decision_digest" description:"Digest binding the saved user decision to its effective limits."`
	UserRef        string    `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Response       string    `json:"response" description:"场景的完整返回值。"`
	ConfirmedAt    time.Time `json:"confirmed_at" description:"UTC time when this budget decision was saved."`
}
