package application

import (
	"context"
	"errors"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func originSelection(input WorkspaceOriginInput) repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: input.Mode, SourceType: input.SourceType, CarryChanges: input.CarryChanges, RemoteName: input.RemoteName, BaseBranch: input.BaseBranch, BaseCommit: input.BaseCommit, TaskBranch: input.TaskBranch, ProvisioningReceiptID: input.ProvisioningReceiptID}
}

func persistedOriginSelection(origin domain.WorkspaceOrigin) repository.WorkspaceOriginSelection {
	return repository.WorkspaceOriginSelection{Mode: origin.Mode, SourceType: origin.SourceType, CarryChanges: origin.CarryChanges, RemoteName: origin.RemoteName, BaseBranch: origin.BaseBranch, BaseCommit: origin.BaseCommit, TaskBranch: origin.TaskBranch, ProvisioningReceiptID: origin.ProvisioningReceiptID}
}

func mapWorkspaceOpenError(err error) error {
	if errors.Is(err, repository.ErrInconsistentWorktree) {
		return domain.WithExplanation(domain.ErrWorkspaceObservationUnstable, repositoryObservationExplanation(err))
	}
	if errors.Is(err, repository.ErrProvisioningRequired) || errors.Is(err, repository.ErrNotGitRepository) || errors.Is(err, repository.ErrInvalidRepositoryPath) {
		return domain.WithExplanation(domain.ErrWorktreeProvisioningRequired, repositoryObservationExplanation(err))
	}
	return domain.WithExplanation(domain.ErrInternal, repositoryObservationExplanation(err))
}

func mapWorkspaceObservationError(err error) error {
	if errors.Is(err, repository.ErrInconsistentWorktree) {
		return domain.WithExplanation(domain.ErrWorkspaceObservationUnstable, repositoryObservationExplanation(err))
	}
	if errors.Is(err, repository.ErrNotGitRepository) || errors.Is(err, repository.ErrInvalidRepositoryPath) {
		return domain.WithExplanation(domain.ErrWorkspaceUnavailable, repositoryObservationExplanation(err))
	}
	return domain.WithExplanation(domain.ErrInternal, repositoryObservationExplanation(err))
}

func repositoryObservationExplanation(err error) string {
	switch {
	case errors.Is(err, repository.ErrInconsistentWorktree):
		return "Git status or file identity changed during repository observation."
	case errors.Is(err, repository.ErrNotGitRepository):
		return "Git could not identify a worktree at the requested repository path."
	case errors.Is(err, repository.ErrInvalidRepositoryPath):
		return "The repository path cannot identify an accessible, bounded Git worktree."
	case errors.Is(err, repository.ErrProvisioningRequired):
		return "The observed workspace does not match its confirmed mode, source, branch, commit or carried-change selection."
	case errors.Is(err, repository.ErrDirtySubmodule):
		return "A dirty submodule cannot be represented by the supported ordinary-file content fingerprint."
	case errors.Is(err, repository.ErrFingerprintPathLimit):
		return "The repository observation exceeds the maximum number of fingerprinted paths."
	case errors.Is(err, repository.ErrGitOutputLimit):
		return "A Git observation command exceeded the output byte limit."
	case errors.Is(err, repository.ErrGitCommandTimeout), errors.Is(err, context.DeadlineExceeded):
		return "A Git observation command exceeded its deadline."
	case errors.Is(err, context.Canceled):
		return "Repository observation was cancelled."
	default:
		return "Git could not complete repository observation; no more specific safe diagnostic is available."
	}
}

func currentRepositoryScopePaths(primaryKey domain.RepositoryKey, primary domain.RepositoryBinding, additional []domain.RepositoryScopeEntry) []string {
	return domain.RepositoryScopeTaskSurfacePaths(primaryKey, primary, additional)
}

