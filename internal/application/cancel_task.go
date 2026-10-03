package application

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func (s *Service) CancelTask(ctx context.Context, r CancelTaskRequest) (CancelTaskResult, error) {
	if !s.valid() || ctx == nil || !r.RequestID.IsValid() || !r.Host.IsValid() || !r.TaskID.IsValid() {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The application service, request context or required request identity is invalid.")
	}
	task, err := s.loadOwned(ctx, r.Host, r.TaskID)
	if err != nil {
		return CancelTaskResult{}, err
	}
	if task.CurrentNode.Terminal() {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrTaskTerminal, "This operation requires an active Task, but the Task has already reached DONE or CANCELLED.")
	}
	if task.Revision != r.ExpectedRevision {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "The supplied revision does not match the saved Task revision.")
	}
	if !utf8.ValidString(r.Reason) || strings.TrimSpace(r.Reason) == "" || r.Reason != strings.TrimSpace(r.Reason) || len(r.Reason) > domain.MaxReasonBytes {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "Cancellation reason must be non-empty, trimmed UTF-8 text of at most 4096 bytes.")
	}
	fresh, err := s.observeTaskRepositories(ctx, task)
	if task.BranchRename != nil {
		observed, renameErr := s.observeBranchRenameScope(ctx, task, *task.BranchRename, "cancel")
		if renameErr != nil {
			observed, renameErr = s.observeBranchRenameScope(ctx, task, *task.BranchRename, "complete")
		}
		if renameErr != nil {
			return CancelTaskResult{}, renameErr
		}
		fresh, err = observed.scope, nil
	}
	if err != nil {
		return CancelTaskResult{}, err
	}
	if scopeHasUnavailableWorkspace(task, fresh) {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "An observed worktree is missing or no longer has the instance identity retained by the Task.")
	}
	comparison, err := recovery.CompareRepositoryScope(task, fresh)
	if err != nil {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInternal, "The repository observations could not be compared with the Task repository scope.")
	}
	if task.BranchRename == nil && comparison.Relation == recovery.RepositoryForbiddenChange {
		return CancelTaskResult{}, repositoryDriftError(comparison)
	}
	source := task.CurrentNode
	next, err := cloneProcessTask(task)
	if err != nil {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInternal, "The saved Task could not be decoded into a working copy for this operation.")
	}
	now := s.now().UTC()
	next.BranchRename = nil
	next.CurrentNode = domain.NodeCancelled
	next.CurrentAction = nil
	next.Blocker = nil
	next.ResumeNode = nil
	next.Relocation = nil
	next.Revision++
	next.UpdatedAt = now
	next.CompletedAt = &now
	rebindProcessAuthorities(&next, fresh)
	rev := uint32(0)
	if next.Requirements != nil {
		rev = next.Requirements.Revision
	}
	effectiveDigest, err := next.EffectiveRepositoryBindingDigest()
	if err != nil {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInternal, "The Task repository bindings are invalid and cannot produce a binding digest.")
	}
	next.Outcome = &domain.ProcessOutcome{Status: domain.TerminalCancelled, Summary: r.Reason, RequirementsRevision: rev, FinalRepositoryDigest: effectiveDigest, CompletedAt: now}
	digest, err := digestCanonical(r)
	if err != nil {
		return CancelTaskResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the cancellation request for its operation digest.")
	}
	next.LastOperation = &domain.LastOperation{OperationID: r.RequestID, Kind: domain.OperationCancelTask, FromRevision: r.ExpectedRevision, ToRevision: next.Revision, PayloadDigest: digest, CommittedAt: now}
	eventID, err := s.id("event")
	if err != nil {
		return CancelTaskResult{}, err
	}
	event := store.TaskEvent{EventID: eventID, TaskID: next.TaskID, Revision: next.Revision, Kind: domain.OperationCancelTask, SourceNode: source, DestinationNode: domain.NodeCancelled, RepositoryDeltaPaths: observedTaskDeltaPaths(task, fresh), RequestID: r.RequestID, PayloadDigest: digest, CreatedAt: now}
	if err := s.taskStore.CommitTask(ctx, store.TaskMutation{ExpectedRevision: r.ExpectedRevision, Task: next, Event: event, Claim: store.ClaimRelease}); err != nil {
		return CancelTaskResult{}, mapStoreError(err)
	}
	return CancelTaskResult{Task: next}, nil
}
