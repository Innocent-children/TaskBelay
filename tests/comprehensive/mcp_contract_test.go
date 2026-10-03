package comprehensive_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	coremcp "github.com/Innocent-children/taskbelay/internal/mcp"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func TestMCPGeneratedCatalogMatchesProcessAndClosedSchemas(t *testing.T) {
	names := coremcp.ToolNames()
	catalog := coremcp.ToolCatalog()
	if len(names) != 18 || len(catalog) != len(names) {
		t.Fatalf("names=%d catalog=%d", len(names), len(catalog))
	}
	seen := map[string]bool{}
	submissionTools := 0
	for index, tool := range catalog {
		if tool.Name != names[index] || seen[tool.Name] {
			t.Fatalf("catalog order or uniqueness failed at %s", tool.Name)
		}
		seen[tool.Name] = true
		if strings.HasPrefix(tool.Name, "taskbelay_submit_") {
			submissionTools++
		}
		var schema any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s schema: %v", tool.Name, err)
		}
		assertClosedSchema(t, schema, tool.Name)
		readOnly := tool.Name == coremcp.ToolServerInfo || tool.Name == coremcp.ToolGetTask
		idempotent := tool.Name != coremcp.ToolOpenTask && tool.Name != coremcp.ToolCancelTask && tool.Name != coremcp.ToolAbandonTask
		destructive := tool.Name == coremcp.ToolCancelTask || tool.Name == coremcp.ToolAbandonTask
		if tool.Annotations.ReadOnly != readOnly || tool.Annotations.Idempotent != idempotent || tool.Annotations.Destructive != destructive || tool.Annotations.OpenWorld {
			t.Fatalf("%s annotations=%#v", tool.Name, tool.Annotations)
		}
	}

	activeNodes := 0
	for _, node := range workflow.StandardProcess().Nodes {
		if len(node.OutgoingTransitions) != 0 {
			activeNodes++
		}
	}
	if submissionTools != activeNodes {
		t.Fatalf("submission tools=%d active nodes=%d", submissionTools, activeNodes)
	}
}

func TestMCPReadLifecycleInputsAndClosedRejections(t *testing.T) {
	valid := map[string]string{
		coremcp.ToolServerInfo:            `{}`,
		coremcp.ToolOpenTask:              `{"host":"codex","repository_path":"/repo"}`,
		coremcp.ToolGetTask:               `{"host":"codex","task_id":"task","operation_probe":null}`,
		coremcp.ToolGetNextAction:         `{"host":"deepseek","task_id":"task","operation_probe":null}`,
		coremcp.ToolResolveBlocker:        `{"host":"codex","task_id":"task","action_id":"action"}`,
		coremcp.ToolRecoverAction:         `{"host":"codex","task_id":"task","action_id":"action"}`,
		coremcp.ToolCancelTask:            `{"request_id":"request","host":"codex","task_id":"task","revision":1,"reason":"cancel requested"}`,
		coremcp.ToolPrepareTaskRelocation: `{"host":"codex","task_id":"task","revision":1}`,
		coremcp.ToolAbandonTask:           `{"host":"codex","task_id":"task","revision":1,"reason":"worktree is unavailable"}`,
	}
	for tool, raw := range valid {
		if err := coremcp.ValidateToolInput(tool, []byte(raw)); err != nil {
			t.Fatalf("%s valid input rejected: %v", tool, err)
		}
		object := decodeJSONObject(t, []byte(raw))
		object["unknown_member"] = true
		unknown, err := json.Marshal(object)
		if err != nil {
			t.Fatal(err)
		}
		if coremcp.ValidateToolInput(tool, unknown) == nil {
			t.Fatalf("%s accepted an unknown member", tool)
		}
	}

	for _, tool := range coremcp.ToolNames() {
		if err := coremcp.ValidateToolInput(tool, []byte(`{"unknown_member":true}`)); err == nil {
			t.Fatalf("%s accepted an open request", tool)
		}
	}
	for _, raw := range []string{
		`{"host":"codex","host":"deepseek","task_id":"task","operation_probe":null}`,
		`{"host":"codex","repository_path":"/repo","repository_path":"/other"}`,
		`{"request_id":"request","host":"codex","task_id":"task","revision":1,"reason":"cancel","reason":"again"}`,
	} {
		for _, tool := range []string{coremcp.ToolGetTask, coremcp.ToolOpenTask, coremcp.ToolCancelTask} {
			if coremcp.ValidateToolInput(tool, []byte(raw)) == nil {
				t.Fatalf("%s accepted duplicate JSON members: %s", tool, raw)
			}
		}
	}
}

