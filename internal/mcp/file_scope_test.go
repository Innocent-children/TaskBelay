package mcp

import (
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func TestResolveBlockerFileScopeChoiceKeepsClosedEighteenToolCatalog(t *testing.T) {
	valid := []byte(`{"host":"codex","task_id":"task","action_id":"action","choice":"allow_once","reason":"Allow this prepared write."}`)
	if err := ValidateToolInput(ToolResolveBlocker, valid); err != nil {
		t.Fatalf("valid file-scope resolution rejected: %v", err)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"host":"codex","task_id":"task","action_id":"action","reason":"missing choice"}`),
		[]byte(`{"host":"codex","task_id":"task","action_id":"action","choice":"future","reason":"invalid choice"}`),
		[]byte(`{"host":"codex","task_id":"task","action_id":"action","choice":"reject","reason":""}`),
	} {
		if err := ValidateToolInput(ToolResolveBlocker, invalid); err == nil {
			t.Fatalf("invalid file-scope resolution accepted: %s", invalid)
		}
	}
	if err := ValidateToolInput(ToolRecoverAction, valid); err == nil {
		t.Fatal("recover_action accepted file-scope decision members")
	}
	if len(ToolNames()) != 18 {
		t.Fatalf("tool catalog size=%d", len(ToolNames()))
	}
}

func TestTaskProjectionIncludesFileScopeState(t *testing.T) {
	projected := projectTask(domain.ProcessTask{CurrentChangedPaths: []string{"src/file.go"}, FileScopeRecords: []domain.FileScopeRecord{}}).(map[string]any)
	paths, ok := projected["current_changed_paths"].([]string)
	if !ok || len(paths) != 1 || paths[0] != "src/file.go" {
		t.Fatalf("current_changed_paths projection=%#v", projected["current_changed_paths"])
	}
	if _, ok := projected["file_scope_records"]; !ok {
		t.Fatal("file_scope_records projection missing")
	}
}
