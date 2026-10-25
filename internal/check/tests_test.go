package check

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func testRequirement(at string) ledger.Requirement {
	return ledger.Requirement{ID: "R1", Status: "in_progress", ImplementedBy: []ledger.Location{{At: at}}}
}

func riskFor(t *testing.T, result ledger.CheckRecord) ledger.TestRisk {
	t.Helper()
	if len(result.Report.TestRisk) != 1 {
		t.Fatal(result.Report.TestRisk)
	}
	return result.Report.TestRisk[0]
}

func TestRiskUsesChangedBehaviorLinesAndNamedUnchangedDeclaration(t *testing.T) {
	source := `package p
import "strconv"
func parse(input string) int {
 go func(){}()
 n, _ := strconv.Atoi(input)
 if n > 0 { return n }
 return -1
}
`
	old := map[string]string{"parse.go": source}
	current := map[string]string{"parse.go": strings.Replace(source, "return -1", "return -2", 1)}
	requirement := testRequirement("parse.go:parse")
	risk := riskFor(t, staticFixture(t, old, current, []ledger.Requirement{requirement}))
	if !risk.Complete || risk.Recommended != 1 || len(risk.Sources) != 0 || risk.Branches != 0 {
		t.Fatal("unchanged risks leaked", risk)
	}
	risk = riskFor(t, staticFixture(t, old, old, []ledger.Requirement{requirement}))
	if !risk.Complete || risk.Recommended != 5 || risk.Branches != 1 {
		t.Fatal("named unchanged declaration not analyzed in full", risk)
	}
}

func TestRiskCountsCategoriesOnceAndCapsAtEight(t *testing.T) {
	source := `package p
import("os";"os/exec";"strconv")
func work(input string) int {
 go func(){}()
 go func(){}()
 os.WriteFile("a",nil,0600)
 os.WriteFile("b",nil,0600)
 n,_ := strconv.Atoi(input)
 exec.Command("tool").Run()
 if n<0{return 0}
 if n==0{return 1}
 if n==1{return 2}
 if n==2{return 3}
 return n
}
`
	current := map[string]string{"work.go": source}
	risk := riskFor(t, staticFixture(t, nil, current, []ledger.Requirement{testRequirement("work.go:work")}))
	if risk.Recommended != 8 || risk.Branches != 4 || !risk.Complete {
		t.Fatal(risk)
	}
	categories := map[string]bool{}
	for _, source := range risk.Sources {
		categories[source.Kind] = true
	}
	if len(categories) != 4 {
		t.Fatal(risk)
	}
}

func TestT3CountsAddedAndUserRetainedSeparately(t *testing.T) {
	current := map[string]string{"work.go": "package p\nfunc work(x int) int {return x+1}\n"}
	var tests strings.Builder
	tests.WriteString("package p\nimport \"testing\"\n")
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&tests, "func Test%d(t *testing.T){if work(%d)!=%d {t.Fatal(\"result\")}}\n", i, i, i+1)
	}
	current["work_test.go"] = tests.String()
	req := testRequirement("work.go:work")
	result := staticFixture(t, nil, current, []ledger.Requirement{req})
	risk := riskFor(t, result)
	if risk.Added != 3 || risk.Recommended != 1 || len(issuesFor(result, "T3")) != 1 {
		t.Fatal(risk, result.Report.Tests, result.Issues)
	}
	for i := 0; i < 2; i++ {
		req.ImplementedBy = append(req.ImplementedBy, ledger.Location{At: fmt.Sprintf("test:work_test.go:Test%d", i), Purpose: "用户要求守护这个输入契约", Citation: &ledger.Citation{SourceID: "m1", Quote: "保留"}})
	}
	result = staticFixture(t, nil, current, []ledger.Requirement{req})
	risk = riskFor(t, result)
	if risk.Retained != 2 || risk.Added != 3 || len(issuesFor(result, "T3")) != 0 {
		t.Fatal(risk, result.Issues)
	}
	for _, issue := range result.Issues {
		if strings.HasPrefix(issue.Rule, "T") && (issue.Level != "hint" || issue.BlockOnDelivery) {
			t.Fatal("test suggestion blocks", issue)
		}
	}
}