func TestMCPOpenTaskRepositoryRangesAndDefersVerificationBudget(t *testing.T) {
	for additionalCount := 0; additionalCount <= domain.MaxAdditionalRepositories; additionalCount++ {
		additional := make([]map[string]any, additionalCount)
		for index := range additional {
			additional[index] = map[string]any{
				"key":              fmt.Sprintf("repo-%d", index),
				"repository_path":  fmt.Sprintf("/repo/%d", index),
				"workspace_origin": comprehensiveWorkspaceOrigin(fmt.Sprintf("task/repo-%d", index), fmt.Sprintf("receipt-repo-%d", index)),
			}
		}
		request := map[string]any{
			"host":                    "codex",
			"repository_path":         "/repo/primary",
			"workspace_origin":        comprehensiveWorkspaceOrigin("task/primary", "receipt-primary"),
			"primary_repository_key":  "primary",
			"additional_repositories": additional,
			"new_task": map[string]any{
				"request":                   "Exercise repository boundaries before verification is planned.",
				"initial_scope":             []string{},
				"initial_out_of_scope":      []string{},
				"known_acceptance_criteria": []string{},
				"method_profile":            "plain",
			},
		}
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := coremcp.ValidateToolInput(coremcp.ToolOpenTask, raw); err != nil {
			t.Fatalf("repositories=%d: %v", additionalCount+1, err)
		}
	}

	invalidAdditional := make([]map[string]any, domain.MaxAdditionalRepositories+1)
	for index := range invalidAdditional {
		invalidAdditional[index] = map[string]any{"key": fmt.Sprintf("repo-%d", index), "repository_path": fmt.Sprintf("/repo/%d", index), "workspace_origin": comprehensiveWorkspaceOrigin(fmt.Sprintf("task/repo-%d", index), fmt.Sprintf("receipt-repo-%d", index))}
	}
	creationBudget := openTaskRequest(nil)
	creationBudget["new_task"].(map[string]any)["verification_budget"] = map[string]any{"level": "targeted", "max_automatic_commands": 1, "allow_full_suite": false, "allow_manual_handoff": false}
	invalidRequests := []map[string]any{
		openTaskRequest(invalidAdditional),
		creationBudget,
		openTaskRequest([]map[string]any{{"key": "primary", "repository_path": "/duplicate", "workspace_origin": comprehensiveWorkspaceOrigin("task/duplicate", "receipt-duplicate")}}),
	}
	for index, request := range invalidRequests {
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if coremcp.ValidateToolInput(coremcp.ToolOpenTask, raw) == nil {
			t.Fatalf("invalid boundary request %d accepted", index)
		}
	}
}

func TestMCPSubmissionSchemasRemainActionSpecificAndHostBounded(t *testing.T) {
	wantRequired := []string{"host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"}
	coreOwned := []string{"request_id", "revision", "action_kind", "process_id", "process_definition_digest", "source_cursor", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "payload", "destination"}
	for _, tool := range coremcp.ToolCatalog() {
		if !strings.HasPrefix(tool.Name, "taskbelay_submit_") {
			continue
		}
		schema := decodeJSONObject(t, tool.InputSchema)
		required := jsonStrings(schema["required"])
		if !slices.Equal(required, wantRequired) {
			t.Fatalf("%s required=%v", tool.Name, required)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no properties", tool.Name)
		}
		for _, member := range coreOwned {
			if _, exists := properties[member]; exists {
				t.Fatalf("%s exposes Core-owned %s", tool.Name, member)
			}
		}
		for _, member := range []string{"artifacts", "method_results", "node_result"} {
			if _, exists := properties[member]; !exists {
				t.Fatalf("%s omits %s", tool.Name, member)
			}
		}
		for _, removed := range []string{"changed_paths", "no_file_changes"} {
			if bytes.Contains(tool.InputSchema, []byte(`"`+removed+`"`)) {
				t.Fatalf("%s retains removed Host file field %s", tool.Name, removed)
			}
		}
	}
}

