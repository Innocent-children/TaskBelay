package testrun

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

type evidenceRunner struct {
	request  Request
	mode     string
	compiled int
	runs     int
}

func (r *evidenceRunner) Prepare(context.Context) Result { return Result{} }
func (r *evidenceRunner) Compile(context.Context) Result {
	r.compiled++
	if r.mode == "invalid" && r.compiled > 1 {
		return failure(CompileFailure, "invalid mutant")
	}
	return Result{}
}
func (r *evidenceRunner) Enumerate(_ context.Context, cases []Case) ([]Case, Result) {
	return cases, Result{}
}
func (r *evidenceRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	r.runs++
	result := RunResult{Coverage: Coverage{Valid: true, CopyExecuted: true, Lines: r.request.Requirements["R1"]}, Selected: []string{test.Name}}
	if ctx.Err() != nil {
		result.Result = failure(Canceled, ctx.Err().Error())
	}
	if stage == "mutation" && test.Name == "boundary" && r.mode != "survived" {
		result.Result = failure(AssertionFailure, "known assertion")
	}
	if r.mode == "coverage" || r.mode == "mutation-coverage" && stage == "mutation" {
		result.Coverage.Valid = false
	}
	return result
}
func (r *evidenceRunner) ReadCoverage(_ context.Context, result RunResult) Coverage {
	return result.Coverage
}
func (r *evidenceRunner) Terminate() {}

func evidenceFixture(t *testing.T, code string, count int) (*Executor, Request) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "range.go"), []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := codescan.Analyze(t.Context(), "tree", []codescan.Source{{Path: "range.go", Data: []byte(code), Mode: "100644"}})
	request := Request{Snapshot: snapshot, Requirements: map[string][]Line{"R1": {{File: "range.go", Number: 2}}}, Changed: map[string][]Line{"R1": {{File: "range.go", Number: 2}}}}
	for i, name := range []string{"boundary", "same", "another", "more"}[:count] {
		request.Cases = append(request.Cases, Case{At: name, Name: name, Package: "pkg", Change: "add", AgentOnly: true, Requirements: []string{"R1"}, Source: codescan.TestCase{File: "range_test.go", Span: codescan.Span{Start: uint32(i)}}})
	}
	return &Executor{Root: root, Output: t.TempDir()}, request
}

func TestMutationBudgetAndInvalidEvidenceNeverProduceDeletionList(t *testing.T) {
	code := "package sample\nfunc Allowed(n int) bool { if n > 0 { return n + 1 > 2 }; return !false }\n"
	for _, mode := range []string{"budget", "invalid", "survived", "coverage", "mutation-coverage"} {
		t.Run(mode, func(t *testing.T) {
			executor, request := evidenceFixture(t, code, 4)
			runner := &evidenceRunner{request: request, mode: mode}
			report := Report{Complete: true}
			runAnalysis(t.Context(), executor, runner, request, &report)
			for _, group := range report.Groups {
				if len(group.Candidates) > 0 || len(group.Mutations) > 20 {
					t.Fatal(group)
				}
				runs := 0
				for _, run := range group.Runs {
					if run.Case != "" {
						runs++
					}
				}
				if runs > 40 {
					t.Fatal(runs)
				}
				if mode == "budget" && group.Complete {
					t.Fatal("partial mutation matrix marked complete")
				}
			}
			if (mode == "budget" || mode == "mutation-coverage") && report.Complete {
				t.Fatal("budget was ignored")
			}
			actual, _ := os.ReadFile(filepath.Join(executor.Root, "range.go"))
			if string(actual) != code {
				t.Fatal("mutation was not restored")
			}
		})
	}
}

func TestGreedySelectionKeepsModifiedRetainedAndMixedCases(t *testing.T) {
	group := Group{Complete: true, Cases: []string{"modified", "redundant", "boundary", "retained", "mixed"}, Mutations: []Mutation{{ID: "one"}, {ID: "two"}}}
	var observations []Observation
	for _, name := range group.Cases {
		test := Case{At: name, Name: name, Change: "add", AgentOnly: true}
		switch name {
		case "modified":
			test.Change = "modify"
		case "retained":
			test.Retained = true
		case "mixed":
			test.Mixed = true
			test.AgentOnly = false
		}
		killed := []string{"one"}
		if name == "boundary" {
			killed = []string{"two"}
		}
		observations = append(observations, Observation{Case: test, Killed: killed})
	}
	selectCandidates(&group, observations)
	if !slices.Equal(group.Candidates, []string{"redundant"}) || !slices.Contains(group.Keep, "boundary") || !slices.Contains(group.Keep, "modified") || !slices.Contains(group.Keep, "retained") || !slices.Contains(group.Keep, "mixed") {
		t.Fatal(group)
	}
}

func TestGoAffectedPackagesIncludesBothTestImports(t *testing.T) {
	packages := map[string]goPackage{"core": {}, "api": {Imports: []string{"core"}}, "internal-test": {TestImports: []string{"api"}}, "external-test": {XTestImports: []string{"internal-test"}}, "unrelated": {}}
	got := affectedGoPackages(packages, map[string]bool{"core": true})
	if !slices.Equal(got, []string{"api", "core", "external-test", "internal-test"}) {
		t.Fatal(got)
	}
}
