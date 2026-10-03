package application

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func (s *Service) PrepareTaskBranchRename(ctx context.Context, r PrepareTaskBranchRenameRequest) (PrepareTaskBranchRenameResult, error) {
	if !s.valid() || ctx == nil || !r.RequestID.IsValid() || !r.Host.IsValid() || !r.TaskID.IsValid() || r.ExpectedRevision == 0 || !r.RepositoryKey.IsValid() || !domain.ValidTaskBranchName(r.TargetBranch) || !validBranchRenameReason(r.Reason) {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "Branch rename requires valid Task/revision/repository_key, a branch name and a non-empty reason.")
	}
	task, err := s.loadOwned(ctx, r.Host, r.TaskID)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	if task.BranchRename != nil && task.Revision == r.ExpectedRevision+1 && task.BranchRename.RepositoryKey == r.RepositoryKey && task.BranchRename.TargetBranch == r.TargetBranch && task.BranchRename.Reason == r.Reason {
		return PrepareTaskBranchRenameResult{Task: task, RenameID: task.BranchRename.RenameID}, nil
	}
	if task.CurrentNode.Terminal() {
		return PrepareTaskBranchRenameResult{}, domain.ErrTaskTerminal
	}
	if task.CurrentNode == domain.NodeBlocked || task.Revision != r.ExpectedRevision {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrRevisionConflict, "Resolve the current blocker and use the current Task revision before preparing a branch rename.")
	}
	pending, err := s.hasPendingActionOperation(ctx, task)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	if pending {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrRecoveryUnavailable, "Recover the pending Action operation before preparing a branch rename.")
	}
	binding, exists := task.RepositoryBindingForKey(r.RepositoryKey)
	if !exists || binding.Detached || binding.CurrentBranch == nil || *binding.CurrentBranch == r.TargetBranch {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "Select one bound repository and a different target branch.")
	}
	rename := domain.TaskBranchRename{RepositoryKey: r.RepositoryKey, SourceBranch: *binding.CurrentBranch, TargetBranch: r.TargetBranch, Reason: r.Reason, ResumeNode: task.CurrentNode, PreparedAt: s.now().UTC()}
	observed, err := s.observeBranchRenameScope(ctx, task, rename, "cancel")
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	if scopeHasUnavailableWorkspace(task, observed.scope) || scopeHasHistoryConflict(observed.scope) {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The workspace instance or history changed before branch-rename preparation.")
	}
	if task.TaskPlan != nil && len(task.UnexplainedChangedPaths(observed.scope.Primary, observed.scope.Additional)) != 0 {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrRepositoryDrift, "Resolve the changed-file scope before preparing a branch rename.")
	}
	workspace, err := scopeWorkspaceDigests(task, observed.scope)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	oldWorkspace, _ := task.EffectiveWorkspaceDigests()
	if workspace.Content != oldWorkspace.Content && task.CurrentNode != domain.NodeImplement && task.CurrentNode != domain.NodeRefactor {
		return PrepareTaskBranchRenameResult{}, domain.WithExplanation(domain.ErrRepositoryDrift, "Repository content must match the current node authority before branch rename.")
	}
	next, err := cloneProcessTask(task)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, domain.ErrInternal
	}
	rename.RenameID, err = s.id("rename")
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	rename.Repositories = observed.repositories
	rebindProcessAuthorities(&next, observed.scope)
	if workspace.Content != oldWorkspace.Content {
		next.Implementation, next.Test, next.Comprehension = nil, nil, nil
	}
	if err := consumeFileScopeAuthorizations(&next, observedTaskDeltaPaths(task, observed.scope), task.CurrentAction.ActionID); err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	blockerID, err := s.id("blocker")
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	actionID, err := s.id("action")
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	eventID, err := s.id("event")
	if err != nil {
		return PrepareTaskBranchRenameResult{}, err
	}
	resume := task.CurrentNode
	next.CurrentNode, next.ResumeNode, next.BranchRename = domain.NodeBlocked, &resume, &rename
	next.Revision++
	next.UpdatedAt = rename.PreparedAt
	next.Blocker = &domain.ProcessBlocker{BlockerID: blockerID, Code: domain.ErrorTaskBlocked, Cause: domain.BlockerCauseTaskBranchRenamePending, Message: "One pure branch rename is prepared for the authorized Host.", ResumeNode: resume, ObservedBindingDigest: workspace.Binding, Condition: domain.BlockerCondition{Kind: domain.BlockerConditionResolveBranchRename, RenameID: rename.RenameID, ExpectedBindingDigest: workspace.Binding, ExpectedIdentityDigest: workspace.Identity, ExpectedHistoryDigest: workspace.History, ExpectedContentDigest: workspace.Content}, RequiredResolution: "Execute one non-force git branch -m, then complete; cancel only while all original facts remain unchanged.", CreatedAt: rename.PreparedAt}
	action, err := workflow.BuildProcessActionForWorkspace(workflow.StandardProcess(), domain.NodeBlocked, next.TaskID, next.Revision, workspace, next.Intent.MethodProfile, actionID, rename.PreparedAt)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, domain.ErrInternal
	}
	next.CurrentAction = &action
	digest, err := digestCanonical(rename)
	if err != nil {
		return PrepareTaskBranchRenameResult{}, domain.ErrInternal
	}
	next.LastOperation = &domain.LastOperation{OperationID: r.RequestID, Kind: domain.OperationPrepareTaskBranchRename, FromRevision: task.Revision, ToRevision: next.Revision, PayloadDigest: digest, CommittedAt: rename.PreparedAt}
	event := store.TaskEvent{EventID: eventID, TaskID: task.TaskID, Revision: next.Revision, Kind: domain.OperationPrepareTaskBranchRename, SourceNode: resume, DestinationNode: domain.NodeBlocked, TransitionReason: r.Reason, ObservedBindingDigest: &workspace.Binding, RequestID: r.RequestID, PayloadDigest: digest, CreatedAt: rename.PreparedAt}
	if err := s.taskStore.CommitTask(ctx, store.TaskMutation{ExpectedRevision: task.Revision, Task: next, Event: event, Claim: store.ClaimRetain}); err != nil {
		return PrepareTaskBranchRenameResult{}, mapStoreError(err)
	}
	return PrepareTaskBranchRenameResult{Task: next, RenameID: rename.RenameID}, nil
}

