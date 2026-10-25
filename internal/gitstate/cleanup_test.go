package gitstate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReleaseOnlyOwnedSnapshotAndPreserveUserFiles(t *testing.T) {
	w := fixture(t)
	put(t, w, "source.txt", "user content")
	snapshot := capture(t, w)
	retained, err := w.Retain(t.Context(), "task-test", "o1", snapshot.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ReleaseSnapshot(t.Context(), retained.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := w.command(t.Context(), nil, nil, "rev-parse", "--verify", retained.Ref); err == nil {
		t.Fatal("reference survived")
	}
	if err := w.ReleaseSnapshot(t.Context(), retained.Ref); err != nil {
		t.Fatal("cleanup is not idempotent", err)
	}
	if err := w.ReleaseSnapshot(t.Context(), "refs/heads/main"); err == nil {
		t.Fatal("accepted user branch")
	}
	user, err := w.command(t.Context(), strings.NewReader("User commit\n"), []string{"GIT_AUTHOR_NAME=User", "GIT_AUTHOR_EMAIL=user@example.test", "GIT_COMMITTER_NAME=User", "GIT_COMMITTER_EMAIL=user@example.test"}, "-c", "commit.gpgsign=false", "commit-tree", snapshot.Tree)
	if err != nil {
		t.Fatal(err)
	}
	git(t, w.Root, "update-ref", retained.Ref, strings.TrimSpace(string(user)))
	if err := w.ReleaseSnapshot(t.Context(), retained.Ref); err == nil {
		t.Fatal("removed a user-owned reference")
	}
	if current := capture(t, w); current.Tree != snapshot.Tree {
		t.Fatal("cleanup changed workspace")
	}
}

func TestGitOutputCaptureUsesCommonDeadlineAndKeepsAllBytes(t *testing.T) {
	w := fixture(t)
	content := strings.Repeat("input and output\n", 100000)
	path := filepath.Join(w.Root, "large.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	object, err := w.command(t.Context(), strings.NewReader(content), nil, "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		out, err := w.command(t.Context(), nil, nil, "cat-file", "blob", strings.TrimSpace(string(object)))
		if err != nil || string(out) != content {
			t.Fatal("incomplete git output", len(out), err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	if _, err := w.command(ctx, nil, nil, "status", "--porcelain"); err != context.Canceled {
		t.Fatal(err)
	}
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("canceled command ignored budget")
	}
}
