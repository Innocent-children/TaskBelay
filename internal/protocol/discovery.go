package protocol

import (
	"encoding/json"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
)

type ServerInfoRequest struct {
	Contracts []string `json:"contracts,omitempty" description:"Optional exact names from tools, auxiliary_contracts, check.run or host-check.pre-file-write/workspace-available. Omission returns compact capabilities; selected contracts include schemas and complete examples without executing them."`
}
type StartupDiagnostics struct {
	CoreReached       bool   `json:"core_reached" description:"This response came from Core; it does not establish Host MCP visibility."`
	SharedContract    string `json:"shared_contract" description:"CLI, MCP and HTTP use the same Core contract."`
	HostMCPVisibility string `json:"host_mcp_visibility" description:"CLI success alone does not prove tools are exposed in the Host."`
}
type HostCoverage struct {
	Host            domain.Host `json:"host" description:"Actual owning Host: cli, codex, deepseek, claude or zcode. This field does not authenticate user authorization."`
	StructuredWrite []string    `json:"structured_write" description:"Registered structured tools; shell writes are observed at checkpoints."`
	Context         string      `json:"context" description:"How the Host receives Task instructions: skill_only, skill, skill_and_host_instructions or activation_instructions. This is not a Task snapshot."`
	Usage           string      `json:"usage" description:"host_reported means explicit Host totals may be submitted; no automatic token callback is claimed."`
	Boundary        string      `json:"boundary" description:"Actual capability limitations; this grants no native permissions."`
}
type NativeCheckContract struct {
	Command     string `json:"command" description:"Native argv entrypoint; supply one JSON request on stdin under actual Host permissions."`
	Contract    string `json:"contract" description:"Request this exact name in contracts, or use check run --help --json, for full input/output schemas and examples."`
	Description string `json:"description" description:"When to use native execution and how to interpret its result."`
}