func TestOT02OT05T4KeepsDefaultsAndPublicContracts(t *testing.T) {
	current := map[string]string{
		"state.go":      "package p\ntype State struct{enabled bool}\nfunc (s State) Enabled() bool{return s.enabled}\nfunc NewState() *State {return &State{enabled:false}}\n",
		"state_test.go": "package p\nimport \"testing\"\nfunc TestDefault(t *testing.T){if NewState().Enabled(){t.Fatal(\"unsafe default\")}}\n",
	}
	req := testRequirement("state.go:NewState")
	req.ImplementedBy = append(req.ImplementedBy, ledger.Location{At: "test:state_test.go:TestDefault", Purpose: "安全默认值必须关闭", Citation: &ledger.Citation{SourceID: "m1", Quote: "默认关闭"}})
	result := staticFixture(t, nil, current, []ledger.Requirement{req})
	issues := issuesFor(result, "T4")
	if len(issues) != 1 || issues[0].Status != "retained" || issues[0].Level != "hint" || !strings.Contains(issues[0].Advice, "公开约定") {
		for id, d := range snapshots(t, current).Declarations() {
			t.Log(id, d.Shape)
		}
		t.Fatal(result.Issues, result.Report.Tests)
	}
}

func TestOT06T5OnlyDirectMockAssertionsAndPurposeRetention(t *testing.T) {
	current := map[string]string{"charge_test.go": `package p
import("testing";"github.com/stretchr/testify/mock")
func TestNoRepeat(t *testing.T){var m mock.Mock;m.AssertNumberOfCalls(t,"Charge",1)}
func TestResult(t *testing.T){var m mock.Mock;m.AssertCalled(t,"Charge");if value()!=1{t.Fatal("result")}}
`}
	req := testRequirement("test:charge_test.go:TestNoRepeat")
	req.ImplementedBy[0].Purpose = "不能重复扣款，调用次数就是需求"
	req.ImplementedBy[0].Citation = &ledger.Citation{SourceID: "m1", Quote: "不能重复扣款"}
	result := staticFixture(t, nil, current, []ledger.Requirement{req})
	issues := issuesFor(result, "T5")
	if len(issues) != 1 || issues[0].At != "test:charge_test.go:TestNoRepeat" || issues[0].Status != "retained" {
		t.Fatal(result.Issues)
	}
}

func TestOT04OT07T6ExplicitExistingTestAndUnchangedProduction(t *testing.T) {
	base := map[string]string{
		"parse.go":      "package p\nfunc parse(x int) int {if x<0{return 0};return x+1}\n",
		"parse_test.go": "package p\nimport \"testing\"\nfunc TestParse(t *testing.T){if parse(1)!=2{t.Fatal(\"result\")}}\n",
	}
	req := testRequirement("parse.go:parse")
	result := staticFixture(t, base, base, []ledger.Requirement{req})
	if len(issuesFor(result, "T6")) != 1 || riskFor(t, result).Existing != 1 {
		t.Fatal(result.Issues, result.Report)
	}
	req.ImplementedBy = append(req.ImplementedBy, ledger.Location{At: "test:parse_test.go:TestParse", Purpose: "现有主路径断言已验证当前要求"})
	result = staticFixture(t, base, base, []ledger.Requirement{req})
	if len(issuesFor(result, "T6")) != 0 {
		t.Fatal(result.Issues)
	}
	current := map[string]string{"parse.go": base["parse.go"], "parse_test.go": base["parse_test.go"] + "func TestNegative(t *testing.T){if parse(-1)!=0{t.Fatal(\"boundary\")}}\n"}
	result = staticFixture(t, base, current, []ledger.Requirement{testRequirement("parse.go:parse")})
	if len(issuesFor(result, "T6")) != 0 || riskFor(t, result).Added != 1 {
		t.Fatal(result.Issues, result.Report)
	}
}

