package gitstate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

func gitEnvironment(extra []string) []string {
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE":
			continue
		}
		env = append(env, item)
	}
	return append(append(env, "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_LITERAL_PATHSPECS=1"), extra...)
}

func (w Workspace) command(ctx context.Context, input io.Reader, env []string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	program, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "taskbelay-git-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		return nil, err
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		return nil, err
	}
	defer stderr.Close()
	// 调用方提供的文件或管道直接交给子进程；保留其所有权，避免预读阻塞共同截止时间。
	stdin, streaming := input.(*os.File)
	if !streaming {
		stdin, err = os.Create(filepath.Join(dir, "stdin"))
		if err != nil {
			return nil, err
		}
		defer stdin.Close()
		if input != nil {
			if _, err := io.Copy(stdin, contextReader{ctx, input}); err != nil {
				return nil, err
			}
			if _, err := stdin.Seek(0, io.SeekStart); err != nil {
				return nil, err
			}
		}
	}
	arguments := append([]string{"-C", w.Root, "-c", "core.fsmonitor=false", "-c", "core.splitIndex=false", "-c", "gc.auto=0"}, args...)
	code, _, runErr := platform.RunProcess(ctx, platform.ProcessCommand{Path: program, Args: arguments, Dir: w.Root, Env: gitEnvironment(env), Stdin: stdin, Stdout: stdout, Stderr: stderr})
	out, readErr := os.ReadFile(stdout.Name())
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if runErr != nil || readErr != nil {
		return out, errors.Join(runErr, readErr)
	}
	if code != 0 {
		diagnostic, _ := os.ReadFile(stderr.Name())
		if len(diagnostic) > 64*1024 {
			diagnostic = diagnostic[:64*1024]
		}
		return out, &gitExitError{Code: code, Operation: args[0], Stderr: string(diagnostic)}
	}
	return out, nil
}

type gitExitError struct {
	Code              int
	Operation, Stderr string
}

func (e *gitExitError) Error() string {
	return fmt.Sprintf("git %s: exit status %d: %s", e.Operation, e.Code, strings.TrimSpace(e.Stderr))
}

type Context struct {
	Branch string
	HEAD   string
}

func (w Workspace) Context(ctx context.Context) (Context, error) {
	var result Context
	out, err := w.command(ctx, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if !isExit(err, 1) {
			return result, err
		}
	} else {
		result.Branch = strings.TrimSpace(string(out))
	}
	out, err = w.command(ctx, nil, nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		if !isExit(err, 1) {
			return result, err
		}
	} else {
		result.HEAD = strings.TrimSpace(string(out))
	}
	return result, nil
}
