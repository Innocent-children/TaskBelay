package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type openWire struct {
	Host                   domain.Host                `json:"host"`
	RepositoryPath         string                     `json:"repository_path"`
	WorkspaceOrigin        *workspaceOriginWire       `json:"workspace_origin"`
	PrimaryRepositoryKey   domain.RepositoryKey       `json:"primary_repository_key"`
	AdditionalRepositories []additionalRepositoryWire `json:"additional_repositories"`
	NewTask                *struct {
		Request                 string               `json:"request"`
		InitialScope            []string             `json:"initial_scope"`
		InitialOutOfScope       []string             `json:"initial_out_of_scope"`
		KnownAcceptanceCriteria []string             `json:"known_acceptance_criteria"`
		MethodProfile           domain.MethodProfile `json:"method_profile"`
	} `json:"new_task"`
}
type additionalRepositoryWire struct {
	Key             domain.RepositoryKey `json:"key"`
	RepositoryPath  string               `json:"repository_path"`
	WorkspaceOrigin workspaceOriginWire  `json:"workspace_origin"`
}
type workspaceOriginWire struct {
	Mode                  domain.WorkspaceMode `json:"mode"`
	SourceType            string               `json:"source_type"`
	CarryChanges          *bool                `json:"carry_changes"`
	RemoteName            *string              `json:"remote_name"`
	BaseBranch            string               `json:"base_branch"`
	BaseCommit            string               `json:"base_commit"`
	TaskBranch            string               `json:"task_branch"`
	ProvisioningReceiptID domain.ID            `json:"provisioning_receipt_id"`
}
type readWire struct {
	Host            domain.Host                  `json:"host"`
	TaskID          domain.ID                    `json:"task_id"`
	OperationProbe  *operationProbeWire          `json:"operation_probe"`
	BaselineHistory *domain.BaselineHistoryQuery `json:"baseline_history"`
}
type operationProbeWire struct {
	OperationID             domain.ID         `json:"operation_id"`
	ProcessID               domain.ProcessID  `json:"process_id"`
	ProcessDefinitionDigest domain.Digest     `json:"process_definition_digest"`
	SourceCursor            domain.NodeID     `json:"source_cursor"`
	ExpectedRevision        uint64            `json:"expected_revision"`
	ActionID                domain.ID         `json:"action_id"`
	ActionKind              domain.ActionKind `json:"action_kind"`
	RepositoryBindingDigest domain.Digest     `json:"repository_binding_digest"`
	IssuanceIdentityDigest  domain.Digest     `json:"issuance_identity_digest"`
	IssuanceHistoryDigest   domain.Digest     `json:"issuance_history_digest"`
	IssuanceContentDigest   domain.Digest     `json:"issuance_content_digest"`
	Payload                 json.RawMessage   `json:"payload"`
}
type artifactSubmissionWire struct {
	Path    string        `json:"path"`
	Digest  domain.Digest `json:"digest"`
	Summary string        `json:"summary"`
}
type methodResultSubmissionWire struct {
	Capability string `json:"capability"`
	Summary    string `json:"summary"`
}
type submitActionWire struct {
	Host         domain.Host         `json:"host"`
	TaskID       domain.ID           `json:"task_id"`
	ActionID     domain.ID           `json:"action_id"`
	TransitionID domain.TransitionID `json:"transition_id"`
	Summary      string              `json:"summary"`
	Reason       string              `json:"reason"`
	Artifacts    struct {
		Current      []artifactSubmissionWire `json:"current"`
		OtherProcess []artifactSubmissionWire `json:"other_process"`
	} `json:"artifacts"`
	MethodResults map[domain.MethodStepID]methodResultSubmissionWire `json:"method_results"`
	NodeResult    json.RawMessage                                    `json:"node_result"`
}
type actionReferenceWire struct {
	Host     domain.Host `json:"host"`
	TaskID   domain.ID   `json:"task_id"`
	ActionID domain.ID   `json:"action_id"`
}
type resolveBlockerWire struct {
	RenameID               domain.ID                               `json:"rename_id"`
	RenameChoice           string                                  `json:"rename_choice"`
	Host                   domain.Host                             `json:"host"`
	TaskID                 domain.ID                               `json:"task_id"`
	ActionID               domain.ID                               `json:"action_id"`
	Choice                 domain.FileScopeDecision                `json:"choice"`
	Reason                 string                                  `json:"reason"`
	RelocationID           domain.ID                               `json:"relocation_id"`
	RelocationDestinations []domain.RelocationDestination          `json:"relocation_destinations"`
	HistoryResolution      *domain.WorkspaceHistoryResolutionInput `json:"history_resolution"`
}
type cancelWire struct {
	RequestID domain.ID   `json:"request_id"`
	Host      domain.Host `json:"host"`
	TaskID    domain.ID   `json:"task_id"`
	Revision  uint64      `json:"revision"`
	Reason    string      `json:"reason"`
}
type lifecycleWire struct {
	Host     domain.Host `json:"host"`
	TaskID   domain.ID   `json:"task_id"`
	Revision uint64      `json:"revision"`
}
type abandonWire struct {
	Host     domain.Host `json:"host"`
	TaskID   domain.ID   `json:"task_id"`
	Revision uint64      `json:"revision"`
	Reason   string      `json:"reason"`
}

