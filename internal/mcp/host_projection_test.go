package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

// The Host tool-schema projector models a bounded JSON Schema subset:
// $ref, type, description, enum, items, properties, required,
// additionalProperties, anyOf, oneOf, allOf, $defs and definitions. Every other
// keyword is dropped before the declaration reaches the caller. When the
// modelled schema exceeds the Host compaction budget the projector applies
// increasingly lossy passes: it strips descriptions, drops definition tables,
// replaces complex objects at or below the collapse depth with an empty schema,
// and finally replaces every node carrying a composition keyword with an empty
// schema. The last pass reaches the root, which is exactly how a discriminated
// root union becomes an untyped callable argument.
const (
	hostProjectionBudgetBytes = 6000
	hostProjectionMarginBytes = 64
	hostProjectionCollapse    = 3
)

var hostProjectionKeywords = map[string]bool{
	"$ref": true, "type": true, "description": true, "enum": true, "items": true,
	"properties": true, "required": true, "additionalProperties": true,
	"anyOf": true, "oneOf": true, "allOf": true, "$defs": true, "definitions": true,
}

var hostCompositionKeywords = []string{"anyOf", "oneOf", "allOf"}

func TestHostProjectionUsesWorkflowActionSchemaCatalog(t *testing.T) {
	payloads, kinds := graphPayloads()
	entries := workflow.ActionPayloadSchemas()
	if len(payloads) != len(entries) || len(kinds) != len(entries) {
		t.Fatalf("MCP payloads=%d kinds=%d Workflow entries=%d", len(payloads), len(kinds), len(entries))
	}
	for index, entry := range entries {
		if kinds[index] != string(entry.Kind) || !reflect.DeepEqual(payloads[index], entry.Schema) {
			t.Fatalf("projection[%d] diverged from Workflow kind %s", index, entry.Kind)
		}
	}
}

// modelledHostSchema keeps only the keywords the Host projector can model.
func modelledHostSchema(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, member := range typed {
			if !hostProjectionKeywords[key] {
				continue
			}
			switch key {
			case "properties", "$defs", "definitions":
				table, _ := member.(map[string]any)
				projected := map[string]any{}
				for name, entry := range table {
					projected[name] = modelledHostSchema(entry)
				}
				out[key] = projected
			case "items", "additionalProperties":
				out[key] = modelledHostSchema(member)
			case "anyOf", "oneOf", "allOf":
				out[key] = modelledHostSchema(member)
			default:
				out[key] = member
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, member := range typed {
			out[i] = modelledHostSchema(member)
		}
		return out
	case []map[string]any:
		out := make([]any, len(typed))
		for i, member := range typed {
			out[i] = modelledHostSchema(member)
		}
		return out
	default:
		return value
	}
}

func modelledHostSchemaBytes(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var schema any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(modelledHostSchema(schema))
	if err != nil {
		t.Fatal(err)
	}
	return len(encoded)
}

func toolSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	for _, definition := range ToolCatalog() {
		if definition.Name != name {
			continue
		}
		var schema map[string]any
		if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		return schema
	}
	t.Fatalf("tool %s is not in the catalog", name)
	return nil
}

// TestApplyActionSchemaIsHostProjectable is the Host projection contract. It
// fails for any apply schema the Host would reduce to an untyped argument.
func TestActionSubmissionSchemasAreHostProjectable(t *testing.T) {
	for _, entry := range actionSubmissionTools {
		schema := toolSchema(t, entry.Name)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("%s root=%#v", entry.Name, schema)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok || len(properties) != 9 {
			t.Fatalf("%s properties=%#v", entry.Name, schema["properties"])
		}
		for _, name := range []string{"host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"} {
			if _, present := properties[name]; !present {
				t.Fatalf("%s cannot see %s", entry.Name, name)
			}
		}
		assertHostProjectable(t, schema, "$")
	}
}

