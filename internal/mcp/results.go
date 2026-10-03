package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/userconfig"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type Envelope struct {
	OK        bool              `json:"ok"`
	RequestID string            `json:"request_id"`
	Tool      string            `json:"tool"`
	Result    any               `json:"result,omitempty"`
	Error     *ErrorResult      `json:"error,omitempty"`
	Recovery  *RecoveryGuidance `json:"recovery,omitempty"`
}
type ErrorResult struct {
	Code    domain.ErrorCode      `json:"code"`
	Budget  *domain.BudgetFailure `json:"budget,omitempty"`
	Message string                `json:"message"`
	// Details names safe field-level contract failures when available.
	Details []domain.ContractViolation `json:"details,omitempty"`
	// Guard is the closed transition-guard detail.
	Guard           *domain.GuardFailure `json:"guard,omitempty"`
	RepositoryPaths []string             `json:"repository_paths,omitempty"`
}
type RecoveryGuidance struct {
	RetrySafe bool   `json:"retry_safe"`
	Action    string `json:"action"`
	Message   string `json:"message"`
	// AllowedPaths lists the exact members a bounded correction may change. It
	// appears only with a proven zero-write failure.
	AllowedPaths []string `json:"allowed_paths,omitempty"`
}

// Ordinary node submissions retain their Action identity during correction.
// Other tools correct their request envelope without inventing an Action.
const correctCurrentAction = "correct_current_action"
const correctRequest = "correct_request"

type EncodedResult struct {
	JSON    []byte
	IsError bool
}

func EncodeSuccess(id, tool string, result any) EncodedResult {
	value := reflect.ValueOf(result)
	if !value.IsValid() || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Map || value.Kind() == reflect.Slice || value.Kind() == reflect.Interface) && value.IsNil()) {
		return failureFallback(id, tool, "Core produced no result for a successful operation.")
	}
	if !domain.ID(id).IsValid() || !isToolName(tool) {
		return failureFallback(id, tool, "Core could not identify the response request or tool.")
	}
	raw, err := encodeEnvelope(Envelope{OK: true, RequestID: id, Tool: tool, Result: result})
	if err != nil {
		return failureFallback(id, tool, "Core could not encode the operation result as JSON.")
	}
	// Only response pages may shrink. Stored history and every other result field
	// remain unchanged; next_after always identifies the last reference sent.
	for !WithinResultEnvelopeLimit(raw) && shrinkResultHistoryPage(result) {
		raw, err = encodeEnvelope(Envelope{OK: true, RequestID: id, Tool: tool, Result: result})
		if err != nil {
			return failureFallback(id, tool, "Core could not encode the operation result as JSON.")
		}
	}
	if !WithinResultEnvelopeLimit(raw) {
		return failureFallback(id, tool, "The encoded operation result exceeds the Core response byte limit.")
	}
	return EncodedResult{JSON: raw}
}

// Inline Task history can be empty when the envelope needs its space: the
// explicit zero cursor and nonzero total direct the Host to get_task pagination.
// An explicitly requested page always retains at least one complete reference.
func shrinkResultHistoryPage(result any) bool {
	value, ok := result.(map[string]any)
	if !ok {
		return false
	}
	task := value
	if nested, ok := value["task"].(map[string]any); ok {
		task = nested
	}
	if baselines, ok := task["baselines"].(map[string]any); ok {
		if history, ok := baselines["history"].([]domain.BaselineReference); ok && len(history) > 0 {
			history = history[:len(history)-1]
			baselines["history"] = history
			baselines["history_next_after"] = uint64(len(history))
			return true
		}
	}
	if page, ok := value["baseline_history"].(*domain.BaselineHistoryPage); ok && len(page.Entries) > 1 {
		page.Entries = page.Entries[:len(page.Entries)-1]
		after := page.Entries[len(page.Entries)-1].Sequence
		page.NextAfter = &after
		return true
	}
	return false
}

