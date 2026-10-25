package gitstate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) Workspace {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "--quiet")
	w, err := Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func put(t *testing.T, w Workspace, path, text string) {
	t.Helper()
	full := filepath.Join(w.Root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func capture(t *testing.T, w Workspace) Snapshot {
	t.Helper()
	s, err := w.Capture(t.Context())
	if err != nil || !s.Complete() {
		t.Fatalf("capture=%+v err=%v", s, err)
	}
	return s
}

func TestEmptyIndexSplitIndexAndIgnoredTrackedFiles(t *testing.T) {
	w := fixture(t)
	empty := capture(t, w)
	evidence, err := w.EvidenceTree(t.Context(), nil, empty.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Retain(t.Context(), "session-test", "empty", evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.GitDir, "index")); !os.IsNotExist(err) {
		t.Fatalf("capture created a real index: %v", err)
	}
	put(t, w, "tracked", "before\n")
	git(t, w.Root, "add", "tracked")
	put(t, w, ".gitignore", "tracked\nignored\n")
	put(t, w, "ignored", "secret")
	git(t, w.Root, "update-index", "--assume-unchanged", "tracked")
	git(t, w.Root, "update-index", "--skip-worktree", "tracked")
	git(t, w.Root, "update-index", "--split-index")
	before, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
	if err != nil {
		t.Fatal(err)
	}
	put(t, w, "tracked", "current\n")
	s := capture(t, w)
	after, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("split index changed")
	}
	out, err := w.command(t.Context(), nil, nil, "show", s.Tree+":tracked")
	if err != nil || string(out) != "current\n" {
		t.Fatalf("assume-unchanged concealed working content: %q %v", out, err)
	}
	if _, ok := s.Files["ignored"]; ok {
		t.Fatal("ignore rules changed")
	}
}

func TestCaptureRechecksSameStatContentAndKeepsRealIndexTimestamp(t *testing.T) {
	w := fixture(t)
	git(t, w.Root, "config", "core.trustctime", "false")
	git(t, w.Root, "config", "core.checkStat", "minimal")
	path := filepath.Join(w.Root, "tracked")
	stamp := time.Unix(1700000000, 0)
	setTime := func(path string) {
		t.Helper()
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	put(t, w, "tracked", "before\n")
	setTime(path)
	git(t, w.Root, "add", "tracked")
	index := filepath.Join(w.GitDir, "index")
	setTime(index)
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	put(t, w, "tracked", "edited\n")
	setTime(path)
	snapshot := capture(t, w)
	content, err := w.command(t.Context(), nil, nil, "show", snapshot.Tree+":tracked")
	if err != nil || string(content) != "edited\n" {
		t.Fatalf("same-stat edit was missed: %q %v", content, err)
	}
	after, err := os.ReadFile(index)
	info, statErr := os.Stat(index)
	if err != nil || statErr != nil || !bytes.Equal(before, after) || !info.ModTime().Equal(stamp) {
		t.Fatal("capture changed the real index bytes or timestamp", err, statErr)
	}
}

func TestCaptureReusesTreeObjectsAndKeepsFilteredRawContents(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "--quiet", "--object-format="+format)
			git(t, dir, "config", "core.autocrlf", "false")
			w, err := Discover(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			contents := map[string]string{"text.txt": "中文\n", "binary.bin": "\x00binary\xff\r\n", "empty.txt": ""}
			for name, content := range contents {
				put(t, w, name, content)
			}
			git(t, dir, "add", ".")
			index, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
			if err != nil {
				t.Fatal(err)
			}
			contents["text.txt"] = "用户改过的中文\n"
			put(t, w, "text.txt", contents["text.txt"])
			trace := filepath.Join(t.TempDir(), "git.jsonl")
			t.Setenv("GIT_TRACE2_EVENT", trace)
			writes := func() int {
				t.Helper()
				data, err := os.ReadFile(trace)
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, line := range bytes.Split(data, []byte{'\n'}) {
					if len(line) == 0 {
						continue
					}
					var event struct {
						Event string   `json:"event"`
						Argv  []string `json:"argv"`
					}
					if err := json.Unmarshal(line, &event); err != nil {
						t.Fatal(err)
					}
					if event.Event == "start" && strings.Contains(strings.Join(event.Argv, " "), "hash-object -w") {
						count++
					}
				}
				return count
			}
			first := capture(t, w)
			if count := writes(); count != 0 {
				t.Fatalf("tree contents were written again in %d hash-object processes", count)
			}
			for name, want := range contents {
				got, err := w.ReadBlob(t.Context(), first.Files[name].Blob)
				if err != nil || string(got) != want {
					t.Fatalf("raw %s content changed: %q %v", name, got, err)
				}
			}
			put(t, w, ".gitattributes", "filtered.txt text eol=lf\n")
			put(t, w, "filtered.txt", "first\r\nsecond\r\n")
			filtered := capture(t, w)
			if count := writes(); count != 1 {
				t.Fatalf("expected only the filtered raw blob to be written, got %d", count)
			}
			raw, err := w.ReadBlob(t.Context(), filtered.Files["filtered.txt"].Blob)
			if err != nil || string(raw) != "first\r\nsecond\r\n" {
				t.Fatal("filtered file lost its raw bytes", err)
			}
			normalized, err := w.command(t.Context(), nil, nil, "show", filtered.Tree+":filtered.txt")
			if err != nil || string(normalized) != "first\nsecond\n" {
				t.Fatal("tree lost Git's normalized content", err)
			}
			after, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
			if err != nil || !bytes.Equal(index, after) {
				t.Fatal("real index changed", err)
			}
			for name, want := range contents {
				got, err := os.ReadFile(filepath.Join(w.Root, name))
				if err != nil || string(got) != want {
					t.Fatal("user working content changed", name, err)
				}
			}
		})
	}
}

