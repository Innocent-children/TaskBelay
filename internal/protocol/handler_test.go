package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	"github.com/google/jsonschema-go/jsonschema"
)

func testServer(t *testing.T) *Handler {
	t.Helper()
	s, e := store.Open(context.Background(), filepath.Join(t.TempDir(), "taskbelay.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	a, _ := application.NewService(s, repository.NewGitObserver())
	v := NewHandler(a, "0.25.0", userconfig.Preferences{})
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
		{`{"plan":{"checks":[{"argv":[true]}]}}`, "arguments.plan.checks[0].argv[0]", "requires string"},
		{`{"plan":{"checks":[{"id":"a","id":"b"}]}}`, "arguments.plan.checks[0].id", "duplicate JSON member"},
	} {
		actual := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(c.raw))
		if actual.OK || actual.Error.Code != "INVALID_ARGUMENT" || len(actual.Error.Details) != 1 || actual.Error.Details[0].Path != c.path || actual.Error.Message != c.message {
			t.Fatalf("incorrect field error for %s: %+v", c.raw, actual)
		}
		validate(t, OutputSchema(TaskResult{}), actual)
		if tasks, e := s.service.Store.List(context.Background(), store.ListQuery{}); e != nil || len(tasks.Items) != 0 {
			t.Fatal("invalid input created semantic state", e)
		}
	}
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
		if name == "check.run" {
			v = s.RunCheck(ctx, b)
			validate(t, OutputSchema(NativeCheck.Output), v)
		}
		for _, tool := range Catalog {
			if tool.Name == name {
				validate(t, OutputSchema(tool.Output), v)
			}
		}
		return v
	}
	q := application.OpenRequest{OperationID: "open-contract", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: domain.Authorization{UserRef: "fixture-request", Request: "Inspect the committed fixture", Goals: []domain.Goal{{ID: "goal", Statement: "The fixture status command succeeds"}}},
		Plan: domain.Plan{Approach: "Use read-only Git status", VerificationBasis: "One exact status command suffices", Checks: []domain.Check{{GoalIDs: []string{"goal"}, ID: "check", Kind: "command", Argv: []string{"git", "status", "--porcelain"}, Rationale: "Inspect actual Git state", Conditions: "Local fixture", Retry: "repeatable", TimeoutMS: 3000}}}}
	opened := dispatch("taskbelay_open_task", q)
	if !opened.OK {
		t.Fatal(opened)
	}
	v := opened.Result.(TaskResult)
	if v.Details != nil || v.Decision.Advice != "continue" {
		t.Fatal("ordinary start returned history or requested unnecessary correction", v)
	}
	detailed := dispatch("taskbelay_get_task", GetRequest{TaskID: v.Task.TaskID, Detail: true})
	if !detailed.OK {
		t.Fatal(detailed)
	}
	full := detailed.Result.(TaskResult).Details
	if full == nil || full.Task.TaskID != v.Task.TaskID || len(full.Task.Repositories[0].Binding.TaskSurface) != 0 {
		t.Fatal("detail lost saved task", full)
	}
	compactBytes, _ := json.Marshal(v)
	fullBytes, _ := json.Marshal(full)
	if len(compactBytes) >= len(fullBytes) || strings.Contains(string(compactBytes), "worktree_instance_digest") {
		t.Fatal("default result still includes full repository detail")
	}
	t.Logf("initial Task response: summary %d bytes; full view %d bytes", len(compactBytes), len(fullBytes))
	run := application.RunRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "authorized-run"}, CheckID: "check", Rationale: "Inspect fixture under the existing user request"}
	executed := dispatch("check.run", run)
	if !executed.OK || executed.Result.(RunResult).Run.Status != "passed" {
		t.Fatal(executed)
	}
	before := executed.Result.(RunResult).View.Task.Revision
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
	call := func(name string, q any) TaskResult {
		t.Helper()
		raw, _ := json.Marshal(q)
		actual := s.Dispatch(ctx, name, raw)
		if !actual.OK {
			t.Fatal(actual)
		}
		validate(t, OutputSchema(TaskResult{}), actual)
		b, _ := json.Marshal(actual.Result)
		var v TaskResult
		if e := json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	authorization := domain.Authorization{UserRef: "actual-fixture-request", Request: "Verify local clean diff", Goals: []domain.Goal{{ID: "goal", Statement: "No unintended changes"}}, Limits: domain.Limits{Runs: 20, ExecutionMS: 600000, CoreMS: 600000, HostMS: 3600000, Tokens: 1000000}}
	plan := domain.Plan{Approach: "Use existing local Git check", VerificationBasis: "Actual diff exit"}
	v := call("taskbelay_open_task", application.OpenRequest{OperationID: "budget-transport-open", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: authorization, Plan: withChecks(plan, []domain.Check{{GoalIDs: []string{"goal"}, ID: "check", Kind: "command", Argv: []string{"git", "diff", "--quiet"}, TimeoutMS: 3000, Retry: "repeatable", Rationale: "Actual diff", Conditions: "Local fixture"}})})
	q := application.BudgetRequest{Identity: application.Identity{TaskID: v.Task.TaskID, Revision: v.Task.Revision, OperationID: "budget-transport-update"}, AuthorizationDigest: v.AuthorizationDigest, Budget: domain.BudgetDecision{Runs: 40, ExecutionMS: 14400000, WorkMS: 14400000, Tokens: 1000000}, UserRef: "actual-complete-budget-decision", Response: "Approve four hours for command and work time and 40 runs"}
	updated := call("taskbelay_update_budget", q)
	if updated.Task.Authorization.Limits.CoreMS != 14400000 || updated.Task.Authorization.Limits.HostMS != 14400000 || updated.Task.BudgetDecision.UserRef != q.UserRef {
		t.Fatal(updated)
	}
	replay := call("taskbelay_update_budget", q)
	if !replay.Replayed || replay.Task.Revision != updated.Task.Revision {
		t.Fatal("replayed budget changed state")
	}
}

