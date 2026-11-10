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
	Category   string `json:"category,omitempty" jsonschema:"省略返回概要；messages、sources、requirements、notes、plans、checks、verifications、workspace_updates 或 task_updates；指定后默认只列编号和状态，不含正文"`
	AfterSeq   int64  `json:"after_seq,omitempty" jsonschema:"仅列表使用，返回序号更大的记录；不能与 record_id 混用"`
	Limit      int    `json:"limit,omitempty" jsonschema:"仅列表使用，默认 20，最多 100；不能与 record_id 混用"`
	RecordID   string `json:"record_id,omitempty" jsonschema:"单条正文编号，从列表或写入结果取得"`
	Revision   int64  `json:"revision,omitempty" jsonschema:"仅指定 record_id 时使用；需求和方案正文必须指定列表给出的修订号"`
	CharOffset int    `json:"char_offset,omitempty" jsonschema:"仅指定 record_id 时使用，不能用于列表；正文的 Unicode 字符偏移，默认 0，续读用 next_offset"`
	CharLimit  int    `json:"char_limit,omitempty" jsonschema:"仅指定 record_id 时使用，不能用于列表；正文字符数默认 2000，最多 12000；受响应长度上限限制，实际返回内容可能更短"`
}
type finishInput struct {
	Outcome string `json:"outcome,omitempty" jsonschema:"complete 表示请求交付；仅用户明确放弃时用 cancel，并保存本轮用户消息作为中止原因"`
}
type locationInput struct {
	At       string `json:"at" jsonschema:"文档规定的文件、声明、test、flag、cmd、env、route、config 或 dep 位置"`
	Quote    string `json:"quote,omitempty" jsonschema:"来源中的原话，必须与 source_id 同时提供；不引用原话时两项同时省略"`
	SourceID string `json:"source_id,omitempty" jsonschema:"quote 的来源编号，取 messages 或 sources 列表的 id，如 m12；不能使用需求编号 R1；必须与 quote 同时提供"`
	Purpose  string `json:"purpose,omitempty"`
	Change   string `json:"change,omitempty" jsonschema:"add、modify、delete、move 或 rename"`
}
type requirementInput struct {
	ID            string           `json:"id,omitempty"`
	Content       string           `json:"content"`
	Quote         string           `json:"quote,omitempty" jsonschema:"来源中的原话，必须与 source_id 同时提供；无原话依据时两项同时省略，保存为推断需求"`
	SourceID      string           `json:"source_id,omitempty" jsonschema:"quote 的来源编号，先用 task_get 查询 messages 或 sources 列表的 id，如 m12；不能使用需求编号 R1；必须与 quote 同时提供"`
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
	register(server, "task_get", `默认返回简要信息与记录总数。列表只返回编号和状态，不含正文，例如 {"category":"messages","limit":5}；只用 category、after_seq、limit，不传 revision、char_offset、char_limit。再按 records[].read 的 category、record_id 读取单条 JSON 正文，需求和方案还须指定 revision；此时才可用 char_offset、char_limit，不传 after_seq、limit。按 next_offset 续读，拼接完整后再解析。state 表示查询时的状态，已保存的正文不随状态变化而改写。`, false, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input getInput) (any, error) {
		return getTask(ctx, service, actor, input)
	})
	register(server, "task_start", "开始、恢复或重新打开任务；保留原始起点，并记录此时的工作区状态。workflow 只接受 auto（全自动）或 semi（半自动），省略时沿用有效设置。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.StartTaskInput) (any, error) {
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
	register(server, "requirement_save", "保存需求的新修订。quote 与 source_id 必须成对提供，引用须能在完整来源中找到；先用 task_get 查询 messages 或 sources 列表，再按 read 读取原文。source_id 使用来源 id（如 m12），不能使用需求编号 R1；无原话依据时两项同时省略，保存为推断需求，不构成用户授权。implemented_by 中的引用同样成对提供。发生修订冲突时返回最新版本，供合并后重试。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input requirementInput) (any, error) {
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
	register(server, "note_add", `保存 decision 或 next_step。关联需求用 target={kind:"requirement",id:"R1"}，不能把 R1 填入 source_id。实现判断笔记可同时省略 quote 和 source_id；引用原话时两项必须成对提供，source_id 从 task_get 的 messages 或 sources 列表取得（如 m12），按 read 读取原文。对少量可疑实现，可用 decision 指向有效 requirement，以 review 给出对象、判断及代码出处，用 text 说明实际变化、断言或失败条件。用途核对只作提示。用户决定使用 target 的 kind、id、at、effect 和 scope。retain 的 scope 为 task；pause_checks 为 round 或 task；rerun_once 指向 verification，scope 为 once。allow_edit 须依据用户消息，仅允许编辑当前 task 中指定的草稿文件或目录，scope 为 task。revoke 撤回同一对象的决定；撤回 plan 时可省略 at 撤回整版，也可准确列出范围、入口或 dep:模块名。`, true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.NoteInput) (any, error) {
		result, err := service.AddNote(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return noteResponse(result), nil
	})
	register(server, "check_run", "按本次捕获的任务状态和代码快照执行检查。depth 为 static 或 tests；tests 仅为本次请求开启动态检查，refresh 表示重新检查。返回输入版本、分析记录和确认结果，并分别统计无法判断、执行出错和尚未完成的部分。", false, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input check.RunInput) (any, error) {
		result, err := service.Check(ctx, actor, input)
		if err != nil {
			return nil, err
		}
		return checkResponse(result, true), nil
	})
	register(server, "task_finish", "outcome 为 complete 时提交交付请求，立即返回已有检查结果，后续由 Stop 处理交付。仅用户明确放弃任务时使用 cancel，并保存本轮消息作为中止原因。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input finishInput) (any, error) {
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
	register(server, "plan_confirm", "仅在用户明确确认后调用，须结合完整消息核对否定表述和附带修改；version、source_id、quote 指明方案版本及确认原话。确认消息通常晚于方案提交；若提出方案的同一请求已明确要求直接开发，也可使用该请求。用户只讨论方案或要求继续分析时，保持待确认状态。", true, service, unavailable, resolver, func(ctx context.Context, actor ledger.Actor, input ledger.PlanConfirmInput) (any, error) {
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
		response := boundedResponse(name, value, false)
		toolResult = response.result
		if task, ok := value["task_id"].(string); ok && !toolResult.IsError {
			if feedbackErr := service.Store.RecordFeedback(ctx, service.Workspace.ID, task, response.wire); feedbackErr != nil && service.ReportError != nil {
				service.ReportError(feedbackErr)
			}
		}
		if ledger.DebugEnabled(ctx) {
			business, ok := toolResult.StructuredContent.(json.RawMessage)
			if !ok || toolResult.IsError {
				business, _ = json.Marshal(value)
			}
			ledger.DebugRecord(ctx, "mcp.output_size", map[string]any{"business_bytes": len(business), "protocol_bytes": len(response.wire), "estimated_tokens": response.tokens, "budget": outputBudget(name)})
		}
		failure = nil
		service.Background.Schedule(actor)
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
	return boundedResponse("error", result, true).result, nil, nil
}
