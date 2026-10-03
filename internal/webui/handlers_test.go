package webui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/mcp"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func TestActionCorrectionMatchesMCPForRejectedSemanticPayload(t *testing.T) {
	action, err := workflow.BuildProcessAction(workflow.StandardProcess(), domain.NodeRequirements, "task", 1, domain.Digest(strings.Repeat("a", 64)), domain.MethodPlain, "action", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"transition_id":"requirements_ready","reason":"","artifacts":{"current":[],"other_process":[]},"method_results":{"requirements.capture":{"capability":"","summary":"Captured."},"requirements.clarify":{"capability":"","summary":"Clarified."},"requirements.validate":{"capability":"","summary":"Validated."}},"node_result":{"problem_class":"none","baseline":{"goal":"Correct errors consistently.","scope":["Error responses"],"out_of_scope":[],"acceptance_criteria":["Both adapters allow the same correction."],"constraints":[],"assumptions":[]},"unresolved_questions":[]}}`)
	failure := workflow.ValidateCurrentSubmission(action, nil, payload)
	if failure == nil {
		t.Fatal("missing summary was accepted")
	}
	response := httptest.NewRecorder()
	writeActionError(response, "request-correction", failure, false)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", response.Code)
	}
	wantHTTP := `{"ok":false,"request_id":"request-correction","workflow_write_state":"not_committed","error":{"details":[{"path":"payload.summary","rule":"required_member_missing","message":"the closed contract requires this member"}],"code":"INVALID_ARGUMENT","message":"the domain value is invalid","field_paths":["payload.summary"],"guard_id":null},"recovery":{"allowed_paths":["payload.summary"],"action":"correct_current_action","retry_safe":true,"message":"Correct only allowed_paths using established facts and resubmit once while this Action identity remains current. Do not guess a user decision; stop if the correction fails."}}`
	wantMCP := `{"ok":false,"request_id":"request-correction","tool":"taskbelay_submit_requirements","error":{"code":"INVALID_ARGUMENT","message":"summary: the closed contract requires this member","details":[{"path":"summary","rule":"required_member_missing","message":"the closed contract requires this member"}]},"recovery":{"retry_safe":true,"action":"correct_current_action","message":"Correct only the members listed in allowed_paths, using facts already confirmed in the current Action work, and resubmit through the same submission tool once. Do not re-expand requirements, change more code, or guess a user decision; stop when the resubmission fails.","allowed_paths":["summary"]}}`
	for name, pair := range map[string][2][]byte{
		"HTTP": {response.Body.Bytes(), []byte(wantHTTP)},
		"MCP":  {mcp.EncodeError("request-correction", mcp.ToolSubmitRequirements, failure).JSON, []byte(wantMCP)},
	} {
		t.Run(name, func(t *testing.T) {
			var actual, expected any
			if err := json.Unmarshal(pair[0], &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(pair[1], &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("response=%s\nwant=%s", pair[0], pair[1])
			}
		})
	}
}

func TestActionCorrectionParityForCurrentFactsAndRejectedDecisions(t *testing.T) {
	cases := []struct {
		name    string
		failure error
		allowed []string
	}{
		{"current set", domain.InvalidArgumentViolations(domain.Violation("payload.node_result.manual_evidence_ids", domain.RuleCurrentSetRequired)), []string{"payload.node_result.manual_evidence_ids"}},
		{"guard", domain.TransitionGuardFailure("delivery_current_and_complete", domain.GuardViolation("payload.node_result.test_record_id", domain.GuardCurrentValueRequired)), []string{"payload.node_result.test_record_id"}},
		{"user decision", domain.TransitionGuardFailure("current_user_comprehension_confirmed", domain.GuardViolation("payload.node_result.user_confirmation", domain.GuardUserConfirmationRequired)), nil},
		{"mixed", domain.InvalidArgumentViolations(domain.Violation("payload.node_result.summary", domain.RuleRequiredMemberMissing), domain.Violation("payload.node_result.checks[0].status", domain.RuleEvidenceStatusInvalid)), nil},
		{"uncertain", domain.WithoutZeroWriteProof(domain.InvalidArgumentViolations(domain.Violation("payload.summary", domain.RuleRequiredMemberMissing))), nil},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			writeActionError(response, "request-parity", item.failure, false)
			var httpResult FailureResponse
			var mcpResult mcp.Envelope
			if err := json.Unmarshal(response.Body.Bytes(), &httpResult); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(mcp.EncodeError("request-parity", mcp.ToolSubmitDelivery, item.failure).JSON, &mcpResult); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(httpResult.Recovery.AllowedPaths, item.allowed) || httpResult.Recovery.RetrySafe != (len(item.allowed) != 0) || httpResult.Recovery.RetrySafe != mcpResult.Recovery.RetrySafe {
				t.Fatalf("HTTP=%+v MCP=%+v", httpResult.Recovery, mcpResult.Recovery)
			}
			for index, path := range item.allowed {
				if mcpResult.Recovery.AllowedPaths[index] != strings.TrimPrefix(path, "payload.") {
					t.Fatalf("paths=%v", mcpResult.Recovery.AllowedPaths)
				}
			}
		})
	}
}

func TestActionCorrectionRejectsMissingDecisionMembersFromCurrentSchema(t *testing.T) {
	cases := []struct {
		name       string
		node       domain.NodeID
		blocker    *domain.ProcessBlocker
		payload    map[string]any
		nodeResult map[string]any
		path       string
		tool       string
	}{
		{name: "file scope choice", node: domain.NodeBlocked, blocker: &domain.ProcessBlocker{Cause: domain.BlockerCauseFileScopeDecision}, payload: map[string]any{"reason": "The paths need a user choice."}, path: "payload.choice", tool: mcp.ToolResolveBlocker},
		{name: "history decision", node: domain.NodeBlocked, blocker: &domain.ProcessBlocker{Cause: domain.BlockerCauseWorkspaceHistoryConflict}, payload: map[string]any{}, path: "payload.history_resolution", tool: mcp.ToolResolveBlocker},
		{name: "plan confirmation", node: domain.NodeTasks, nodeResult: map[string]any{"baseline": nil, "findings": []string{}, "problem_class": "none"}, path: "payload.node_result.user_confirmation", tool: mcp.ToolSubmitTasks},
		{name: "understanding confirmation", node: domain.NodeComprehensionReview, nodeResult: map[string]any{"explained_components": []string{"Core"}, "findings": []string{}, "maintenance_risks": []string{}, "unnecessary_abstractions": []string{}, "unresolved_questions": []string{}, "problem_class": "none"}, path: "payload.node_result.user_confirmation", tool: mcp.ToolSubmitComprehension},
		{name: "whole result", node: domain.NodeTasks, path: "payload.node_result", tool: mcp.ToolSubmitTasks},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			action, err := workflow.BuildProcessAction(workflow.StandardProcess(), item.node, "task", 1, domain.Digest(strings.Repeat("a", 64)), domain.MethodPlain, "action", time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			payload := item.payload
			if payload == nil {
				methods := map[string]any{}
				for _, step := range action.SemanticMethodSteps {
					methods[string(step.StepID)] = map[string]any{"capability": "", "summary": "Current work recorded."}
				}
				payload = map[string]any{"transition_id": action.AvailableTransitions[0].TransitionID, "summary": "Submitted current facts.", "reason": "", "artifacts": map[string]any{"current": []any{}, "other_process": []any{}}, "method_results": methods}
				if item.nodeResult != nil {
					payload["node_result"] = item.nodeResult
				}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			failure := workflow.ValidateCurrentSubmission(action, item.blocker, raw)
			if failure == nil || !reflect.DeepEqual(domain.ViolationPaths(failure), []string{item.path}) {
				t.Fatalf("failure=%v paths=%v", failure, domain.ViolationPaths(failure))
			}
			response := httptest.NewRecorder()
			writeActionError(response, "request-decision", failure, false)
			var httpResult FailureResponse
			var mcpResult mcp.Envelope
			if err := json.Unmarshal(response.Body.Bytes(), &httpResult); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(mcp.EncodeError("request-decision", item.tool, failure).JSON, &mcpResult); err != nil {
				t.Fatal(err)
			}
			if httpResult.Recovery.RetrySafe || len(httpResult.Recovery.AllowedPaths) != 0 || mcpResult.Recovery.RetrySafe || len(mcpResult.Recovery.AllowedPaths) != 0 {
				t.Fatalf("missing decision: HTTP=%+v MCP=%+v", httpResult.Recovery, mcpResult.Recovery)
			}
		})
	}
}

func TestActionErrorShowsMissingRepositoryPathsAndCorrection(t *testing.T) {
	failure := domain.InvalidArgumentViolations(domain.Violation("artifacts.other_process", domain.RuleArtifactManifestIncomplete))
	failure.RepositoryPaths = []string{"openspec/config.yaml"}
	response := httptest.NewRecorder()
	writeActionError(response, "artifact-rejection", failure, true)
	var body FailureResponse
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || body.WorkflowWriteState != "not_committed" ||
		len(body.Error.RepositoryPaths) != 1 || body.Error.RepositoryPaths[0] != "openspec/config.yaml" ||
		len(body.Error.FieldPaths) != 1 || body.Error.FieldPaths[0] != "payload.artifacts.other_process" ||
		body.Recovery.Action != RecoveryCorrectCurrentAction || !body.Recovery.RetrySafe ||
		!reflect.DeepEqual(body.Recovery.AllowedPaths, []string{"payload.artifacts.other_process"}) {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestActionErrorShowsCheckExplanationAndBudgetDetails(t *testing.T) {
	failure := domain.InvalidArgumentViolations(domain.Violation("payload.node_result.budget_adjustment.additional_checks", domain.RuleBudgetChecksRequired))
	response := httptest.NewRecorder()
	writeActionError(response, "check-explanation", failure, true)
	var body FailureResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Recovery.RetrySafe || body.Recovery.Action != RecoveryCorrectCurrentAction || len(body.Error.Details) != 1 || body.Error.Details[0].Rule != domain.RuleBudgetChecksRequired {
		t.Fatalf("body=%s", response.Body.String())
	}
	failure = &domain.Error{Code: domain.ErrorVerificationBudgetExceeded, Message: "Automatic commands exceed the limit.", Budget: &domain.BudgetFailure{Used: 8, Requested: 6, Limit: 13}}
	response = httptest.NewRecorder()
	writeActionError(response, "budget-exceeded", failure, true)
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Budget == nil || body.Error.Budget.Limit != 13 {
		t.Fatalf("body=%s", response.Body.String())
	}
}

func TestZCodeHostFilterAndResumeRetainOriginIdentity(t *testing.T) {
	reader := &hostFilterReader{}
	mutator := &stubControlCenterMutator{}
	api, err := NewAPI(reader, mutator, func() SystemStatusResponse { return SystemStatusResponse{Readiness: ReadinessReady} })
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tasks?host=zcode", nil))
	var listed TaskListResponse
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &listed) != nil || !listed.OK || reader.host != domain.HostZCode || len(listed.Items) != 1 || listed.Items[0].OriginHost != "zcode" || listed.Items[0].ExecutionHost != "zcode" {
		t.Fatalf("filter host=%s response=%s", reader.host, response.Body.String())
	}
	response = httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/tasks/resume", strings.NewReader(`{"request_id":"resume-zcode","execution_host":"zcode","repository_path":"/worktrees/zcode","csrf":"`+strings.Repeat("s", 32)+`"}`)))
	if response.Code != http.StatusOK || mutator.lastOpen.Host != domain.HostZCode || mutator.lastOpen.RepositoryPath != "/worktrees/zcode" || mutator.lastOpen.NewTask != nil {
		t.Fatalf("resume=%+v response=%s", mutator.lastOpen, response.Body.String())
	}
}

type hostFilterReader struct {
	stubControlCenterReader
	host domain.Host
}

func (r *hostFilterReader) ListTasks(_ context.Context, request application.ListControlCenterTasksRequest) (application.ControlCenterTaskList, error) {
	r.host = request.Filter.Host
	return application.ControlCenterTaskList{Page: 1, Items: []application.ControlCenterTaskSummary{{
		TaskID: "zcode-task", OriginHost: domain.HostZCode, ExecutionHost: domain.HostZCode,
	}}}, nil
}

func TestLifecycleHandlersCP2(t *testing.T) {
	mutator := &stubControlCenterMutator{}
	api, err := NewAPI(&stubControlCenterReader{}, mutator, func() SystemStatusResponse { return SystemStatusResponse{Readiness: ReadinessReady} })
	if err != nil {
		t.Fatal(err)
	}
	csrf := strings.Repeat("s", 32)
	cases := []struct {
		name string
		path string
		body string
		call string
	}{
		{"resume", "/api/tasks/resume", `{"request_id":"resume-request","execution_host":"codex","repository_path":"/worktrees/task","csrf":"` + csrf + `"}`, "open"},
		{"prepare relocation", "/api/tasks/task/relocation/prepare", `{"request_id":"relocation-request","execution_host":"codex","task_revision":1,"confirmed":true,"csrf":"` + csrf + `"}`, "relocation"},
		{"abandon", "/api/tasks/task/abandon", `{"request_id":"abandon-request","execution_host":"codex","task_revision":1,"reason":"Original worktree is unavailable.","confirmed":true,"csrf":"` + csrf + `"}`, "abandon"},
		{"cancel", "/api/tasks/task/cancel", `{"request_id":"cancel-request","task_revision":1,"reason":"Stop task.","confirmed":true,"csrf":"` + csrf + `"}`, "cancel"},
		{"archive", "/api/tasks/task/archive", `{"request_id":"archive-request","task_revision":2,"archived":true,"csrf":"` + csrf + `"}`, "archive"},
		{"purge", "/api/tasks/task/purge", `{"request_id":"purge-request","task_revision":2,"typed_task_id":"task","reason":"Remove task.","irreversible":true,"csrf":"` + csrf + `"}`, "purge"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var body MutationResponse
			if json.Unmarshal(response.Body.Bytes(), &body) != nil || !body.OK || mutator.lastCall != test.call {
				t.Fatalf("body=%#v call=%s", body, mutator.lastCall)
			}
			if test.call == "relocation" && (body.RelocationID == nil || *body.RelocationID != "relocation") {
				t.Fatalf("relocation body=%#v", body)
			}
		})
	}

	t.Run("unknown member is rejected", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/tasks/task/archive", strings.NewReader(`{"request_id":"archive-request","task_revision":2,"archived":true,"csrf":"`+csrf+`","unknown":true}`))
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("shared-checkout create route is absent", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/tasks/open", strings.NewReader(`{"csrf":"`+csrf+`"}`))
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("stale mutation returns reload advice", func(t *testing.T) {
		mutator.stale = true
		request := httptest.NewRequest(http.MethodPost, "/api/tasks/task/cancel", strings.NewReader(`{"request_id":"stale-request","task_revision":1,"reason":"Stop task.","confirmed":true,"csrf":"`+csrf+`"}`))
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		var body FailureResponse
		if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != http.StatusConflict || body.Error.Code != string(domain.ErrorRevisionConflict) || body.Recovery.Action != RecoveryReadNextAction {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestActionHandlersCP3(t *testing.T) {
	mutator := &stubControlCenterMutator{}
	api, err := NewAPI(&stubControlCenterReader{}, mutator, func() SystemStatusResponse { return SystemStatusResponse{Readiness: ReadinessReady} })
	if err != nil {
		t.Fatal(err)
	}

	csrf := strings.Repeat("s", 32)
	payload := `{"transition_id":"requirements_ready"}`
	cases := []struct{ name, path, body, call string }{
		{"submit", "/api/tasks/task/actions/submit", `{"request_id":"action-request","task_revision":1,"action_id":"action","payload":` + payload + `,"csrf":"` + csrf + `"}`, "submit"},
		{"assess", "/api/tasks/task/recovery/assess", `{"action_id":"action","csrf":"` + csrf + `"}`, "assess"},
		{"apply", "/api/tasks/task/recovery/apply", `{"action_id":"action","csrf":"` + csrf + `"}`, "recover"},
		{"resolve blocker", "/api/tasks/task/actions/submit", `{"request_id":"blocker-request","task_revision":2,"action_id":"blocker-action","payload":{"choice":"allow_once","reason":"Approve exact requested paths."},"csrf":"` + csrf + `"}`, "submit"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
			if response.Code != http.StatusOK || mutator.lastCall != test.call {
				t.Fatalf("status=%d call=%s body=%s", response.Code, mutator.lastCall, response.Body.String())
			}
			var body MutationResponse
			if json.Unmarshal(response.Body.Bytes(), &body) != nil || !body.OK {
				t.Fatalf("body=%s", response.Body.String())
			}
			if test.call == "assess" && (body.Recovery == nil || body.Recovery.Action != RecoverySubmitRecoveryApply) {
				t.Fatalf("assessment body=%#v", body)
			}
		})
	}

	t.Run("field correction envelope", func(t *testing.T) {
		mutator.actionErr = domain.InvalidArgumentViolations(domain.Violation("payload.node_result.unknown", domain.RuleUnknownMember))
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, cases[0].path, strings.NewReader(cases[0].body)))
		var body FailureResponse
		if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != http.StatusBadRequest || body.Recovery.Action != RecoveryCorrectCurrentAction || !body.Recovery.RetrySafe || len(body.Error.FieldPaths) != 1 {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("guard envelope", func(t *testing.T) {
		mutator.actionErr = domain.TransitionGuardFailure("implementation_report_complete", domain.GuardViolation("payload.node_result.findings", domain.GuardForwardFindingsEmpty))
		response := httptest.NewRecorder()
		api.ServeHTTP(response, httptest.NewRequest(http.MethodPost, cases[0].path, strings.NewReader(cases[0].body)))
		var body FailureResponse
		if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != http.StatusConflict || body.Error.GuardID == nil || *body.Error.GuardID != "implementation_report_complete" || body.Recovery.Action != RecoveryCorrectCurrentAction {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}

func TestFilterOptionsUseCurrentWorkflowDefinition(t *testing.T) {
	api, err := NewReadAPI(&stubControlCenterReader{}, func() SystemStatusResponse { return SystemStatusResponse{Readiness: ReadinessReady} })
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/system/filter-options", nil))
	var body FilterOptionsResponse
	if json.Unmarshal(response.Body.Bytes(), &body) != nil || response.Code != http.StatusOK || !body.OK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(body.NodeIDs) != 11 || body.NodeIDs[0] != string(domain.NodeRequirements) || body.NodeIDs[len(body.NodeIDs)-1] != string(domain.NodeCancelled) {
		t.Fatalf("node IDs=%v", body.NodeIDs)
	}
}

func TestTaskReadModelsExposeRepositoryGroupAndWorktree(t *testing.T) {
	group := strings.Repeat("a", 64)
	now := time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)
	summaries := projectSummaries([]application.ControlCenterTaskSummary{{
		TaskID: "task", RequestSummary: "Parallel worktree task", OriginHost: domain.HostCodex,
		ExecutionHost: domain.HostCodex, CurrentNode: domain.NodeRequirements, Lifecycle: "active",
		Revision: 1, UpdatedAt: now, RepositoryKeys: []domain.RepositoryKey{"primary"},
		RepositoryGroupID: domain.Digest(group), WorktreePath: "/worktrees/task-a",
	}})
	if len(summaries) != 1 || summaries[0].RepositoryGroupID != group || summaries[0].WorktreePath != "/worktrees/task-a" {
		t.Fatalf("summary projection=%+v", summaries)
	}

	branch := "main"
	head := strings.Repeat("b", 40)
	digest := domain.Digest(strings.Repeat("c", 64))
	detail, err := projectTaskDetail("request-read", application.ControlCenterTaskDetail{Task: domain.ProcessTask{
		TaskID: "task", OriginHost: domain.HostCodex,
		Intent:          domain.TaskIntent{Request: "Parallel worktree task", MethodProfile: domain.MethodPlain},
		Process:         domain.ProcessReference{ID: domain.ProcessStandardDevelopment, DefinitionDigest: digest},
		CurrentNode:     domain.NodeRequirements,
		WorkspaceOrigin: domain.WorkspaceOrigin{Mode: domain.WorkspaceModeDedicatedWorktree, SourceType: "remote", RemoteName: "origin", BaseBranch: "main", BaseCommit: head, TaskBranch: "feature/task", SourceRepositoryGroupDigest: domain.Digest(group), CanonicalWorktreeRoot: "/worktrees/task-a", WorktreeGitDirDigest: digest, ProvisioningReceiptID: "receipt"},
		Repository:      domain.RepositoryBinding{WorktreeInstanceDigest: digest, IdentityDigest: digest, HistoryDigest: digest, ContentDigest: digest, CurrentBranch: &branch, CurrentHead: head, HeadTree: head, HistoryRelation: domain.RepositoryHistoryExact, ObservedAt: now, BindingDigest: digest},
		Revision:        1, CreatedAt: now, UpdatedAt: now,
	}, Events: []store.TaskEvent{{Revision: 1, Kind: domain.OperationApplyAction, SourceNode: domain.NodeRequirements, DestinationNode: domain.NodeRequirements, RepositoryDeltaPaths: []string{"internal/file.go"}, CreatedAt: now}}})
	if err != nil || len(detail.Repositories) != 1 || detail.Repositories[0].RepositoryGroupID != group || detail.Repositories[0].Path != "/worktrees/task-a" {
		t.Fatalf("detail projection=%+v err=%v", detail.Repositories, err)
	}
	if detail.Workspace.ProvisioningStatus != "last_known" || len(detail.Events) != 1 || len(detail.Events[0].RepositoryDeltaPaths) != 1 || detail.Events[0].RepositoryDeltaPaths[0] != "internal/file.go" {
		t.Fatalf("workspace/event projection=%+v %+v", detail.Workspace, detail.Events)
	}
	abandoned := domain.ProcessTask{LastOperation: &domain.LastOperation{Kind: domain.OperationAbandonTask}}
	if status := projectWorkspace(abandoned).ProvisioningStatus; status != "unavailable" {
		t.Fatalf("abandoned provisioning status=%q", status)
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"current_changed_paths":[]`) || !strings.Contains(string(raw), `"provisioning_status":"last_known"`) || !strings.Contains(string(raw), `"repository_delta_paths":["internal/file.go"]`) {
		t.Fatalf("workspace projection must contain current paths and provisioning: %s", raw)
	}
}