func TestMeterInitializationClassifiesActualErrorCategoryAndDoesNotDispatch(t *testing.T) {
	cases := []struct {
		name, phase  string
		cause        error
		code, action string
	}{
		{"ps permission", "process_identity", &os.PathError{Op: "fork/exec", Path: "/bin/ps", Err: syscall.EACCES}, "PROCESS_IDENTITY_DENIED", "inspect_process_access"},
		{"ps unavailable", "process_identity", &os.PathError{Op: "fork/exec", Path: "/bin/ps", Err: syscall.ENOENT}, "PROCESS_IDENTITY_UNAVAILABLE", "inspect_process_access"},
		{"storage denied", "storage_write", &os.PathError{Op: "open", Path: "taskbelay.db", Err: os.ErrPermission}, "STORAGE_UNAVAILABLE", "repair_storage"},
		{"storage corrupt", "storage_read", domain.Fail("STORAGE_CORRUPT", "data", "invalid retained timestamp"), "STORAGE_CORRUPT", "repair_storage"},
		{"entropy failure", "operation_identity", errors.New("secure random source unavailable"), "CONTROL_INITIALIZATION_FAILED", "inspect_initialization"},
		{"cancelled", "storage_read", context.Canceled, "CONTROL_INITIALIZATION_CANCELLED", "inspect_host_context"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			s.measureControl = func(context.Context) (func(), error) {
				return nil, &application.ControlMeterError{Stage: c.phase, Err: c.cause}
			}
			result := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(`{}`))
			if result.OK || result.Result != nil || result.Error.Code != c.code || result.Recovery.Action != c.action || result.Recovery.RetrySafe {
				t.Fatal(result)
			}
			if len(result.Error.Details) != 1 || result.Error.Details[0].Path != "control_meter."+c.phase || !strings.Contains(result.Error.Details[0].Message, c.cause.Error()) {
				t.Fatal("phase or original cause lost", result)
			}
			result.RequestID = "example-request"
			validate(t, OutputSchema(TaskResult{}), result)
			page, e := s.service.Store.List(context.Background(), store.ListQuery{})
			if e != nil || len(page.Items) != 0 {
				t.Fatal("tool executed after failed metering", e, page)
			}
		})
	}
}

