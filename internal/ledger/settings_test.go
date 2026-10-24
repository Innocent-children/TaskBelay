package ledger

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSettings(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "taskbelay.db")

	l := mustOpen(t, path)
	wantSetting(t, l, "workflow", "auto")
	wantSetting(t, l, "enforcement", "block")

	if err := l.SetSetting(ctx, "workflow", "semi"); err != nil {
		t.Fatal(err)
	}
	if err := l.SetSetting(ctx, "enforcement", "hint"); err != nil {
		t.Fatal(err)
	}
	if err := l.SetSetting(ctx, "workflow", "manual"); err == nil {
		t.Fatal("workflow 设成不允许的值，应该报错")
	}
	l.Close()

	// 重新打开账本，设置还在。
	l = mustOpen(t, path)
	defer l.Close()
	wantSetting(t, l, "workflow", "semi")
	wantSetting(t, l, "enforcement", "hint")
}

func mustOpen(t *testing.T, path string) *Ledger {
	t.Helper()
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func wantSetting(t *testing.T, l *Ledger, key, want string) {
	t.Helper()
	got, err := l.Setting(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s = %q，期望 %q", key, got, want)
	}
}
