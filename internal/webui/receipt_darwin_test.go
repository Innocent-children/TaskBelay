//go:build darwin

package webui

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestWebUILifecycleAcrossTerminalLocales(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TASKBELAY_WEBUI_STARTUP_FIXTURE", "initializing")
	t.Setenv("TASKBELAY_DATA_DIR", root)
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	state, err := Start(ctx, root, "startup-fixture", true)
	if err != nil || state.Readiness != ReadinessReady {
		t.Fatal(state, err)
	}
	t.Cleanup(func() { _, _ = Stop(context.Background(), root, "startup-fixture") })
	receipt, err := os.ReadFile(ReceiptPath(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"C", "en_US.UTF-8", "zh_CN.UTF-8"} {
		t.Setenv("LC_ALL", locale)
		status, err := Status(ctx, root, "startup-fixture")
		if err != nil || status.Readiness != ReadinessReady || status.PID != state.PID {
			t.Fatal(locale, status, err)
		}
		again, err := Start(ctx, root, "startup-fixture", true)
		if err != nil || again.PID != state.PID || again.URL != state.URL {
			t.Fatal("duplicated service", locale, again, err)
		}
		current, err := os.ReadFile(ReceiptPath(root))
		if err != nil || string(current) != string(receipt) {
			t.Fatal("runtime record changed", locale, err)
		}
	}
	t.Setenv("LC_ALL", "C")
	stopped, err := Stop(ctx, root, "startup-fixture")
	if err != nil || stopped.Readiness != ReadinessUnavailable {
		t.Fatal(stopped, err)
	}
}
