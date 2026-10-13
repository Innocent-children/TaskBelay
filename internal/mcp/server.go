// Package mcp is a transport projection of the engineering Core; it owns no workflow.
package mcp

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
	{"taskbelay_open_task", "Save the understanding and complete proposed plan. Implementation and checks require actual user confirmation of its digest.", application.OpenRequest{}, application.View{}, false},
	{"taskbelay_get_task", "Refresh actual code changes and evidence applicability; recover latest requirements and progress.", GetRequest{}, application.View{}, false},
	{"taskbelay_checkpoint", "Record current reasoning, necessary changes, progress and cost. This cannot change the approved plan or check definitions.", application.CheckpointRequest{}, application.View{}, false},
	{"taskbelay_revise_task", "Replace the current user-authorized contract; preserve unaffected work/evidence and cumulative cost.", application.ReviseRequest{}, application.View{}, false},
	{"taskbelay_run_check", "Execute one approved argv locally with a durable reservation and actual result. Respect native Host permission.", application.RunRequest{}, application.RunResult{}, false},
	{"taskbelay_record_evidence", "Record an agent claim or a completed human observation; cannot manufacture Core execution.", application.EvidenceRequest{}, application.View{}, false},
	{"taskbelay_get_run", "Read a saved actual execution after response loss; never executes a command.", GetRunRequest{}, execution.Run{}, true},
	{"taskbelay_recover_run", "Recover a signed result receipt or classify a stopped owner with no receipt; never infer passing or replay execution.", application.RecoverRequest{}, application.RunResult{}, false},
	{"taskbelay_finish_task", "Conclude only with current scope reasoning and applicable passing evidence for every authorized goal.", application.FinishRequest{}, application.View{}, false},
	{"taskbelay_cancel_task", "End the task and release claims while retaining files and results; unresolved execution prevents cancellation.", application.CancelRequest{}, application.View{}, false},
	{"taskbelay_list_tasks", "Read a bounded page of saved task summaries, including archived tasks when requested.", store.ListQuery{}, map[string]any{}, true},
	{"taskbelay_confirm_plan", "Record the actual user's understanding and approval of the exact displayed plan; never fabricate a confirmation.", application.ConfirmPlanRequest{}, application.View{}, false},
	{"taskbelay_accept_known_failure", "Record a user's explicit acceptance of named existing regression failures with current actual comparison evidence; never waive acceptance checks.", application.AcceptFailureRequest{}, application.View{}, false},
	{"taskbelay_update_budget", "Apply one explicitly approved complete budget decision; synchronize Core and Host work-time caps and retain the original plan authority.", application.BudgetRequest{}, application.View{}, false},
	{"taskbelay_archive_task", "Archive or restore a terminal task without changing its outcome or deleting work.", application.ArchiveRequest{}, application.View{}, false},
}

type ServerOptions struct {
	Instructions    string
	HostPreferences userconfig.Preferences
}
type Server struct {
	measureControl func(context.Context) (func(), error)
	service        *application.Service
	sdk            *sdk.Server
	version        string
	options        ServerOptions
}

