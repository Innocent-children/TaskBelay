package mcpserver

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/summary"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func outputBudget(name string) int {
	switch name {
	case "task_get", "check_run":
		return 8000
	case "task_start":
		return 2000
	default:
		return 1000
	}
}

func readLink(task, category, id string, revision int64) map[string]any {
	link := map[string]any{"tool": "task_get", "task_id": task}
	if category != "" {
		link["category"] = category
	}
	if id != "" {
		link["record_id"] = id
	}
	if revision != 0 {
		link["revision"] = revision
	}
	return link
}

func protocolResult(value map[string]any, isError bool) (*mcp.CallToolResult, []byte) {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`{"error":"响应编码失败"}`)
		isError = true
	}
	result := &mcp.CallToolResult{IsError: isError, StructuredContent: json.RawMessage(data), Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}
	wire, _ := json.Marshal(result)
	return result, wire
}

/**
 * 预算覆盖文本与结构化结果的最终序列化，同时为 JSON-RPC 外层预留空间。
 * 身份、状态、总数和续读入口不裁剪；正文缩短后同步续读偏移。
 */
func boundedResponse(name string, value map[string]any, isError bool) *mcp.CallToolResult {
	budget := outputBudget(name) - 64
	for {
		result, wire := protocolResult(value, isError)
		if summary.Tokens(string(wire)) <= budget {
			return result
		}
		value["truncated"] = true
		if body, ok := value["body"].(string); ok && len([]rune(body)) > 1 {
			runes := []rune(body)
			value["body"] = string(runes[:len(runes)/2])
			end := value["char_offset"].(int) + len(runes)/2
			value["char_end"], value["next_offset"] = end, end
			continue
		}
		if records, ok := value["records"].([]any); ok && len(records) > 1 {
			records = records[:len(records)-1]
			value["records"], value["has_more"] = records, true
			value["last_seq"] = records[len(records)-1].(map[string]any)["seq"]
			continue
		}
		if details, ok := value["details"]; ok {
			if reduced, changed := shrink(details); changed {
				value["details"] = reduced
			} else {
				delete(value, "details")
			}
			continue
		}
		if message, ok := value["error"].(string); ok && summary.Tokens(message) > 80 {
			value["error"] = summary.Truncate(message, 80)
			continue
		}
		// 固定元数据只使用程序生成的短身份；不能截断一条无法再缩短的响应。
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "响应元数据超出预算，请缩小查询范围。"}}}
	}
}

func shrink(value any) (any, bool) {
	switch v := value.(type) {
	case string:
		if summary.Tokens(v) > 32 {
			return summary.Truncate(v, max(32, summary.Tokens(v)/2)), true
		}
	case []any:
		if len(v) > 1 {
			return v[:(len(v)+1)/2], true
		}
		if len(v) == 1 {
			next, changed := shrink(v[0])
			return []any{next}, changed
		}
	case map[string]any:
		changed := false
		for key, child := range v {
			var reduced bool
			v[key], reduced = shrink(child)
			changed = changed || reduced
		}
		return v, changed
	}
	return value, false
}

func taskIdentity(task ledger.TaskRecord) map[string]any {
	return map[string]any{"task_id": task.ID, "status": task.State, "workflow": task.Workflow, "revision": task.Revision, "read": readLink(task.ID, "", "", 0)}
}

func requirementResponse(record ledger.Requirement) map[string]any {
	var pending []any
	for _, location := range record.ImplementedBy {
		if location.Pending != "" {
			pending = append(pending, map[string]any{"at": summary.Truncate(location.At, 60), "reason": summary.Truncate(location.Pending, 60)})
		}
	}
	result := map[string]any{"task_id": record.TaskID, "id": record.ID, "revision": record.Revision, "status": record.Status, "pending_locations": len(pending), "read": readLink(record.TaskID, "requirements", record.ID, record.Revision)}
	if record.Citation != nil {
		result["source_id"] = record.Citation.SourceID
	}
	result["details"] = map[string]any{"pending_examples": pending[:min(3, len(pending))]}
	return result
}

func noteResponse(note ledger.Note) map[string]any {
	reason := "已保存普通笔记，不授予检查例外。"
	if note.Active {
		reason = "决定已按来源和目标生效。"
	} else if note.InvalidatedBy != "" {
		reason = "决定已失效：" + note.InvalidatedBy
	} else if note.Consumed {
		reason = "一次性决定已使用。"
	}
	return map[string]any{"task_id": note.TaskID, "id": note.ID, "kind": note.Kind, "saved": true, "effective": note.Active && !note.Consumed, "read": readLink(note.TaskID, "notes", note.ID, 0), "details": map[string]any{"reason": reason}}
}

func planSummary(plan *ledger.PlanRecord) map[string]any {
	if plan == nil {
		return nil
	}
	return map[string]any{"id": plan.ID, "version": plan.Version, "revision": plan.Revision, "confirmed": plan.Confirmation != nil, "revocations": len(plan.Revocations), "read": readLink(plan.TaskID, "plans", plan.ID, plan.Revision), "path": summary.Truncate(plan.Path, 100), "changes": summary.Truncate(strings.Join(plan.Changes, "；"), 100)}
}