func (s *Service) observeTaskRepositories(ctx context.Context, task domain.ProcessTask) (recovery.RepositoryScopeObservation, error) {
	observer, contextual := s.repositoryObserver.(repository.WorkspaceRepositoryObserver)
	observe := func(path string, origin domain.WorkspaceOrigin, previous domain.RepositoryBinding) (domain.RepositoryBinding, error) {
		if contextual {
			observedOrigin, binding, err := observer.ObserveWorkspace(ctx, path, persistedOriginSelection(origin), &previous)
			if err != nil {
				return domain.RepositoryBinding{}, mapWorkspaceObservationError(err)
			}
			if observedOrigin != origin {
				return domain.RepositoryBinding{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "The observed workspace origin differs from the origin retained by the Task.")
			}
			return binding, nil
		}
		binding, err := s.repositoryObserver.Observe(ctx, path)
		if err != nil {
			return domain.RepositoryBinding{}, mapWorkspaceObservationError(err)
		}
		return binding, nil
	}
	observeScope := func() (recovery.RepositoryScopeObservation, error) {
		primary, err := observe(task.WorkspaceOrigin.CanonicalWorktreeRoot, task.WorkspaceOrigin, task.Repository)
		if err != nil {
			return recovery.RepositoryScopeObservation{}, err
		}
		additional := make([]domain.RepositoryScopeEntry, len(task.AdditionalRepositories))
		for i, entry := range task.AdditionalRepositories {
			binding, observeErr := observe(entry.Origin.CanonicalWorktreeRoot, entry.Origin, entry.Binding)
			if observeErr != nil {
				return recovery.RepositoryScopeObservation{}, observeErr
			}
			additional[i] = domain.RepositoryScopeEntry{Key: entry.Key, Origin: entry.Origin, Binding: binding}
		}
		return recovery.RepositoryScopeObservation{Primary: primary, Additional: additional}, nil
	}
	first, err := observeScope()
	if err != nil || len(task.AdditionalRepositories) == 0 {
		return first, err
	}
	second, err := observeScope()
	if err != nil {
		return recovery.RepositoryScopeObservation{}, err
	}
	if !sameRepositoryScopeObservation(first, second) {
		return recovery.RepositoryScopeObservation{}, domain.WithExplanation(domain.ErrWorkspaceObservationUnstable, "The repository scope changed between the two consistency observations.")
	}
	return second, nil
}

func sameRepositoryScopeObservation(left, right recovery.RepositoryScopeObservation) bool {
	if left.Primary.BindingDigest != right.Primary.BindingDigest || len(left.Additional) != len(right.Additional) {
		return false
	}
	for index := range left.Additional {
		if left.Additional[index].Key != right.Additional[index].Key ||
			left.Additional[index].Binding.BindingDigest != right.Additional[index].Binding.BindingDigest {
			return false
		}
	}
	return true
}

func scopeWorkspaceDigests(task domain.ProcessTask, fresh recovery.RepositoryScopeObservation) (domain.WorkspaceDigests, error) {
	copy := task
	rebindTaskRepositories(&copy, fresh)
	return copy.EffectiveWorkspaceDigests()
}

func scopeHasUnavailableWorkspace(task domain.ProcessTask, fresh recovery.RepositoryScopeObservation) bool {
	if task.Repository.WorktreeInstanceDigest != fresh.Primary.WorktreeInstanceDigest || task.Repository.IdentityDigest != fresh.Primary.IdentityDigest {
		return true
	}
	for i := range task.AdditionalRepositories {
		if task.AdditionalRepositories[i].Binding.WorktreeInstanceDigest != fresh.Additional[i].Binding.WorktreeInstanceDigest || task.AdditionalRepositories[i].Binding.IdentityDigest != fresh.Additional[i].Binding.IdentityDigest {
			return true
		}
	}
	return false
}

func scopeHasHistoryConflict(fresh recovery.RepositoryScopeObservation) bool {
	conflict := func(binding domain.RepositoryBinding) bool {
		return binding.HistoryRelation != domain.RepositoryHistoryExact && binding.HistoryRelation != domain.RepositoryHistoryLinearAdvance
	}
	if conflict(fresh.Primary) {
		return true
	}
	for _, entry := range fresh.Additional {
		if conflict(entry.Binding) {
			return true
		}
	}
	return false
}

func contentDiffersFromCurrentAuthority(task domain.ProcessTask, fresh recovery.RepositoryScopeObservation) bool {
	digests, err := scopeWorkspaceDigests(task, fresh)
	if err != nil {
		return true
	}
	switch task.CurrentNode {
	case domain.NodeTest:
		return task.Implementation != nil && task.Implementation.ContentDigest != digests.Content
	case domain.NodeComprehensionReview, domain.NodeDelivery:
		return task.Test != nil && task.Test.ContentDigest != digests.Content
	default:
		return false
	}
}

