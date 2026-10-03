package repository

import (
	"context"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

type BranchRenameFacts struct {
	IndexDigest domain.Digest
	SourceHead  string
	TargetHead  string
}

// BranchRenameObserver exposes only read-only index and exact local-ref facts.
type BranchRenameObserver interface {
	WorkspaceRepositoryObserver
	ObserveBranchRename(context.Context, string, WorkspaceOriginSelection, domain.RepositoryBinding, string, string) (domain.WorkspaceOrigin, domain.RepositoryBinding, BranchRenameFacts, error)
}

func (o *GitObserver) ObserveBranchRename(ctx context.Context, root string, selection WorkspaceOriginSelection, previous domain.RepositoryBinding, source, target string) (domain.WorkspaceOrigin, domain.RepositoryBinding, BranchRenameFacts, error) {
	readFacts := func() (BranchRenameFacts, error) {
		index, err := o.required(ctx, gitShowWholeIndex, root, "")
		if err != nil {
			return BranchRenameFacts{}, err
		}
		facts := BranchRenameFacts{IndexDigest: digestFields("taskbelay/branch-rename-index", index.stdout)}
		for _, ref := range []struct {
			name string
			head *string
		}{{source, &facts.SourceHead}, {target, &facts.TargetHead}} {
			if ref.name == "" {
				continue
			}
			result, err := o.runner.run(ctx, gitShowBranchRef, root, ref.name)
			if err != nil {
				return BranchRenameFacts{}, err
			}
			if result.exitCode == 1 {
				continue
			}
			if result.exitCode != 0 {
				return BranchRenameFacts{}, ErrGitObservation
			}
			*ref.head, err = parseObjectLine(result.stdout)
			if err != nil {
				return BranchRenameFacts{}, err
			}
		}
		return facts, nil
	}
	first, err := readFacts()
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, BranchRenameFacts{}, err
	}
	origin, binding, err := o.ObserveWorkspace(ctx, root, selection, &previous)
	if err != nil {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, BranchRenameFacts{}, err
	}
	second, err := readFacts()
	if err != nil || first != second {
		return domain.WorkspaceOrigin{}, domain.RepositoryBinding{}, BranchRenameFacts{}, ErrInconsistentWorktree
	}
	return origin, binding, second, nil
}