func TestTargetPathsResolveAliasesWithoutEscaping(t *testing.T) {
	w := fixture(t)
	put(t, w, "目录 Space/Actual.txt", "hello")
	path, err := w.TargetPath("目录 Space/new/sub.txt")
	if err != nil || path != "目录 Space/new/sub.txt" {
		t.Fatalf("new target %s %v", path, err)
	}
	if _, err := w.TargetPath("../outside"); err == nil {
		t.Fatal("outside target accepted")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(w.Root, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := w.TargetPath("external/new.txt"); err == nil {
		t.Fatal("external symlink target accepted")
	}
	if _, _, err := w.ReadFile(t.Context(), "external/file.txt"); err == nil {
		t.Fatal("raw read followed external directory")
	}
	if err := os.Symlink(filepath.Join(w.Root, "目录 Space"), filepath.Join(w.Root, "alias")); err != nil {
		t.Fatal(err)
	}
	path, err = w.TargetPath("alias/Actual.txt")
	if err != nil || path != "目录 Space/Actual.txt" {
		t.Fatalf("alias %s %v", path, err)
	}
	s := capture(t, w)
	if s.Files["external"].Mode != "120000" || s.Files["alias"].Mode != "120000" {
		t.Fatal("snapshot expanded symlinks")
	}
}

func TestFragmentsPreserveFinalNewlineAndRenameAmbiguity(t *testing.T) {
	w := fixture(t)
	before, err := w.WriteBlob(t.Context(), []byte("α\nold"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := w.WriteBlob(t.Context(), []byte("α\nnew\n"))
	if err != nil {
		t.Fatal(err)
	}
	fragments, err := w.Fragments(t.Context(), File{before, "100644"}, File{after, "100644"})
	if err != nil || len(fragments) != 1 || fragments[0].BeforeText != "old" || fragments[0].AfterText != "new\n" {
		t.Fatalf("fragments=%+v err=%v", fragments, err)
	}
	put(t, w, "a", "same\n")
	put(t, w, "b", "same\n")
	origin := capture(t, w)
	if err := os.Rename(filepath.Join(w.Root, "a"), filepath.Join(w.Root, "c")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(w.Root, "b"), filepath.Join(w.Root, "d")); err != nil {
		t.Fatal(err)
	}
	current := capture(t, w)
	changes, err := w.Diff(t.Context(), origin.Tree, current.Tree)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		if change.UniqueRename {
			t.Fatalf("ambiguous rename migrated automatically: %+v", change)
		}
	}
}

func TestSnapshotReferenceCannotMoveAndContentRemainsReachable(t *testing.T) {
	w := fixture(t)
	put(t, w, "file.txt", "origin")
	origin := capture(t, w)
	ref, err := w.Retain(t.Context(), "t1", "origin", origin.Tree)
	if err != nil {
		t.Fatal(err)
	}
	again, err := w.Retain(t.Context(), "t1", "origin", origin.Tree)
	if err != nil || ref != again {
		t.Fatalf("retention retry %v %v", again, err)
	}
	put(t, w, "file.txt", "current")
	current := capture(t, w)
	if _, err := w.Retain(t.Context(), "t1", "origin", current.Tree); err == nil {
		t.Fatal("origin ref moved")
	}
	if _, err := w.Retain(t.Context(), "../heads", "main", current.Tree); err == nil {
		t.Fatal("invalid owner accepted")
	}
	out, err := w.command(t.Context(), nil, nil, "fsck", "--no-reflogs", "--unreachable")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "unreachable tree "+origin.Tree) || strings.Contains(string(out), "unreachable blob "+origin.Files["file.txt"].Blob) {
		t.Fatalf("retained origin is unreachable: %s", out)
	}
}
