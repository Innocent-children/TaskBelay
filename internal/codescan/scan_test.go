package codescan

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func scanSources(t *testing.T, files map[string]string) Snapshot {
	t.Helper()
	var sources []Source
	for path, text := range files {
		sources = append(sources, Source{Path: path, Data: []byte(text), Mode: "100644"})
	}
	return Analyze(t.Context(), "fixed-tree", sources)
}

func TestSyntaxLookupSelectsNestedDeclaration(t *testing.T) {
	for _, sample := range []struct{ file, language, source string }{
		{"a.java", "java", `class Outer {int value;class Inner{int nested;}}`},
		{"a.py", "python", "class Outer:\n value=1\n class Inner:\n  nested=2\n"},
		{"a.cs", "c_sharp", `class Outer {int value;class Inner{int nested;}}`},
		{"a.cpp", "cpp", `class Outer {int value;class Inner{int nested;};};`},
	} {
		t.Run(sample.language, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{sample.file: sample.source})
			tree, err := parse(t.Context(), grammars.DetectLanguageByName(sample.language), []byte(sample.source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			language := grammars.DetectLanguageByName(sample.language).Language()
			found := false
			for _, declaration := range snapshot.Files[sample.file].Declarations {
				if declaration.Name != "Inner" {
					continue
				}
				found = true
				n := syntaxNodeAt(tree.RootNode(), language, declaration.NodeType, declaration.NameSpan)
				if n == nil {
					t.Fatal("nested declaration missing")
				}
				name := n.ChildByFieldName("name", language)
				if name == nil || span(name) != declaration.NameSpan {
					t.Fatal("nested declaration resolved to outer scope", declaration, n.SExpr(language))
				}
			}
			if !found {
				t.Fatal(snapshot.Files[sample.file].Declarations)
			}
		})
	}
}

func TestDocumentedDeclarationsAndLocalDefinitions(t *testing.T) {
	for _, test := range []struct {
		file, text string
		want       []string
	}{
		{"a.go", "package p\nconst A, B = 1, 2\nvar C, D int\ntype Box struct { V int }\nfunc (b *Box) Read() int { type Local int; return b.V }\n", []string{"A", "B", "Box", "Box.Read", "C", "D"}},
		{"a.py", "COUNT = 1\nclass Box:\n def read(self):\n  def local():\n   return COUNT\n  return local()\n", []string{"Box", "Box.read", "COUNT"}},
		{"a.js", "const value = 1; class Box { read() { const local=2; return value+local; } }", []string{"Box", "Box.read", "value"}},
		{"a.ts", "interface Reader { read(): number; } type Count = number; const value=1; class Box { read(): number { return value; } }", []string{"Box", "Box.read", "Count", "Reader", "Reader.read", "value"}},
		{"a.java", "class Box { static final int VALUE=1; int read() { int local=2; return VALUE+local; } }", []string{"Box", "Box.VALUE", "Box.read"}},
		{"a.rs", "const VALUE:u32=1; struct Box {v:u32} impl Box { fn read(&self)->u32 { const LOCAL:u32=2; self.v+LOCAL } }", []string{"Box", "Box.read", "VALUE"}},
		{"a.c", "const int VALUE=1; int state; int read_value(void) { return VALUE; }", []string{"VALUE", "read_value", "state"}},
		{"a.cpp", "const int VALUE=1; class Box { public: int read() { return VALUE; } };", []string{"Box", "Box.read", "VALUE"}},
		{"a.cs", "class Box { const int VALUE=1; int Read() { return VALUE; } }", []string{"Box", "Box.Read", "Box.VALUE"}},
		{"a.rb", "VALUE=1\nclass Box\n def read\n  VALUE\n end\nend\n", []string{"Box", "Box.read", "VALUE"}},
		{"a.php", "<?php const VALUE=1; $state=0; class Box { function read() { return VALUE; } }", []string{"$state", "Box", "Box.read", "VALUE"}},
	} {
		t.Run(test.file, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{test.file: test.text})
			if !snapshot.Files[test.file].CodeComplete || !snapshot.Files[test.file].Operations["declarations"].Complete {
				t.Fatal(snapshot.Missing)
			}
			var got []string
			for _, declaration := range snapshot.Files[test.file].Declarations {
				got = append(got, strings.TrimPrefix(declaration.ID, test.file+":"))
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v; want %v", got, test.want)
			}
		})
	}
}