func TestClosedStorageMeteringReportsActualReadFailure(t *testing.T) {
	s := testServer(t)
	if e := s.service.Store.Close(); e != nil {
		t.Fatal(e)
	}
	result := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(`{}`))
	if result.OK || result.Error.Code != "STORAGE_UNAVAILABLE" || result.Recovery.Action != "repair_storage" || !strings.Contains(result.Error.Details[0].Path, "storage_read") {
		t.Fatal(result)
	}
}

func withChecks(p domain.Plan, checks []domain.Check) domain.Plan { p.Checks = checks; return p }

func TestSavedReadsDoNotRequireProcessAccessOrMeterWrites(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("saved read fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@localhost"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(err, string(output))
		}
	}
	opened, err := s.service.Open(context.Background(), application.OpenRequest{OperationID: "saved-read-fixture", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: domain.Authorization{UserRef: "fixture-user", Request: "Read saved state under denied process access", Goals: []domain.Goal{{ID: "goal", Statement: "Inspect the saved request"}}}, Plan: domain.Plan{Approach: "Read saved state", VerificationBasis: "Actual fixture storage", Checks: []domain.Check{{ID: "inspect", Kind: "human", GoalIDs: []string{"goal"}, Rationale: "Inspect saved task", Conditions: "Isolated fixture"}}}})
	if err != nil {
		t.Fatal(err)
	}
	readRequest, _ := json.Marshal(GetRequest{TaskID: opened.Task.TaskID, Refresh: func() *bool { v := false; return &v }(), Detail: true})

	calls := 0
	s.measureControl = func(context.Context) (func(), error) {
		calls++
		return nil, &application.ControlMeterError{Stage: "process_identity", Err: os.ErrPermission}
	}
	for _, q := range []struct {
		name, raw string
		code      string
	}{
		{"taskbelay_server_info", "{}", ""},
		{"taskbelay_list_tasks", "{}", ""},
		{"taskbelay_get_task", `{"task_id":"missing-task","refresh":false}`, "TASK_NOT_FOUND"},
		{"taskbelay_get_run", `{"run_id":"missing-run"}`, "RUN_NOT_FOUND"},
	} {
		result := s.Dispatch(context.Background(), q.name, []byte(q.raw))
		if q.code == "" && !result.OK || q.code != "" && (result.Error == nil || result.Error.Code != q.code) {
			t.Fatal(result)
		}
	}
	actual := s.Dispatch(context.Background(), "taskbelay_get_task", readRequest)
	if !actual.OK || actual.Result.(TaskResult).Details.Task.TaskID != opened.Task.TaskID {
		t.Fatal("actual saved task was not readable", actual)
	}

	if calls != 0 {
		t.Fatal("saved-state read entered mutation metering")
	}
	usage, err := s.service.SavedControlCost(context.Background())
	if err != nil || usage.Calls != 0 || usage.Pending != 0 {
		t.Fatal("queries changed costs", usage, err)
	}
	denied := s.Dispatch(context.Background(), "taskbelay_open_task", []byte(`{}`))
	if denied.OK || calls != 1 {
		t.Fatal("mutation bypassed required metering")
	}
}

func TestNativeExecutionIsAbsentFromRemoteCatalog(t *testing.T) {
	s := testServer(t)
	for _, name := range []string{"taskbelay_run_check", "check.run"} {
		result := s.Dispatch(context.Background(), name, []byte(`{}`))
		if result.OK || result.Error.Code != "INVALID_ARGUMENT" {
			t.Fatal("remote execution allowed", result)
		}
	}
	result := s.RunCheck(context.Background(), []byte(`{"unexpected":true}`))
	result.RequestID = "example-request"
	validate(t, OutputSchema(NativeCheck.Output), result)
	if result.OK || result.Error.Details[0].Path != "arguments.unexpected" {
		t.Fatal(result)
	}
}
