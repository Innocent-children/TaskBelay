package testrun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

type JavaScriptRunner struct {
	exec       *Executor
	request    Request
	config     Config
	tool       string
	separator  string
	related    []string
	configPath string
	command    scriptCommand
	context    context.Context
	cancel     context.CancelFunc
}

type jsAssertion struct {
	Title     string   `json:"title"`
	Ancestors []string `json:"ancestorTitles"`
	FullName  string   `json:"fullName"`
	Status    string   `json:"status"`
	Failures  []string `json:"failureMessages"`
	Details   []struct {
		Matcher json.RawMessage `json:"matcherResult"`
		Name    string          `json:"name"`
	} `json:"failureDetails"`
}

type jsReport struct {
	RuntimeErrors int `json:"numRuntimeErrorTestSuites"`
	Results       []struct {
		Name       string        `json:"name"`
		Assertions []jsAssertion `json:"assertionResults"`
		Message    string        `json:"message"`
	} `json:"testResults"`
}

func NewJavaScriptRunner(executor *Executor, request Request) *JavaScriptRunner {
	config := request.Config["javascript"]
	if len(request.Cases) > 0 {
		if file := request.Snapshot.Files[request.Cases[0].Source.File]; file != nil && file.Kind.Language == "typescript" {
			config = request.Config["typescript"]
		}
	}
	return &JavaScriptRunner{exec: executor, request: request, config: config}
}

func (j *JavaScriptRunner) Prepare(ctx context.Context) Result {
	return j.prepare(ctx, true)
}

func (j *JavaScriptRunner) prepare(ctx context.Context, coverage bool) Result {
	j.context, j.cancel = context.WithCancel(ctx)
	ctx = j.context
	root, err := filepath.EvalSymlinks(j.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	j.exec.Root = root
	if err := linkDependency(j.exec, "node_modules"); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	// 两种工具同时存在时只采用可核对的配置或显式框架导入。
	frameworks := map[string]bool{}
	for _, test := range j.request.Cases {
		if test.Source.Framework == "vitest" {
			frameworks["vitest"] = true
		}
		if test.Source.Framework == "@jest/globals" {
			frameworks["jest"] = true
		}
	}
	for _, tool := range []string{"jest", "vitest"} {
		if matched, _ := regexp.MatchString(`(^|[ /\\])`+tool+`(\s|$)`, j.config.Command); matched {
			j.tool = tool
		}
	}
	if j.tool == "" && len(frameworks) == 1 {
		for tool := range frameworks {
			j.tool = tool
		}
	}
	if j.tool == "" {
		for _, tool := range []string{"jest", "vitest"} {
			if _, err := os.Stat(filepath.Join(root, "node_modules", tool, "package.json")); err == nil {
				if j.tool != "" {
					return failure(EnvironmentFailure, "Jest 与 Vitest 都已安装，缺少可核对的项目测试命令")
				}
				j.tool = tool
			}
		}
	}
	if j.tool == "" {
		return failure(EnvironmentFailure, "项目没有已安装的 Jest 或 Vitest")
	}
	program, err := nodeTool(root, j.tool)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	j.command, err = parseScriptCommand(j.config.Command, root, j.tool, program)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	j.command.env = mergeEnvironment(j.command.env, []string{"PWD=" + root, "CI=true", "npm_config_offline=true", "VITE_CJS_IGNORE_WARNING=true"})
	j.exec.Env = mergeEnvironment(j.exec.Env, j.command.env)
	if result := j.exec.Prepare(ctx, j.config.Prepare); !result.Passed() {
		return result
	}
	if err := checkCopy(j.exec, j.request.Snapshot, "javascript", "typescript"); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	version := j.exec.Execute(ctx, "prepare", root, j.command.args("--version"), j.command.env)
	if !version.Passed() {
		return version
	}
	j.separator = " "
	if j.tool == "vitest" {
		match := regexp.MustCompile(`vitest/(\d+)\.`).FindStringSubmatch(version.Output)
		if len(match) != 2 {
			return failure(EnvironmentFailure, "无法核对 Vitest 版本及测试名字选择规则")
		}
		major, _ := strconv.Atoi(match[1])
		if major >= 5 {
			j.separator = " > "
		}
		if _, err := os.Stat(filepath.Join(root, "node_modules", "@vitest", "coverage-v8", "package.json")); coverage && err != nil {
			return failure(EnvironmentFailure, "缺少项目已有的 @vitest/coverage-v8")
		}
		help := j.exec.Execute(ctx, "prepare", root, j.command.args("--help"), j.command.env)
		if !help.Passed() || !strings.Contains(help.Output, "--configLoader") {
			return failure(EnvironmentFailure, "当前 Vitest 不能确认配置加载和临时缓存位置")
		}
		if err := j.vitestConfig(); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
		j.command.options = append(j.command.options, "--configLoader=runner", "--no-file-parallelism", "--maxWorkers=1", "--no-cache")
		if strings.Contains(help.Output, "--fsModuleCachePath") {
			j.command.options = append(j.command.options, "--fsModuleCachePath="+filepath.Join(j.exec.Output, "module-cache"))
		}
	}
	return Result{}
}

func nodeTool(root, tool string) ([]string, error) {
	bin := filepath.Join(root, "node_modules", ".bin", tool)
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(bin); err != nil {
			return nil, err
		}
		return []string{bin}, nil
	}
	if _, err := os.Stat(bin + ".cmd"); err != nil {
		return nil, err
	}
	var manifest struct {
		Bin json.RawMessage `json:"bin"`
	}
	packageRoot := filepath.Join(root, "node_modules", tool)
	if err := readJSON(filepath.Join(packageRoot, "package.json"), &manifest); err != nil {
		return nil, err
	}
	var entry string
	if json.Unmarshal(manifest.Bin, &entry) != nil {
		var bins map[string]string
		if err := json.Unmarshal(manifest.Bin, &bins); err != nil {
			return nil, err
		}
		entry = bins[tool]
	}
	if entry == "" || !filepath.IsLocal(filepath.FromSlash(entry)) {
		return nil, fmt.Errorf("%s 的本地程序入口无法核对", tool)
	}
	return []string{"node", filepath.Join(packageRoot, filepath.FromSlash(entry))}, nil
}

