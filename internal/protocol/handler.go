package protocol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
)

type Recovery struct {
	Action       string   `json:"action" description:"Concrete next action: correct_request, read_task, self_correct, recover_run, request_authorization, repair_storage or inspect_environment. Preserve actual user authority."`
	Operation    string   `json:"operation" enum:"reuse,replace,read_before_retry,none" description:"How to handle operation_id after this failure. Never automatically replay an uncertain side effect."`
	Message      string   `json:"message" description:"可直接阅读的具体原因，不含敏感原始参数或内部堆栈。"`
	AllowedPaths []string `json:"allowed_paths,omitempty" description:"此错误允许纠正的请求字段路径；不是文件写入授权。"`
}
type Envelope struct {
	OK        bool          `json:"ok" description:"请求是否按接口契约正常处理；true不代表命令检查通过，仍检查run.status。"`
	RequestID string        `json:"request_id" description:"响应身份；变更请求沿用operation_id，只读请求由服务生成。"`
	Tool      string        `json:"tool" description:"执行此请求的公开方法名，原生命令为check.run。"`
	Result    any           `json:"result,omitempty" description:"成功分支的类型化结果；与error互斥。"`
	Error     *domain.Error `json:"error,omitempty" description:"失败分支；不能据失败推定没有任何副作用。"`
	Recovery  *Recovery     `json:"recovery,omitempty" description:"根据实际故障给出的后续处理，不能用它获得新权限。"`
	Outcome   string        `json:"outcome,omitempty" enum:"not_applied,applied,unknown" description:"Semantic commit outcome, separate from control metering. Unknown requires reading saved state."`
	Execution string        `json:"execution,omitempty" enum:"not_started,started,unknown" description:"Known execution phase on native-check failures; does not imply the command passed."`
}
type GetRequest struct {
	TaskID  string `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	Refresh *bool  `json:"refresh,omitempty" description:"Default true; false reads saved state without metering writes or process queries."`
	Detail  bool   `json:"detail,omitempty" description:"Default false. True adds explicit saved assessment, facts, applicability, bindings, evidence, attempts, changes and terminal acceptance; never a writable Task snapshot."`
}
type GetRunRequest struct {
	RunID string `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
}
type Empty struct{}
type Tool struct {
	Name        string
	Description string
	Input       any
	Output      any
	ReadOnly    bool
}