// TestApplyActionSchemaFitsHostProjectionBudget keeps the published schema below
// the Host compaction budget so no lossy pass runs and nested structure at or
// below the collapse depth stays visible.
func TestApplyActionSchemaFitsHostProjectionBudget(t *testing.T) {
	for _, name := range ToolNames() {
		for _, definition := range ToolCatalog() {
			if definition.Name != name {
				continue
			}
			size := modelledHostSchemaBytes(t, definition.InputSchema)
			if size > hostProjectionBudgetBytes-hostProjectionMarginBytes {
				t.Fatalf("%s modelled schema is %d bytes; the Host compaction budget is %d bytes and this feature reserves a %d byte margin", name, size, hostProjectionBudgetBytes, hostProjectionMarginBytes)
			}
			t.Logf("%s modelled schema = %d bytes", name, size)
		}
	}
}

func TestReadToolProjectionPreservesRuntimeGuards(t *testing.T) {
	for _, tool := range []string{ToolGetTask, ToolGetNextAction} {
		t.Run(tool, func(t *testing.T) {
			schema := toolSchema(t, tool)
			assertHostProjectable(t, schema, "$")
			if size := modelledHostSchemaBytes(t, mustSchemaJSON(t, schema)); size > hostProjectionBudgetBytes-hostProjectionMarginBytes {
				t.Fatalf("read-tool Host schema is %d bytes", size)
			}
			if !reflect.DeepEqual(stringSlice(schema["required"]), []string{"host", "task_id"}) {
				t.Fatalf("read-tool required=%#v", schema["required"])
			}
			probe := childSchema(t, schema, "operation_probe")
			if probe["additionalProperties"] != false || len(stringSlice(probe["required"])) != 0 {
				t.Fatalf("recovery probe is not closed or still publishes repeated required members: %#v", probe)
			}
			for _, name := range []string{"operation_id", "process_id", "process_definition_digest", "source_cursor", "expected_revision", "action_id", "action_kind", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "payload"} {
				if _, ok := probe["properties"].(map[string]any)[name]; !ok {
					t.Fatalf("recovery probe lost %s", name)
				}
			}
			payload := childSchema(t, probe, "payload")
			checks := childSchema(t, childSchema(t, payload, "node_result"), "checks")
			item, ok := checks["items"].(map[string]any)
			if !ok {
				t.Fatal("recovery check items lost their type")
			}
			source := childSchema(t, item, "source")
			if source["type"] != "string" || source["enum"] != nil {
				t.Fatalf("recovery source type/enum=%#v", source)
			}
			result := childSchema(t, payload, "node_result")
			baseline := childSchema(t, result, "baseline")
			for name, opaque := range map[string]map[string]any{
				"verification_plan":        childSchema(t, baseline, "verification_plan"),
				"budget_adjustment":        childSchema(t, result, "budget_adjustment"),
				"known_failure_acceptance": childSchema(t, result, "known_failure_acceptance"),
			} {
				if len(opaque) != 1 || !hasSchemaType(opaque["type"], "object") {
					t.Fatalf("%s must be a typed open transport hint: %#v", name, opaque)
				}
			}
			if tool == ToolGetTask {
				history := childSchema(t, schema, "baseline_history")
				if history["additionalProperties"] != false || !reflect.DeepEqual(stringSlice(history["required"]), []string{"revision", "after", "limit"}) {
					t.Fatalf("baseline history lost its closed paging contract: %#v", history)
				}
				for _, name := range []string{"revision", "after", "limit"} {
					if childSchema(t, history, name)["type"] != "integer" {
						t.Fatalf("baseline history %s lost integer type", name)
					}
				}
			}

			for _, name := range []string{"operation_id", "action_id", "payload"} {
				probeInput := map[string]any{"operation_id": "operation", "action_id": "action", "payload": nil}
				delete(probeInput, name)
				input := map[string]any{"host": "codex", "task_id": "task", "operation_probe": probeInput}
				requestID := "request-missing-" + name
				actual := (&Server{}).dispatch(context.Background(), tool, domain.ID(requestID), mustSchemaJSON(t, input))
				want := EncodeError(requestID, tool, domain.InvalidArgumentViolations(domain.Violation("operation_probe."+name, domain.RuleRequiredMemberMissing)))
				if !actual.IsError || !bytes.Equal(actual.JSON, want.JSON) {
					t.Fatalf("missing %s response=%s want=%s", name, actual.JSON, want.JSON)
				}
			}
			probeInput := map[string]any{"operation_id": "operation", "action_id": "action", "payload": nil, "extra": "private-value"}
			input := map[string]any{"host": "codex", "task_id": "task", "operation_probe": probeInput}
			actual := (&Server{}).dispatch(context.Background(), tool, "request-closed-probe", mustSchemaJSON(t, input))
			want := EncodeError("request-closed-probe", tool, domain.InvalidArgumentViolations(domain.Violation("operation_probe.extra", domain.RuleUnknownMember)))
			if !actual.IsError || !bytes.Equal(actual.JSON, want.JSON) || bytes.Contains(actual.JSON, []byte("private-value")) {
				t.Fatalf("closed recovery response=%s want=%s", actual.JSON, want.JSON)
			}
			assertCompleteRecoveryProbePayloads(t, tool, schema)
		})
	}
}

