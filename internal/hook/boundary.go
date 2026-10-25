package hook

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

/**
 * 仅在需要检查子仓库时建立联系。父仓库快照始终保留 gitlink 边界，
 * 子仓库中的内容由调用方使用返回的工作区单独观察。
 */
func (o Observer) LinkBoundary(ctx context.Context, task, path string) (gitstate.Workspace, error) {
	path, err := o.Workspace.TargetPath(path)
	if err != nil {
		return gitstate.Workspace{}, err
	}
	child, err := gitstate.Discover(ctx, filepath.Join(o.Workspace.Root, filepath.FromSlash(path)))
	if err != nil {
		return gitstate.Workspace{}, err
	}
	if child.ID == o.Workspace.ID {
		return gitstate.Workspace{}, fmt.Errorf("目标不是独立工作区")
	}
	if err := o.Store.RegisterGitWorkspace(ctx, child); err != nil {
		return gitstate.Workspace{}, err
	}
	if err := o.Store.LinkWorkspace(ctx, o.Workspace.ID, task, child.ID, path); err != nil {
		return gitstate.Workspace{}, err
	}
	return child, nil
}