func implementationContentMustRemainCurrent(node domain.NodeID) bool {
	return node == domain.NodeTest || node == domain.NodeComprehensionReview || node == domain.NodeDelivery
}

// guardTaskWorkspace performs the proactive guard used by resume and
// GetNextAction. Ordinary content changes remain bound to the issued Action;
// only blockers mutate Task state here.
func (s *Service) guardTaskWorkspace(ctx context.Context, task domain.ProcessTask, requestID domain.ID) (domain.ProcessTask, error) {
	if task.CurrentNode.Terminal() || task.BranchRename != nil {
		return task, nil
	}
	pendingRecovery, err := s.hasPendingActionOperation(ctx, task)
	if err != nil {
		return domain.ProcessTask{}, err
	}
	if pendingRecovery {
		return task, nil
	}
	fresh, err := s.observeTaskRepositories(ctx, task)
	if err != nil {
		return domain.ProcessTask{}, err
	}
	if scopeHasUnavailableWorkspace(task, fresh) {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "An observed worktree is missing or no longer has the instance identity retained by the Task.")
	}
	if scopeHasHistoryConflict(fresh) {
		return s.createWorkspaceHistoryBlocker(ctx, task, fresh, requestID)
	}
	if implementationContentMustRemainCurrent(task.CurrentNode) && contentDiffersFromCurrentAuthority(task, fresh) {
		return s.invalidateContentEvidence(ctx, task, fresh, requestID)
	}
	if task.TaskPlan != nil {
		outside := task.UnexplainedChangedPaths(fresh.Primary, fresh.Additional)
		if len(outside) != 0 && task.CurrentNode != domain.NodeBlocked {
			return s.createObservedFileScopeBlocker(ctx, task, fresh, outside, requestID)
		}
	}
	return task, nil
}

func (s *Service) hasPendingActionOperation(ctx context.Context, task domain.ProcessTask) (bool, error) {
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return false, nil
	}
	operation, found, err := operationStore.LoadActionOperation(ctx, task.TaskID)
	if err != nil {
		return false, mapStoreError(err)
	}
	if !found {
		return false, nil
	}
	if workflow.ValidateActionCommit(task, operation.Commit) != nil {
		return false, domain.WithExplanation(domain.ErrStorageUnavailable, "The prepared Action operation does not match the Task, payload or operation digest.")
	}
	if operation.AppliedRevision == nil {
		return true, nil
	}
	if !operation.RecordedBy(task) {
		return false, domain.WithExplanation(domain.ErrStorageUnavailable, "An Action operation is pending, so workspace guarding cannot replace the Task state before recovery.")
	}
	return false, nil
}

func (s *Service) invalidateContentEvidence(ctx context.Context, task domain.ProcessTask, fresh recovery.RepositoryScopeObservation, requestID domain.ID) (domain.ProcessTask, error) {
	next, err := cloneProcessTask(task)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The saved Task could not be decoded into a working copy for this operation.")
	}
	eventID, err := s.id("event")
	if err != nil {
		return domain.ProcessTask{}, err
	}
	actionID, err := s.id("action")
	if err != nil {
		return domain.ProcessTask{}, err
	}
	now := s.now().UTC()
	source := task.CurrentNode
	rebindProcessAuthorities(&next, fresh)
	next.Implementation, next.Test, next.Comprehension = nil, nil, nil
	next.CurrentNode = domain.NodeImplement
	next.Blocker, next.ResumeNode, next.Relocation = nil, nil, nil
	next.Revision++
	next.UpdatedAt = now
	workspace, err := next.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	action, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), domain.NodeImplement, next.TaskID, next.Revision, workspace, next.Intent.MethodProfile, actionID, now)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "Core could not construct an Action from the current node, Task revision and workspace.")
	}
	next.CurrentAction = &action
	previousContent := task.Implementation.ContentDigest
	if task.Test != nil {
		previousContent = task.Test.ContentDigest
	}
	payloadDigest, err := digestCanonical(struct {
		Previous domain.Digest `json:"previous_content_digest"`
		Current  domain.Digest `json:"current_content_digest"`
	}{previousContent, workspace.Content})
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the content change for its invalidation digest.")
	}
	oldAction := task.CurrentAction.ActionID
	next.LastOperation = &domain.LastOperation{OperationID: requestID, Kind: domain.OperationObserveWorkspace, ActionID: &oldAction, FromRevision: task.Revision, ToRevision: next.Revision, PayloadDigest: payloadDigest, CommittedAt: now}
	event := store.TaskEvent{EventID: eventID, TaskID: task.TaskID, Revision: next.Revision, Kind: domain.OperationObserveWorkspace, SourceNode: source, DestinationNode: domain.NodeImplement, TransitionReason: "Repository content changed after implementation verification; implementation, Test and comprehension must be re-established.", ActionID: &oldAction, RepositoryDeltaPaths: observedTaskDeltaPaths(task, fresh), RequestID: requestID, PayloadDigest: payloadDigest, CreatedAt: now}
	if err := s.taskStore.CommitTask(ctx, store.TaskMutation{ExpectedRevision: task.Revision, Task: next, Event: event, Claim: store.ClaimRetain}); err != nil {
		return domain.ProcessTask{}, mapStoreError(err)
	}
	return next, nil
}

