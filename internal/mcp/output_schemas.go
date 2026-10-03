package mcp

import "sort"

// Output schemas describe the public envelope and the paths a Host uses to continue.
// Core domain validation owns the contents of retained task records.
func outputRecord(description string) map[string]any {
	return map[string]any{"type": "object", "description": description, "additionalProperties": true}
}

func outputNullable(schema map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}

func outputActionSchema() map[string]any {
	properties := map[string]any{}
	for _, name := range []string{"task_id", "action_id", "action_kind", "submission_tool", "process_id", "process_definition_digest", "current_node", "node_purpose", "payload_contract", "guidance", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "issued_at"} {
		properties[name] = str()
	}
	properties["revision"] = map[string]any{"type": "integer", "minimum": 1}
	properties["method_profile"] = map[string]any{"enum": []string{"plain", "spec-kit", "openspec"}}
	for _, name := range []string{"entry_conditions", "completion_conditions"} {
		properties[name] = list()
	}
	for _, name := range []string{"allowed_effects", "required_evidence", "method_steps", "available_transitions"} {
		properties[name] = map[string]any{"type": "array"}
	}
	properties["submission_tool"] = map[string]any{"type": "string", "description": "Exact tool for the current Action; empty for an Action without a submission tool."}
	required := make([]string, 0, len(properties))
	for key := range properties {
		required = append(required, key)
	}
	sort.Strings(required)
	return obj(required, properties)
}

func outputTaskSchema() map[string]any {
	schema := outputRecord("Saved Core Task. Read current_action only after handling terminal state, blocker and any enclosing recovery_assessment.")
	schema["required"] = []string{"task_id", "revision", "current_cursor", "current_action", "blocker", "outcome", "baselines", "last_operation"}
	schema["properties"] = map[string]any{
		"task_id": id(), "revision": map[string]any{"type": "integer", "minimum": 1},
		"current_cursor": str(), "resume_cursor": outputNullable(str()),
		"current_action": outputNullable(outputActionSchema()),
		"blocker":        outputNullable(outputRecord("Current blocker and required resolution; null when absent.")),
		"outcome":        outputNullable(outputRecord("Terminal outcome; null for active Tasks.")),
		"last_operation": outputNullable(outputRecord("Compare operation_id and kind to the retained request identity after lifecycle uncertainty.")),
		"baselines": obj([]string{"requirements", "design", "task_plan", "history", "history_total", "history_next_after", "history_revision"}, map[string]any{
			"requirements":       outputNullable(outputRecord("acceptance_criteria is the ordered array addressed by zero-based acceptance_indexes.")),
			"design":             outputNullable(outputRecord("Current design baseline.")),
			"task_plan":          outputNullable(outputRecord("Current work_items and verification_plan.")),
			"history":            map[string]any{"type": []string{"array", "null"}, "maxItems": 32, "description": "First page of the complete saved baseline history, possibly shortened for response bytes; history_total and history_next_after explicitly describe the remaining references."},
			"history_total":      map[string]any{"type": "integer", "minimum": 0, "description": "Exact cumulative number of saved baseline references; no configured retention limit."},
			"history_next_after": outputNullable(map[string]any{"type": "integer", "minimum": 0, "description": "Last sequence sent, or zero for an empty first page; null only at the end."}),
			"history_revision":   map[string]any{"type": "integer", "minimum": 1, "description": "Task revision shared by the first page and explicit history pagination."},
		}),
		"repository":    outputRecord("Primary repository observation, including current_branch and current_head."),
		"verification":  outputRecord("plan, current_budget, usage and adjustments."),
		"test":          outputNullable(outputRecord("Current Test, including eligible evidence_ids.")),
		"evidence":      map[string]any{"type": []string{"array", "null"}, "description": "Saved checks with evidence IDs, names, sources and results."},
		"branch_rename": outputNullable(outputRecord("Prepared rename ID, selected repository, source and target branch, frozen repository/index facts and resume node.")),
		"relocation":    outputNullable(outputRecord("Retained relocation identity and destination/source observations.")),
	}
	return schema
}

func outputRecoverySchema() map[string]any {
	schema := outputRecord("Core recovery assessment. Follow next_advice before executing the returned current Action; operation.action_id identifies the saved submission.")
	schema["required"] = []string{"next_advice", "operation", "task_revision"}
	schema["properties"] = map[string]any{
		"next_advice": str(), "task_revision": map[string]any{"type": "integer"},
		"operation": outputRecord("Saved operation with action_id, operation_id and expected_revision; use these values on a fresh-session resume."),
	}
	return outputNullable(schema)
}

