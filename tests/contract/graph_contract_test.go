package contract_test

import (
	"encoding/json"
	coremcp "github.com/Innocent-children/taskbelay/internal/mcp"
	"os"
	"path/filepath"
	"testing"
)

func TestGraphContractCatalogAndFixtures(t *testing.T) {
	if len(coremcp.ToolNames()) != 18 {
		t.Fatal("tool catalog changed")
	}
	root := contractRepositoryRoot(t)
	for _, name := range []string{"graph-server-info.json", "graph-open-requirements.json", "graph-design-action.json", "graph-invalid-edge.json"} {
		raw, err := os.ReadFile(filepath.Join(root, "protocol", "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			t.Fatalf("invalid %s", name)
		}
	}
}
func TestGraphContractMethodEvidenceSemantics(t *testing.T) {
	var schema map[string]any
	for _, tool := range coremcp.ToolCatalog() {
		if tool.Name == coremcp.ToolSubmitRequirements {
			if json.Unmarshal(tool.InputSchema, &schema) != nil {
				t.Fatal("invalid requirements submission schema")
			}
		}
	}
	methods := schema["properties"].(map[string]any)["method_results"].(map[string]any)
	properties := methods["properties"].(map[string]any)
	for _, step := range []string{"requirements.capture", "requirements.clarify", "requirements.validate"} {
		entry, ok := properties[step].(map[string]any)
		if !ok {
			t.Fatalf("missing method step %s", step)
		}
		fields := entry["properties"].(map[string]any)
		if len(fields) != 2 || fields["capability"] == nil || fields["summary"] == nil || fields["step_id"] != nil || fields["status"] != nil {
			t.Fatalf("method step %s fields=%#v", step, fields)
		}
	}
}
func TestFixtureContractGraphInputsAreClosed(t *testing.T) {
	if err := coremcp.ValidateToolInput(coremcp.ToolGetTask, []byte(`{"host":"codex","task_id":"task","operation_probe":null}`)); err != nil {
		t.Fatal(err)
	}
	if err := coremcp.ValidateToolInput(coremcp.ToolGetTask, []byte(`{"host":"codex","task_id":"task","operation_probe":null,"destination":"DONE"}`)); err == nil {
		t.Fatal("caller destination accepted")
	}
}