func hasSchemaType(value any, name string) bool {
	for _, item := range schemaStringList(value) {
		if item == name {
			return true
		}
	}
	return false
}

// The public probe schema must transport complete saved payloads while the
// private validator still rejects malformed members inside all three opaque
// transport hints.
func assertCompleteRecoveryProbePayloads(t *testing.T, tool string, schema map[string]any) {
	t.Helper()
	fixtures := []struct {
		name, nestedPath, requiredName, invalidName string
		invalidValue                                any
		payload                                     map[string]any
		cursor                                      domain.NodeID
	}{
		{"verification_plan", "node_result.baseline.verification_plan", "checks", "initial_budget.level", "invalid", recoveryTasksPayload(t), domain.NodeTasks},
		{"budget_adjustment", "node_result.budget_adjustment", "basis", "basis", "invalid", recoveryBudgetPayload(t), domain.NodeTest},
		{"known_failure_acceptance", "node_result.known_failure_acceptance", "failed_checks", "source", "automated", recoveryKnownFailurePayload(t), domain.NodeTest},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			input := recoveryReadInput(t, fixture.cursor, fixture.payload)
			raw := mustSchemaJSON(t, input)
			if violations := workflow.RequestStructureViolations("", raw, schema); len(violations) != 0 {
				t.Fatalf("public transport rejects complete payload: %#v", violations)
			}
			if err := ValidateToolInput(tool, raw); err != nil {
				t.Fatalf("private validation rejects complete payload: %v", err)
			}
			for _, mutation := range []struct {
				name, member string
				change       func(map[string]any)
				rule         domain.ViolationRule
			}{
				{"unknown", "unexpected", func(nested map[string]any) { nested["unexpected"] = "private-value" }, domain.RuleUnknownMember},
				{"missing", fixture.requiredName, func(nested map[string]any) { delete(nested, fixture.requiredName) }, domain.RuleRequiredMemberMissing},
				{"invalid", fixture.invalidName, func(nested map[string]any) { setNestedMember(nested, fixture.invalidName, fixture.invalidValue) }, ""},
			} {
				t.Run(mutation.name, func(t *testing.T) {
					mutated := cloneSchemaObject(t, input)
					nested := nestedSchemaObject(t, mutated, "operation_probe.payload."+fixture.nestedPath)
					mutation.change(nested)
					raw := mustSchemaJSON(t, mutated)
					if violations := workflow.RequestStructureViolations("", raw, schema); len(violations) != 0 {
						t.Fatalf("Host transport narrowed opaque payload: %#v", violations)
					}
					if err := ValidateToolInput(tool, raw); err == nil {
						t.Fatal("private validator accepted malformed retained payload")
					}
					requestID := domain.ID("request-" + fixture.name + "-" + mutation.name)
					actual := (&Server{}).dispatch(context.Background(), tool, requestID, raw)
					response := decodeEnvelope(t, actual)
					if !actual.IsError || response.Error.Code != domain.ErrorInvalidArgument || bytes.Contains(actual.JSON, []byte("private-value")) {
						t.Fatalf("malformed retained payload response=%s", actual.JSON)
					}
					if mutation.rule != "" {
						path := "operation_probe.payload." + fixture.nestedPath + "." + mutation.member
						want := EncodeError(string(requestID), tool, domain.InvalidArgumentViolations(domain.Violation(path, mutation.rule)))
						if !bytes.Equal(actual.JSON, want.JSON) {
							t.Fatalf("response=%s want=%s", actual.JSON, want.JSON)
						}
					}
				})
			}
		})
	}
}

