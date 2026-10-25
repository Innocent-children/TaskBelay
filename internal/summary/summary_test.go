package summary

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestTokenBudgetAndUnicodeBoundaries(t *testing.T) {
	for text, want := range map[string]int{"": 0, "abcd": 1, "abcde": 2, "中文abcd": 3, "🙂𠀀": 2} {
		if got := Tokens(text); got != want {
			t.Fatalf("%q = %d, want %d", text, got, want)
		}
	}
	for _, text := range []string{"中文 mixed ASCII 🙂𠀀", strings.Repeat("a", 500), strings.Repeat("中", 500)} {
		for budget := range 40 {
			got := Truncate(text, budget)
			if !utf8.ValidString(got) || Tokens(got) > budget {
				t.Fatalf("invalid truncation at %d: %q", budget, got)
			}
		}
	}
}

func TestSummaryRetainsFixedContextAndLimitsEverySection(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	long := strings.Repeat("很长的需求🙂 ASCII ", 300)
	view := ledger.TaskView{Task: &ledger.TaskRecord{ID: "t17", Title: long, State: ledger.TaskActive, UpdatedAt: now}, Settings: ledger.EffectiveSettings{Values: map[string]ledger.Setting{"workflow": {Value: "semi"}}}}
	for i := range 20 {
		view.Requirements = append(view.Requirements, ledger.Requirement{ID: fmt.Sprintf("R%d", i+1), Content: long, Status: "pending", At: now.Add(time.Duration(i) * time.Minute)})
		view.WorkspaceUpdates = append(view.WorkspaceUpdates, ledger.FileChange{Path: fmt.Sprintf("src/file%d.go", i), AfterObservation: int64(i + 1), After: gitstate.File{Blob: "blob"}, At: now})
		view.EffectiveDecisions = append(view.EffectiveDecisions, ledger.Note{Text: long})
	}
	view.Notes = []ledger.Note{{Kind: "next_step", Text: long, At: now}}
	input := Input{View: view, NextStepChanged: true, Peers: []ledger.SessionActivity{{Agent: "claude", SessionID: "other"}}}
	for i := range 20 {
		input.ChangedFiles = append(input.ChangedFiles, fmt.Sprintf("file%d.go", i))
	}
	text := Build(input, true)
	if Tokens(text) > Budget || !utf8.ValidString(text) || !strings.HasSuffix(text, Footer) {
		t.Fatalf("budget or footer: %d\n%s", Tokens(text), text)
	}
	for _, part := range []string{"任务 t17", "工作模式：半自动", "方案状态：", "共 20 条", "省略", "先读当前内容", "笔记后工作区已有变化", "尚未检查"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %s", part)
		}
	}
	markers := []string{"任务 t17", "当前需求：", "方案状态：", "最新工作区更新", "下一步（", "仍有效的用户决定：", "累计改动文件：", Footer}
	limits := []int{120, 500, 180, 250, 120, 200, 150}
	first := strings.Index(text, markers[0])
	if Tokens(strings.TrimSpace(text[:first])) > GuidanceBudget {
		t.Fatal("guidance exceeded 350 tokens")
	}
	for i, limit := range limits {
		start, end := strings.Index(text, markers[i]), strings.Index(text, markers[i+1])
		if start < 0 || end <= start || Tokens(strings.TrimSpace(text[start:end])) > limit {
			t.Fatalf("section %d exceeded %d tokens", i, limit)
		}
	}
	shown := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "R") && strings.Contains(line, "未开始") {
			shown++
		}
	}
	if shown > 8 || !strings.Contains(text, fmt.Sprintf("省略 %d 条", 20-shown)) {
		t.Fatal("incorrect requirement omission count", shown)
	}
	if strings.Contains(text, "R1 未开始") || !strings.Contains(text, "R20 未开始") {
		t.Fatal("recent requirements were not preferred")
	}
}

