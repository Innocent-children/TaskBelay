package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestFreshnessSchemasValidateActualOperationBranches(t *testing.T) {
	command := exec.Command("node", "freshness-output.mjs", t.TempDir())
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("freshness fixture: %v\n%s", err, output)
	}
	var cases []struct {
		Name   string         `json:"name"`
		Output map[string]any `json:"output"`
	}
	if err = json.Unmarshal(output, &cases); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"codex", "claude", "zcode", "deepseek"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "packages", host, "contracts", "interfaces.json"))
		if err != nil {
			t.Fatal(err)
		}
		var entries []struct {
			Name   string            `json:"name"`
			Output jsonschema.Schema `json:"output_schema"`
		}
		if err = json.Unmarshal(raw, &entries); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name, ".freshness") {
				continue
			}
			found = true
			schema, err := entry.Output.Resolve(nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, example := range cases {
				t.Run(host+"/"+example.Name, func(t *testing.T) {
					if err := schema.Validate(example.Output); err != nil {
						t.Fatal(err)
					}
				})
			}
			invalid := cases[0].Output
			invalid["unexpected"] = true
			if schema.Validate(invalid) == nil {
				t.Fatal("freshness schema accepted an unknown output field", host)
			}
			delete(invalid, "unexpected")
		}
		if !found {
			t.Fatal("missing freshness contract", host)
		}
	}
}