func NewServer(s *application.Service, version string, options *ServerOptions) (*Server, error) {
	if s == nil || version == "" {
		return nil, domain.ErrInvalidArgument
	}
	v := &Server{service: s, version: version}
	if options != nil {
		v.options = *options
	}
	v.measureControl = v.service.MeasureControl
	v.sdk = sdk.NewServer(&sdk.Implementation{Name: "taskbelay", Version: version}, &sdk.ServerOptions{Instructions: v.options.Instructions})
	for _, d := range Catalog {
		d := d
		destructive := d.Name == "taskbelay_run_check"
		openWorld := destructive
		v.sdk.AddTool(&sdk.Tool{Name: d.Name, Description: d.Description, InputSchema: Schema(d.Input), OutputSchema: OutputSchema(d.Output), Annotations: &sdk.ToolAnnotations{ReadOnlyHint: d.ReadOnly, IdempotentHint: !destructive, DestructiveHint: &destructive, OpenWorldHint: &openWorld}}, func(ctx context.Context, r *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			raw := json.RawMessage(`{}`)
			if r != nil && r.Params != nil {
				raw = r.Params.Arguments
			}
			value := v.Dispatch(ctx, d.Name, raw)
			b, e := json.Marshal(value)
			if e != nil {
				return nil, e
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, StructuredContent: json.RawMessage(b), IsError: !value.OK}, nil
		})
	}
	return v, nil
}
func (s *Server) Run(ctx context.Context, t sdk.Transport) error { return s.sdk.Run(ctx, t) }
func (s *Server) Dispatch(ctx context.Context, name string, raw []byte) (envelope Envelope) {
	start := time.Now()
	id := newID()
	complete, e := s.measureControl(ctx)
	if e != nil {
		return meteringFailure(id, name, e)
	}
	defer complete()
	taskID := ""
	var identity struct {
		TaskID      string `json:"task_id"`
		OperationID string `json:"operation_id"`
	}
	_ = json.Unmarshal(raw, &identity)
	taskID = identity.TaskID
	startedTask := taskID
	if taskID != "" {
		_ = s.service.Store.StartCall(ctx, taskID)
	}
	if domain.ID(identity.OperationID).IsValid() {
		id = identity.OperationID
	}
	envelope = Envelope{RequestID: id, Tool: name}
	defer func() {
		if taskID != "" {
			if startedTask == "" {
				_ = s.service.Store.StartCall(context.WithoutCancel(ctx), taskID)
			}
			_ = s.service.Store.AddCost(context.WithoutCancel(ctx), taskID, time.Since(start).Milliseconds(), 0)
		}
	}()
	fail := func(e error) Envelope {
		var de *domain.Error
		if !errors.As(e, &de) {
			de = &domain.Error{Code: "INTERNAL_ERROR", Message: "Core could not complete the operation; inspect saved task and run state before retrying."}
		}
		r := Recovery{Action: "read_task", Message: "Read current saved state; do not blindly replay the operation."}
		switch de.Code {
		case "INVALID_ARGUMENT", "ASSESSMENT_STALE", "EVIDENCE_STALE":
			r.Action = "correct_request"
			r.Message = "Correct only the identified fields from existing facts; preserve operation identity and real authorization."
			for _, d := range de.Details {
				r.AllowedPaths = append(r.AllowedPaths, d.Path)
			}
		case "EXECUTION_UNCERTAIN", "EXECUTION_PENDING", "EXECUTION_RUNNING":
			r.Action = "recover_run"
			r.Message = "Read the original run and recover its actual receipt; never infer success or repeat an uncertain command."
		case "STORAGE_CORRUPT", "SCHEMA_UNSUPPORTED":
			r.Action = "repair_storage"
			r.Message = "Preserve local data and report the storage problem; do not reset or migrate it."
		case "COST_LIMIT", "PLAN_CONFIRMATION_REQUIRED", "PLAN_CHANGED", "AUTHORIZATION_PENDING":
			r.Action = "request_authorization"
			r.Message = "Show the current plan or required decision and obtain actual user authorization; preserve acceptance requirements."
		case "PLAN_BUDGET_INFEASIBLE", "BUDGET_UNCHANGED", "SCOPE_VIOLATION", "REPEATED_FAILURE", "ACCEPTANCE_INCOMPLETE", "CHECK_ALREADY_SATISFIED", "RETRY_BASIS_REQUIRED", "UNAPPROVED_CHANGES", "FAILURE_ACCEPTANCE_NOT_ALLOWED":
			r.Action = "self_correct"
			r.Message = "Inspect the current facts and gaps, then correct within existing authorization."
		}
		return Envelope{RequestID: id, Tool: name, Error: de, Recovery: &r}
	}
	found := false
	for _, d := range Catalog {
		found = found || d.Name == name
	}
	if !found {
		return fail(domain.Invalid("tool", "tool is not in the current catalog"))
	}
	var result any
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
			result = map[string]any{"startup_diagnostics": map[string]any{"core_reached": true, "shared_contract": "CLI, MCP and HTTP use the same service", "host_mcp_visibility": "check tools visible in the current Host; CLI success does not establish MCP availability"}, "product": "taskbelay", "version": s.version, "schema": store.DatabaseSchemaVersion, "tools": names, "host_preferences": s.options.HostPreferences, "coverage": map[string]any{"core_execution": "local argv only; not a sandbox", "codex": "apply_patch path preflight; other writes observed at checkpoints", "claude": "Write/Edit/NotebookEdit path preflight; other writes observed at checkpoints", "zcode": "registered structured writes only; Host capability must be verified", "deepseek": "write/edit/str_replace_editor path preflight; other writes observed at checkpoints"}, "semantic_evidence": "Host judgment with cited basis and uncertainty; not a correctness proof", "cost": "Core wall/observation/execution measured; Host analysis, tokens and wait are reported, potentially incomplete"}
			cost, err := s.service.ControlCost(ctx)
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
			result, e = s.service.Inspect(ctx, q.TaskID, refresh)
		}
	case "taskbelay_update_budget":
		var q application.BudgetRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.UpdateBudget(ctx, q)
		}
	case "taskbelay_confirm_plan":
		var q application.ConfirmPlanRequest
		if e = Decode(raw, &q); e == nil {
			result, e = s.service.ConfirmPlan(ctx, q)
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
	case "taskbelay_run_check":
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
			cost, err := s.service.ControlCost(ctx)
			if e == nil {
				e = err
			}
			result = map[string]any{"items": items, "page": page.Page, "has_next": page.HasNext, "control_cost": cost}
		}
	}
	if e != nil {
		return fail(e)
	}
	envelope.OK = true
	envelope.Result = result
	return envelope
}
func Summary(t domain.Task) map[string]any {
	lifecycle := t.Status
	node := strings.ToUpper(t.Status)
	var blocker *string
	if t.Status == "active" && !t.PlanApproved() {
		lifecycle = "blocked"
		node = "BLOCKED"
		x := "Waiting for confirmation of the displayed plan"
		blocker = &x
	}
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

func Decode(raw []byte, out any) error {
	if len(raw) > 1024*1024 {
		return domain.Invalid("arguments", "request exceeds 1 MiB")
	}
	if !utf8.Valid(raw) {
		return domain.Invalid("arguments", "request must be UTF-8")
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return domain.Invalid("arguments", "request must be one JSON object")
	}
	tokens := json.NewDecoder(bytes.NewReader(raw))
	tokens.UseNumber()
	if e := walkJSON(tokens, "arguments", Schema(out)); e != nil {
		return e
	}
	if _, e := tokens.Token(); e != io.EOF {
		return domain.Invalid("arguments", "trailing JSON is not allowed")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(e, &mismatch) {
			return domain.Invalid(mismatch.Field, "requires "+mismatch.Type.String())
		}
		return domain.Invalid("arguments", "request contains an unknown member or invalid value")
	}
	return nil
}
func walkJSON(d *json.Decoder, p string, schema map[string]any) error {
	if choices, ok := schema["anyOf"].([]any); ok && len(choices) > 0 {
		schema, _ = choices[0].(map[string]any)
	}
	t, e := d.Token()
	if e != nil {
		return domain.Invalid(p, "malformed JSON")
	}
	delim, ok := t.(json.Delim)
	if t != nil {
		kind := ""
		switch t.(type) {
		case string:
			kind = "string"
		case bool:
			kind = "boolean"
		case json.Number:
			kind = "integer"
		}
		if ok {
			if delim == '{' {
				kind = "object"
			} else if delim == '[' {
				kind = "array"
			}
		}
		if required, has := schema["type"].(string); has && required != kind {
			return domain.Invalid(p, "requires "+required)
		}
	}
	if !ok {
		return nil
	}
	if delim == '{' {
		properties, _ := schema["properties"].(map[string]any)
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return domain.Invalid(p, "malformed object")
			}
			k := key.(string)
			if seen[k] {
				return domain.Invalid(p+"."+k, "duplicate JSON member")
			}
			seen[k] = true
			child, known := properties[k].(map[string]any)
			if !known && schema["additionalProperties"] == false {
				return domain.Invalid(p+"."+k, "member is not allowed by the current request")
			}
			if e = walkJSON(d, p+"."+k, child); e != nil {
				return e
			}
		}
	} else if delim == '[' {
		item, _ := schema["items"].(map[string]any)
		i := 0
		for d.More() {
			if e = walkJSON(d, fmt.Sprintf("%s[%d]", p, i), item); e != nil {
				return e
			}
			i++
		}
	}
	_, e = d.Token()
	if e != nil {
		return domain.Invalid(p, "malformed JSON")
	}
	return nil
}