func TestBaselineHistoryDetailExposesFirstPageAndTotal(t *testing.T) {
	task := domain.ProcessTask{TaskID: "history-task", Revision: 40, CurrentNode: domain.NodeRequirements}
	for revision := uint32(1); revision <= 40; revision++ {
		task.BaselineHistory = append(task.BaselineHistory, domain.BaselineReference{
			Kind: domain.BaselineRequirements, Revision: revision,
			Digest: domain.Digest(strings.Repeat("a", 64)), Summary: "Saved requirement",
			CreatedAt: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
		})
	}
	detail, err := projectTaskDetail("history-detail", application.ControlCenterTaskDetail{Task: task})
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range detail.Baselines {
		if fact.Kind != "baseline_history" {
			continue
		}
		var page domain.BaselineHistoryPage
		if err := json.Unmarshal([]byte(fact.Value), &page); err != nil {
			t.Fatal(err)
		}
		if page.TaskID != task.TaskID || page.Revision != 40 || page.Total != 40 || len(page.Entries) != 32 || page.NextAfter == nil || *page.NextAfter != 32 {
			t.Fatalf("incomplete history metadata: %+v", page)
		}
		for index, entry := range page.Entries {
			if entry.Sequence != uint64(index+1) || entry.Reference != task.BaselineHistory[index] {
				t.Fatal("first page changed a saved reference")
			}
		}
		if !strings.Contains(fact.Label, "first page") || !strings.Contains(fact.Label, "taskbelay_get_task baseline_history") || len(task.BaselineHistory) != 40 {
			t.Fatal("history must identify its partial display and complete-page entry point")
		}
		return
	}
	t.Fatal("history fact is missing")
}