func TestMCPErrorResultsAreBoundedAndRedacted(t *testing.T) {
	private := "/Users/private/taskbelay.db SELECT * FROM tasks token=secret"
	encoded := coremcp.EncodeError("request-comprehensive", coremcp.ToolGetTask, fmt.Errorf("%s", private))
	if bytes.Contains(encoded.JSON, []byte(private)) || !bytes.Contains(encoded.JSON, []byte(`"code":"INTERNAL_ERROR"`)) {
		t.Fatal(string(encoded.JSON))
	}
	for _, forbidden := range []string{"/Users/", "taskbelay.db", "SELECT *", "token=secret"} {
		if bytes.Contains(encoded.JSON, []byte(forbidden)) {
			t.Fatalf("error result leaked %q", forbidden)
		}
	}
	if !errors.Is(coremcp.ValidateToolInput("taskbelay_unknown", []byte(`{}`)), domain.ErrInvalidArgument) {
		t.Fatal("unknown MCP tool was not rejected")
	}
}

func FuzzMCPInputBoundaryNeverAcceptsUnknownTools(f *testing.F) {
	for _, seed := range []struct {
		tool string
		raw  []byte
	}{
		{coremcp.ToolServerInfo, []byte(`{}`)},
		{coremcp.ToolGetTask, []byte(`{"host":"codex","task_id":"task"}`)},
		{"taskbelay_unknown", []byte(`{}`)},
		{"", []byte(`{"host":"codex"}`)},
		{coremcp.ToolOpenTask, []byte{0xff, 0xfe}},
	} {
		f.Add(seed.tool, seed.raw)
	}
	catalog := map[string]bool{}
	for _, name := range coremcp.ToolNames() {
		catalog[name] = true
	}
	f.Fuzz(func(t *testing.T, tool string, raw []byte) {
		err := coremcp.ValidateToolInput(tool, raw)
		if !catalog[tool] && err == nil {
			t.Fatalf("unknown tool %q accepted input %q", tool, raw)
		}
	})
}

func assertClosedSchema(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		if schemaHasObjectType(typed["type"]) && typed["additionalProperties"] != false {
			if !isOpaqueRecoveryTransportPath(path) || len(typed) != 1 {
				t.Fatalf("open object schema at %s", path)
			}
		}
		for name, member := range typed {
			assertClosedSchema(t, member, path+"/"+name)
		}
	case []any:
		for index, member := range typed {
			assertClosedSchema(t, member, fmt.Sprintf("%s/%d", path, index))
		}
	}
}

func isOpaqueRecoveryTransportPath(path string) bool {
	for _, tool := range []string{coremcp.ToolGetTask, coremcp.ToolGetNextAction} {
		prefix := tool + "/properties/operation_probe/properties/payload/properties/node_result/properties/"
		switch path {
		case prefix + "baseline/properties/verification_plan",
			prefix + "budget_adjustment",
			prefix + "known_failure_acceptance":
			return true
		}
	}
	return false
}

func schemaHasObjectType(value any) bool {
	if value == "object" {
		return true
	}
	for _, item := range jsonStrings(value) {
		if item == "object" {
			return true
		}
	}
	return false
}

func jsonStrings(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index], _ = item.(string)
	}
	return result
}

func openTaskRequest(additional []map[string]any) map[string]any {
	return map[string]any{
		"host":                    "codex",
		"repository_path":         "/repo/primary",
		"workspace_origin":        comprehensiveWorkspaceOrigin("task/primary", "receipt-primary"),
		"primary_repository_key":  "primary",
		"additional_repositories": additional,
		"new_task": map[string]any{
			"request":                   "Invalid boundary request.",
			"initial_scope":             []string{},
			"initial_out_of_scope":      []string{},
			"known_acceptance_criteria": []string{},
			"method_profile":            "plain",
		},
	}
}

func comprehensiveWorkspaceOrigin(taskBranch, receiptID string) map[string]any {
	return map[string]any{"mode": "dedicated_worktree", "source_type": "remote", "carry_changes": false, "remote_name": "origin", "base_branch": "main", "base_commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "task_branch": taskBranch, "provisioning_receipt_id": receiptID}
}
