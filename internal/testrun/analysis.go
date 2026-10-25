package testrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

/**
 * 所有动态阶段共用一次截止时间。只导出固定对象，清理仅针对本次创建的
 * 临时目录；原工作区在运行期间不加锁，当前性由调用方发布前重新核对。
 */
func Analyze(ctx context.Context, workspace gitstate.Workspace, request Request) (report Report) {
	started := time.Now()
	report.Rules, report.Tree, report.Complete = DynamicVersion, request.Snapshot.Tree, true
	defer func() {
		report.Duration = time.Since(started)
		if !report.Complete {
			report.Reusable = false
			for i := range report.Groups {
				report.Groups[i].Candidates = nil
			}
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, event.TestsBudget)
	defer cancel()
	base, err := os.MkdirTemp("", "taskbelay-tests-*")
	if err != nil {
		report.Complete = false
		report.Missing = append(report.Missing, err.Error())
		return
	}
	defer func() {
		if err := os.RemoveAll(base); err != nil {
			report.Complete = false
			report.Missing = append(report.Missing, "临时资源清理未完成: "+err.Error())
		}
	}()
	root, output := filepath.Join(base, "source"), filepath.Join(base, "output")
	if err := workspace.ExportTree(ctx, request.Snapshot.Tree, root); err != nil {
		report.Complete = false
		report.Missing = append(report.Missing, err.Error())
		return
	}
	for _, path := range []string{output, filepath.Join(base, "cache"), filepath.Join(base, "tmp")} {
		if err := os.Mkdir(path, 0700); err != nil {
			report.Complete = false
			report.Missing = append(report.Missing, err.Error())
			return
		}
	}
	environment := mergeEnvironment(os.Environ(), []string{"GOCACHE=" + filepath.Join(base, "cache"), "GOTMPDIR=" + filepath.Join(base, "tmp"), "TMPDIR=" + filepath.Join(base, "tmp"), "TMP=" + filepath.Join(base, "tmp"), "TEMP=" + filepath.Join(base, "tmp")})
	byLanguage := map[string][]Case{}
	for _, test := range request.Cases {
		language := ""
		if file := request.Snapshot.Files[test.Source.File]; file != nil {
			language = file.Kind.Language
		}
		if language != "go" && language != "python" && language != "javascript" && language != "typescript" && language != "rust" && language != "java" {
			report.Complete = false
			report.Missing = append(report.Missing, test.At+"：该语言 Runner 尚未接入")
			continue
		}
		byLanguage[language] = append(byLanguage[language], test)
	}
	report.Reusable = len(byLanguage) > 0
	for _, language := range []string{"go", "python", "javascript", "typescript", "rust", "java"} {
		cases := byLanguage[language]
		if len(cases) == 0 {
			continue
		}
		dir := filepath.Join(output, language)
		if err := os.Mkdir(dir, 0700); err != nil {
			report.Complete = false
			report.Missing = append(report.Missing, err.Error())
			continue
		}
		executor := &Executor{Root: root, Output: dir, Workspace: workspace.Root, Env: slices.Clone(environment)}
		current := request
		current.Cases = cases
		var runner Runner
		switch language {
		case "go":
			runner = NewGoRunner(executor, current)
		case "python":
			runner = NewPythonRunner(executor, current)
		case "javascript", "typescript":
			runner = NewJavaScriptRunner(executor, current)
		case "rust":
			runner = NewRustRunner(executor, current)
		case "java":
			runner = NewJavaRunner(executor, current)
		}
		runAnalysis(ctx, executor, runner, current, &report)
		runner.Terminate()
		report.Stages = append(report.Stages, executor.Records()...)
		if goRunner, ok := runner.(*GoRunner); ok {
			report.Reusable = report.Reusable && goRunner.Reusable()
		} else {
			report.Reusable = false
		}
	}
	if ctx.Err() != nil {
		report.Complete = false
		report.Missing = append(report.Missing, ctx.Err().Error())
	}
	return report
}

func runAnalysis(ctx context.Context, executor *Executor, runner Runner, request Request, report *Report) {
	failed := func(stage string, result Result) bool {
		if result.Passed() {
			return false
		}
		report.Complete = false
		report.Missing = append(report.Missing, stage+": "+string(result.Failure)+" "+result.Reason)
		return true
	}
	if failed("准备", runner.Prepare(ctx)) {
		return
	}
	if failed("编译", runner.Compile(ctx)) {
		return
	}
	cases, result := runner.Enumerate(ctx, request.Cases)
	if failed("枚举", result) {
		return
	}
	language := "go"
	if len(request.Cases) > 0 {
		if file := request.Snapshot.Files[request.Cases[0].Source.File]; file != nil {
			language = file.Kind.Language
		}
	}
	parallel := min(max(1, request.Config[language].Parallel), runtime.NumCPU(), 4)
	observations := make([]Observation, len(cases))
	for i, test := range cases {
		observations[i] = Observation{Case: test, Baseline: RunResult{Result: failure(Canceled, "用例尚未执行")}}
	}
	runCases(ctx, len(cases), parallel, func(i int) {
		test := cases[i]
		observations[i].Case = test
		if test.Package == "" {
			observations[i].Baseline.Result = failure(SelectionFailure, strings.Join(test.Unknown, "；"))
			return
		}
		observations[i].Baseline = runner.Run(ctx, test, "baseline")
	})
	for _, observation := range observations {
		if !observation.Baseline.Passed() || !observation.Baseline.Coverage.Valid || !observation.Baseline.Coverage.CopyExecuted {
			report.Complete = false
			report.Missing = append(report.Missing, observation.Case.At+"：基线或覆盖未完成 "+observation.Baseline.Reason+" "+observation.Baseline.Coverage.Reason)
		}
	}
	start := len(report.Observations)
	report.Observations = append(report.Observations, observations...)
	groups := coverageGroups(request, observations)
	for _, group := range groups {
		if ctx.Err() != nil {
			group.Complete = false
			group.Reason = ctx.Err().Error()
			report.Groups = append(report.Groups, group)
			continue
		}
		group = mutateGroup(ctx, executor, runner, request, group, parallel, report.Observations[start:])
		if !group.Complete {
			report.Complete = false
		}
		report.Groups = append(report.Groups, group)
	}
}

func runCases(ctx context.Context, count, parallel int, run func(int)) {
	var workers sync.WaitGroup
	next := make(chan int)
	for range parallel {
		workers.Go(func() {
			for i := range next {
				run(i)
			}
		})
	}
	for i := 0; i < count; i++ {
		if ctx.Err() != nil {
			break
		}
		select {
		case next <- i:
		case <-ctx.Done():
		}
	}
	close(next)
	workers.Wait()
}

func coverageGroups(request Request, observations []Observation) []Group {
	groups := map[string]Group{}
	for requirement, required := range request.Requirements {
		for _, observation := range observations {
			if !observation.Baseline.Passed() || !observation.Baseline.Coverage.Valid || !observation.Baseline.Coverage.CopyExecuted {
				continue
			}
			covered := intersect(observation.Baseline.Coverage.Lines, required)
			if len(covered) == 0 {
				continue
			}
			var identity strings.Builder
			fmt.Fprintln(&identity, requirement)
			for _, line := range covered {
				fmt.Fprintf(&identity, "%q:%d\n", line.File, line.Number)
			}
			group := groups[identity.String()]
			group.Requirement, group.Coverage = requirement, covered
			group.Cases = append(group.Cases, observation.Case.At)
			groups[identity.String()] = group
		}
	}
	var keys []string
	for key, group := range groups {
		if len(group.Cases) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var result []Group
	for _, key := range keys {
		result = append(result, groups[key])
	}
	return result
}

func intersect(a, b []Line) []Line {
	set := map[Line]bool{}
	for _, line := range b {
		set[line] = true
	}
	seen := map[Line]bool{}
	var result []Line
	for _, line := range a {
		if set[line] && !seen[line] {
			result = append(result, line)
			seen[line] = true
		}
	}
	sortLines(result)
	return result
}

func mutateGroup(ctx context.Context, executor *Executor, runner Runner, request Request, group Group, parallel int, observations []Observation) Group {
	group.Complete = true
	var cases []int
	hasCandidate := false
	for i, observation := range observations {
		if slices.Contains(group.Cases, observation.Case.At) {
			cases = append(cases, i)
			hasCandidate = hasCandidate || candidateCase(observation.Case)
		}
	}
	if !hasCandidate {
		group.Reason = "没有符合来源和用户决定条件的新增精简对象"
		return group
	}
	allowed := map[string]map[int]bool{}
	for _, line := range intersect(group.Coverage, request.Changed[group.Requirement]) {
		if allowed[line.File] == nil {
			allowed[line.File] = map[int]bool{}
		}
		allowed[line.File][line.Number] = true
	}
	var paths []string
	for path := range allowed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		mutations, err := codescan.Mutations(ctx, request.Snapshot.Files[path], allowed[path])
		if err != nil {
			group.Complete = false
			group.Reason = err.Error()
			return group
		}
		for _, mutation := range mutations {
			if len(group.Mutations) == 20 {
				break
			}
			group.Mutations = append(group.Mutations, mutation)
		}
		if len(group.Mutations) == 20 {
			break
		}
	}
	if len(group.Mutations) == 0 {
		group.Reason = "有效覆盖的行为变化范围没有可用变异"
		return group
	}
	runs, valid := 0, 0
	for _, mutation := range group.Mutations {
		if ctx.Err() != nil || runs+len(cases) > 40 {
			group.Complete = false
			group.Reason = "已到达时间或每组四十次运行预算"
			break
		}
		original := request.Snapshot.Files[mutation.File].Data
		path := filepath.Join(executor.Root, filepath.FromSlash(mutation.File))
		actual, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(actual, original) {
			group.Complete = false
			group.Reason = "变异前副本源码与固定快照不一致"
			break
		}
		updated := append(append(append([]byte(nil), original[:mutation.Start]...), mutation.After...), original[mutation.End:]...)
		if err := os.WriteFile(path, updated, 0600); err != nil {
			group.Complete = false
			group.Reason = err.Error()
			break
		}
		compiled := runner.Compile(ctx)
		if compiled.Passed() {
			results := make([]RunResult, len(cases))
			for i := range results {
				results[i].Result = failure(Canceled, "变异用例尚未执行")
			}
			runCases(ctx, len(cases), parallel, func(i int) { results[i] = runner.Run(ctx, observations[cases[i]].Case, "mutation") })
			valid++
			for i, result := range results {
				if result.Passed() && (!result.Coverage.Valid || !result.Coverage.CopyExecuted) {
					result.Failure, result.Reason = CoverageFailure, "变异运行缺少有效副本覆盖: "+result.Coverage.Reason
				}
				runs++
				observation := &observations[cases[i]]
				group.Runs = append(group.Runs, MutationRun{Mutation: mutation.ID, Case: observation.Case.At, Result: result.Result})
				if result.Failure == AssertionFailure {
					observation.Killed = append(observation.Killed, mutation.ID)
				} else if !result.Passed() {
					group.Complete = false
					group.Reason = "变异用例存在非断言失败或未完成运行"
				}
			}
		} else {
			group.Runs = append(group.Runs, MutationRun{Mutation: mutation.ID, Result: compiled})
			if compiled.Failure != CompileFailure {
				group.Complete = false
				group.Reason = "变异编译或执行环境未完成"
			}
		}
		if err := os.WriteFile(path, original, 0600); err != nil {
			group.Complete = false
			group.Reason = "副本恢复失败: " + err.Error()
			break
		}
		if !group.Complete {
			break
		}
	}
	if valid == 0 {
		group.Reason = "没有完成有效变异；编译和环境失败不算用例发现"
		return group
	}
	if group.Complete {
		selectCandidates(&group, observations)
	}
	return group
}

func candidateCase(test Case) bool {
	return test.Change == "add" && test.AgentOnly && !test.Mixed && !test.Retained && !test.Whole && len(test.Unknown) == 0 && len(test.Source.Unknown) == 0
}

func selectCandidates(group *Group, observations []Observation) {
	mutationIDs := map[string]bool{}
	for _, mutation := range group.Mutations {
		mutationIDs[mutation.ID] = true
	}
	sets := map[string]map[string]bool{}
	selected := map[string]bool{}
	covered := map[string]bool{}
	union := map[string]bool{}
	var cases []Case
	for _, observation := range observations {
		if !slices.Contains(group.Cases, observation.Case.At) {
			continue
		}
		cases = append(cases, observation.Case)
		sets[observation.Case.At] = map[string]bool{}
		for _, id := range observation.Killed {
			if mutationIDs[id] {
				sets[observation.Case.At][id] = true
				union[id] = true
			}
		}
		if !candidateCase(observation.Case) {
			selected[observation.Case.At] = true
			for id := range sets[observation.Case.At] {
				covered[id] = true
			}
		}
	}
	if len(union) == 0 {
		group.Reason = "没有用例发现有效变异，不提供删除清单"
		return
	}
	sort.Slice(cases, func(i, j int) bool {
		if (cases[i].Change == "modify") != (cases[j].Change == "modify") {
			return cases[i].Change == "modify"
		}
		if cases[i].Source.File != cases[j].Source.File {
			return cases[i].Source.File < cases[j].Source.File
		}
		if cases[i].Source.Span.Start != cases[j].Source.Span.Start {
			return cases[i].Source.Span.Start < cases[j].Source.Span.Start
		}
		return cases[i].At < cases[j].At
	})
	for len(covered) < len(union) {
		best, gain := "", 0
		for _, test := range cases {
			if selected[test.At] {
				continue
			}
			count := 0
			for id := range sets[test.At] {
				if !covered[id] {
					count++
				}
			}
			if count > gain {
				best, gain = test.At, count
			}
		}
		if best == "" {
			group.Complete = false
			group.Reason = "发现集合无法完整重建"
			return
		}
		selected[best] = true
		for id := range sets[best] {
			covered[id] = true
		}
	}
	for _, test := range cases {
		if selected[test.At] {
			group.Keep = append(group.Keep, test.At)
		} else if candidateCase(test) {
			group.Candidates = append(group.Candidates, test.At)
		}
	}
	group.Reason = "发现集合仅供候选比较；不同输入契约、输出、权限、幂等、副作用、时序和共享状态须逐条核对，不能由相同行覆盖替代"
}
