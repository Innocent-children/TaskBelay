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
	if len(os.Args) > 2 && os.Args[1] == "webui" && os.Args[2] == "serve" {
		switch os.Getenv("TASKBELAY_WEBUI_STARTUP_FIXTURE") {
		case "exit":
			fmt.Fprintln(os.Stderr, "isolated fixture: storage was unavailable before serving")
			os.Exit(7)
		case "initializing":
			root := os.Getenv("TASKBELAY_DATA_DIR")
			file := filepath.Join(root, databaseFileName)
			// Keep a new SQLite file visible before its schema is committed.
			if err := os.WriteFile(file, nil, 0600); err != nil {
				panic(err)
			}
			time.Sleep(500 * time.Millisecond)
			if err := os.Remove(file); err != nil {
				panic(err)
			}
			if err := Serve(context.Background(), root, "startup-fixture"); err != nil {
				panic(err)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func TestStartWaitsForChildStorageInitialization(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TASKBELAY_WEBUI_STARTUP_FIXTURE", "initializing")
	t.Setenv("TASKBELAY_DATA_DIR", root)
	t.Cleanup(func() {
		if _, err := Stop(context.Background(), root, "startup-fixture"); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := Start(ctx, root, "startup-fixture", true)
	if err != nil || state.Readiness != ReadinessReady {
		t.Fatal("unfinished child storage was classified before service readiness", state, err)
	}
	state, err = Status(ctx, root, "startup-fixture")
	if err != nil || state.Readiness != ReadinessReady {
		t.Fatal("initialized service is unavailable", state, err)
	}
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
