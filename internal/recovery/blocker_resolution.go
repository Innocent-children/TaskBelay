package recovery

import (
	"github.com/Innocent-children/taskbelay/internal/domain"
	"slices"
)

/**
 * Ordinary submission and recovery validate the same saved decision against
 * the same repository observation. Accepting history preserves its real relation.
 */
func ValidateBlockerResolution(task domain.ProcessTask, observed RepositoryScopeObservation, comparison RepositoryScopeComparison, payload domain.BlockerResolutionPayload) error {
	blocker := task.Blocker
	if blocker == nil || task.CurrentNode != domain.NodeBlocked || task.ResumeNode == nil ||
		blocker.Cause == domain.BlockerCauseTaskRelocationPending ||
		payload.RelocationID != "" || len(payload.RelocationDestinations) != 0 {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task has no resolvable ordinary blocker, or relocation input was supplied to the ordinary blocker path.")
	}
	if blocker.Cause == domain.BlockerCauseTaskBranchRenamePending {
		if task.BranchRename == nil || payload.RenameID != task.BranchRename.RenameID || payload.FileScopeDecision != nil || payload.HistoryResolution != nil || payload.BlockerID != blocker.BlockerID || payload.Condition != blocker.Condition || payload.ObservedBindingDigest != comparison.ObservedDigest {
			return domain.ErrInvalidArgument
		}
		return ValidateBranchRenameBindings(task, observed, payload.RenameChoice)
	}
	if payload.RenameID != "" || payload.RenameChoice != "" {
		return domain.ErrInvalidArgument
	}
	fileScope := blocker.Cause == domain.BlockerCauseFileScopeDecision
	history := blocker.Cause == domain.BlockerCauseWorkspaceHistoryConflict
	if fileScope != (payload.FileScopeDecision != nil) || history != (payload.HistoryResolution != nil) {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The supplied decision type does not match the current file-scope or history blocker.")
	}
	if fileScope {
		if payload.FileScopeDecision.Validate() != nil {
			return domain.WithExplanation(domain.ErrInvalidArgument, "The file-scope decision requires a supported choice and a normalized non-empty reason.")
		}
		var pending *domain.FileScopeRecord
		for i := range task.FileScopeRecords {
			record := &task.FileScopeRecords[i]
			if record.RequestID == blocker.Condition.ScopeRequestID && record.Decision == domain.FileScopePending {
				pending = record
				break
			}
		}
		if pending == nil {
			return domain.WithExplanation(domain.ErrInvalidArgument, "No pending file-scope record matches the blocker scope_request_id.")
		}
		unexplained := task.UnexplainedChangedPaths(observed.Primary, observed.Additional)
		if !pending.Observed {
			if !pendingWriteRepositoriesUnchanged(task, observed) {
				return domain.WithExplanation(domain.ErrRepositoryDrift, "A pending file write cannot accept a changed worktree, branch or HEAD.")
			}
			unexplained = pendingWriteScopeDelta(task, observed)
		}
		if comparison.Relation == RepositoryForbiddenChange || !containsEveryPath(pending.Paths, unexplained) ||
			payload.FileScopeDecision.Choice == domain.FileScopeReject && len(unexplained) != 0 {
			return domain.WithExplanation(domain.ErrRepositoryDrift, "The repository changed outside the pending file-scope paths, or rejected paths are still changed.")
		}
	} else if history {
		if payload.HistoryResolution.Validate() != nil {
			return domain.WithExplanation(domain.ErrInvalidArgument, "The history decision requires accept_current_history and a normalized non-empty reason.")
		}
		for _, fact := range comparison.Repositories {
			if fact.Reason == RepositoryReasonWorktreeInstance {
				return domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "A history decision cannot accept a replacement worktree instance.")
			}
		}
		if !scopeAtRetainedBranch(task, observed) ||
			comparison.ObservedDigest != blocker.ObservedBindingDigest &&
				comparison.ObservedDigest != blocker.Condition.ExpectedBindingDigest &&
				comparison.Relation == RepositoryForbiddenChange {
			return domain.WithExplanation(domain.ErrWorkspaceHistoryConflict, "The observed history does not match the prepared history or an allowed restoration.")
		}
	} else if comparison.Relation != RepositoryExact {
		return domain.WithExplanation(domain.ErrRepositoryDrift, "The repository must be restored to the retained state before this blocker can be resolved.")
	}
	if payload.BlockerID != blocker.BlockerID || payload.Condition != blocker.Condition ||
		payload.ObservedBindingDigest != comparison.ObservedDigest {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The submitted blocker identity, condition or observed binding digest differs from the current blocker and observation.")
	}
	return nil
}

func scopeAtRetainedBranch(task domain.ProcessTask, observed RepositoryScopeObservation) bool {
	match := func(previous domain.RepositoryBinding, binding domain.RepositoryBinding) bool {
		return !binding.Detached && binding.CurrentBranch != nil && previous.CurrentBranch != nil && *binding.CurrentBranch == *previous.CurrentBranch && binding.BaseCommitAncestor
	}
	if !match(task.Repository, observed.Primary) || len(task.AdditionalRepositories) != len(observed.Additional) {
		return false
	}
	for i, entry := range task.AdditionalRepositories {
		if !match(entry.Binding, observed.Additional[i].Binding) {
			return false
		}
	}
	return true
}

// ValidateBranchRenameBindings allows only the selected current_branch field to
// change. Ref existence and the complete logical index are checked by the caller.
func ValidateBranchRenameBindings(task domain.ProcessTask, observed RepositoryScopeObservation, choice string) error {
	if task.BranchRename == nil || choice != "complete" && choice != "cancel" || len(task.AdditionalRepositories) != len(observed.Additional) {
		return domain.ErrInvalidArgument
	}
	matches := func(key domain.RepositoryKey, before, after domain.RepositoryBinding) bool {
		if before.CurrentBranch == nil || after.CurrentBranch == nil {
			return false
		}
		branch := *before.CurrentBranch
		if key == task.BranchRename.RepositoryKey && choice == "complete" {
			branch = task.BranchRename.TargetBranch
		}
		return *after.CurrentBranch == branch && before.Detached == after.Detached &&
			before.WorktreeInstanceDigest == after.WorktreeInstanceDigest && before.IdentityDigest == after.IdentityDigest &&
			before.HistoryDigest == after.HistoryDigest && before.ContentDigest == after.ContentDigest &&
			before.CurrentHead == after.CurrentHead && before.HeadTree == after.HeadTree && before.BaseCommitAncestor == after.BaseCommitAncestor &&
			slices.Equal(before.ChangedEntries, after.ChangedEntries) && slices.Equal(before.TaskSurface, after.TaskSurface) &&
			(after.HistoryRelation == domain.RepositoryHistoryExact || after.HistoryRelation == domain.RepositoryHistoryLinearAdvance)
	}
	if !matches(task.EffectivePrimaryRepositoryKey(), task.Repository, observed.Primary) {
		return domain.ErrRepositoryDrift
	}
	for i, before := range task.AdditionalRepositories {
		after := observed.Additional[i]
		if before.Key != after.Key || before.Origin != after.Origin || !matches(before.Key, before.Binding, after.Binding) {
			return domain.ErrRepositoryDrift
		}
	}
	return nil
}
