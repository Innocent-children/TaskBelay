package application

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

// WorkspaceRelocationSupported limits Host handoff to dedicated Task worktrees.
func WorkspaceRelocationSupported(task domain.ProcessTask) bool {
	if task.WorkspaceOrigin.Mode != domain.WorkspaceModeDedicatedWorktree {
		return false
	}
	for _, entry := range task.AdditionalRepositories {
		if entry.Origin.Mode != domain.WorkspaceModeDedicatedWorktree {
			return false
		}
	}
	return true
}

func (s *Service) PrepareTaskRelocation(ctx context.Context, request PrepareTaskRelocationRequest) (PrepareTaskRelocationResult, error) {
	if !s.valid() || ctx == nil || !request.RequestID.IsValid() || !request.Host.IsValid() || !request.TaskID.IsValid() || request.ExpectedRevision == 0 {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The application service, request context or required request identity is invalid.")
	}
	task, err := s.loadOwned(ctx, request.Host, request.TaskID)
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	if !WorkspaceRelocationSupported(task) {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "Relocation requires dedicated_worktree mode for every repository; local branch modes must resume in their original directories.")
	}
	if task.CurrentNode == domain.NodeBlocked && task.Blocker != nil && task.Blocker.Cause == domain.BlockerCauseTaskRelocationPending && task.Relocation != nil && request.ExpectedRevision+1 == task.Revision {
		return PrepareTaskRelocationResult{Task: task, RelocationID: task.Relocation.RelocationID}, nil
	}
	if task.CurrentNode.Terminal() {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrTaskTerminal, "This operation requires an active Task, but the Task has already reached DONE or CANCELLED.")
	}
	if task.CurrentNode == domain.NodeBlocked {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "The Task is already BLOCKED; resolve the current blocker before preparing a relocation.")
	}
	if task.Revision != request.ExpectedRevision {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "The supplied revision does not match the saved Task revision.")
	}
	fresh, err := s.observeTaskRepositories(ctx, task)
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	if scopeHasUnavailableWorkspace(task, fresh) {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "An observed worktree is missing or no longer has the instance identity retained by the Task.")
	}
	if scopeHasHistoryConflict(fresh) {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The observed branch or commit history conflicts with the history retained by the Task.")
	}
	next, err := cloneProcessTask(task)
	if err != nil {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInternal, "The saved Task could not be decoded into a working copy for this operation.")
	}
	relocationID, err := s.id("relocation")
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	blockerID, err := s.id("blocker")
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	actionID, err := s.id("action")
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	eventID, err := s.id("event")
	if err != nil {
		return PrepareTaskRelocationResult{}, err
	}
	now := s.now().UTC()
	resume := task.CurrentNode
	workspace, err := scopeWorkspaceDigests(task, fresh)
	if err != nil {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInternal, "The observed repository scope is invalid and cannot produce workspace digests.")
	}
	surface := currentRepositoryScopePaths(task.EffectivePrimaryRepositoryKey(), fresh.Primary, fresh.Additional)
	next.Relocation = &domain.TaskRelocation{
		RelocationID:         relocationID,
		SourceBindingDigest:  workspace.Binding,
		SourceIdentityDigest: workspace.Identity,
		SourceHistoryDigest:  workspace.History,
		SourceContentDigest:  workspace.Content,
		SourceTaskSurface:    surface,
		ResumeNode:           resume,
		PreparedAt:           now,
	}
	retainedWorkspace, err := next.EffectiveWorkspaceDigests()
	if err != nil {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInternal, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	condition := domain.BlockerCondition{
		Kind:                   domain.BlockerConditionResolveRelocation,
		ExpectedBindingDigest:  retainedWorkspace.Binding,
		ExpectedIdentityDigest: retainedWorkspace.Identity,
		ExpectedHistoryDigest:  retainedWorkspace.History,
		ExpectedContentDigest:  retainedWorkspace.Content,
		RelocationID:           relocationID,
	}
	next.CurrentNode, next.ResumeNode = domain.NodeBlocked, &resume
	next.Revision++
	next.UpdatedAt = now
	next.Blocker = &domain.ProcessBlocker{
		BlockerID:             blockerID,
		Code:                  domain.ErrorTaskBlocked,
		Cause:                 domain.BlockerCauseTaskRelocationPending,
		Message:               "Task relocation is prepared and waiting for the Host to hand off the same workspace content.",
		ResumeNode:            resume,
		ObservedBindingDigest: workspace.Binding,
		Condition:             condition,
		RequiredResolution:    "Complete one same-machine Host handoff, then resolve with the relocation ID and every destination repository path.",
		CreatedAt:             now,
	}
	blockedWorkspace := retainedWorkspace
	action, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), domain.NodeBlocked, next.TaskID, next.Revision, blockedWorkspace, next.Intent.MethodProfile, actionID, now)
	if err != nil {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not construct an Action from the current node, Task revision and workspace.")
	}
	next.CurrentAction = &action
	payloadDigest, err := digestCanonical(struct {
		TaskID       domain.ID `json:"task_id"`
		Revision     uint64    `json:"revision"`
		RelocationID domain.ID `json:"relocation_id"`
	}{task.TaskID, task.Revision, relocationID})
	if err != nil {
		return PrepareTaskRelocationResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the relocation preparation for its operation digest.")
	}
	next.LastOperation = &domain.LastOperation{
		OperationID:   request.RequestID,
		Kind:          domain.OperationPrepareTaskRelocation,
		FromRevision:  task.Revision,
		ToRevision:    next.Revision,
		PayloadDigest: payloadDigest,
		CommittedAt:   now,
	}
	sourceBindingDigest := workspace.Binding
	event := store.TaskEvent{
		EventID:               eventID,
		TaskID:                task.TaskID,
		Revision:              next.Revision,
		Kind:                  domain.OperationPrepareTaskRelocation,
		SourceNode:            resume,
		DestinationNode:       domain.NodeBlocked,
		TransitionReason:      next.Blocker.Message,
		ObservedBindingDigest: &sourceBindingDigest,
		RequestID:             request.RequestID,
		PayloadDigest:         payloadDigest,
		CreatedAt:             now,
	}
	if err := s.taskStore.CommitTask(ctx, store.TaskMutation{ExpectedRevision: task.Revision, Task: next, Event: event, Claim: store.ClaimRetain}); err != nil {
		return PrepareTaskRelocationResult{}, mapStoreError(err)
	}
	return PrepareTaskRelocationResult{Task: next, RelocationID: relocationID}, nil
}

