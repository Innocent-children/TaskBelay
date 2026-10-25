package eval_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestRepairedPythonToolAndExplicitRefreshCompleteSameInput(t *testing.T) {
	tools := os.Getenv("TASKBELAY_TEST_TOOLS")
	if tools == "" || runtime.GOOS == "windows" {
		t.Skip("需要独立 Python 环境及 POSIX 启动器")
	}
	service, task, message, _ := deliveryFixture(t)
	for path, source := range map[string]string{"limits.py": "def allowed(n):\n    return n < 5\n", "test_limits.py": "from limits import allowed\ndef test_limit():\n    assert allowed(1)\n"} {
		verifiedTaskEdit(t, service, task.ID, "edit-"+path, path, "", source)
	}
	positions := []ledger.Location{{At: "limits.py:allowed", Purpose: "边界判断"}}
	saveRequirement(t, service, taskActor("req"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "当前结果通过", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &positions})
	launcher := filepath.Join(t.TempDir(), "python3")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "test.python.command", quote(launcher)+" -m pytest"); err != nil {
		t.Fatal(err)
	}
	first, err := service.Check(t.Context(), taskActor("missing"), check.RunInput{Depth: "tests"})
	if err != nil || first.Status != ledger.CheckPartial || first.Report.Dynamic == nil {
		t.Fatal(first.Status, first.Missing, err)
	}
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nexec "+quote(filepath.Join(tools, ".venv", "bin", "python"))+" \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	second, err := service.Check(t.Context(), taskActor("repaired"), check.RunInput{Depth: "tests"})
	if err != nil || second.Status != ledger.CheckComplete || second.InputVersion != first.InputVersion || second.ID == first.ID {
		t.Fatal(second.Status, second.Missing, err)
	}
	third, err := service.Check(t.Context(), taskActor("refresh"), check.RunInput{Depth: "tests", Refresh: true})
	if err != nil || third.Status != ledger.CheckComplete || third.ID == second.ID || third.InputVersion != second.InputVersion {
		t.Fatal(third.Status, third.Missing, err)
	}
}

func TestScriptDynamicLedgerFlowKeepsBoundaryAndOriginalWorkspace(t *testing.T) {
	tools := os.Getenv("TASKBELAY_TEST_TOOLS")
	if tools == "" {
		t.Skip("需要 TASKBELAY_TEST_TOOLS 提供独立的 Python、Jest 和 Vitest 环境")
	}
	for _, tool := range []string{"pytest", "jest", "vitest"} {
		t.Run(tool, func(t *testing.T) {
			service, task, message, _ := deliveryFixture(t)
			files := map[string]string{".gitignore": ".venv\nnode_modules\n"}
			language, location := "python", "limits.py:allowed"
			if tool == "pytest" {
				files["limits.py"] = "def allowed(n):\n    return n < 5\n"
				files["test_limits.py"] = `import pytest
from limits import allowed
@pytest.mark.parametrize("n,want", [(1,True),(2,True),(5,False)], ids=["inside","same","edge"])
def test_limit(n,want):
    assert allowed(n) == want
`
				files["test_other.py"] = "def test_other():\n    assert 1 < 5\n"
			} else {
				files["package.json"] = "{\"private\":true}\n"
				if tool == "jest" {
					language, location = "javascript", "limits.js:allowed"
					files["limits.js"] = "function allowed(n) { return n < 5; }\nfunction other(n) {return n > 0;}\nmodule.exports={allowed,other};\n"
					files["limits.test.js"] = `const {allowed,other}=require('./limits');
test.each([['inside',1,true],['same',2,true],['edge',5,false]])('%s',(name,n,want)=>{expect(allowed(n)).toBe(want)});
test('other',()=>{expect(other(1)).toBe(true)});
`
				} else {
					language, location = "typescript", "limits.ts:allowed"
					files["limits.ts"] = "export function allowed(n: number): boolean { return n < 5; }\nexport function other(n: number): boolean {return n > 0;}\n"
					files["limits.test.ts"] = `import {test,expect} from 'vitest';
import {allowed,other} from './limits';
test.each([['inside',1,true],['same',2,true],['edge',5,false]])('%s',(name,n,want)=>{expect(allowed(n as number)).toBe(want)});
test('other',()=>{expect(other(1)).toBe(true)});
`
				}
			}
			for path, source := range files {
				verifiedTaskEdit(t, service, task.ID, "edit-"+path, path, "", source)
			}
			for _, name := range []string{".venv", "node_modules"} {
				if err := os.Symlink(filepath.Join(tools, name), filepath.Join(service.Workspace.Root, name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "test."+language+".command", tool); err != nil {
				t.Fatal(err)
			}
			positions := []ledger.Location{{At: location, Purpose: "范围边界"}}
			saveRequirement(t, service, taskActor("req"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "范围边界得到验证", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &positions})
			before, err := service.Workspace.Capture(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Check(t.Context(), taskActor("dynamic"), check.RunInput{Depth: "tests"})
			if err != nil || result.Status != ledger.CheckComplete || result.Report.Dynamic == nil {
				t.Fatal(result.Status, result.Missing, err)
			}
			report := result.Report.Dynamic
			if !report.Complete || len(report.Observations) != 4 || len(report.Groups) != 1 || report.Reusable {
				t.Fatal(report)
			}
			group := report.Groups[0]
			if !group.Complete || len(group.Mutations) == 0 || len(group.Candidates) == 0 {
				t.Fatal(group)
			}
			boundaryKept := false
			for _, observation := range report.Observations {
				if strings.Contains(observation.Case.Name, "edge") {
					boundaryKept = slices.Contains(group.Keep, observation.Case.At) && !slices.Contains(group.Candidates, observation.Case.At)
				}
			}
			if !boundaryKept {
				t.Fatal(group)
			}
			t1, t2 := false, false
			for _, issue := range result.Issues {
				t1 = t1 || issue.Rule == "T1"
				t2 = t2 || issue.Rule == "T2"
				if issue.BlockOnDelivery {
					t.Fatal(issue)
				}
			}
			if !t1 || !t2 {
				t.Fatal(result.Issues)
			}
			after, err := service.Workspace.Capture(t.Context())
			if err != nil || after.Tree != before.Tree {
				t.Fatal("workspace changed", err)
			}
			for _, name := range []string{".coverage", "coverage", "coverage.json", ".pytest_cache", "__pycache__", ".taskbelay-vitest.config.mjs"} {
				if _, err := os.Stat(filepath.Join(service.Workspace.Root, name)); !os.IsNotExist(err) {
					t.Fatal("workspace output", name, err)
				}
			}
			if result.Input.AnalysisSettings["analysis.tests"] != "off" {
				t.Fatal("default setting changed")
			}
		})
	}
}
