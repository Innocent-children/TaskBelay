package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func TestMCPContractGraphCatalogAndClosedSchemas(t *testing.T) {
	want := []string{ToolServerInfo, ToolOpenTask, ToolGetTask, ToolGetNextAction,
		ToolSubmitRequirements, ToolSubmitDesign, ToolSubmitTasks, ToolSubmitImplementation,
		ToolSubmitTest, ToolSubmitComprehension, ToolSubmitRefactor, ToolSubmitDelivery,
		ToolPrepareTaskBranchRename, ToolPrepareTaskRelocation, ToolResolveBlocker, ToolRecoverAction, ToolCancelTask, ToolAbandonTask}
	got := ToolNames()
	if len(got) != len(want) {
		t.Fatalf("tools=%d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tool %d=%s", i, got[i])
		}
	}
	for _, d := range ToolCatalog() {
		var schema map[string]any
		if err := json.Unmarshal(d.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["additionalProperties"] != false {
			t.Fatalf("%s schema open", d.Name)
		}
	}
	if err := ValidateToolInput(ToolGetTask, []byte(`{"host":"codex","task_id":"task","extra":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	var openSchema map[string]any
	if err := json.Unmarshal(catalog[1].InputSchema, &openSchema); err != nil {
		t.Fatal(err)
	}
	properties := openSchema["properties"].(map[string]any)
	additional := properties["additional_repositories"].(map[string]any)
	items := additional["items"].(map[string]any)
	if additional["maxItems"] != float64(domain.MaxAdditionalRepositories) || items["additionalProperties"] != false {
		t.Fatalf("additional repository schema=%#v", additional)
	}
	key := properties["primary_repository_key"].(map[string]any)
	if key["pattern"] != "^[a-z0-9][a-z0-9._-]{0,127}$" {
		t.Fatalf("primary repository key schema=%#v", key)
	}
}

func TestActionSubmissionSchemasAreKindSpecificAndCoreOwned(t *testing.T) {
	definition := workflow.StandardProcess()
	for _, entry := range actionSubmissionTools {
		schema := toolSchema(t, entry.Name)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("%s root=%#v", entry.Name, schema)
		}
		properties := schema["properties"].(map[string]any)
		for _, forbidden := range []string{"request_id", "revision", "action_kind", "process_id", "process_definition_digest", "source_cursor", "repository_binding_digest", "issuance_identity_digest", "issuance_history_digest", "issuance_content_digest", "payload"} {
			if _, present := properties[forbidden]; present {
				t.Fatalf("%s exposes Core-owned %s", entry.Name, forbidden)
			}
		}
		for _, required := range []string{"host", "task_id", "action_id", "transition_id", "summary", "reason", "artifacts", "method_results", "node_result"} {
			if _, present := properties[required]; !present {
				t.Fatalf("%s misses %s", entry.Name, required)
			}
		}
		node, err := workflow.NodeDefinitionForActionKind(definition, entry.Kind)
		if err != nil {
			t.Fatal(err)
		}
		methodProperties := properties["method_results"].(map[string]any)["properties"].(map[string]any)
		if len(methodProperties) != len(node.SemanticMethodSteps) {
			t.Fatalf("%s method results=%#v", entry.Name, methodProperties)
		}
		for _, step := range node.SemanticMethodSteps {
			if _, present := methodProperties[string(step.StepID)]; !present {
				t.Fatalf("%s misses method step %s", entry.Name, step.StepID)
			}
		}
		artifactProperties := properties["artifacts"].(map[string]any)["properties"].(map[string]any)
		_, primaryAllowed := workflow.PrimaryArtifactRoleForNode(node.NodeID)
		_, currentVisible := artifactProperties["current"]
		if primaryAllowed != currentVisible {
			t.Fatalf("%s current artifact visibility=%v want=%v", entry.Name, currentVisible, primaryAllowed)
		}
		for slot, raw := range artifactProperties {
			item := raw.(map[string]any)["items"].(map[string]any)
			fields := item["properties"].(map[string]any)
			if len(fields) != 3 || fields["path"] == nil || fields["digest"] == nil || fields["summary"] == nil || fields["role"] != nil {
				t.Fatalf("%s artifact slot %s fields=%#v", entry.Name, slot, fields)
			}
		}
	}
}

func TestEveryCurrentActionProjectsItsSingleSubmissionTool(t *testing.T) {
	definition := workflow.StandardProcess()
	issuedAt := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	workspace := domain.WorkspaceDigests{Binding: repeatedDigest('a'), Identity: repeatedDigest('b'), History: repeatedDigest('c'), Content: repeatedDigest('d')}
	for _, node := range definition.Nodes {
		if node.NodeID.Terminal() {
			continue
		}
		action, err := workflow.BuildProcessActionForWorkspace(definition, node.NodeID, "task", 1, workspace, domain.MethodPlain, domain.ID("action-"+strings.ToLower(string(node.NodeID))), issuedAt)
		if err != nil {
			t.Fatal(err)
		}
		projection := projectAction(&action).(map[string]any)
		tool, ok := projection["submission_tool"].(string)
		if !ok || tool == "" || !isToolName(tool) {
			t.Fatalf("node %s submission tool=%#v", node.NodeID, projection["submission_tool"])
		}
		for key, want := range map[string]domain.Digest{"repository_binding_digest": workspace.Binding, "issuance_identity_digest": workspace.Identity, "issuance_history_digest": workspace.History, "issuance_content_digest": workspace.Content} {
			if projection[key] != want {
				t.Fatalf("node %s %s=%v want=%s", node.NodeID, key, projection[key], want)
			}
		}
	}
}

func TestStandardNodeResultsAreSemanticOnly(t *testing.T) {
	payloads, _ := graphPayloads()
	if len(payloads) != 9 {
		t.Fatalf("payloads=%d", len(payloads))
	}
	for index, name := range []string{"requirements", "design", "tasks", "implementation", "test", "comprehension", "refactor", "delivery"} {
		payload := payloads[index].(map[string]any)
		properties := payload["properties"].(map[string]any)
		nodeResult := properties["node_result"].(map[string]any)
		nodeProperties := nodeResult["properties"].(map[string]any)
		if nodeProperties["changed_paths"] != nil || nodeProperties["no_file_changes"] != nil {
			t.Fatalf("%s exposes legacy repository effect members: %v", name, nodeProperties)
		}
	}
}

func containsSchemaMember(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestMultiRepositoryTaskProjectionIsSortedAndUsesOneDigest(t *testing.T) {
	now := time.Date(2026, 8, 23, 7, 0, 0, 0, time.UTC)
	corePath, apiPath, docsPath := testPath("core"), testPath("api"), testPath("docs")
	primary := graphContractBinding(now, corePath, 'a')
	primaryOrigin := graphContractOrigin(corePath, 'a')
	api := graphContractBinding(now, apiPath, 'b')
	apiOrigin := graphContractOrigin(apiPath, 'b')
	docs := graphContractBinding(now, docsPath, 'c')
	docsOrigin := graphContractOrigin(docsPath, 'c')
	task := domain.ProcessTask{
		PrimaryRepositoryKey: "core",
		WorkspaceOrigin:      primaryOrigin,
		Repository:           primary,
		AdditionalRepositories: []domain.RepositoryScopeEntry{
			{Key: "docs", Origin: docsOrigin, Binding: docs},
			{Key: "api", Origin: apiOrigin, Binding: api},
		},
	}
	effectiveTask := task
	effectiveTask.AdditionalRepositories = []domain.RepositoryScopeEntry{{Key: "api", Origin: apiOrigin, Binding: api}, {Key: "docs", Origin: docsOrigin, Binding: docs}}
	effective, err := effectiveTask.EffectiveWorkspaceDigests()
	if err != nil {
		t.Fatal(err)
	}
	task.CurrentAction = &domain.ProcessAction{RepositoryBindingDigest: effective.Binding, IssuanceIdentityDigest: effective.Identity, IssuanceHistoryDigest: effective.History, IssuanceContentDigest: effective.Content}
	raw, err := json.Marshal(projectTask(task))
	if err != nil {
		t.Fatal(err)
	}
	var projection map[string]any
	if json.Unmarshal(raw, &projection) != nil {
		t.Fatal("invalid task projection")
	}
	if projection["primary_repository_key"] != "core" || projection["workspace_origin"].(map[string]any)["canonical_worktree_root"] != corePath {
		t.Fatalf("primary projection=%#v", projection)
	}
	additional, ok := projection["additional_repositories"].([]any)
	if !ok || len(additional) != 2 || additional[0].(map[string]any)["key"] != "api" || additional[1].(map[string]any)["key"] != "docs" {
		t.Fatalf("additional projection=%#v", projection["additional_repositories"])
	}
	if bytes.Count(raw, []byte(`"repository_binding_digest"`)) != 1 || bytes.Contains(raw, []byte("repository_scope_digest")) || !bytes.Contains(raw, []byte(effective.Binding)) {
		t.Fatalf("digest projection=%s", raw)
	}

	single := projectTask(domain.ProcessTask{WorkspaceOrigin: primaryOrigin, Repository: primary}).(map[string]any)
	if single["primary_repository_key"] != domain.DefaultPrimaryRepositoryKey || single["workspace_origin"].(domain.WorkspaceOrigin).CanonicalWorktreeRoot != corePath {
		t.Fatalf("single repository projection=%#v", single)
	}
	if _, exists := single["additional_repositories"]; exists {
		t.Fatal("single repository projection added an empty collection")
	}
}

func TestOpenTaskMultiRepositoryWireMapsApplicationRequest(t *testing.T) {
	raw := []byte(`{"host":"codex","repository_path":"/core","workspace_origin":{"mode":"dedicated_worktree","source_type":"remote","carry_changes":false,"remote_name":"origin","base_branch":"main","base_commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","task_branch":"feature/core","provisioning_receipt_id":"receipt-core"},"primary_repository_key":"core","additional_repositories":[{"key":"docs","repository_path":"/docs","workspace_origin":{"mode":"dedicated_worktree","source_type":"remote","carry_changes":false,"remote_name":"origin","base_branch":"main","base_commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","task_branch":"feature/docs","provisioning_receipt_id":"receipt-docs"}}],"new_task":{"request":"Build feature","initial_scope":[],"initial_out_of_scope":[],"known_acceptance_criteria":[],"method_profile":"plain"}}`)
	if err := ValidateToolInput(ToolOpenTask, raw); err != nil {
		t.Fatal(err)
	}
	var wire openWire
	if err := decodeClosed(raw, &wire); err != nil {
		t.Fatal(err)
	}
	request := toOpen(wire, "request-mapped")
	if request.RequestID != "request-mapped" || request.RepositoryPath != "/core" || request.WorkspaceOrigin == nil || request.WorkspaceOrigin.TaskBranch != "feature/core" || request.PrimaryRepositoryKey != "core" || len(request.AdditionalRepositories) != 1 || request.AdditionalRepositories[0].Key != "docs" || request.AdditionalRepositories[0].RepositoryPath != "/docs" || request.AdditionalRepositories[0].WorkspaceOrigin.TaskBranch != "feature/docs" || request.NewTask == nil {
		t.Fatalf("mapped request=%#v", request)
	}
}

func TestWorkspaceLifecycleToolInputsAreClosed(t *testing.T) {
	valid := map[string][]byte{
		ToolPrepareTaskRelocation:          []byte(`{"host":"codex","task_id":"task","revision":4}`),
		ToolAbandonTask:                    []byte(`{"host":"deepseek","task_id":"task","revision":4,"reason":"The original worktree instance is unavailable."}`),
		ToolResolveBlocker + "/relocation": []byte(`{"host":"codex","task_id":"task","action_id":"action","relocation_id":"relocation","relocation_destinations":[{"key":"primary","repository_path":"/worktree"}]}`),
		ToolResolveBlocker + "/history":    []byte(`{"host":"codex","task_id":"task","action_id":"action","history_resolution":{"choice":"accept_current_history","reason":"The rewritten history was reviewed."}}`),
	}
	for label, raw := range valid {
		tool := strings.Split(label, "/")[0]
		if err := ValidateToolInput(tool, raw); err != nil {
			t.Fatalf("%s rejected: %v", label, err)
		}
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			t.Fatal("invalid test input")
		}
		value["destination"] = "DONE"
		invalid, _ := json.Marshal(value)
		if err := ValidateToolInput(tool, invalid); err == nil {
			t.Fatalf("%s accepted an unknown destination", label)
		}
	}
}

func graphContractBinding(now time.Time, root string, marker byte) domain.RepositoryBinding {
	digest := domain.Digest(strings.Repeat(string(marker), 64))
	branch := "feature/" + string(marker)
	head := strings.Repeat(string(marker), 40)
	return domain.RepositoryBinding{WorktreeInstanceDigest: digest, IdentityDigest: digest, HistoryDigest: digest, ContentDigest: digest, CurrentBranch: &branch, CurrentHead: head, HeadTree: head, HistoryRelation: domain.RepositoryHistoryExact, BaseCommitAncestor: true, ObservedAt: now, BindingDigest: digest}
}

func graphContractOrigin(root string, marker byte) domain.WorkspaceOrigin {
	digest := repeatedDigest(marker)
	return domain.WorkspaceOrigin{Mode: domain.WorkspaceModeDedicatedWorktree, SourceType: "remote", RemoteName: "origin", BaseBranch: "main", BaseCommit: strings.Repeat(string(marker), 40), TaskBranch: "feature/" + string(marker), SourceRepositoryGroupDigest: digest, CanonicalWorktreeRoot: root, WorktreeGitDirDigest: digest, ProvisioningReceiptID: domain.ID("receipt-" + string(marker))}
}

func repeatedDigest(marker byte) domain.Digest {
	return domain.Digest(strings.Repeat(string(marker), 64))
}