func (s *Service) resolveTaskRelocationPayload(ctx context.Context, apply ApplyActionRequest, task domain.ProcessTask, payload recovery.BlockerResolutionPayload, canonical json.RawMessage) (ApplyActionResult, error) {
	if task.Blocker == nil || task.Relocation == nil ||
		payload.BlockerID != task.Blocker.BlockerID || payload.Condition != task.Blocker.Condition ||
		payload.RelocationID != task.Relocation.RelocationID {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The saved relocation payload does not match the current blocker, its condition or the prepared relocation identity.")
	}
	request := RecoverActionRequest{
		Host:                   apply.Host,
		TaskID:                 apply.TaskID,
		ActionID:               apply.ActionID,
		RelocationID:           payload.RelocationID,
		RelocationDestinations: payload.RelocationDestinations,
	}
	return s.resolveTaskRelocation(ctx, request, apply.RequestID, task, payload.ObservedBindingDigest, canonical)
}

func (s *Service) resolveTaskRelocation(ctx context.Context, request RecoverActionRequest, requestID domain.ID, task domain.ProcessTask, expectedObservedDigest domain.Digest, retainedCanonical json.RawMessage) (ApplyActionResult, error) {
	if task.Relocation == nil || task.Blocker == nil || task.ResumeNode == nil || task.CurrentAction == nil ||
		request.ActionID != task.CurrentAction.ActionID || !requestID.IsValid() ||
		request.RelocationID != task.Relocation.RelocationID ||
		len(request.RelocationDestinations) != len(task.AdditionalRepositories)+1 {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "The relocation identity or destination count does not match the relocation prepared for this Task.")
	}
	target, err := s.validateTaskRelocationDestination(ctx, task, request.RelocationDestinations, expectedObservedDigest)
	if err != nil {
		return ApplyActionResult{}, err
	}
	primaryOrigin := target.primaryOrigin
	primary := target.scope.Primary
	additional := target.scope.Additional
	fresh := target.scope
	workspace := target.workspace
	current := target.currentChangedPaths
	next, err := cloneProcessTask(task)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The saved Task could not be decoded into a working copy for this operation.")
	}
	previousClaims := store.RepositoryClaimIdentities(task)
	next.WorkspaceOrigin, next.Repository, next.AdditionalRepositories = primaryOrigin, primary, additional
	next.CurrentChangedPaths = current
	destination := *task.ResumeNode
	contentAuthorityChanged := destination == domain.NodeTest && task.Implementation != nil && task.Implementation.ContentDigest != workspace.Content ||
		(destination == domain.NodeComprehensionReview || destination == domain.NodeDelivery) && task.Test != nil && task.Test.ContentDigest != workspace.Content
	if contentAuthorityChanged {
		next.Implementation, next.Test, next.Comprehension = nil, nil, nil
		destination = domain.NodeImplement
	}
	next.CurrentNode = destination
	next.ResumeNode, next.Blocker, next.Relocation = nil, nil, nil
	next.Revision++
	now := s.now().UTC()
	next.UpdatedAt = now
	actionID, err := s.id("action")
	if err != nil {
		return ApplyActionResult{}, err
	}
	eventID, err := s.id("event")
	if err != nil {
		return ApplyActionResult{}, err
	}
	workspace, err = next.EffectiveWorkspaceDigests()
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	action, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), destination, next.TaskID, next.Revision, workspace, next.Intent.MethodProfile, actionID, now)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not construct an Action from the current node, Task revision and workspace.")
	}
	next.CurrentAction = &action
	canonical := append(json.RawMessage(nil), retainedCanonical...)
	if len(canonical) == 0 {
		payload := domain.BlockerResolutionPayload{
			BlockerID:              task.Blocker.BlockerID,
			Condition:              task.Blocker.Condition,
			ObservedBindingDigest:  workspace.Binding,
			RelocationID:           request.RelocationID,
			RelocationDestinations: append([]domain.RelocationDestination(nil), request.RelocationDestinations...),
		}
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "Core could not encode the prepared operation payload as JSON.")
		}
		_, canonical, err = workflow.DecodeBlockerResolutionPayload(raw)
		if err != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The blocker-resolution payload does not match the saved blocker contract.")
		}
	}
	apply := applyRequestForCurrentAction(requestID, request.Host, task, canonical)
	operation := operationFromApply(apply)
	payloadDigest, err := workflow.GraphOperationDigest(request.Host, task.TaskID, operation, canonical)
	if err != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The Action identity or canonical payload could not be encoded into an operation digest.")
	}
	resolvedAction := task.CurrentAction.ActionID
	next.LastOperation = &domain.LastOperation{OperationID: requestID, Kind: domain.OperationApplyAction, ActionID: &resolvedAction, FromRevision: task.Revision, ToRevision: next.Revision, PayloadDigest: payloadDigest, CommittedAt: now}
	event := store.TaskEvent{EventID: eventID, TaskID: task.TaskID, Revision: next.Revision, Kind: domain.OperationApplyAction, SourceNode: domain.NodeBlocked, DestinationNode: destination, TransitionReason: "Task relocation verified and claims moved to the destination worktree instances.", ActionID: &resolvedAction, RepositoryDeltaPaths: observedTaskDeltaPaths(task, fresh), RequestID: requestID, PayloadDigest: payloadDigest, CreatedAt: now}
	mutation := store.TaskMutation{ExpectedRevision: task.Revision, Task: next, Event: event, Claim: store.ClaimReplace, PreviousClaims: previousClaims}
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The connected Task store does not support retaining and recovering Action operations.")
	}
	commit := domain.ActionCommit{Operation: operation, Payload: canonical, PayloadDigest: payloadDigest, PreparedAt: now}
	if workflow.ValidateActionCommit(task, commit) != nil {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInternal, "The prepared Action operation does not match the Task, payload or operation digest.")
	}
	existing, found, err := operationStore.LoadActionOperation(ctx, task.TaskID)
	if err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	staged := false
	if found && existing.Commit.Operation.ActionID == operation.ActionID {
		if existing.RecordedBy(task) {
			return ApplyActionResult{Task: task}, nil
		}
		expected := commit
		expected.PreparedAt = existing.Commit.PreparedAt
		if !existing.Commit.Equal(expected) || existing.AppliedRevision != nil {
			return ApplyActionResult{}, domain.WithExplanation(domain.ErrRecoveryUnavailable, "A different relocation operation is already retained, or the retained operation has already been applied.")
		}
		commit = existing.Commit
		staged = true
	}
	if !staged {
		if err := operationStore.StageActionOperation(ctx, task, commit); err != nil {
			return ApplyActionResult{}, mapStoreError(err)
		}
	}
	if err := operationStore.CommitActionOperation(ctx, operation.OperationID, mutation); err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	return ApplyActionResult{Task: next}, nil
}

