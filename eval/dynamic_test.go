package eval_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func TestGoDynamicCoverageMutationAndReuseStayInCopy(t *testing.T) {
	service, task, message, _ := deliveryFixture(t)
	files := map[string]string{
		"go.mod":   "module example.test/dynamic\n\ngo 1.26\n",
		"range.go": "package main\nfunc Allowed(n int) bool { return n < 5 }\nfunc Other(n int) bool { return n > 0 }\n",
		"range_test.go": `package main
import "testing"
func TestRange(t *testing.T) {
 rows := []struct{name string; n int; want bool}{{"inside",1,true},{"same",2,true},{"edge",5,false}}
 for _,row:=range rows {t.Run(row.name,func(t *testing.T){if Allowed(row.n)!=row.want{t.Errorf("got %v",Allowed(row.n))}})}
}

func TestOther(t *testing.T){if !Other(1){t.Fatal("other")}}
`,
	}
	for file, content := range files {
		verifiedTaskEdit(t, service, task.ID, "edit-"+file, file, "", content)
	}
	locations := []ledger.Location{{At: "range.go:Allowed", Purpose: "范围边界"}}
	saveRequirement(t, service, taskActor("requirements"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "范围边界得到验证", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &locations})
	before, err := service.Workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Check(t.Context(), taskActor("dynamic"), check.RunInput{Depth: "tests"})
	if err != nil || result.Status != ledger.CheckComplete || result.Report.Dynamic == nil {
		t.Fatal(result.Status, result.Missing, err)
	}
	report := result.Report.Dynamic
	stages := map[string]int{}
	for _, stage := range report.Stages {
		if stage.Result.Process == nil || !stage.Result.Process.Started || stage.Result.Process.Duration <= 0 {
			t.Fatal("successful native check is missing process evidence", stage)
		}
		stages[stage.Name]++
	}
	for _, phase := range []string{"prepare", "compile", "baseline", "mutation"} {
		if stages[phase] == 0 {
			t.Fatal("expected phase was not recorded", phase, stages)
		}
	}
	if !report.Complete || len(report.Observations) != 4 || len(report.Groups) != 1 {
		t.Fatal(report)
	}
	group := report.Groups[0]
	if !group.Complete || len(group.Mutations) == 0 || !slices.Contains(group.Keep, "test:range_test.go:TestRange/edge") || slices.Contains(group.Candidates, "test:range_test.go:TestRange/edge") || len(group.Candidates) == 0 {
		t.Fatal(group)
	}
	t1, t2 := false, false
	for _, issue := range result.Issues {
		t1 = t1 || issue.Rule == "T1"
		t2 = t2 || issue.Rule == "T2"
		if issue.Level != "hint" || issue.BlockOnDelivery {
			t.Fatal(issue)
		}
	}
	if !t1 || !t2 {
		t.Fatal(result.Issues)
	}
	after, err := service.Workspace.Capture(t.Context())
	if err != nil || after.Tree != before.Tree {
		t.Fatal("analysis changed workspace", err)
	}
	if result.Input.AnalysisSettings["analysis.tests"] != "off" {
		t.Fatal("one request changed default setting")
	}
	if report.Reusable {
		cached, err := service.Check(t.Context(), taskActor("reuse"), check.RunInput{Depth: "tests"})
		if err != nil || cached.ID != result.ID {
			t.Fatal("dynamic result was not reused", cached.Status, cached.Missing, err)
		}
		original, _ := json.Marshal(report.Stages)
		restored, _ := json.Marshal(cached.Report.Dynamic.Stages)
		if string(original) != string(restored) {
			t.Fatal("persisted stage resources changed when reusing the check")
		}
		t.Logf("persisted phase command counts: %v; check sequence %d reused without new execution", stages, cached.ID)
	} else {
		t.Fatal("self-contained default Go fixture did not record reusable environment", report.Missing)
	}
}

func TestDynamicCancellationAndConcurrentUserEditKeepObservations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("该准备命令使用 POSIX shell；Windows 进程回收由原生进程测试另行核验")
	}
	for _, mode := range []string{"cancel", "edit"} {
		t.Run(mode, func(t *testing.T) {
			service, task, message, _ := deliveryFixture(t)
			for file, source := range map[string]string{"go.mod": "module example.test/interrupt\n\ngo 1.26\n", "range.go": "package main\nfunc Allowed(n int) bool {return n<5}\n", "range_test.go": "package main\nimport \"testing\"\nfunc TestAllowed(t *testing.T){if !Allowed(1){t.Fatal(\"expected true\")}}\n"} {
				verifiedTaskEdit(t, service, task.ID, "edit-"+file, file, "", source)
			}
			locations := []ledger.Location{{At: "range.go:Allowed", Purpose: "范围判断"}}
			saveRequirement(t, service, taskActor("req"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "当前内容可验证", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &locations})
			marker := filepath.Join(t.TempDir(), "prepared")
			quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
			delay := "0.3"
			if mode == "cancel" {
				delay = "10"
			}
			if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "test.go.prepare", "printf ready > "+quote(marker)+"; sleep "+delay); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			type completed struct {
				check ledger.CheckRecord
				err   error
			}
			done := make(chan completed, 1)
			go func() {
				record, err := service.Check(ctx, taskActor("running"), check.RunInput{Depth: "tests"})
				done <- completed{record, err}
			}()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, err := os.Stat(marker); err == nil {
					break
				}
				select {
				case result := <-done:
					t.Fatal("check stopped before preparation", result)
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("preparation did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if mode == "cancel" {
				cancel()
			} else {
				write(t, service.Workspace.Root, "range.go", "package main\nfunc Allowed(n int) bool {return n<3} // user\n")
			}
			result := <-done
			if result.err != nil {
				t.Fatal(result.err)
			}
			want := ledger.CheckPartial
			if mode == "edit" {
				want = ledger.CheckExpired
			}
			if result.check.Status != want || result.check.Report.Dynamic == nil {
				t.Fatal(result.check.Status, result.check.Missing)
			}
			if mode == "cancel" && result.check.Report.Dynamic.Complete {
				t.Fatal("canceled execution marked complete")
			}
			if mode == "edit" && !strings.Contains(string(read(t, filepath.Join(service.Workspace.Root, "range.go"))), "// user") {
				t.Fatal("user content changed")
			}
			stored, err := service.Store.LatestCheck(t.Context(), task.ID, "tests", "")
			if err != nil || stored == nil || stored.ID != result.check.ID {
				t.Fatal("completed observations were lost", err)
			}
		})
	}
}

