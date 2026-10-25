package gitstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportUsesFixedTreeAndLeavesWorkspace(t *testing.T) {
	w := fixture(t)
	put(t, w, "pkg/source.go", "package pkg\nconst Value=1\n")
	put(t, w, "assets/input.txt", "fixed")
	snapshot := capture(t, w)
	put(t, w, "pkg/source.go", "user update")
	root := filepath.Join(t.TempDir(), "copy")
	if err := w.ExportTree(t.Context(), snapshot.Tree, root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "pkg/source.go"))
	if err != nil || string(data) != "package pkg\nconst Value=1\n" {
		t.Fatal(string(data), err)
	}
	current, _ := os.ReadFile(filepath.Join(w.Root, "pkg/source.go"))
	if string(current) != "user update" {
		t.Fatal("workspace changed")
	}
	if err := w.ExportTree(t.Context(), snapshot.Tree, root); err == nil {
		t.Fatal("overwrote an existing directory")
	}
}

func TestExportRejectsExternalSymlink(t *testing.T) {
	w := fixture(t)
	if err := os.Symlink("../../outside", filepath.Join(w.Root, "escape")); err != nil {
		t.Skip(err)
	}
	snapshot := capture(t, w)
	if err := w.ExportTree(t.Context(), snapshot.Tree, filepath.Join(t.TempDir(), "copy")); err == nil {
		t.Fatal("external link was exported")
	}
}
