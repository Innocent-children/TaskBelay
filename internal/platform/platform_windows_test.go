package platform

import "testing"

func TestCodexHookCommandQuotesPowerShellPaths(t *testing.T) {
	got := CodexHookCommand(`C:\用户 O'Brian ‘x’\taskbelay.exe`, "stop")
	want := `& 'C:\用户 O''Brian ‘‘x’’\taskbelay.exe' hook codex 'stop'`
	if got != want {
		t.Fatalf("hook command = %q, want %q", got, want)
	}
}