func TestDecisionsAndIssuesShareFiveItemLimit(t *testing.T) {
	for _, decisions := range []int{0, 2, 5, 6} {
		t.Run(fmt.Sprintf("%d decisions", decisions), func(t *testing.T) {
			view := ledger.TaskView{}
			for i := range decisions {
				view.EffectiveDecisions = append(view.EffectiveDecisions, ledger.Note{Text: fmt.Sprintf("decision-%d", i)})
			}
			check := ledger.CheckRecord{Status: ledger.CheckComplete}
			for i := range 6 {
				check.Issues = append(check.Issues, ledger.Issue{Rule: "D1", At: fmt.Sprintf("file.go:issue%d", i), Status: "open"})
			}
			view.Checks = []ledger.CheckRecord{check}
			text := Build(Input{View: view}, true)
			start, end := strings.Index(text, "仍有效的用户决定："), strings.Index(text, "累计改动文件：")
			if start < 0 || end < start {
				t.Fatal(text)
			}
			part := strings.TrimSpace(text[start:end])
			shownDecisions, shownIssues := 0, 0
			for _, line := range strings.Split(part, "\n") {
				if strings.HasPrefix(line, "decision-") {
					shownDecisions++
				}
				if strings.HasPrefix(line, "D1 file.go:issue") {
					shownIssues++
				}
			}
			if shownDecisions != min(decisions, 5) || shownDecisions+shownIssues != 5 {
				t.Fatalf("decisions and issues must share five entries, got %d + %d\n%s", shownDecisions, shownIssues, part)
			}
			if !strings.Contains(part, "有 6 处") || Tokens(part) > 200 || Tokens(text) > Budget || !strings.HasSuffix(text, Footer) {
				t.Fatal("lost issue count, budget or full ledger entry", text)
			}
		})
	}
}

func TestVersionIgnoresObservationAndMessageChurn(t *testing.T) {
	input := Input{View: ledger.TaskView{Task: &ledger.TaskRecord{ID: "t1", Revision: 2}}}
	before := Version(input)
	input.Observation.ID = 500
	input.Observation.At = time.Now()
	input.View.Task.LastObservationID = 500
	input.View.Messages = []ledger.Message{{ID: "new-message"}}
	if Version(input) != before {
		t.Fatal("unchanged observation or user message changed summary version")
	}
	input.Observation.Context.HEAD = "new-head"
	if Version(input) == before {
		t.Fatal("commit context was ignored")
	}
}

func TestVersionTracksCheckFactsButNotRefreshTime(t *testing.T) {
	input := Input{View: ledger.TaskView{LatestCheck: "static: partial", Checks: []ledger.CheckRecord{{
		ID: 1, Depth: "static", InputVersion: "same-input", Status: ledger.CheckPartial,
		Issues: []ledger.Issue{{ID: "i1", Rule: "D1", At: "main.go:unused", FactKey: "unused", Status: "open", Facts: []string{"内部函数没有引用"}}},
	}}}}
	before := Version(input)
	input.View.Checks[0].ID = 2
	input.View.Checks[0].FinishedAt = time.Now()
	if Version(input) != before {
		t.Fatal("refresh metadata produced a duplicate summary")
	}
	input.View.Checks[0].Issues[0].Status = "retained"
	if Version(input) == before {
		t.Fatal("changed issue was omitted from summary version")
	}
	before = Version(input)
	input.View.Checks[0].Missing = []ledger.CheckGap{{At: "other.go", Operation: "references", Reason: "无法完整解析"}}
	if Version(input) == before {
		t.Fatal("changed unfinished scope was omitted from summary version")
	}
}

func TestPlanSummaryKeepsBothVersionsWithinBudget(t *testing.T) {
	view := ledger.TaskView{Task: &ledger.TaskRecord{ID: "t1", State: ledger.TaskActive}, Settings: ledger.EffectiveSettings{Values: map[string]ledger.Setting{"workflow": {Value: "semi"}}}, Plans: ledger.PlanView{
		Effective: &ledger.PlanRecord{Version: 1, Path: "/data/" + strings.Repeat("方案", 150) + "/v1.md", Files: []ledger.PlanFile{{Path: "internal/export/"}}},
		Pending:   &ledger.PlanRecord{Version: 2, Path: "/data/" + strings.Repeat("方案", 150) + "/v2.md", Changes: []string{"新增 JSON 导出"}},
	}}
	text := Build(Input{View: view}, true)
	start, end := strings.Index(text, "方案状态："), strings.Index(text, "工作区更新：")
	if start < 0 || end < start {
		t.Fatal(text)
	}
	part := text[start:end]
	if Tokens(strings.TrimSpace(part)) > 180 || Tokens(text) > Budget {
		t.Fatal("plan summary exceeded budget", part)
	}
	for _, expected := range []string{"有效 v1", "待确认 v2", "有效路径：", "待确认路径：", "范围：internal/export/", "变更："} {
		if !strings.Contains(part, expected) {
			t.Fatal("plan context missing "+expected, part)
		}
	}
}