type validatedRelocationTarget struct {
	primaryOrigin       domain.WorkspaceOrigin
	scope               recovery.RepositoryScopeObservation
	workspace           domain.WorkspaceDigests
	currentChangedPaths []string
}

func (s *Service) assessTaskRelocationCommit(ctx context.Context, host domain.Host, task domain.ProcessTask, commit domain.ActionCommit) (*recovery.RecoveryAssessment, error) {
	if host != task.OriginHost {
		return nil, domain.WithExplanation(domain.ErrHostOwnershipConflict, "The requested host does not match the host that owns this Task.")
	}
	payload, _, err := workflow.DecodeBlockerResolutionPayload(commit.Payload)
	if err != nil || task.Blocker == nil || task.Relocation == nil ||
		payload.BlockerID != task.Blocker.BlockerID || payload.Condition != task.Blocker.Condition ||
		payload.RelocationID != task.Relocation.RelocationID {
		return nil, domain.WithExplanation(domain.ErrStorageUnavailable, "The saved relocation payload does not match the current blocker, its condition or the prepared relocation identity.")
	}
	target, err := s.validateTaskRelocationDestination(ctx, task, payload.RelocationDestinations, payload.ObservedBindingDigest)
	if err != nil {
		return nil, err
	}
	authoritative, err := task.EffectiveRepositoryBindingDigest()
	if err != nil {
		return nil, domain.WithExplanation(domain.ErrInternal, "The Task repository bindings are invalid and cannot produce a binding digest.")
	}
	observedAt := target.scope.Primary.ObservedAt
	for _, repository := range target.scope.Additional {
		if repository.Binding.ObservedAt.After(observedAt) {
			observedAt = repository.Binding.ObservedAt
		}
	}
	var currentActionID *domain.ID
	if task.CurrentAction != nil {
		value := task.CurrentAction.ActionID
		currentActionID = &value
	}
	payloadDigest := commit.PayloadDigest
	condition := task.Blocker.Condition
	assessment := &recovery.RecoveryAssessment{
		Classification:             domain.RecoveryCompletedButUnrecorded,
		Operation:                  commit.Operation,
		TaskRevision:               task.Revision,
		CurrentActionID:            currentActionID,
		IssuanceBindingDigest:      commit.Operation.RepositoryBindingDigest,
		AuthoritativeBindingDigest: authoritative,
		ObservedBindingDigest:      target.workspace.Binding,
		RepositoryRelation:         recovery.RepositoryWorktreeOnlyChanged,
		LastOperationRelation:      recovery.LastOperationUnrelated,
		OperationEvidence:          recovery.OperationEvidenceComplete,
		OperationPayloadDigest:     &payloadDigest,
		ActionRetrySafe:            false,
		NextAdvice:                 recovery.AdviceSubmitRecoveryApply,
		UnblockCondition:           &condition,
		ObservedAt:                 observedAt,
	}
	if len(task.AdditionalRepositories) != 0 {
		assessment.Repositories = make([]recovery.RepositoryFact, 0, len(task.AdditionalRepositories)+1)
		assessment.Repositories = append(assessment.Repositories, recovery.RepositoryFact{
			RepositoryKey: task.EffectivePrimaryRepositoryKey(),
			Relation:      recovery.RepositoryWorktreeOnlyChanged,
			Reason:        recovery.RepositoryReasonWorktreeInstance,
		})
		for _, repository := range task.AdditionalRepositories {
			assessment.Repositories = append(assessment.Repositories, recovery.RepositoryFact{
				RepositoryKey: repository.Key,
				Relation:      recovery.RepositoryWorktreeOnlyChanged,
				Reason:        recovery.RepositoryReasonWorktreeInstance,
			})
		}
		sort.Slice(assessment.Repositories, func(i, j int) bool {
			return assessment.Repositories[i].RepositoryKey < assessment.Repositories[j].RepositoryKey
		})
	}
	return assessment, nil
}

