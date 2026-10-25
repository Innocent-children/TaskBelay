package testrun

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

func nativeScriptRequest(t *testing.T, contents map[string]string) (*Executor, Request) {
	t.Helper()
	tools := os.Getenv("TASKBELAY_TEST_TOOLS")
	if tools == "" {
		t.Skip("原生框架验证需要 TASKBELAY_TEST_TOOLS 指向独立工具环境；不自动安装项目依赖")
	}
	return nativeToolRequest(t, contents, tools)
}

func nativeToolRequest(t *testing.T, contents map[string]string, tools string) (*Executor, Request) {
	t.Helper()
	root, output := t.TempDir(), t.TempDir()
	var sources []codescan.Source
	for path, data := range contents {
		target := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, codescan.Source{Path: path, Data: []byte(data), Mode: "100644"})
	}
	snapshot := codescan.Analyze(t.Context(), "fixed", sources)
	request := Request{Snapshot: snapshot, Config: map[string]Config{}, Requirements: map[string][]Line{}, Changed: map[string][]Line{}}
	for _, file := range snapshot.Files {
		for _, source := range file.Tests {
			request.Cases = append(request.Cases, Case{Source: source, At: source.Location(), Name: source.Name, Change: "add", AgentOnly: true, Requirements: []string{"R1"}})
		}
	}
	slices.SortFunc(request.Cases, func(a, b Case) int { return a.Source.Span.StartLine - b.Source.Span.StartLine })
	return &Executor{Root: root, Output: output, Workspace: tools, Env: os.Environ()}, request
}

func TestPythonNativeNodeIDsCoverageAndFailureKinds(t *testing.T) {
	executor, request := nativeScriptRequest(t, map[string]string{
		"src/limits.py": "def allowed(n):\n    return n < 5\n",
		"test_limits.py": `import pytest
import unittest
from limits import allowed
@pytest.mark.parametrize("n,want", [(1,True),(5,False)], ids=["inside","edge.[x]"])
def test_limit(n,want):
    assert allowed(n) == want
@pytest.mark.parametrize("n", [1], ids=lambda n: "runtime-" + str(n))
def test_dynamic(n):
    assert allowed(n)
class RangeChecks(unittest.TestCase):
    def test_allowed(self):
        self.assertTrue(allowed(1))
`,
	})
	request.Requirements["R1"] = []Line{{File: "src/limits.py", Number: 2}}
	runner := NewPythonRunner(executor, request)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	defer runner.Terminate()
	if result := runner.Prepare(ctx); !result.Passed() {
		t.Fatal(result)
	}
	if result := runner.Compile(ctx); !result.Passed() {
		t.Fatal(result)
	}
	cases, result := runner.Enumerate(ctx, request.Cases)
	if !result.Passed() || len(cases) != 4 {
		t.Fatal(result, cases)
	}
	var edge Case
	for _, test := range cases {
		run := runner.Run(ctx, test, "baseline")
		if !run.Passed() || !run.Coverage.Valid || !run.Coverage.CopyExecuted || !slices.Contains(run.Coverage.Lines, Line{File: "src/limits.py", Number: 2}) {
			t.Fatal(test, run)
		}
		if test.Name == "test_dynamic" && !test.Whole {
			t.Fatal("dynamic ids require whole test", test)
		}
		if test.Name == "test_limit[edge.[x]]" {
			edge = test
		}
	}
	if edge.Name == "" {
		t.Fatal(cases)
	}
	file := filepath.Join(executor.Root, "src", "limits.py")
	if err := os.WriteFile(file, []byte("def allowed(n):\n    return n <= 5\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if run := runner.Run(ctx, edge, "mutation"); run.Failure != AssertionFailure {
		t.Fatal(run)
	}
	if err := os.WriteFile(file, []byte("def allowed(n):\n    raise RuntimeError('infrastructure')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if run := runner.Run(ctx, edge, "mutation"); run.Failure != EnvironmentFailure {
		t.Fatal(run)
	}
	if err := os.WriteFile(file, []byte("def allowed(:\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := runner.Compile(ctx); result.Failure != CompileFailure {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(executor.Root, "src", "__pycache__")); !os.IsNotExist(err) {
		t.Fatal("source bytecode output", err)
	}
}