func EncodeError(id, tool string, err error) EncodedResult {
	if !domain.ID(id).IsValid() || !isToolName(tool) {
		return failureFallback(id, tool, "Core could not identify the response request or tool.")
	}
	code := domain.ErrorInternal
	var typed *domain.Error
	if errors.As(err, &typed) && typed != nil && typed.Code.IsValid() {
		code = typed.Code
	}
	message, action, guidance := publicFailure(code)
	if typed != nil && typed.PublicExplanation() != "" {
		message = typed.PublicExplanation()
	} else if errors.Is(err, context.Canceled) {
		message = "The operation was cancelled before Core could return a result."
	} else if errors.Is(err, context.DeadlineExceeded) {
		message = "The operation deadline expired before Core could return a result."
	} else if code == domain.ErrorInternal {
		message = "Core encountered an unclassified internal failure; no safe underlying diagnostic is available."
	}
	if code == domain.ErrorRepositoryDrift && typed != nil && validRepositoryDriftMessage(typed.Message) {
		message = typed.Message
	}
	result := &ErrorResult{Code: code, Message: message}
	if code == domain.ErrorVerificationBudgetExceeded && typed != nil && typed.Budget != nil && typed.Budget.Used >= 0 && typed.Budget.Requested >= 0 && typed.Budget.Limit >= 0 {
		result.Budget = typed.Budget
	}
	guard := publicGuardFailure(code, typed)
	if guard != nil {
		guard.Failures = projectSubmissionViolationPaths(tool, guard.Failures)
		result.Guard = guard
		message = "The transition guard was not satisfied."
		result.Message = message
	}
	result.Details = projectSubmissionViolationPaths(tool, publicViolations(code, typed))
	if len(result.Details) != 0 {
		result.Message = violationSummary(result.Details)
	} else if result.Guard != nil {
		result.Message = violationSummary(result.Guard.Failures)
	}
	result.RepositoryPaths = domain.ViolationRepositoryPaths(err)
	recoveryResult := &RecoveryGuidance{RetrySafe: false, Action: action, Message: guidance}
	if paths := boundedCorrectionPaths(tool, typed); len(paths) != 0 {
		recoveryResult = &RecoveryGuidance{RetrySafe: true, Action: correctCurrentAction, Message: boundedCorrectionMessage, AllowedPaths: paths}
	} else if paths := requestCorrectionPaths(tool, typed, result); len(paths) != 0 {
		recoveryResult = &RecoveryGuidance{RetrySafe: true, Action: correctRequest, Message: requestCorrectionMessage, AllowedPaths: paths}
	}
	raw, encodeErr := encodeEnvelope(Envelope{OK: false, RequestID: id, Tool: tool, Error: result, Recovery: recoveryResult})
	if encodeErr != nil {
		return failureFallback(id, tool, "Core could not encode the error response as JSON.")
	}
	if !WithinResultEnvelopeLimit(raw) {
		return failureFallback(id, tool, "The encoded error details exceed the Core response byte limit.")
	}
	return EncodedResult{JSON: raw, IsError: true}
}

func violationSummary(details []domain.ContractViolation) string {
	var parts []string
	size := 0
	for _, detail := range details {
		part := detail.Path + ": " + detail.Message
		if size+len(part) > domain.MaxErrorMessageBytes-64 {
			parts = append(parts, "See details for the remaining failure conditions.")
			break
		}
		parts = append(parts, part)
		size += len(part) + 2
	}
	return strings.Join(parts, "; ")
}

func projectSubmissionViolationPaths(tool string, violations []domain.ContractViolation) []domain.ContractViolation {
	if _, ok := submissionKindForTool(tool); !ok {
		return violations
	}
	out := make([]domain.ContractViolation, len(violations))
	for index, violation := range violations {
		out[index] = violation
		out[index].Path = strings.TrimPrefix(violation.Path, "payload.")
	}
	return out
}

// publicViolations keeps only closed, safe field detail for a contract failure.
func publicViolations(code domain.ErrorCode, typed *domain.Error) []domain.ContractViolation {
	if (code != domain.ErrorInvalidArgument && code != domain.ErrorVerificationBudgetExceeded && code != domain.ErrorVerificationNotAllowed && code != domain.ErrorWorktreeProvisioningRequired) || typed == nil {
		return nil
	}
	return retainedViolations(typed.Violations)
}

