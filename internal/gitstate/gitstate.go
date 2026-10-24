// Package gitstate 通过 git 命令读取工作区：找到工作区根目录，算出工作区状态，
// 保存和删除起点快照，算出两个状态之间的 diff。
package gitstate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotWorkspace 表示目录不在 git 工作区里。
var ErrNotWorkspace = errors.New("不在 git 工作区里")

// Root 返回 dir 所在 git 工作区的根目录，符号链接已解析。dir 不在 git 工作区里时
// 返回 ErrNotWorkspace：git rev-parse 以 128 退出，就是 git 认为这里不能当工作区用，
// 比如不是仓库、在 .git 目录里，或者仓库的所有者不被信任。
func Root(dir string) (string, error) {
	out, err := git(dir, nil, "rev-parse", "--show-toplevel")
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		return "", ErrNotWorkspace
	}
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(filepath.FromSlash(strings.TrimSpace(out)))
}

// State 算出工作区状态：工作区里文件内容的 git tree 哈希，包括没提交的改动和
// 没被 .gitignore 忽略的新文件。做法是把 git 索引复制一份，在副本上 git add -A
// 再 git write-tree。用户真正的索引保持原样。
func State(root string) (string, error) {
	indexPath, err := git(root, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return "", err
	}
	indexPath = filepath.FromSlash(strings.TrimSpace(indexPath))
	if !filepath.IsAbs(indexPath) {
		indexPath = filepath.Join(root, indexPath)
	}

	tmpDir, err := os.MkdirTemp("", "taskbelay-index-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)
	tmpIndex := filepath.Join(tmpDir, "index")

	// 仓库还没有索引时（比如刚 git init），副本从空的开始。
	if err := copyFile(indexPath, tmpIndex); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	env := []string{"GIT_INDEX_FILE=" + tmpIndex}
	if _, err := git(root, env, "add", "-A"); err != nil {
		return "", err
	}
	tree, err := git(root, env, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(tree), nil
}

// snapshotIdentity 是给快照包 commit 时用的作者信息，不依赖用户的 git 配置。
var snapshotIdentity = []string{
	"GIT_AUTHOR_NAME=TaskBelay",
	"GIT_AUTHOR_EMAIL=taskbelay@localhost",
	"GIT_COMMITTER_NAME=TaskBelay",
	"GIT_COMMITTER_EMAIL=taskbelay@localhost",
}

// SaveSnapshot 给工作区状态 tree 包一个 commit，用 refs/taskbelay/<name> 指向它。
// 这样快照不会被 git gc 清掉，也不会出现在分支列表里。
func SaveSnapshot(root, name, tree string) error {
	commit, err := git(root, snapshotIdentity, "commit-tree", "--no-gpg-sign", "-m", "TaskBelay snapshot "+name, tree)
	if err != nil {
		return err
	}
	_, err = git(root, nil, "update-ref", snapshotRef(name), strings.TrimSpace(commit))
	return err
}

// DeleteSnapshot 删除 refs/taskbelay/<name>，快照之后由 git 按自己的规则清理。
func DeleteSnapshot(root, name string) error {
	_, err := git(root, nil, "update-ref", "-d", snapshotRef(name))
	return err
}

func snapshotRef(name string) string {
	return "refs/taskbelay/" + name
}

// git 在 dir 里运行一条 git 命令，返回标准输出。env 追加在当前环境变量之后。
func git(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s：%w：%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