func recoveryReadInput(t *testing.T, cursor domain.NodeID, payload map[string]any) map[string]any {
	t.Helper()
	process := workflow.StandardProcess().Reference
	node, err := workflow.NodeDefinition(workflow.StandardProcess(), cursor)
	if err != nil {
		t.Fatal(err)
	}
	steps := make([]any, 0, len(node.SemanticMethodSteps))
	for _, step := range node.SemanticMethodSteps {
		steps = append(steps, map[string]any{"step_id": step.StepID, "status": "plain_fallback", "capability": "", "summary": "Completed."})
	}
	payload["method_evidence"] = steps
	digest := strings.Repeat("a", 64)
	return map[string]any{"host": "codex", "task_id": "task", "operation_probe": map[string]any{
		"operation_id": "operation", "process_id": process.ID, "process_definition_digest": process.DefinitionDigest,
		"source_cursor": cursor, "expected_revision": 1, "action_id": "action", "action_kind": node.ActionKind,
		"repository_binding_digest": digest, "issuance_identity_digest": digest, "issuance_history_digest": digest,
		"issuance_content_digest": digest, "payload": payload,
	}}
}

func recoveryTasksPayload(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"transition_id": "tasks_plan_saved", "summary": "Saved plan.", "reason": "", "artifacts": []any{},
		"node_result": map[string]any{"problem_class": "none", "findings": []any{}, "user_confirmation": nil,
			"baseline": map[string]any{"design_revision": 1, "work_items": []any{map[string]any{
				"work_item_id": "work", "summary": "Repair projection.", "expected_paths": []any{"internal/mcp/schemas.go"},
				"acceptance_indexes": []any{0}, "verification_steps": []any{"Run focused test."}, "dependencies": []any{},
			}}, "verification_plan": map[string]any{
				"checks":              []any{map[string]any{"name": "focused", "rationale": "Check projection."}},
				"initial_budget":      map[string]any{"level": "targeted", "max_automatic_commands": 1, "allow_full_suite": false, "allow_manual_handoff": false},
				"full_suite_expected": false, "test_code_changes_expected": true,
			}},
		},
	}
}

func recoveryBudgetPayload(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{"transition_id": "verification_budget_increased", "summary": "Increased budget.", "reason": "A new risk needs one focused check.", "artifacts": []any{},
		"node_result": map[string]any{"problem_class": "none", "checks": []any{}, "failed_items": []any{}, "unverified_items": []any{}, "manual_handoff_items": []any{}, "findings": []any{},
			"budget_adjustment": map[string]any{"basis": "new_risk", "additional_checks": []any{map[string]any{"name": "focused", "rationale": "Check new risk."}},
				"additional_automatic_commands": 1, "allow_full_suite": false, "allow_manual_handoff": false},
		},
	}
}

func recoveryKnownFailurePayload(t *testing.T) map[string]any {
	t.Helper()
	check := func(name, status string) map[string]any {
		return map[string]any{"source": "automated", "name": name, "status": status, "summary": "Observed.", "command_count": 1, "full_suite": false, "full_suite_reason": ""}
	}
	return map[string]any{"transition_id": "tests_accepted_with_known_failures", "summary": "Accepted exact known failure.", "reason": "The user accepted the compared existing failure.", "artifacts": []any{},
		"node_result": map[string]any{"problem_class": "none", "checks": []any{check("existing", "failed"), check("comparison", "passed")},
			"failed_items": []any{"existing"}, "unverified_items": []any{}, "manual_handoff_items": []any{}, "findings": []any{}, "budget_adjustment": nil,
			"known_failure_acceptance": map[string]any{"source": "user", "summary": "User accepted existing failure.", "failed_checks": []any{"existing"},
				"comparison_check": "comparison", "task_plan_revision": 1, "content_digest": strings.Repeat("a", 64)},
		},
	}
}

