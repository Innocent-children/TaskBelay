package protocol

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
)

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
	return Envelope{RequestID: id, Tool: name, Error: &domain.Error{Code: code, Message: "control metering initialization failed; tool was not executed", Details: []domain.ContractViolation{{Path: "control_meter." + phase, Rule: "initialization", Message: err.Error()}}}, Outcome: "not_applied", Recovery: &Recovery{Action: action, Operation: "read_before_retry", Message: message}}
}

func failure(id, name string, e error) Envelope {
	var de *domain.Error
	if !errors.As(e, &de) {
		de = &domain.Error{Code: "INTERNAL_ERROR", Message: "Core could not complete the operation; inspect saved task and run state before retrying."}
	}
	r := Recovery{Action: "read_task", Operation: "read_before_retry", Message: "Read current saved state; do not blindly replay the operation."}
	outcome := "unknown"
	switch de.Code {
	case "LOCAL_SESSION_REQUIRED", "HOST_REJECTED":
		r.Action, r.Operation, outcome = "inspect_host_context", "none", "not_applied"
		r.Message = "Use the exact local service address and reload its current session before another request."
	case "INVALID_ARGUMENT", "ASSESSMENT_STALE", "EVIDENCE_STALE", "HOST_USAGE_REGRESSION", "HOST_USAGE_CONFLICT":
		r.Action = "correct_request"
		r.Operation, outcome = "reuse", "not_applied"
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
	case "COST_LIMIT", "AUTHORIZATION_REQUIRED", "AUTHORIZATION_PENDING":
		r.Action = "request_authorization"
		r.Operation, outcome = "reuse", "not_applied"
		r.Message = "Explain the specific boundary or limit needing a user decision; reuse existing authorization for routine work."
	case "BUDGET_UNCHANGED", "SCOPE_VIOLATION", "REPEATED_FAILURE", "ACCEPTANCE_INCOMPLETE", "CHECK_ALREADY_SATISFIED", "RETRY_BASIS_REQUIRED", "FAILURE_ACCEPTANCE_NOT_ALLOWED":
		r.Action = "self_correct"
		r.Operation, outcome = "reuse", "not_applied"
		r.Message = "Inspect the current facts and gaps, then correct within existing authorization."
	case "REVISION_CONFLICT", "AUTHORIZATION_CHANGED", "ISSUE_CHANGED":
		r.Operation, outcome = "replace", "not_applied"
		r.Message = "Read the returned current state and then use a new operation identity for the corrected request."
	case "HOST_USAGE_UNAVAILABLE":
		r.Action, r.Operation, outcome = "self_correct", "read_before_retry", "not_applied"
		r.Message = "Report actual Host usage for the explicit cap, or record a real budget change. Unknown usage is not zero."
	case "TASK_NOT_FOUND", "RUN_NOT_FOUND", "TASK_TERMINAL", "ARCHIVE_NOT_ALLOWED", "OPERATION_CONFLICT", "WORKSPACE_CLAIMED":
		r.Operation, outcome = "none", "not_applied"
	}
	if de.Code == "CHECK_ALREADY_SATISFIED" {
		r.Operation = "none"
		r.Message = "Reuse error.existing_result; the same passing check must not be executed again."
	}
	if de.Outcome != "" {
		outcome = de.Outcome
	}
	if outcome != "not_applied" {
		r.Operation = "read_before_retry"
	}
	execution := de.Execution
	if name == "check.run" && execution == "" {
		execution = "unknown"
		if outcome == "not_applied" {
			execution = "not_started"
		}
	}
	return Envelope{RequestID: id, Tool: name, Error: de, Recovery: &r, Outcome: outcome, Execution: execution}
}

/**
 * ErrorEnvelope exposes the same response producer to HTTP and CLI preflight
 * boundaries without dispatching a Task operation.
 */
func ErrorEnvelope(name string, err error) Envelope { return failure(newID(), name, err) }
