package mcp

import (
	"encoding/json"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

const (
	ToolServerInfo              = "taskbelay_server_info"
	ToolOpenTask                = "taskbelay_open_task"
	ToolGetTask                 = "taskbelay_get_task"
	ToolGetNextAction           = "taskbelay_get_next_action"
	ToolSubmitRequirements      = "taskbelay_submit_requirements"
	ToolSubmitDesign            = "taskbelay_submit_design"
	ToolSubmitTasks             = "taskbelay_submit_tasks"
	ToolSubmitImplementation    = "taskbelay_submit_implementation"
	ToolSubmitTest              = "taskbelay_submit_test"
	ToolSubmitComprehension     = "taskbelay_submit_comprehension"
	ToolSubmitRefactor          = "taskbelay_submit_refactor"
	ToolSubmitDelivery          = "taskbelay_submit_delivery"
	ToolPrepareTaskBranchRename = "taskbelay_prepare_task_branch_rename"
	ToolPrepareTaskRelocation   = "taskbelay_prepare_task_relocation"
	ToolResolveBlocker          = "taskbelay_resolve_blocker"
	ToolRecoverAction           = "taskbelay_recover_action"
	ToolCancelTask              = "taskbelay_cancel_task"
	ToolAbandonTask             = "taskbelay_abandon_task"
)

var actionSubmissionTools = []struct {
	Name string
	Kind domain.ActionKind
}{
	{ToolSubmitRequirements, domain.ActionCompleteRequirements},
	{ToolSubmitDesign, domain.ActionCompleteDesign},
	{ToolSubmitTasks, domain.ActionCompleteTasks},
	{ToolSubmitImplementation, domain.ActionCompleteImplementation},
	{ToolSubmitTest, domain.ActionCompleteTest},
	{ToolSubmitComprehension, domain.ActionCompleteComprehensionReview},
	{ToolSubmitRefactor, domain.ActionCompleteRefactor},
	{ToolSubmitDelivery, domain.ActionCompleteDelivery},
}

type ToolAnnotations struct{ ReadOnly, Destructive, Idempotent, OpenWorld bool }
type ToolDefinition struct {
	Name, Description string
	InputSchema       json.RawMessage
	OutputSchema      json.RawMessage
	Annotations       ToolAnnotations
}

var catalog = buildCatalog()

func obj(required []string, properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
}
func mergeProperties(left, right map[string]any) map[string]any {
	result := make(map[string]any, len(left)+len(right))
	for key, value := range left {
		result[key] = value
	}
	for key, value := range right {
		result[key] = value
	}
	return result
}
func str() map[string]any    { return map[string]any{"type": "string", "minLength": 1, "maxLength": 4096} }
func id() map[string]any     { return map[string]any{"type": "string", "minLength": 1, "maxLength": 128} }
func digest() map[string]any { return map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"} }
func list() map[string]any {
	return map[string]any{"type": "array", "maxItems": 64, "items": map[string]any{"type": "string", "maxLength": 4096}}
}
func problemClass(values ...string) map[string]any { return map[string]any{"enum": values} }
func nullableSchema(value any) map[string]any {
	return map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
}

// schemaTypeOrder fixes the order of a projected JSON Schema type union so the
// published catalog stays byte-stable across builds.
var schemaTypeOrder = []string{"object", "array", "string", "integer", "number", "boolean", "null"}

// projectableUnion relaxes closed alternative schemas into one closed schema
// that declares an explicit type on every node and carries no composition
// keyword. A Host tool-schema projector that cannot model `anyOf`, `oneOf`, or
// `allOf` replaces every node carrying one with an empty schema, so a
// discriminated union is invisible to the caller even when it is valid JSON
// Schema. The exact per-branch contract remains Core-owned and is reported as
// field-level violations instead of Host-side narrowing.
func projectableUnion(alternatives []any) map[string]any {
	var merged map[string]any
	for _, alternative := range alternatives {
		flattened := flattenSchema(alternative)
		if len(flattened) == 0 {
			continue
		}
		if merged == nil {
			merged = flattened
			continue
		}
		merged = mergeSchema(merged, flattened)
	}
	return merged
}

// flattenSchema removes composition keywords from one schema tree. `anyOf` and
// `oneOf` alternatives are relaxed into their union; `allOf` narrowing is
// dropped because a union cannot carry a per-alternative constraint.
func flattenSchema(value any) map[string]any {
	schema, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	residual := make(map[string]any, len(schema))
	for key, member := range schema {
		switch key {
		case "anyOf", "oneOf", "allOf", "title":
		case "properties":
			residual[key] = flattenSchemaProperties(member)
		case "items":
			residual[key] = flattenSchema(member)
		default:
			residual[key] = member
		}
	}
	normalized := normalizeSchemaLeaf(residual)
	var alternatives []any
	for _, key := range []string{"anyOf", "oneOf"} {
		alternatives = append(alternatives, schemaAlternatives(schema[key])...)
	}
	if len(alternatives) == 0 {
		return normalized
	}
	union := projectableUnion(alternatives)
	if len(normalized) == 0 {
		return union
	}
	return mergeSchema(normalized, union)
}
func flattenSchemaProperties(value any) map[string]any {
	properties, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	out := make(map[string]any, len(properties))
	for name, member := range properties {
		out[name] = flattenSchema(member)
	}
	return out
}
func schemaAlternatives(value any) []any {
	switch items := value.(type) {
	case []any:
		return items
	case []map[string]any:
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return out
	default:
		return nil
	}
}

// normalizeSchemaLeaf rewrites `const` into a single-value `enum` and gives
// every node an explicit type, mirroring what a projector can model.
func normalizeSchemaLeaf(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return schema
	}
	if constant, ok := schema["const"]; ok {
		delete(schema, "const")
		switch value := constant.(type) {
		case string, bool:
			schema["enum"] = []any{value}
		default:
			schema["minimum"], schema["maximum"] = value, value
		}
	}
	if _, ok := schema["type"]; !ok {
		schema["type"] = inferredSchemaType(schema)
	}
	return schema
}
func inferredSchemaType(schema map[string]any) string {
	switch {
	case hasSchemaKey(schema, "properties", "required", "additionalProperties"):
		return "object"
	case hasSchemaKey(schema, "items", "maxItems"):
		return "array"
	case hasSchemaKey(schema, "enum"):
		return enumSchemaType(schema["enum"])
	case hasSchemaKey(schema, "minimum", "maximum"):
		return "integer"
	default:
		return "string"
	}
}
func hasSchemaKey(schema map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := schema[key]; ok {
			return true
		}
	}
	return false
}
func enumSchemaType(value any) string {
	for _, item := range enumValues(value) {
		switch item.(type) {
		case bool:
			return "boolean"
		case string:
			return "string"
		default:
			return "integer"
		}
	}
	return "string"
}
func enumValues(value any) []any {
	switch items := value.(type) {
	case []any:
		return items
	case []string:
		out := make([]any, len(items))
		for i, item := range items {
			out[i] = item
		}
		return out
	default:
		return nil
	}
}