func toolOutputSchema(name string) map[string]any {
	var result map[string]any
	switch name {
	case ToolServerInfo:
		result = outputRecord("Core identity, supported_hosts, supported_processes, method_profiles, tools and host_preferences.")
	case ToolOpenTask:
		result = obj([]string{"created", "task", "recovery_assessment"}, map[string]any{"created": map[string]any{"type": "boolean"}, "task": outputTaskSchema(), "recovery_assessment": outputRecoverySchema()})
	case ToolGetTask:
		result = obj([]string{"task", "recovery_assessment"}, map[string]any{"task": outputTaskSchema(), "recovery_assessment": outputRecoverySchema(), "baseline_history": outputBaselineHistorySchema()})
	case ToolGetNextAction:
		result = outputRecord("Action lookup with action, blocker, outcome and recovery_assessment. Handle recovery before action.")
		result["properties"] = map[string]any{"action": outputNullable(outputActionSchema()), "recovery_assessment": outputRecoverySchema(), "blocker": outputNullable(outputRecord("Current blocker.")), "outcome": outputNullable(outputRecord("Terminal outcome."))}
	case ToolPrepareTaskBranchRename:
		result = obj([]string{"rename_id", "task"}, map[string]any{"rename_id": id(), "task": outputTaskSchema()})
	case ToolPrepareTaskRelocation:
		result = obj([]string{"relocation_id", "task"}, map[string]any{"relocation_id": id(), "task": outputTaskSchema()})
	default:
		result = outputTaskSchema()
	}
	success := obj([]string{"ok", "request_id", "tool", "result"}, map[string]any{
		"ok": map[string]any{"const": true}, "request_id": str(), "tool": map[string]any{"const": name}, "result": result,
	})
	failure := obj([]string{"ok", "request_id", "tool", "error", "recovery"}, map[string]any{
		"ok": map[string]any{"const": false}, "request_id": str(), "tool": map[string]any{"const": name},
		"error": outputErrorSchema(), "recovery": outputFailureRecoverySchema(name),
	})
	return map[string]any{"type": "object", "oneOf": []any{success, failure}}

}

func outputDescription(name string) string {
	switch name {
	case ToolServerInfo:
		return " Success: envelope.result contains server identity and capabilities."
	case ToolOpenTask, ToolGetTask:
		return " Success: result.task and result.recovery_assessment. Process recovery_assessment.next_advice first; saved operation.action_id is available on resume. Then read result.task.current_action."
	case ToolGetNextAction:
		return " Success: result.action, result.blocker, result.outcome and result.recovery_assessment. Follow recovery advice before action; read_next_action consumes the guarded Action in this response without repeated queries."
	case ToolPrepareTaskBranchRename:
		return " Success: result.rename_id and result.task. After response loss, read the retained branch_rename on the same Task. Execute Git only for that prepared identity; recover any saved Action operation before retrying."
	case ToolPrepareTaskRelocation:
		return " Success: result.relocation_id and result.task. After a lost response, read the same Task and its retained relocation; do not prepare another relocation blindly."
	default:
		return " Success: result is the Task itself; the next Action is result.current_action. A terminal Task has no current Action."
	}
}

func outputViolationSchema() map[string]any {
	return obj([]string{"path", "rule", "message"}, map[string]any{"path": str(), "rule": str(), "message": str()})
}
func outputErrorSchema() map[string]any {
	details := map[string]any{"type": "array", "minItems": 1, "items": outputViolationSchema()}
	quantity := map[string]any{"type": "integer", "minimum": 0}
	return obj([]string{"code", "message"}, map[string]any{
		"code": str(), "message": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096, "description": "Specific failed condition or bounded summary of details/guard failures. Unknown internal failures identify the known failing operation without publishing raw diagnostics."}, "details": details,
		"guard":            obj([]string{"guard_id", "failures"}, map[string]any{"guard_id": str(), "failures": details}),
		"repository_paths": list(),
		"budget":           obj([]string{"used", "requested", "limit"}, map[string]any{"used": quantity, "requested": quantity, "limit": quantity}),
	})
}
func outputFailureRecoverySchema(tool string) map[string]any {
	correctionAction := correctRequest
	if _, ordinary := submissionKindForTool(tool); ordinary {
		correctionAction = correctCurrentAction
	}
	correction := obj([]string{"action", "retry_safe", "message", "allowed_paths"}, map[string]any{
		"action": map[string]any{"const": correctionAction}, "retry_safe": map[string]any{"const": true},
		"message": str(), "allowed_paths": map[string]any{"type": "array", "minItems": 1, "items": str()},
	})
	other := obj([]string{"action", "retry_safe", "message"}, map[string]any{
		"action":     map[string]any{"enum": []string{"none", "read_task", "read_next_action", "retry_read", "resolve_blocker", "restore_or_abandon", "use_origin_host", "provision_worktree", "repair_storage", "report_internal_error"}},
		"retry_safe": map[string]any{"const": false}, "message": str(),
	})
	return map[string]any{"oneOf": []any{correction, other}}
}

func outputBaselineHistorySchema() map[string]any {
	counter := map[string]any{"type": "integer", "minimum": 0}
	reference := obj([]string{"kind", "revision", "digest", "summary", "created_at"}, map[string]any{"kind": map[string]any{"enum": []string{"requirements", "design", "task_plan"}}, "revision": map[string]any{"type": "integer", "minimum": 1}, "digest": str(), "summary": str(), "created_at": str()})
	return obj([]string{"task_id", "revision", "total", "next_after", "entries"}, map[string]any{"task_id": id(), "revision": map[string]any{"type": "integer", "minimum": 1}, "total": counter, "next_after": outputNullable(counter), "entries": map[string]any{"type": "array", "maxItems": 32, "items": obj([]string{"sequence", "reference"}, map[string]any{"sequence": map[string]any{"type": "integer", "minimum": 1}, "reference": reference})}})
}
