package testrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPythonRejectsOriginalWorkspaceImports(t *testing.T) {
	tools := os.Getenv("TASKBELAY_TEST_TOOLS")
	if tools == "" {
		t.Skip("需要独立原生 Python 环境")
	}
	original := t.TempDir()
	if err := os.Symlink(filepath.Join(tools, ".venv"), filepath.Join(original, ".venv")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(original, "logic.py"), []byte("def allowed():\n    return True\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path, _ := json.Marshal(original)
	executor, request := nativeScriptRequest(t, map[string]string{"logic.py": "def allowed():\n    return False\n", "test_logic.py": "import sys\nsys.path.insert(0," + string(path) + ")\nfrom logic import allowed\ndef test_value():\n    assert allowed()\n"})
	executor.Workspace = original
	runner := NewPythonRunner(executor, request)
	defer runner.Terminate()
	if result := runner.Prepare(t.Context()); !result.Passed() {
		t.Fatal(result)
	}
	cases, result := runner.Enumerate(t.Context(), request.Cases)
	if !result.Passed() || len(cases) != 1 {
		t.Fatal(result, cases)
	}
	run := runner.Run(t.Context(), cases[0], "baseline")
	if run.Failure != CoverageFailure || run.Coverage.CopyExecuted {
		t.Fatal("external original passed as copy", run)
	}
}

func TestScriptCancellationAndInvalidCoverage(t *testing.T) {
	executor, request := nativeScriptRequest(t, map[string]string{"test_wait.py": "import time\ndef test_wait():\n    time.sleep(10)\n    assert True\n"})
	runner := NewPythonRunner(executor, request)
	defer runner.Terminate()
	if result := runner.Prepare(t.Context()); !result.Passed() {
		t.Fatal(result)
	}
	cases, result := runner.Enumerate(t.Context(), request.Cases)
	if !result.Passed() || len(cases) != 1 {
		t.Fatal(result, cases)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if run := runner.Run(ctx, cases[0], "baseline"); run.Failure != Timeout {
		t.Fatal(run)
	}
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	if run := runner.Run(ctx, cases[0], "baseline"); run.Failure != Canceled {
		t.Fatal(run)
	}
	profile := filepath.Join(executor.Output, "bad.json")
	for _, data := range []string{"{}", "null", "not json"} {
		if err := os.WriteFile(profile, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		coverage := runner.ReadCoverage(t.Context(), RunResult{Profile: profile})
		if coverage.Valid || coverage.CopyExecuted {
			t.Fatal(data, coverage)
		}
	}
}

func TestMissingScriptToolsStayUnavailable(t *testing.T) {
	executor, request := nativeScriptRequest(t, map[string]string{"package.json": "{\"private\":true}\n", "basic.test.js": "test('ok',()=>{expect(true).toBe(true)});\n"})
	executor.Workspace = ""
	runner := NewJavaScriptRunner(executor, request)
	defer runner.Terminate()
	if result := runner.Prepare(t.Context()); result.Failure != EnvironmentFailure {
		t.Fatal(result)
	}
	tools := os.Getenv("TASKBELAY_TEST_TOOLS")
	modules := filepath.Join(executor.Root, "node_modules")
	if err := os.MkdirAll(filepath.Join(modules, ".bin"), 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(".bin", "vitest")
	if runtime.GOOS == "windows" {
		bin += ".cmd"
	}
	for _, name := range []string{"vitest", bin} {
		if err := os.Symlink(filepath.Join(tools, "node_modules", name), filepath.Join(modules, name)); err != nil {
			t.Fatal(err)
		}
	}
	request.Config["javascript"] = Config{Command: "vitest"}
	withoutCoverage := NewJavaScriptRunner(executor, request)
	defer withoutCoverage.Terminate()
	if result := withoutCoverage.Prepare(t.Context()); result.Failure != EnvironmentFailure || !strings.Contains(result.Reason, "coverage-v8") {
		t.Fatal("missing coverage plugin", result)
	}
}
