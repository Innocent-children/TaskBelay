package gitstate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestDiscoverSharedRepositoryAndSeparateWorktrees(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "主仓库 space")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "--quiet")
	git(t, repo, "-c", "user.name=TaskBelay test", "-c", "user.email=taskbelay@example.invalid", "-c", "core.hooksPath="+filepath.Join(base, "no-hooks"), "-c", "commit.gpgsign=false", "commit", "--quiet", "--allow-empty", "-m", "fixture")
	worktree := filepath.Join(base, "linked worktree")
	git(t, repo, "worktree", "add", "--quiet", "-b", "fixture-worktree", worktree)
	first, err := Discover(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(t.Context(), worktree)
	if err != nil {
		t.Fatal(err)
	}
	if first.CommonDir != second.CommonDir || first.GitDir == second.GitDir || first.Root == second.Root {
		t.Fatalf("incorrect worktree identity: %+v / %+v", first, second)
	}
	nested := filepath.Join(worktree, "child")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	fromChild, err := Discover(t.Context(), nested)
	if err != nil || fromChild != second {
		t.Fatalf("nested directory = %+v, err = %v", fromChild, err)
	}
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(base, "link")
		if err := os.Symlink(repo, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		throughLink, err := Discover(t.Context(), link)
		if err != nil || throughLink != first {
			t.Fatalf("symlink = %+v, err = %v", throughLink, err)
		}
	})
}

func TestDiscoverNonGitAndCanceledContext(t *testing.T) {
	dir := t.TempDir()
	if _, err := Discover(t.Context(), dir); !errors.Is(err, ErrNotWorkspace) {
		t.Fatalf("non-git error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Discover(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled discovery = %v", err)
	}
}

func TestDiscoverPathsContainingNewlines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not allow newline characters in directory names")
	}
	dir := filepath.Join(t.TempDir(), "line\nbreak")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "--quiet")
	w, err := Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil || w.Root != root || w.GitDir != filepath.Join(root, ".git") || w.CommonDir != w.GitDir {
		t.Fatalf("newline path = %+v, root = %q, error = %v", w, root, err)
	}
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	fromChild, err := Discover(t.Context(), child)
	if err != nil || fromChild != w {
		t.Fatalf("nested newline path = %+v, error = %v", fromChild, err)
	}
}
