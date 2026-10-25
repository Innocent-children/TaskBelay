package eval_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestStaticTestsPreserveFrameworkLocationsAndDoNotRunProjectTools(t *testing.T) {
	service, task, message, _ := deliveryFixture(t)
	fixtures := []struct{ file, source, at string }{
		{"cases_test.go", `package main
import("testing";"os")
func TestRows(t *testing.T){t.Run("one",func(t *testing.T){os.WriteFile("analysis-was-run",nil,0600);if 1!=1{t.Fatal("wrong")}})}
`, "test:cases_test.go:TestRows/one"},
		{"tests/test_range.py", "class TestRange:\n def test_result(self):\n  assert 1 == 1\n", "test:tests/test_range.py:TestRange::test_result"},
		{"range.test.ts", "import {test,describe,expect} from 'vitest';\ndescribe('range',()=>{test('one',()=>expect(1).toBe(1));});\n", "test:range.test.ts:range > one"},
		{"src/test/java/RangeTest.java", "import org.junit.jupiter.api.Test;\nimport static org.junit.jupiter.api.Assertions.assertEquals;\nclass RangeTest {@Test void result(){assertEquals(1,1);}}\n", "test:src/test/java/RangeTest.java:RangeTest#result"},
		{"tests/range.rs", "#[test] fn result(){assert_eq!(1,1);}\n", "test:tests/range.rs:result"},
	}
	var locations []ledger.Location
	for _, fixture := range fixtures {
		verifiedTaskEdit(t, service, task.ID, "edit-"+fixture.file, fixture.file, "", fixture.source)
		locations = append(locations, ledger.Location{At: fixture.at, Purpose: "核对这些框架中的当前用例", SourceID: message.ID, Quote: "完成当前功能"})
	}
	requirement := saveRequirement(t, service, taskActor("requirement"), ledger.RequirementInput{Content: "完成当前功能", SourceID: message.ID, Quote: "完成当前功能", Acceptance: "当前用例位置可查询", ImplementedBy: &locations})
	result, err := service.Check(t.Context(), taskActor("static"), check.RunInput{Depth: "static"})
	if err != nil || result.Status != ledger.CheckComplete || len(result.Report.Tests) != len(fixtures) {
		t.Fatal(result.Status, result.Missing, result.Report.Tests, err)
	}
	view := getTask(t, service, taskActor("read"), task.ID)
	for i, location := range view.Requirements[0].ImplementedBy {
		if location.At != fixtures[i].at || location.Pending != "" || location.File != fixtures[i].file || location.AnalysisTree != result.Input.Tree {
			t.Fatal(location)
		}
	}
	for _, issue := range result.Issues {
		if strings.HasPrefix(issue.Rule, "T") && (issue.Level != "hint" || issue.BlockOnDelivery) {
			t.Fatal("test advice blocks", issue)
		}
		if issue.Rule == "T1" || issue.Rule == "T2" {
			t.Fatal("static analysis fabricated dynamic evidence", issue)
		}
	}
	if _, err := os.Stat(filepath.Join(service.Workspace.Root, "analysis-was-run")); !os.IsNotExist(err) {
		t.Fatal("static analysis ran project tests", err)
	}
	if len(view.Verifications) != 0 {
		t.Fatal("static analysis fabricated execution", view.Verifications)
	}
	if _, err := service.AddNote(t.Context(), taskActor("explain"), ledger.NoteInput{Kind: "decision", Text: "现有黑盒验收已核对，断言用途见需求位置", Target: &ledger.DecisionTarget{Kind: "requirement", ID: requirement.ID}}); err != nil {
		t.Fatal(err)
	}
	cached, err := service.Check(t.Context(), taskActor("cached"), check.RunInput{Depth: "static"})
	if err != nil || cached.ID != result.ID || len(cached.Report.ValidationNotes) != 1 || !strings.Contains(check.Format(cached, 0), "现有黑盒验收") {
		t.Fatal(cached, err)
	}
}

func TestStaticTestsKeepMixedUserAssertionsAndParameterHistory(t *testing.T) {
	service, task, message, _ := deliveryFixture(t)
	initial := "package main\nimport \"testing\"\nfunc TestRows(t *testing.T){\n t.Run(\"old\",func(t *testing.T){t.Fatal(\"agent assertion\")})\n}\n"
	verifiedTaskEdit(t, service, task.ID, "tests", "rows_test.go", "", initial)
	locations := []ledger.Location{{At: "test:rows_test.go:TestRows/old", Purpose: "参数回归"}}
	saveRequirement(t, service, taskActor("requirement"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "保留当前用例断言及历史", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &locations})
	first, err := service.Check(t.Context(), taskActor("first"), check.RunInput{})
	if err != nil || len(first.Report.Tests) != 1 || !first.Report.Tests[0].AgentOnly {
		t.Fatal(first, err)
	}
	user := strings.Replace(initial, "agent assertion", "user assertion", 1)
	write(t, service.Workspace.Root, "rows_test.go", user)
	second, err := service.Check(t.Context(), taskActor("after-user"), check.RunInput{})
	if err != nil || !second.Report.Tests[0].Mixed || second.Report.Tests[0].AgentOnly || second.ID == first.ID {
		t.Fatal(second, err)
	}
	if got := string(read(t, filepath.Join(service.Workspace.Root, "rows_test.go"))); got != user {
		t.Fatal("user assertion changed", got)
	}
	// 这个用例在任务开始后才加入，删除前证据来自中间观察点。
	removed := "package main\nimport \"testing\"\nfunc TestRows(t *testing.T){}\n"
	verifiedTaskEdit(t, service, task.ID, "remove-case", "rows_test.go", user, removed)
	view := getTask(t, service, taskActor("current"), task.ID)
	req := view.Requirements[0]
	locations[0].Change = "delete"
	saveRequirement(t, service, taskActor("deleted-location"), ledger.RequirementInput{ID: req.ID, BaseRevision: &req.Revision, Content: req.Content, Acceptance: req.Acceptance, SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &locations})
	result, err := service.Check(t.Context(), taskActor("deleted-check"), check.RunInput{})
	if err != nil || result.Report.Requirements[0].ImplementedBy[0].Pending != "" || result.Report.Requirements[0].ImplementedBy[0].BeforeBlob == "" {
		t.Fatal(result.Report.Requirements, result.Missing, err)
	}
}