func (j *JavaScriptRunner) vitestConfig() error {
	config := ""
	var options []string
	for i := 0; i < len(j.command.options); i++ {
		if j.command.options[i] == "--config" {
			i++
			config = filepath.Join(j.exec.Root, j.command.options[i])
		} else {
			options = append(options, j.command.options[i])
		}
	}
	if config == "" {
		for _, base := range []string{"vitest.config", "vite.config"} {
			for _, ext := range []string{".ts", ".mts", ".cts", ".js", ".mjs", ".cjs"} {
				candidate := filepath.Join(j.exec.Root, base+ext)
				if _, err := os.Stat(candidate); err == nil {
					config = candidate
					break
				}
			}
			if config != "" {
				break
			}
		}
	}
	source := "const original = {};\n"
	if config != "" {
		if _, ok := copyFile(j.exec.Root, config); !ok {
			return fmt.Errorf("Vitest 配置不在固定副本内")
		}
		encoded, _ := json.Marshal((&url.URL{Scheme: "file", Path: filepath.ToSlash(config)}).String())
		source = "import original from " + string(encoded) + ";\n"
	}
	cache, _ := json.Marshal(filepath.Join(j.exec.Output, "vite-cache"))
	related := ""
	if j.related != nil {
		data, _ := json.Marshal(j.related)
		related = ", related: " + string(data)
	}
	source += `export default async (env) => {
  const value = typeof original === 'function' ? await original(env) : await original;
  return {...value, cacheDir: ` + string(cache) + `, test: {...value?.test, cache: false` + related + `}};
};`
	path := filepath.Join(j.exec.Root, ".taskbelay-vitest.config.mjs")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("临时 Vitest 配置名称与快照文件冲突")
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		return err
	}
	j.configPath = path
	j.command.options = append(options, "--config", path)
	return nil
}

func (j *JavaScriptRunner) Compile(ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		return failure(Canceled, err.Error())
	}
	// Jest 与 Vitest 在本次运行中转换模块；不另造项目没有的构建命令。
	return Result{}
}

