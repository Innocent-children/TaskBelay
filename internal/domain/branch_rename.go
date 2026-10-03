package domain

import "time"

type BranchRenameRepository struct {
	Key           RepositoryKey `json:"key"`
	BindingDigest Digest        `json:"binding_digest"`
	IndexDigest   Digest        `json:"index_digest"`
}

// TaskBranchRename freezes one explicit rename and every participating repository.
// The Task's ordinary bindings retain the source facts until resolution commits.
type TaskBranchRename struct {
	RenameID      ID                       `json:"rename_id"`
	RepositoryKey RepositoryKey            `json:"repository_key"`
	SourceBranch  string                   `json:"source_branch"`
	TargetBranch  string                   `json:"target_branch"`
	Reason        string                   `json:"reason"`
	Repositories  []BranchRenameRepository `json:"repositories"`
	ResumeNode    NodeID                   `json:"resume_node"`
	PreparedAt    time.Time                `json:"prepared_at"`
}

func ValidTaskBranchName(branch string) bool { return validBranchName(branch) && branch != "HEAD" }

func (r TaskBranchRename) Validate() error {
	if !r.RenameID.IsValid() || !r.RepositoryKey.IsValid() || !ValidTaskBranchName(r.SourceBranch) || !ValidTaskBranchName(r.TargetBranch) || r.SourceBranch == r.TargetBranch || requireNormalizedText(r.Reason, MaxReasonBytes, true) != nil || !r.ResumeNode.Normal() || validateUTC(r.PreparedAt) != nil || len(r.Repositories) < 1 || len(r.Repositories) > MaxRepositoryScopeEntries {
		return ErrInvalidArgument
	}
	found := false
	for i, entry := range r.Repositories {
		if !entry.Key.IsValid() || !entry.BindingDigest.IsValid() || !entry.IndexDigest.IsValid() || i > 0 && r.Repositories[i-1].Key >= entry.Key {
			return ErrInvalidArgument
		}
		found = found || entry.Key == r.RepositoryKey
	}
	if !found {
		return ErrInvalidArgument
	}
	return nil
}

func (t ProcessTask) BranchRenameValid() bool {
	if t.BranchRename == nil {
		return t.Blocker == nil || t.Blocker.Cause != BlockerCauseTaskBranchRenamePending
	}
	r := t.BranchRename
	if r.Validate() != nil || t.CurrentNode != NodeBlocked || t.Blocker == nil || t.ResumeNode == nil || t.Relocation != nil || t.Blocker.Cause != BlockerCauseTaskBranchRenamePending || t.Blocker.Condition.RenameID != r.RenameID || *t.ResumeNode != r.ResumeNode || !t.Blocker.CreatedAt.Equal(r.PreparedAt) || len(r.Repositories) != len(t.AdditionalRepositories)+1 {
		return false
	}
	for _, entry := range r.Repositories {
		binding, exists := t.RepositoryBindingForKey(entry.Key)
		if !exists || entry.BindingDigest != binding.BindingDigest || binding.Detached || binding.CurrentBranch == nil || entry.Key == r.RepositoryKey && *binding.CurrentBranch != r.SourceBranch {
			return false
		}
	}
	return true
}

func (t ProcessTask) RepositoryBindingForKey(key RepositoryKey) (RepositoryBinding, bool) {
	if key == t.EffectivePrimaryRepositoryKey() {
		return t.Repository, true
	}
	for _, entry := range t.AdditionalRepositories {
		if entry.Key == key {
			return entry.Binding, true
		}
	}
	return RepositoryBinding{}, false
}
