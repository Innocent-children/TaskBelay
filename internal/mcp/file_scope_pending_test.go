package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/google/jsonschema-go/jsonschema"
)

// This is an actual MCP/application/SQLite call with a controlled observer;
// native Git behavior is exercised by the application integration tests.
func TestPendingScopeMCPCompleteErrorAndSuccessSchema(t *testing.T) {
	s := newSkillScenario(t, "codex", readSkillExamples(t, "codex"))
	example := s.example("expand_scope")
	s.prepare(example)
	before := s.task
	root := before.WorkspaceOrigin.CanonicalWorktreeRoot
	binding := s.observer.bindings[root].Clone()
	changed := binding.Clone()
	digest := domain.Digest(strings.Repeat("e", 64))
	entry := domain.RepositoryChangedEntry{Path: "outside.go", ChangeType: domain.RepositoryChangeAdded,
		FileMode: "100644", BaseMode: "000000", BaseContentDigest: digest, IndexMode: "000000",
		IndexContentDigest: digest, WorktreeMode: "100644", WorktreeContentDigest: digest, ContentDigest: digest}
	changed.TaskSurface, changed.ChangedEntries = []domain.RepositoryChangedEntry{entry}, []domain.RepositoryChangedEntry{entry}
	changed.ContentDigest, changed.BindingDigest = digest, digest
	s.observer.bindings[root] = changed
	input := s.bind(example)
	output := s.server.dispatch(context.Background(), ToolResolveBlocker, "scope-mcp-error", input)
	expected := []byte(`{"ok":false,"request_id":"scope-mcp-error","tool":"taskbelay_resolve_blocker","error":{"code":"REPOSITORY_DRIFT","message":"Repository \"primary\" has repository drift: content_changed."},"recovery":{"retry_safe":false,"action":"read_next_action","message":"Read the current workspace blocker or action."}}`)
	var actualValue, expectedValue any
	if json.Unmarshal(output.JSON, &actualValue) != nil || json.Unmarshal(expected, &expectedValue) != nil {
		t.Fatalf("invalid encoded response: %s / %s", output.JSON, expected)
	}
	if !output.IsError || !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("complete response mismatch:\n%s\nwant %s", output.JSON, expected)
	}
	saved, err := s.database.LoadTask(context.Background(), before.TaskID)
	if err != nil || saved.Revision != before.Revision {
		t.Fatal("rejected MCP decision changed Task")
	}
	s.observer.bindings[root] = binding
	success := s.server.dispatch(context.Background(), ToolResolveBlocker, "scope-mcp-success", input)
	if success.IsError {
		t.Fatalf("restored decision: %s", success.JSON)
	}
	for _, definition := range ToolCatalog() {
		if definition.Name != ToolResolveBlocker {
			continue
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(definition.OutputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, encoded := range [][]byte{output.JSON, success.JSON} {
			var value any
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(value); err != nil {
				t.Fatal(err)
			}
		}
	}
}