func TestVerificationProjectionShowsPlanUsageAndAdjustmentReason(t *testing.T) {
	now := time.Date(2026, 9, 3, 4, 0, 0, 0, time.UTC)
	initial := domain.VerificationBudget{Level: domain.VerificationTargeted, MaxAutomaticCommands: 2}
	current := initial
	current.MaxAutomaticCommands = 3
	task := domain.ProcessTask{
		TaskPlan: &domain.TaskPlanBaseline{Revision: 2, VerificationPlan: domain.VerificationPlan{
			Checks:        []domain.VerificationPlanCheck{{Name: "targeted", Rationale: "The check covers the changed package."}},
			InitialBudget: initial, TestCodeChangesExpected: true,
		}},
		Evidence: []domain.EvidenceSummary{{TaskPlanRevision: 2, Source: domain.EvidenceSourceAutomated, CommandCount: 1, FullSuite: false}},
		VerificationBudgetAdjustments: []domain.VerificationBudgetAdjustment{{
			Revision: 1, TaskPlanRevision: 2, Basis: domain.VerificationAdjustmentNewImpact,
			Reason: "A newly found caller needs one focused check.", AdditionalChecks: []domain.VerificationPlanCheck{{Name: "caller", Rationale: "The caller shares the changed contract."}},
			AdditionalAutomaticCommands: 1, PreviousBudget: initial, CurrentBudget: current, CreatedAt: now,
		}},
	}
	view := projectVerification(task)
	if view.Plan == nil || view.CurrentBudget == nil || view.CurrentBudget.MaxAutomaticCommands != 3 || view.Usage.AutomaticCommands != 1 || len(view.Adjustments) != 1 || view.Adjustments[0].Reason == "" || !view.Plan.TestCodeChangesExpected {
		t.Fatalf("verification view=%#v", view)
	}
}

