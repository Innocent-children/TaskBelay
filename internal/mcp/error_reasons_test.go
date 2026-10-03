package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestEveryToolReportsJSONFailureReasonsBeforeExecution(t *testing.T) {
	cases := []struct {
		name, raw, path string
		rule            domain.ViolationRule
	}{
		{"malformed", `{"private":`, "arguments", domain.RuleJSONMalformed},
		{"not object", `[]`, "arguments", domain.RuleArgumentsObjectRequired},
		{"invalid UTF8", "{\"private\":\"\xff\"}", "arguments", domain.RuleUTF8Required},
		{"duplicate", `{"host":"private-value","host":"codex"}`, "host", domain.RuleDuplicateMember},
		{"unsafe member", `{"/private/token":"private-value","/private/token":null}`, "arguments", domain.RuleDuplicateMember},
	}
	if len(ToolNames()) != 18 {
		t.Fatal("update failure coverage for the tool catalog")
	}
	for _, tool := range ToolNames() {
		for _, tc := range cases {
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				encoded := (&Server{}).dispatch(context.Background(), tool, "request-diagnostic", []byte(tc.raw))
				response := decodeEnvelope(t, encoded)
				if !encoded.IsError || response.Tool != tool || response.RequestID != "request-diagnostic" || response.Error.Code != domain.ErrorInvalidArgument || len(response.Error.Details) != 1 {
					t.Fatalf("response=%s", encoded.JSON)
				}
				detail := response.Error.Details[0]
				if detail.Path != tc.path || detail.Rule != tc.rule || !strings.Contains(response.Error.Message, detail.Path+": ") || detail.Message == "" {
					t.Fatalf("reason=%s", encoded.JSON)
				}
				if strings.Contains(string(encoded.JSON), "private-value") || strings.Contains(string(encoded.JSON), "/private/token") {
					t.Fatal("request content leaked")
				}
				if response.Recovery.RetrySafe {
					t.Fatal("ambiguous JSON must not grant automatic correction")
				}
				validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(tool)), encoded.JSON)
			})
		}
	}
}

func TestEveryToolReportsWrongMemberTypes(t *testing.T) {
	seen := map[string]bool{}
	for _, example := range readSkillExamples(t, "codex") {
		if example.Kind != "mcp" || seen[example.Tool] {
			continue
		}
		seen[example.Tool] = true
		t.Run(example.Tool, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal(example.Input, &input); err != nil {
				t.Fatal(err)
			}
			input["host"] = map[string]any{"private": "do-not-return"}
			encoded := (&Server{}).dispatch(context.Background(), example.Tool, "request-types", mustSchemaJSON(t, input))
			response := decodeEnvelope(t, encoded)
			want := domain.RuleValueType
			if example.Tool == ToolServerInfo {
				want = domain.RuleUnknownMember
			}
			if !encoded.IsError || len(response.Error.Details) != 1 || response.Error.Details[0].Path != "host" || response.Error.Details[0].Rule != want {
				t.Fatalf("response=%s", encoded.JSON)
			}
			if strings.Contains(string(encoded.JSON), "do-not-return") {
				t.Fatal("request value leaked")
			}
			validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(example.Tool)), encoded.JSON)
		})
	}
	if len(seen) != 18 {
		t.Fatalf("tested %d tools", len(seen))
	}
}

