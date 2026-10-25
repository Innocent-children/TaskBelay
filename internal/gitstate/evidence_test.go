package gitstate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriteEvidenceTreeRetainsRawContentsAndKeepsUserState(t *testing.T) {
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
			put(t, w, "file.txt", "user working content\n")
			snapshot := capture(t, w)
			index, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
			if err != nil {
				t.Fatal(err)
			}
			contents := [][]byte{[]byte("\x00raw\xff\r\n中文"), {}, []byte("\x00raw\xff\r\n中文")}
			files := []File{snapshot.Files["file.txt"]}
			tree, blobs, err := w.WriteEvidenceTree(t.Context(), files, snapshot.Tree, contents)
			if err != nil || len(blobs) != len(contents) {
				t.Fatal("evidence contents were not saved", blobs, err)
			}
			if blobs[0] != blobs[2] || files[0] != snapshot.Files["file.txt"] {
				t.Fatal("duplicate contents or original files changed")
			}
			for i, blob := range blobs {
				data, err := w.ReadBlob(t.Context(), blob)
				if err != nil || !bytes.Equal(data, contents[i]) {
					t.Fatal("raw content changed", i, err)
				}
				files = append(files, File{Blob: blob, Mode: "100644"})
			}
			ordinary, err := w.EvidenceTree(t.Context(), files, snapshot.Tree)
			if err != nil || ordinary != tree {
				t.Fatal("pipelined and ordinary evidence trees differ", ordinary, tree, err)
			}
			retained, err := w.Retain(t.Context(), "task-test", "evidence", tree)
			if err != nil {
				t.Fatal(err)
			}
			out, err := w.command(t.Context(), nil, nil, "rev-list", "--objects", "--no-object-names", retained.Ref)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range append(blobs, snapshot.Tree, snapshot.Files["file.txt"].Blob) {
				if !strings.Contains("\n"+string(out), "\n"+id+"\n") {
					t.Fatal("retained reference cannot reach content", id)
				}
			}
			after, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
			if err != nil || !bytes.Equal(index, after) {
				t.Fatal("user index changed", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
			if err != nil || string(data) != "user working content\n" {
				t.Fatal("user working content changed", err)
			}
		})
	}
}

func TestWriteEvidenceTreeFailureReturnsAndDoesNotPublishReference(t *testing.T) {
	w := fixture(t)
	blob, err := w.WriteBlob(t.Context(), []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"invalid", strings.Repeat("1", len(blob))} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		tree, blobs, err := w.WriteEvidenceTree(ctx, []File{{Blob: id}}, "", [][]byte{[]byte("expected")})
		cancel()
		if err == nil || errors.Is(err, context.DeadlineExceeded) || tree != "" || len(blobs) != 0 {
			t.Fatal("invalid input was accepted or left a process waiting", tree, blobs, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := w.WriteEvidenceTree(ctx, nil, "", [][]byte{[]byte("canceled")}); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "写入证据 blob") || !strings.Contains(err.Error(), "构造证据 tree") {
		t.Fatal("cancellation lost", err)
	}
	out, err := w.command(t.Context(), nil, nil, "for-each-ref", "refs/taskbelay")
	if err != nil || len(out) != 0 {
		t.Fatal("failed evidence published a snapshot reference", string(out), err)
	}
}

func TestWriteEvidenceTreeStartsWaitingReaderBeforeBlobFinishes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process gate; native Windows pipe behavior requires separate verification")
	}
	w := fixture(t)
	program, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "reader-ready")
	script := `#!/bin/sh
for arg do
  case "$arg" in
    mktree) : > "$TASKBELAY_TEST_READER_READY" ;;
    hash-object)
      while [ ! -f "$TASKBELAY_TEST_READER_READY" ]; do sleep 0.01; done
      ;;
  esac
done
exec "$TASKBELAY_TEST_REAL_GIT" "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TASKBELAY_TEST_REAL_GIT", program)
	t.Setenv("TASKBELAY_TEST_READER_READY", marker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tree, blobs, err := w.WriteEvidenceTree(ctx, nil, "", [][]byte{[]byte("new content")})
	if err != nil || len(blobs) != 1 || tree == "" {
		t.Fatal("tree reader did not start while the blob was waiting", tree, blobs, err)
	}
}
