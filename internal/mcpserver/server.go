package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/summary"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ContextResolver func(context.Context, *mcp.CallToolRequest) (ledger.Actor, error)

/**
 * 原生 Agent 到 MCP 的身份映射由接入层提供；身份字段与关联规则见 docs/product-agent-integration.md。
 * 不使用 MCP 连接编号代替 Agent session，也不从最近活动猜测当前轮次。
 */
func MetadataContext(_ context.Context, request *mcp.CallToolRequest) (ledger.Actor, error) {
	value, ok := request.Params.Meta["taskbelay/context"]
	if !ok {
		return ledger.Actor{}, fmt.Errorf("缺少 taskbelay/context；需要接入层提供 Agent、session、轮次和工具调用编号")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ledger.Actor{}, err
	}
	var actor ledger.Actor
	if err := json.Unmarshal(data, &actor); err != nil {
		return actor, err
	}
	return actor, actor.Validate()
}

type getInput struct {
	TaskID     string `json:"task_id,omitempty" jsonschema:"省略时读取当前工作区任务"`
	Category   string `json:"category,omitempty" jsonschema:"省略返回概要；messages、sources、requirements、notes、plans、checks、verifications、workspace_updates 或 task_updates"`
	AfterSeq   int64  `json:"after_seq,omitempty" jsonschema:"列表游标，返回序号更大的记录"`
	Limit      int    `json:"limit,omitempty" jsonschema:"列表条数，默认 20，最多 100"`
	RecordID   string `json:"record_id,omitempty" jsonschema:"单条正文编号，从列表或写入结果取得"`
	Revision   int64  `json:"revision,omitempty" jsonschema:"需求和方案正文必须指定列表给出的修订号"`
	CharOffset int    `json:"char_offset,omitempty" jsonschema:"正文的 Unicode 字符偏移，默认 0"`
	CharLimit  int    `json:"char_limit,omitempty" jsonschema:"正文字符数，默认 2000，最多 12000；最终响应预算可能进一步缩短"`
}
type finishInput struct {
	Outcome string `json:"outcome,omitempty" jsonschema:"complete 请求交付；仅用户明确放弃时用 cancel，中止原因保留本轮用户消息"`
}
type locationInput struct {
	At       string `json:"at" jsonschema:"文档规定的文件、声明、test、flag、cmd、env、route、config 或 dep 位置"`
	Quote    string `json:"quote,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	Change   string `json:"change,omitempty" jsonschema:"add、modify、delete、move 或 rename"`
}
type requirementInput struct {
	ID            string           `json:"id,omitempty"`
	Content       string           `json:"content"`
	Quote         string           `json:"quote,omitempty"`
	SourceID      string           `json:"source_id,omitempty"`
	Acceptance    string           `json:"acceptance"`
	Status        string           `json:"status,omitempty" jsonschema:"pending、in_progress、completed 或 revoked"`
	ImplementedBy *[]locationInput `json:"implemented_by,omitempty" jsonschema:"提供则替换位置列表；省略则保留；空列表清除"`
	BaseRevision  *int64           `json:"base_revision,omitempty" jsonschema:"更新已有需求必须提供读取时的修订号"`
}

func New(service hook.Tasks, unavailable error, resolver ContextResolver) *mcp.Server {
	if resolver == nil {
		resolver = MetadataContext
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "taskbelay", Version: "0.1.0"}, nil)
	register(server, "task_get", "默认返回有限概要与记录总数；category 按 after_seq 分页。record_id 定位完整 JSON 正文，需求和方案同时指定 revision；按 next_offset 继续读取并拼接后解析。state 是查询时状态，正文不随显示状态改写。", false, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input getInput) (any, error) {
		return getTask(ctx, service, actor, input)
	})
	register(server, "task_start", "开始、恢复或重新打开任务；保留原始起点并保存当前恢复观察。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.StartTaskInput) (any, error) {
		result, err := service.Start(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		text, err := service.TaskSummary(ctx, actor, result.ID)
		if err != nil {
			return nil, err
		}
		response := taskIdentity(result)
		response["details"] = map[string]any{"summary": text}
		return response, nil
	})
	register(server, "requirement_save", "保存需求修订；引用须对应完整来源，冲突返回最新版本供合并。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input requirementInput) (any, error) {
		request := ledger.RequirementInput{ID: input.ID, Content: input.Content, Quote: input.Quote, SourceID: input.SourceID, Acceptance: input.Acceptance, Status: input.Status, BaseRevision: input.BaseRevision}
		if input.ImplementedBy != nil {
			locations := make([]ledger.Location, len(*input.ImplementedBy))
			for i, location := range *input.ImplementedBy {
				locations[i] = ledger.Location{At: location.At, Quote: location.Quote, SourceID: location.SourceID, Purpose: location.Purpose, Change: location.Change}
			}
			request.ImplementedBy = &locations
		}
		result, err := service.SaveRequirement(ctx, actor, request)
		if err != nil {
			return nil, err
		}
		return requirementResponse(result), nil
	})
	register(server, "note_add", "保存 decision 或 next_step。决定 target 指定 kind、id、精确 at、effect 和 scope；retain 用 task 期限，pause_checks 用 round/task，rerun_once 用 verification 目标及 once 期限；allow_edit 仅按用户消息授予当前 task 的具体草稿文件或目录，scope 为 task。revoke 撤回同一目标；撤回 plan 时 at 可省略以撤回整版，或精确列出范围、入口、dep:模块名。普通说明不豁免检查。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.NoteInput) (any, error) {
		result, err := service.AddNote(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return noteResponse(result), nil
	})
	register(server, "check_run", "检查固定任务输入；depth 为 static 或 tests，tests 仅对本次请求开启动态检查，refresh 重新核对。返回输入版本、实际覆盖、有限变异证据及未完成部分。", false, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input check.RunInput) (any, error) {
		result, err := service.Check(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return checkResponse(result, true), nil
	})
	register(server, "task_finish", "申请 complete 待交付，立即返回已有检查；Stop 完成交付。用户明确放弃时用 cancel，保存本轮消息作为中止原因。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input finishInput) (any, error) {
		result, err := service.Finish(ctx, actor, ledger.FinishInput{Outcome: input.Outcome})
		if err != nil {
			return nil, err
		}
		return finishResponse(result), nil
	})
	register(server, "plan_save", "半自动提交完整方案及 files、entries、deps；base_revision 使用 task_get 的 plans_revision。已确认范围累积保留，正文应说明原因、成功标准、做法与步骤；返回版本、变更摘要和数据目录路径。全自动仅返回当前模式和需求。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.PlanInput) (any, error) {
		result, err := service.SavePlan(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return planResponse(result), nil
	})
	register(server, "plan_confirm", "仅在用户明确确认后调用，核对完整消息的否定和附带修改；version、source_id、quote 绑定具体版本及原话。通常消息晚于提交；同一请求明确要求按方案直接开发时允许随请求授权。讨论或要求继续分析应保持待确认。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.PlanConfirmInput) (any, error) {
		result, err := service.ConfirmPlan(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return planResponse(result), nil
	})
	return server
}

func register[Input any](server *mcp.Server, name, description string, write bool, service hook.Tasks, unavailable error, resolver ContextResolver, handle func(context.Context, ledger.Actor, Input) (any, error)) {
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, request *mcp.CallToolRequest, input Input) (toolResult *mcp.CallToolResult, result any, protocolErr error) {
		budget := event.MCPBudget
		if name == "check_run" {
			budget = event.StaticBudget
			if options, ok := any(input).(check.RunInput); ok && options.Depth == "tests" {
				budget += event.TestsBudget
			}
		}
		ctx, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		failure := fmt.Errorf("MCP 调用未正常返回，过程记录不完整")
		toolFailure := func(err error) (*mcp.CallToolResult, any, error) {
			failure = err
			if service.ReportError != nil {
				service.ReportError(fmt.Errorf("MCP %s: %w", name, err))
			}
			return toolError(err)
		}
		if unavailable != nil {
			return toolFailure(unavailable)
		}
		if err := service.Store.CheckSchema(ctx); err != nil {
			return toolFailure(err)
		}
		actor, err := resolver(ctx, request)
		var target *string
		switch options := any(input).(type) {
		case getInput:
			if options.TaskID != "" {
				target = &options.TaskID
			}
		case ledger.StartTaskInput:
			target = &options.TaskID
		}
		ctx, trace := service.Store.StartDebug(ctx, service.Workspace.ID, target, actor, "mcp."+name, service.ReportError)
		defer func() { trace.Finish(ctx, toolResult, failure) }()
		ledger.DebugRecord(ctx, "mcp.input", map[string]any{"arguments": input, "identity_resolved": err == nil, "write": write})
		if write && err != nil {
			return toolFailure(err)
		}
		if !write && err != nil {
			actor = ledger.Actor{}
		}
		if actor.Agent != "" {
			if err := service.Store.TouchSession(ctx, service.Workspace.ID, actor); err != nil {
				return toolFailure(err)
			}
		}
		if !write && actor.Agent != "" {
			if err := service.Store.ValidateSession(ctx, service.Workspace.ID, actor); err != nil {
				return toolFailure(err)
			}
		}
		ledger.DebugRecord(ctx, "mcp.handle.start", nil)
		result, err = handle(ctx, actor, input)
		if err != nil {
			return toolFailure(err)
		}
		ledger.DebugRecord(ctx, "mcp.handle.end", nil)
		value, ok := result.(map[string]any)
		if !ok {
			return toolFailure(fmt.Errorf("工具响应不是可分页的对象"))
		}
		toolResult = boundedResponse(name, value, false)
		business, _ := json.Marshal(value)
		wire, _ := json.Marshal(toolResult)
		ledger.DebugRecord(ctx, "mcp.output_size", map[string]any{"business_bytes": len(business), "protocol_bytes": len(wire), "estimated_tokens": summary.Tokens(string(wire)), "budget": outputBudget(name)})
		failure = nil
		return toolResult, nil, nil
	})
}

func toolError(err error) (*mcp.CallToolResult, any, error) {
	result := map[string]any{"error": err.Error()}
	var conflict *ledger.RequirementConflict
	if errors.As(err, &conflict) {
		result["task_id"], result["id"], result["latest_revision"] = conflict.Latest.TaskID, conflict.Latest.ID, conflict.Latest.Revision
		result["read"] = readLink(conflict.Latest.TaskID, "requirements", conflict.Latest.ID, conflict.Latest.Revision)
	}
	var planConflict *ledger.PlanConflict
	if errors.As(err, &planConflict) {
		result["latest_revision"] = planConflict.Latest.Revision
		id := ""
		if len(planConflict.Latest.Versions) > 0 {
			id = planConflict.Latest.Versions[0].TaskID
		}
		result["read"] = readLink(id, "plans", "", 0)
	}
	var taskConflict *ledger.TaskConflict
	if errors.As(err, &taskConflict) {
		result["current_task_id"] = taskConflict.CurrentTaskID
	}
	return boundedResponse("error", result, true), nil, nil
}