func TestNestedSubmissionAndPlanFailureReasons(t *testing.T) {
	examples := readSkillExamples(t, "codex")
	for _, scenarioName := range []string{"nested type", "dependency cycle"} {
		t.Run(scenarioName, func(t *testing.T) {
			var request skillExample
			for _, example := range examples {
				if example.Kind == "mcp" && example.Tool == ToolSubmitTasks && example.Name == "tasks_plan_saved" {
					request = example
					break
				}
			}
			if request.Tool == "" {
				t.Fatal("missing task-plan scenario")
			}
			scenario := newSkillScenario(t, "codex", examples)
			scenario.prepare(request)
			before := scenario.task.Revision
			var input map[string]any
			if err := json.Unmarshal(scenario.bind(request), &input); err != nil {
				t.Fatal(err)
			}
			work := input["node_result"].(map[string]any)["baseline"].(map[string]any)["work_items"].([]any)[0].(map[string]any)
			path, fragment := "node_result.baseline.work_items[0].summary", "JSON type string"
			if scenarioName == "nested type" {
				work["summary"] = false
			} else {
				work["dependencies"] = []any{work["work_item_id"]}
				path, fragment = "node_result.baseline.work_items[0].dependencies[0]", "cannot depend on itself"
			}
			encoded := scenario.server.dispatch(context.Background(), ToolSubmitTasks, "request-nested", mustSchemaJSON(t, input))
			response := decodeEnvelope(t, encoded)
			if !encoded.IsError || response.Error.Code != domain.ErrorInvalidArgument || !strings.Contains(response.Error.Message, path) || !strings.Contains(response.Error.Message, fragment) {
				t.Fatalf("response=%s", encoded.JSON)
			}
			after, err := scenario.database.LoadTask(context.Background(), scenario.task.TaskID)
			if err != nil || after.Revision != before {
				t.Fatalf("failed validation changed Task: %v", err)
			}
			validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(ToolSubmitTasks)), encoded.JSON)
		})
	}
}

func TestStorageDecodeReasonSurvivesApplicationAndMCP(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "task.db")
	database, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	origin, binding, selection := mcpWorkspaceFixture(time.Now().UTC(), testPath("diagnostic-repo"), 'a')
	service, err := application.NewService(database, recoveryProjectionObserver{origin: origin, binding: binding})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenTask(ctx, application.OpenTaskRequest{RequestID: "request-open", Host: domain.HostCodex, RepositoryPath: testPath("diagnostic-repo"), WorkspaceOrigin: &selection, NewTask: &application.NewTaskInput{Request: "Check diagnostics", MethodProfile: domain.MethodPlain}})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, `UPDATE tasks SET snapshot=? WHERE task_id=?`, []byte(`{"private-secret":`), opened.Task.TaskID); err != nil {
		t.Fatal(err)
	}
	encoded := (&Server{application: service}).dispatch(ctx, ToolGetTask, "request-storage", mustSchemaJSON(t, map[string]any{"host": "codex", "task_id": opened.Task.TaskID}))
	response := decodeEnvelope(t, encoded)
	if !encoded.IsError || response.Error.Code != domain.ErrorStorageUnavailable || !strings.Contains(response.Error.Message, "saved Task snapshot") {
		t.Fatalf("response=%s", encoded.JSON)
	}
	if strings.Contains(string(encoded.JSON), path) || strings.Contains(string(encoded.JSON), "private-secret") || response.Recovery.RetrySafe {
		t.Fatalf("unsafe response=%s", encoded.JSON)
	}
	validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(ToolGetTask)), encoded.JSON)
}

func TestEveryToolPreservesIdentityOnResponseEncodingFailure(t *testing.T) {
	for _, tool := range ToolNames() {
		for _, tc := range []struct {
			name   string
			result any
			reason string
		}{
			{"empty", nil, "no result"}, {"encoding", make(chan int), "encode"}, {"size", strings.Repeat("x", domain.MaxResultEnvelopeBytes), "byte limit"},
		} {
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				encoded := EncodeSuccess("request-original", tool, tc.result)
				response := decodeEnvelope(t, encoded)
				if !encoded.IsError || response.Tool != tool || response.RequestID != "request-original" || !strings.Contains(response.Error.Message, tc.reason) || response.Recovery.RetrySafe {
					t.Fatalf("response=%s", encoded.JSON)
				}
				validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(tool)), encoded.JSON)
			})
		}
	}
}

func TestRequestIdentityFailureReturnsTheActualToolOverMCP(t *testing.T) {
	base := missingTaskSkillServer(t)
	server, err := NewServer(base.application, "test", &ServerOptions{NewRequestID: func() (domain.ID, error) { return "", errors.New("/private/entropy-device") }})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	go server.Run(ctx, serverTransport)
	client := sdk.NewClient(&sdk.Implementation{Name: "diagnostic-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, tool := range ToolNames() {
		t.Run(tool, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: map[string]any{}})
			if err != nil {
				t.Fatal(err)
			}
			text := result.Content[0].(*sdk.TextContent).Text
			var response Envelope
			if err := json.Unmarshal([]byte(text), &response); err != nil {
				t.Fatal(err)
			}
			if !result.IsError || response.Tool != tool || response.RequestID != "request-unavailable" || !strings.Contains(response.Error.Message, "tool was not executed") || strings.Contains(text, "/private/") {
				t.Fatalf("response=%s", text)
			}
			validateSkillSchema(t, mustSchemaJSON(t, toolOutputSchema(tool)), []byte(text))
		})
	}
}