func workspaceCondition(task domain.ProcessTask, relocationID domain.ID, kind domain.BlockerConditionKind) (domain.BlockerCondition, error) {
	digests, err := task.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.BlockerCondition{}, err
	}
	return domain.BlockerCondition{Kind: kind, ExpectedBindingDigest: digests.Binding, ExpectedIdentityDigest: digests.Identity, ExpectedHistoryDigest: digests.History, ExpectedContentDigest: digests.Content, RelocationID: relocationID}, nil
}

func (s *Service) createWorkspaceHistoryBlocker(ctx context.Context, task domain.ProcessTask, fresh recovery.RepositoryScopeObservation, requestID domain.ID) (domain.ProcessTask, error) {
	refreshing := task.CurrentNode == domain.NodeBlocked && task.Blocker != nil && task.Blocker.Cause == domain.BlockerCauseWorkspaceHistoryConflict
	if task.CurrentNode == domain.NodeBlocked && !refreshing {
		return task, nil
	}
	next, err := cloneProcessTask(task)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The saved Task could not be decoded into a working copy for this operation.")
	}
	blockerID, err := s.id("blocker")
	if err != nil {
		return domain.ProcessTask{}, err
	}
	actionID, err := s.id("action")
	if err != nil {
		return domain.ProcessTask{}, err
	}
	eventID, err := s.id("event")
	if err != nil {
		return domain.ProcessTask{}, err
	}
	condition := domain.BlockerCondition{}
	if refreshing {
		condition = task.Blocker.Condition
	} else {
		condition, err = workspaceCondition(task, "", domain.BlockerConditionResolveHistory)
		if err != nil {
			return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "Core could not construct the workspace-history condition from the saved Task.")
		}
	}
	observed, err := scopeWorkspaceDigests(task, fresh)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The observed repository scope is invalid and cannot produce workspace digests.")
	}
	if refreshing && observed.Binding == task.Blocker.ObservedBindingDigest {
		return task, nil
	}
	observedHistory, err := workspaceHistorySnapshot(task, fresh, observed)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The observed repository history could not form a valid history snapshot.")
	}
	now := s.now().UTC()
	resume := task.CurrentNode
	if refreshing {
		resume = *task.ResumeNode
	}
	sourceNode := task.CurrentNode
	next.CurrentNode, next.ResumeNode = domain.NodeBlocked, &resume
	next.Revision++
	next.UpdatedAt = now
	next.Blocker = &domain.ProcessBlocker{
		BlockerID:                blockerID,
		Code:                     domain.ErrorTaskBlocked,
		Cause:                    domain.BlockerCauseWorkspaceHistoryConflict,
		Message:                  workspaceHistoryBlockerMessage(observedHistory),
		ResumeNode:               resume,
		ObservedBindingDigest:    observed.Binding,
		ObservedWorkspaceHistory: &observedHistory,
		Condition:                condition,
		RequiredResolution:       "Restore the retained history or explicitly accept the reviewed same-branch history after confirming its content and task surface.",
		CreatedAt:                now,
	}
	workspace, err := next.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	action, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), domain.NodeBlocked, next.TaskID, next.Revision, workspace, next.Intent.MethodProfile, actionID, now)
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "Core could not construct an Action from the current node, Task revision and workspace.")
	}
	next.CurrentAction = &action
	payloadDigest, err := digestCanonical(struct {
		TaskID   domain.ID     `json:"task_id"`
		Observed domain.Digest `json:"observed_binding_digest"`
	}{task.TaskID, observed.Binding})
	if err != nil {
		return domain.ProcessTask{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the workspace-history blocker for its operation digest.")
	}
	sourceAction := task.CurrentAction.ActionID
	next.LastOperation = &domain.LastOperation{OperationID: requestID, Kind: domain.OperationObserveWorkspace, ActionID: &sourceAction, FromRevision: task.Revision, ToRevision: next.Revision, PayloadDigest: payloadDigest, CommittedAt: now}
	event := store.TaskEvent{EventID: eventID, TaskID: task.TaskID, Revision: next.Revision, Kind: domain.OperationObserveWorkspace, SourceNode: sourceNode, DestinationNode: domain.NodeBlocked, TransitionReason: next.Blocker.Message, ActionID: &sourceAction, RepositoryDeltaPaths: observedTaskDeltaPaths(task, fresh), RequestID: requestID, PayloadDigest: payloadDigest, CreatedAt: now}
	if err := s.taskStore.CommitTask(ctx, store.TaskMutation{ExpectedRevision: task.Revision, Task: next, Event: event, Claim: store.ClaimRetain}); err != nil {
		return domain.ProcessTask{}, mapStoreError(err)
	}
	return next, nil
}

