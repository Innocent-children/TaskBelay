package protocol

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/store"
	"os"
	"strings"
	"testing"
)

func TestEveryPublishedPropertyExplainsItsMeaning(t *testing.T) {
	var inspect func(map[string]any, string)
	inspect = func(schema map[string]any, path string) {
		if properties, ok := schema["properties"].(map[string]any); ok {
			for name, raw := range properties {
				field := raw.(map[string]any)
				if description, _ := field["description"].(string); strings.TrimSpace(description) == "" {
					t.Errorf("%s.%s has no field description", path, name)
				}
				inspect(field, path+"."+name)
			}
		}
		if child, ok := schema["items"].(map[string]any); ok {
			inspect(child, path+"[]")
		}
		for _, name := range []string{"anyOf", "oneOf"} {
			if choices, ok := schema[name].([]any); ok {
				for _, choice := range choices {
					inspect(choice.(map[string]any), path)
				}
			}
		}
	}
	for _, tool := range append(append([]Tool{NativeCheck}, NativePreflights...), Catalog...) {
		if len(tool.Description) < 60 {
			t.Errorf("%s lacks usable method instructions", tool.Name)
		}
		inspect(Schema(tool.Input), tool.Name+".input")
		inspect(OutputSchema(tool.Output), tool.Name+".output")
	}
}
func TestDiscoveryIsSelectiveAndCannotExecuteMethods(t *testing.T) {
	h := testServer(t)
	r := h.Dispatch(context.Background(), "taskbelay_server_info", []byte(`{"contracts":["taskbelay_checkpoint","check.run","host-check.workspace-available"]}`))
	if !r.OK {
		t.Fatal(r)
	}
	value := r.Result.(ServerInfoResult)
	if len(value.Tools) != 15 || len(value.Contracts) != 3 {
		t.Fatal(value)
	}
	for _, c := range value.Contracts {
		if len(c.Examples) < 2 {
			t.Fatal("incomplete packaged contract", c.Name)
		}
		validate(t, OutputSchema(ServerInfoResult{}), r)
	}
	task, err := h.service.Store.List(context.Background(), store.ListQuery{})
	if err != nil || len(task.Items) != 0 {
		t.Fatal("description executed a method", err)
	}
}
func TestCheckpointSchemaRejectsServerGeneratedFields(t *testing.T) {
	h := testServer(t)
	for _, raw := range []string{
		`{"assessment":{"review":{"summary":"review","content_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`,
		`{"assessment":{"paths":[{"path":"file","goal_ids":["g"],"basis":"reason","scope_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}}`,
		`{"progress":[{"goal_id":"g","status":"done","summary":"done","goal_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`,
		`{"host_ms":10}`,
		`{"assessment":{"review":{"summary":"done","uncertainty":""}}}`,
	} {
		result := h.Dispatch(context.Background(), "taskbelay_checkpoint", []byte(raw))
		if result.OK || result.Error.Code != "INVALID_ARGUMENT" {
			t.Fatal(raw, result)
		}
	}
}
func TestMutationSchemasDoNotRepeatContextOrPrivateFingerprints(t *testing.T) {
	for _, tool := range Catalog {
		if tool.Name == "taskbelay_get_task" || tool.Name == "taskbelay_server_info" {
			continue
		}
		raw, _ := json.Marshal(OutputSchema(tool.Output))
		if strings.Contains(string(raw), "worktree_instance_digest") {
			t.Fatalf("%s includes full repository details", tool.Name)
		}
	}
}

func TestDefaultDiscoveryDoesNotRepeatMethodSchemas(t *testing.T) {
	h := testServer(t)
	output := h.Dispatch(context.Background(), "taskbelay_server_info", []byte(`{}`))
	raw, _ := json.Marshal(output)
	if strings.Contains(string(raw), `"input_schema"`) || strings.Contains(string(raw), `"output_schema"`) || len(raw) > 8000 {
		t.Fatalf("default discovery is not compact: %d bytes", len(raw))
	}
	value := output.Result.(ServerInfoResult)
	if value.NativeCheck.Contract != "check.run" {
		t.Fatal(value.NativeCheck)
	}
}

func TestPackagedCoreMethodSchemasMatchCurrentDTOs(t *testing.T) {
	raw, err := os.ReadFile("../../contracts/core-methods.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []MethodContract
	if err = json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(Catalog)+len(NativePreflights)+1 {
		t.Fatal("packaged Core method inventory differs")
	}
	for _, entry := range entries {
		current, err := Describe(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		for key, pair := range map[string][2]any{"input_schema": {entry.InputSchema, current.InputSchema}, "output_schema": {entry.OutputSchema, current.OutputSchema}, "description": {entry.Description, current.Description}} {
			left, _ := json.Marshal(pair[0])
			right, _ := json.Marshal(pair[1])
			if string(left) != string(right) {
				t.Errorf("%s %s differs from the current producer; regenerate the packaged reference", entry.Name, key)
			}
		}
	}
}