// publicGuardFailure keeps only a closed guard detail whose identity comes from
// the current Process Definition. Repository drift, member format failures and
// unknown work items never reach this shape.
func publicGuardFailure(code domain.ErrorCode, typed *domain.Error) *domain.GuardFailure {
	if code != domain.ErrorTransitionNotAllowed || typed == nil || typed.Guard == nil {
		return nil
	}
	if !workflow.KnownTransitionGuard(typed.Guard.GuardID) {
		return nil
	}
	failures := retainedViolations(typed.Guard.Failures)
	if len(failures) == 0 {
		return nil
	}
	return &domain.GuardFailure{GuardID: typed.Guard.GuardID, Failures: failures}
}
func retainedViolations(violations []domain.ContractViolation) []domain.ContractViolation {
	out := make([]domain.ContractViolation, 0, len(violations))
	for _, violation := range violations {
		if !violation.Rule.IsValid() && !domain.GuardRule(violation.Rule).IsValid() {
			continue
		}
		if !domain.ValidViolationPath(violation.Path) || violation.Message == "" {
			continue
		}
		violation.Message = violation.PublicMessage()
		out = append(out, violation)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// boundedCorrectionMessage states the closed boundary of one correction: only
// the listed members change, the values come from facts the current Action work
// already established, and a second failed submission stops the attempt.
const boundedCorrectionMessage = "Correct only the members listed in allowed_paths, using facts already confirmed in the current Action work, and resubmit through the same submission tool once. Do not re-expand requirements, change more code, or guess a user decision; stop when the resubmission fails."

func boundedCorrectionPaths(tool string, failure *domain.Error) []string {
	if _, ordinary := submissionKindForTool(tool); !ordinary {
		return nil
	}
	paths := recovery.ActionCorrectionPaths(failure)
	for index, path := range paths {
		paths[index] = strings.TrimPrefix(path, "payload.")
	}
	return paths
}

func validRepositoryDriftMessage(message string) bool {
	const prefix = `Repository "`
	const middle = `" has repository drift: `
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, ".") {
		return false
	}
	remainder := strings.TrimSuffix(strings.TrimPrefix(message, prefix), ".")
	parts := strings.Split(remainder, middle)
	if len(parts) != 2 {
		return false
	}
	return domain.RepositoryKey(parts[0]).IsValid() && recovery.RepositoryReason(parts[1]).IsValid()
}
func failureFallback(id, tool, explanation string) EncodedResult {
	if !domain.ID(id).IsValid() {
		id = "request-unavailable"
	}
	if !isToolName(tool) {
		tool = ToolServerInfo
	}
	raw := mustEncode(Envelope{OK: false, RequestID: id, Tool: tool,
		Error:    &ErrorResult{Code: domain.ErrorInternal, Message: explanation},
		Recovery: &RecoveryGuidance{RetrySafe: false, Action: "report_internal_error", Message: "Report this failure. If a mutation was attempted, read its saved operation before deciding whether it may be retried."}})
	return EncodedResult{JSON: raw, IsError: true}
}
func encodeEnvelope(v Envelope) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}
func mustEncode(v Envelope) []byte {
	raw, err := encodeEnvelope(v)
	if err != nil || len(raw) > domain.MaxResultEnvelopeBytes {
		panic("invalid fixed result envelope")
	}
	return raw
}
func publicFailure(code domain.ErrorCode) (string, string, string) {
	m := map[domain.ErrorCode][3]string{
		domain.ErrorInvalidArgument:              {"The request does not match the closed Core contract.", "none", "Inspect the reported fields and current schema. This response does not authorize automatic resubmission."},
		domain.ErrorNotGitRepository:             {"The requested path is not a Git repository.", "none", "Choose a valid local Git repository."},
		domain.ErrorTaskNotFound:                 {"The task was not found.", "read_task", "Confirm the retained Task identity and connected Core instance before reading; do not repeat the same missing lookup unchanged."},
		domain.ErrorActiveTaskConflict:           {"The worktree instance already has an active task.", "read_task", "Resume the active task or choose another provisioned worktree."},
		domain.ErrorHostOwnershipConflict:        {"The task belongs to another host.", "use_origin_host", "Resume the task from its origin host."},
		domain.ErrorRevisionConflict:             {"The submitted task revision is stale.", "read_task", "Read the authoritative task before another mutation."},
		domain.ErrorActionStale:                  {"The submitted action identity is stale.", "read_next_action", "Read and use the exact persisted next action."},
		domain.ErrorRepositoryDrift:              {"The repository observation is not permitted for this operation.", "read_next_action", "Read the current workspace blocker or action."},
		domain.ErrorWorkspaceUnavailable:         {"The retained Task worktree instance is unavailable.", "restore_or_abandon", "Restore the original worktree instance or explicitly abandon the Task."},
		domain.ErrorWorkspaceObservationUnstable: {"The Task repository scope changed during observation.", "retry_read", "Wait for repository activity to settle, then read the Task again."},
		domain.ErrorWorkspaceHistoryConflict:     {"The Task worktree history conflicts with its retained state.", "resolve_blocker", "Restore the retained history or resolve the prepared history decision."},
		domain.ErrorWorktreeProvisioningRequired: {"A confirmed workspace origin is required before opening a Task.", "provision_worktree", "Read the confirmed workspace preparation receipt and submit its exact origin; prepare the workspace only if preparation has not completed."},
		domain.ErrorTransitionNotAllowed:         {"The transition is not allowed from the current node.", "read_next_action", "Read the complete current transition set."},
		domain.ErrorProcessUnsupported:           {"The process definition is unsupported.", "repair_storage", "Use storage created by this graph Core."},
		domain.ErrorRecoveryUnavailable:          {"Recovery is unavailable for this operation.", "none", "Do not automatically retry; use only a supported graph recovery route."},
		domain.ErrorVerificationNotAllowed:       {"The requested verification is not allowed by the current plan.", "read_next_action", "Read the current TEST Action and adjust only the specifically required permission with a check explanation."},
		domain.ErrorVerificationBudgetExceeded:   {"The submitted evidence exceeds the current verification budget.", "read_next_action", "Return to the current TEST Action and submit only a specifically justified verification budget increase before running more automatic checks."},
		domain.ErrorTaskBlocked:                  {"The task is blocked.", "read_next_action", "Read the blocker-resolution action."},
		domain.ErrorTaskTerminal:                 {"The task is terminal.", "read_task", "Read the retained terminal outcome."},
		domain.ErrorSchemaUnsupported:            {"The storage schema is unsupported.", "none", "Stop this operation."},
		domain.ErrorStorageUnavailable:           {"Core storage is unavailable.", "repair_storage", "Restore storage availability before continuing."},
		domain.ErrorInternal:                     {"The Core could not complete the operation.", "report_internal_error", "Report the bounded failure and stop this operation."},
	}
	v, ok := m[code]
	if !ok {
		v = m[domain.ErrorInternal]
	}
	return v[0], v[1], v[2]
}

