package protocol

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
)

type Recovery struct {
	Action       string   `json:"action"`
	RetrySafe    bool     `json:"retry_safe"`
	Message      string   `json:"message"`
	AllowedPaths []string `json:"allowed_paths,omitempty"`
}
type Envelope struct {
	OK        bool          `json:"ok"`
	RequestID string        `json:"request_id"`
	Tool      string        `json:"tool"`
	Result    any           `json:"result,omitempty"`
	Error     *domain.Error `json:"error,omitempty"`
	Recovery  *Recovery     `json:"recovery,omitempty"`
}
type GetRequest struct {
	TaskID  string `json:"task_id"`
	Refresh *bool  `json:"refresh,omitempty"`
	Detail  bool   `json:"detail,omitempty" description:"Default false. True adds result.details with full saved Task, repository fingerprints and history."`
}
type GetRunRequest struct {
	RunID string `json:"run_id"`
}
type Empty struct{}
type Tool struct {
	Name        string
	Description string
	Input       any
	Output      any
	ReadOnly    bool
}

var Catalog = []Tool{
	{"taskbelay_server_info", "Read actual tools, evidence boundary and Host coverage.", Empty{}, map[string]any{}, true},
	{"taskbelay_open_task", "Start from the actual user request in the selected workspace. Existing authorization permits work without a separate plan approval.", application.OpenRequest{}, TaskResult{}, false},
	{"taskbelay_get_task", "Refresh actual code changes and evidence applicability; recover current requirements and progress. Set detail:true only for full repository fingerprints and history.", GetRequest{}, TaskResult{}, false},
	{"taskbelay_checkpoint", "Record current reasoning, necessary changes, progress and cost. Use revise_task for changes to the approach or check definitions.", application.CheckpointRequest{}, TaskResult{}, false},
	{"taskbelay_resolve_issue", "Record the actual user answer to the displayed issue without changing authorization.", application.ResolveIssueRequest{}, TaskResult{}, false},
	{"taskbelay_revise_task", "Replace only supplied authorization or plan, including its check definitions. Routine method changes reuse authority; changed user boundaries require a new actual user reference. Preserve unaffected results and cost.", application.ReviseRequest{}, TaskResult{}, false},
	{"taskbelay_record_evidence", "Record an agent claim or a completed human observation; cannot manufacture Core execution.", application.EvidenceRequest{}, TaskResult{}, false},
	{"taskbelay_get_run", "Read a saved actual execution after response loss; never executes a command.", GetRunRequest{}, execution.Run{}, true},
	{"taskbelay_recover_run", "Recover a signed result receipt or classify a stopped owner with no receipt; never infer passing or replay execution.", application.RecoverRequest{}, RunResult{}, false},
	{"taskbelay_finish_task", "Conclude only with current scope reasoning and applicable passing evidence for every authorized goal.", application.FinishRequest{}, TaskResult{}, false},
	{"taskbelay_cancel_task", "End the task and release claims while retaining files and results; unresolved execution prevents cancellation.", application.CancelRequest{}, TaskResult{}, false},
	{"taskbelay_list_tasks", "Read a bounded page of saved task summaries, including archived tasks when requested.", store.ListQuery{}, map[string]any{}, true},
	{"taskbelay_accept_known_failure", "Record a user's explicit acceptance of named existing regression failures with current actual comparison evidence; never waive acceptance checks.", application.AcceptFailureRequest{}, TaskResult{}, false},
	{"taskbelay_update_budget", "Apply one explicitly approved complete budget decision; synchronize Core and Host work-time caps without resetting cumulative cost.", application.BudgetRequest{}, TaskResult{}, false},
	{"taskbelay_archive_task", "Archive or restore a terminal task without changing its outcome or deleting work.", application.ArchiveRequest{}, TaskResult{}, false},
}

type Handler struct {
	measureControl func(context.Context) (func(), error)
	service        *application.Service
	version        string
	preferences    userconfig.Preferences
}