func TestUnsafeUnknownMembersDoNotAuthorizeCorrection(t *testing.T) {
	for _, tc := range []struct {
		tool string
		raw  []byte
		path string
	}{
		{ToolServerInfo, []byte(`{"/private/token":"private-value"}`), "arguments"},
		{ToolSubmitDesign, submissionInput(t, domain.ActionCompleteDesign, map[string]any{"problem_class": "none", "baseline": designBaseline("Direct approach"), "findings": []any{}, "/private/token": "private-value"}), "node_result"},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			encoded := (&Server{}).dispatch(context.Background(), tc.tool, "request-unsafe", tc.raw)
			response := decodeEnvelope(t, encoded)
			if !encoded.IsError || len(response.Error.Details) != 1 || response.Error.Details[0].Path != tc.path || response.Error.Details[0].Rule != domain.RuleUnsafeMemberName || response.Recovery.RetrySafe || len(response.Recovery.AllowedPaths) != 0 {
				t.Fatalf("response=%s", encoded.JSON)
			}
			if strings.Contains(string(encoded.JSON), "/private/token") || strings.Contains(string(encoded.JSON), "private-value") {
				t.Fatal("unsafe member leaked")
			}
		})
	}
}

func TestOpenWorkspaceReasonRetainsProvisioningCategory(t *testing.T) {
	for _, example := range readSkillExamples(t, "codex") {
		if example.Kind != "mcp" || example.Tool != ToolOpenTask || example.Name != "create" {
			continue
		}
		var input map[string]any
		if err := json.Unmarshal(example.Input, &input); err != nil {
			t.Fatal(err)
		}
		origin := input["workspace_origin"].(map[string]any)
		origin["mode"] = "invalid-mode"
		encoded := (&Server{}).dispatch(context.Background(), ToolOpenTask, "request-origin", mustSchemaJSON(t, input))
		response := decodeEnvelope(t, encoded)
		if !encoded.IsError || response.Error.Code != domain.ErrorWorktreeProvisioningRequired || !strings.Contains(response.Error.Message, "workspace_origin.mode") || response.Recovery.Action != "provision_worktree" {
			t.Fatalf("response=%s", encoded.JSON)
		}
		return
	}
	t.Fatal("missing creation example")
}

func TestProbePreservesRetainedPayloadFailureReason(t *testing.T) {
	process := workflow.StandardProcess().Reference
	digest := strings.Repeat("a", 64)
	payload := map[string]any{"transition_id": "requirements_ready", "summary": "Requirements captured.", "reason": "", "artifacts": []any{}, "method_evidence": []any{}, "node_result": map[string]any{"problem_class": "none", "baseline": map[string]any{"goal": "Explain errors", "scope": []any{}, "out_of_scope": []any{}, "acceptance_criteria": []any{"Errors include reasons"}, "constraints": []any{}, "assumptions": []any{}}, "unresolved_questions": []any{}}}
	probe := map[string]any{"operation_id": "operation", "process_id": process.ID, "process_definition_digest": process.DefinitionDigest, "source_cursor": "REQUIREMENTS", "expected_revision": 1, "action_id": "action", "action_kind": domain.ActionCompleteRequirements, "repository_binding_digest": digest, "issuance_identity_digest": digest, "issuance_history_digest": digest, "issuance_content_digest": digest, "payload": payload}
	encoded := (&Server{}).dispatch(context.Background(), ToolGetTask, "request-probe", mustSchemaJSON(t, map[string]any{"host": "codex", "task_id": "task", "operation_probe": probe}))
	response := decodeEnvelope(t, encoded)
	if !encoded.IsError || !strings.Contains(response.Error.Message, "operation_probe.payload") || !strings.Contains(response.Error.Message, "exactly one result for every required Action method step") || response.Recovery.RetrySafe {
		t.Fatalf("response=%s", encoded.JSON)
	}
}
