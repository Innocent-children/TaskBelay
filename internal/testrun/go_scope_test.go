package testrun

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func TestGoNativeScopeUsesReverseTestImportsAndFixedInputs(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	put := func(path, source string) {
		t.Helper()
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("go.mod", "module example.test/scope\n\ngo 1.26\n")
	put("core/core.go", "package core\nfunc Value() int {return 1}\n")
	put("api/api.go", "package api\nfunc Value() int {return 1}\n")
	put("api/api_test.go", "package api_test\nimport(\"testing\";\"example.test/scope/core\")\nfunc TestValue(t *testing.T){if core.Value()<1{t.Fatal(\"bad\")}}\n")
	put("unrelated/value.go", "package unrelated\nfunc Value() int {return 1}\n")
	workspace, err := gitstate.Discover(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	origin, err := workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	put("core/core.go", "package core\nfunc Value() int {return 2}\n")
	current, err := workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	script := shell.Script{Segments: []shell.Segment{{Args: []string{"go", "test", "./..."}, Dir: workspace.Root, Kind: shell.Verification, Known: true}}}
	result, err := (Scope{OriginTree: origin.Tree}).VerificationScope(t.Context(), workspace, script, current.Tree)
	if err != nil || !result.Complete || !result.EnvironmentKnown || result.InputID == "" || result.ToolID == "" || !strings.Contains(result.Suggestion, "example.test/scope/api") || strings.Contains(result.Suggestion, "unrelated") {
		t.Fatal(result, err)
	}
	put("api/api_test.go", "package api_test\nimport(\"testing\";\"os\")\nfunc TestValue(t *testing.T){if os.Getenv(\"REMOTE_STATE\")==\"bad\"{t.Fatal(\"external\")}}\n")
	updated, err := workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	unsafe, err := (Scope{OriginTree: origin.Tree}).VerificationScope(t.Context(), workspace, script, updated.Tree)
	if err != nil || unsafe.EnvironmentKnown || unsafe.InputID == result.InputID {
		t.Fatal("external state was treated as reusable", unsafe, err)
	}
}
