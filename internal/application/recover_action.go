package application

import (
	"context"
	"encoding/json"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func (s *Service) RecoverAction(ctx context.Context, request RecoverActionRequest) (ApplyActionResult, error) {
	if !s.valid() || ctx == nil || !request.Host.IsValid() || !request.TaskID.IsValid() || !request.ActionID.IsValid() {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The application service, request context or required request identity is invalid.")
	}
	task, err := s.loadOwned(ctx, request.Host, request.TaskID)
	if err != nil {
		return ApplyActionResult{}, err
	}
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The connected Task store does not support retaining and recovering Action operations.")
	}
	stored, found, err := operationStore.LoadActionOperation(ctx, task.TaskID)
	if err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	if !found || stored.Commit.Operation.ActionID != request.ActionID || workflow.ValidateActionCommit(task, stored.Commit) != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrRecoveryUnavailable, "The prepared Action operation does not match the Task, payload or operation digest.")
	}
	if stored.RecordedBy(task) {
		return ApplyActionResult{Task: task}, nil
	}
	commit := stored.Commit
	if task.BranchRename != nil && commit.Operation.SourceCursor == domain.NodeBlocked {
		payload, canonical, err := workflow.DecodeBlockerResolutionPayload(commit.Payload)
		if err != nil {
			return ApplyActionResult{}, err
		}
		return s.resolveTaskBranchRename(ctx, RecoverActionRequest{Host: request.Host, TaskID: task.TaskID, ActionID: request.ActionID, RenameID: payload.RenameID, RenameChoice: payload.RenameChoice}, commit.Operation.OperationID, task, canonical)
	}
	if task.CurrentNode == domain.NodeBlocked && task.Blocker != nil &&
		task.Blocker.Cause == domain.BlockerCauseTaskRelocationPending &&
		commit.Operation.SourceCursor == domain.NodeBlocked {
		payload, canonical, decodeErr := workflow.DecodeBlockerResolutionPayload(commit.Payload)
		if decodeErr != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The blocker-resolution payload does not match the saved blocker contract.")
		}
		apply := applyRequestFromCommit(request.Host, task.TaskID, commit)
		return s.resolveTaskRelocationPayload(ctx, apply, task, payload, canonical)
	}
	fresh, err := s.observeTaskRepositories(ctx, task)
	if err != nil {
		return ApplyActionResult{}, err
	}
	decision, err := recovery.Reconcile(recovery.ReconcileInput{Host: request.Host, Task: task, Operation: commit.Operation, Payload: commit.Payload, ObservedScope: &fresh})
	if err != nil {
		return ApplyActionResult{}, err
	}
	apply := applyRequestFromCommit(request.Host, task.TaskID, commit)
	switch decision.Directive {
	case recovery.DirectiveNoWrite:
		if decision.Assessment.Classification != domain.RecoveryNotStarted {
			return ApplyActionResult{Task: task}, nil
		}
		comparison, comparisonErr := recovery.CompareRepositoryScope(task, fresh)
		if comparisonErr != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The repository observations could not be compared with the Task repository scope.")
		}
		return s.commitStandardActionOperation(ctx, operationStore, apply, task, fresh, comparison)
	case recovery.DirectiveReturnExistingBlocker:
		return ApplyActionResult{Task: task}, nil
	case recovery.DirectiveCommitRecoveredTransition:
		if commit.Operation.SourceCursor == domain.NodeBlocked {
			payload, canonical, decodeErr := workflow.DecodeBlockerResolutionPayload(commit.Payload)
			if decodeErr != nil {
				return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The blocker-resolution payload does not match the saved blocker contract.")
			}
			mutation, planErr := s.planResolveBlockerMutation(apply, task, fresh, payload, canonical)
			if planErr != nil {
				return ApplyActionResult{}, planErr
			}
			if commitErr := operationStore.CommitActionOperation(ctx, apply.RequestID, mutation); commitErr != nil {
				return ApplyActionResult{}, mapStoreError(commitErr)
			}
			return ApplyActionResult{Task: mutation.Task}, nil
		}
		comparison, comparisonErr := recovery.CompareRepositoryScope(task, fresh)
		if comparisonErr != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The repository observations could not be compared with the Task repository scope.")
		}
		return s.commitStandardActionOperation(ctx, operationStore, apply, task, fresh, comparison)
	case recovery.DirectiveCreateBlocker:
		mutation, planErr := s.planRecoveryBlocker(apply, task, fresh, decision)
		if planErr != nil {
			return ApplyActionResult{}, planErr
		}
		if commitErr := operationStore.CommitActionOperation(ctx, apply.RequestID, mutation); commitErr != nil {
			return ApplyActionResult{}, mapStoreError(commitErr)
		}
		return ApplyActionResult{Task: mutation.Task}, nil
	case recovery.DirectiveRevisionConflict:
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "The saved operation was prepared for a different Task revision.")
	case recovery.DirectiveActionStale:
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrActionStale, "The saved operation no longer refers to the current Action.")
	default:
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "Recovery returned a directive that this Core cannot execute.")
	}
}

