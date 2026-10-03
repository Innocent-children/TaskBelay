package contract_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/domain"
	core "github.com/Innocent-children/taskbelay/internal/mcp"
	"slices"
	"strings"
	"testing"
)

func TestMCPToolCatalogIsExactStableAndConservative(t *testing.T) {
	want := []string{core.ToolServerInfo, core.ToolOpenTask, core.ToolGetTask, core.ToolGetNextAction,
		core.ToolSubmitRequirements, core.ToolSubmitDesign, core.ToolSubmitTasks, core.ToolSubmitImplementation,
		core.ToolSubmitTest, core.ToolSubmitComprehension, core.ToolSubmitRefactor, core.ToolSubmitDelivery,
		core.ToolPrepareTaskBranchRename, core.ToolPrepareTaskRelocation, core.ToolResolveBlocker, core.ToolRecoverAction, core.ToolCancelTask, core.ToolAbandonTask}
	if !slices.Equal(core.ToolNames(), want) {
		t.Fatal(core.ToolNames())
	}
	for _, tool := range core.ToolCatalog() {
		read := tool.Name == core.ToolServerInfo || tool.Name == core.ToolGetTask
		idempotent := tool.Name != core.ToolOpenTask && tool.Name != core.ToolCancelTask && tool.Name != core.ToolAbandonTask
		destructive := tool.Name == core.ToolCancelTask || tool.Name == core.ToolAbandonTask
		if tool.Annotations.ReadOnly != read || tool.Annotations.Idempotent != idempotent || tool.Annotations.Destructive != destructive || tool.Annotations.OpenWorld {
			t.Fatalf("annotations %s %#v", tool.Name, tool.Annotations)
		}
		var schema any
		if json.Unmarshal(tool.InputSchema, &schema) != nil {
			t.Fatal("invalid schema")
		}
		assertClosed(t, schema, "$")
	}
}
func assertClosed(t *testing.T, value any, path string) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		_, discriminated := v["oneOf"]
		if isObjectSchema(v) && v["additionalProperties"] != false && !discriminated {
			t.Fatalf("open object %s", path)
		}
		for k, x := range v {
			assertClosed(t, x, path+"/"+k)
		}
	case []any:
		for i, x := range v {
			assertClosed(t, x, fmt.Sprintf("%s/%d", path, i))
		}
	}
}
func TestMCPCurrentContractRequiredShapes(t *testing.T) {
	schemas := map[string]map[string]any{}
	for _, tool := range core.ToolCatalog() {
		var v map[string]any
		_ = json.Unmarshal(tool.InputSchema, &v)
		schemas[tool.Name] = v
	}
	requireNames := func(tool string, names []string) {
		required := stringsOf(schemas[tool]["required"])
		if !slices.Equal(required, names) {
			t.Fatalf("%s required=%v", tool, required)
		}
	}
	requireNames(core.ToolOpenTask, []string{"host", "repository_path"})
	requireNames(core.ToolGetTask, []string{"host", "task_id"})
	requireNames(core.ToolGetNextAction, []string{"host", "task_id"})
	for _, tool := range []string{core.ToolSubmitRequirements, core.ToolSubmitDesign, core.ToolSubmitTasks, core.ToolSubmitImplementation, core.ToolSubmitTest, core.ToolSubmitComprehension, core.ToolSubmitRefactor, core.ToolSubmitDelivery} {
		requireNames(tool, []string{"host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"})
	}
	requireNames(core.ToolResolveBlocker, []string{"host", "task_id", "action_id"})
	requireNames(core.ToolRecoverAction, []string{"host", "task_id", "action_id"})
	requireNames(core.ToolCancelTask, []string{"request_id", "host", "task_id", "revision", "reason"})
	requireNames(core.ToolPrepareTaskRelocation, []string{"host", "task_id", "revision"})
	requireNames(core.ToolAbandonTask, []string{"host", "task_id", "revision", "reason"})
	openProperties := schemas[core.ToolOpenTask]["properties"].(map[string]any)
	workspaceOrigin := openProperties["workspace_origin"].(map[string]any)
	if !slices.Equal(stringsOf(workspaceOrigin["required"]), []string{"mode", "source_type", "carry_changes", "remote_name", "base_branch", "base_commit", "task_branch", "provisioning_receipt_id"}) {
		t.Fatalf("workspace_origin schema=%#v", workspaceOrigin)
	}
	additional := openProperties["additional_repositories"].(map[string]any)
	if additional["maxItems"] != float64(domain.MaxAdditionalRepositories) || additional["items"].(map[string]any)["additionalProperties"] != false {
		t.Fatalf("additional_repositories schema=%#v", additional)
	}
	if openProperties["primary_repository_key"].(map[string]any)["pattern"] != "^[a-z0-9][a-z0-9._-]{0,127}$" {
		t.Fatal("primary_repository_key pattern changed")
	}
	additionalRequired := stringsOf(additional["items"].(map[string]any)["required"])
	if !slices.Equal(additionalRequired, []string{"key", "repository_path", "workspace_origin"}) {
		t.Fatalf("additional repository required=%v", additionalRequired)
	}
	resolveProperties := schemas[core.ToolResolveBlocker]["properties"].(map[string]any)
	relocationItems := resolveProperties["relocation_destinations"].(map[string]any)["items"].(map[string]any)
	if !slices.Equal(stringsOf(relocationItems["required"]), []string{"key", "repository_path"}) {
		t.Fatalf("relocation destination schema=%#v", relocationItems)
	}
	history := resolveProperties["history_resolution"].(map[string]any)
	if !slices.Equal(stringsOf(history["required"]), []string{"choice", "reason"}) {
		t.Fatalf("history resolution schema=%#v", history)
	}
	for _, tool := range core.ToolNames() {
		if tool == core.ToolOpenTask {
			continue
		}
		properties, _ := schemas[tool]["properties"].(map[string]any)
		if _, exists := properties["primary_repository_key"]; exists {
			t.Fatalf("%s input gained repository scope", tool)
		}
		if _, exists := properties["additional_repositories"]; exists {
			t.Fatalf("%s input gained additional repositories", tool)
		}
	}
}

