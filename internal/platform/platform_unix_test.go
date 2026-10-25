//go:build unix

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCodexHookCommandPreservesArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "用户 O'Brian tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(t.Context(), "sh", "-c", CodexHookCommand(path, "session-start")).CombinedOutput()
	if err != nil || string(output) != "hook\ncodex\nsession-start\n" {
		t.Fatalf("hook arguments = %q, err = %v", output, err)
	}
}