func (j *JavaScriptRunner) execute(ctx context.Context, stage string, files []string, pattern string, coverage bool) (RunResult, jsReport) {
	ctx, done := runnerContext(ctx, j.context)
	defer done()
	dir, err := os.MkdirTemp(j.exec.Output, "js-run-*")
	if err != nil {
		return RunResult{Result: failure(EnvironmentFailure, err.Error())}, jsReport{}
	}
	resultPath := filepath.Join(dir, "results.json")
	var args []string
	if j.tool == "vitest" {
		args = append(args, "run")
	}
	args = append(args, j.command.options...)
	if j.tool == "jest" {
		args = append(args, "--runInBand", "--json", "--outputFile="+resultPath, "--cacheDirectory="+filepath.Join(j.exec.Output, "jest-cache"), "--testLocationInResults", "--runTestsByPath")
	} else {
		args = append(args, "--reporter=json", "--outputFile="+resultPath)
	}
	args = append(args, files...)
	if pattern != "" {
		args = append(args, "-t", pattern)
	}
	if coverage {
		if j.tool == "jest" {
			args = append(args, "--coverage", "--coverageReporters=json", "--coverageDirectory="+dir)
		} else {
			args = append(args, "--coverage.enabled", "--coverage.provider=v8", "--coverage.reporter=json", "--coverage.reportsDirectory="+dir)
		}
	}
	run := RunResult{Result: j.exec.Execute(ctx, stage, j.exec.Root, j.command.args(args...), j.command.env), Profile: filepath.Join(dir, "coverage-final.json")}
	var report jsReport
	if run.Failure == Canceled || run.Failure == Timeout {
		return run, report
	}
	if err := readJSON(resultPath, &report); err != nil {
		run.Failure, run.Reason = EnvironmentFailure, "框架没有完整结果报告: "+err.Error()
		return run, report
	}
	asserted, infrastructure := false, report.RuntimeErrors > 0
	for _, file := range report.Results {
		rel, inside := copyFile(j.exec.Root, file.Name)
		if !inside || !slices.Contains(files, rel) {
			run.Failure, run.Reason = SelectionFailure, "实际测试文件不在本次副本选择内"
			return run, report
		}
		if len(file.Assertions) == 0 {
			infrastructure = true
		}
		for _, test := range file.Assertions {
			if test.Status != "passed" && test.Status != "failed" {
				if pattern == "" {
					infrastructure = true
				}
				continue
			}
			run.Selected = append(run.Selected, test.FullName)
			if test.Status == "failed" {
				if jsAssertionFailure(test) {
					asserted = true
				} else {
					infrastructure = true
				}
			}
		}
	}
	if !run.Passed() && run.ExitCode == 1 && asserted && !infrastructure {
		run.Failure, run.Reason = AssertionFailure, j.tool+" 用例断言失败"
	}
	if run.Passed() && infrastructure {
		run.Failure, run.Reason = EnvironmentFailure, "测试文件或运行环境未完成"
	}
	if coverage && run.Passed() {
		run.Coverage = j.ReadCoverage(ctx, run)
	}
	return run, report
}

func jsAssertionFailure(test jsAssertion) bool {
	if len(test.Details) > 0 {
		for _, detail := range test.Details {
			if len(detail.Matcher) == 0 && detail.Name != "AssertionError" {
				return false
			}
		}
		return true
	}
	if len(test.Failures) == 0 {
		return false
	}
	for _, message := range test.Failures {
		if !strings.HasPrefix(strings.TrimSpace(message), "AssertionError:") {
			return false
		}
	}
	return true
}