func (s *Service) ResolveBlockerAction(ctx context.Context, request RecoverActionRequest, requestID domain.ID) (ApplyActionResult, error) {
	if !s.valid() || ctx == nil || !requestID.IsValid() || !request.Host.IsValid() || !request.TaskID.IsValid() || !request.ActionID.IsValid() {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The application service, request context or required request identity is invalid.")
	}
	task, err := s.loadOwned(ctx, request.Host, request.TaskID)
	if err != nil {
		return ApplyActionResult{}, err
	}
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The connected Task store does not support retaining and recovering Action operations.")
	}
	existing, found, err := operationStore.LoadActionOperation(ctx, task.TaskID)
	if err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	if found && existing.Commit.Operation.ActionID == request.ActionID {
		if existing.RecordedBy(task) {
			return ApplyActionResult{Task: task}, nil
		}
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrRecoveryUnavailable, "This blocker Action already has an unrecorded operation; recover the saved operation before submitting another decision.")
	}
	if request.ExpectedRevision != 0 && task.Revision != request.ExpectedRevision {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "The supplied revision does not match the saved Task revision.")
	}
	if task.CurrentNode != domain.NodeBlocked || task.CurrentAction == nil || task.CurrentAction.ActionID != request.ActionID || task.Blocker == nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrActionStale, "The requested Action is not the current blocker-resolution Action, or the Task has no active blocker.")
	}
	if task.BranchRename != nil {
		return s.resolveTaskBranchRename(ctx, request, requestID, task, nil)
	}
	if request.RenameID != "" || request.RenameChoice != "" {
		return ApplyActionResult{}, domain.ErrInvalidArgument
	}
	if task.Blocker.Cause == domain.BlockerCauseTaskRelocationPending {
		if request.RelocationID == "" || request.FileScopeDecision != nil || request.HistoryResolution != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "A relocation blocker requires relocation_id and destinations, and cannot include file-scope or history decisions.")
		}
		return s.resolveTaskRelocation(ctx, request, requestID, task, "", nil)
	}
	fileScopeBlocker := task.Blocker.Cause == domain.BlockerCauseFileScopeDecision
	if fileScopeBlocker {
		if request.FileScopeDecision == nil || request.FileScopeDecision.Validate() != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "A file-scope blocker requires choice (allow_once, expand_scope or reject) and a non-empty normalized reason.")
		}
	} else if request.FileScopeDecision != nil || request.RelocationID != "" || len(request.RelocationDestinations) != 0 {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The current blocker does not accept file-scope decisions or relocation inputs.")
	}
	if task.Blocker.Cause == domain.BlockerCauseWorkspaceHistoryConflict {
		if request.HistoryResolution == nil || request.HistoryResolution.Validate() != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "A history blocker requires history_resolution with choice accept_current_history and a non-empty normalized reason.")
		}
	} else if request.HistoryResolution != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "history_resolution is only accepted for a workspace-history blocker.")
	}
	fresh, err := s.observeTaskRepositories(ctx, task)
	if err != nil {
		return ApplyActionResult{}, err
	}
	comparison, err := recovery.CompareRepositoryScope(task, fresh)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The repository observations could not be compared with the Task repository scope.")
	}
	payload := domain.BlockerResolutionPayload{BlockerID: task.Blocker.BlockerID, Condition: task.Blocker.Condition, ObservedBindingDigest: comparison.ObservedDigest, FileScopeDecision: request.FileScopeDecision, HistoryResolution: request.HistoryResolution}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the prepared operation payload as JSON.")
	}
	_, canonical, err := workflow.DecodeBlockerResolutionPayload(raw)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The blocker-resolution payload does not match the saved blocker contract.")
	}
	apply := applyRequestForCurrentAction(requestID, request.Host, task, canonical)
	operation := operationFromApply(apply)
	digest, err := workflow.GraphOperationDigest(request.Host, task.TaskID, operation, canonical)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The Action identity or canonical payload could not be encoded into an operation digest.")
	}
	mutation, err := s.planResolveBlockerMutation(apply, task, fresh, payload, canonical)
	if err != nil {
		return ApplyActionResult{}, err
	}
	commit := domain.ActionCommit{Operation: operation, Payload: canonical, PayloadDigest: digest, PreparedAt: mutation.Task.LastOperation.CommittedAt}
	if workflow.ValidateActionCommit(task, commit) != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The prepared Action operation does not match the Task, payload or operation digest.")
	}
	if err := operationStore.StageActionOperation(ctx, task, commit); err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	if err := operationStore.CommitActionOperation(ctx, operation.OperationID, mutation); err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	return ApplyActionResult{Task: mutation.Task}, nil
}

func applyRequestFromCommit(host domain.Host, taskID domain.ID, commit domain.ActionCommit) ApplyActionRequest {
	operation := commit.Operation
	return ApplyActionRequest{
		RequestID: operation.OperationID, Host: host, TaskID: taskID, ExpectedRevision: operation.ExpectedRevision,
		ActionID: operation.ActionID, ActionKind: operation.ActionKind, ProcessID: operation.Process.ID,
		ProcessDefinitionDigest: operation.Process.DefinitionDigest, SourceCursor: operation.SourceCursor,
		RepositoryBindingDigest: operation.RepositoryBindingDigest, IssuanceIdentityDigest: operation.IssuanceIdentityDigest,
		IssuanceHistoryDigest: operation.IssuanceHistoryDigest, IssuanceContentDigest: operation.IssuanceContentDigest, Payload: commit.Payload,
	}
}

func (s *Service) commitStandardActionOperation(ctx context.Context, operationStore store.ActionOperationStore, apply ApplyActionRequest, task domain.ProcessTask, fresh recovery.RepositoryScopeObservation, comparison recovery.RepositoryScopeComparison) (ApplyActionResult, error) {
	mutation, err := s.planStandardMutation(apply, task, fresh, comparison)
	if err != nil {
		return ApplyActionResult{}, err
	}
	if err := operationStore.CommitActionOperation(ctx, apply.RequestID, mutation); err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	return ApplyActionResult{Task: mutation.Task}, nil
}
