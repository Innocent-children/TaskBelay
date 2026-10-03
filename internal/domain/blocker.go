package domain

import (
	"sort"
	"time"
)

type BlockerConditionKind string

const (
	BlockerConditionRestoreIssuanceBinding BlockerConditionKind = "restore_issuance_binding"
	BlockerConditionAllowVerificationRetry BlockerConditionKind = "allow_verification_retry"
	BlockerConditionResolveFileScope       BlockerConditionKind = "resolve_file_scope"
	BlockerConditionResolveHistory         BlockerConditionKind = "resolve_workspace_history"
	BlockerConditionResolveRelocation      BlockerConditionKind = "resolve_task_relocation"
	BlockerConditionResolveBranchRename    BlockerConditionKind = "resolve_task_branch_rename"
)

func (k BlockerConditionKind) IsValid() bool {
	return k == BlockerConditionRestoreIssuanceBinding || k == BlockerConditionAllowVerificationRetry || k == BlockerConditionResolveFileScope || k == BlockerConditionResolveHistory || k == BlockerConditionResolveRelocation || k == BlockerConditionResolveBranchRename
}

type BlockerCause string

const (
	BlockerCauseRecoveryPartiallyCompleted      BlockerCause = BlockerCause(RecoveryPartiallyCompleted)
	BlockerCauseRecoveryConflicting             BlockerCause = BlockerCause(RecoveryConflicting)
	BlockerCauseRepeatedVerificationFailure     BlockerCause = "repeated_verification_failure"
	BlockerCauseUnchangedVerificationResult     BlockerCause = "unchanged_verification_result"
	BlockerCauseUnchangedTestImplementationLoop BlockerCause = "unchanged_test_implementation_loop"
	BlockerCauseFileScopeDecision               BlockerCause = "file_scope_decision"
	BlockerCauseWorkspaceHistoryConflict        BlockerCause = "workspace_history_conflict"
	BlockerCauseTaskRelocationPending           BlockerCause = "task_relocation_pending"
	BlockerCauseTaskBranchRenamePending         BlockerCause = "task_branch_rename_pending"
)

func (c BlockerCause) IsVerificationBrake() bool {
	return c == BlockerCauseRepeatedVerificationFailure ||
		c == BlockerCauseUnchangedVerificationResult ||
		c == BlockerCauseUnchangedTestImplementationLoop
}

func (c BlockerCause) IsValid() bool {
	return c == BlockerCauseRecoveryPartiallyCompleted || c == BlockerCauseRecoveryConflicting || c.IsVerificationBrake() || c == BlockerCauseFileScopeDecision || c == BlockerCauseWorkspaceHistoryConflict || c == BlockerCauseTaskRelocationPending || c == BlockerCauseTaskBranchRenamePending
}

type BlockerCondition struct {
	Kind                   BlockerConditionKind `json:"kind"`
	ExpectedBindingDigest  Digest               `json:"expected_binding_digest"`
	ScopeRequestID         ID                   `json:"scope_request_id,omitempty"`
	RelocationID           ID                   `json:"relocation_id,omitempty"`
	RenameID               ID                   `json:"rename_id,omitempty"`
	ExpectedIdentityDigest Digest               `json:"expected_identity_digest"`
	ExpectedHistoryDigest  Digest               `json:"expected_history_digest"`
	ExpectedContentDigest  Digest               `json:"expected_content_digest"`
}

type RelocationDestination struct {
	Key            RepositoryKey `json:"key"`
	RepositoryPath string        `json:"repository_path"`
}

type WorkspaceHistoryResolutionInput struct {
	Choice string `json:"choice"`
	Reason string `json:"reason"`
}

// WorkspaceHistoryRepositorySnapshot retains the bounded Git facts a user
// reviews before accepting a history rewrite or merge.
type WorkspaceHistoryRepositorySnapshot struct {
	RepositoryKey      RepositoryKey             `json:"repository_key"`
	HistoryRelation    RepositoryHistoryRelation `json:"history_relation"`
	CurrentBranch      *string                   `json:"current_branch"`
	Detached           bool                      `json:"detached"`
	CurrentHead        string                    `json:"current_head"`
	BaseCommitAncestor bool                      `json:"base_commit_ancestor"`
	ContentDigest      Digest                    `json:"content_digest"`
	TaskSurface        []string                  `json:"task_surface"`
}

