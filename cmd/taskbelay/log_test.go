package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestPruneLogs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-10-17.log", "2026-10-18.log", "2026-10-24.log", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	pruneLogs(dir, time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local))

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	// 保留 10-18 到 10-24 这 7 天，10-17 被删除，不是日志的文件不动。
	if want := []string{"2026-10-18.log", "2026-10-24.log", "notes.txt"}; !slices.Equal(left, want) {
		t.Fatalf("剩下 %v，期望 %v", left, want)
	}
}