// isObjectSchema recognizes both `"type":"object"` and the nullable union form
// `"type":["object","null"]` the apply projection uses for optional objects.
func isObjectSchema(schema map[string]any) bool {
	if schema["type"] == "object" {
		return true
	}
	for _, name := range stringsOf(schema["type"]) {
		if name == "object" {
			return true
		}
	}
	return false
}
func stringsOf(v any) []string {
	items, _ := v.([]any)
	out := make([]string, len(items))
	for i, x := range items {
		out[i], _ = x.(string)
	}
	return out
}
func TestMCPStrictInputBoundaryAndDuplicateMembers(t *testing.T) {
	valid := []byte(`{"host":"codex","task_id":"task","operation_probe":null}`)
	if err := core.ValidateToolInput(core.ToolGetTask, valid); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{[]byte(`{"host":"codex","task_id":"a","task_id":"b","operation_probe":null}`), []byte(`{"host":"codex","repository_path":"/repo","new_task":{"request":"x","initial_scope":[],"initial_out_of_scope":[],"known_acceptance_criteria":[],"method_profile":"plain","method_profile":"spec-kit"}}`)} {
		if err := core.ValidateToolInput(core.ToolGetTask, raw); err == nil {
			t.Fatal("duplicate accepted")
		}
	}
	validLifecycle := map[string]string{
		core.ToolPrepareTaskRelocation: `{"host":"codex","task_id":"task","revision":3}`,
		core.ToolAbandonTask:           `{"host":"codex","task_id":"task","revision":3,"reason":"The retained worktree instance is unavailable."}`,
		core.ToolResolveBlocker:        `{"host":"codex","task_id":"task","action_id":"action","relocation_id":"relocation","relocation_destinations":[{"key":"primary","repository_path":"/worktree"}]}`,
	}
	for tool, raw := range validLifecycle {
		if err := core.ValidateToolInput(tool, []byte(raw)); err != nil {
			t.Fatalf("%s lifecycle input rejected: %v", tool, err)
		}
	}
	if err := core.ValidateToolInput(core.ToolResolveBlocker, []byte(`{"host":"codex","task_id":"task","action_id":"action","history_resolution":{"choice":"accept_current_history","reason":"The reviewed history is current."}}`)); err != nil {
		t.Fatalf("history resolution input rejected: %v", err)
	}
}

