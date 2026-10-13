package mcp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	a, _ := application.NewService(s, repository.NewGitObserver())
	v, e := NewServer(a, "0.20.0", nil)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func validate(t *testing.T, schema map[string]any, v any) {
	t.Helper()
	raw, _ := json.Marshal(schema)
	var definition jsonschema.Schema
	if e := json.Unmarshal(raw, &definition); e != nil {
		t.Fatal(e)
	}
	resolved, e := definition.Resolve(nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(v)
	var instance any
	json.Unmarshal(raw, &instance)
	if e = resolved.Validate(instance); e != nil {
		t.Fatalf("schema mismatch: %v\n%s", e, raw)
	}
}
func TestEveryToolEncodesActualErrorAgainstPublishedSchema(t *testing.T) {
	s := testServer(t)
	for _, tool := range Catalog {
		t.Run(tool.Name, func(t *testing.T) {
			actual := s.Dispatch(context.Background(), tool.Name, []byte(`{"unexpected":true}`))
			actual.RequestID = "example-request"
			if actual.OK {
				t.Fatal("unknown member accepted")
			}
			expected := Envelope{RequestID: "example-request", Tool: tool.Name, Error: &domain.Error{Code: "INVALID_ARGUMENT", Message: "member is not allowed by the current request", Details: []domain.ContractViolation{{Path: "arguments.unexpected", Rule: "requirement", Message: "member is not allowed by the current request"}}}, Recovery: &Recovery{Action: "correct_request", RetrySafe: false, Message: "Correct only the identified fields from existing facts; preserve operation identity and real authorization.", AllowedPaths: []string{"arguments.unexpected"}}}
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(expected)
			if string(a) != string(b) {
				t.Fatalf("encoded error differs\n%s\n%s", a, b)
			}
			validate(t, OutputSchema(tool.Output), actual)
		})
	}
}

func TestNestedRequestErrorsIdentifyTheExactMember(t *testing.T) {
	s := testServer(t)
	for _, c := range []struct{ raw, path, message string }{
		{`{"authorization":{"goals":[{"unexpected":true}]}}`, "arguments.authorization.goals[0].unexpected", "member is not allowed by the current request"},
		{`{"authorization":{"goals":[{"statement":2}]}}`, "arguments.authorization.goals[0].statement", "requires string"},
		{`{"checks":[{"argv":[true]}]}`, "arguments.checks[0].argv[0]", "requires string"},
		{`{"checks":[{"id":"a","id":"b"}]}`, "arguments.checks[0].id", "duplicate JSON member"},
	} {
		actual := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(c.raw))
		if actual.OK || actual.Error.Code != "INVALID_ARGUMENT" || len(actual.Error.Details) != 1 || actual.Error.Details[0].Path != c.path || actual.Error.Message != c.message {
			t.Fatalf("incorrect field error for %s: %+v", c.raw, actual)
		}
		validate(t, OutputSchema(application.View{}), actual)
		if tasks, e := s.service.Store.List(context.Background(), store.ListQuery{}); e != nil || len(tasks.Items) != 0 {
			t.Fatal("invalid input created semantic state", e)
		}
	}
}
func TestSDKTransportPublishesOnlyCurrentToolsAndRealEnvelopes(t *testing.T) {
	s := testServer(t)
	left, right := sdk.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, e := s.sdk.Connect(ctx, left, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer session.Close()
	client := sdk.NewClient(&sdk.Implementation{Name: "contract-test", Version: "1"}, nil)
	connection, e := client.Connect(ctx, right, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer connection.Close()
	tools, e := connection.ListTools(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(tools.Tools) != len(Catalog) {
		t.Fatal("catalog mismatch")
	}
	r, e := connection.CallTool(ctx, &sdk.CallToolParams{Name: "taskbelay_server_info", Arguments: map[string]any{}})
	if e != nil {
		t.Fatal(e)
	}
	if r.IsError {
		t.Fatal(r)
	}
	raw := r.Content[0].(*sdk.TextContent).Text
	var v Envelope
	if json.Unmarshal([]byte(raw), &v) != nil || !v.OK || v.Tool != "taskbelay_server_info" {
		t.Fatal("invalid actual SDK response")
	}
	validate(t, OutputSchema(Catalog[0].Output), v)
}
func TestRequestDecodeRejectsDuplicatesTypesAndTrailingValues(t *testing.T) {
	for _, raw := range []string{`{"task_id":"x","task_id":"y"}`, `{"task_id":7}`, `{} {}`, `[]`, "{\"task_id\":\"\xff\"}"} {
		var q GetRequest
		if e := Decode([]byte(raw), &q); e == nil {
			t.Fatal("invalid request accepted", raw)
		}
	}
	var q GetRequest
	if e := Decode([]byte(`{"task_id":"x"}`), &q); e != nil {
		t.Fatal(e)
	}
}
func TestCompleteSkillErrorExampleMatchesActualProducer(t *testing.T) {
	s := testServer(t)
	actual := s.Dispatch(context.Background(), "taskbelay_server_info", []byte(`{"unexpected":true}`))
	actual.RequestID = "example-request"
	b, _ := json.Marshal(actual)
	p := filepath.Join("..", "..", "skills", "taskbelay", "core", "protocol.md")
	doc, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(doc), string(b)) {
		t.Fatal("shared complete error example differs from the producer")
	}
}

func TestPlanBoundaryResponsesAndExecutionValidateAgainstPublishedSchemas(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Contract fixture"}, {"config", "user.email", "fixture@localhost"}} {
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatal(e, string(b))
		}
	}
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("fixture"), 0600)
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "fixture"}} {
		c := exec.Command("git", append([]string{"-C", root}, args...)...)
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatal(e, string(b))
		}
	}
	dispatch := func(name string, q any) Envelope {
		t.Helper()
		b, _ := json.Marshal(q)
		v := s.Dispatch(ctx, name, b)
		for _, tool := range Catalog {
			if tool.Name == name {
				validate(t, OutputSchema(tool.Output), v)
			}
		}
		return v
	}
	q := application.OpenRequest{OperationID: "open-contract", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: domain.Authorization{UserRef: "fixture-request", Request: "Inspect the committed fixture", Goals: []domain.Goal{{ID: "goal", Statement: "The fixture status command succeeds", Checks: []string{"check"}}}},
		Plan:   domain.Plan{Understanding: "Inspect the fixture", RootCause: "Status needs verification", Approach: "Use read-only Git status", SimplerAlternative: "No file editing or framework needed", VerificationBasis: "One exact status command suffices", Completion: "Status succeeds and no code changes", Work: []domain.PlannedWork{{ID: "inspect", GoalIDs: []string{"goal"}, Change: "Inspect current status", Owner: "Git observer", Necessity: "Requested fixture check"}}},
		Checks: []domain.Check{{ID: "check", Kind: "command", Argv: []string{"git", "status", "--porcelain"}, Rationale: "Inspect actual Git state", Conditions: "Local fixture", Retry: "repeatable", TimeoutMS: 3000}}}
	opened := dispatch("taskbelay_open_task", q)
	if !opened.OK {
		t.Fatal(opened)
	}
	v := opened.Result.(application.View)
	run := application.RunRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "unconfirmed-run"}, CheckID: "check", Rationale: "Inspect fixture"}
	rejected := dispatch("taskbelay_run_check", run)
	expected := `{"ok":false,"request_id":"unconfirmed-run","tool":"taskbelay_run_check","error":{"code":"PLAN_CONFIRMATION_REQUIRED","message":"show the understanding and complete plan and record actual user confirmation first","details":[{"path":"plan_digest","rule":"requirement","message":"show the understanding and complete plan and record actual user confirmation first"}]},"recovery":{"action":"request_authorization","retry_safe":false,"message":"Show the current plan or required decision and obtain actual user authorization; preserve acceptance requirements."}}`
	b, _ := json.Marshal(rejected)
	if string(b) != expected {
		t.Fatalf("unexpected complete error\n%s", b)
	}
	confirmed := dispatch("taskbelay_confirm_plan", application.ConfirmPlanRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "confirm-contract"}, PlanDigest: v.PlanDigest, UserRef: "fixture-answer", Response: "I understand and approve the displayed plan"})
	if !confirmed.OK {
		t.Fatal(confirmed)
	}
	v = confirmed.Result.(application.View)
	run.Identity = application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "approved-run"}
	executed := dispatch("taskbelay_run_check", run)
	if !executed.OK || executed.Result.(application.RunResult).Run.Status != "passed" {
		t.Fatal(executed)
	}
	before := executed.Result.(application.RunResult).View.Task.Revision
	invalid := dispatch("taskbelay_checkpoint", map[string]any{"task_id": v.Task.TaskID, "revision": before, "operation_id": "weaken-check", "checks": []any{}})
	if invalid.OK || invalid.Error.Details[0].Path != "arguments.checks" {
		t.Fatal("checkpoint accepted changed checks", invalid)
	}
	saved, e := s.service.Store.Get(ctx, v.Task.TaskID)
	if e != nil || saved.Revision != before || saved.Usage.Runs != 1 {
		t.Fatal("rejection changed semantic state", e)
	}
}