type ServerInfoResult struct {
	Product            string                   `json:"product"`
	Version            string                   `json:"version"`
	Transport          string                   `json:"transport"`
	Health             string                   `json:"health"`
	SupportedHosts     []string                 `json:"supported_hosts"`
	SupportedProcesses []SupportedProcessResult `json:"supported_processes"`
	MethodProfiles     []domain.MethodProfile   `json:"method_profiles"`
	Tools              []string                 `json:"tools"`
	HostPreferences    userconfig.Preferences   `json:"host_preferences"`
}
type SupportedProcessResult struct {
	ProcessID        domain.ProcessID `json:"process_id"`
	DefinitionDigest domain.Digest    `json:"definition_digest"`
	NewTaskSupported bool             `json:"new_task_supported"`
}

func projectAction(a *domain.ProcessAction) any {
	if a == nil {
		return nil
	}
	tool, _ := submissionToolForActionKind(a.Kind)
	return map[string]any{"task_id": a.TaskID, "revision": a.Revision, "action_id": a.ActionID, "action_kind": a.Kind, "submission_tool": tool, "process_id": a.Process.ID, "process_definition_digest": a.Process.DefinitionDigest, "current_node": a.NodeID, "node_purpose": a.NodeContract.Purpose, "entry_conditions": a.NodeContract.EntryConditions, "completion_conditions": a.NodeContract.CompletionConditions, "allowed_effects": a.AllowedEffects, "required_evidence": a.RequiredEvidence, "method_profile": a.MethodProfile, "method_steps": a.SemanticMethodSteps, "available_transitions": a.AvailableTransitions, "payload_contract": a.PayloadContract, "guidance": a.Guidance, "repository_binding_digest": a.RepositoryBindingDigest, "issuance_identity_digest": a.IssuanceIdentityDigest, "issuance_history_digest": a.IssuanceHistoryDigest, "issuance_content_digest": a.IssuanceContentDigest, "issued_at": a.IssuedAt}
}
func projectTask(t domain.ProcessTask) any {
	// Core has already validated every reference. This fixed valid first-page
	// query cannot fail for a saved Task; the same pager serves explicit reads.
	page, _ := t.ReadBaselineHistory(domain.BaselineHistoryQuery{Revision: t.Revision, Limit: domain.MaxBaselineHistoryPageEntries})
	var history []domain.BaselineReference
	for _, entry := range page.Entries {
		history = append(history, entry.Reference)
	}

	var plan any
	var currentBudget any
	if t.TaskPlan != nil {
		plan = t.TaskPlan.VerificationPlan
		if budget, ok := t.CurrentVerificationBudget(); ok {
			currentBudget = budget
		}
	}
	verification := map[string]any{"plan": plan, "current_budget": currentBudget, "usage": t.CurrentVerificationUsage(), "adjustments": t.VerificationBudgetAdjustments}
	result := map[string]any{"task_id": t.TaskID, "origin_host": t.OriginHost, "process_id": t.Process.ID, "process_definition_digest": t.Process.DefinitionDigest, "intent": t.Intent, "current_cursor": t.CurrentNode, "resume_cursor": t.ResumeNode, "primary_repository_key": t.EffectivePrimaryRepositoryKey(), "workspace_origin": t.WorkspaceOrigin, "repository": projectRepository(t.Repository), "baselines": map[string]any{"requirements": t.Requirements, "design": t.Design, "task_plan": t.TaskPlan, "history": history, "history_total": page.Total, "history_next_after": page.NextAfter, "history_revision": page.Revision}, "implementation": t.Implementation, "test": t.Test, "comprehension": t.Comprehension, "verification": verification, "verification_attempts": t.VerificationAttempts, "file_scope_records": t.FileScopeRecords, "current_changed_paths": t.CurrentChangedPaths, "relocation": t.Relocation, "current_action": projectAction(t.CurrentAction), "blocker": t.Blocker, "last_operation": t.LastOperation, "evidence": t.Evidence, "outcome": t.Outcome, "revision": t.Revision, "created_at": t.CreatedAt, "updated_at": t.UpdatedAt, "completed_at": t.CompletedAt}
	if t.BranchRename != nil {
		result["branch_rename"] = t.BranchRename
	}
	if len(t.AdditionalRepositories) != 0 {
		entries := append([]domain.RepositoryScopeEntry(nil), t.AdditionalRepositories...)
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		additional := make([]map[string]any, len(entries))
		for i, entry := range entries {
			additional[i] = map[string]any{"key": entry.Key, "workspace_origin": entry.Origin, "repository": projectRepository(entry.Binding)}
		}
		result["additional_repositories"] = additional
	}
	return result
}
func projectRepository(repository domain.RepositoryBinding) map[string]any {
	return map[string]any{"worktree_instance_digest": repository.WorktreeInstanceDigest, "identity_digest": repository.IdentityDigest, "history_digest": repository.HistoryDigest, "content_digest": repository.ContentDigest, "current_branch": repository.CurrentBranch, "detached": repository.Detached, "current_head": repository.CurrentHead, "head_tree": repository.HeadTree, "history_relation": repository.HistoryRelation, "base_commit_ancestor": repository.BaseCommitAncestor, "changed_entries": repository.ChangedEntries, "task_surface": repository.TaskSurface, "observed_at": repository.ObservedAt, "binding_digest": repository.BindingDigest}
}
func projectNextAction(result application.NextActionResult) any {
	return map[string]any{"task_id": result.TaskID, "process": result.Process, "current_cursor": result.CurrentNode, "revision": result.Revision, "method_profile": result.MethodProfile, "blocker": result.Blocker, "action": projectAction(result.Action), "outcome": result.Outcome, "recovery_assessment": projectRecoveryAssessment(result.RecoveryAssessment)}
}