func workspaceHistorySnapshot(task domain.ProcessTask, fresh recovery.RepositoryScopeObservation, digests domain.WorkspaceDigests) (domain.WorkspaceHistorySnapshot, error) {
	repositories := make([]domain.WorkspaceHistoryRepositorySnapshot, 0, len(fresh.Additional)+1)
	observedAt := fresh.Primary.ObservedAt
	appendRepository := func(key domain.RepositoryKey, binding domain.RepositoryBinding) {
		var branch *string
		if binding.CurrentBranch != nil {
			value := *binding.CurrentBranch
			branch = &value
		}
		repositories = append(repositories, domain.WorkspaceHistoryRepositorySnapshot{
			RepositoryKey:      key,
			HistoryRelation:    binding.HistoryRelation,
			CurrentBranch:      branch,
			Detached:           binding.Detached,
			CurrentHead:        binding.CurrentHead,
			BaseCommitAncestor: binding.BaseCommitAncestor,
			ContentDigest:      binding.ContentDigest,
			TaskSurface:        domain.RepositoryChangedPaths(binding.TaskSurface),
		})
		if binding.ObservedAt.After(observedAt) {
			observedAt = binding.ObservedAt
		}
	}
	appendRepository(task.EffectivePrimaryRepositoryKey(), fresh.Primary)
	for _, repository := range fresh.Additional {
		appendRepository(repository.Key, repository.Binding)
	}
	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].RepositoryKey < repositories[j].RepositoryKey
	})
	snapshot := domain.WorkspaceHistorySnapshot{
		BindingDigest:       digests.Binding,
		IdentityDigest:      digests.Identity,
		HistoryDigest:       digests.History,
		ContentDigest:       digests.Content,
		CurrentChangedPaths: currentRepositoryScopePaths(task.EffectivePrimaryRepositoryKey(), fresh.Primary, fresh.Additional),
		Repositories:        repositories,
		ObservedAt:          observedAt,
	}
	if snapshot.Validate() != nil {
		return domain.WorkspaceHistorySnapshot{}, domain.WithExplanation(domain.ErrInvalidArgument, "The history snapshot has invalid repository keys, workspace digests or observation metadata.")
	}
	return snapshot, nil
}

func workspaceHistoryBlockerMessage(snapshot domain.WorkspaceHistorySnapshot) string {
	for _, repository := range snapshot.Repositories {
		if repository.HistoryRelation == domain.RepositoryHistoryExact || repository.HistoryRelation == domain.RepositoryHistoryLinearAdvance {
			continue
		}
		return `Repository "` + string(repository.RepositoryKey) + `" history is ` + string(repository.HistoryRelation) + "."
	}
	return "The Task worktree history no longer has an allowed linear relationship to the issued Action."
}
