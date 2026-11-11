package mcpserver

import (
	"context"
	"fmt"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/hook"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

func getTask(ctx context.Context, service hook.Tasks, actor ledger.Actor, input getInput) (map[string]any, error) {
	if input.CommandContains != "" && (input.Category != "verifications" || input.RecordID != "") {
		return nil, fmt.Errorf("command_contains 仅用于 category 为 verifications 的列表；读取正文时请移除该参数")
	}
	if input.Category == "" {
		if input.RecordID != "" || input.Revision != 0 || input.AfterSeq != 0 || input.CharOffset != 0 || input.CharLimit != 0 || input.Limit != 0 {
			return nil, fmt.Errorf("分页或正文读取必须指定 category")
		}
		view, err := service.Get(ctx, actor, input.TaskID)
		if err != nil {
			return nil, err
		}
		return taskOverview(ctx, service, view)
	}
	if input.CharOffset < 0 || input.CharLimit < 0 || input.CharLimit > 12000 {
		return nil, fmt.Errorf("char_offset 不能为负；char_limit 为 1 至 12000，默认 2000")
	}
	if input.RecordID == "" && (input.CharOffset != 0 || input.CharLimit != 0 || input.Revision != 0) {
		return nil, fmt.Errorf("revision、char_offset、char_limit 仅用于指定 record_id 的正文读取。请移除正文参数查询列表，再按 records[].read 读取单条正文")
	}
	if input.RecordID != "" && (input.AfterSeq != 0 || input.Limit != 0) {
		return nil, fmt.Errorf("指定 record_id 读取单条正文时不能传 after_seq 或 limit；请移除列表参数，需要限制正文长度时用 char_limit")
	}
	page, err := service.Store.TaskRecords(ctx, service.Workspace.ID, actor, ledger.RecordQuery{
		TaskID: input.TaskID, Category: input.Category, CommandContains: input.CommandContains,
		ID: input.RecordID, Revision: input.Revision, AfterSeq: input.AfterSeq, Limit: input.Limit,
		CharOffset: input.CharOffset, CharLimit: input.CharLimit,
	})
	if err != nil {
		return nil, err
	}
	value := map[string]any{"task_id": page.TaskID, "category": input.Category, "total": page.Total}
	if input.RecordID != "" {
		if len(page.Records) != 1 {
			return nil, fmt.Errorf("指定记录或修订不存在于该任务：%s", input.RecordID)
		}
		record := page.Records[0]
		value["record_id"], value["revision"], value["seq"] = record.ID, record.Revision, record.Seq
		value["state"], value["encoding"] = record.State, "json"
		value["body"], value["char_offset"], value["char_end"], value["char_total"] = record.Body, input.CharOffset, record.CharEnd, record.CharTotal
		value["read"] = readLink(page.TaskID, input.Category, record.ID, record.Revision)
		if record.CharEnd < record.CharTotal {
			value["next_offset"] = record.CharEnd
		} else {
			value["next_offset"] = nil
		}
		return value, nil
	}
	items := make([]any, 0, len(page.Records))
	last := input.AfterSeq
	for _, record := range page.Records {
		item := map[string]any{"seq": record.Seq, "id": record.ID, "revision": record.Revision, "state": record.State, "read": readLink(page.TaskID, input.Category, record.ID, record.Revision)}
		if input.CommandContains != "" {
			item["command_prefix"] = record.CommandPrefix
		}
		items = append(items, item)
		last = record.Seq
	}
	value["records"], value["last_seq"], value["has_more"] = items, last, page.HasMore
	list := readLink(page.TaskID, input.Category, "", 0)
	if input.CommandContains != "" {
		list["command_contains"] = input.CommandContains
	}
	value["read"] = list
	return value, nil
}

func taskOverview(ctx context.Context, service hook.Tasks, view ledger.TaskView) (map[string]any, error) {
	value := map[string]any{"current_task_id": view.CurrentTaskID}
	if view.Task == nil {
		candidates := append([]ledger.TaskRecord(nil), view.Candidates...)
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt) })
		var items []any
		for _, task := range candidates[:min(5, len(candidates))] {
			items = append(items, map[string]any{"task_id": task.ID, "status": task.State, "summary": summary.Truncate(task.Title, 80), "read": readLink(task.ID, "", "", 0)})
		}
		value["status"], value["candidate_count"] = "no_current_task", len(candidates)
		value["select"] = map[string]any{"tool": "task_start", "task_id": "从候选任务选择，或用 title 建立新任务"}
		value["details"] = map[string]any{"candidates": items, "pending_actions": view.PendingActions}
		return value, nil
	}
	value = taskIdentity(*view.Task)
	value["current_task_id"] = view.CurrentTaskID
	counts, err := service.Store.TaskRecordCounts(ctx, service.Workspace.ID, view.Task.ID)
	if err != nil {
		return nil, err
	}
	value["counts"], value["requirements_count"], value["decisions_count"] = counts, len(view.Requirements), len(view.EffectiveDecisions)
	value["plans_revision"], value["latest_check"] = view.Plans.Revision, view.LatestCheck
	value["categories"] = ledger.RecordCategories
	var decisions, updates, checks []any
	for _, note := range view.EffectiveDecisions[:min(10, len(view.EffectiveDecisions))] {
		decisions = append(decisions, map[string]any{"id": note.ID, "summary": summary.Truncate(note.Text, 100), "read": readLink(note.TaskID, "notes", note.ID, 0)})
	}
	for i := len(view.WorkspaceUpdates) - 1; i >= 0 && len(updates) < 10; i-- {
		change := view.WorkspaceUpdates[i]
		updates = append(updates, map[string]any{"path": summary.Truncate(change.Path, 100), "observation_id": change.AfterObservation, "at": change.At, "tool_call_id": change.ToolCallID, "trigger": change.Trigger})
	}
	for _, record := range view.Checks {
		checks = append(checks, checkResponse(record, false))
	}
	var pending []any
	for _, action := range view.PendingActions {
		pending = append(pending, action)
	}
	value["details"] = map[string]any{"title": summary.Truncate(view.Task.Title, 100), "requirements": requirementsSummary(view.Requirements), "plans": map[string]any{"revision": view.Plans.Revision, "effective": planSummary(view.Plans.Effective), "pending": planSummary(view.Plans.Pending)}, "effective_decisions": decisions, "workspace_updates": updates, "checks": checks, "pending_actions": pending, "settings": view.Settings}
	return value, nil
}