func projectRecoveryAssessment(assessment *recovery.RecoveryAssessment) any {
	if assessment == nil {
		return nil
	}
	operation := assessment.Operation
	result := map[string]any{
		"classification":               assessment.Classification,
		"operation":                    map[string]any{"operation_id": operation.OperationID, "process_id": operation.Process.ID, "process_definition_digest": operation.Process.DefinitionDigest, "source_cursor": operation.SourceCursor, "expected_revision": operation.ExpectedRevision, "action_id": operation.ActionID, "action_kind": operation.ActionKind, "repository_binding_digest": operation.RepositoryBindingDigest, "issuance_identity_digest": operation.IssuanceIdentityDigest, "issuance_history_digest": operation.IssuanceHistoryDigest, "issuance_content_digest": operation.IssuanceContentDigest},
		"task_revision":                assessment.TaskRevision,
		"current_action_id":            assessment.CurrentActionID,
		"issuance_binding_digest":      assessment.IssuanceBindingDigest,
		"authoritative_binding_digest": assessment.AuthoritativeBindingDigest,
		"observed_binding_digest":      assessment.ObservedBindingDigest,
		"repository_relation":          assessment.RepositoryRelation,
		"last_operation_relation":      assessment.LastOperationRelation,
		"operation_evidence":           assessment.OperationEvidence,
		"operation_payload_digest":     assessment.OperationPayloadDigest,
		"committed_proof":              assessment.CommittedProof,
		"action_retry_safe":            assessment.ActionRetrySafe,
		"next_advice":                  assessment.NextAdvice,
		"unblock_condition":            assessment.UnblockCondition,
		"observed_at":                  assessment.ObservedAt,
	}
	if len(assessment.Repositories) != 0 {
		repositories := append([]recovery.RepositoryFact(nil), assessment.Repositories...)
		sort.Slice(repositories, func(i, j int) bool { return repositories[i].RepositoryKey < repositories[j].RepositoryKey })
		projected := make([]map[string]any, len(repositories))
		for i, repository := range repositories {
			projected[i] = map[string]any{"key": repository.RepositoryKey, "relation": repository.Relation, "reason": repository.Reason}
		}
		result["repositories"] = projected
	}
	return result
}