func (j *JavaScriptRunner) Enumerate(ctx context.Context, tests []Case) ([]Case, Result) {
	groups := map[string][]Case{}
	var paths []string
	for _, test := range tests {
		if _, ok := groups[test.Source.File]; !ok {
			paths = append(paths, test.Source.File)
		}
		groups[test.Source.File] = append(groups[test.Source.File], test)
	}
	slices.Sort(paths)
	var result []Case
	for _, path := range paths {
		group := groups[path]
		if len(j.command.selectors) > 0 && !slices.Contains(j.command.selectors, path) {
			for _, test := range group {
				test.Unknown = append(test.Unknown, "配置的文件选择不能唯一对应此文件")
				result = append(result, test)
			}
			continue
		}
		run, report := j.execute(ctx, "enumerate", []string{path}, "", false)
		if !run.Passed() {
			return result, run.Result
		}
		byName := map[string][]string{}
		selectors := map[string]string{}
		for _, file := range report.Results {
			for _, test := range file.Assertions {
				name := strings.Join(append(slices.Clone(test.Ancestors), test.Title), " > ")
				byName[name] = append(byName[name], test.FullName)
				selectors[name] = strings.Join(append(slices.Clone(test.Ancestors), test.Title), j.separator)
			}
		}
		exact := len(group) == len(run.Selected)
		for _, test := range group {
			exact = exact && len(test.Source.Unknown) == 0 && len(byName[test.Source.Name]) == 1
		}
		if !exact {
			test := wholeCase(group, "<文件整体>", run.Selected, "框架原生名字与源码不能唯一对应，保留整个测试文件")
			test.Package = path
			result = append(result, test)
			continue
		}
		for _, test := range group {
			test.Package = path
			test.Expected = byName[test.Source.Name]
			test.Selector = selectors[test.Source.Name]
			result = append(result, test)
		}
	}
	return result, Result{}
}

func (j *JavaScriptRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	pattern := ""
	if !test.Whole {
		if len(test.Expected) != 1 {
			return RunResult{Result: failure(SelectionFailure, "缺少唯一原生测试名字")}
		}
		pattern = "^" + regexp.QuoteMeta(test.Selector) + "$"
	}
	run, _ := j.execute(ctx, stage, []string{test.Source.File}, pattern, true)
	if (run.Passed() || run.Failure == AssertionFailure) && !sameNames(run.Selected, test.Expected) {
		run.Failure, run.Reason = SelectionFailure, "框架实际执行的用例与精确选择不一致"
	}
	return run
}

func (j *JavaScriptRunner) ReadCoverage(ctx context.Context, run RunResult) Coverage {
	if err := ctx.Err(); err != nil {
		return Coverage{Reason: err.Error()}
	}
	var report map[string]struct {
		Path       string                                                     `json:"path"`
		Statements map[string]struct{ Start, End struct{ Line, Column int } } `json:"statementMap"`
		Counts     map[string]int                                             `json:"s"`
	}
	if err := readJSON(run.Profile, &report); err != nil {
		return Coverage{Reason: err.Error()}
	}
	if report == nil {
		return Coverage{Reason: "覆盖报告不是有效的文件集合"}
	}
	result := Coverage{Valid: true}
	for name, entry := range report {
		if entry.Path != "" && filepath.Clean(name) != filepath.Clean(entry.Path) {
			return Coverage{Reason: "覆盖报告文件身份不一致"}
		}
		rel, inside := copyFile(j.exec.Root, name)
		if !inside {
			if j.exec.Workspace != "" {
				if outside, known := copyFile(j.exec.Workspace, name); known && j.request.Snapshot.Files[outside] != nil {
					return Coverage{Reason: "覆盖来自原工作区源码: " + outside}
				}
			}
			continue
		}
		file := j.request.Snapshot.Files[rel]
		if file == nil {
			continue
		}
		for id, statement := range entry.Statements {
			count, ok := entry.Counts[id]
			if !ok || count < 0 || statement.Start.Line <= 0 || statement.End.Line < statement.Start.Line || statement.End.Line > lineCount(file.Data) {
				return Coverage{Reason: "覆盖语句或行范围无法对应固定源码: " + rel}
			}
			if count == 0 {
				continue
			}
			result.CopyExecuted = true
			end := statement.End.Line
			if statement.End.Column == 0 && end > statement.Start.Line {
				end--
			}
			for line := statement.Start.Line; line <= end; line++ {
				result.Lines = append(result.Lines, Line{File: rel, Number: line})
			}
		}
	}
	result.Lines = normalizedLines(result.Lines)
	if !result.CopyExecuted {
		result.Reason = "覆盖数据没有副本源码执行证据，不能据此比较变异"
	}
	return result
}

func (j *JavaScriptRunner) Terminate() {
	if j.cancel != nil {
		j.cancel()
	}
	if j.configPath != "" {
		_ = os.Remove(j.configPath)
	}
}
