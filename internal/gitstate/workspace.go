package gitstate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

var ErrNotWorkspace = errors.New("当前目录不在 git 工作区里")

type Workspace struct {
	ID        string
	ProjectID string
	Root      string
	GitDir    string
	CommonDir string
}

func Discover(ctx context.Context, dir string) (Workspace, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Workspace{}, err
	}
	readPaths := func(flags ...string) (string, error) {
		output, err := (Workspace{Root: dir}).command(ctx, nil, nil, append([]string{"rev-parse"}, flags...)...)
		if err == nil {
			return strings.TrimSuffix(string(output), "\n"), nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		var exit *gitExitError
		if errors.As(err, &exit) && (strings.Contains(exit.Stderr, "not a git repository") || strings.Contains(exit.Stderr, "must be run in a work tree")) {
			return "", ErrNotWorkspace
		}
		return "", fmt.Errorf("读取 git 工作区: %w", err)
	}
	output, err := readPaths("--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return Workspace{}, err
	}
	paths := strings.Split(output, "\n")
	var workspace Workspace
	for i, item := range []struct {
		flag   string
		target *string
	}{
		{"--show-toplevel", &workspace.Root},
		{"--git-dir", &workspace.GitDir},
		{"--git-common-dir", &workspace.CommonDir},
	} {
		var path string
		if len(paths) == 3 {
			path = paths[i]
		} else {
			// rev-parse 没有 NUL 路径分隔；含换行的目录仍逐项读取。
			path, err = readPaths(item.flag)
			if err != nil {
				return Workspace{}, err
			}
		}
		path = strings.TrimSuffix(path, "\r")
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return Workspace{}, fmt.Errorf("解析 git 路径: %w", err)
		}
		*item.target = filepath.Clean(path)
	}
	workspace.ID, err = platform.FileIdentity(workspace.GitDir)
	if err != nil {
		return Workspace{}, err
	}
	workspace.ProjectID, err = platform.FileIdentity(workspace.CommonDir)
	if err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}