func decodeClosed(raw []byte, out any) error {
	if err := workflow.ValidateRequestJSON("arguments", raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(err, &mismatch) {
			path := mismatch.Field
			if !domain.ValidViolationPath(path) {
				path = "arguments"
			}
			return domain.InvalidArgumentViolations(domain.ExplainedViolation(path, domain.RuleValueType, "value cannot be decoded as "+mismatch.Type.String()))
		}
		return domain.InvalidArgumentViolations(domain.Violation("arguments", domain.RuleUnknownMember))
	}
	var x any
	if err := d.Decode(&x); err != io.EOF {
		return domain.InvalidArgumentViolations(domain.Violation("arguments", domain.RuleJSONMalformed))
	}
	return nil
}
func ValidateToolInput(tool string, raw []byte) error {
	if err := workflow.ValidateRequestJSON("arguments", raw); err != nil {
		return err
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return domain.InvalidArgumentViolations(domain.Violation("arguments", domain.RuleArgumentsObjectRequired))
	}
	if !isToolName(tool) {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The requested tool is not in the current 18-tool catalog.")
	}
	if violations := toolRequestMemberViolations(tool, raw); len(violations) != 0 {
		return domain.InvalidArgumentViolations(violations...)
	}
	if tool == ToolOpenTask && (!hasAnyKey(raw, "new_task") || nullField(raw, "new_task")) {
		var violations []domain.ContractViolation
		for _, name := range []string{"workspace_origin", "primary_repository_key", "additional_repositories"} {
			if hasAnyKey(raw, name) {
				violations = append(violations, domain.Violation(name, domain.RuleCreationMemberOnResume))
			}
		}
		if len(violations) != 0 {
			return domain.InvalidArgumentViolations(violations...)
		}
	}
	if violations := toolRequestStructureViolations(tool, raw); len(violations) != 0 {
		failure := domain.InvalidArgumentViolations(violations...)
		if tool == ToolOpenTask {
			var value openWire
			onlyOrigin := true
			for _, violation := range violations {
				onlyOrigin = onlyOrigin && (violation.Path == "workspace_origin" || strings.HasPrefix(violation.Path, "workspace_origin."))
			}
			if onlyOrigin && decodeClosed(raw, &value) == nil && value.NewTask != nil {
				failure.Code = domain.ErrorWorktreeProvisioningRequired
				failure.Message = domain.ErrWorktreeProvisioningRequired.Message
			}
		}
		return failure
	}
	if kind, ok := submissionKindForTool(tool); ok {
		return validateSubmitActionInput(kind, raw)
	}
	var violations []domain.ContractViolation
	switch tool {
	case ToolServerInfo:
		var v struct{}
		return decodeClosed(raw, &v)
	case ToolOpenTask:
		var v openWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, repositoryPathViolations("repository_path", v.RepositoryPath)...)
		primaryKey := v.PrimaryRepositoryKey
		if primaryKey == "" && !hasAnyKey(raw, "primary_repository_key") {
			primaryKey = domain.DefaultPrimaryRepositoryKey
		}
		if !primaryKey.IsValid() {
			violations = append(violations, domain.ExplainedViolation("primary_repository_key", domain.RuleValueFormat, "repository keys must match [a-z0-9][a-z0-9._-]{0,127}"))
		}
		if len(v.AdditionalRepositories) > domain.MaxAdditionalRepositories {
			violations = append(violations, domain.ExplainedViolation("additional_repositories", domain.RuleStringListTooLong, "at most 7 additional repositories are allowed"))
		}
		keys := map[domain.RepositoryKey]bool{primaryKey: true}
		for index, repository := range v.AdditionalRepositories {
			path := fmt.Sprintf("additional_repositories[%d]", index)
			if !repository.Key.IsValid() {
				violations = append(violations, domain.ExplainedViolation(path+".key", domain.RuleValueFormat, "repository keys must match [a-z0-9][a-z0-9._-]{0,127}"))
			} else if keys[repository.Key] {
				violations = append(violations, domain.ExplainedViolation(path+".key", domain.RuleStringListDuplicate, "repository keys must be unique across the primary and additional repositories"))
			}
			keys[repository.Key] = true
			violations = append(violations, repositoryPathViolations(path+".repository_path", repository.RepositoryPath)...)
			violations = append(violations, workspaceOriginViolations(path+".workspace_origin", repository.WorkspaceOrigin)...)
		}
		if v.NewTask != nil {
			if v.WorkspaceOrigin == nil {
				return &domain.Error{Code: domain.ErrorWorktreeProvisioningRequired, Message: domain.ErrWorktreeProvisioningRequired.Message, ZeroWrite: true,
					Violations: []domain.ContractViolation{domain.Violation("workspace_origin", domain.RuleWorkspaceOriginRequired)}}
			}
			originFailures := workspaceOriginViolations("workspace_origin", *v.WorkspaceOrigin)
			if len(originFailures) != 0 && len(violations) == 0 {
				failure := domain.InvalidArgumentViolations(originFailures...)
				failure.Code = domain.ErrorWorktreeProvisioningRequired
				failure.Message = domain.ErrWorktreeProvisioningRequired.Message
				return failure
			}
			violations = append(violations, originFailures...)
			violations = append(violations, textViolations("new_task.request", v.NewTask.Request, domain.MaxGoalBytes, true)...)
			if !v.NewTask.MethodProfile.IsValid() {
				violations = append(violations, domain.ExplainedViolation("new_task.method_profile", domain.RuleEnumValueInvalid, "method_profile must be plain, spec-kit or openspec"))
			}
			for _, list := range []struct {
				name   string
				values []string
			}{
				{"initial_scope", v.NewTask.InitialScope}, {"initial_out_of_scope", v.NewTask.InitialOutOfScope}, {"known_acceptance_criteria", v.NewTask.KnownAcceptanceCriteria},
			} {
				violations = append(violations, requestListViolations("new_task."+list.name, list.values)...)
			}
		}
	case ToolGetTask, ToolGetNextAction:
		var v readWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, operationProbeViolations(v.OperationProbe)...)
	case ToolCancelTask:
		var v cancelWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("request_id", v.RequestID)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, revisionViolations(v.Revision)...)
		violations = append(violations, textViolations("reason", v.Reason, domain.MaxReasonBytes, true)...)
	case ToolPrepareTaskBranchRename:
		var v prepareBranchRenameWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, revisionViolations(v.Revision)...)
		violations = append(violations, textViolations("reason", v.Reason, domain.MaxReasonBytes, true)...)
		if !v.RepositoryKey.IsValid() {
			violations = append(violations, domain.Violation("repository_key", domain.RuleValueFormat))
		}
		if !domain.ValidTaskBranchName(v.TargetBranch) {
			violations = append(violations, domain.Violation("target_branch", domain.RuleValueFormat))
		}
	case ToolPrepareTaskRelocation:
		var v lifecycleWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, revisionViolations(v.Revision)...)
	case ToolAbandonTask:
		var v abandonWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, revisionViolations(v.Revision)...)
		violations = append(violations, textViolations("reason", v.Reason, domain.MaxReasonBytes, true)...)
	case ToolResolveBlocker:
		var v resolveBlockerWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, idViolations("action_id", v.ActionID)...)
		if v.Choice == "" {
			if v.Reason != "" {
				violations = append(violations, domain.ExplainedViolation("choice", domain.RuleMemberDependency, "a file-scope reason requires choice: allow_once, expand_scope or reject"))
			}
		} else {
			if !v.Choice.IsValid() {
				violations = append(violations, domain.ExplainedViolation("choice", domain.RuleEnumValueInvalid, "choice must be allow_once, expand_scope or reject"))
			}
			violations = append(violations, textViolations("reason", v.Reason, domain.MaxReasonBytes, true)...)
		}
		if v.RenameID != "" {
			violations = append(violations, idViolations("rename_id", v.RenameID)...)
		}
		if v.RelocationID != "" {
			violations = append(violations, idViolations("relocation_id", v.RelocationID)...)
			if len(v.RelocationDestinations) == 0 {
				violations = append(violations, domain.Violation("relocation_destinations", domain.RuleRequiredCollectionNonEmpty))
			}
		}
		for index, destination := range v.RelocationDestinations {
			path := fmt.Sprintf("relocation_destinations[%d]", index)
			if !destination.Key.IsValid() {
				violations = append(violations, domain.ExplainedViolation(path+".key", domain.RuleValueFormat, "repository keys must match [a-z0-9][a-z0-9._-]{0,127}"))
			}
			violations = append(violations, repositoryPathViolations(path+".repository_path", destination.RepositoryPath)...)
		}
		if history := v.HistoryResolution; history != nil {
			if history.Choice != "accept_current_history" {
				violations = append(violations, domain.ExplainedViolation("history_resolution.choice", domain.RuleEnumValueInvalid, "choice must be accept_current_history"))
			}
			violations = append(violations, textViolations("history_resolution.reason", history.Reason, domain.MaxReasonBytes, true)...)
		}
	case ToolRecoverAction:
		var v actionReferenceWire
		if err := decodeClosed(raw, &v); err != nil {
			return err
		}
		violations = append(violations, hostViolations(v.Host)...)
		violations = append(violations, idViolations("task_id", v.TaskID)...)
		violations = append(violations, idViolations("action_id", v.ActionID)...)
	}
	if len(violations) != 0 {
		return domain.InvalidArgumentViolations(violations...)
	}
	return nil
}

