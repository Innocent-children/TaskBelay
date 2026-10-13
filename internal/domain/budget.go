package domain

import "slices"

// BudgetDecision expresses one actual decision, synchronizing overlapping work caps.
type BudgetDecision struct {
	Runs        int64 `json:"runs"`
	ExecutionMS int64 `json:"execution_ms"`
	WorkMS      int64 `json:"work_ms"`
	Tokens      int64 `json:"tokens"`
}

func (b BudgetDecision) Limits() Limits {
	return Limits{Runs: b.Runs, ExecutionMS: b.ExecutionMS, CoreMS: b.WorkMS, HostMS: b.WorkMS, Tokens: b.Tokens}
}
func (l Limits) Within(ceiling Limits) bool {
	return l.Runs <= ceiling.Runs && l.ExecutionMS <= ceiling.ExecutionMS && l.CoreMS <= ceiling.CoreMS && l.HostMS <= ceiling.HostMS && l.Tokens <= ceiling.Tokens
}

type BudgetLine struct {
	Name      string `json:"name"`
	Actual    int64  `json:"actual"`
	Reserved  int64  `json:"reserved"`
	Uncertain int64  `json:"uncertain"`
	Limit     int64  `json:"limit"`
	Remaining int64  `json:"remaining"`
}

func NewBudgetLine(name string, actual, reserved, uncertain, limit int64) BudgetLine {
	return BudgetLine{name, actual, reserved, uncertain, limit, max(int64(0), limit-actual-reserved-uncertain)}
}
func (b BudgetLine) Failure(requested int64) *BudgetFailure {
	return &BudgetFailure{Used: b.Actual + b.Reserved + b.Uncertain, Requested: requested, Limit: b.Limit, Actual: b.Actual, Reserved: b.Reserved, Uncertain: b.Uncertain, Remaining: b.Remaining}
}

type CheckBudget struct {
	CheckID   string `json:"check_id"`
	Required  bool   `json:"required"`
	TimeoutMS int64  `json:"timeout_ms"`
	Startable bool   `json:"startable"`
	Reason    string `json:"reason"`
}
type BudgetReport struct {
	Limits         []BudgetLine  `json:"limits"`
	Checks         []CheckBudget `json:"checks"`
	Feasible       bool          `json:"feasible"`
	RequiredRuns   int64         `json:"required_runs"`
	WorstCaseMS    int64         `json:"worst_case_ms"`
	RetryReserveMS int64         `json:"retry_reserve_ms"`
	Warnings       []string      `json:"warnings"`
}

func (t Task) Budget(missing []string) BudgetReport {
	l, u := t.Authorization.Limits, t.Usage
	report := BudgetReport{Feasible: true, Checks: []CheckBudget{}, Warnings: []string{}, Limits: []BudgetLine{
		NewBudgetLine("runs", u.Runs, 0, 0, l.Runs), NewBudgetLine("execution_ms", u.ExecutionMS, u.ReservedMS, u.UncertainMS, l.ExecutionMS),
		NewBudgetLine("core_ms", u.CoreMS, 0, 0, l.CoreMS), NewBudgetLine("host_ms", u.HostMS+u.WaitMS, 0, 0, l.HostMS), NewBudgetLine("tokens", u.Tokens, 0, 0, l.Tokens),
	}}
	for _, c := range t.Checks {
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
		if report.Limits[0].Remaining < 1 || c.TimeoutMS > report.Limits[1].Remaining || report.Limits[2].Remaining == 0 || report.Limits[3].Remaining == 0 || report.Limits[4].Remaining == 0 {
			item.Startable = false
			item.Reason = "an effective limit cannot reserve this check; inspect all limits"
			if required {
				report.Feasible = false
			}
		}
		report.Checks = append(report.Checks, item)
	}
	if report.RequiredRuns > report.Limits[0].Remaining {
		report.Feasible = false
		report.Warnings = append(report.Warnings, "Remaining runs cannot cover the required command checks.")
	}
	if report.WorstCaseMS+report.RetryReserveMS > report.Limits[1].Remaining {
		report.Warnings = append(report.Warnings, "Aggregate timeout estimate including one retry exceeds remaining execution time. This is a planning risk, not measured cost or a hard rejection; choose realistic timeouts and bounded retries.")
	}
	if l.CoreMS < l.ExecutionMS || l.CoreMS < l.HostMS {
		report.Warnings = append(report.Warnings, "Core wall time overlaps command and Host work; its lower independent cap may become the bottleneck. Review one complete work-time decision and all effective limits.")
	}
	return report
}
