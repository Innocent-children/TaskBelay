//go:build unix

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// 程序路径里有空格、中文和单引号时，bash 和 zsh 都能按 CodexCommand 写出的命令找到程序。
func TestCodexCommand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "张 三's")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, ExeName)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := CodexCommand(exe, "hook", "codex", "stop")
	for _, shell := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(shell); err != nil {
			continue
		}
		out, err := exec.Command(shell, "-c", command).CombinedOutput()
		if err != nil || string(out) != "hook codex stop\n" {
			t.Errorf("%s -c %s：%v，输出 %q", shell, command, err, out)
		}
	}
}
