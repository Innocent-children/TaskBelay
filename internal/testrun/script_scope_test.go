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

func TestNativeScriptScopesUseFixedDependencies(t *testing.T) {
	for _, tool := range []string{"pytest", "jest", "vitest"} {
		t.Run(tool, func(t *testing.T) {
			tools := os.Getenv("TASKBELAY_TEST_TOOLS")
			if tools == "" {
				t.Skip("需要独立原生框架环境")
			}
			dir := t.TempDir()
			put := func(path, data string) {
				t.Helper()
				path = filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := exec.CommandContext(t.Context(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
				t.Fatal(err, string(out))
			}
			put(".gitignore", ".venv\nnode_modules\n")
			changed, affected := "src/core.py", "test_bridge.py"
			if tool == "pytest" {
				put(changed, "def value():\n    return 1\n")
				put("src/bridge.py", "from core import value\n")
				put(affected, "from bridge import value\ndef test_value():\n    assert value() == 1\n")
				put("test_other.py", "def test_other():\n    assert True\n")
			} else {
				changed, affected = "core.js", "bridge.test.js"
				put("package.json", "{\"private\":true}\n")
				if tool == "jest" {
					put("jest.config.cjs", "if(process.env.TASKBELAY_SCOPE_FIXTURE !== 'present') throw new Error('scope environment lost');\nmodule.exports={};\n")
				}
				if tool == "vitest" {
					put("vitest.config.mjs", "if(process.env.TASKBELAY_SCOPE_FIXTURE !== 'present') throw new Error('scope environment lost');\nexport default {};\n")
				}
				put(changed, "export const value = 1;\n")
				put("bridge.js", "export {value} from './core.js';\n")
				if tool == "vitest" {
					put(affected, "import {test,expect} from 'vitest';\nimport {value} from './bridge.js';\ntest('value',()=>expect(value).toBe(1));\n")
					put("other.test.js", "import {test,expect} from 'vitest';\ntest('other',()=>expect(true).toBe(true));\n")
				} else {
					put(affected, "const {value}=require('./bridge');\ntest('value',()=>expect(value).toBe(1));\n")
					put("other.test.js", "test('other',()=>expect(true).toBe(true));\n")
				}
			}
			for _, name := range []string{".venv", "node_modules"} {
				if err := os.Symlink(filepath.Join(tools, name), filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			workspace, err := gitstate.Discover(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			before, err := workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if tool == "pytest" {
				put(changed, "def value():\n    return 2\n")
			} else {
				put(changed, "export const value = 2;\n")
			}
			after, err := workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			args := []string{tool}
			if tool == "vitest" {
				args = append(args, "run")
			}
			script := shell.Script{Segments: []shell.Segment{{Args: args, Dir: workspace.Root, Kind: shell.Verification, Known: true, Environment: map[string]string{"TASKBELAY_SCOPE_FIXTURE": "present"}}}}
			result, err := (Scope{OriginTree: before.Tree}).VerificationScope(t.Context(), workspace, script, after.Tree)
			if err != nil || result.Suggestion == "" || result.InputID == "" || result.EnvironmentKnown {
				t.Fatal(result, err)
			}
			joined := strings.Join(result.Evidence, " ") + result.Suggestion
			if !strings.Contains(joined, affected) || strings.Contains(joined, "other.test") || strings.Contains(joined, "test_other") {
				t.Fatal(result)
			}
			current, err := workspace.Capture(t.Context())
			if err != nil || current.Tree != after.Tree {
				t.Fatal("scope changed workspace", err)
			}
			if tool == "pytest" {
				put("src/bridge.py", "from importlib import import_module\nvalue = import_module('core').value\n")
				current, err = workspace.Capture(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				result, err = (Scope{OriginTree: before.Tree}).VerificationScope(t.Context(), workspace, script, current.Tree)
				if err != nil || result.Suggestion != "" || result.Complete {
					t.Fatal(result, err)
				}
			} else {
				put(tool+".config.js", "module.exports = {};\n")
				current, err = workspace.Capture(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				result, err = (Scope{OriginTree: before.Tree}).VerificationScope(t.Context(), workspace, script, current.Tree)
				if err != nil || result.Suggestion != "" || !strings.Contains(strings.Join(result.Evidence, " "), "共享配置变化") {
					t.Fatal(result, err)
				}
			}
		})
	}
}
