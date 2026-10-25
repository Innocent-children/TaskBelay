package check

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func snapshots(t *testing.T, files map[string]string) codescan.Snapshot {
	t.Helper()
	var sources []codescan.Source
	for path, data := range files {
		sources = append(sources, codescan.Source{Path: path, Data: []byte(data), Blob: path + data, Mode: "100644"})
	}
	return codescan.Analyze(t.Context(), "fixture", sources)
}

func staticFixture(t *testing.T, old, current map[string]string, requirements []ledger.Requirement) ledger.CheckRecord {
	t.Helper()
	before, after := snapshots(t, old), snapshots(t, current)
	changes, err := codescan.Compare(t.Context(), before, after, nil)
	if err != nil {
		t.Fatal(err)
	}
	view := ledger.TaskView{Task: &ledger.TaskRecord{State: ledger.TaskPending}, Requirements: requirements, Sources: []ledger.Source{{ID: "m1", Kind: "user_message"}}}
	var positions []codescan.Position
	for _, req := range requirements {
		if req.Status != "revoked" {
			for _, location := range req.ImplementedBy {
				positions = append(positions, codescan.Position{Requirement: req.ID, At: location.At, Purpose: location.Purpose, Change: location.Change})
			}
		}
	}
	association, err := codescan.Associate(t.Context(), before, after, changes, positions)
	if err != nil {
		t.Fatal(err)
	}
	sources := provenance{}
	for path, file := range after.Files {
		sources[path] = fileSources{gitstate.File{Blob: file.Blob, Mode: file.Mode}, sourceLines(file.Data, "agent")}
	}
	result, err := staticCheck(t.Context(), ledger.CheckInput{TaskID: "t"}, view, before, after, changes, association, sources)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func issuesFor(result ledger.CheckRecord, rule string) []ledger.Issue {
	var issues []ledger.Issue
	for _, issue := range result.Issues {
		if issue.Rule == rule {
			issues = append(issues, issue)
		}
	}
	return issues
}

func TestOD07UnusedInternalDeclarationRequiresCompleteEvidence(t *testing.T) {
	base := map[string]string{"main.go": "package main\nfunc main() {}\n"}
	current := map[string]string{"main.go": base["main.go"] + "func unused() int { return 7 }\n"}
	result := staticFixture(t, base, current, nil)
	issues := issuesFor(result, "D1")
	if len(issues) != 1 || issues[0].At != "main.go:unused" || issues[0].Level != "block" {
		t.Fatalf("unused result: %+v; gaps=%+v", issues, result.Missing)
	}
	current["calls.json"] = `{"handler":"unused"}`
	if issues := issuesFor(staticFixture(t, base, current, nil), "D1"); len(issues) != 0 {
		t.Fatal("named config reference ignored", issues)
	}
	delete(current, "calls.json")
	current["bad.go"] = "package main\nfunc broken("
	for _, issue := range issuesFor(staticFixture(t, base, current, nil), "D1") {
		if issue.Level == "block" {
			t.Fatal("incomplete analysis blocked", issue)
		}
	}
}

func TestOD01AndOD02InterfaceUsesAndBaselineConvention(t *testing.T) {
	definition := func(name, method string) string {
		return fmt.Sprintf("type %s interface { %s() }\ntype %sImpl struct{}\nfunc (%sImpl) %s() {}\n", name, method, name, name, method)
	}
	base := "package main\n"
	added := definition("writer", "write")
	result := staticFixture(t, map[string]string{"x.go": base}, map[string]string{"x.go": base + added}, nil)
	issues := issuesFor(result, "D4")
	if len(issues) == 0 || issues[0].Status != "open" {
		t.Fatalf("interface hint missing: %+v; gaps=%+v", issues, result.Missing)
	}
	for i := 0; i < 3; i++ {
		base += definition(fmt.Sprintf("old%d", i), fmt.Sprintf("method%d", i))
	}
	result = staticFixture(t, map[string]string{"x.go": base}, map[string]string{"x.go": base + added}, nil)
	issues = issuesFor(result, "D4")
	if len(issues) == 0 || issues[0].Status != "convention" || len(issues[0].HandledBy) == 0 {
		t.Fatalf("baseline convention missing: %+v; gaps=%+v", issues, result.Missing)
	}
}

func TestOD03AndOD13ConstantsIncludeImplicitZero(t *testing.T) {
	base := map[string]string{"main.go": "package main\nfunc main() {}\n"}
	constant := "package main\nfunc main() { _ = writeCSV(',') }\nfunc writeCSV(sep rune) string { return string(sep) }\n"
	result := staticFixture(t, base, map[string]string{"main.go": constant}, nil)
	if !slices.ContainsFunc(issuesFor(result, "D5"), func(issue ledger.Issue) bool { return issue.At == "main.go:writeCSV.sep" }) {
		t.Fatalf("constant argument missing: %+v; gaps=%+v", result.Issues, result.Missing)
	}
	changed := "package main\ntype state struct { done bool }\nfunc main() { s := state{}; s.done = true; _ = s.done }\n"
	result = staticFixture(t, base, map[string]string{"main.go": changed}, nil)
	if slices.ContainsFunc(issuesFor(result, "D5"), func(issue ledger.Issue) bool { return issue.At == "main.go:state.done" }) {
		t.Fatal("false then true marked constant", result.Issues)
	}
}

func TestOD04EntryRequiresSourcedPurpose(t *testing.T) {
	base := map[string]string{"main.go": "package main\nfunc main() {}\n"}
	current := map[string]string{"main.go": "package main\nimport \"flag\"\nfunc main() { _ = flag.String(\"format\", \"csv\", \"output\") }\n"}
	result := staticFixture(t, base, current, nil)
	issues := issuesFor(result, "D3")
	if len(issues) != 1 || issues[0].Level != "block" {
		t.Fatalf("entry not found: %+v; gaps=%+v", issues, result.Missing)
	}
	at := issues[0].At
	req := ledger.Requirement{ID: "R1", Status: "completed", Citation: &ledger.Citation{SourceID: "m1", Quote: "CSV"}, ImplementedBy: []ledger.Location{{At: at, Purpose: "选择 CSV 输出"}}}
	if issues := issuesFor(staticFixture(t, base, current, []ledger.Requirement{req}), "D3"); len(issues) != 0 {
		t.Fatal("sourced entry blocked", issues)
	}
	req.Citation = &ledger.Citation{SourceID: "project-convention"}
	if issues := issuesFor(staticFixture(t, base, current, []ledger.Requirement{req}), "D3"); len(issues) != 1 {
		t.Fatal("convention used as current user request", issues)
	}
}

func TestOD05OD09OD14AssociationForwardingAndDeletion(t *testing.T) {
	base := map[string]string{"main.go": "package main\nfunc old() int { return 3 }\nfunc target(x int) int { return x + 1 }\n"}
	current := map[string]string{"main.go": "package main\nfunc target(x int) int { return x + 1 }\nfunc wrapper(x int) int { return target(x) }\n"}
	reqs := []ledger.Requirement{{ID: "R1", Status: "completed", ImplementedBy: []ledger.Location{{At: "main.go:old", Change: "delete"}, {At: "main.go:wrapper", Purpose: "适配入口"}}}}
	result := staticFixture(t, base, current, reqs)
	if issues := issuesFor(result, "D2"); len(issues) != 0 {
		t.Fatal("deletion or reachable helper lacks attribution", issues)
	}
	if issues := issuesFor(result, "D6"); len(issues) != 1 || issues[0].At != "main.go:wrapper" {
		t.Fatal("forwarder missing", issues)
	}
	current["main.go"] += "func changedLog() { println(\"extra\") }\n"
	result = staticFixture(t, base, current, reqs)
	if !slices.ContainsFunc(issuesFor(result, "D2"), func(issue ledger.Issue) bool { return issue.At == "main.go:changedLog" }) {
		t.Fatal("unassociated change omitted", result.Issues)
	}
}

func TestOD08OldForwarderAndD8DirectDependency(t *testing.T) {
	base := map[string]string{"main.go": "package main\nfunc old(x int) int { return x + 1 }\n", "go.mod": "module example\ngo 1.26\n"}
	current := map[string]string{"main.go": "package main\nfunc old(x int) int { return next(x) }\nfunc next(x int) int { return x + 1 }\n", "go.mod": "module example\ngo 1.26\nrequire example.org/format v1.0.0\n"}
	result := staticFixture(t, base, current, nil)
	if issues := issuesFor(result, "D7"); len(issues) == 0 || issues[0].At != "main.go:old" {
		t.Fatal("old compatibility forwarding omitted", issues)
	}
	if issues := issuesFor(result, "D8"); len(issues) != 1 || issues[0].At != "dep:example.org/format" {
		t.Fatal("new dependency omitted", issues)
	}
	base["go.mod"] = current["go.mod"]
	current["go.mod"] = strings.ReplaceAll(current["go.mod"], "v1.0.0", "v1.1.0")
	if issues := issuesFor(staticFixture(t, base, current, nil), "D8"); len(issues) != 0 {
		t.Fatal("version update counted as new dependency", issues)
	}
}

func TestUnknownUseDoesNotProduceConstantOrUniqueUseConclusion(t *testing.T) {
	d := codescan.Declaration{File: "a.go", Name: "generic", Usage: &codescan.Usage{Generic: true, Inferred: true, Parameters: []codescan.Slot{{Name: "v", Read: true, Values: []codescan.Value{{Kind: "int", Text: "1"}}, Unknown: []codescan.Use{{Reason: "indirect write"}}}}}}
	s := staticAnalysis{ctx: context.Background(), after: codescan.Snapshot{Files: map[string]*codescan.File{"a.go": {}}}, evaluated: map[string]bool{}}
	s.abstractions(d)
	s.slots(codescan.Change{After: &d, Kind: "add"})
	if len(s.result.Issues) != 0 || len(s.result.Missing) != 2 || len(s.evaluated) != 0 {
		t.Fatal("unknown use became simplification", s.result)
	}
}

func TestD7DetectsNewDeprecatedCommentOnExistingFunction(t *testing.T) {
	base := map[string]string{"api.go": "package api\nfunc Old() {}\n"}
	current := map[string]string{"api.go": "package api\n// Deprecated: use Next.\nfunc Old() {}\n"}
	result := staticFixture(t, base, current, nil)
	if !slices.ContainsFunc(issuesFor(result, "D7"), func(issue ledger.Issue) bool { return issue.FactKey == "deprecated" }) {
		t.Fatal("deprecated comment excluded as ordinary commentary", result.Issues)
	}
}

func TestD1PublicAPIAndTestOnlyReference(t *testing.T) {
	base := map[string]string{"api.go": "package api\n"}
	current := map[string]string{"api.go": "package api\nfunc PublicAPI() int { return 2 }\nfunc internalOnly() int { return 3 }\n", "api_test.go": "package api\nimport \"testing\"\nfunc TestInternal(t *testing.T) { _ = internalOnly() }\n"}
	result := staticFixture(t, base, current, nil)
	issues := issuesFor(result, "D1")
	if !slices.ContainsFunc(issues, func(issue ledger.Issue) bool { return issue.At == "api.go:PublicAPI" && issue.Level == "hint" }) {
		t.Fatal("public API not preserved as hint", issues)
	}
	if !slices.ContainsFunc(issues, func(issue ledger.Issue) bool { return issue.At == "api.go:internalOnly" && issue.Level == "block" }) {
		t.Fatal("test-only use inflated production references", issues, result.Missing)
	}
}

func TestD1AmbiguousLogicalLocationIsNeverABlocker(t *testing.T) {
	d := codescan.Declaration{ID: "one", File: "a.java", Name: "overloaded", Span: codescan.Span{StartLine: 1, EndLine: 1}, Usage: &codescan.Usage{}}
	other := d
	other.ID = "two"
	snapshot := codescan.Snapshot{Files: map[string]*codescan.File{"a.java": {CodeComplete: true, Source: codescan.Source{Path: "a.java", Data: []byte("fixture")}, Declarations: []codescan.Declaration{d, other}, Operations: map[string]codescan.Operation{"references": {Complete: true}, "usage": {Complete: true}, "entries": {Complete: true}}}}}
	s := staticAnalysis{after: snapshot, sources: provenance{"a.java": {lines: []lineSource{{kind: "agent"}}}}, nameUses: map[string][]string{}, evaluated: map[string]bool{}}
	s.unused(d)
	if len(s.result.Issues) != 1 || s.result.Issues[0].BlockOnDelivery || s.result.Issues[0].Level == "block" {
		t.Fatal("ambiguous declaration became a blocker", s.result.Issues)
	}
}
