package protocol

import (
	"slices"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
)

type TaskIdentity struct {
	TaskID   string `json:"task_id" description:"Core Task identity returned by open_task."`
	Revision uint64 `json:"revision" description:"Latest saved revision. Copy it into the next mutation; do not increment it yourself."`
}
type Digests struct {
	Authorization domain.Digest `json:"authorization" description:"Current user authorization digest."`
	Content       domain.Digest `json:"content" description:"Current observed contents digest; saved reads do not refresh it."`
	Plan          domain.Digest `json:"plan" description:"Current implementation and verification plan digest."`
}
type RepositorySummary struct {
	Key          domain.RepositoryKey `json:"key" description:"仓库稳定key，1..128个小写ASCII字母数字及._-，以字母数字开头。"`
	Path         string               `json:"repository_path" description:"当前规范绝对Git工作目录。"`
	Branch       string               `json:"branch" description:"当前实际分支名称；不代表远端已创建同名分支。"`
	ChangedPaths []string             `json:"changed_paths" description:"当前Task观察到的完整限定变化路径集合，不是权限名单。"`
	Upstream     *string              `json:"upstream" description:"Observed task-branch upstream; null means none. It does not replace the original source selection or predict IDE push behavior."`
}
type TaskContext struct {
	Authorization domain.Authorization `json:"authorization" description:"用户的真实请求、目标及明确限制；不得把实施建议混入权限。"`
	Plan          domain.Plan          `json:"plan" description:"当前实施与验证安排；修订时完整替换提供的plan对象。"`
	Repositories  []RepositorySummary  `json:"repositories" description:"按稳定key组织的完整仓库集合；处理各项结果，不只读取第一项。"`
}
type CheckSummary struct {
	ID              string               `json:"id" description:"Current plan check ID."`
	GoalIDs         []string             `json:"goal_ids" description:"关联当前用户目标的ID数组；check中是唯一目标→检查关联。"`
	Purpose         string               `json:"purpose" description:"acceptance/regression/diagnostic；acceptance不可豁免，diagnostic不作为完成必需条件。"`
	Kind            string               `json:"kind" description:"Current check kind: command or human."`
	Status          string               `json:"status" description:"Actual result status, or not_run. Read applicable/refreshed before using it for current acceptance."`
	Source          string               `json:"source" description:"core_execution or human; an agent claim never becomes a passing check."`
	EvidenceID      *string              `json:"evidence_id" description:"该检查摘要采用的实际记录身份；null为没有结果。"`
	RunID           *string              `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
	Applicable      bool                 `json:"applicable" description:"此记录对本次已观察条件是否适用；保存状态读取不保证当前仍适用。"`
	Summary         string               `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Reason          string               `json:"reason" description:"Why the selected saved result applies, is stale, failed or absent."`
	AcceptedFailure *domain.KnownFailure `json:"accepted_failure,omitempty" description:"用户已明确接受的既有回归失败，结果仍为failed。"`
}
type DecisionResult struct {
	Advice        string       `json:"advice" description:"下一类有意义的操作；stop也可能表示终态或未知运行，应结合gaps。"`
	Ready         bool         `json:"ready" description:"仅表示active Task当前满足完成前置条件；终态通常为false。"`
	Gaps          []domain.Gap `json:"gaps" description:"当前未满足条件的具体条目；与finish错误使用同一次计算。"`
	MissingChecks []string     `json:"missing_checks" description:"当前还缺少适用结果的必需检查ID集合。"`
	PendingRuns   []string     `json:"pending_runs" description:"尚不能确定最终结果的原运行ID；不能因此启动另一份命令。"`
}
type TaskDetails struct {
	Facts                 []domain.Fact                 `json:"facts" description:"Actual observed fact IDs and reasons; use these IDs in assessment.facts. Blocking facts cannot be waived by a judgment."`
	EvidenceApplicability []domain.Freshness            `json:"evidence_applicability" description:"Current applicability for every retained evidence record, including older runs; saved reads do not re-observe workspace conditions."`
	Assessment            *domain.Assessment            `json:"assessment" description:"公开审查对象，提交新增/受影响片段。"`
	Repositories          []domain.RepositoryScopeEntry `json:"repositories" description:"按稳定key组织的完整仓库集合；处理各项结果，不只读取第一项。"`
	Evidence              []domain.Evidence             `json:"evidence" description:"Saved observations and execution records; claims do not satisfy checks."`
	Attempts              []domain.Attempt              `json:"attempts" description:"近期真实运行尝试，不能从进展文字生成。"`
	Changes               []domain.Change               `json:"changes" description:"有界保存的实际决定/修订历史。"`
	Acceptance            []domain.Acceptance           `json:"acceptance" description:"Per-goal conclusions citing every applicable passed acceptance check; regression exceptions cannot replace them."`
	Exceptions            []domain.KnownFailure         `json:"exceptions" description:"Explicit accepted existing regression failures retained for delivery."`
	BudgetDecision        *domain.BudgetApproval        `json:"budget_decision" description:"The saved actual user decision that changed limits; null if none."`
	CreatedAt             time.Time                     `json:"created_at" description:"UTC time when this record was created."`
	UpdatedAt             time.Time                     `json:"updated_at" description:"该保存结果的更新时间，UTC。"`
}
type TaskState struct {
	Identity         TaskIdentity          `json:"identity" description:"当前Task身份与revision；后续变更直接沿用。"`
	Host             domain.Host           `json:"host" description:"Actual owning Host: cli, codex, deepseek, claude or zcode. This field does not authenticate user authorization."`
	Status           string                `json:"status" description:"Saved lifecycle: active, done or cancelled. Only Core can end an active Task."`
	Archived         bool                  `json:"archived" description:"终态记录是否从默认列表隐藏；不删除、不重开Task。"`
	Summary          string                `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Digests          Digests               `json:"digests" description:"Core根据当前授权、内容与计划生成的SHA-256摘要。"`
	Decision         DecisionResult        `json:"decision" description:"当前事实推导的控制结果，不是另一份持久化工作流。"`
	Progress         []domain.Progress     `json:"progress" description:"按goal_id保存的当前实现进展，不是验证结果。"`
	Checks           []CheckSummary        `json:"checks" description:"默认返回检查摘要；完整定义在context.plan.checks。"`
	Concerns         []domain.ConcernState `json:"concerns" description:"稳定ID的工程疑问及处理记录；省略或空数组不清除已存事项。"`
	Limitations      []domain.Limitation   `json:"limitations" description:"由适用not_blocking处理派生的交付限制，保留原因。"`
	Issue            *domain.Issue         `json:"issue" description:"Actual unresolved user authorization/tradeoff question; null in checkpoint does not clear it."`
	IssueDigest      domain.Digest         `json:"issue_digest,omitempty" description:"Current issue digest, copied when recording the real user answer."`
	Usage            domain.Usage          `json:"usage" description:"本Task累计计量及Host报告覆盖；读取不会重置累计值。"`
	Budget           domain.BudgetReport   `json:"budget" description:"Current reservation feasibility and explicit limits; timing categories overlap."`
	Refreshed        bool                  `json:"refreshed" description:"是否刚核对实际条件；false只返回保存内容。"`
	Replayed         bool                  `json:"replayed" description:"是否回读既有操作身份的结果；true没有再次执行副作用。"`
	RecordedEvidence *domain.Evidence      `json:"recorded_evidence,omitempty" description:"Record created by record_evidence; claims still do not satisfy checks."`
	ChangedLimits    *domain.Limits        `json:"changed_limits,omitempty" description:"Complete limits saved by update_budget; cumulative usage is retained."`
}
type TaskResult struct {
	TaskState
	SavedSummary TaskSummary  `json:"-"`
	Context      *TaskContext `json:"context,omitempty" description:"Present on open/get. Current request, full plan and repository summaries, without file fingerprints."`
	Details      *TaskDetails `json:"details,omitempty" description:"Present only for get_task detail=true; read-only full records, never a writable Task snapshot."`
}
type ContextResult struct {
	TaskState
	Context TaskContext `json:"context" description:"open/get提供当前请求、完整计划和仓库摘要；普通变更不重复长上下文。"`
}

type RunSummary struct {
	RunID       string `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
	CheckID     string `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Status      string `json:"status" description:"Actual execution state: reserved, running, passed, failed, timeout, interrupted, interrupted_unknown, invalidated or output_limit. ok=true does not imply passed."`
	ExitCode    *int   `json:"exit_code" description:"CLI进程或运行实际退出码；CLI的0不代替run.status。"`
	DurationMS  int64  `json:"duration_ms" description:"真实运行耗时毫秒；未知结果的0值不能解释为没有执行。"`
	OutputBytes int64  `json:"output_bytes" description:"真实输出总字节数，详情通过get_run读取。"`
	Truncated   bool   `json:"truncated" description:"是否只返回输出前缀。"`
}
type RunResult struct {
	View TaskState  `json:"view" description:"原生执行/恢复携带的最新TaskState，不是旧请求快照。"`
	Run  RunSummary `json:"run" description:"本次实际或恢复运行的摘要，必须检查status。"`
}
type RunOutput struct {
	Stdout     string `json:"stdout" description:"实际标准输出文本；CLI展示中为空字符串表示没有输出。"`
	Stderr     string `json:"stderr" description:"独立诊断文本；不与stdout合并后解析JSON。"`
	TotalBytes int64  `json:"total_bytes" description:"原始输出字节数，截断时大于返回前缀长度。"`
	Truncated  bool   `json:"truncated" description:"是否只返回输出前缀。"`
}
type RunFingerprints struct {
	Inputs      domain.Digest `json:"inputs" description:"SHA-256 digest of actual declared inputs observed for this run."`
	Environment domain.Digest `json:"environment" description:"SHA-256 digest of executable, cwd, platform, conditions and declared environment values."`
	Check       domain.Digest `json:"check" description:"当前检查定义及相关要求的摘要。"`
}
type RunDetail struct {
	RunSummary
	TaskID            string                     `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	OperationID       string                     `json:"operation_id" description:"稳定逻辑操作身份；同一已提交身份不能用于不同请求。"`
	StartedAt         time.Time                  `json:"started_at" description:"实际开始时间，UTC RFC3339。"`
	FinishedAt        *time.Time                 `json:"finished_at" description:"实际结束时间，UTC；尚未结束或未知时按类型为null。"`
	Argv              []string                   `json:"argv" description:"实际执行程序和逐项参数；直接argv优先，需要shell时保留关键命令失败退出码。"`
	Cwd               string                     `json:"cwd" description:"Actual absolute directory in which this recorded command was started."`
	Output            RunOutput                  `json:"output" description:"get_run的真实输出内容与截断说明。"`
	Fingerprints      RunFingerprints            `json:"fingerprints" description:"用于核对输入/执行条件的摘要，不是用户提交的正确性证明。"`
	Process           *execution.ProcessIdentity `json:"process" description:"实际进程身份；执行尚未激活或不公开此信息时为null。"`
	Retry             string                     `json:"retry" description:"repeatable/requires_user；仅描述结果未知后的重复条件，不授予新权限。"`
	Reconciliation    string                     `json:"reconciliation" description:"Saved explanation of inspected side effects for an unknown run; empty when no reconciliation was required."`
	ResolutionUserRef string                     `json:"resolution_user_ref" description:"Actual decision for reconciling a non-repeatable run; empty when unnecessary."`
}

