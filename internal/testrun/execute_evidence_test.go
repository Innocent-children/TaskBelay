package testrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestExecutorEvidenceHelper(t *testing.T) {
	mode := os.Getenv("TASKBELAY_EXECUTOR_EVIDENCE_HELPER")
	if mode == "" {
		return
	}
	_, _ = os.Stdout.WriteString("literal $HOME `text` 中文\n")
	if mode == "failed" {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestExecutorPersistsActualStartAndKeepsLegacyEvidenceUnknown(t *testing.T) {
	executor := &Executor{Root: t.TempDir(), Output: t.TempDir(), Env: append(os.Environ(), "GORACE=atexit_sleep_ms=0")}
	for _, mode := range []string{"passed", "failed", "missing", "canceled", "empty"} {
		args := []string{os.Args[0], "-test.run=^TestExecutorEvidenceHelper$"}
		ctx, cancel := context.WithCancel(t.Context())
		if mode == "missing" {
			args[0] = filepath.Join(executor.Root, "absent-program")
		} else if mode == "empty" {
			args = nil
		} else if mode == "canceled" {
			cancel()
		}
		result := executor.Execute(ctx, "baseline", executor.Root, args, []string{"TASKBELAY_EXECUTOR_EVIDENCE_HELPER=" + mode})
		cancel()
		started := mode == "passed" || mode == "failed"
		if result.Process == nil || result.Process.Started != started || result.Duration <= 0 {
			t.Fatal(mode, result)
		}
		if started && result.Output != "literal $HOME `text` 中文\n" {
			t.Fatal("output changed", result)
		}
		if !started && (result.Process.Resources != nil || result.ExitCode != -1 || result.Passed()) {
			t.Fatal("failed launch has execution evidence", result)
		}
		if mode == "failed" && (result.ExitCode != 7 || result.Passed()) {
			t.Fatal("native failure lost", result)
		}
	}
	data, err := json.Marshal(executor.Records())
	if err != nil {
		t.Fatal(err)
	}
	var restored []Stage
	if err := json.Unmarshal(data, &restored); err != nil || len(restored) != 5 {
		t.Fatal(string(data), err)
	}
	for i, stage := range restored {
		if stage.Name != "baseline" || stage.Result.Process == nil || stage.Result.Process.Started != (i < 2) {
			t.Fatal("stage process evidence did not round trip", stage)
		}
	}
	var legacy Result
	if err := json.Unmarshal([]byte(`{"exit_code":0,"duration_ns":100}`), &legacy); err != nil || legacy.Process != nil {
		t.Fatal("legacy result was assigned synthetic execution evidence", legacy, err)
	}
}

func TestConcurrentExecutorStagesKeepIndependentProcessEvidence(t *testing.T) {
	executor := &Executor{Root: t.TempDir(), Output: t.TempDir(), Env: append(os.Environ(), "GORACE=atexit_sleep_ms=0")}
	var wait sync.WaitGroup
	for _, stage := range []string{"prepare", "compile", "baseline", "mutation"} {
		wait.Go(func() {
			executor.Execute(t.Context(), stage, executor.Root, []string{os.Args[0], "-test.run=^TestExecutorEvidenceHelper$"}, []string{"TASKBELAY_EXECUTOR_EVIDENCE_HELPER=passed"})
		})
	}
	wait.Wait()
	stages := executor.Records()
	seen := map[string]bool{}
	for _, stage := range stages {
		if seen[stage.Name] || !stage.Result.Passed() || stage.Result.Process == nil || !stage.Result.Process.Started {
			t.Fatal("concurrent command records were mixed", stages)
		}
		seen[stage.Name] = true
	}
	if len(seen) != 4 {
		t.Fatal(stages)
	}
}