// Schemas derive from the same DTOs as every transport, with closed object members.
func Schema(v any) map[string]any { return typeSchema(reflect.TypeOf(v)) }
func typeSchema(t reflect.Type) map[string]any {
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeFor[domain.Digest]() {
		return map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
	}
	if t.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{typeSchema(t.Elem()), map[string]any{"type": "null"}}}
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				x := typeSchema(f.Type)
				for k, v := range x["properties"].(map[string]any) {
					props[k] = v
				}
				required = append(required, x["required"].([]string)...)
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "-" || f.PkgPath != "" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			props[name] = typeSchema(f.Type)
			if !strings.Contains(tag, "omitempty") {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	case reflect.Slice:
		return map[string]any{"type": []string{"array", "null"}, "items": typeSchema(t.Elem())}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64, reflect.Int32:
		return map[string]any{"type": "integer"}
	case reflect.Map:
		return map[string]any{"type": "object"}
	default:
		return map[string]any{}
	}
}
func OutputSchema(v any) map[string]any {
	base := map[string]any{"request_id": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}}
	success := map[string]any{}
	failure := map[string]any{}
	for k, x := range base {
		success[k] = x
		failure[k] = x
	}
	success["ok"] = map[string]any{"const": true}
	success["result"] = Schema(v)
	failure["ok"] = map[string]any{"const": false}
	failure["error"] = Schema(domain.Error{})
	failure["recovery"] = Schema(Recovery{})
	return map[string]any{"type": "object", "oneOf": []any{map[string]any{"properties": success, "required": []string{"ok", "request_id", "tool", "result"}, "additionalProperties": false}, map[string]any{"properties": failure, "required": []string{"ok", "request_id", "tool", "error", "recovery"}, "additionalProperties": false}}}
}

