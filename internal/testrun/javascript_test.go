package testrun

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestJavaScriptNativeFrameworkSelectionCoverageAndMutation(t *testing.T) {
	for _, tool := range []string{"jest", "vitest"} {
		t.Run(tool, func(t *testing.T) {
			production, testPath := "limits.js", "limits.test.js"
			contents := map[string]string{"package.json": "{\"private\":true}\n", "limits.js": "exports.allowed = n => n < 5;\n", "limits.test.js": `const {allowed} = require('./limits');
describe('range > literal', () => {
  test.each([['inside',1,true], ['edge.[x]',5,false]])('%s', (name,n,want) => {
    expect(allowed(n)).toBe(want);
  });
});
`}
			if tool == "vitest" {
				production, testPath = "limits.ts", "limits.test.ts"
				delete(contents, "limits.js")
				delete(contents, "limits.test.js")
				contents[production] = "export const allowed = (n: number): boolean => n < 5;\n"
				contents[testPath] = `import {describe,test,expect} from 'vitest';
import {allowed} from './limits';
describe('range > literal', () => {
  test.each([['inside',1,true], ['edge.[x]',5,false]])('%s', (name,n,want) => {
    expect(allowed(n as number)).toBe(want);
  });
});
`
			}
			executor, request := nativeScriptRequest(t, contents)
			request.Config["javascript"] = Config{Command: tool}
			request.Config["typescript"] = Config{Command: tool}
			request.Requirements["R1"] = []Line{{File: production, Number: 1}}
			runner := NewJavaScriptRunner(executor, request)
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
			defer cancel()
			defer runner.Terminate()
			if result := runner.Prepare(ctx); !result.Passed() {
				t.Fatal(result)
			}
			if result := runner.Compile(ctx); !result.Passed() {
				t.Fatal(result)
			}
			cases, result := runner.Enumerate(ctx, request.Cases)
			if !result.Passed() || len(cases) != 2 {
				t.Fatal(result, cases)
			}
			var edge Case
			for _, test := range cases {
				run := runner.Run(ctx, test, "baseline")
				if !run.Passed() || !run.Coverage.Valid || !slices.Contains(run.Coverage.Lines, Line{File: production, Number: 1}) {
					t.Fatal(test, run)
				}
				if strings.HasSuffix(test.Name, "edge.[x]") {
					edge = test
				}
			}
			if edge.Name == "" {
				t.Fatal(cases)
			}
			path := filepath.Join(executor.Root, production)
			if err := os.WriteFile(path, []byte(strings.Replace(contents[production], "n < 5", "n <= 5", 1)), 0600); err != nil {
				t.Fatal(err)
			}
			if run := runner.Run(ctx, edge, "mutation"); run.Failure != AssertionFailure {
				t.Fatal(run)
			}
			if err := os.WriteFile(path, []byte(strings.Replace(contents[production], "n < 5", "(() => { throw new TypeError('environment'); })()", 1)), 0600); err != nil {
				t.Fatal(err)
			}
			if run := runner.Run(ctx, edge, "mutation"); run.Failure != EnvironmentFailure {
				t.Fatal(run)
			}
		})
	}
}

func TestJavaScriptNativeDynamicNamesRemainWhole(t *testing.T) {
	for _, tool := range []string{"jest", "vitest"} {
		t.Run(tool, func(t *testing.T) {
			production := "exports.allowed = n => n < 5;\n"
			imports := "const {allowed}=require('./limits');\n"
			if tool == "vitest" {
				production = "export const allowed = n => n < 5;\n"
				imports = "import {test,expect} from 'vitest';\nimport {allowed} from './limits';\n"
			}
			tests := imports + "for (const name of ['first','second']) {test(name,()=>{expect(allowed(1)).toBe(true)});}\ntest('known',()=>{expect(allowed(5)).toBe(false)});\n"
			executor, request := nativeScriptRequest(t, map[string]string{"package.json": "{\"private\":true}\n", "limits.js": production, "dynamic.test.js": tests})
			command := tool
			if runtime.GOOS != "windows" {
				command = "env TASKBELAY_RUNNER_PROBE=yes " + tool
			}
			request.Config["javascript"] = Config{Command: command}
			runner := NewJavaScriptRunner(executor, request)
			defer runner.Terminate()
			if result := runner.Prepare(t.Context()); !result.Passed() {
				t.Fatal(result)
			}
			cases, result := runner.Enumerate(t.Context(), request.Cases)
			if !result.Passed() || len(cases) != 1 || !cases[0].Whole || len(cases[0].Expected) != 3 || candidateCase(cases[0]) {
				t.Fatal(result, cases)
			}
			if run := runner.Run(t.Context(), cases[0], "baseline"); !run.Passed() || !run.Coverage.CopyExecuted {
				t.Fatal(run)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if run := runner.Run(ctx, cases[0], "baseline"); run.Failure != Canceled {
				t.Fatal(run)
			}
		})
	}
}
