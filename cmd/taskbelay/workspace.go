package main

import (
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// findWorkspace 返回 dir 所在的 git 工作区。工作区一律取 git 给出的根目录，解析过符号链接，
// 比较时按这个平台的规则。dir 不在 git 工作区里时返回 gitstate.ErrNotWorkspace。
func findWorkspace(dir string) (ledger.Workspace, error) {
	root, err := gitstate.Root(dir)
	if err != nil {
		return ledger.Workspace{}, err
	}
	return ledger.Workspace{Path: root, Key: platform.PathKey(root)}, nil
}
