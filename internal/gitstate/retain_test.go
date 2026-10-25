package gitstate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitStreamingInputHonorsCancellationAndCallerOwnership(t *testing.T) {
	w := fixture(t)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := w.command(ctx, reader, nil, "update-ref", "--stdin")
		done <- err
	}()
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("waiting Git did not return cancellation", err)
		}
	case <-time.After(5 * time.Second):
		writer.Close()
		<-done
		t.Fatal("waiting stdin prevented cancellation")
	}
	if _, err := reader.Stat(); err != nil {
		t.Fatal("Git command closed its caller's input", err)
	}
}

func TestRetainConcurrentCreateKeepsOneSnapshotAndUserIndex(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "--quiet", "--object-format="+format)
			w, err := Discover(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			put(t, w, "file.txt", "staged\n")
			git(t, dir, "add", "file.txt")
			first := capture(t, w)
			put(t, w, "file.txt", "working\n")
			second := capture(t, w)
			index := filepath.Join(w.GitDir, "index")
			before, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			type outcome struct {
				retained Retained
				err      error
			}
			results := make(chan outcome, 2)
			for _, tree := range []string{first.Tree, second.Tree} {
				go func() {
					retained, err := w.Retain(ctx, "task-test", "shared", tree)
					results <- outcome{retained, err}
				}()
			}
			var winner Retained
			succeeded := 0
			for range 2 {
				result := <-results
				if result.err == nil {
					winner = result.retained
					succeeded++
				} else if errors.Is(result.err, context.DeadlineExceeded) {
					t.Fatal("concurrent retention did not terminate", result.err)
				}
			}
			if succeeded != 1 {
				t.Fatalf("different trees claimed the same ref: %d successes", succeeded)
			}
			again, err := w.Retain(t.Context(), "task-test", "shared", winner.Tree)
			if err != nil || again != winner {
				t.Fatal("retry did not keep the winning commit", again, err)
			}
			actual, err := w.command(t.Context(), nil, nil, "rev-parse", winner.Ref+"^{tree}")
			if err != nil || strings.TrimSpace(string(actual)) != winner.Tree {
				t.Fatal("reference no longer selects the winning tree", err)
			}
			after, err := os.ReadFile(index)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("retention changed the user's index", err)
			}
			content, err := os.ReadFile(filepath.Join(w.Root, "file.txt"))
			if err != nil || string(content) != "working\n" {
				t.Fatal("retention changed user content", err)
			}
			if err := w.ReleaseSnapshot(t.Context(), winner.Ref); err != nil {
				t.Fatal("retained commit is incompatible with cleanup", err)
			}
		})
	}
}

func TestRetainCommitFailureLeavesNoReference(t *testing.T) {
	w := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	blob, err := w.WriteBlob(ctx, []byte("not a tree"))
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range []string{blob, strings.Repeat("1", len(blob))} {
		retained, err := w.Retain(ctx, "task-test", "failed", object)
		if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "创建快照提交") {
			t.Fatal("invalid tree was accepted or left the reference process waiting", err)
		}
		if _, err := w.command(t.Context(), nil, nil, "rev-parse", "--verify", "--quiet", retained.Ref); !isExit(err, 1) {
			t.Fatal("failed commit published a reference", err)
		}
	}
	snapshot := capture(t, w)
	canceled, stop := context.WithCancel(t.Context())
	stop()
	retained, err := w.Retain(canceled, "task-test", "canceled", snapshot.Tree)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("retention ignored cancellation", err)
	}
	if _, err := w.command(t.Context(), nil, nil, "rev-parse", "--verify", "--quiet", retained.Ref); !isExit(err, 1) {
		t.Fatal("canceled call published a reference", err)
	}
}
