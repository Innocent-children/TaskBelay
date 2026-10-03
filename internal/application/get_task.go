package application

import (
	"bytes"
	"context"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func (s *Service) GetTask(ctx context.Context, r GetTaskRequest) (GetTaskResult, error) {
	result, err := s.getTask(ctx, r)
	if err != nil || r.BaselineHistory == nil {
		return result, err
	}
	page, err := result.Task.ReadBaselineHistory(*r.BaselineHistory)
	if err != nil {
		return GetTaskResult{}, err
	}
	result.BaselineHistory = &page
	return result, nil
}

func (s *Service) getTask(ctx context.Context, r GetTaskRequest) (GetTaskResult, error) {
	if !s.valid() || ctx == nil || !r.Host.IsValid() || !r.TaskID.IsValid() {
		return GetTaskResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The application service, request context or required request identity is invalid.")
	}
	if r.OperationProbe != nil {
		if err := validateProbeInput(r.OperationProbe); err != nil {
			return GetTaskResult{}, err
		}
		task, err := s.loadOwned(ctx, r.Host, r.TaskID)
		if err != nil {
			return GetTaskResult{}, err
		}
		fresh, err := s.observeOperationScope(ctx, task, r.OperationProbe.Payload)
		if err != nil {
			return GetTaskResult{}, err
		}
		decision, err := recovery.Reconcile(recovery.ReconcileInput{Host: r.Host, Task: task, Operation: r.OperationProbe.Reference(), Payload: r.OperationProbe.Payload, ObservedScope: &fresh})
		if err != nil {
			return GetTaskResult{}, err
		}
		return GetTaskResult{Task: task, RecoveryAssessment: &decision.Assessment}, nil
	}
	task, err := s.loadOwned(ctx, r.Host, r.TaskID)
	if err != nil {
		return GetTaskResult{}, err
	}
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return GetTaskResult{Task: task}, nil
	}
	operation, found, err := operationStore.LoadActionOperation(ctx, task.TaskID)
	if err != nil {
		return GetTaskResult{}, mapStoreError(err)
	}
	if !found || workflow.ValidateActionCommit(task, operation.Commit) != nil {
		if found {
			return GetTaskResult{}, domain.WithExplanation(domain.ErrStorageUnavailable, "The prepared Action operation does not match the Task, payload or operation digest.")
		}
		return GetTaskResult{Task: task}, nil
	}
	if operation.RecordedBy(task) {
		assessment, err := assessRecordedActionCommit(r.Host, task, operation.Commit)
		if err != nil {
			return GetTaskResult{}, err
		}
		return GetTaskResult{Task: task, RecoveryAssessment: assessment}, nil
	}
	if task.CurrentNode == domain.NodeBlocked && task.Blocker != nil &&
		task.Blocker.Cause == domain.BlockerCauseTaskRelocationPending &&
		operation.AppliedRevision == nil && operation.Commit.Operation.SourceCursor == domain.NodeBlocked {
		assessment, err := s.assessTaskRelocationCommit(ctx, r.Host, task, operation.Commit)
		if err != nil {
			return GetTaskResult{}, err
		}
		return GetTaskResult{Task: task, RecoveryAssessment: assessment}, nil
	}
	assessment, err := s.assessActionCommit(ctx, r.Host, task, operation.Commit)
	if err != nil {
		return GetTaskResult{}, err
	}
	return GetTaskResult{Task: task, RecoveryAssessment: assessment}, nil
}

func assessRecordedActionCommit(host domain.Host, task domain.ProcessTask, commit domain.ActionCommit) (*recovery.RecoveryAssessment, error) {
	additional := make([]domain.RepositoryScopeEntry, len(task.AdditionalRepositories))
	for index, repository := range task.AdditionalRepositories {
		additional[index] = repository.Clone()
	}
	observed := recovery.RepositoryScopeObservation{
		Primary:    task.Repository.Clone(),
		Additional: additional,
	}
	decision, err := recovery.Reconcile(recovery.ReconcileInput{
		Host:          host,
		Task:          task,
		Operation:     commit.Operation,
		Payload:       commit.Payload,
		ObservedScope: &observed,
	})
	if err != nil || decision.Assessment.Classification != domain.RecoveryCompletedAndRecorded {
		return nil, domain.WithExplanation(domain.ErrStorageUnavailable, "The saved Action operation is not proven to be the operation already recorded by this Task.")
	}
	return &decision.Assessment, nil
}

func (s *Service) assessActionCommit(ctx context.Context, host domain.Host, task domain.ProcessTask, commit domain.ActionCommit) (*recovery.RecoveryAssessment, error) {
	fresh, err := s.observeOperationScope(ctx, task, commit.Payload)
	if err != nil {
		return nil, err
	}
	decision, err := recovery.Reconcile(recovery.ReconcileInput{Host: host, Task: task, Operation: commit.Operation, Payload: commit.Payload, ObservedScope: &fresh})
	if err != nil {
		return nil, err
	}
	return &decision.Assessment, nil
}
func validateProbeInput(p *OperationProbe) error {
	if p == nil || len(p.Payload) == 0 || workflow.ValidateOperationReference(p.Reference()) != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "operation_probe requires a complete valid operation identity and a payload member.")
	}
	if !bytes.Equal(bytes.TrimSpace(p.Payload), []byte("null")) {
		if p.SourceCursor == domain.NodeBlocked {
			if _, _, err := recovery.DecodeBlockerResolutionPayload(p.Payload); err != nil {
				return domain.WithExplanation(domain.ErrInvalidArgument, "A BLOCKED recovery probe requires the complete saved blocker-resolution payload.")
			}
			return nil
		}
		if err := workflow.ValidateRetainedPayload(p.SourceCursor, p.Payload); err != nil {
			return domain.WithExplanation(domain.ErrInvalidArgument, "The recovery probe payload must satisfy its source node contract.")
		}
	}
	return nil
}

func (s *Service) observeOperationScope(ctx context.Context, task domain.ProcessTask, payload []byte) (recovery.RepositoryScopeObservation, error) {
	if task.BranchRename != nil && !bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		decision, _, err := workflow.DecodeBlockerResolutionPayload(payload)
		if err != nil {
			return recovery.RepositoryScopeObservation{}, err
		}
		observed, err := s.observeBranchRenameScope(ctx, task, *task.BranchRename, decision.RenameChoice)
		return observed.scope, err
	}
	return s.observeTaskRepositories(ctx, task)
}