func (s *Service) validateTaskRelocationDestination(ctx context.Context, task domain.ProcessTask, requested []domain.RelocationDestination, expectedObservedDigest domain.Digest) (validatedRelocationTarget, error) {
	observer, ok := s.repositoryObserver.(repository.WorkspaceRepositoryObserver)
	if !ok {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInternal, "The repository observer cannot verify prepared workspaces for this operation.")
	}
	destinations := append([]domain.RelocationDestination(nil), requested...)
	sort.Slice(destinations, func(i, j int) bool {
		return destinations[i].Key < destinations[j].Key
	})
	expectedKeys := []domain.RepositoryKey{task.EffectivePrimaryRepositoryKey()}
	for _, entry := range task.AdditionalRepositories {
		expectedKeys = append(expectedKeys, entry.Key)
	}
	sort.Slice(expectedKeys, func(i, j int) bool {
		return expectedKeys[i] < expectedKeys[j]
	})
	if len(destinations) != len(expectedKeys) {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "Relocation must include exactly one destination for every repository in the Task.")
	}
	byKey := make(map[domain.RepositoryKey]domain.RelocationDestination, len(destinations))
	for index, key := range expectedKeys {
		destination := destinations[index]
		if destination.Key != key || !validRelocationRepositoryPath(destination.RepositoryPath) {
			return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "Each relocation destination must use the expected repository key and a normalized absolute repository path.")
		}
		byKey[destination.Key] = destination
	}
	observed, err := s.observeRelocationDestinations(ctx, observer, task, byKey)
	if err != nil {
		return validatedRelocationTarget{}, err
	}
	primary := observed.scope.Primary
	if primary.WorktreeInstanceDigest == task.Repository.WorktreeInstanceDigest {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "A relocation destination must be a different worktree instance from its source.")
	}
	if !relocationDestinationHistoryAllowed(task.Repository, primary) {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The relocation destination does not preserve the prepared source branch and permitted commit history.")
	}
	if observed.primaryOrigin.SourceRepositoryGroupDigest != task.WorkspaceOrigin.SourceRepositoryGroupDigest {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "The relocation destination belongs to a different Git repository group than the retained source.")
	}
	for index, source := range task.AdditionalRepositories {
		destination := observed.scope.Additional[index]
		if destination.Binding.WorktreeInstanceDigest == source.Binding.WorktreeInstanceDigest {
			return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "A relocation destination must be a different worktree instance from its source.")
		}
		if !relocationDestinationHistoryAllowed(source.Binding, destination.Binding) {
			return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The relocation destination does not preserve the prepared source branch and permitted commit history.")
		}
		if destination.Origin.SourceRepositoryGroupDigest != source.Origin.SourceRepositoryGroupDigest {
			return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrWorkspaceUnavailable, "The relocation destination belongs to a different Git repository group than the retained source.")
		}
	}
	candidate := task
	candidate.WorkspaceOrigin = observed.primaryOrigin
	candidate.Repository = primary
	candidate.AdditionalRepositories = observed.scope.Additional
	workspace, err := candidate.EffectiveWorkspaceDigests()
	if err != nil {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInternal, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	if workspace.Identity == task.Relocation.SourceIdentityDigest {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "The relocation destinations still have the original workspace identity.")
	}
	if expectedObservedDigest != "" && (!expectedObservedDigest.IsValid() || expectedObservedDigest != workspace.Binding) {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrInvalidArgument, "The observed relocation binding digest no longer matches the prepared destination digest.")
	}
	current := currentRepositoryScopePaths(candidate.EffectivePrimaryRepositoryKey(), observed.scope.Primary, observed.scope.Additional)
	if workspace.Content != task.Relocation.SourceContentDigest || !sameStrings(current, task.Relocation.SourceTaskSurface) {
		return validatedRelocationTarget{}, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The relocation destination content or changed-file set differs from the prepared source.")
	}
	return validatedRelocationTarget{
		primaryOrigin:       observed.primaryOrigin,
		scope:               observed.scope,
		workspace:           workspace,
		currentChangedPaths: current,
	}, nil
}