/**
 * Mutations return only information needed to continue. Complete context is
 * requested on open/get; full saved records and output have explicit read paths.
 */
func taskResult(v application.View, context, detail bool) TaskResult {
	t := v.Task
	state := TaskState{
		Identity: TaskIdentity{t.TaskID, t.Revision}, Host: t.Host, Status: t.Status, Archived: t.Archived, Summary: t.Summary,
		Digests:  Digests{v.AuthorizationDigest, v.ContentDigest, v.PlanDigest},
		Decision: DecisionResult{v.Decision.Advice, v.Decision.Ready, v.Decision.Gaps, v.Decision.MissingChecks, v.Decision.PendingRuns},
		Progress: t.Progress, Checks: checkSummaries(v), Concerns: t.ConcernStates(v.Decision.Evidence), Limitations: t.DeliveryLimitations(v.Decision.Evidence),
		Issue: t.Issue, IssueDigest: v.IssueDigest, Usage: t.Usage, Budget: v.Budget, Refreshed: v.Refreshed, Replayed: v.Replayed,
	}
	state.RecordedEvidence, state.ChangedLimits = v.RecordedEvidence, v.ChangedLimits
	result := TaskResult{TaskState: state, SavedSummary: Summary(t)}
	if context {
		repositories := []RepositorySummary{}
		for _, r := range t.Repositories {
			paths := []string{}
			for _, entry := range r.Binding.TaskSurface {
				paths = append(paths, t.QualifiedPath(r.Key, entry.Path))
			}
			repositories = append(repositories, RepositorySummary{Key: r.Key, Path: r.Origin.CanonicalWorktreeRoot, Branch: r.Origin.TaskBranch, ChangedPaths: paths, Upstream: r.Binding.BranchUpstream})
		}
		result.Context = &TaskContext{t.Authorization, t.Plan, repositories}
	}
	if detail {
		result.Details = &TaskDetails{t.Facts, v.Decision.Evidence, t.Assessment, t.Repositories, t.Evidence, t.Attempts, t.Changes, t.Acceptance, t.Exceptions, t.BudgetDecision, t.CreatedAt, t.UpdatedAt}
	}
	return result
}
func checkSummaries(v application.View) []CheckSummary {
	result := []CheckSummary{}
	for _, c := range v.Task.Plan.Checks {
		item := CheckSummary{ID: c.ID, GoalIDs: c.GoalIDs, Purpose: c.Purpose, Kind: c.Kind, Status: "not_run", Summary: "No actual result recorded", Reason: "Run the required check, or record an actual user observation for a human check"}
		var chosen *domain.Evidence
		var freshness domain.Freshness
		for _, ev := range v.Task.Evidence {
			if ev.CheckID != c.ID || ev.Source == "claim" {
				continue
			}
			at := slices.IndexFunc(v.Decision.Evidence, func(f domain.Freshness) bool { return f.EvidenceID == ev.ID })
			f := domain.Freshness{}
			if at >= 0 {
				f = v.Decision.Evidence[at]
			}
			if chosen == nil || f.Valid || !freshness.Valid && (f.Applicable || !freshness.Applicable) {
				value := ev
				chosen, freshness = &value, f
			}
		}
		if chosen != nil {
			item.Status, item.Source, item.EvidenceID = chosen.Status, chosen.Source, &chosen.ID
			item.Summary, item.Reason, item.Applicable = chosen.Summary, freshness.Reason, freshness.Applicable
			if chosen.Source == "core_execution" {
				item.RunID = &chosen.ID
			}
		}
		for _, x := range v.Decision.AcceptedFailures {
			if x.CheckID == c.ID {
				value := x
				item.AcceptedFailure = &value
			}
		}
		result = append(result, item)
	}
	return result
}
func runResult(v application.RunResult) RunResult {
	return RunResult{View: taskResult(v.View, false, false).TaskState, Run: summarizeRun(v.Run)}
}
func summarizeRun(r execution.Run) RunSummary {
	return RunSummary{r.RunID, r.CheckID, r.Status, r.ExitCode, r.DurationMS, r.OutputBytes, r.Truncated}
}
func runDetail(r execution.Run) RunDetail {
	return RunDetail{RunSummary: summarizeRun(r), TaskID: r.TaskID, OperationID: r.OperationID, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Argv: r.Check.Argv, Cwd: r.Before.Cwd, Output: RunOutput{r.Stdout, r.Stderr, r.OutputBytes, r.Truncated}, Fingerprints: RunFingerprints{r.Before.Inputs, r.Before.Environment, r.CheckDigest}, Process: r.Process, Retry: r.Check.Retry, Reconciliation: r.Reconciliation, ResolutionUserRef: r.ResolutionUserRef}
}