// mergeSchema relaxes two flattened schemas into the smallest closed schema
// that accepts both.
func mergeSchema(left, right map[string]any) map[string]any {
	if isNullSchema(right) {
		return withNullableType(left)
	}
	if isNullSchema(left) {
		return withNullableType(right)
	}
	merged := map[string]any{"type": mergeSchemaTypes(left["type"], right["type"])}
	if properties := unionSchemaProperties(left["properties"], right["properties"]); len(properties) != 0 {
		merged["properties"] = properties
		merged["additionalProperties"] = false
		if required := intersectSchemaRequired(left["required"], right["required"]); len(required) != 0 {
			merged["required"] = required
		}
	}
	if items := mergeSchemaItems(left["items"], right["items"]); items != nil {
		merged["items"] = items
	}
	if values := unionSchemaEnums(left["enum"], right["enum"]); values != nil {
		merged["enum"] = values
	}
	for _, bound := range []struct {
		key     string
		takeMax bool
	}{{"minimum", false}, {"maximum", true}, {"minLength", false}, {"maxLength", true}, {"maxItems", true}} {
		if value, ok := mergeSchemaBound(left[bound.key], right[bound.key], bound.takeMax); ok {
			merged[bound.key] = value
		}
	}
	if pattern, ok := left["pattern"].(string); ok && pattern == right["pattern"] {
		merged["pattern"] = pattern
	}
	return merged
}
func isNullSchema(schema map[string]any) bool {
	return len(schema) == 1 && schema["type"] == "null"
}
func withNullableType(schema map[string]any) map[string]any {
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		out[key] = value
	}
	out["type"] = mergeSchemaTypes(schema["type"], "null")
	return out
}
func mergeSchemaTypes(left, right any) any {
	present := map[string]bool{}
	for _, value := range []any{left, right} {
		for _, name := range schemaStringList(value) {
			present[name] = true
		}
	}
	out := make([]string, 0, len(present))
	for _, name := range schemaTypeOrder {
		if present[name] {
			out = append(out, name)
		}
	}
	if len(out) == 1 {
		return out[0]
	}
	return out
}
func schemaStringList(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}
func unionSchemaProperties(left, right any) map[string]any {
	leftProperties, _ := left.(map[string]any)
	rightProperties, _ := right.(map[string]any)
	if len(leftProperties) == 0 && len(rightProperties) == 0 {
		return nil
	}
	out := make(map[string]any, len(leftProperties)+len(rightProperties))
	for name, value := range leftProperties {
		out[name] = value
	}
	for name, value := range rightProperties {
		existing, ok := out[name].(map[string]any)
		incoming, incomingOK := value.(map[string]any)
		if ok && incomingOK {
			out[name] = mergeSchema(existing, incoming)
			continue
		}
		out[name] = value
	}
	return out
}
func intersectSchemaRequired(left, right any) []string {
	rightNames := map[string]bool{}
	for _, name := range schemaStringList(right) {
		rightNames[name] = true
	}
	var out []string
	for _, name := range schemaStringList(left) {
		if rightNames[name] {
			out = append(out, name)
		}
	}
	return out
}
func mergeSchemaItems(left, right any) map[string]any {
	leftItems, leftOK := left.(map[string]any)
	rightItems, rightOK := right.(map[string]any)
	switch {
	case leftOK && rightOK:
		return mergeSchema(leftItems, rightItems)
	case leftOK:
		return leftItems
	case rightOK:
		return rightItems
	default:
		return nil
	}
}
func unionSchemaEnums(left, right any) []any {
	leftValues, rightValues := enumValues(left), enumValues(right)
	if len(leftValues) == 0 || len(rightValues) == 0 {
		return nil
	}
	out := append([]any(nil), leftValues...)
	for _, value := range rightValues {
		duplicate := false
		for _, existing := range out {
			if existing == value {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, value)
		}
	}
	return out
}
func mergeSchemaBound(left, right any, takeMax bool) (int, bool) {
	leftValue, leftOK := schemaNumber(left)
	rightValue, rightOK := schemaNumber(right)
	if !leftOK || !rightOK {
		return 0, false
	}
	if takeMax == (rightValue > leftValue) {
		return rightValue, true
	}
	return leftValue, true
}
func schemaNumber(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

// A Host tool-schema projector models a bounded keyword subset and enforces a
// small byte budget on the modelled result. Once that budget is exceeded the
// projector discards descriptions, then definition tables, then every complex
// object below its collapse depth, and finally every node carrying a
// composition keyword. The published projection keeps every top-level member,
// type and object closure, and keeps only the enumerations and required sets
// that a caller cannot read from the current Action. Recovery-probe payload
// completeness is checked by Core against the saved operation. The exact
// per-action-kind and per-evidence-source contract stays Core-owned and is
// reported as field-level violations. Large verification-plan and adjustment
// objects are collapsed only inside the nested recovery probe; their ordinary
// submission tools still publish the exact closed members.
var (
	projectedCollapsedPaths = map[string]bool{
		"payload.node_result.baseline.verification_plan": true,
		"payload.node_result.budget_adjustment":          true,
		"payload.node_result.known_failure_acceptance":   true,
	}
	projectedEnumPaths = map[string]bool{
		"host":       true,
		"process_id": true,
		// The discriminant. A caller cannot choose a payload branch without it.
		"action_kind": true,
		// The evidence source drives every other evidence rule.
		"payload.node_result.checks[].source": true,
	}
	projectedRequiredPaths = map[string]bool{
		"": true,
	}
	// Rule text that must survive projection lives in the tool description,
	// which is projected in full and is not charged against the schema budget.
	// Schema descriptions are the first thing a projector discards.
	projectedDescriptions = map[string]string{}
)

// projectForHostBudget applies the published-projection rules above to one
// flattened schema tree.
func projectForHostBudget(schema map[string]any, path string) map[string]any {
	if projectedCollapsedPaths[path] {
		out := map[string]any{}
		for _, key := range []string{"type", "additionalProperties"} {
			if value, ok := schema[key]; ok {
				out[key] = value
			}
		}
		return out
	}
	out := make(map[string]any, len(schema))
	for key, value := range schema {
		switch key {
		case "enum":
			if projectedEnumPaths[path] {
				out[key] = value
			}
		case "required":
			if projectedRequiredPaths[path] {
				out[key] = value
			}
		case "properties":
			members, _ := value.(map[string]any)
			projected := make(map[string]any, len(members))
			for name, member := range members {
				child, ok := member.(map[string]any)
				if !ok {
					continue
				}
				projected[name] = projectForHostBudget(child, joinSchemaPath(path, name))
			}
			out[key] = projected
		case "items":
			item, ok := value.(map[string]any)
			if !ok {
				continue
			}
			out[key] = projectForHostBudget(item, path+"[]")
		default:
			out[key] = value
		}
	}
	if description, ok := projectedDescriptions[path]; ok {
		out["description"] = description
	}
	return out
}
func joinSchemaPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}
func graphPayloads() ([]any, []string) {
	entries := workflow.ActionPayloadSchemas()
	payloads := make([]any, len(entries))
	kinds := make([]string, len(entries))
	for index, entry := range entries {
		payloads[index] = entry.Schema
		kinds[index] = string(entry.Kind)
	}
	return payloads, kinds
}
func buildCatalog() []ToolDefinition {
	newTask := obj([]string{"request", "initial_scope", "initial_out_of_scope", "known_acceptance_criteria", "method_profile"}, map[string]any{"request": map[string]any{"type": "string", "minLength": 1, "maxLength": 8192}, "initial_scope": list(), "initial_out_of_scope": list(), "known_acceptance_criteria": list(), "method_profile": map[string]any{"enum": []string{"plain", "spec-kit", "openspec"}}})
	empty := obj([]string{}, map[string]any{})
	payloads, _ := graphPayloads()
	standardPayload := map[string]any{"oneOf": payloads}
	payload := map[string]any{"anyOf": []any{standardPayload, map[string]any{"type": "null"}}}
	// Recovery probes copy saved operations. Core validates all identity fields;
	// the Host projection retains their types and the three primary required members.
	probe := obj([]string{"operation_id", "action_id", "payload"}, map[string]any{"operation_id": id(), "process_id": map[string]any{"const": "standard-development"}, "process_definition_digest": digest(), "source_cursor": id(), "expected_revision": map[string]any{"type": "integer", "minimum": 1}, "action_id": id(), "action_kind": id(), "repository_binding_digest": digest(), "issuance_identity_digest": digest(), "issuance_history_digest": digest(), "issuance_content_digest": digest(), "payload": projectForHostBudget(projectableUnion([]any{payload, map[string]any{"type": "null"}}), "payload")})
	read := obj([]string{"host", "task_id"}, map[string]any{"host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "operation_probe": projectableUnion([]any{probe, map[string]any{"type": "null"}})})
	repositoryKey := map[string]any{"type": "string", "pattern": "^[a-z0-9][a-z0-9._-]{0,127}$"}
	workspaceOrigin := obj([]string{"mode", "source_type", "carry_changes", "remote_name", "base_branch", "base_commit", "task_branch", "provisioning_receipt_id"}, map[string]any{"mode": map[string]any{"enum": []string{"new_branch", "current_branch", "dedicated_worktree"}}, "source_type": map[string]any{"enum": []string{"local", "remote"}}, "carry_changes": map[string]any{"type": "boolean"}, "remote_name": map[string]any{"type": "string", "maxLength": 128}, "base_branch": str(), "base_commit": map[string]any{"type": "string", "pattern": "^(?:[0-9a-f]{40}|[0-9a-f]{64})$"}, "task_branch": str(), "provisioning_receipt_id": id()})
	additionalRepository := obj([]string{"key", "repository_path", "workspace_origin"}, map[string]any{"key": repositoryKey, "repository_path": str(), "workspace_origin": workspaceOrigin})
	open := obj([]string{"host", "repository_path"}, map[string]any{
		"host":                    map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}},
		"repository_path":         str(),
		"workspace_origin":        workspaceOrigin,
		"primary_repository_key":  repositoryKey,
		"additional_repositories": map[string]any{"type": "array", "maxItems": 7, "items": additionalRepository},
		"new_task":                map[string]any{"anyOf": []any{newTask, map[string]any{"type": "null"}}},
	})
	cancel := obj([]string{"request_id", "host", "task_id", "revision", "reason"}, map[string]any{"request_id": id(), "host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "revision": map[string]any{"type": "integer", "minimum": 1}, "reason": str()})
	defs := map[string]any{"newTask": newTask}
	open["$defs"] = defs
	tools := []ToolDefinition{
		makeTool(ToolServerInfo, "Read the current Core server identity.", empty, true, true, false),
		makeTool(ToolOpenTask, "Open a Task after the Host verifies every confirmed workspace. Default to a new branch in the current directory; current_branch and dedicated_worktree are explicit alternatives. New Tasks require new_task and receipt-backed workspace_origin for every repository. Local modes use the starting HEAD and confirmed initial changes. Resume uses the original directory and omits creation fields. Codex discovers Host preparation with taskbelay-codex host-launch --help. After an uncertain creation, resume that exact directory without new_task.", open, false, false, false),
		makeTool(ToolGetTask, "Read one graph task and any Core-retained recovery assessment. Optionally page its complete baseline-reference history; continue with the returned revision and next_after.", obj([]string{"host", "task_id"}, mergeProperties(read["properties"].(map[string]any), map[string]any{"baseline_history": obj([]string{"revision", "after", "limit"}, map[string]any{"revision": map[string]any{"type": "integer", "minimum": 0}, "after": map[string]any{"type": "integer", "minimum": 0}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": domain.MaxBaselineHistoryPageEntries}})})), true, true, false),
		makeTool(ToolGetNextAction, "Observe the Task worktree, persist any required guard, and return the current graph action.", read, false, true, false),
	}
	for _, entry := range actionSubmissionTools {
		tools = append(tools, makeTool(entry.Name, actionSubmissionDescription(entry.Kind), actionSubmissionSchema(entry.Kind), false, true, false))
	}
	actionReference := obj([]string{"host", "task_id", "action_id"}, map[string]any{"host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "action_id": id()})
	resolveBlocker := obj([]string{"host", "task_id", "action_id"}, map[string]any{
		"host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "action_id": id(),
		"rename_id": id(), "rename_choice": map[string]any{"enum": []string{"complete", "cancel"}}, "choice": map[string]any{"enum": []string{"allow_once", "expand_scope", "reject"}}, "reason": str(), "relocation_id": id(), "relocation_destinations": map[string]any{"type": "array", "maxItems": domain.MaxRepositoryScopeEntries, "items": additionalRepositoryPathSchema(repositoryKey)}, "history_resolution": obj([]string{"choice", "reason"}, map[string]any{"choice": map[string]any{"const": "accept_current_history"}, "reason": str()}),
	})
	lifecycle := obj([]string{"host", "task_id", "revision"}, map[string]any{"host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "revision": map[string]any{"type": "integer", "minimum": 1}})
	abandon := obj([]string{"host", "task_id", "revision", "reason"}, map[string]any{"host": map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}, "task_id": id(), "revision": map[string]any{"type": "integer", "minimum": 1}, "reason": str()})
	tools = append(tools,
		makeTool(ToolPrepareTaskBranchRename, "Prepare one pure branch rename in one active Task repository. Core retains all workspace facts and returns BLOCKED; the authorized Host performs one non-force git branch -m before resolving complete, or cancels only while source facts remain unchanged.", obj([]string{"host", "task_id", "revision", "repository_key", "target_branch", "reason"}, mergeProperties(lifecycle["properties"].(map[string]any), map[string]any{"repository_key": repositoryKey, "target_branch": str(), "reason": str()})), false, true, false),
		makeTool(ToolPrepareTaskRelocation, "Prepare one same-machine Task relocation and retain its exact source workspace state.", lifecycle, false, true, false),
		makeTool(ToolResolveBlocker, "Resolve the current blocker after Core verifies the required repository condition. File-scope blockers also require choice and reason.", resolveBlocker, false, true, false),
		makeTool(ToolRecoverAction, "Recover the Core-retained Action submission without resending its payload.", actionReference, false, true, false),
		makeTool(ToolCancelTask, "Cancel one active Task after user authorization. Generate and retain one request_id for this cancellation, using the current revision and reason. After response loss, get_task and compare last_operation.operation_id/kind and outcome; this is not Action recovery.", cancel, false, false, true),
		makeTool(ToolAbandonTask, "Explicitly abandon a Task whose retained worktree instance is unavailable. Core attempts a repository observation to establish unavailability. After response loss, read the same Task and its outcome; this is not Action recovery.", abandon, false, false, true),
	)
	return tools
}

func additionalRepositoryPathSchema(repositoryKey map[string]any) map[string]any {
	return obj([]string{"key", "repository_path"}, map[string]any{"key": repositoryKey, "repository_path": str()})
}

func actionSubmissionDescription(kind domain.ActionKind) string {
	description := "Submit the result of the current " + string(kind) + " Action. Core fills the complete Action identity, artifact roles, method step identities and payload envelope."
	if kind == domain.ActionCompleteTasks {
		description += " expected_paths supports exact repository-relative paths or a directory followed by /**, not general globs. A multi-repository Task writes every expected path and artifact path as key::relative-path, including the primary repository, which uses primary_repository_key; a single-repository Task uses plain paths. acceptance_indexes are zero-based indexes into current requirements.acceptance_criteria."
	}
	if kind == domain.ActionCompleteTest {
		description += " The Task Plan owns the initial verification budget. A justified verification_budget_increased transition stays in TEST. Automated command_count is 1 to 20; user, static and host_observed use command_count 0 and full_suite false. Every full-suite check records its current reason."
	}
	if kind == domain.ActionCompleteDelivery {
		description += " Submit every current acceptance criterion with completed work_item_ids and passed current Test evidence_ids. Core fills test and comprehension record IDs and aggregate automated and manual evidence IDs from the current Task."
	}
	return description
}

func actionSubmissionSchema(kind domain.ActionKind) map[string]any {
	schema, err := workflow.ActionSubmissionSchema(kind)
	if err != nil {
		panic("missing Action submission schema")
	}
	properties := schema["properties"].(map[string]any)
	properties["host"] = map[string]any{"enum": []string{"codex", "deepseek", "claude", "zcode"}}
	properties["task_id"], properties["action_id"] = id(), id()
	schema["required"] = append([]string{"host", "task_id", "action_id"}, schema["required"].([]string)...)
	return flattenSchema(schema)
}

func submissionKindForTool(name string) (domain.ActionKind, bool) {
	for _, entry := range actionSubmissionTools {
		if entry.Name == name {
			return entry.Kind, true
		}
	}
	return "", false
}

func submissionToolForActionKind(kind domain.ActionKind) (string, bool) {
	for _, entry := range actionSubmissionTools {
		if entry.Kind == kind {
			return entry.Name, true
		}
	}
	if kind == domain.ActionResolveBlocker {
		return ToolResolveBlocker, true
	}
	return "", false
}
func makeTool(name, description string, schema map[string]any, read, idempotent, destructive bool) ToolDefinition {
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	output, err := json.Marshal(toolOutputSchema(name))
	if err != nil {
		panic(err)
	}
	return ToolDefinition{Name: name, Description: description + outputDescription(name), InputSchema: raw, OutputSchema: output, Annotations: ToolAnnotations{ReadOnly: read, Idempotent: idempotent, Destructive: destructive}}
}
func ToolCatalog() []ToolDefinition { return append([]ToolDefinition(nil), catalog...) }
func ToolNames() []string {
	out := make([]string, len(catalog))
	for i, v := range catalog {
		out[i] = v.Name
	}
	return out
}
func isToolName(name string) bool {
	for _, v := range catalog {
		if v.Name == name {
			return true
		}
	}
	return false
}