func (s WorkspaceHistoryRepositorySnapshot) Validate() error {
	if !s.RepositoryKey.IsValid() || !s.HistoryRelation.IsValid() ||
		s.Detached != (s.CurrentBranch == nil) || validateObjectID(s.CurrentHead) != nil ||
		!s.ContentDigest.IsValid() || len(s.TaskSurface) > MaxFingerprintPaths {
		return ErrInvalidArgument
	}
	if s.CurrentBranch != nil && requireNormalizedText(*s.CurrentBranch, MaxRepositoryPathBytes, true) != nil {
		return ErrInvalidArgument
	}
	for index, path := range s.TaskSurface {
		if validateRepositoryRelativePath(path) != nil || index > 0 && s.TaskSurface[index-1] >= path {
			return ErrInvalidArgument
		}
	}
	return nil
}

// WorkspaceHistorySnapshot is separate from the issuance condition: it is the
// exact conflicting workspace state presented for review.
type WorkspaceHistorySnapshot struct {
	BindingDigest       Digest                               `json:"binding_digest"`
	IdentityDigest      Digest                               `json:"identity_digest"`
	HistoryDigest       Digest                               `json:"history_digest"`
	ContentDigest       Digest                               `json:"content_digest"`
	CurrentChangedPaths []string                             `json:"current_changed_paths"`
	Repositories        []WorkspaceHistoryRepositorySnapshot `json:"repositories"`
	ObservedAt          time.Time                            `json:"observed_at"`
}

func (s WorkspaceHistorySnapshot) Validate() error {
	if !s.BindingDigest.IsValid() || !s.IdentityDigest.IsValid() ||
		!s.HistoryDigest.IsValid() || !s.ContentDigest.IsValid() ||
		len(s.CurrentChangedPaths) > MaxFingerprintPaths ||
		len(s.Repositories) == 0 || len(s.Repositories) > MaxRepositoryScopeEntries ||
		validateUTC(s.ObservedAt) != nil {
		return ErrInvalidArgument
	}
	for index, path := range s.CurrentChangedPaths {
		if ValidateRepositoryContractPath(path) != nil || index > 0 && s.CurrentChangedPaths[index-1] >= path {
			return ErrInvalidArgument
		}
	}
	conflict := false
	previous := RepositoryKey("")
	for _, repository := range s.Repositories {
		if repository.Validate() != nil || previous != "" && repository.RepositoryKey <= previous {
			return ErrInvalidArgument
		}
		if repository.HistoryRelation != RepositoryHistoryExact && repository.HistoryRelation != RepositoryHistoryLinearAdvance {
			conflict = true
		}
		previous = repository.RepositoryKey
	}
	if !conflict {
		return ErrInvalidArgument
	}
	return nil
}

func (i WorkspaceHistoryResolutionInput) Validate() error {
	if i.Choice != "accept_current_history" || requireNormalizedText(i.Reason, MaxReasonBytes, true) != nil {
		return ErrInvalidArgument
	}
	return nil
}

type BlockerResolutionPayload struct {
	BlockerID              ID                               `json:"blocker_id"`
	Condition              BlockerCondition                 `json:"condition"`
	ObservedBindingDigest  Digest                           `json:"observed_binding_digest"`
	FileScopeDecision      *FileScopeDecisionInput          `json:"file_scope_decision,omitempty"`
	RelocationID           ID                               `json:"relocation_id,omitempty"`
	RelocationDestinations []RelocationDestination          `json:"relocation_destinations,omitempty"`
	HistoryResolution      *WorkspaceHistoryResolutionInput `json:"history_resolution,omitempty"`
	RenameID               ID                               `json:"rename_id,omitempty"`
	RenameChoice           string                           `json:"rename_choice,omitempty"`
}

func (c BlockerCondition) Validate() error {
	if !c.Kind.IsValid() || validateDigest(c.ExpectedBindingDigest) != nil || !c.ExpectedIdentityDigest.IsValid() || !c.ExpectedHistoryDigest.IsValid() || !c.ExpectedContentDigest.IsValid() {
		return ErrInvalidArgument
	}
	if c.Kind == BlockerConditionResolveFileScope {
		if !c.ScopeRequestID.IsValid() {
			return ErrInvalidArgument
		}
	} else if c.ScopeRequestID != "" {
		return ErrInvalidArgument
	}
	if c.Kind == BlockerConditionResolveBranchRename {
		if !c.RenameID.IsValid() {
			return ErrInvalidArgument
		}
	} else if c.RenameID != "" {
		return ErrInvalidArgument
	}
	if c.Kind == BlockerConditionResolveRelocation {
		if !c.RelocationID.IsValid() {
			return ErrInvalidArgument
		}
	} else if c.RelocationID != "" {
		return ErrInvalidArgument
	}
	return nil
}