func TestStartupDiagnosesReachableCoreWithoutClaimingHostMCPVisibility(t *testing.T) {
	s := testServer(t)
	actual := s.Dispatch(context.Background(), "taskbelay_server_info", []byte(`{}`))
	if !actual.OK {
		t.Fatal(actual)
	}
	raw, _ := json.Marshal(actual.Result)
	if !strings.Contains(string(raw), `"core_reached":true`) || !strings.Contains(string(raw), "CLI success does not establish MCP availability") {
		t.Fatal(string(raw))
	}
	validate(t, OutputSchema(map[string]any{}), actual)
}

func TestCompleteBudgetDecisionRoundTripsActualTransportSchema(t *testing.T) {
	s := testServer(t)
	ctx := context.Background()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "test"}, {"config", "user.email", "test@localhost"}, {"commit", "--allow-empty", "-m", "baseline"}} {
		if out, e := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
			t.Fatal(e, string(out))
		}
	}
	call := func(name string, q any) application.View {
		t.Helper()
		raw, _ := json.Marshal(q)
		actual := s.Dispatch(ctx, name, raw)
		if !actual.OK {
			t.Fatal(actual)
		}
		validate(t, OutputSchema(application.View{}), actual)
		b, _ := json.Marshal(actual.Result)
		var v application.View
		if e := json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	authorization := domain.Authorization{UserRef: "actual-fixture-request", Request: "Verify local clean diff", Goals: []domain.Goal{{ID: "goal", Statement: "No unintended changes", Checks: []string{"check"}}}, Limits: domain.DefaultLimits()}
	plan := domain.Plan{Understanding: "Verify requested clean diff", RootCause: "Need actual result", Approach: "Use existing local Git check", SimplerAlternative: "No additional tool needed", VerificationBasis: "Actual diff exit", Completion: "Current required check passed", Work: []domain.PlannedWork{{ID: "work", GoalIDs: []string{"goal"}, Change: "Verify requested state", Owner: "Existing repository", Necessity: "Required actual result"}}}
	v := call("taskbelay_open_task", application.OpenRequest{OperationID: "budget-transport-open", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: authorization, Plan: plan, Checks: []domain.Check{{ID: "check", Kind: "command", Argv: []string{"git", "diff", "--quiet"}, TimeoutMS: 3000, Retry: "repeatable", Rationale: "Actual diff", Conditions: "Local fixture"}}})
	v = call("taskbelay_confirm_plan", application.ConfirmPlanRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "budget-transport-confirm"}, PlanDigest: v.PlanDigest, UserRef: "actual-confirmation", Response: "Approved exact fixture plan"})
	q := application.BudgetRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "budget-transport-update"}, PlanDigest: v.PlanDigest, Budget: domain.BudgetDecision{Runs: 40, ExecutionMS: 14400000, WorkMS: 14400000, Tokens: 1000000}, UserRef: "actual-complete-budget-decision", Response: "Approve four hours for command and work time and 40 runs"}
	updated := call("taskbelay_update_budget", q)
	if updated.Task.Authorization.Limits.CoreMS != 14400000 || updated.Task.Authorization.Limits.HostMS != 14400000 || !updated.Task.PlanApproved() {
		t.Fatal(updated)
	}
	replay := call("taskbelay_update_budget", q)
	if !replay.Replayed || replay.Task.Revision != updated.Task.Revision {
		t.Fatal("replayed budget changed state")
	}
}