type stubControlCenterMutator struct {
	lastCall  string
	lastOpen  application.OpenTaskRequest
	stale     bool
	actionErr error
}

func (s *stubControlCenterMutator) OpenOrResumeTask(_ context.Context, request application.OpenTaskRequest) (application.ControlCenterMutationResult, error) {
	s.lastCall = "open"
	s.lastOpen = request
	return mutationTask(), nil
}

func (s *stubControlCenterMutator) CancelLifecycleTask(context.Context, application.CancelControlCenterTaskRequest) (application.ControlCenterMutationResult, error) {
	s.lastCall = "cancel"
	if s.stale {
		return application.ControlCenterMutationResult{}, domain.ErrRevisionConflict
	}
	return mutationTask(), nil
}

func (s *stubControlCenterMutator) PrepareTaskRelocation(context.Context, application.PrepareTaskRelocationRequest) (application.PrepareTaskRelocationResult, error) {
	s.lastCall = "relocation"
	return application.PrepareTaskRelocationResult{Task: domain.ProcessTask{TaskID: "task", Revision: 2}, RelocationID: "relocation"}, nil
}

func (s *stubControlCenterMutator) AbandonTask(context.Context, application.AbandonTaskRequest) (application.AbandonTaskResult, error) {
	s.lastCall = "abandon"
	return application.AbandonTaskResult{Task: domain.ProcessTask{TaskID: "task", Revision: 2}}, nil
}

