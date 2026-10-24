//go:build windows

package platform

import "testing"

// 路径里的单引号和 PowerShell 当作单引号的弯引号都要写两遍。
func TestCodexCommand(t *testing.T) {
	got := CodexCommand(`C:\Users\张 三's ‘x’\go\bin\taskbelay.exe`, "hook", "codex", "stop")
	want := `& 'C:\Users\张 三''s ‘‘x’’\go\bin\taskbelay.exe' hook codex stop`
	if got != want {
		t.Errorf("得到 %s\n应为 %s", got, want)
	}
}
