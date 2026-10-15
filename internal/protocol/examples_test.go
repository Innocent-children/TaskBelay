package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
)

func TestPublishedMethodExamplesUseActualProducers(t *testing.T) {
	ctx := context.Background()
	h := testServer(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Contract fixture"}, {"config", "user.email", "fixture@localhost"}, {"commit", "--allow-empty", "-m", "fixture"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	examples := map[string][]ContractExample{}
	normal := &exampleNormalizer{root: root, ids: map[string]string{}}
	var current TaskIdentity
	var state TaskState
	sequence := 0
	call := func(name string, input any, publish bool) Envelope {
		t.Helper()
		raw, _ := json.Marshal(input)
		var output Envelope
		if name == "check.run" {
			output = h.RunCheck(ctx, raw)
		} else if strings.HasPrefix(name, "host-check.") {
			output = h.RunPreflight(ctx, name, raw)
		} else {
			output = h.Dispatch(ctx, name, raw)
		}
		if !output.OK {
			t.Fatalf("%s failed: %+v", name, output)
		}
		tool := NativeCheck
		for _, candidate := range append(NativePreflights, Catalog...) {
			if candidate.Name == name {
				tool = candidate
			}
		}
		validate(t, Schema(tool.Input), input)
		validate(t, OutputSchema(tool.Output), output)
		switch value := output.Result.(type) {
		case TaskResult:
			state, current = value.TaskState, value.Identity
		case RunResult:
			state, current = value.View, value.View.Identity
		}
		if current.TaskID != "" && normal.ids[current.TaskID] == "" {
			normal.ids[current.TaskID] = fmt.Sprintf("task-example-%d", len(normal.ids)+1)
		}
		for _, c := range state.Checks {
			if c.EvidenceID != nil {
				normal.ids[*c.EvidenceID] = "evidence-" + c.ID
			}
			if c.RunID != nil {
				normal.ids[*c.RunID] = "run-" + c.ID
			}
		}
		if publish {
			examples[name] = append(examples[name], ContractExample{
				Name: "success", Preconditions: "Isolated Git fixture; replace /workspace/demo and returned identities with actual values. Semantic/user inputs are simulated fixture testimony, not a claim of real Host verification. Timestamps, process IDs, hashes and timings are normalized.",
				Input: normal.encode(t, input), Output: normal.encode(t, output),
			})
		}
		return output
	}
	id := func() application.Identity {
		sequence++
		return application.Identity{TaskID: current.TaskID, Revision: current.Revision, OperationID: fmt.Sprintf("example-operation-%d", sequence)}
	}
	call("taskbelay_server_info", ServerInfoRequest{}, true)
	check := domain.Check{ID: "accept", Kind: "command", Argv: []string{"git", "diff", "--quiet"}, GoalIDs: []string{"goal"}, Conditions: "Isolated committed fixture", Rationale: "Verify the tracked tree has no unintended changes", Retry: "repeatable", TimeoutMS: 3000}
	human := domain.Check{ID: "human", Kind: "human", GoalIDs: []string{"goal"}, Conditions: "Fixture user inspected the requested result", Rationale: "Retain the actual completed human observation"}
	regression := check
	regression.ID, regression.Purpose, regression.Argv = "regression", "regression", []string{"git", "rev-parse", "--verify", "missing-fixture-ref"}
	comparison := check
	comparison.ID, comparison.Purpose, comparison.Compares, comparison.Argv = "comparison", "diagnostic", "regression", []string{"git", "status", "--porcelain"}
	open := application.OpenRequest{OperationID: "example-open", Host: "cli", Repositories: []application.RepositoryInput{{Key: "primary", Path: root}}, Authorization: domain.Authorization{UserRef: "fixture-user-message-1", Request: "Verify the tracked result without committing or publishing", Goals: []domain.Goal{{ID: "goal", Statement: "The tracked result and its user observation are verified"}}}, Plan: domain.Plan{Approach: "Use existing Git and human checks", VerificationBasis: "Actual local Git checks plus a separately labelled fixture user observation", Checks: []domain.Check{check, human, regression, comparison}}}
	call("taskbelay_open_task", open, true)
	call("host-check.workspace-available", application.WorkspaceRequest{Repositories: []application.WorkspaceSelection{{Key: "primary", RepositoryPath: root}}}, true)
	call("host-check.pre-file-write", application.WriteRequest{Host: "zcode", RepositoryPath: root, ToolName: "Edit", Paths: []string{filepath.Join(root, "file.txt")}, IntentDigest: domain.Hash("fixture intent"), PathParseComplete: true}, true)
	call("taskbelay_list_tasks", map[string]any{"page": 1, "page_size": 20, "status": "active"}, true)
	call("taskbelay_get_task", GetRequest{TaskID: current.TaskID}, true)
	plan := open.Plan
	plan.Approach = "Keep responsibilities unchanged and execute only the declared checks"
	call("taskbelay_revise_task", application.ReviseRequest{Identity: id(), Plan: &plan, Reason: "Refine implementation without changing the requested outcome"}, true)
	concern := domain.ConcernInput{ID: "tracked-result", Summary: "Confirm the tracked result using an actual check", GoalIDs: []string{"goal"}, CheckIDs: []string{"accept"}}
	call("taskbelay_checkpoint", application.CheckpointRequest{Identity: id(), Assessment: &application.AssessmentInput{AuthorizationDigest: state.Digests.Authorization, ContentDigest: state.Digests.Content, Summary: "Retain the question before verification", Concerns: []domain.ConcernInput{concern}}, Issue: &domain.Issue{Kind: "tradeoff", Summary: "Which actual requested result should this fixture retain?", Options: []string{"Keep tracked result", "Change the requested output"}}}, false)
	call("taskbelay_resolve_issue", application.ResolveIssueRequest{Identity: id(), IssueDigest: state.IssueDigest, UserRef: "fixture-user-answer-2", Decision: "Keep the requested tracked result"}, true)
	call("taskbelay_update_budget", application.BudgetRequest{Identity: id(), AuthorizationDigest: state.Digests.Authorization, Budget: domain.BudgetDecision{Runs: 15}, UserRef: "fixture-explicit-budget-3", Response: "Allow at most 15 runs; no time or token caps"}, true)
	run := call("check.run", application.RunRequest{Identity: id(), CheckID: "accept", Rationale: "Verify current tracked content"}, true).Result.(RunResult)
	acceptID := run.Run.RunID
	call("taskbelay_get_run", GetRunRequest{RunID: acceptID}, true)
	call("taskbelay_recover_run", application.RecoverRequest{Identity: id(), RunID: acceptID}, true)
	failed := call("check.run", application.RunRequest{Identity: id(), CheckID: "regression", Rationale: "Observe the explicitly named existing failure"}, false).Result.(RunResult)
	compared := call("check.run", application.RunRequest{Identity: id(), CheckID: "comparison", Rationale: "Compare the existing regression against the tracked fixture"}, false).Result.(RunResult)
	if failed.Run.Status != "failed" || compared.Run.Status != "passed" {
		t.Fatal("fixture did not reproduce actual outcomes")
	}
	call("taskbelay_record_evidence", application.EvidenceRequest{Identity: id(), CheckID: "human", Source: "human", Status: "passed", Summary: "Fixture user inspected the tracked result", UserRef: "fixture-human-observation-4", ContentDigest: state.Digests.Content}, true)
	call("taskbelay_accept_known_failure", application.AcceptFailureRequest{Identity: id(), CheckID: "regression", FailedEvidenceID: failed.Run.RunID, ComparisonEvidenceID: compared.Run.RunID, Failures: []string{"missing-fixture-ref"}, UserRef: "fixture-user-acceptance-5", Reason: "Actual comparison confirms this separately accepted existing failure"}, true)
	concern.Resolution = &domain.ConcernResolution{Kind: "verified", Basis: "Actual current Git check covers the tracked-result question", EvidenceIDs: []string{acceptID}}
	call("taskbelay_checkpoint", application.CheckpointRequest{Identity: id(), Progress: []application.ProgressInput{{GoalID: "goal", Status: "done", Summary: "Actual checks and fixture observation are recorded"}}, Assessment: &application.AssessmentInput{AuthorizationDigest: state.Digests.Authorization, ContentDigest: state.Digests.Content, Summary: "Review current results and responsibilities", Review: &application.ReviewInput{Summary: "The existing Git/text responsibilities fit the request; actual required results are recorded"}, Concerns: []domain.ConcernInput{concern}}}, true)
	evidence := []string{acceptID}
	for _, c := range state.Checks {
		if c.ID == "human" {
			evidence = append(evidence, *c.EvidenceID)
		}
	}
	call("taskbelay_finish_task", application.FinishRequest{Identity: id(), Summary: "Tracked result verified; existing regression remains explicitly accepted", Acceptance: []application.Acceptance{{GoalID: "goal", EvidenceIDs: evidence, Basis: "Current actual command and labelled fixture human observation"}}}, true)
	call("taskbelay_archive_task", application.ArchiveRequest{Identity: id(), Archived: true}, true)
	open.OperationID = "example-cancel-open"
	call("taskbelay_open_task", open, false)
	call("taskbelay_cancel_task", application.CancelRequest{Identity: id(), Reason: "Fixture user cancels this separate request"}, true)
	for _, tool := range append(append([]Tool{NativeCheck}, NativePreflights...), Catalog...) {
		bad := json.RawMessage(`{"unexpected":true}`)
		output := h.Dispatch(ctx, tool.Name, bad)
		if tool.Name == "check.run" {
			output = h.RunCheck(ctx, bad)
		} else if strings.HasPrefix(tool.Name, "host-check.") {
			output = h.RunPreflight(ctx, tool.Name, bad)
		}
		output.RequestID = "example-request"
		validate(t, OutputSchema(tool.Output), output)
		examples[tool.Name] = append(examples[tool.Name], ContractExample{Name: "unknown_member", Preconditions: "Send an unknown member to this exact method. The request is rejected before any task or process operation.", Input: bad, Output: mustJSON(t, output)})
	}
	if os.Getenv("TASKBELAY_UPDATE_EXAMPLES") == "1" {
		raw, _ := json.MarshalIndent(examples, "", "  ")
		if err := os.WriteFile("examples.json", append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, tool := range append(append([]Tool{NativeCheck}, NativePreflights...), Catalog...) {
		saved := methodExamples(tool.Name)
		if len(saved) != 2 {
			t.Fatalf("%s lacks complete adjacent success/error examples", tool.Name)
		}
		for _, example := range saved {
			var value any
			if json.Unmarshal(example.Output, &value) != nil {
				t.Fatal("invalid example JSON", tool.Name)
			}
			validate(t, OutputSchema(tool.Output), value)
		}
		var actual, expected any
		json.Unmarshal(examples[tool.Name][1].Output, &actual)
		json.Unmarshal(saved[1].Output, &expected)
		if string(mustJSON(t, actual)) != string(mustJSON(t, expected)) {
			t.Fatalf("%s complete encoded error changed", tool.Name)
		}
	}
}
func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type exampleNormalizer struct {
	root string
	ids  map[string]string
}

var exampleHash = regexp.MustCompile(`^[a-f0-9]{40,64}$`)

func (n *exampleNormalizer) encode(t *testing.T, value any) json.RawMessage {
	var data any
	if err := json.Unmarshal(mustJSON(t, value), &data); err != nil {
		t.Fatal(err)
	}
	var walk func(any, string) any
	walk = func(v any, key string) any {
		switch x := v.(type) {
		case map[string]any:
			keys := []string{}
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				x[k] = walk(x[k], k)
			}
			return x
		case []any:
			for i, item := range x {
				x[i] = walk(item, key)
			}
			return x
		case string:
			if fixed, ok := n.ids[x]; ok {
				return fixed
			}
			if strings.HasSuffix(key, "_at") || key == "observed_through" {
				return "2026-10-14T09:00:00Z"
			}
			if exampleHash.MatchString(x) {
				if n.ids[x] == "" {
					n.ids[x] = fmt.Sprintf("%0*x", len(x), len(n.ids)+1)
				}
				return n.ids[x]
			}
			if key == "request_id" && strings.HasPrefix(x, "request-") {
				return "example-request"
			}
			if key == "identity" {
				return "fixture-process-start"
			}
			return strings.ReplaceAll(x, n.root, "/workspace/demo")
		case float64:
			if strings.HasSuffix(key, "_ms") && key != "timeout_ms" && key != "worst_case_ms" && key != "retry_reserve_ms" {
				return float64(0)
			}
			if key == "pid" {
				return float64(1234)
			}
		}
		return v
	}
	return mustJSON(t, walk(data, ""))
}
