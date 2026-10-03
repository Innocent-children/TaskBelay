package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	"github.com/Innocent-children/taskbelay/internal/workflow"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type RequestIDGenerator func() (domain.ID, error)
type ServerOptions struct {
	Diagnostics     *Diagnostics
	NewRequestID    RequestIDGenerator
	Instructions    string
	HostPreferences userconfig.Preferences
}
type Server struct {
	application     *application.Service
	version         string
	sdk             *sdk.Server
	newRequestID    RequestIDGenerator
	hostPreferences userconfig.Preferences
}

func NewServer(service *application.Service, version string, options *ServerOptions) (*Server, error) {
	if service == nil || strings.TrimSpace(version) != version || version == "" {
		return nil, domain.ErrInvalidArgument
	}
	newID := randomRequestID
	if options != nil && options.NewRequestID != nil {
		newID = options.NewRequestID
	}
	preferences := userconfig.Preferences{}
	instructions := ""
	if options != nil {
		preferences = options.HostPreferences
		instructions = options.Instructions
	}
	s := &Server{application: service, version: version, newRequestID: newID, hostPreferences: preferences}
	s.sdk = sdk.NewServer(&sdk.Implementation{Name: "taskbelay", Title: "TaskBelay Core", Description: "Local STDIO TaskBelay Core", Version: version}, &sdk.ServerOptions{Instructions: instructions})
	for _, d := range catalog {
		d := d
		destructive, openWorld := d.Annotations.Destructive, d.Annotations.OpenWorld
		s.sdk.AddTool(&sdk.Tool{Name: d.Name, Description: d.Description, InputSchema: json.RawMessage(d.InputSchema), OutputSchema: json.RawMessage(d.OutputSchema), Annotations: &sdk.ToolAnnotations{ReadOnlyHint: d.Annotations.ReadOnly, IdempotentHint: d.Annotations.Idempotent, DestructiveHint: &destructive, OpenWorldHint: &openWorld}}, func(ctx context.Context, r *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			raw := json.RawMessage(`{}`)
			if r != nil && r.Params != nil {
				raw = r.Params.Arguments
			}
			id, err := s.newRequestID()
			if err != nil || !id.IsValid() {
				encoded := failureFallback("", d.Name, "Core could not generate a valid request identity; the tool was not executed.")
				return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded.JSON)}}, StructuredContent: json.RawMessage(encoded.JSON), IsError: true}, nil
			}
			encoded := s.dispatch(ctx, d.Name, id, raw)
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(encoded.JSON)}}, StructuredContent: json.RawMessage(encoded.JSON), IsError: encoded.IsError}, nil
		})
	}
	return s, nil
}
func (s *Server) Run(ctx context.Context, t sdk.Transport) error { return s.sdk.Run(ctx, t) }
func (s *Server) dispatch(ctx context.Context, tool string, id domain.ID, raw []byte) EncodedResult {
	resultID := resultEnvelopeRequestID(tool, raw, id)
	if err := ValidateToolInput(tool, raw); err != nil {
		return EncodeError(string(resultID), tool, err)
	}
	if kind, ok := submissionKindForTool(tool); ok {
		var wire submitActionWire
		_ = decodeClosed(raw, &wire)
		result, err := s.application.SubmitAction(ctx, toSubmitAction(wire, id, kind))
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectTask(result.Task))
	}
	switch tool {
	case ToolServerInfo:
		d := workflow.StandardProcess()
		process := SupportedProcessResult{ProcessID: d.Reference.ID, DefinitionDigest: d.Reference.DefinitionDigest, NewTaskSupported: true}
		return EncodeSuccess(string(resultID), tool, ServerInfoResult{Product: "taskbelay", Version: s.version, Transport: "stdio", Health: "ready", SupportedProcesses: []SupportedProcessResult{process}, SupportedHosts: []string{"codex", "deepseek", "claude", "zcode"}, MethodProfiles: []domain.MethodProfile{domain.MethodPlain, domain.MethodSpecKit, domain.MethodOpenSpec}, Tools: ToolNames(), HostPreferences: s.hostPreferences})
	case ToolOpenTask:
		var w openWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.OpenTask(ctx, toOpen(w, id))
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, map[string]any{"created": r.Created, "task": projectTask(r.Task), "recovery_assessment": projectRecoveryAssessment(r.RecoveryAssessment)})
	case ToolGetTask:
		var w readWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.GetTask(ctx, application.GetTaskRequest{Host: w.Host, TaskID: w.TaskID, OperationProbe: toProbe(w.OperationProbe), BaselineHistory: w.BaselineHistory})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		result := map[string]any{"task": projectTask(r.Task), "recovery_assessment": projectRecoveryAssessment(r.RecoveryAssessment)}
		if r.BaselineHistory != nil {
			result["baseline_history"] = r.BaselineHistory
		}
		return EncodeSuccess(string(resultID), tool, result)
	case ToolGetNextAction:
		var w readWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.GetNextAction(ctx, application.GetNextActionRequest{Host: w.Host, TaskID: w.TaskID, OperationProbe: toProbe(w.OperationProbe)})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectNextAction(r))
	case ToolResolveBlocker:
		var wire resolveBlockerWire
		_ = decodeClosed(raw, &wire)
		var decision *domain.FileScopeDecisionInput
		if wire.Choice != "" {
			decision = &domain.FileScopeDecisionInput{Choice: wire.Choice, Reason: wire.Reason}
		}
		result, err := s.application.ResolveBlockerAction(ctx, application.RecoverActionRequest{Host: wire.Host, TaskID: wire.TaskID, ActionID: wire.ActionID, FileScopeDecision: decision, RenameID: wire.RenameID, RenameChoice: wire.RenameChoice, RelocationID: wire.RelocationID, RelocationDestinations: wire.RelocationDestinations, HistoryResolution: wire.HistoryResolution}, id)
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectTask(result.Task))
	case ToolRecoverAction:
		var wire actionReferenceWire
		_ = decodeClosed(raw, &wire)
		result, err := s.application.RecoverAction(ctx, application.RecoverActionRequest{Host: wire.Host, TaskID: wire.TaskID, ActionID: wire.ActionID})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectTask(result.Task))
	case ToolCancelTask:
		var w cancelWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.CancelTask(ctx, application.CancelTaskRequest{RequestID: w.RequestID, Host: w.Host, TaskID: w.TaskID, ExpectedRevision: w.Revision, Reason: w.Reason})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectTask(r.Task))
	case ToolPrepareTaskBranchRename:
		var w prepareBranchRenameWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.PrepareTaskBranchRename(ctx, application.PrepareTaskBranchRenameRequest{RequestID: id, Host: w.Host, TaskID: w.TaskID, ExpectedRevision: w.Revision, RepositoryKey: w.RepositoryKey, TargetBranch: w.TargetBranch, Reason: w.Reason})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, map[string]any{"rename_id": r.RenameID, "task": projectTask(r.Task)})
	case ToolPrepareTaskRelocation:
		var w lifecycleWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.PrepareTaskRelocation(ctx, application.PrepareTaskRelocationRequest{RequestID: id, Host: w.Host, TaskID: w.TaskID, ExpectedRevision: w.Revision})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, map[string]any{"relocation_id": r.RelocationID, "task": projectTask(r.Task)})
	case ToolAbandonTask:
		var w abandonWire
		_ = decodeClosed(raw, &w)
		r, err := s.application.AbandonTask(ctx, application.AbandonTaskRequest{RequestID: id, Host: w.Host, TaskID: w.TaskID, ExpectedRevision: w.Revision, Reason: w.Reason})
		if err != nil {
			return EncodeError(string(resultID), tool, err)
		}
		return EncodeSuccess(string(resultID), tool, projectTask(r.Task))
	default:
		return EncodeError(string(resultID), tool, domain.ErrInvalidArgument)
	}
}

func resultEnvelopeRequestID(tool string, raw []byte, generated domain.ID) domain.ID {
	if tool != ToolCancelTask || !generated.IsValid() || workflow.ValidateRequestJSON("arguments", raw) != nil {
		return generated
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return generated
	}
	var caller domain.ID
	if json.Unmarshal(object["request_id"], &caller) != nil || !caller.IsValid() {
		return generated
	}
	return caller
}
func randomRequestID() (domain.ID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return domain.ID("request-" + hex.EncodeToString(b[:])), nil
}
