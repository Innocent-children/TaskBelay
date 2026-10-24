package gitstate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStateFollowsWorkspaceContent(t *testing.T) {
	root := newRepo(t)
	indexBefore := readFile(t, root, ".git/index")

	base := mustState(t, root)
	if again := mustState(t, root); again != base {
		t.Fatalf("文件没变，状态却变了：%s → %s", base, again)
	}

	writeFile(t, root, "build/out.bin", "ignored")
	if s := mustState(t, root); s != base {
		t.Fatalf("改了被忽略的文件，状态不该变")
	}

	writeFile(t, root, "new.txt", "untracked")
	withNewFile := mustState(t, root)
	if withNewFile == base {
		t.Fatalf("加了没被忽略的新文件，状态应该变")
	}

	writeFile(t, root, "a.txt", "changed\n")
	if s := mustState(t, root); s == withNewFile {
		t.Fatalf("改了已跟踪的文件，状态应该变")
	}

	if !bytes.Equal(readFile(t, root, ".git/index"), indexBefore) {
		t.Fatalf("算工作区状态改动了用户的索引")
	}
}

func TestSnapshotRef(t *testing.T) {
	root := newRepo(t)
	// 新文件只存在于快照里，快照引用是它在 git 里唯一的引用。
	writeFile(t, root, "new.txt", "only in snapshot")
	tree := mustState(t, root)

	if err := SaveSnapshot(root, "t1", tree); err != nil {
		t.Fatal(err)
	}
	if branches := runGit(t, root, "branch", "--all"); strings.Contains(branches, "taskbelay") {
		t.Fatalf("快照出现在分支列表里：%s", branches)
	}
	runGit(t, root, "gc", "--quiet", "--prune=now")
	runGit(t, root, "cat-file", "-e", tree)

	if err := DeleteSnapshot(root, "t1"); err != nil {
		t.Fatal(err)
	}
	if refs := runGit(t, root, "for-each-ref", "refs/taskbelay/"); refs != "" {
		t.Fatalf("删除后引用还在：%s", refs)
	}
}

func TestDiff(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "a.txt", "1\n2\n3\n4\n5\n")
	writeFile(t, root, "old.txt", "gone\n")
	from := mustState(t, root)

	writeFile(t, root, "a.txt", "1\nTWO\n3\n4\n5\n6\n")
	if err := os.Remove(filepath.Join(root, "old.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "dir with space/新 文件.txt", "x\ny\n")
	to := mustState(t, root)

	got, err := Diff(root, from, to)
	if err != nil {
		t.Fatal(err)
	}
	want := []FileChange{
		{Path: "a.txt", Status: Modified, Hunks: []Hunk{{2, 1, 2, 1}, {5, 0, 6, 1}}},
		{Path: "dir with space/新 文件.txt", Status: Added, Hunks: []Hunk{{0, 0, 1, 2}}},
		{Path: "old.txt", Status: Deleted, Hunks: []Hunk{{1, 1, 0, 0}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Diff 结果不对\n得到 %+v\n期望 %+v", got, want)
	}
}

// newRepo 建一个有一次提交的仓库：已跟踪 a.txt，忽略 build/ 目录。
func newRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "test")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "commit.gpgSign", "false")
	writeFile(t, root, ".gitignore", "build/\n")
	writeFile(t, root, "a.txt", "one\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "--quiet", "-m", "init")
	return root
}

func mustState(t *testing.T, root string) string {
	t.Helper()
	s, err := State(root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s：%v：%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