func TestPartialCodeKeepsFileFactsAndRustTestRange(t *testing.T) {
	snapshot := scanSources(t, map[string]string{
		"bad.go":           "package p\nfunc broken( {",
		"unknown.zzz":      "opaque",
		"a.d.ts":           "declare const value: number;",
		"vendor/a.go":      "package p\nfunc external() {}",
		"generated.go":     "// @generated\npackage p\nfunc generated() {}",
		"tests/input.json": "{}",
		"src/lib.rs":       "pub fn prod() {}\n#[cfg(test)] mod tests { #[test] fn check() {} }\n",
	})
	if len(snapshot.Missing) != 1 || snapshot.Missing[0].Path != "bad.go" || snapshot.Files["bad.go"].CodeComplete {
		t.Fatal(snapshot.Missing)
	}
	for _, file := range []string{"unknown.zzz", "a.d.ts", "vendor/a.go", "generated.go", "tests/input.json"} {
		if snapshot.Files[file] == nil || len(snapshot.Files[file].Declarations) != 0 {
			t.Fatal(file)
		}
	}
	decls := snapshot.Declarations()
	if decls["src/lib.rs:prod"].Test || !decls["src/lib.rs:tests.check"].Test {
		t.Fatal(decls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	partial := Analyze(ctx, "tree", []Source{{Path: "a.go", Data: []byte("package p")}})
	if len(partial.Missing) != 1 || partial.Files["a.go"] == nil {
		t.Fatal(partial)
	}
}

func TestComplexBindingsAndRepeatedAssignments(t *testing.T) {
	for _, test := range []struct {
		file, source string
		names        []string
	}{
		{"a.py", "first, [second, *rest] = source\nclass Box:\n value: int = 1\n value = 2\n", []string{"Box", "Box.value", "first", "rest", "second"}},
		{"a.js", "const {value: renamed, other, nested: {x}, ready = true, ...rest} = source; const [first,,last=1,...tail]=items;", []string{"first", "last", "other", "ready", "renamed", "rest", "tail", "x"}},
		{"a.ts", "const {value: renamed, ...rest}: Source = source;", []string{"renamed", "rest"}},
		{"a.rb", "first, second, *rest = source\nVALUE=1\nVALUE=2\n", []string{"VALUE", "first", "rest", "second"}},
		{"a.php", "<?php [$first, $second] = source(); list($last, $rest) = source(); $state=1; $state=2;", []string{"$first", "$last", "$rest", "$second", "$state"}},
	} {
		t.Run(test.file, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{test.file: test.source})
			file := snapshot.Files[test.file]
			if !file.CodeComplete {
				t.Fatal(file.Missing)
			}
			var got []string
			for _, d := range file.Declarations {
				got = append(got, strings.TrimPrefix(d.Location(), test.file+":"))
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, test.names) {
				t.Fatalf("got %v, want %v", got, test.names)
			}
		})
	}
	before := scanSources(t, map[string]string{"a.py": "VALUE = 1\nobserve(VALUE)\nVALUE = 2\n"})
	after := scanSources(t, map[string]string{"a.py": "VALUE = 1\nobserve(VALUE)\nVALUE = 3\n"})
	d := after.Declarations()["a.py:VALUE"]
	if len(d.Ranges()) != 2 || len(d.NameRanges()) != 2 || strings.Contains(d.Text, "observe") || len(d.References) != 0 || !reflect.DeepEqual(after.Files["a.py"].References, []string{"VALUE", "observe"}) {
		t.Fatal(d, after.Files["a.py"].References)
	}
	changes, err := Compare(t.Context(), before, after, nil)
	if err != nil || len(changes) != 1 || changes[0].Kind != "modify" || changes[0].Detail != "literal" {
		t.Fatal(changes, err)
	}
}