func validateSubmitActionInput(kind domain.ActionKind, raw []byte) error {
	var value submitActionWire
	if err := decodeClosed(raw, &value); err != nil {
		return err
	}
	violations := hostViolations(value.Host)
	violations = append(violations, idViolations("task_id", value.TaskID)...)
	violations = append(violations, idViolations("action_id", value.ActionID)...)
	if !value.TransitionID.IsValid() {
		violations = append(violations, domain.ExplainedViolation("transition_id", domain.RuleValueFormat, "transition_id must be a non-empty semantic identifier using lowercase letters, digits, underscores, dots or hyphens"))
	}
	if len(value.Artifacts.Current)+len(value.Artifacts.OtherProcess) > domain.MaxArtifactReferencesPerAction {
		violations = append(violations, domain.ExplainedViolation("artifacts", domain.RuleStringListTooLong, fmt.Sprintf("current and other_process together must contain at most %d artifact references", domain.MaxArtifactReferencesPerAction)))
	}
	if len(value.MethodResults) > domain.MaxMethodEvidencePerAction {
		violations = append(violations, domain.ExplainedViolation("method_results", domain.RuleStringListTooLong, fmt.Sprintf("at most %d method results are allowed", domain.MaxMethodEvidencePerAction)))
	}
	if len(violations) != 0 {
		return domain.InvalidArgumentViolations(violations...)
	}
	node, err := workflow.NodeDefinitionForActionKind(workflow.StandardProcess(), kind)
	if err != nil {
		return domain.WithExplanation(err, "The submission tool has no node in the current process definition.")
	}
	if _, err := workflow.TransitionFor(workflow.StandardProcess(), node.NodeID, value.TransitionID); err != nil {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "transition_id is not an outgoing transition of the node handled by this submission tool.")
	}
	if err := workflow.ValidateSubmissionNodeResult(kind, value.NodeResult); err != nil {
		return err
	}
	if _, allowed := workflow.PrimaryArtifactRoleForNode(node.NodeID); !allowed && len(value.Artifacts.Current) != 0 {
		return domain.InvalidArgumentViolations(domain.Violation("artifacts.current", domain.RuleArtifactRoleNotAllowed))
	}
	return nil
}
func missingRequestMembers(raw []byte, keys ...string) []domain.ContractViolation {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	var out []domain.ContractViolation
	for _, key := range keys {
		if _, present := value[key]; !present {
			out = append(out, domain.Violation(key, domain.RuleRequiredMemberMissing))
		}
	}
	return out
}