func TestMCPOpenTaskSingleAndMultiRepositoryInputBoundary(t *testing.T) {
	newTask := `"new_task":{"request":"Build feature","initial_scope":[],"initial_out_of_scope":[],"known_acceptance_criteria":[],"method_profile":"plain"}`
	origin := `"workspace_origin":{"mode":"dedicated_worktree","source_type":"remote","carry_changes":false,"remote_name":"origin","base_branch":"main","base_commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","task_branch":"task/core","provisioning_receipt_id":"receipt-core"}`
	additionalOrigin := `"workspace_origin":{"mode":"dedicated_worktree","source_type":"remote","carry_changes":false,"remote_name":"origin","base_branch":"main","base_commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","task_branch":"task/docs","provisioning_receipt_id":"receipt-docs"}`
	for _, raw := range []string{
		`{"host":"codex","repository_path":"/repo"}`,
		`{"host":"codex","repository_path":"/core",` + origin + `,"primary_repository_key":"core","additional_repositories":[{"key":"docs","repository_path":"/docs",` + additionalOrigin + `}],` + newTask + `}`,
	} {
		if err := core.ValidateToolInput(core.ToolOpenTask, []byte(raw)); err != nil {
			t.Fatalf("valid open input rejected: %v %s", err, raw)
		}
	}
	additional := make([]map[string]any, domain.MaxAdditionalRepositories+1)
	for i := range additional {
		additional[i] = map[string]any{"key": fmt.Sprintf("repo%d", i), "repository_path": fmt.Sprintf("/repo%d", i), "workspace_origin": workspaceOriginValue(fmt.Sprintf("task/repo%d", i), fmt.Sprintf("receipt-repo%d", i))}
	}
	tooMany, err := json.Marshal(map[string]any{"host": "codex", "repository_path": "/core", "workspace_origin": workspaceOriginValue("task/core", "receipt-core"), "additional_repositories": additional, "new_task": map[string]any{"request": "Build feature", "initial_scope": []string{}, "initial_out_of_scope": []string{}, "known_acceptance_criteria": []string{}, "method_profile": "plain"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := core.ValidateToolInput(core.ToolOpenTask, []byte(`{"host":"codex","repository_path":"/core",`+newTask+`}`)); !errors.Is(err, domain.ErrWorktreeProvisioningRequired) {
		t.Fatalf("missing workspace origin error=%v", err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"host":"codex","repository_path":"/core",` + origin + `,"additional_repositories":[{"key":"docs","repository_path":"/docs"}],` + newTask + `}`),
		[]byte(`{"host":"codex","repository_path":"/core",` + origin + `,"additional_repositories":[{"key":"docs","repository_path":"/docs",` + additionalOrigin + `,"unknown":true}],` + newTask + `}`),
		tooMany,
	} {
		if err := core.ValidateToolInput(core.ToolOpenTask, raw); !errors.Is(err, domain.ErrInvalidArgument) {
			t.Fatalf("invalid multi repository input error=%v raw=%s", err, raw)
		}
	}
}

func workspaceOriginValue(taskBranch, receiptID string) map[string]any {
	return map[string]any{"mode": "dedicated_worktree", "source_type": "remote", "carry_changes": false, "remote_name": "origin", "base_branch": "main", "base_commit": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "task_branch": taskBranch, "provisioning_receipt_id": receiptID}
}
func TestMCPStableErrorEnvelopesAreClosedAndRedacted(t *testing.T) {
	secret := "/Users/private/secret.db SELECT * FROM tasks"
	encoded := core.EncodeError("request-error", core.ToolGetTask, fmt.Errorf("%s", secret))
	if bytes.Contains(encoded.JSON, []byte(secret)) || !bytes.Contains(encoded.JSON, []byte(`"code":"INTERNAL_ERROR"`)) {
		t.Fatal(string(encoded.JSON))
	}
	_ = domain.ErrorInternal
}

func TestSchemaUnsupportedResultIsBoundedAndPathFree(t *testing.T) {
	encoded := core.EncodeError("schema-unsupported", core.ToolOpenTask, domain.ErrSchemaUnsupported)
	text := string(encoded.JSON)
	for _, forbidden := range []string{"/Users/", "/home/", "HOME=", "taskbelay.db", "SELECT ", "sqlite", "repository_path", "data_path"} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("SCHEMA_UNSUPPORTED leaked private/storage detail %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{`"code":"SCHEMA_UNSUPPORTED"`, "storage schema is unsupported", "Stop this operation"} {
		if !strings.Contains(text, required) {
			t.Fatalf("SCHEMA_UNSUPPORTED guidance missing %q: %s", required, text)
		}
	}
}

func TestMCPLocalWorkspaceRequiresExplicitContentChoice(t *testing.T) {
	valid := `{"host":"codex","repository_path":"/local","workspace_origin":{"mode":"dedicated_worktree","source_type":"local","carry_changes":true,"remote_name":"","base_branch":"main","base_commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","task_branch":"task/local","provisioning_receipt_id":"receipt-local"},"new_task":{"request":"Local work","initial_scope":[],"initial_out_of_scope":[],"known_acceptance_criteria":[],"method_profile":"plain"}}`
	if err := core.ValidateToolInput(core.ToolOpenTask, []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, `"carry_changes":true,`, "", 1),
		strings.Replace(valid, `"carry_changes":true`, `"carry_changes":null`, 1),
		strings.Replace(valid, `"source_type":"local"`, `"source_type":"remote"`, 1),
		strings.Replace(valid, `"remote_name":""`, `"remote_name":"origin"`, 1),
	} {
		if err := core.ValidateToolInput(core.ToolOpenTask, []byte(invalid)); err == nil {
			t.Fatalf("invalid source accepted: %s", invalid)
		}
	}
}