func TestOverloadsKeepCanonicalPositions(t *testing.T) {
	before := scanSources(t, map[string]string{"Box.java": "class Box { int read(int n){return n;} String read(String s){return s;} }"})
	after := scanSources(t, map[string]string{"Box.java": "class Box { boolean read(boolean flag){return flag;} int read(int n){return n;} String read(String s){return s;} }"})
	changes, err := Compare(t.Context(), before, after, nil)
	if err != nil || len(changes) != 2 || changes[1].Kind != "add" || changes[1].Location() != "Box.java:Box.read" || len(Migrations(changes)) != 0 {
		t.Fatal(changes, err)
	}
	association, err := Associate(t.Context(), before, after, changes, []Position{{Requirement: "R1", At: "Box.java:Box.read"}})
	if err != nil || len(association.Pending) != 0 {
		t.Fatal(association, err)
	}
	for id, d := range after.Declarations() {
		if d.Name == "read" && !reflect.DeepEqual(association.Requirements[id], []string{"R1"}) {
			t.Fatal(id, association)
		}
	}
	renamed := scanSources(t, map[string]string{"Box.java": "class Box { int parse(int n){return n;} String read(String s){return s;} }"})
	changes, err = Compare(t.Context(), before, renamed, nil)
	if err != nil || len(Migrations(changes)) != 0 {
		t.Fatal(changes, err)
	}
}

func TestDeclaratorsAndNestedOwners(t *testing.T) {
	for _, test := range []struct {
		file, source string
		names        []string
	}{
		{"a.c", "typedef int Count; struct Box {int value;}; int *pointer, values[2]; int (*callback)(int); int fn(int n){static int local=1; return n+local;}", []string{"Box", "Count", "callback", "fn", "pointer", "values"}},
		{"a.cpp", "namespace a { using Count=int; class Box { public: int get(){return 1;} }; int *pointer; int Box::read(){return 2;} }", []string{"a.Box", "a.Box.get", "a.Box.read", "a.Count", "a.pointer"}},
		{"a.cs", "namespace N { interface Reader { int Read(); } record Box(int Value); class A { public int Field; public int Read()=>Field; } }", []string{"N.A", "N.A.Field", "N.A.Read", "N.Box", "N.Reader", "N.Reader.Read"}},
		{"a.ts", "namespace N { export const A=1; }", []string{"N.A"}},
	} {
		t.Run(test.file, func(t *testing.T) {
			snapshot := scanSources(t, map[string]string{test.file: test.source})
			file := snapshot.Files[test.file]
			if !file.CodeComplete {
				t.Fatal(file.Missing)
			}
			var names []string
			for _, d := range file.Declarations {
				names = append(names, strings.TrimPrefix(d.Location(), test.file+":"))
			}
			sort.Strings(names)
			if !reflect.DeepEqual(names, test.names) {
				t.Fatal(names, test.names)
			}
		})
	}
}

func TestMissingTagBindingsRemainIncomplete(t *testing.T) {
	entry := grammars.DetectLanguageByName("python")
	data := []byte("first, second = source\n")
	tree, err := parse(t.Context(), entry, data)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	file := scanSources(t, map[string]string{"a.py": string(data)}).Files["a.py"]
	file.Declarations = nil
	if err := auditDefinitions(t.Context(), file, tree, entry.Language(), nil); err != nil {
		t.Fatal(err)
	}
	if len(file.Missing) != 2 {
		t.Fatal(file.Missing)
	}
}

func TestLeafChangesAndUniqueRelocation(t *testing.T) {
	base := scanSources(t, map[string]string{"a.go": "package p\nfunc value(n int) int { return n+1 }\n"})
	for _, test := range []struct{ text, detail string }{
		{"package p\nfunc value(n int) int {\n return n + 1\n}\n", "format"},
		{"package p\nfunc value(n int) int { /* reason */ return n+1 }\n", "comment"},
		{"package p\n// Deprecated: use another function.\nfunc value(n int) int { return n+1 }\n", "comment"},
		{"package p\nfunc value(n int) int { return n+2 }\n", "literal"},
		{"package p\nfunc value(input int) int { return input+1 }\n", "names"},
		{"package p\nfunc value(n int) int { return n-1 }\n", "behavior"},
	} {
		current := scanSources(t, map[string]string{"a.go": test.text})
		changes, err := Compare(t.Context(), base, current, nil)
		if err != nil || len(changes) != 1 || changes[0].Detail != test.detail {
			t.Fatalf("%s: %+v %v", test.detail, changes, err)
		}
	}
	moved := scanSources(t, map[string]string{"b.go": "package p\nfunc count(n int) int { return n+1 }\n"})
	changes, err := Compare(t.Context(), base, moved, nil)
	if err != nil || len(changes) != 1 || changes[0].Kind != "move" || Migrations(changes)["a.go:value"] != "b.go:count" {
		t.Fatal(changes, err)
	}
	ambiguous := scanSources(t, map[string]string{"b.go": "package p\nfunc first(n int) int { return n+1 }\nfunc second(n int) int { return n+1 }\n"})
	changes, err = Compare(t.Context(), base, ambiguous, nil)
	if err != nil || len(Migrations(changes)) != 0 {
		t.Fatal("ambiguous structure moved", changes, err)
	}
	changes, err = Compare(t.Context(), base, ambiguous, []Relocation{{From: "a.go:value", To: "b.go:second"}})
	if err != nil || Migrations(changes)["a.go:value"] != "b.go:second" {
		t.Fatal("verified edit mapping was ignored", changes, err)
	}
}

