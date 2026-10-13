package webui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("TASKBELAY_WEBUI_STARTUP_FIXTURE") == "exit" && len(os.Args) > 2 && os.Args[1] == "webui" && os.Args[2] == "serve" {
		fmt.Fprintln(os.Stderr, "isolated fixture: storage was unavailable before serving")
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func TestStartReportsActualChildExitAndRetainsOriginalLog(t *testing.T) {
	t.Setenv("TASKBELAY_WEBUI_STARTUP_FIXTURE", "exit")
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, e = Start(ctx, root, "0.23.0", true)
	if e == nil || !strings.Contains(e.Error(), "process exited before readiness") || !strings.Contains(e.Error(), filepath.Join(root, "webui-runtime.log")) {
		t.Fatal("exit was mislabeled as a readiness/connection timeout", e)
	}
	log, e := os.ReadFile(filepath.Join(root, "webui-runtime.log"))
	if e != nil || !strings.Contains(string(log), "storage was unavailable") {
		t.Fatal("original child diagnostic was lost", e)
	}
}
