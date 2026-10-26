package check

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

func Run(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, actor ledger.Actor, view ledger.TaskView, observation ledger.Observation, options RunInput) (ledger.CheckRecord, error) {
	ledger.DebugRecord(ctx, "check.start", map[string]any{"options": options, "task": view.Task, "observation": observation, "rules": RulesVersion, "capabilities": CapabilitiesVersion, "requirements": view.Requirements, "decisions": view.EffectiveDecisions})
	if options.Depth != "" && options.Depth != "static" && options.Depth != "tests" {
		return ledger.CheckRecord{}, fmt.Errorf("depth 只能为 static 或 tests")
	}
	static, err := runStatic(ctx, store, workspace, actor, view, observation, RunInput{Depth: "static", Refresh: options.Refresh})
	ledger.DebugRecord(ctx, "check.static.end", map[string]any{"record": static, "returned": err == nil})
	if err != nil || options.Depth != "tests" {
		return static, err
	}
	dynamic, err := runDynamic(ctx, store, workspace, actor, view, static, options)
	dynamic.Static = &static
	ledger.DebugRecord(ctx, "check.dynamic.end", map[string]any{"record": dynamic, "returned": err == nil})
	return dynamic, err
}

func runDynamic(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, actor ledger.Actor, view ledger.TaskView, static ledger.CheckRecord, options RunInput) (ledger.CheckRecord, error) {
	started := time.Now().UTC()
	ctx, cancel := context.WithTimeout(ctx, event.TestsBudget)
	defer cancel()
	deadline, _ := ctx.Deadline()
	work, end := context.WithDeadline(ctx, deadline.Add(-event.MCPBudget))
	defer end()
	result := ledger.CheckRecord{Depth: "tests", Input: static.Input, InputVersion: static.InputVersion, StartedAt: started, Status: ledger.CheckComplete, Report: static.Report}
	result.Report.NotRun = slices.DeleteFunc(slices.Clone(result.Report.NotRun), func(value string) bool { return value == "动态测试未运行" })
	missing := func(reason string) {
		result.Status = ledger.CheckPartial
		result.Missing = append(result.Missing, ledger.CheckGap{Operation: "tests", Reason: reason})
	}
	if static.Status == ledger.CheckExpired {
		missing("静态输入已过期，本次不启动测试")
	} else {
		paused := false
		for _, note := range view.EffectiveDecisions {
			if note.Target != nil && note.Target.Effect == "pause_checks" {
				paused = true
			}
		}
		if paused {
			missing("用户已暂停当前检查")
		} else {
			goIdentity := len(static.Report.Tests) > 0 && static.Input.AnalysisSettings["test.go.command"] == "" && static.Input.AnalysisSettings["test.go.prepare"] == ""
			for _, test := range static.Report.Tests {
				goIdentity = goIdentity && test.Framework == "go"
			}
			identity := testrun.DynamicVersion
			var identityErr error
			if goIdentity {
				identity, identityErr = testrun.ExecutionIdentity(work, workspace.Root)
			}
			if goIdentity && !options.Refresh && identityErr == nil {
				cached, err := store.LatestCheck(ctx, static.Input.TaskID, "tests", static.InputVersion)
				if err != nil {
					return result, err
				}
				if cached != nil && cached.Status == ledger.CheckComplete && cached.Report.Dynamic != nil && cached.Report.Dynamic.Rules == testrun.DynamicVersion && cached.Report.Dynamic.Reusable && cached.Report.Dynamic.ExecutionID == identity {
					current, err := workspace.Capture(ctx)
					currentInput, inputErr := store.CheckInput(ctx, workspace.ID, static.Input.TaskID, actor, static.Input.Rules, static.Input.Capabilities)
					if err == nil && inputErr == nil && currentInput.Version() == static.InputVersion && current.Complete() && current.Tree == static.Input.Tree {
						cached.Report.Verifications = view.Verifications
						cached.Report.ValidationNotes = validationNotes(view)
						ledger.DebugRecord(ctx, "check.cache", map[string]any{"depth": "tests", "reused": true, "record_id": cached.ID, "input_version": static.InputVersion})
						return *cached, nil
					}
				}
			}
			ledger.DebugRecord(ctx, "check.cache", map[string]any{"depth": "tests", "reused": false, "refresh": options.Refresh})
			request, err := dynamicRequest(work, workspace, static)
			if err != nil {
				missing(err.Error())
			} else {
				if len(request.Cases) == 0 {
					result.Report.NotRun = append(result.Report.NotRun, "固定源码没有可定位的测试用例；没有覆盖或变异结论")
				}
				report := testrun.Analyze(work, workspace, request)
				report.ExecutionID = identity
				report.Reusable = report.Reusable && goIdentity
				if identityErr != nil {
					report.Reusable = false
					report.Missing = append(report.Missing, "工具与环境标识未完整核对，结果不复用")
				}
				if report.Reusable {
					currentIdentity, err := testrun.ExecutionIdentity(work, workspace.Root)
					if err != nil || currentIdentity != identity {
						report.Complete = false
						report.Reusable = false
						report.Missing = append(report.Missing, "检查期间工具或环境变化，当前动态观察未完成")
					}
				}
				result.Report.Dynamic = &report
				if !report.Complete {
					for i := range report.Groups {
						report.Groups[i].Candidates = nil
					}
					for _, reason := range report.Missing {
						missing(reason)
					}
					if len(report.Missing) == 0 {
						missing("动态分组检查未完成")
					}
				}
				addDynamicIssues(&result, request)
			}
		}
	}
	// 执行阶段预留最后一秒发布观察；取消只停止运行，已有证据仍作有限保存。
	publish, endPublish := context.WithDeadline(context.WithoutCancel(ctx), minTime(deadline, time.Now().Add(event.MCPBudget)))
	defer endPublish()
	current, err := workspace.Capture(publish)
	tree := current.Tree
	if err != nil || !current.Complete() {
		tree = ""
		missing("当前工作区复核未完成")
	}
	return store.SaveCheck(publish, workspace.ID, actor, tree, result)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func dynamicRequest(ctx context.Context, workspace gitstate.Workspace, static ledger.CheckRecord) (testrun.Request, error) {
	after, err := AnalyzeTree(ctx, workspace, static.Input.Tree)
	if err != nil {
		return testrun.Request{}, err
	}
	before, err := AnalyzeTree(ctx, workspace, static.Input.OriginTree)
	if err != nil {
		return testrun.Request{}, err
	}
	changes, err := codescan.Compare(ctx, before, after, nil)
	if err != nil {
		return testrun.Request{}, err
	}
	request := testrun.Request{Snapshot: after, Requirements: map[string][]testrun.Line{}, Changed: map[string][]testrun.Line{}, Config: map[string]testrun.Config{}}
	var positions []codescan.Position
	for _, requirement := range static.Report.Requirements {
		for _, location := range requirement.ImplementedBy {
			if location.Pending == "" {
				positions = append(positions, codescan.Position{Requirement: requirement.ID, At: location.At, Change: location.Change, Purpose: location.Purpose})
			}
		}
	}
	association, err := codescan.Associate(ctx, before, after, changes, positions)
	if err != nil {
		return request, err
	}
	for _, language := range []string{"go", "python", "javascript", "typescript", "java", "rust"} {
		parallel, _ := strconv.Atoi(static.Input.AnalysisSettings["test."+language+".parallel"])
		request.Config[language] = testrun.Config{Command: static.Input.AnalysisSettings["test."+language+".command"], Prepare: static.Input.AnalysisSettings["test."+language+".prepare"], Parallel: parallel}
	}
	for _, test := range snapshotTests(after) {
		for _, fact := range static.Report.Tests {
			if fact.At != test.Location() {
				continue
			}
			item := testrun.Case{Source: test, At: fact.At, Name: test.Name, Change: fact.Change, Requirements: fact.Requirements, AgentOnly: fact.AgentOnly, Mixed: fact.Mixed, Retained: fact.Retained, Unknown: fact.Unknown}
			for _, assertion := range fact.Assertions {
				item.Assertions = append(item.Assertions, fmt.Sprintf("%s 第 %d 行: %s", assertion.Kind, assertion.Line, assertion.Evidence))
			}
			request.Cases = append(request.Cases, item)
			break
		}
	}
	for id, declaration := range after.Declarations() {
		if declaration.Test || excluded(after.Files[declaration.File]) {
			continue
		}
		requirements := slices.Clone(association.Requirements[id])
		for _, position := range positions {
			if position.At == declaration.Location() {
				requirements = append(requirements, position.Requirement)
			}
		}
		slices.Sort(requirements)
		requirements = slices.Compact(requirements)
		for _, requirement := range requirements {
			for _, scope := range declaration.Ranges() {
				for line := scope.StartLine; line <= scope.EndLine; line++ {
					request.Requirements[requirement] = append(request.Requirements[requirement], testrun.Line{File: declaration.File, Number: line})
				}
			}
		}
	}
	for _, change := range changes {
		declaration := change.After
		if declaration == nil || declaration.Test || excluded(after.Files[declaration.File]) || slices.Contains([]string{"format", "comment", "names"}, change.Detail) {
			continue
		}
		var changed map[int]bool
		if change.Before != nil {
			var complete bool
			_, changed, complete = codescan.BehaviorLines(ctx, *change.Before, *declaration)
			if !complete {
				return request, fmt.Errorf("行为变化范围尚未完整核对")
			}
		}
		for _, requirement := range association.Requirements[declaration.ID] {
			for _, scope := range declaration.Ranges() {
				for line := scope.StartLine; line <= scope.EndLine; line++ {
					if changed == nil || changed[line] {
						request.Changed[requirement] = append(request.Changed[requirement], testrun.Line{File: declaration.File, Number: line})
					}
				}
			}
		}
	}
	return request, nil
}

func addDynamicIssues(result *ledger.CheckRecord, request testrun.Request) {
	report := result.Report.Dynamic
	if report == nil {
		return
	}
	for _, observation := range report.Observations {
		test := observation.Case
		if test.Change != "add" || !observation.Baseline.Passed() || !observation.Baseline.Coverage.Valid || !observation.Baseline.Coverage.CopyExecuted {
			continue
		}
		var requirements []testrun.Line
		for id, lines := range request.Requirements {
			if len(test.Requirements) == 0 || slices.Contains(test.Requirements, id) {
				requirements = append(requirements, lines...)
			}
		}
		if len(requirements) == 0 {
			continue
		}
		result.Evaluated = append(result.Evaluated, ledger.CheckScope("T1", test.At))
		covered := false
		for _, line := range observation.Baseline.Coverage.Lines {
			if slices.Contains(requirements, line) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		issue := ledger.Issue{Rule: "T1", At: test.At, FactKey: "no_requirement_coverage", Level: "hint", Status: "open", Facts: []string{"有效副本覆盖中未观察到这个新增用例执行需求代码。"}, Advice: "核对测试用途、子进程或外部进程、配置以及删除旧入口的验收方式；覆盖为空不等于测试无用。", Evidence: test.Assertions}
		if test.Retained {
			issue.Status = "retained"
		}
		result.Issues = append(result.Issues, issue)
	}
	for _, group := range report.Groups {
		at := "requirement:" + group.Requirement
		if group.Complete {
			result.Evaluated = append(result.Evaluated, ledger.CheckScope("T2", at))
		}
		if !report.Complete || !group.Complete || len(group.Candidates) == 0 {
			continue
		}
		result.Issues = append(result.Issues, ledger.Issue{Rule: "T2", At: at, FactKey: "mutation_candidates:" + strings.Join(group.Cases, "|"), Level: "hint", Status: "open", Facts: []string{"这些用例的需求代码行覆盖相同；有效变异发现集合可由所列保留组合维持。", "保留组合：" + strings.Join(group.Keep, ", "), "待人工核对的新增候选：" + strings.Join(group.Candidates, ", ")}, Advice: group.Reason, Evidence: group.Cases})
	}
	result.Level = ledger.HighestLevel(result.Issues)
}