type branchRenameObservation struct {
	scope        recovery.RepositoryScopeObservation
	repositories []domain.BranchRenameRepository
}

func (s *Service) observeBranchRenameScope(ctx context.Context, task domain.ProcessTask, rename domain.TaskBranchRename, choice string) (branchRenameObservation, error) {
	observer, ok := s.repositoryObserver.(repository.BranchRenameObserver)
	if !ok {
		return branchRenameObservation{}, domain.WithExplanation(domain.ErrInternal, "The repository observer cannot verify branch names and index facts.")
	}
	if choice != "complete" && choice != "cancel" {
		return branchRenameObservation{}, domain.WithExplanation(domain.ErrInvalidArgument, "rename_choice must be complete or cancel.")
	}
	read := func() (branchRenameObservation, error) {
		result := branchRenameObservation{}
		entries := append([]domain.RepositoryScopeEntry{{Key: task.EffectivePrimaryRepositoryKey(), Origin: task.WorkspaceOrigin, Binding: task.Repository}}, task.AdditionalRepositories...)
		for i, entry := range entries {
			previous := entry.Binding.Clone()
			source, target := "", ""
			if previous.CurrentBranch != nil {
				source = *previous.CurrentBranch
			}
			selected := entry.Key == rename.RepositoryKey
			if selected {
				source, target = rename.SourceBranch, rename.TargetBranch
				if choice == "complete" {
					previous.CurrentBranch = &target
				}
			}
			origin, binding, facts, err := observer.ObserveBranchRename(ctx, entry.Origin.CanonicalWorktreeRoot, persistedOriginSelection(entry.Origin), previous, source, target)
			if err != nil {
				return result, mapWorkspaceObservationError(err)
			}
			if origin != entry.Origin {
				return result, domain.ErrWorkspaceUnavailable
			}
			if selected {
				valid := !binding.Detached && binding.CurrentBranch != nil
				if choice == "complete" {
					valid = valid && *binding.CurrentBranch == target && facts.SourceHead == "" && facts.TargetHead == binding.CurrentHead
				} else {
					valid = valid && *binding.CurrentBranch == source && facts.SourceHead == binding.CurrentHead && facts.TargetHead == ""
				}
				if !valid {
					return result, domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The exact source and target refs do not prove the selected pure branch rename or its cancellation.")
				}
			}
			result.repositories = append(result.repositories, domain.BranchRenameRepository{Key: entry.Key, BindingDigest: binding.BindingDigest, IndexDigest: facts.IndexDigest})
			if i == 0 {
				result.scope.Primary = binding
			} else {
				result.scope.Additional = append(result.scope.Additional, domain.RepositoryScopeEntry{Key: entry.Key, Origin: entry.Origin, Binding: binding})
			}
		}
		sort.Slice(result.repositories, func(i, j int) bool { return result.repositories[i].Key < result.repositories[j].Key })
		return result, nil
	}
	first, err := read()
	if err != nil {
		return first, err
	}
	second, err := read()
	if err != nil {
		return second, err
	}
	if !slices.Equal(first.repositories, second.repositories) {
		return branchRenameObservation{}, domain.ErrWorkspaceObservationUnstable
	}
	if task.BranchRename != nil {
		for i, retained := range task.BranchRename.Repositories {
			if second.repositories[i].Key != retained.Key || second.repositories[i].IndexDigest != retained.IndexDigest {
				return branchRenameObservation{}, domain.WithExplanation(domain.ErrRepositoryDrift, "A repository index changed after branch-rename preparation.")
			}
		}
		if err := recovery.ValidateBranchRenameBindings(task, second.scope, choice); err != nil {
			return branchRenameObservation{}, err
		}
	}
	return second, nil
}