func cloneSchemaObject(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	var clone map[string]any
	if err := json.Unmarshal(mustSchemaJSON(t, value), &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func nestedSchemaObject(t *testing.T, root map[string]any, path string) map[string]any {
	t.Helper()
	for _, name := range strings.Split(path, ".") {
		child, ok := root[name].(map[string]any)
		if !ok {
			t.Fatalf("%s does not contain object %s", path, name)
		}
		root = child
	}
	return root
}

func setNestedMember(root map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	for _, name := range parts[:len(parts)-1] {
		root = root[name].(map[string]any)
	}
	root[parts[len(parts)-1]] = value
}

// TestApplyActionTestEvidenceIsVisibleBelowCollapseDepth proves the TEST check
// structure survives at the depth the Host would otherwise collapse.
func TestApplyActionTestEvidenceSchemaIsVisible(t *testing.T) {
	schema := toolSchema(t, ToolSubmitTest)
	nodeResult := childSchema(t, schema, "node_result")
	checks := childSchema(t, nodeResult, "checks")
	if checks["type"] != "array" {
		t.Fatalf("checks type=%#v", checks["type"])
	}
	item, ok := checks["items"].(map[string]any)
	if !ok {
		t.Fatalf("checks items=%#v", checks["items"])
	}
	if item["type"] != "object" || item["additionalProperties"] != false {
		t.Fatalf("check item is not a closed object: %#v", item)
	}
	itemProperties := item["properties"].(map[string]any)
	for _, name := range []string{"source", "name", "status", "summary", "command_count", "full_suite", "full_suite_reason"} {
		if _, present := itemProperties[name]; !present {
			t.Fatalf("check item cannot see %s", name)
		}
	}
	source := itemProperties["source"].(map[string]any)
	wantSources := []string{"automated", "user", "static", "host_observed"}
	got := stringSlice(source["enum"])
	if len(got) != len(wantSources) {
		t.Fatalf("check source enum=%#v", source["enum"])
	}
	for index, want := range wantSources {
		if got[index] != want {
			t.Fatalf("check source enum[%d]=%s", index, got[index])
		}
	}
	commandCount := itemProperties["command_count"].(map[string]any)
	if commandCount["type"] != "integer" || commandCount["minimum"] != float64(0) || commandCount["maximum"] != float64(20) {
		t.Fatalf("check command_count=%#v", commandCount)
	}
	fullSuite := itemProperties["full_suite"].(map[string]any)
	if fullSuite["type"] != "boolean" {
		t.Fatalf("check full_suite=%#v", fullSuite)
	}
}

// TestApplyActionToolDescriptionCarriesSourceRules keeps the cross-field evidence
// rules in the tool description. A projector charges the description separately
// from the schema budget and discards schema descriptions first, so the
// description is the only place a source-specific rule survives reliably.
func TestActionSubmissionDescriptionsStateCoreOwnedAssembly(t *testing.T) {
	for _, entry := range actionSubmissionTools {
		var description string
		for _, definition := range ToolCatalog() {
			if definition.Name == entry.Name {
				description = definition.Description
			}
		}
		if len(description) > 1000 || !strings.Contains(description, "Core fills the complete Action identity") || !strings.Contains(description, "payload envelope") {
			t.Fatalf("%s description=%q", entry.Name, description)
		}
		if entry.Name == ToolSubmitTest {
			for _, rule := range []string{"Automated command_count is 1 to 20", "user, static and host_observed", "command_count 0", "full_suite false"} {
				if !strings.Contains(description, rule) {
					t.Fatalf("%s description misses %q", entry.Name, rule)
				}
			}
		}
		if entry.Name == ToolSubmitDelivery {
			for _, fact := range []string{"acceptance criterion", "test and comprehension record IDs", "automated and manual evidence IDs", "current Task"} {
				if !strings.Contains(description, fact) {
					t.Fatalf("%s description misses %q", entry.Name, fact)
				}
			}
		}
	}
}

// TestApplyActionSchemaSurvivesHostCompaction verifies the current published
// schema retains its callable surface after Host projection.
func TestApplyActionSchemaSurvivesHostCompaction(t *testing.T) {
	published := toolSchema(t, ToolSubmitTest)
	compacted, ok := hostCompact(t, published).(map[string]any)
	if !ok || compacted["type"] != "object" {
		t.Fatalf("published apply schema collapsed: %#v", compacted)
	}
	properties, ok := compacted["properties"].(map[string]any)
	if !ok || len(properties) != 9 {
		t.Fatalf("published submission schema lost top-level properties: %#v", compacted["properties"])
	}
	nodeResult, ok := properties["node_result"].(map[string]any)
	if !ok || len(nodeResult) == 0 {
		t.Fatal("node_result did not survive compaction")
	}
	checks := childSchema(t, nodeResult, "checks")
	item, ok := checks["items"].(map[string]any)
	if !ok {
		t.Fatalf("checks items did not survive compaction: %#v", checks)
	}
	itemProperties, ok := item["properties"].(map[string]any)
	if !ok || len(itemProperties) != 7 {
		t.Fatalf("check item lost members during compaction: %#v", item)
	}
}

// hostCompact applies the Host compaction passes in order while the modelled
// schema stays over budget.
func hostCompact(t *testing.T, schema map[string]any) any {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	passes := []func(any) any{
		hostStripDescriptions,
		hostDropDefinitions,
		func(node any) any { return hostCollapseDeep(node, 0) },
		hostPruneCompositions,
	}
	for _, pass := range passes {
		if hostModelledLength(t, value) <= hostProjectionBudgetBytes {
			break
		}
		value = pass(value)
	}
	return value
}
func hostModelledLength(t *testing.T, value any) int {
	t.Helper()
	encoded, err := json.Marshal(modelledHostSchema(value))
	if err != nil {
		t.Fatal(err)
	}
	return len(encoded)
}
func walkHostSchemaChildren(node map[string]any, includeDefinitions bool, transform func(any) any) {
	if properties, ok := node["properties"].(map[string]any); ok {
		for name, value := range properties {
			properties[name] = transform(value)
		}
	}
	for _, key := range []string{"items", "anyOf", "oneOf", "allOf"} {
		if value, present := node[key]; present {
			node[key] = transform(value)
		}
	}
	if value, present := node["additionalProperties"]; present {
		if _, isBool := value.(bool); !isBool {
			node["additionalProperties"] = transform(value)
		}
	}
	if !includeDefinitions {
		return
	}
	for _, key := range []string{"$defs", "definitions"} {
		if table, ok := node[key].(map[string]any); ok {
			for name, value := range table {
				table[name] = transform(value)
			}
		}
	}
}
func hostStripDescriptions(value any) any {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = hostStripDescriptions(item)
		}
		return typed
	case map[string]any:
		delete(typed, "description")
		walkHostSchemaChildren(typed, true, hostStripDescriptions)
		return typed
	default:
		return value
	}
}
func hostDropDefinitions(value any) any {
	blanked := hostBlankDefinitionRefs(value)
	node, ok := blanked.(map[string]any)
	if !ok {
		return blanked
	}
	delete(node, "$defs")
	delete(node, "definitions")
	return node
}
func hostBlankDefinitionRefs(value any) any {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = hostBlankDefinitionRefs(item)
		}
		return typed
	case map[string]any:
		if reference, ok := typed["$ref"].(string); ok && strings.HasPrefix(reference, "#/") {
			return map[string]any{}
		}
		walkHostSchemaChildren(typed, false, hostBlankDefinitionRefs)
		return typed
	default:
		return value
	}
}
func hostCollapseDeep(value any, depth int) any {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = hostCollapseDeep(item, depth)
		}
		return typed
	case map[string]any:
		if depth >= hostProjectionCollapse && hostComplexSchema(typed) {
			return map[string]any{}
		}
		walkHostSchemaChildren(typed, false, func(child any) any { return hostCollapseDeep(child, depth+1) })
		return typed
	default:
		return value
	}
}
func hostPruneCompositions(value any) any {
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			typed[index] = hostPruneCompositions(item)
		}
		return typed
	case map[string]any:
		for _, keyword := range hostCompositionKeywords {
			if _, present := typed[keyword]; present {
				return map[string]any{}
			}
		}
		walkHostSchemaChildren(typed, false, hostPruneCompositions)
		return typed
	default:
		return value
	}
}
func hostComplexSchema(node map[string]any) bool {
	for _, key := range []string{"items", "anyOf", "oneOf", "allOf", "properties", "additionalProperties", "$ref"} {
		if _, present := node[key]; present {
			return true
		}
	}
	return false
}

