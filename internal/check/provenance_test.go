package check

import (
	"testing"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestLineProvenanceKeepsDisjointAgentEditAndTaintsUserDeletion(t *testing.T) {
	before := codescan.Snapshot{Files: map[string]*codescan.File{"a.go": {Source: codescan.Source{Blob: "base", Mode: "100644", Data: []byte("header\n")}}}}
	after := codescan.Snapshot{Files: map[string]*codescan.File{"a.go": {Source: codescan.Source{Blob: "final", Mode: "100644", Data: []byte("changed header\nfunc unused() {\n}\n")}}}}
	file := func(blob string) gitstate.File { return gitstate.File{Blob: blob, Mode: "100644"} }
	edits := []ledger.FileChange{
		{Path: "a.go", Kind: "agent_edit", ToolCallID: "one", Before: file("base"), After: file("agent"), Fragments: []gitstate.Fragment{{BeforeStart: 1, BeforeLines: 0, AfterStart: 2, AfterLines: 2}}},
		{Path: "a.go", Kind: "workspace_update", Before: file("agent"), After: file("final"), Fragments: []gitstate.Fragment{{BeforeStart: 1, BeforeLines: 1, AfterStart: 1, AfterLines: 1}}},
	}
	contents := map[string][]byte{"base": []byte("header\n"), "agent": []byte("header\nfunc unused() {\n}\n"), "final": after.Files["a.go"].Data}
	p, err := buildProvenance(t.Context(), before, after, edits, contents)
	if err != nil {
		t.Fatal(err)
	}
	span := []codescan.Span{{StartLine: 2, EndLine: 3}}
	if agent, mixed, _ := p.at("a.go", span); !agent || mixed {
		t.Fatal("disjoint update tainted Agent declaration", agent, mixed)
	}
	contents["deleted"] = []byte("changed header\n}\n")
	edits = append(edits, ledger.FileChange{Path: "a.go", Kind: "workspace_update", Before: file("final"), After: file("deleted"), Fragments: []gitstate.Fragment{{BeforeStart: 2, BeforeLines: 1, AfterStart: 1, AfterLines: 0}}})
	after.Files["a.go"].Blob, after.Files["a.go"].Data = "deleted", contents["deleted"]
	p, err = buildProvenance(t.Context(), before, after, edits, contents)
	if err != nil {
		t.Fatal(err)
	}
	if agent, mixed, _ := p.at("a.go", []codescan.Span{{StartLine: 2, EndLine: 2}}); agent || !mixed {
		t.Fatal("workspace deletion lost its provenance", agent, mixed)
	}
}