func TestEnabledDynamicDeliveryRunsOnlyAfterFinish(t *testing.T) {
	service, task, message, _ := deliveryFixture(t)
	for file, source := range map[string]string{"go.mod": "module example.test/stop\n\ngo 1.26\n", "range.go": "package main\nfunc Allowed(n int) bool {return n<5}\n", "range_test.go": "package main\nimport \"testing\"\nfunc TestAllowed(t *testing.T){if !Allowed(1){t.Fatal(\"expected true\")}}\n"} {
		verifiedTaskEdit(t, service, task.ID, "edit-"+file, file, "", source)
	}
	locations := []ledger.Location{{At: "range.go:Allowed", Purpose: "范围判断"}, {At: "go.mod", Purpose: "模块配置"}}
	saveRequirement(t, service, taskActor("req"), ledger.RequirementInput{Content: "完成当前功能", Acceptance: "当前内容可验证", SourceID: message.ID, Quote: "完成当前功能", ImplementedBy: &locations})
	if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, "analysis.tests", "on"); err != nil {
		t.Fatal(err)
	}
	stopTask(t, service)
	if record, err := service.Store.LatestCheck(t.Context(), task.ID, "tests", ""); err != nil || record != nil {
		t.Fatal("ordinary stop started tests", record, err)
	}
	if _, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{}); err != nil {
		t.Fatal(err)
	}
	decision := stopTask(t, service)
	if decision.BlockStop != "" || !strings.Contains(decision.UserMessage, "tests 检查：complete") {
		t.Fatal(decision)
	}
	view := getTask(t, service, taskActor("view"), task.ID)
	if view.Task.State != ledger.TaskCompleted || len(view.Checks) != 2 {
		t.Fatal(view.Task, view.Checks)
	}
}

func TestDynamicFeedbackRetainsIncompleteObservationAndRefreshesOldRules(t *testing.T) {
	for _, changeRules := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-feedback", true: "changed-rules"}[changeRules], func(t *testing.T) {
			service, task, _, base := deliveryFixture(t)
			verifiedTaskEdit(t, service, task.ID, "entry", "main.go", base, "package main\nimport \"flag\"\nfunc main(){_ = flag.String(\"extra\",\"x\",\"help\")}\n")
			verifiedTaskEdit(t, service, task.ID, "python", "test_value.py", "", "def test_value():\n    assert 1 == 1\n")
			for name, value := range map[string]string{"analysis.tests": "on", "test.python.command": "taskbelay-missing-python pytest"} {
				if err := service.Store.SetSetting(t.Context(), service.Workspace.ID, ledger.Task, name, value); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Finish(t.Context(), taskActor("finish"), ledger.FinishInput{}); err != nil {
				t.Fatal(err)
			}
			if decision := stopTask(t, service); !strings.Contains(decision.BlockStop, "D3") {
				t.Fatal(decision)
			}
			one, err := service.Store.LatestCheck(t.Context(), task.ID, "tests", "")
			if err != nil || one == nil || one.Status != ledger.CheckPartial || one.Report.Dynamic == nil || one.Report.Dynamic.ExecutionID != testrun.DynamicVersion {
				t.Fatal("Python failure must retain observations without computing Go identity", one, err)
			}
			if changeRules {
				one.Report.Dynamic.Rules = "older-rules"
				stored, err := service.Store.SaveCheck(t.Context(), service.Workspace.ID, taskActor("old-rules"), one.Input.Tree, *one)
				if err != nil {
					t.Fatal(err)
				}
				one = &stored
			} else {
				one.StartedAt = one.StartedAt.Add(-time.Hour)
				stored, err := service.Store.SaveCheck(t.Context(), service.Workspace.ID, taskActor("cached-earlier-observation"), one.Input.Tree, *one)
				if err != nil {
					t.Fatal(err)
				}
				one = &stored
			}
			if decision := stopTask(t, service); decision.BlockStop != "" || !strings.Contains(decision.UserMessage, "tests 检查：partial") {
				t.Fatal(decision)
			}
			two, err := service.Store.LatestCheck(t.Context(), task.ID, "tests", "")
			if err != nil || two == nil || (one.ID != two.ID) != changeRules || two.Report.Dynamic.Rules != testrun.DynamicVersion {
				t.Fatal("feedback reuse did not respect rules", one, two, err)
			}
			if !changeRules && !one.StartedAt.Equal(two.StartedAt) {
				t.Fatal("reused observation changed its execution time")
			}
		})
	}
}