func (s *Service) resolveTaskBranchRename(ctx context.Context, request RecoverActionRequest, requestID domain.ID, task domain.ProcessTask, retained json.RawMessage) (ApplyActionResult, error) {
	if task.BranchRename == nil || request.RenameID != task.BranchRename.RenameID || request.FileScopeDecision != nil || request.HistoryResolution != nil || request.RelocationID != "" || len(request.RelocationDestinations) != 0 {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrInvalidArgument, "Branch rename requires the saved rename_id and no other decision types.")
	}
	fresh, err := s.observeBranchRenameScope(ctx, task, *task.BranchRename, request.RenameChoice)
	if err != nil {
		return ApplyActionResult{}, err
	}
	workspace, err := scopeWorkspaceDigests(task, fresh.scope)
	if err != nil {
		return ApplyActionResult{}, err
	}
	payload := domain.BlockerResolutionPayload{BlockerID: task.Blocker.BlockerID, Condition: task.Blocker.Condition, ObservedBindingDigest: workspace.Binding, RenameID: request.RenameID, RenameChoice: request.RenameChoice}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ApplyActionResult{}, domain.ErrInternal
	}
	_, canonical, err := workflow.DecodeBlockerResolutionPayload(raw)
	if err != nil {
		return ApplyActionResult{}, err
	}
	if len(retained) != 0 && !slices.Equal([]byte(retained), []byte(canonical)) {
		return ApplyActionResult{}, domain.WithExplanation(domain.ErrRepositoryDrift, "The saved rename decision no longer matches the exact repository facts.")
	}
	apply := applyRequestForCurrentAction(requestID, request.Host, task, canonical)
	mutation, err := s.planResolveBlockerMutation(apply, task, fresh.scope, payload, canonical)
	if err != nil {
		return ApplyActionResult{}, err
	}
	operationStore, ok := s.taskStore.(store.ActionOperationStore)
	if !ok {
		return ApplyActionResult{}, domain.ErrInternal
	}
	commit := domain.ActionCommit{Operation: operationFromApply(apply), Payload: canonical, PayloadDigest: mutation.Task.LastOperation.PayloadDigest, PreparedAt: mutation.Task.UpdatedAt}
	if len(retained) == 0 {
		if err := operationStore.StageActionOperation(ctx, task, commit); err != nil {
			return ApplyActionResult{}, mapStoreError(err)
		}
	}
	if err := operationStore.CommitActionOperation(ctx, requestID, mutation); err != nil {
		return ApplyActionResult{}, mapStoreError(err)
	}
	return ApplyActionResult{Task: mutation.Task}, nil
}

func validBranchRenameReason(reason string) bool {
	return utf8.ValidString(reason) && strings.TrimSpace(reason) == reason && reason != "" && len(reason) <= domain.MaxReasonBytes
}