var Catalog = []Tool{
	{"taskbelay_server_info", "读取实际工具和能力。可选 contracts 按名称展开方法说明、输入输出Schema、字段注释与完整样例；查询契约不会执行方法。 只读保存状态，不写计量或查询进程身份。 可重复读取。", ServerInfoRequest{}, ServerInfoResult{}, true},
	{"taskbelay_open_task", "从真实请求在现有工作区创建active Task。无需重复批准计划；本方法不创建分支或运行检查。 更新Core任务、操作与计量；不修改Git，不启动命令。占用目录claims。 同一operation_id和相同请求回读原Task；不同内容拒绝。响应丢失先查已保存Task，不能用新身份重复创建。", application.OpenRequest{}, ContextResult{}, false},
	{"taskbelay_get_task", "读取当前请求和进展。refresh=true观察实际变化；false只读保存状态。detail=true展开公开详情。 refresh=true可能保存新观察并计量；false只读。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", GetRequest{}, TaskResult{}, false},
	{"taskbelay_checkpoint", "记录有意义的进展、局部审查、未解决事项、用户待决定问题和用量。省略保留；concerns按id合并，空数组不删除旧事项。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.CheckpointRequest{}, TaskState{}, false},
	{"taskbelay_resolve_issue", "记录针对当前issue的真实用户答复并清除问题。答复本身不修改授权；目标或限制改变还需revise_task。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.ResolveIssueRequest{}, TaskState{}, false},
	{"taskbelay_revise_task", "仅替换提供的完整authorization或plan；省略保留。普通方法调整沿用原请求；改变目标/限制或扩大上限需真实新决定。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.ReviseRequest{}, TaskState{}, false},
	{"taskbelay_record_evidence", "记录agent声明或已完成的用户人工观察。claim不能满足必需检查，human只用于human检查；不能伪造Core执行。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.EvidenceRequest{}, TaskState{}, false},
	{"taskbelay_get_run", "读取保存的真实运行与输出，不查询进程或重新执行。running仅为保存状态；恢复接口核对实际进程。 只读保存状态。 可重复读取。", GetRunRequest{}, RunDetail{}, true},
	{"taskbelay_recover_run", "恢复匹配收据或分类已停止且无收据的运行，永远不重跑命令。Core及命令组均已停止后，缺收据只能记录interrupted_unknown。 结算原结果/用量，不重执行。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.RecoverRequest{}, RunResult{}, false},
	{"taskbelay_finish_task", "在所有当前目标、检查、审查、事项和运行满足完成条件后结束Task。limitations从已保存事项产生，不接收独立risks。 更新Core任务、操作与计量；不修改Git，不启动命令。active→done，释放claims，保留文件。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.FinishRequest{}, TaskState{}, false},
	{"taskbelay_cancel_task", "明确取消Task并释放claims，保留文件与结果。存在未知执行时先处理原运行。 更新Core任务、操作与计量；不修改Git，不启动命令。active→cancelled并释放claims。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.CancelRequest{}, TaskState{}, false},
	{"taskbelay_list_tasks", "分页读取保存摘要；先过滤后分页，不访问Git。归档默认隐藏。 只读保存状态。 可重复读取；有界页码不是稳定全量快照。", store.ListQuery{}, TaskPage{}, true},
	{"taskbelay_accept_known_failure", "记录用户明确接受的既有回归失败。只有regression可用，失败继续显示failed，不豁免acceptance。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.AcceptFailureRequest{}, TaskState{}, false},
	{"taskbelay_update_budget", "应用已经发生的完整预算决定。0移除该维度上限；work_ms同时更新Core和Host上限，累计消耗保留。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.BudgetRequest{}, TaskState{}, false},
	{"taskbelay_archive_task", "归档或恢复终态Task的可见性，保留文件、执行结果与终态。 更新Core任务、操作与计量；不修改Git，不启动命令。 响应丢失先读保存状态；相同已保存身份与相同内容回读，不重做。revision 冲突读取后换 operation_id；输入被拒且未登记可保留身份修正。", application.ArchiveRequest{}, TaskState{}, false},
}

type Handler struct {
	measureControl func(context.Context) (func(), error)
	service        *application.Service
	version        string
	preferences    userconfig.Preferences
}

func NewHandler(service *application.Service, version string, preferences userconfig.Preferences) *Handler {
	return &Handler{service: service, version: version, preferences: preferences, measureControl: service.MeasureControl}
}

var NativeCheck = Tool{"check.run", "Execute an existing command check once under native Host permissions. Retain the returned view.identity and run_id. ok=true can contain a failed check; inspect run.status. Existing passing results are reused; lost responses require reading/recovering the original run, not another command.", application.RunRequest{}, RunResult{}, false}

var NativePreflights = []Tool{
	{"host-check.pre-file-write", "Preflight actual structured write paths against the current Core claim. Does not judge future semantics or grant native permissions. A deny decision is a successful preflight response, not permission to bypass it.", application.WriteRequest{}, application.WriteDecision{}, true},
	{"host-check.workspace-available", "Read every selected canonical workspace claim. Returns all active_task_id values, not only the first conflict. No claim does not prove a previous open failed. Does not create a Task or grant write authority.", application.WorkspaceRequest{}, application.WorkspaceAvailability{}, true},
}

func (s *Handler) RunPreflight(ctx context.Context, name string, raw []byte) Envelope {
	for _, tool := range NativePreflights {
		if tool.Name == name {
			return s.dispatch(ctx, tool, raw)
		}
	}
	return failure(newID(), name, domain.Invalid("tool", "unknown native preflight"))
}

func (s *Handler) Dispatch(ctx context.Context, name string, raw []byte) Envelope {
	for _, tool := range Catalog {
		if tool.Name == name {
			return s.dispatch(ctx, tool, raw)
		}
	}
	return failure(newID(), name, domain.Invalid("tool", "tool is not in the current catalog"))
}

func (s *Handler) RunCheck(ctx context.Context, raw []byte) Envelope {
	return s.dispatch(ctx, NativeCheck, raw)
}

func savedRead(name string, raw []byte) bool {
	switch name {
	case "taskbelay_server_info", "taskbelay_list_tasks", "taskbelay_get_run", "host-check.pre-file-write", "host-check.workspace-available":
		return true
	case "taskbelay_get_task":
		var q GetRequest
		return Decode(raw, &q) == nil && q.Refresh != nil && !*q.Refresh
	}
	return false
}

func (s *Handler) dispatch(ctx context.Context, tool Tool, raw []byte) (envelope Envelope) {
	name := tool.Name
	readOnly := savedRead(name, raw)
	start := time.Now()
	id := newID()
	var e error
	if !readOnly {
		complete, err := s.measureControl(ctx)
		if err != nil {
			return meteringFailure(id, name, err)
		}
		defer complete()
	}
	taskID := ""
	var identity struct {
		TaskID      string `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
		OperationID string `json:"operation_id" description:"稳定逻辑操作身份；同一已提交身份不能用于不同请求。"`
	}
	_ = json.Unmarshal(raw, &identity)
	taskID = identity.TaskID
	startedTask := taskID
	if taskID != "" && !readOnly {
		_ = s.service.Store.StartCall(ctx, taskID)
	}
	if domain.ID(identity.OperationID).IsValid() {
		id = identity.OperationID
	}
	envelope = Envelope{RequestID: id, Tool: name}
	defer func() {
		if taskID != "" && !readOnly {
			if startedTask == "" {
				_ = s.service.Store.StartCall(context.WithoutCancel(ctx), taskID)
			}
			_ = s.service.Store.AddCost(context.WithoutCancel(ctx), taskID, time.Since(start).Milliseconds(), 0)
		}
	}()
	var result any
	detail := false
	e = nil
	switch name {
	case "host-check.pre-file-write":
		var q application.WriteRequest
		if e = Decode(raw, &q); e == nil {
			if !q.Host.IsValid() || !q.IntentDigest.IsValid() || domain.Text("tool_name", q.ToolName) != nil {
				e = domain.Invalid("arguments", "requires the actual Host, tool_name and intent_digest")
			} else {
				result, e = s.service.PreWrite(ctx, q.RepositoryPath, q.Paths, q.PathParseComplete)
			}
		}
	case "host-check.workspace-available":
		var q application.WorkspaceRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.WorkspaceAvailable(ctx, q)
		}
	case "taskbelay_server_info":
		var q ServerInfoRequest
		if e = Decode(raw, &q); e == nil {
			var info ServerInfoResult
			info, e = s.serverInfo(q)
			if e == nil {
				info.ControlCost, e = s.service.SavedControlCost(ctx)
			}
			result = info
		}
	case "taskbelay_open_task":
		var q application.OpenRequest
		if e = Decode(raw, &q); e == nil {
			var v application.View
			v, e = s.service.Open(ctx, q)
			result = v
			taskID = v.Task.TaskID
		}
	case "taskbelay_get_task":
		var q GetRequest
		if e = Decode(raw, &q); e == nil {
			refresh := q.Refresh == nil || *q.Refresh
			detail = q.Detail
			result, e = s.service.Inspect(ctx, q.TaskID, refresh)
		}
	case "taskbelay_update_budget":
		var q application.BudgetRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.UpdateBudget(ctx, q)
		}
	case "taskbelay_accept_known_failure":
		var q application.AcceptFailureRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.AcceptFailure(ctx, q)
		}
	case "taskbelay_archive_task":
		var q application.ArchiveRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Archive(ctx, q)
		}
	case "taskbelay_resolve_issue":
		var q application.ResolveIssueRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.ResolveIssue(ctx, q)
		}
	case "taskbelay_checkpoint":
		var q application.CheckpointRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Checkpoint(ctx, q)
		}
	case "taskbelay_revise_task":
		var q application.ReviseRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Revise(ctx, q)
		}
	case "check.run":
		var q application.RunRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Execute(ctx, q)
		}
	case "taskbelay_record_evidence":
		var q application.EvidenceRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.RecordEvidence(ctx, q)
		}
	case "taskbelay_get_run":
		var q GetRunRequest
		if e = Decode(raw, &q); e == nil {
			if !domain.ID(q.RunID).IsValid() {
				e = domain.Invalid("run_id", "requires a canonical run identity")
			} else {
				var r execution.Run
				e = s.service.Store.Run(ctx, q.RunID, &r)
				result = runDetail(r)
				taskID = r.TaskID
			}
		}
	case "taskbelay_recover_run":
		var q application.RecoverRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Recover(ctx, q)
		}
	case "taskbelay_finish_task":
		var q application.FinishRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Finish(ctx, q)
		}
	case "taskbelay_cancel_task":
		var q application.CancelRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Cancel(ctx, q)
		}
	case "taskbelay_list_tasks":
		var q store.ListQuery
		if e = Decode(raw, &q); e == nil {
			var page store.TaskPage
			page, e = s.service.Store.List(ctx, q)
			items := []TaskSummary{}
			for _, t := range page.Items {
				items = append(items, Summary(t))
			}
			cost, err := s.service.SavedControlCost(ctx)
			if e == nil {
				e = err
			}
			result = TaskPage{Items: items, Page: page.Page, HasNext: page.HasNext, ControlCost: cost}
		}
	}
	if e != nil {
		return failure(id, name, e)
	}
	switch v := result.(type) {
	case application.View:
		result = taskResult(v, name == "taskbelay_open_task" || name == "taskbelay_get_task", detail)
	case application.RunResult:
		result = runResult(v)
	}
	envelope.OK = true
	envelope.Result = result
	return envelope
}
func newID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "request-unavailable"
	}
	return "request-" + hex.EncodeToString(b)
}
