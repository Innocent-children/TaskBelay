package testrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

func TestGoRunnerCompilesCopyAndSelectsNativeSubtests(t *testing.T) {
	root, output := t.TempDir(), t.TempDir()
	contents := map[string]string{
		"go.mod":   "module example.test/range\n\ngo 1.26\n",
		"range.go": "package sample\nfunc Allowed(n int) bool { return n < 5 }\n",
		"range_test.go": `package sample
import "testing"
func TestRange(t *testing.T) {
 rows := []struct{name string; n int; want bool}{{"inside",1,true},{"edge.[x]",5,false}}
 for _, row := range rows {
  t.Run(row.name,func(t *testing.T){if Allowed(row.n)!=row.want {t.Errorf("result = %v",Allowed(row.n))}})
 }
}
func TestWhitespace(t *testing.T) {t.Run("two words",func(t *testing.T){if !Allowed(1){t.Fatal("false")}})}
`,
	}
	var sources []codescan.Source
	for path, data := range contents {
		if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, codescan.Source{Path: path, Data: []byte(data), Mode: "100644"})
	}
	snapshot := codescan.Analyze(t.Context(), "fixed", sources)
	request := Request{Snapshot: snapshot, Requirements: map[string][]Line{"R1": {{File: "range.go", Number: 2}}}}
	if runtime.GOOS != "windows" {
		request.Config = map[string]Config{"go": {Command: "env GOCACHE='" + filepath.Join(root, "must-not-create-cache") + "' go test ./..."}}
	}
	for _, source := range snapshot.Files["range_test.go"].Tests {
		request.Cases = append(request.Cases, Case{Source: source, At: source.Location(), Name: source.Name, Change: "add", AgentOnly: true, Requirements: []string{"R1"}})
	}
	executor := &Executor{Root: root, Output: output, Env: append(os.Environ(), "GOWORK=off")}
	runner := NewGoRunner(executor, request)
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	defer runner.Terminate()
	if result := runner.Prepare(ctx); !result.Passed() {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Join(root, "must-not-create-cache")); !os.IsNotExist(err) {
		t.Fatal("explicit env redirected cache to source", err)
	}
	if result := runner.Compile(ctx); !result.Passed() {
		t.Fatal(result)
	}
	cases, result := runner.Enumerate(ctx, request.Cases)
	if !result.Passed() || len(cases) != 3 {
		t.Fatal(result, cases)
	}
	for _, test := range cases {
		result := runner.Run(ctx, test, "baseline")
		if !result.Passed() || !result.Coverage.Valid || !result.Coverage.CopyExecuted || !slices.Contains(result.Coverage.Lines, Line{File: "range.go", Number: 2}) {
			t.Fatal(test, result)
		}
		if test.Name == "TestWhitespace" && (!test.Whole || len(test.Unknown) == 0) {
			t.Fatal("rewritten name should keep whole test", test)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "range.go"), []byte("package sample\nfunc Allowed(n int) bool { return n <= 5 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := runner.Compile(ctx); !result.Passed() {
		t.Fatal(result)
	}
	for _, test := range cases {
		if test.Name == "TestRange/edge.[x]" {
			result := runner.Run(ctx, test, "baseline")
			if result.Failure != AssertionFailure {
				t.Fatal(result)
			}
		}
	}
	compileCount := 0
	for _, stage := range executor.Records() {
		if stage.Name == "compile" {
			compileCount++
		}
	}
	if compileCount != 2 {
		t.Fatal("compiled per case instead of per source version", compileCount)
	}
}