func planResponse(result ledger.PlanResult) map[string]any {
	value := map[string]any{"workflow": result.Workflow, "revision": result.View.Revision, "requirements_count": len(result.Requirements)}
	if result.Plan != nil {
		value["task_id"], value["version"], value["confirmed"] = result.Plan.TaskID, result.Plan.Version, result.Plan.Confirmation != nil
		value["read"] = readLink(result.Plan.TaskID, "plans", result.Plan.ID, result.Plan.Revision)
	} else if len(result.Requirements) > 0 {
		value["task_id"] = result.Requirements[0].TaskID
		value["read"] = readLink(result.Requirements[0].TaskID, "", "", 0)
	} else {
		value["read"] = readLink("", "", "", 0)
	}
	value["details"] = map[string]any{"message": result.Message, "plan": planSummary(result.Plan), "effective": planSummary(result.View.Effective), "pending": planSummary(result.View.Pending), "requirements": requirementsSummary(result.Requirements)}
	return value
}

func requirementsSummary(records []ledger.Requirement) []any {
	items := make([]any, 0, min(len(records), 20))
	for _, record := range records[:min(len(records), 20)] {
		items = append(items, map[string]any{"id": record.ID, "revision": record.Revision, "status": record.Status, "summary": summary.Truncate(record.Content, 80)})
	}
	return items
}

func checkResponse(record ledger.CheckRecord, execution bool) map[string]any {
	issues := append([]ledger.Issue(nil), record.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		return (issues[i].Level == "block" || issues[i].BlockOnDelivery) && !(issues[j].Level == "block" || issues[j].BlockOnDelivery)
	})
	var items, gaps, tests []any
	for _, issue := range issues[:min(20, len(issues))] {
		items = append(items, map[string]any{"id": issue.ID, "rule": issue.Rule, "level": issue.Level, "status": issue.Status, "at": summary.Truncate(issue.At, 80), "advice": summary.Truncate(issue.Advice, 100), "facts": summary.Truncate(strings.Join(issue.Facts, "；"), 150)})
	}
	for _, gap := range record.Missing[:min(20, len(record.Missing))] {
		gaps = append(gaps, map[string]any{"at": summary.Truncate(gap.At, 80), "operation": gap.Operation, "kind": gap.Kind, "count": gap.Size(), "candidate": summary.Truncate(gap.Candidate, 80), "reason": summary.Truncate(gap.Reason, 150)})
	}
	added, modified, existing, unknown := 0, 0, 0, 0
	for _, test := range record.Report.Tests {
		switch test.Change {
		case "add":
			added++
		case "modify":
			modified++
		case "existing":
			existing++
			continue
		default:
			unknown++
			continue
		}
		if len(tests) < 10 {
			tests = append(tests, map[string]any{"at": summary.Truncate(test.At, 80), "change": test.Change, "framework": test.Framework})
		}
	}
	value := map[string]any{"task_id": record.Input.TaskID, "id": record.ID, "input_version": record.InputVersion, "status": record.Status, "computed_status": record.ComputedStatus, "level": record.Level, "depth": record.Depth, "started_at": record.StartedAt, "finished_at": record.FinishedAt, "issues_count": len(record.Issues), "missing_count": len(record.Missing), "evaluated_count": len(record.Evaluated), "tests_count": map[string]int{"added": added, "modified": modified, "existing_related": existing, "unknown": unknown}, "read": readLink(record.Input.TaskID, "checks", strconv.FormatInt(record.ID, 10), 0)}
	value["gap_counts"] = record.GapCounts()
	value["unconfirmed_count"] = len(record.Unconfirmed)
	value["details"] = map[string]any{"issues": items, "missing": gaps, "tests": tests}
	if execution {
		value["reused"] = record.Reused
	}
	if record.Static != nil {
		value["static"] = checkResponse(*record.Static, execution)
		// 静态明细也受同一对外预算约束。
		static := value["static"].(map[string]any)
		value["details"].(map[string]any)["static"] = static["details"]
		delete(static, "details")
	}
	return value
}

func finishResponse(result ledger.FinishResult) map[string]any {
	value := taskIdentity(result.Task)
	if result.Request != nil {
		value["delivery_state"] = result.Request.State
		value["check_id"] = result.Request.CheckID
	}
	details := map[string]any{}
	for depth, record := range map[string]*ledger.CheckRecord{"static": result.Static, "tests": result.Dynamic} {
		if record != nil {
			brief := checkResponse(*record, false)
			details[depth] = brief["details"]
			value[depth] = map[string]any{"id": record.ID, "status": record.Status, "level": record.Level, "issues_count": len(record.Issues), "gap_counts": record.GapCounts(), "unconfirmed_count": len(record.Unconfirmed), "read": brief["read"]}
		}
	}
	value["details"] = details
	return value
}
