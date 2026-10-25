package gitstate

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestObservedReadRechecksRawContentAndMode(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			dir := t.TempDir()
			git(t, dir, "init", "--quiet", "--object-format="+format)
			w, err := Discover(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			path := "nested/原始.txt"
			original := "before\r\n保留\x00\n"
			put(t, w, path, original)
			put(t, w, ".gitattributes", "*.txt text eol=lf\n")
			before, _, err := w.ReadFile(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			trace := filepath.Join(t.TempDir(), "read-trace.jsonl")
			t.Setenv("GIT_TRACE2_EVENT", trace)
			actual, content, err := w.ReadObservedFile(t.Context(), path, before)
			if err != nil || actual != before || string(content) != original {
				t.Fatalf("unchanged raw content: %+v %q %v", actual, content, err)
			}
			if _, err := os.Stat(trace); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unchanged retained blob started Git: %v", err)
			}
			full := filepath.Join(w.Root, filepath.FromSlash(path))
			info, err := os.Stat(full)
			if err != nil {
				t.Fatal(err)
			}
			replacement := strings.Replace(original, "before", "change", 1)
			put(t, w, path, replacement)
			if err := os.Chtimes(full, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			actual, content, err = w.ReadObservedFile(t.Context(), path, before)
			if err != nil || actual.Blob == before.Blob || string(content) != replacement {
				t.Fatalf("same size and timestamp hid update: %+v %q %v", actual, content, err)
			}
			stored, err := w.ReadBlob(t.Context(), actual.Blob)
			if err != nil || !bytes.Equal(stored, content) {
				t.Fatalf("changed raw evidence missing: %q %v", stored, err)
			}
			if runtime.GOOS != "windows" {
				if err := os.Chmod(full, 0o700); err != nil {
					t.Fatal(err)
				}
				modeChanged, _, err := w.ReadObservedFile(t.Context(), path, actual)
				if err != nil || modeChanged.Blob != actual.Blob || modeChanged.Mode != "100755" {
					t.Fatalf("mode update hidden: %+v %v", modeChanged, err)
				}
			}
			missing, content, err := w.ReadObservedFile(t.Context(), "deleted.txt", actual)
			if err != nil || missing != (File{}) || len(content) != 0 {
				t.Fatalf("old evidence resurrected missing file: %+v %q %v", missing, content, err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, _, err := w.ReadObservedFile(ctx, path, actual); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled read: %v", err)
			}
		})
	}
}

func TestObservedReadKeepsSymlinkIdentity(t *testing.T) {
	w := fixture(t)
	put(t, w, "ordinary", "target")
	observed, _, err := w.ReadFile(t.Context(), "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	put(t, w, "target", "destination content")
	if err := os.Symlink("target", filepath.Join(w.Root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	actual, content, err := w.ReadObservedFile(t.Context(), "link", observed)
	if err != nil || actual.Blob != observed.Blob || actual.Mode != "120000" || string(content) != "target" {
		t.Fatalf("symlink replaced with destination or old mode: %+v %q %v", actual, content, err)
	}
}

func TestTargetSpellingPreservesCaseAliasesAtWorkspaceBoundary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "CaseRepo")
	git(t, filepath.Dir(dir), "init", "--quiet", dir)
	w, err := Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	put(t, w, "CaseDir/Actual.txt", "value")
	if platform.DirectoryCaseRule(w.Root) != platform.CaseInsensitive {
		t.Skip("current volume is not confirmed case insensitive")
	}
	for _, input := range []string{
		filepath.Join(w.Root, "casedir", "actual.txt"),
		filepath.Join(filepath.Dir(w.Root), strings.ToUpper(filepath.Base(w.Root)), "casedir", "actual.txt"),
	} {
		got, err := w.TargetPath(input)
		if err != nil || got != "CaseDir/Actual.txt" {
			t.Fatalf("case alias %q: %q %v", input, got, err)
		}
	}
}