func TestReferenceCandidatesAndRequirementDirection(t *testing.T) {
	base := scanSources(t, map[string]string{
		"a.go": "package p\nfunc root(){ middle() }; func middle(){ helper() }; func helper(){}; func unrelated(){ helper() }; func caller(){ root() }; func removed(){}\n",
	})
	current := scanSources(t, map[string]string{
		"a.go":     "package p\nfunc root(){ middle() }; func middle(){ helper() }; func helper(){}; func unrelated(){ helper() }; func caller(){ root() }; func usesDeleted(){ removed() }; func words(){ _ = \"unrelated\" /* removed */ }\n",
		"other.go": "package p\nfunc helper(){}\n",
	})
	if len(base.Missing)+len(current.Missing) != 0 {
		t.Fatal(base.Missing, current.Missing)
	}
	changes, err := Compare(t.Context(), base, current, nil)
	if err != nil {
		t.Fatal(err)
	}
	association, err := Associate(t.Context(), base, current, changes, []Position{{Requirement: "R1", At: "a.go:root"}, {Requirement: "R2", At: "a.go:helper"}, {Requirement: "R3", At: "a.go:removed", Change: "delete"}})
	if err != nil || !association.Candidate {
		t.Fatal(association, err)
	}
	contains := func(id, requirement string) bool {
		for _, item := range association.Requirements[id] {
			if item == requirement {
				return true
			}
		}
		return false
	}
	for _, id := range []string{"a.go:root", "a.go:middle", "a.go:helper", "a.go:caller", "other.go:helper"} {
		if !contains(id, "R1") {
			t.Error("missing reachable candidate", id, association.Requirements)
		}
	}
	if contains("a.go:unrelated", "R1") || contains("a.go:words", "R3") || !contains("a.go:usesDeleted", "R3") || !contains("a.go:helper", "R2") {
		t.Fatal(association.Requirements)
	}
}

func TestIncompleteParseDoesNotInventDeclarationDeletion(t *testing.T) {
	base := scanSources(t, map[string]string{"a.go": "package p\nfunc existing() {}\n"})
	current := scanSources(t, map[string]string{"a.go": "package p\nfunc existing( {\n"})
	changes, err := Compare(t.Context(), base, current, nil)
	if err != nil || len(changes) != 0 || len(current.Missing) == 0 {
		t.Fatal(changes, current.Missing, err)
	}
}

func TestInterpolatedExpressionsRemainReferenceCandidates(t *testing.T) {
	for _, test := range []struct{ file, text, id string }{
		{"a.py", "def helper():\n return 1\ndef caller():\n return f'answer={helper()}'\n", "a.py:caller"},
		{"a.ts", "function helper(){return 1}; function caller(){return `answer=${helper()}`}", "a.ts:caller"},
	} {
		snapshot := scanSources(t, map[string]string{test.file: test.text})
		declaration := snapshot.Declarations()[test.id]
		found := false
		for _, reference := range declaration.References {
			found = found || reference == "helper"
		}
		if !found || len(snapshot.Missing) != 0 {
			t.Fatal(test.file, declaration, snapshot.Missing)
		}
	}
}

func TestMultipleBindingsDoNotReferenceEachOthersNames(t *testing.T) {
	for _, test := range []struct{ file, source string }{
		{"a.go", "package p\nvar first,second=1,2\nconst third=first\n"},
		{"a.py", "first,second=(1,2)\nthird=first\n"},
		{"a.js", "const [first,second]=[1,2];const third=first;"},
	} {
		snapshot := scanSources(t, map[string]string{test.file: test.source})
		d := snapshot.Declarations()
		if len(d[test.file+":first"].References) != 0 || len(d[test.file+":second"].References) != 0 || !reflect.DeepEqual(d[test.file+":third"].References, []string{"first"}) {
			t.Fatal(test, d)
		}
	}
}