func WithinResultEnvelopeLimit(raw []byte) bool { return len(raw) <= domain.MaxResultEnvelopeBytes }

const requestCorrectionMessage = "Correct only allowed_paths using established values and user decisions. Keep the same tool and existing request identity fields unless listed. Do not add the response request_id to tools that do not accept it. Submit once; ask only for missing facts or decisions, and stop if the corrected request fails."

// requestCorrectionPaths handles envelope mistakes before an ordinary node Action
// exists. Only fields belonging to this tool's request can authorize correction.
func requestCorrectionPaths(tool string, typed *domain.Error, result *ErrorResult) []string {
	if typed == nil || !typed.ZeroWrite || typed.Code != domain.ErrorInvalidArgument {
		return nil
	}
	if _, ordinary := submissionKindForTool(tool); ordinary {
		return nil
	}
	var properties map[string]json.RawMessage
	for _, definition := range catalog {
		if definition.Name == tool {
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
			}
			if json.Unmarshal(definition.InputSchema, &schema) != nil {
				return nil
			}
			properties = schema.Properties
		}
	}
	if properties == nil || len(result.Details) == 0 || len(result.Details) != len(typed.Violations) {
		return nil
	}
	paths := make([]string, 0, len(result.Details))
	for _, detail := range result.Details {
		if strings.ContainsAny(detail.Path, ".[") {
			return nil
		}
		_, declared := properties[detail.Path]
		switch detail.Rule {
		case domain.RuleUnknownMember:
			if declared {
				return nil
			}
		case domain.RuleRequiredMemberMissing, domain.RuleTextNotNormalized, domain.RuleRequiredCollectionNonEmpty, domain.RuleCreationMemberOnResume:
			if !declared {
				return nil
			}
		default:
			return nil
		}
		paths = append(paths, detail.Path)
	}
	return paths
}