func NewHandler(service *application.Service, version string, preferences userconfig.Preferences) *Handler {
	return &Handler{service: service, version: version, preferences: preferences, measureControl: service.MeasureControl}
}

var NativeCheck = Tool{"check.run", "Run an approved check through the Host native process permission and environment.", application.RunRequest{}, RunResult{}, false}

func (s *Handler) Dispatch(ctx context.Context, name string, raw []byte) Envelope {
	for _, tool := range Catalog {
		if tool.Name == name {
			return s.dispatch(ctx, tool, raw)
		}
	}
	return failure(newID(), name, domain.Invalid("tool", "tool is not in the current catalog"))
}

func (s *Handler) RunCheck(ctx context.Context, raw []byte) Envelope {
	return s.dispatch(ctx, NativeCheck, raw)
}

func savedRead(name string, raw []byte) bool {
	switch name {
	case "taskbelay_server_info", "taskbelay_list_tasks", "taskbelay_get_run":
		return true
	case "taskbelay_get_task":
		var q GetRequest
		return Decode(raw, &q) == nil && q.Refresh != nil && !*q.Refresh
	}
	return false
}

func (s *Handler) dispatch(ctx context.Context, tool Tool, raw []byte) (envelope Envelope) {
	name := tool.Name
	readOnly := savedRead(name, raw)
	start := time.Now()
	id := newID()
	var e error
	if !readOnly {
		complete, err := s.measureControl(ctx)
		if err != nil {
			return meteringFailure(id, name, err)
		}
		defer complete()
	}
	taskID := ""
	var identity struct {
		TaskID      string `json:"task_id"`
		OperationID string `json:"operation_id"`
	}
	_ = json.Unmarshal(raw, &identity)
	taskID = identity.TaskID
	startedTask := taskID
	if taskID != "" && !readOnly {
		_ = s.service.Store.StartCall(ctx, taskID)
	}
	if domain.ID(identity.OperationID).IsValid() {
		id = identity.OperationID
	}
	envelope = Envelope{RequestID: id, Tool: name}
	defer func() {
		if taskID != "" && !readOnly {
			if startedTask == "" {
				_ = s.service.Store.StartCall(context.WithoutCancel(ctx), taskID)
			}
			_ = s.service.Store.AddCost(context.WithoutCancel(ctx), taskID, time.Since(start).Milliseconds(), 0)
		}
	}()
	var result any
	detail := false
	e = nil
	switch name {
	case "taskbelay_server_info":
		var q Empty
		e = Decode(raw, &q)
		if e == nil {
			names := []string{}
			for _, d := range Catalog {
				names = append(names, d.Name)
			}
			result = map[string]any{"startup_diagnostics": map[string]any{"core_reached": true, "shared_contract": "CLI, MCP and HTTP use the same service", "host_mcp_visibility": "check tools visible in the current Host; CLI success does not establish MCP availability"}, "product": "taskbelay", "version": s.version, "schema": store.DatabaseSchemaVersion, "tools": names, "host_preferences": s.preferences, "coverage": map[string]any{"core_execution": "local argv only; not a sandbox", "codex": "apply_patch path preflight; other writes observed at checkpoints", "claude": "Write/Edit/NotebookEdit path preflight; other writes observed at checkpoints", "zcode": "registered structured writes only; Host capability must be verified", "deepseek": "write/edit/str_replace_editor path preflight; other writes observed at checkpoints"}, "semantic_evidence": "Host judgment with cited basis and uncertainty; not a correctness proof", "cost": "Mutating Core calls are metered; saved-state reads do not write cost or query process identity. Host usage is reported.", "native_check": map[string]any{"command": "check run", "input_schema": Schema(NativeCheck.Input), "output_schema": OutputSchema(NativeCheck.Output)}}
			cost, err := s.service.SavedControlCost(ctx)
			e = err
			result.(map[string]any)["control_cost"] = cost
		}
	case "taskbelay_open_task":
		var q application.OpenRequest
		if e = Decode(raw, &q); e == nil {
			var v application.View
			v, e = s.service.Open(ctx, q)
			result = v
			taskID = v.Task.TaskID
		}
	case "taskbelay_get_task":
		var q GetRequest
		if e = Decode(raw, &q); e == nil {
			refresh := q.Refresh == nil || *q.Refresh
			detail = q.Detail
			result, e = s.service.Inspect(ctx, q.TaskID, refresh)
		}
	case "taskbelay_update_budget":
		var q application.BudgetRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.UpdateBudget(ctx, q)
		}
	case "taskbelay_accept_known_failure":
		var q application.AcceptFailureRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.AcceptFailure(ctx, q)
		}
	case "taskbelay_archive_task":
		var q application.ArchiveRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Archive(ctx, q)
		}
	case "taskbelay_resolve_issue":
		var q application.ResolveIssueRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.ResolveIssue(ctx, q)
		}
	case "taskbelay_checkpoint":
		var q application.CheckpointRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Checkpoint(ctx, q)
		}
	case "taskbelay_revise_task":
		var q application.ReviseRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Revise(ctx, q)
		}
	case "check.run":
		var q application.RunRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Execute(ctx, q)
		}
	case "taskbelay_record_evidence":
		var q application.EvidenceRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.RecordEvidence(ctx, q)
		}
	case "taskbelay_get_run":
		var q GetRunRequest
		if e = Decode(raw, &q); e == nil {
			if !domain.ID(q.RunID).IsValid() {
				e = domain.Invalid("run_id", "requires a canonical run identity")
			} else {
				var r execution.Run
				e = s.service.Store.Run(ctx, q.RunID, &r)
				result = r
				taskID = r.TaskID
			}
		}
	case "taskbelay_recover_run":
		var q application.RecoverRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Recover(ctx, q)
		}
	case "taskbelay_finish_task":
		var q application.FinishRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Finish(ctx, q)
		}
	case "taskbelay_cancel_task":
		var q application.CancelRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.Cancel(ctx, q)
		}
	case "taskbelay_list_tasks":
		var q store.ListQuery
		if e = Decode(raw, &q); e == nil {
			var page store.TaskPage
			page, e = s.service.Store.List(ctx, q)
			items := []map[string]any{}
			for _, t := range page.Items {
				items = append(items, Summary(t))
			}
			cost, err := s.service.SavedControlCost(ctx)
			if e == nil {
				e = err
			}
			result = map[string]any{"items": items, "page": page.Page, "has_next": page.HasNext, "control_cost": cost}
		}
	}
	if e != nil {
		return failure(id, name, e)
	}
	switch v := result.(type) {
	case application.View:
		result = taskResult(v, detail)
	case application.RunResult:
		result = runResult(v)
	}
	envelope.OK = true
	envelope.Result = result
	return envelope
}
func Summary(t domain.Task) map[string]any {
	lifecycle := t.Status
	node := strings.ToUpper(t.Status)
	var blocker *string
	if t.Status == "active" && t.Issue != nil {
		lifecycle = "blocked"
		node = "BLOCKED"
		blocker = &t.Issue.Summary
	}
	for _, f := range t.Facts {
		if t.Status == "active" && f.Blocking {
			lifecycle = "blocked"
			node = "BLOCKED"
			x := f.Detail
			blocker = &x
		}
	}
	keys := []string{}
	for _, r := range t.Repositories {
		keys = append(keys, string(r.Key))
	}
	return map[string]any{"task_id": t.TaskID, "request_summary": t.Authorization.Request, "origin_host": t.Host, "execution_host": t.Host, "current_node": node, "lifecycle": lifecycle, "revision": t.Revision, "updated_at": t.UpdatedAt, "archived": t.Archived, "repository_keys": keys, "repository_group_id": t.Repositories[0].Origin.SourceRepositoryGroupDigest, "worktree_path": t.Repositories[0].Origin.CanonicalWorktreeRoot, "blocker": blocker, "outcome": t.Summary}
}
func newID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "request-unavailable"
	}
	return "request-" + hex.EncodeToString(b)
}