func (s *stubControlCenterMutator) SetTaskArchive(context.Context, application.SetTaskArchiveRequest) (application.ControlCenterMutationResult, error) {
	s.lastCall = "archive"
	archived := true
	return application.ControlCenterMutationResult{Archived: &archived}, nil
}

func (s *stubControlCenterMutator) PurgeLifecycleTask(context.Context, application.PurgeControlCenterTaskRequest) (application.ControlCenterMutationResult, error) {
	s.lastCall = "purge"
	return application.ControlCenterMutationResult{Purged: true}, nil
}

func (s *stubControlCenterMutator) SubmitCurrentAction(context.Context, application.SubmitControlCenterActionRequest) (application.ControlCenterActionResult, error) {
	s.lastCall = "submit"
	if s.actionErr != nil {
		return application.ControlCenterActionResult{}, s.actionErr
	}
	return actionMutation(), nil
}

func (s *stubControlCenterMutator) AssessTaskOperation(context.Context, application.AssessControlCenterRecoveryRequest) (application.ControlCenterActionResult, error) {
	s.lastCall = "assess"
	assessment := recovery.RecoveryAssessment{NextAdvice: recovery.AdviceSubmitRecoveryApply}
	return application.ControlCenterActionResult{Task: domain.ProcessTask{TaskID: "task", Revision: 2}, Assessment: &assessment}, nil
}

func (s *stubControlCenterMutator) ApplyTaskRecovery(context.Context, application.ApplyControlCenterRecoveryRequest) (application.ControlCenterActionResult, error) {
	s.lastCall = "recover"
	return actionMutation(), nil
}

func actionMutation() application.ControlCenterActionResult {
	return application.ControlCenterActionResult{Task: domain.ProcessTask{TaskID: "task", Revision: 2}, Committed: true}
}

func mutationTask() application.ControlCenterMutationResult {
	task := domain.ProcessTask{TaskID: "task", Revision: 2}
	return application.ControlCenterMutationResult{Task: &task}
}

func quoteJSON(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