type ContractExample struct {
	Name          string          `json:"name" description:"方法/对象的公开名称。"`
	Preconditions string          `json:"preconditions" description:"Concrete fixture state and substitutions required for this complete example."`
	Input         json.RawMessage `json:"input" description:"One complete JSON request, with illustrative identities explained by preconditions."`
	Output        json.RawMessage `json:"output" description:"Complete response from the production handler, normalized only for fixture identities."`
}
type MethodContract struct {
	Transport      string            `json:"transport,omitempty" description:"How to supply the request and decode the complete transport result, including exit status or HTTP/MCP errors."`
	Implementation []string          `json:"implementation,omitempty" description:"Maintainer references; callers do not need these files to use this contract."`
	Name           string            `json:"name" description:"方法/对象的公开名称。"`
	Description    string            `json:"description" description:"可直接使用的方法或字段语义说明。"`
	InputSchema    map[string]any    `json:"input_schema" description:"当前方法完整输入Schema，不从内部源码推断。"`
	OutputSchema   map[string]any    `json:"output_schema" description:"当前方法完整成功和失败输出Schema。"`
	ReadOnly       bool              `json:"read_only" description:"Whether the method always reads saved state; get_task depends on refresh."`
	Examples       []ContractExample `json:"examples" description:"同一方法的完整成功、失败场景。"`
}
type ServerInfoResult struct {
	AuxiliaryContracts []string               `json:"auxiliary_contracts" description:"Names of native command and HTTP contracts available through contracts; these are descriptions, not additional MCP tools."`
	StartupDiagnostics StartupDiagnostics     `json:"startup_diagnostics" description:"Core reachability distinct from actual Host tool visibility."`
	Product            string                 `json:"product" description:"产品标识taskbelay。"`
	Version            string                 `json:"version" description:"实际Core产品版本，不表示所有Host发布版本相同。"`
	Schema             string                 `json:"schema" description:"当前唯一数据库Schema版本；不表示兼容历史布局。"`
	Tools              []string               `json:"tools" description:"当前实际暴露的Core方法名数组；不包含原生check.run。"`
	HostPreferences    userconfig.Preferences `json:"host_preferences" description:"四个Host的已解析偏好，不授予工具权限。"`
	Coverage           []HostCoverage         `json:"coverage" description:"Actual per-Host instruction/write-guard/usage capability and its limits."`
	SemanticEvidence   string                 `json:"semantic_evidence" description:"Host judgments cannot prove execution or semantic truth."`
	Cost               string                 `json:"cost" description:"Metering coverage and overlap; unavailable Host usage is not zero."`
	ControlCost        store.ControlUsage     `json:"control_cost" description:"当前数据目录跨Task的Core计量，不能与单Task重复相加。"`
	NativeCheck        NativeCheckContract    `json:"native_check" description:"原生检查命令入口及其独立契约名称。"`
	Contracts          []MethodContract       `json:"contracts,omitempty" description:"按需返回的指定方法契约条目，未请求时省略。"`
}
type TaskSummary struct {
	TaskID         string              `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	RequestSummary string              `json:"request_summary" description:"保存的请求摘要，不是当前实时推理。"`
	Host           domain.Host         `json:"host" description:"Actual owning Host: cli, codex, deepseek, claude or zcode. This field does not authenticate user authorization."`
	Status         string              `json:"status" description:"Saved lifecycle: active, done or cancelled."`
	Lifecycle      string              `json:"lifecycle" description:"展示分类active/blocked/done/cancelled；blocked不新增Task生命周期。"`
	Revision       uint64              `json:"revision" description:"当前持久化修订。检查预留/结算都可能推进，不能自己加一。"`
	UpdatedAt      time.Time           `json:"updated_at" description:"该保存结果的更新时间，UTC。"`
	Archived       bool                `json:"archived" description:"终态记录是否从默认列表隐藏；不删除、不重开Task。"`
	Repositories   []RepositorySummary `json:"repositories" description:"按稳定key组织的完整仓库集合；处理各项结果，不只读取第一项。"`
	Blocker        *string             `json:"blocker" description:"保存摘要中的具体阻碍；null不保证当前工作区无新问题。"`
	Outcome        string              `json:"outcome" description:"Saved terminal summary, or empty before a terminal conclusion. It is not the error-envelope commit outcome."`
}
type TaskPage struct {
	Items       []TaskSummary      `json:"items" description:"当前页的摘要项，空数组是真实空页。"`
	Page        int                `json:"page" description:"当前页码，从1开始。"`
	HasNext     bool               `json:"has_next" description:"过滤后的结果是否还有下一页。"`
	ControlCost store.ControlUsage `json:"control_cost" description:"当前数据目录跨Task的Core计量，不能与单Task重复相加。"`
}

/**
 * Method discovery shares the live catalog and DTO schemas. It reads packaged
 * examples and never opens a Task, starts a process, or grants authorization.
 */
func Describe(name string) (MethodContract, error) {
	for _, tool := range append(append([]Tool{NativeCheck}, NativePreflights...), Catalog...) {
		if tool.Name == name {
			return MethodContract{Implementation: []string{"internal/protocol/handler.go", "internal/protocol/examples_test.go"}, Name: name, Description: tool.Description, InputSchema: Schema(tool.Input), OutputSchema: OutputSchema(tool.Output), ReadOnly: tool.ReadOnly, Examples: methodExamples(name)}, nil
		}
	}
	for _, contract := range auxiliaryContracts() {
		if contract.Name == name {
			return contract, nil
		}
	}
	return MethodContract{}, domain.Invalid("contracts", "unknown method; select a tools or auxiliary_contracts name, check.run or host-check.pre-file-write/workspace-available")
}
func (s *Handler) serverInfo(q ServerInfoRequest) (ServerInfoResult, error) {
	result := ServerInfoResult{
		StartupDiagnostics: StartupDiagnostics{true, "CLI, MCP and HTTP use the same service", "CLI success does not establish MCP availability; inspect tools exposed by the actual Host"},
		Product:            "taskbelay", Version: s.version, Schema: store.DatabaseSchemaVersion, Tools: []string{}, HostPreferences: s.preferences,
		Coverage: []HostCoverage{
			{Host: "codex", StructuredWrite: []string{"apply_patch"}, Context: "skill_and_host_instructions", Usage: "host_reported", Boundary: "Other writes are observed at checkpoints; no sandbox or automatic new-chat permission"},
			{Host: "claude", StructuredWrite: []string{"Write", "Edit", "NotebookEdit"}, Context: "skill", Usage: "host_reported", Boundary: "Other writes are observed at checkpoints; native Host permissions remain authoritative"},
			{Host: "zcode", StructuredWrite: []string{"Write", "Edit"}, Context: "skill_only", Usage: "host_reported", Boundary: "Actual registered Hook capability must be verified; no assumed turn/resume event or token callback"},
			{Host: "deepseek", StructuredWrite: []string{"write", "edit", "str_replace_editor"}, Context: "activation_instructions", Usage: "host_reported", Boundary: "Other writes are observed at checkpoints; existing user authority is retained"},
		},
		SemanticEvidence: "Host judgment with concrete basis; programs do not prove semantic truth. Claims never establish execution.",
		Cost:             "Core mutation timing overlaps execution. Host usage may be unreported or partial; unknown is not zero.",
		NativeCheck:      NativeCheckContract{Command: "check run", Contract: "check.run", Description: NativeCheck.Description},
	}
	for _, tool := range Catalog {
		result.Tools = append(result.Tools, tool.Name)
	}
	for _, contract := range auxiliaryContracts() {
		result.AuxiliaryContracts = append(result.AuxiliaryContracts, contract.Name)
	}
	seen := map[string]bool{}
	for _, name := range q.Contracts {
		if seen[name] {
			return ServerInfoResult{}, domain.Invalid("contracts", "method names must be unique")
		}
		seen[name] = true
		c, err := Describe(name)
		if err != nil {
			return ServerInfoResult{}, err
		}
		result.Contracts = append(result.Contracts, c)
	}
	return result, nil
}
func Summary(t domain.Task) TaskSummary {
	result := TaskSummary{TaskID: t.TaskID, RequestSummary: t.Authorization.Request, Host: t.Host, Status: t.Status, Lifecycle: t.Status, Revision: t.Revision, UpdatedAt: t.UpdatedAt, Archived: t.Archived, Outcome: t.Summary, Repositories: []RepositorySummary{}}
	if t.Status == "active" && t.Issue != nil {
		result.Lifecycle, result.Blocker = "blocked", &t.Issue.Summary
	}
	for _, f := range t.Facts {
		if t.Status == "active" && f.Blocking {
			value := f.Detail
			result.Lifecycle, result.Blocker = "blocked", &value
			break
		}
	}
	for _, r := range t.Repositories {
		result.Repositories = append(result.Repositories, RepositorySummary{Key: r.Key, Path: r.Origin.CanonicalWorktreeRoot, Branch: r.Origin.TaskBranch, ChangedPaths: []string{}, Upstream: r.Binding.BranchUpstream})
	}
	return result
}