func hasAnyKey(raw []byte, keys ...string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	for _, key := range keys {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
}
func nullField(raw []byte, key string) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	field, ok := value[key]
	return ok && bytes.Equal(bytes.TrimSpace(field), []byte("null"))
}
func toOpen(w openWire, id domain.ID) application.OpenTaskRequest {
	r := application.OpenTaskRequest{RequestID: id, Host: w.Host, RepositoryPath: w.RepositoryPath, PrimaryRepositoryKey: w.PrimaryRepositoryKey}
	if w.WorkspaceOrigin != nil {
		input := toWorkspaceOrigin(*w.WorkspaceOrigin)
		r.WorkspaceOrigin = &input
	}
	if len(w.AdditionalRepositories) != 0 {
		r.AdditionalRepositories = make([]application.AdditionalRepositoryInput, len(w.AdditionalRepositories))
		for i, repository := range w.AdditionalRepositories {
			r.AdditionalRepositories[i] = application.AdditionalRepositoryInput{Key: repository.Key, RepositoryPath: repository.RepositoryPath, WorkspaceOrigin: toWorkspaceOrigin(repository.WorkspaceOrigin)}
		}
	}
	if w.NewTask != nil {
		r.NewTask = &application.NewTaskInput{Request: w.NewTask.Request, InitialScope: w.NewTask.InitialScope, InitialOutOfScope: w.NewTask.InitialOutOfScope, KnownAcceptanceCriteria: w.NewTask.KnownAcceptanceCriteria, MethodProfile: w.NewTask.MethodProfile}
	}
	return r
}
func validRepositoryPath(path string) bool {
	return path != "" && len(path) <= domain.MaxRepositoryPathBytes && utf8.ValidString(path)
}
func toProbe(w *operationProbeWire) *application.OperationProbe {
	if w == nil {
		return nil
	}
	return &application.OperationProbe{OperationID: w.OperationID, ProcessID: w.ProcessID, ProcessDefinitionDigest: w.ProcessDefinitionDigest, SourceCursor: w.SourceCursor, ExpectedRevision: w.ExpectedRevision, ActionID: w.ActionID, ActionKind: w.ActionKind, RepositoryBindingDigest: w.RepositoryBindingDigest, IssuanceIdentityDigest: w.IssuanceIdentityDigest, IssuanceHistoryDigest: w.IssuanceHistoryDigest, IssuanceContentDigest: w.IssuanceContentDigest, Payload: w.Payload}
}