func childSchema(t *testing.T, parent map[string]any, name string) map[string]any {
	t.Helper()
	properties, ok := parent["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties while reading %s", name)
	}
	child, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema member %s is not an object schema: %#v", name, properties[name])
	}
	return child
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		out = append(out, text)
	}
	return out
}

// assertHostProjectable walks a schema tree and fails on any node the Host
// projector cannot model as a concrete type.
func assertHostProjectable(t *testing.T, schema map[string]any, path string) {
	t.Helper()
	for _, keyword := range hostCompositionKeywords {
		if _, present := schema[keyword]; present {
			t.Fatalf("%s carries composition keyword %s", path, keyword)
		}
	}
	if _, present := schema["$ref"]; present {
		t.Fatalf("%s uses $ref, which the Host projector may not resolve", path)
	}
	types := stringSlice(schema["type"])
	if single, ok := schema["type"].(string); ok {
		types = []string{single}
	}
	if len(types) == 0 {
		t.Fatalf("%s declares no type", path)
	}
	object, array := false, false
	for _, name := range types {
		switch name {
		case "object":
			object = true
		case "array":
			array = true
		case "string", "integer", "number", "boolean", "null":
		default:
			t.Fatalf("%s declares unsupported type %s", path, name)
		}
	}
	if object {
		properties, ok := schema["properties"].(map[string]any)
		if !ok || len(properties) == 0 {
			projectedPath := strings.TrimPrefix(path, "$.operation_probe.")
			if projectedPath != path && projectedCollapsedPaths[projectedPath] && len(schema) == 1 {
				// Only these three deep read-probe values are opaque to the Host.
				// ValidateToolInput still checks their complete closed payload.
				return
			}
			t.Fatalf("%s is an object with no projectable properties", path)
		}
		if schema["additionalProperties"] != false {
			t.Fatalf("%s is an open object", path)
		}
		names := make([]string, 0, len(properties))
		for name := range properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			member, ok := properties[name].(map[string]any)
			if !ok {
				t.Fatalf("%s/%s is not an object schema", path, name)
			}
			assertHostProjectable(t, member, fmt.Sprintf("%s.%s", path, name))
		}
		for _, name := range stringSlice(schema["required"]) {
			if _, present := properties[name]; !present {
				t.Fatalf("%s requires %s without declaring it", path, name)
			}
		}
	}
	if array {
		item, ok := schema["items"].(map[string]any)
		if !ok {
			t.Fatalf("%s is an array with no projectable items", path)
		}
		assertHostProjectable(t, item, path+"[]")
	}
}

// TestSDKListedApplySchemaIsProjectable proves the published apply schema can be
// registered with the Go MCP SDK, served, listed, and still projected.
func TestSDKListedSubmissionSchemasAreProjectable(t *testing.T) {
	service, err := application.NewService(annotationStore{}, annotationObserver{})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(service, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Run(ctx, serverTransport)
	client := sdk.NewClient(&sdk.Implementation{Name: "projection-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != len(ToolNames()) {
		t.Fatalf("tools=%d", len(listed.Tools))
	}
	seen := 0
	for _, tool := range listed.Tools {
		_, submission := submissionKindForTool(tool.Name)
		if !submission && tool.Name != ToolGetTask && tool.Name != ToolGetNextAction {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["type"] != "object" {
			t.Fatalf("listed submission schema root type=%#v", schema["type"])
		}
		assertHostProjectable(t, schema, "$")
		if size := modelledHostSchemaBytes(t, raw); size > hostProjectionBudgetBytes-hostProjectionMarginBytes {
			t.Fatalf("listed submission schema is %d bytes", size)
		}
		seen++
	}
	if seen != len(actionSubmissionTools)+2 {
		t.Fatalf("listed submission and read tools=%d", seen)
	}
}