func validRelocationRepositoryPath(path string) bool {
	return validRepositoryPathInput(path) && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func relocationDestinationHistoryAllowed(previous domain.RepositoryBinding, binding domain.RepositoryBinding) bool {
	if binding.Detached || binding.CurrentBranch == nil || previous.CurrentBranch == nil || *binding.CurrentBranch != *previous.CurrentBranch || !binding.BaseCommitAncestor {
		return false
	}
	return binding.HistoryRelation == domain.RepositoryHistoryExact ||
		binding.HistoryRelation == domain.RepositoryHistoryLinearAdvance
}

type relocationObservation struct {
	primaryOrigin domain.WorkspaceOrigin
	scope         recovery.RepositoryScopeObservation
}

func (s *Service) observeRelocationDestinations(ctx context.Context, observer repository.WorkspaceRepositoryObserver, task domain.ProcessTask, destinations map[domain.RepositoryKey]domain.RelocationDestination) (relocationObservation, error) {
	observeScope := func() (relocationObservation, error) {
		primaryDestination := destinations[task.EffectivePrimaryRepositoryKey()]
		primaryOrigin, primary, err := observer.ObserveWorkspace(
			ctx,
			primaryDestination.RepositoryPath,
			persistedOriginSelection(task.WorkspaceOrigin),
			&task.Repository,
		)
		if err != nil {
			return relocationObservation{}, mapWorkspaceObservationError(err)
		}
		additional := make([]domain.RepositoryScopeEntry, len(task.AdditionalRepositories))
		for index, source := range task.AdditionalRepositories {
			destination := destinations[source.Key]
			origin, binding, observeErr := observer.ObserveWorkspace(
				ctx,
				destination.RepositoryPath,
				persistedOriginSelection(source.Origin),
				&source.Binding,
			)
			if observeErr != nil {
				return relocationObservation{}, mapWorkspaceObservationError(observeErr)
			}
			additional[index] = domain.RepositoryScopeEntry{
				Key:     source.Key,
				Origin:  origin,
				Binding: binding,
			}
		}
		return relocationObservation{
			primaryOrigin: primaryOrigin,
			scope: recovery.RepositoryScopeObservation{
				Primary:    primary,
				Additional: additional,
			},
		}, nil
	}

	first, err := observeScope()
	if err != nil || len(task.AdditionalRepositories) == 0 {
		return first, err
	}
	second, err := observeScope()
	if err != nil {
		return relocationObservation{}, err
	}
	if first.primaryOrigin != second.primaryOrigin || !sameRepositoryScopeObservation(first.scope, second.scope) {
		return relocationObservation{}, domain.WithExplanation(domain.ErrWorkspaceObservationUnstable, "The workspace origin or repository observation changed between the two consistency reads.")
	}
	for index := range first.scope.Additional {
		if first.scope.Additional[index].Origin != second.scope.Additional[index].Origin {
			return relocationObservation{}, domain.WithExplanation(domain.ErrWorkspaceObservationUnstable, "An additional relocation destination changed between the two consistency reads.")
		}
	}
	return second, nil
}