func toWorkspaceOrigin(w workspaceOriginWire) application.WorkspaceOriginInput {
	return application.WorkspaceOriginInput{Mode: w.Mode, SourceType: w.SourceType, CarryChanges: *w.CarryChanges, RemoteName: *w.RemoteName, BaseBranch: w.BaseBranch, BaseCommit: w.BaseCommit, TaskBranch: w.TaskBranch, ProvisioningReceiptID: w.ProvisioningReceiptID}
}
func toSubmitAction(w submitActionWire, requestID domain.ID, kind domain.ActionKind) application.SubmitActionRequest {
	current := make([]application.ArtifactSubmission, len(w.Artifacts.Current))
	for index, item := range w.Artifacts.Current {
		current[index] = application.ArtifactSubmission{Path: item.Path, Digest: item.Digest, Summary: item.Summary}
	}
	other := make([]application.ArtifactSubmission, len(w.Artifacts.OtherProcess))
	for index, item := range w.Artifacts.OtherProcess {
		other[index] = application.ArtifactSubmission{Path: item.Path, Digest: item.Digest, Summary: item.Summary}
	}
	methods := make(map[domain.MethodStepID]application.MethodResultSubmission, len(w.MethodResults))
	for step, result := range w.MethodResults {
		methods[step] = application.MethodResultSubmission{Capability: result.Capability, Summary: result.Summary}
	}
	return application.SubmitActionRequest{
		RequestID: requestID, Host: w.Host, TaskID: w.TaskID, ActionID: w.ActionID, ExpectedActionKind: kind,
		TransitionID: w.TransitionID, Summary: w.Summary, Reason: w.Reason, CurrentArtifacts: current,
		OtherProcessArtifacts: other, MethodResults: methods, NodeResult: append(json.RawMessage(nil), w.NodeResult...),
	}
}

// toolRequestMemberViolations uses the published envelope to identify missing or
// unknown top-level fields before any Task lookup or workspace preparation.
func toolRequestMemberViolations(tool string, raw []byte) []domain.ContractViolation {
	for _, definition := range catalog {
		if definition.Name != tool {
			continue
		}
		var schema struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(definition.InputSchema, &schema) != nil {
			return nil
		}
		violations := missingRequestMembers(raw, schema.Required...)
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return violations
		}
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if _, known := schema.Properties[name]; !known {
				if violation := domain.Violation(name, domain.RuleUnknownMember); violation.Path != "" {
					violations = append(violations, violation)
				} else {
					violations = append(violations, domain.Violation("arguments", domain.RuleUnsafeMemberName))
				}
			}
		}
		return violations
	}
	return nil
}

type prepareBranchRenameWire struct {
	Host          domain.Host          `json:"host"`
	TaskID        domain.ID            `json:"task_id"`
	Revision      uint64               `json:"revision"`
	RepositoryKey domain.RepositoryKey `json:"repository_key"`
	TargetBranch  string               `json:"target_branch"`
	Reason        string               `json:"reason"`
}
