package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type Goal struct {
	ID        string `json:"id" description:"Stable ID for this authorized user outcome; preserve it while the outcome is unchanged."`
	Statement string `json:"statement" description:"目标或限制的明确内容，不能用实施细节替换用户结果。"`
}
type Constraint struct {
	ID        string   `json:"id" description:"Stable ID for this explicit user restriction."`
	Kind      string   `json:"kind" description:"Restriction category: non_goal, interface, data, dependency or design."`
	Statement string   `json:"statement" description:"目标或限制的明确内容，不能用实施细节替换用户结果。"`
	Paths     []string `json:"paths" description:"事项/约束的实际依赖路径；未声明时按契约保守覆盖全部。"`
}
type Pin struct {
	Path   string `json:"path" description:"Qualified repository-relative path whose exact content the user requires preserving."`
	Digest Digest `json:"digest" description:"Lowercase SHA-256 digest of exact pinned content."`
}
type DependencyPolicy struct {
	Path    string   `json:"path" description:"Qualified path of the dependency manifest governed by this explicit policy."`
	Allowed []string `json:"allowed" description:"Explicitly allowed dependency names; an empty array forbids changes in this manifest."`
}
type Limits struct {
	Runs        int64 `json:"runs,omitempty" description:"Explicit user cap on actual command runs; zero removes this dimension cap. It does not reset recorded usage."`
	ExecutionMS int64 `json:"execution_ms,omitempty" description:"Explicit user cap on cumulative command milliseconds; zero removes this dimension cap. It does not reset recorded usage."`
	CoreMS      int64 `json:"core_ms,omitempty" description:"Explicit user cap on cumulative Core mutation milliseconds; zero removes this dimension cap. It does not reset recorded usage."`
	HostMS      int64 `json:"host_ms,omitempty" description:"用户明确的Host累计时间上限毫秒；0不设上限。"`
	Tokens      int64 `json:"tokens,omitempty" description:"Explicit user cap on cumulative Host tokens; zero removes this dimension cap. It does not reset recorded usage."`
}
type Authorization struct {
	UserRef        string             `json:"user_ref" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	Request        string             `json:"request" description:"用户实际请求文本；工作区准备保留同一原始需求。"`
	Goals          []Goal             `json:"goals" description:"当前用户目标数组；目标ID和statement必须对应真实需求。"`
	Constraints    []Constraint       `json:"constraints,omitempty" description:"用户明确约束；不是预测文件列表。"`
	AllowedPaths   []string           `json:"allowed_paths,omitempty" description:"Explicit user allowlist of repository-relative paths or directory prefixes; omission imposes no additional allowlist. Use key::path across repositories."`
	ForbiddenPaths []string           `json:"forbidden_paths,omitempty" description:"Explicit user-prohibited paths; predictions never override them."`
	Pins           []Pin              `json:"pins,omitempty" description:"Exact user-pinned content digests; omitted means no additional pins."`
	Dependencies   []DependencyPolicy `json:"dependencies,omitempty" description:"Only explicit user restrictions per manifest. An empty allowed list forbids changes there; omission requires technical review instead."`
	Limits         Limits             `json:"limits,omitempty" description:"Actual user-set cumulative caps only. Omitted or zero fields mean no cap; usage is still recorded."`
}
type Check struct {
	ID            string        `json:"id" description:"Stable check ID referenced by run requests, progress gaps and evidence records."`
	Kind          string        `json:"kind" enum:"command,human" description:"command runs an actual argv process; human requires an already completed user observation."`
	Argv          []string      `json:"argv,omitempty" description:"实际执行程序和逐项参数；直接argv优先，需要shell时保留关键命令失败退出码。"`
	RepositoryKey RepositoryKey `json:"repository_key,omitempty" description:"Stable bound repository key; omitted defaults to primary. Supply an actual key for a differently named or multi-repository scope."`
	Cwd           string        `json:"cwd,omitempty" description:"Relative working directory within repository_key; omitted defaults to . and may not escape that repository."`
	Inputs        []string      `json:"inputs,omitempty" description:"参与检查适用性计算的输入路径；缺省[.]覆盖绑定仓库Git可见内容。"`
	InputBasis    string        `json:"input_basis,omitempty" description:"Required for inputs narrower than [.]; explain actual helpers, configuration and dependencies."`
	Environment   []string      `json:"environment,omitempty" description:"Environment variable names whose actual current values participate in the fingerprint; this does not set variables. Executable, platform and cwd are also bound."`
	Conditions    string        `json:"conditions" description:"环境与成立前提的文字说明及适用性输入，不是自动执行的自然语言断言。"`
	ConstraintIDs []string      `json:"constraint_ids,omitempty" description:"Relevant constraint IDs; omission conservatively includes all current constraints."`
	TimeoutMS     int64         `json:"timeout_ms,omitempty" description:"执行超时上限毫秒，同时用于预算预留；缺省60000。"`
	Retry         string        `json:"retry,omitempty" enum:"repeatable,requires_user" description:"Default requires_user for command checks. repeatable permits recovery followed by a justified retry without a new user reference; neither grants native permissions."`
	Rationale     string        `json:"rationale" description:"这项/这次检查如何覆盖具体目标或补充当前缺口。"`
	GoalIDs       []string      `json:"goal_ids" description:"关联当前用户目标的ID数组；check中是唯一目标→检查关联。"`
	Purpose       string        `json:"purpose,omitempty" enum:"acceptance,regression,diagnostic" description:"acceptance/regression/diagnostic；acceptance不可豁免，diagnostic不作为完成必需条件。"`
	FullSuite     bool          `json:"full_suite,omitempty" description:"Whether every execution needs full_suite_basis; omitted means false."`
	Compares      string        `json:"compares,omitempty" description:"Optional ID of another regression command whose existing failure this diagnostic compares."`
}
type Usage struct {
	Runs          int64            `json:"runs" description:"Lifetime count of actual reserved command runs; reading or replanning does not reset it."`
	ExecutionMS   int64            `json:"execution_ms" description:"Settled actual process wall time in milliseconds; reservations and uncertain time are separate."`
	ReservedMS    int64            `json:"reserved_ms" description:"已预留尚未结算的命令时间毫秒。"`
	UncertainMS   int64            `json:"uncertain_ms" description:"未确定执行的保守时间记账，不冒充精确实测。"`
	CoreCalls     int64            `json:"core_calls" description:"累计Core变更调用次数，不等于模型调用次数。"`
	CoreMS        int64            `json:"core_ms" description:"Core变更调用墙钟毫秒，包含执行等待，与其它时间不可直接相加。"`
	ObservationMS int64            `json:"observation_ms" description:"Core仓库观察时间毫秒，可能包含在Core时间内。"`
	HostMS        int64            `json:"-"`
	WaitMS        int64            `json:"-"`
	Tokens        int64            `json:"-"`
	Host          HostUsageSummary `json:"host" description:"Reported Host totals and coverage, separate from measured Core costs. Null means unknown; categories can overlap."`
}
type PathJudgment struct {
	Path        string   `json:"path" description:"Exact current qualified changed path reported in context.repositories.changed_paths."`
	GoalIDs     []string `json:"goal_ids" description:"关联当前用户目标的ID数组；check中是唯一目标→检查关联。"`
	Basis       string   `json:"basis" description:"与实际观察、当前目标或检查关联的具体判断理由。"`
	ScopeDigest Digest   `json:"scope_digest,omitempty" description:"Core计算的判断适用性摘要，只读，输入不得填写。"`
}
type Judgment struct {
	ID          string `json:"id" description:"Actual constraint ID or observed fact ID selected by the containing assessment array."`
	Verdict     string `json:"verdict" description:"satisfied/violation/uncertain; satisfied cannot retain unresolved uncertainty."`
	Basis       string `json:"basis" description:"与实际观察、当前目标或检查关联的具体判断理由。"`
	Uncertainty string `json:"uncertainty" description:"仅用于constraints/facts的uncertain说明；最终review已改用concerns。"`
	ScopeDigest Digest `json:"scope_digest,omitempty" description:"Core计算的判断适用性摘要，只读，输入不得填写。"`
}
type Assessment struct {
	AuthorizationDigest Digest         `json:"authorization_digest" description:"当前授权摘要。"`
	ContentDigest       Digest         `json:"content_digest" description:"当前内容摘要。"`
	Paths               []PathJudgment `json:"paths" description:"Saved path-to-goal judgments, including Core-generated applicability digests."`
	Constraints         []Judgment     `json:"constraints" description:"Saved judgments of explicit user constraints, keyed by constraint ID."`
	Facts               []Judgment     `json:"facts" description:"Observed fact IDs, not IDs from derived decision gaps."`
	Summary             string         `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Review              *DesignReview  `json:"review,omitempty" description:"最终审查；请求仅summary，Core生成适用性摘要。"`
	Concerns            []Concern      `json:"concerns" description:"稳定ID的工程疑问及处理记录；省略或空数组不清除已存事项。"`
}
type Progress struct {
	GoalID     string `json:"goal_id" description:"关联当前用户目标的稳定ID。"`
	Status     string `json:"status" description:"Implementation progress only: planned, doing or done. done does not mean checks passed."`
	Summary    string `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	NextStep   string `json:"next_step,omitempty" description:"下一步可用操作或需要用户完成的动作；不得当成授权。"`
	GoalDigest Digest `json:"goal_digest,omitempty" description:"Core计算的目标适用性摘要，只读。"`
}
type Issue struct {
	Kind    string   `json:"kind" description:"authorization means missing actual authority; tradeoff means an important unresolved user choice."`
	Summary string   `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Options []string `json:"options" description:"Two to eight concrete choices for a real decision, not repeated plan approval."`
}
type Fact struct {
	ID       string `json:"id" description:"Observed fact ID to copy into assessment.facts; it is independent of gap codes."`
	Kind     string `json:"kind" description:"Core observation category, such as dependency, exported declaration, path policy or change magnitude; inspect detail and blocking."`
	Path     string `json:"path" description:"Qualified affected repository path, or the observation scope for an aggregate fact."`
	Detail   string `json:"detail" description:"Concrete observed change or violation requiring correction or a supported semantic review."`
	Blocking bool   `json:"blocking" description:"Whether this deterministic observation prevents work or completion."`
}
type Attempt struct {
	RunID   string    `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
	CheckID string    `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Status  string    `json:"status" description:"Saved actual execution status for this attempt; inspect its run_id for output and recovery."`
	Summary string    `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	At      time.Time `json:"at" description:"实际保存时间，UTC。"`
}
type Evidence struct {
	ID                string    `json:"id" description:"Existing result identity. Core execution uses the run ID; submitted human/claim records use the returned evidence ID."`
	CheckID           string    `json:"check_id" description:"当前已保存检查的稳定身份。"`
	Source            string    `json:"source" description:"结果来源claim/human/core_execution；外部终端文本不能声明core_execution。"`
	Status            string    `json:"status" description:"Saved result: passed, failed, not_run, timeout, interrupted, interrupted_unknown, invalidated or output_limit. Applicable passing results are evaluated separately."`
	CheckDigest       Digest    `json:"check_digest" description:"Digest of the check, associated goals and relevant constraints."`
	InputsDigest      Digest    `json:"inputs_digest" description:"Digest of actual files, missing paths and directory membership."`
	EnvironmentDigest Digest    `json:"environment_digest" description:"Digest of actual executable, cwd, platform, declared variables and conditions."`
	Summary           string    `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	UserRef           string    `json:"user_ref,omitempty" description:"真实用户消息/观察/决定的可定位引用，不能仅填姓名或伪造来源。"`
	RecordedAt        time.Time `json:"recorded_at" description:"Core记录结果的UTC时间。"`
}
type Task struct {
	TaskID         string                 `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	Host           Host                   `json:"host" description:"Actual owning Host: cli, codex, deepseek, claude or zcode. This field does not authenticate user authorization."`
	Status         string                 `json:"status" description:"Saved lifecycle: active, done or cancelled. blocked is derived presentation, not a stored lifecycle."`
	Revision       uint64                 `json:"revision" description:"当前持久化修订。检查预留/结算都可能推进，不能自己加一。"`
	Authorization  Authorization          `json:"authorization" description:"用户的真实请求、目标及明确限制；不得把实施建议混入权限。"`
	Plan           Plan                   `json:"plan" description:"当前实施与验证安排；修订时完整替换提供的plan对象。"`
	BudgetDecision *BudgetApproval        `json:"budget_decision,omitempty" description:"The saved actual user decision that changed limits; null if none."`
	Exceptions     []KnownFailure         `json:"exceptions" description:"Explicit accepted existing regression failures retained for delivery."`
	Archived       bool                   `json:"archived" description:"终态记录是否从默认列表隐藏；不删除、不重开Task。"`
	Repositories   []RepositoryScopeEntry `json:"repositories" description:"按稳定key组织的完整仓库集合；处理各项结果，不只读取第一项。"`
	Assessment     *Assessment            `json:"assessment" description:"公开审查对象，提交新增/受影响片段。"`
	Progress       []Progress             `json:"progress" description:"按goal_id保存的当前实现进展，不是验证结果。"`
	Issue          *Issue                 `json:"issue" description:"Actual unresolved user authorization/tradeoff question; null in checkpoint does not clear it."`
	Evidence       []Evidence             `json:"evidence" description:"Saved observations and execution records; claims do not satisfy checks."`
	Attempts       []Attempt              `json:"attempts" description:"近期真实运行尝试，不能从进展文字生成。"`
	Facts          []Fact                 `json:"facts" description:"Observed fact IDs, not IDs from derived decision gaps."`
	Usage          Usage                  `json:"usage" description:"本Task累计计量及Host报告覆盖；读取不会重置累计值。"`
	Acceptance     []Acceptance           `json:"acceptance" description:"Per-goal conclusions citing every applicable passed acceptance check; regression exceptions cannot replace them."`
	Summary        string                 `json:"summary" description:"对实际变化/观察/结果的具体说明，文字本身不证明执行。"`
	Changes        []Change               `json:"changes" description:"有界保存的实际决定/修订历史。"`
	Limitations    []Limitation           `json:"limitations" description:"由适用not_blocking处理派生的交付限制，保留原因。"`
	CreatedAt      time.Time              `json:"created_at" description:"UTC time when this record was created."`
	UpdatedAt      time.Time              `json:"updated_at" description:"该保存结果的更新时间，UTC。"`
}
type Acceptance struct {
	GoalID      string   `json:"goal_id" description:"关联当前用户目标的稳定ID。"`
	EvidenceIDs []string `json:"evidence_ids" description:"引用已有适用实际结果的ID数组。"`
	Basis       string   `json:"basis" description:"与实际观察、当前目标或检查关联的具体判断理由。"`
}

func (a Authorization) Digest() Digest { return Hash(a) }
func (t Task) ContentDigest() Digest {
	vs := []string{}
	for _, r := range t.Repositories {
		vs = append(vs, string(r.Key)+":"+string(r.Binding.ContentDigest))
	}
	return Hash(vs)
}
func (t Task) Goal(id string) (Goal, bool) {
	for _, g := range t.Authorization.Goals {
		if g.ID == id {
			return g, true
		}
	}
	return Goal{}, false
}
func (t Task) Check(id string) (Check, bool) {
	for _, c := range t.Plan.Checks {
		if c.ID == id {
			return c, true
		}
	}
	return Check{}, false
}
func (t Task) Repository(key RepositoryKey) (RepositoryScopeEntry, bool) {
	if key == "" {
		key = "primary"
	}
	for _, r := range t.Repositories {
		if r.Key == key {
			return r, true
		}
	}
	return RepositoryScopeEntry{}, false
}
func (t Task) CheckDigest(c Check) Digest {
	// Execution settings do not change what an existing result proves.
	c.TimeoutMS, c.Rationale, c.InputBasis, c.Retry, c.FullSuite = 0, "", "", "", false
	goals := []Goal{}
	constraints := []Constraint{}
	for _, g := range t.Authorization.Goals {
		if slices.Contains(c.GoalIDs, g.ID) {
			goals = append(goals, g)
		}
	}
	for _, v := range t.Authorization.Constraints {
		if len(c.ConstraintIDs) == 0 || slices.Contains(c.ConstraintIDs, v.ID) {
			constraints = append(constraints, v)
		}
	}
	return Hash(struct {
		Check       Check
		Goals       []Goal
		Constraints []Constraint
	}{c, goals, constraints})
}
func MatchPath(scope, p string) bool {
	return scope == "." || scope == p || strings.HasPrefix(p, strings.TrimSuffix(scope, "/")+"/")
}
func (a Authorization) Validate() error {
	if e := Text("authorization.user_ref", a.UserRef); e != nil {
		return e
	}
	if e := Text("authorization.request", a.Request); e != nil {
		return e
	}
	if len(a.Goals) == 0 || len(a.Goals) > 64 {
		return Invalid("authorization.goals", "must contain 1..64 goals")
	}
	ids := []string{}
	for i, g := range a.Goals {
		p := fmt.Sprintf("authorization.goals[%d]", i)
		ids = append(ids, g.ID)
		if e := Text(p+".statement", g.Statement); e != nil {
			return e
		}
	}
	if e := IDs("authorization.goals.id", ids); e != nil {
		return e
	}
	ids = nil
	for i, c := range a.Constraints {
		p := fmt.Sprintf("authorization.constraints[%d]", i)
		ids = append(ids, c.ID)
		if !slices.Contains([]string{"non_goal", "interface", "data", "dependency", "design"}, c.Kind) {
			return Invalid(p+".kind", "must be non_goal, interface, data, dependency or design")
		}
		if e := Text(p+".statement", c.Statement); e != nil {
			return e
		}
		if e := Paths(p+".paths", c.Paths); e != nil {
			return e
		}
	}
	if e := IDs("authorization.constraints.id", ids); e != nil {
		return e
	}
	if e := Paths("authorization.allowed_paths", a.AllowedPaths); e != nil {
		return e
	}
	if e := Paths("authorization.forbidden_paths", a.ForbiddenPaths); e != nil {
		return e
	}
	if len(a.Pins) > 64 || len(a.Dependencies) > 64 {
		return Invalid("authorization", "at most 64 pins and dependency policies")
	}
	for i, p := range a.Pins {
		if ScopedPath(p.Path) != nil || !p.Digest.IsValid() {
			return Invalid(fmt.Sprintf("authorization.pins[%d]", i), "requires a repository path and observed SHA-256 digest")
		}
	}
	for i, d := range a.Dependencies {
		if ScopedPath(d.Path) != nil || len(d.Allowed) > 64 {
			return Invalid(fmt.Sprintf("authorization.dependencies[%d]", i), "requires a repository manifest path and at most 64 dependency names")
		}
	}
	l := a.Limits
	if l.Runs < 0 || l.Runs > 10000 || l.ExecutionMS < 0 || l.ExecutionMS > 86400000 || l.CoreMS < 0 || l.HostMS < 0 || l.Tokens < 0 {
		return Invalid("authorization.limits", "limits must be nonnegative; zero means unset, runs <=10000 and execution_ms <=86400000")
	}
	return nil
}
func ScopedPath(v string) error {
	if key, relative, ok := strings.Cut(v, "::"); ok {
		if !RepositoryKey(key).IsValid() {
			return ErrInvalidArgument
		}
		return ValidatePath(relative)
	}
	return ValidatePath(v)
}
func Paths(p string, vs []string) error {
	if len(vs) > 256 {
		return Invalid(p, "at most 256 paths")
	}
	seen := map[string]bool{}
	for i, v := range vs {
		if ScopedPath(v) != nil || seen[v] {
			return Invalid(fmt.Sprintf("%s[%d]", p, i), "must be a unique relative path or directory (use . for the repository)")
		}
		seen[v] = true
	}
	return nil
}
func (t Task) Validate() error {
	if !ID(t.TaskID).IsValid() || !t.Host.IsValid() || t.Revision == 0 || !slices.Contains([]string{"active", "done", "cancelled"}, t.Status) {
		return Invalid("task", "invalid identity, host, revision or lifecycle")
	}
	if e := t.Authorization.Validate(); e != nil {
		return e
	}
	if e := t.ValidatePlan(); e != nil {
		return e
	}
	if len(t.Plan.Checks) == 0 || len(t.Plan.Checks) > 64 || len(t.Repositories) == 0 || len(t.Repositories) > 8 || len(t.Evidence) > 192 || len(t.Attempts) > 64 || len(t.Progress) > 64 {
		return Invalid("task", "current snapshot exceeds its bounded collections")
	}
	ids := []string{}
	for i, c := range t.Plan.Checks {
		if c.Kind == "command" && c.Retry != "repeatable" && c.Retry != "requires_user" {
			return Invalid(fmt.Sprintf("plan.checks[%d].retry", i), "must be repeatable or requires_user")
		}
		p := fmt.Sprintf("plan.checks[%d]", i)
		if !slices.Contains([]string{"acceptance", "regression", "diagnostic"}, c.Purpose) {
			return Invalid(p+".purpose", "requires acceptance, regression or diagnostic")
		}
		if len(c.GoalIDs) == 0 {
			return Invalid(p+".goal_ids", "every check must serve an authorized goal")
		}
		if e := IDs(p+".goal_ids", c.GoalIDs); e != nil {
			return e
		}
		for _, id := range c.GoalIDs {
			if _, ok := t.Goal(id); !ok {
				return Invalid(p+".goal_ids", "goal must exist")
			}
		}
		if c.Compares != "" {
			compared, ok := t.Check(c.Compares)
			if !ok || compared.ID == c.ID || compared.Kind != "command" || compared.Purpose != "regression" || c.Kind != "command" {
				return Invalid(p+".compares", "comparison must reference another regression command")
			}
		}
		ids = append(ids, c.ID)
		if c.Kind != "command" && c.Kind != "human" {
			return Invalid(p+".kind", "must be command or human")
		}
		if _, ok := t.Repository(c.RepositoryKey); !ok {
			return Invalid(p+".repository_key", "must name a bound repository")
		}
		if ValidatePath(c.Cwd) != nil {
			return Invalid(p+".cwd", "must be a relative directory")
		}
		if e := Paths(p+".inputs", c.Inputs); e != nil {
			return e
		}
		for _, input := range c.Inputs {
			if key, _, qualified := strings.Cut(input, "::"); qualified {
				if _, ok := t.Repository(RepositoryKey(key)); !ok {
					return Invalid(p+".inputs", "input repository must be bound to this task")
				}
			}
		}
		if len(c.Inputs) == 0 {
			return Invalid(p+".inputs", "must declare inputs, default .")
		}
		if len(c.Inputs) != 1 || c.Inputs[0] != "." {
			if e := Text(p+".input_basis", c.InputBasis); e != nil {
				return e
			}
		}
		if e := Text(p+".rationale", c.Rationale); e != nil {
			return e
		}
		if e := Text(p+".conditions", c.Conditions); e != nil {
			return e
		}
		if c.Kind == "command" && (len(c.Argv) == 0 || len(c.Argv) > 128 || c.TimeoutMS < 1 || c.TimeoutMS > 3600000) {
			return Invalid(p, "command requires argv and timeout_ms in 1..3600000")
		}
		for j, v := range c.Argv {
			if strings.ContainsRune(v, 0) || len(v) > 8192 || j == 0 && v == "" {
				return Invalid(p+".argv", "invalid command argument")
			}
		}
		if e := IDs(p+".constraint_ids", c.ConstraintIDs); e != nil {
			return e
		}
		for _, id := range c.ConstraintIDs {
			found := false
			for _, v := range t.Authorization.Constraints {
				found = found || v.ID == id
			}
			if !found {
				return Invalid(p+".constraint_ids", "constraint must exist")
			}
		}
		for _, name := range c.Environment {
			if name == "" || strings.ContainsAny(name, "=\x00") {
				return Invalid(p+".environment", "requires environment variable names")
			}
		}
	}
	if e := IDs("plan.checks.id", ids); e != nil {
		return e
	}
	for _, g := range t.Authorization.Goals {
		if len(t.AcceptanceChecks(g.ID)) == 0 {
			return Invalid("plan.checks", "every goal requires at least one non-waivable acceptance check")
		}
	}
	keys := []string{}
	for _, r := range t.Repositories {
		keys = append(keys, string(r.Key))
		if e := r.Validate(); e != nil {
			return Invalid("repositories", "invalid observed binding")
		}
	}
	if e := IDs("repositories.key", keys); e != nil {
		return e
	}
	return nil
}