func TestOT12DeletionUsesBeforeCodeAndBlackBoxPurpose(t *testing.T) {
	base := map[string]string{"old.go": "package p\nfunc oldCommand(x int) int {if x>0{return x};return -1}\n"}
	current := map[string]string{"old.go": "package p\n", "cli_test.go": `package p
import("testing";"os/exec")
func TestOldAbsent(t *testing.T){if exec.Command("app","old").Run()==nil{t.Fatal("old entry still exists")}}
`}
	req := testRequirement("old.go:oldCommand")
	req.ImplementedBy[0].Change = "delete"
	req.ImplementedBy = append(req.ImplementedBy, ledger.Location{At: "test:cli_test.go:TestOldAbsent", Purpose: "黑盒确认旧命令已经移除"})
	result := staticFixture(t, base, current, []ledger.Requirement{req})
	if riskFor(t, result).Added != 1 || len(issuesFor(result, "T6")) != 0 || len(issuesFor(result, "T1")) != 0 || len(issuesFor(result, "T2")) != 0 {
		t.Fatal(result.Report, result.Issues)
	}
}

func TestTestReportsKeepMixedChangesAndSeparateSubtestRequirements(t *testing.T) {
	current := map[string]string{"work.go": "package p\nfunc a(){}\nfunc b(){}\n", "work_test.go": `package p
import "testing"
func TestRows(t *testing.T){
 t.Run("a",func(t *testing.T){a()})
 t.Run("b",func(t *testing.T){b()})
}
`}
	before, after := snapshots(t, nil), snapshots(t, current)
	changes, _ := codescan.Compare(t.Context(), before, after, nil)
	req := testRequirement("test:work_test.go:TestRows/a")
	association, err := codescan.Associate(t.Context(), before, after, changes, []codescan.Position{{Requirement: "R1", At: req.ImplementedBy[0].At}})
	if err != nil {
		t.Fatal(err)
	}
	if len(association.Requirements["test:work_test.go:TestRows/b"]) != 0 || len(association.Requirements["work.go:b"]) != 0 {
		t.Fatal("subtest relationship leaked", association)
	}
	sources := provenance{}
	for path, file := range after.Files {
		lines := sourceLines(file.Data, "agent")
		if path == "work_test.go" {
			lines[3].kind = "workspace"
		}
		sources[path] = fileSources{gitstate.File{Blob: file.Blob, Mode: file.Mode}, lines}
	}
	result, err := staticCheck(t.Context(), ledger.CheckInput{}, ledger.TaskView{Task: &ledger.TaskRecord{}, Requirements: []ledger.Requirement{req}}, before, after, changes, association, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Report.Tests) != 2 || !result.Report.Tests[0].Mixed || result.Report.Tests[0].AgentOnly || !slices.Contains(result.Report.Tests[0].Requirements, "R1") {
		t.Fatal(result.Report.Tests)
	}
}

func TestT6DoesNotDemandTestsForNamesAndSimpleValues(t *testing.T) {
	base := map[string]string{"value.go": "package p\nfunc value() int {return 1}\n"}
	current := map[string]string{"value.go": "package p\nfunc value() int {return 2}\n"}
	result := staticFixture(t, base, current, []ledger.Requirement{testRequirement("value.go:value")})
	if riskFor(t, result).Logic || len(issuesFor(result, "T6")) != 0 {
		t.Fatal(result.Report, result.Issues)
	}
}

func TestChangedTestsCompareButOnlyAddedTestsReceiveSimplificationHints(t *testing.T) {
	source := `package p
import("testing";"github.com/stretchr/testify/mock")
func TestOld(t *testing.T){var m mock.Mock;m.AssertCalled(t,"save",1)}
`
	base := map[string]string{"save_test.go": source}
	current := map[string]string{"save_test.go": strings.Replace(source, "\"save\",1", "\"save\",2", 1) + "func TestNew(t *testing.T){var m mock.Mock;m.AssertCalled(t,\"save\",3)}\n"}
	result := staticFixture(t, base, current, nil)
	if len(result.Report.Tests) != 2 || result.Report.Tests[0].Change != "add" || result.Report.Tests[1].Change != "modify" {
		t.Fatal(result.Report.Tests)
	}
	if issues := issuesFor(result, "T5"); len(issues) != 1 || issues[0].At != "test:save_test.go:TestNew" {
		t.Fatal(issues)
	}
}