func meteringFailure(id, name string, err error) Envelope {
	phase := "initialization"
	var meter *application.ControlMeterError
	if errors.As(err, &meter) {
		phase = meter.Stage
	}
	code := "CONTROL_INITIALIZATION_FAILED"
	action := "inspect_initialization"
	message := "Core was reached, but control metering initialization failed; the tool was not executed. Inspect the reported phase and original cause; preserve saved state."
	switch phase {
	case "process_identity":
		code = "PROCESS_IDENTITY_UNAVAILABLE"
		action = "inspect_process_access"
		message = "Core was reached, but the Host could not query process identity. Inspect the original process-query error and restore required Host access; do not bypass denial or settle unknown executions. The tool was not executed."
	case "storage_read", "storage_write":
		code = "STORAGE_UNAVAILABLE"
		action = "repair_storage"
		message = "Core was reached, but control metering storage access failed. Inspect the original storage error and data-directory access; preserve data. The tool was not executed."
		var original *domain.Error
		if errors.As(err, &original) {
			code = original.Code
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = "CONTROL_INITIALIZATION_CANCELLED"
		action = "inspect_host_context"
		message = "Control metering initialization was cancelled or timed out; the tool was not executed. Inspect the Host context before another authorized call."
	}
	if phase == "process_identity" && (errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "operation not permitted") || strings.Contains(strings.ToLower(err.Error()), "permission denied")) {
		code = "PROCESS_IDENTITY_DENIED"
	}
	return Envelope{RequestID: id, Tool: name, Error: &domain.Error{Code: code, Message: "control metering initialization failed; tool was not executed", Details: []domain.ContractViolation{{Path: "control_meter." + phase, Rule: "initialization", Message: err.Error()}}}, Recovery: &Recovery{Action: action, RetrySafe: false, Message: message}}
}