type ProcessBlocker struct {
	BlockerID                ID                        `json:"blocker_id"`
	Code                     ErrorCode                 `json:"code"`
	Cause                    BlockerCause              `json:"cause"`
	Message                  string                    `json:"message"`
	ResumeNode               NodeID                    `json:"resume_node"`
	ObservedBindingDigest    Digest                    `json:"observed_binding_digest"`
	ObservedWorkspaceHistory *WorkspaceHistorySnapshot `json:"observed_workspace_history,omitempty"`
	Condition                BlockerCondition          `json:"condition"`
	RequiredResolution       string                    `json:"required_resolution"`
	CreatedAt                time.Time                 `json:"created_at"`
}

func (b ProcessBlocker) Validate() error {
	if validateID(b.BlockerID) != nil || b.Code != ErrorTaskBlocked ||
		!b.Cause.IsValid() ||
		!b.ResumeNode.Normal() ||
		requireNormalizedText(b.Message, MaxBlockerMessageBytes, true) != nil ||
		validateDigest(b.ObservedBindingDigest) != nil || b.Condition.Validate() != nil ||
		requireNormalizedText(b.RequiredResolution, MaxResolutionTextBytes, true) != nil ||
		validateUTC(b.CreatedAt) != nil {
		return ErrInvalidArgument
	}
	expectedCondition := BlockerConditionRestoreIssuanceBinding
	if b.Cause.IsVerificationBrake() {
		expectedCondition = BlockerConditionAllowVerificationRetry
	} else if b.Cause == BlockerCauseFileScopeDecision {
		expectedCondition = BlockerConditionResolveFileScope
	} else if b.Cause == BlockerCauseWorkspaceHistoryConflict {
		expectedCondition = BlockerConditionResolveHistory
	} else if b.Cause == BlockerCauseTaskRelocationPending {
		expectedCondition = BlockerConditionResolveRelocation
	} else if b.Cause == BlockerCauseTaskBranchRenamePending {
		expectedCondition = BlockerConditionResolveBranchRename
	}
	if b.Condition.Kind != expectedCondition {
		return ErrInvalidArgument
	}
	if b.Cause == BlockerCauseWorkspaceHistoryConflict {
		if b.ObservedWorkspaceHistory == nil || b.ObservedWorkspaceHistory.Validate() != nil ||
			b.ObservedWorkspaceHistory.BindingDigest != b.ObservedBindingDigest ||
			b.ObservedWorkspaceHistory.ObservedAt.After(b.CreatedAt) {
			return ErrInvalidArgument
		}
	} else if b.ObservedWorkspaceHistory != nil {
		return ErrInvalidArgument
	}
	return nil
}

func workspaceHistorySnapshotMatchesTask(task ProcessTask) bool {
	if task.Blocker == nil || task.Blocker.ObservedWorkspaceHistory == nil {
		return false
	}
	snapshot := task.Blocker.ObservedWorkspaceHistory
	expectedKeys := make([]RepositoryKey, 0, len(task.AdditionalRepositories)+1)
	expectedKeys = append(expectedKeys, task.EffectivePrimaryRepositoryKey())
	for _, repository := range task.AdditionalRepositories {
		expectedKeys = append(expectedKeys, repository.Key)
	}
	sort.Slice(expectedKeys, func(i, j int) bool {
		return expectedKeys[i] < expectedKeys[j]
	})
	if len(expectedKeys) != len(snapshot.Repositories) {
		return false
	}
	for index, key := range expectedKeys {
		if snapshot.Repositories[index].RepositoryKey != key {
			return false
		}
	}

	paths := make([]string, 0, len(snapshot.CurrentChangedPaths))
	multipleRepositories := len(snapshot.Repositories) > 1
	for _, repository := range snapshot.Repositories {
		prefix := ""
		if multipleRepositories {
			prefix = string(repository.RepositoryKey) + repositoryPathSeparator
		}
		for _, path := range repository.TaskSurface {
			paths = append(paths, prefix+path)
		}
	}
	sort.Strings(paths)
	return sameStrings(paths, snapshot.CurrentChangedPaths)
}
