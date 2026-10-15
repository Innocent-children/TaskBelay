package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestPublishedInterfacesValidateCompleteExamples(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "interfaces.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		Name     string            `json:"name"`
		Output   jsonschema.Schema `json:"output_schema"`
		Examples []struct {
			Name   string `json:"name"`
			Output any    `json:"output"`
		} `json:"examples"`
	}
	if err = json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Name, func(t *testing.T) {
			schema, err := entry.Output.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, example := range entry.Examples {
				if err := schema.Validate(example.Output); err != nil {
					t.Errorf("%s: %v", example.Name, err)
				}
			}
		})
	}
}
