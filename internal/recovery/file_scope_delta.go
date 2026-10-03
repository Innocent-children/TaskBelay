package recovery

import (
	"sort"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

// pendingWriteScopeDelta checks this decision against the saved observation.
// Unchanged carried content is not an authorization and remains subject to the
// ordinary Task surface guard. Compare every layer: staging can preserve the
// normalized content digest while changing the index.
func pendingWriteScopeDelta(task domain.ProcessTask, observed RepositoryScopeObservation) []string {
	paths := make([]string, 0)
	appendDelta := func(key domain.RepositoryKey, before, after domain.RepositoryBinding) {
		saved := make(map[string]domain.RepositoryChangedEntry, len(before.TaskSurface))
		for _, entry := range before.TaskSurface {
			saved[entry.Path] = entry
		}
		changed := make(map[string]bool)
		for _, entry := range after.TaskSurface {
			prior, exists := saved[entry.Path]
			if !exists || prior != entry {
				changed[entry.Path] = true
			}
			delete(saved, entry.Path)
		}
		for path := range saved {
			changed[path] = true
		}
		for path := range changed {
			if len(task.AdditionalRepositories) != 0 {
				path = string(key) + "::" + path
			}
			if !task.PathExpectedByCurrentPlan(path) && !task.PathRetainedAsProcessArtifact(path) {
				paths = append(paths, path)
			}
		}
	}
	appendDelta(task.EffectivePrimaryRepositoryKey(), task.Repository, observed.Primary)
	for index, repository := range task.AdditionalRepositories {
		appendDelta(repository.Key, repository.Binding, observed.Additional[index].Binding)
	}
	sort.Strings(paths)
	return paths
}

// A pending write cannot also accept a branch, commit or worktree change.
// CompareRepositoryScope validates repository membership before this predicate.
func pendingWriteRepositoriesUnchanged(task domain.ProcessTask, observed RepositoryScopeObservation) bool {
	same := func(saved, fresh domain.RepositoryBinding) bool {
		return saved.WorktreeInstanceDigest == fresh.WorktreeInstanceDigest &&
			saved.IdentityDigest == fresh.IdentityDigest &&
			saved.CurrentHead == fresh.CurrentHead &&
			saved.HeadTree == fresh.HeadTree &&
			saved.Detached == fresh.Detached &&
			saved.BaseCommitAncestor == fresh.BaseCommitAncestor &&
			saved.CurrentBranch != nil && fresh.CurrentBranch != nil &&
			*saved.CurrentBranch == *fresh.CurrentBranch
	}
	if !same(task.Repository, observed.Primary) {
		return false
	}
	for index, repository := range task.AdditionalRepositories {
		if !same(repository.Binding, observed.Additional[index].Binding) {
			return false
		}
	}
	return true
}
