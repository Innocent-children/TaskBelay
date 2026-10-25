package testrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type PythonRunner struct {
	exec    *Executor
	request Request
	command scriptCommand
	context context.Context
	cancel  context.CancelFunc
}

type pytestReport struct {
	Collected []string `json:"collected"`
	Modules   []string `json:"modules"`
	Reports   []struct {
		NodeID    string `json:"nodeid"`
		When      string `json:"when"`
		Outcome   string `json:"outcome"`
		Assertion bool   `json:"assertion"`
	} `json:"reports"`
}

const pytestObserver = `import json, os, sys
import pytest
_collected, _reports = [], []
def pytest_collection_finish(session):
    _collected.extend(item.nodeid for item in session.items)
@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(item, call):
    outcome = yield
    report = outcome.get_result()
    assertion = bool(call.excinfo and call.excinfo.errisinstance((AssertionError, pytest.fail.Exception)))
    _reports.append(dict(nodeid=report.nodeid, when=report.when, outcome=report.outcome, assertion=assertion))
def pytest_sessionfinish(session, exitstatus):
    modules = sorted({os.path.realpath(m.__file__) for m in tuple(sys.modules.values()) if isinstance(getattr(m, '__file__', None), str)})
    with open(os.environ['TASKBELAY_PYTEST_REPORT'], 'w', encoding='utf-8') as f:
        json.dump(dict(collected=_collected, reports=_reports, modules=modules), f)
`

func NewPythonRunner(executor *Executor, request Request) *PythonRunner {
	return &PythonRunner{exec: executor, request: request}
}

func (p *PythonRunner) Prepare(ctx context.Context) Result {
	p.context, p.cancel = context.WithCancel(ctx)
	ctx = p.context
	root, err := filepath.EvalSymlinks(p.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	p.exec.Root = root
	for _, name := range []string{".venv", "venv"} {
		if err := linkDependency(p.exec, name); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
	}
	python := "python3"
	for _, name := range []string{".venv", "venv"} {
		candidate := filepath.Join(root, name, "bin", "python")
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(root, name, "Scripts", "python.exe")
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			python = candidate
			break
		}
	}
	p.command, err = parseScriptCommand(p.request.Config["python"].Command, root, "pytest", []string{python})
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	path := root + string(os.PathListSeparator) + filepath.Join(root, "src") + string(os.PathListSeparator) + p.exec.Output
	for _, env := range mergeEnvironment(p.exec.Env, p.command.env) {
		if value, ok := strings.CutPrefix(env, "PYTHONPATH="); ok && value != "" {
			path += string(os.PathListSeparator) + value
		}
	}
	p.command.env = mergeEnvironment(p.command.env, []string{"PYTHONPATH=" + path, "PYTHONDONTWRITEBYTECODE=1", "UV_NO_SYNC=1", "UV_OFFLINE=1", "PWD=" + root})
	p.exec.Env = mergeEnvironment(p.exec.Env, p.command.env)
	if result := p.exec.Prepare(ctx, p.request.Config["python"].Prepare); !result.Passed() {
		return result
	}
	if err := checkCopy(p.exec, p.request.Snapshot, "python"); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	if err := os.WriteFile(filepath.Join(p.exec.Output, "_taskbelay_pytest.py"), []byte(pytestObserver), 0600); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	probe := p.exec.Execute(ctx, "prepare", root, p.command.args("-c", `import sys,json,pytest,coverage; print(json.dumps(dict(pytest=pytest.__version__,coverage=coverage.__version__,path=sys.path,no_bytecode=sys.dont_write_bytecode)))`), p.command.env)
	if !probe.Passed() {
		return probe
	}
	var actual struct {
		Path       []string `json:"path"`
		NoBytecode bool     `json:"no_bytecode"`
	}
	if json.Unmarshal([]byte(probe.Output), &actual) != nil || !actual.NoBytecode || len(actual.Path) < 3 || actual.Path[1] != root || actual.Path[2] != filepath.Join(root, "src") {
		return failure(EnvironmentFailure, "Python 实际源码优先级或字节码输出设置不能核对")
	}
	return Result{}
}

func (p *PythonRunner) Compile(ctx context.Context) Result {
	ctx, done := runnerContext(ctx, p.context)
	defer done()
	var paths []string
	for path, file := range p.request.Snapshot.Files {
		if file.Kind.Language == "python" {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	args := append([]string{"-c", `import ast,sys,tokenize
for name in sys.argv[1:]:
    with tokenize.open(name) as f: ast.parse(f.read(),filename=name)`}, paths...)
	return p.exec.Execute(ctx, "compile", p.exec.Root, p.command.args(args...), p.command.env)
}

func (p *PythonRunner) pytest(ctx context.Context, stage string, selectors []string, collect bool) (RunResult, pytestReport) {
	ctx, done := runnerContext(ctx, p.context)
	defer done()
	dir, err := os.MkdirTemp(p.exec.Output, "pytest-*")
	if err != nil {
		return RunResult{Result: failure(EnvironmentFailure, err.Error())}, pytestReport{}
	}
	args := []string{"-m", "pytest"}
	profile := filepath.Join(dir, "coverage.json")
	data := filepath.Join(dir, "coverage.data")
	if !collect {
		args = append([]string{"-m", "coverage", "run", "--data-file=" + data}, args...)
	}
	args = append(args, p.command.options...)
	args = append(args, "-p", "_taskbelay_pytest", "-o", "cache_dir="+filepath.Join(dir, "pytest-cache"), "--basetemp="+filepath.Join(dir, "tmp"))
	if collect {
		args = append(args, "--collect-only")
	}
	args = append(args, selectors...)
	reportPath := filepath.Join(dir, "pytest.json")
	env := mergeEnvironment(p.command.env, []string{"TASKBELAY_PYTEST_REPORT=" + reportPath, "COVERAGE_FILE=" + data})
	run := RunResult{Result: p.exec.Execute(ctx, stage, p.exec.Root, p.command.args(args...), env), Profile: profile}
	var report pytestReport
	if run.Failure == Canceled || run.Failure == Timeout {
		return run, report
	}
	if err := readJSON(reportPath, &report); err != nil {
		run.Failure, run.Reason = EnvironmentFailure, "pytest 报告未完整保存: "+err.Error()
		return run, report
	}
	if collect {
		return run, report
	}
	asserted, infrastructure := false, false
	for _, phase := range report.Reports {
		if phase.When == "call" {
			run.Selected = append(run.Selected, phase.NodeID)
		}
		if phase.Outcome == "failed" {
			if phase.When == "call" && phase.Assertion {
				asserted = true
			} else {
				infrastructure = true
			}
		}
		if phase.Outcome == "skipped" {
			infrastructure = true
		}
	}
	if !run.Passed() && run.ExitCode == 1 && asserted && !infrastructure {
		run.Failure, run.Reason = AssertionFailure, "pytest 用例断言失败"
	}
	if run.Passed() && infrastructure {
		run.Failure, run.Reason = SelectionFailure, "pytest 有跳过或未完成的执行阶段"
	}
	if !run.Passed() {
		return run, report
	}
	converted := p.exec.Execute(ctx, "coverage", p.exec.Root, p.command.args("-m", "coverage", "json", "--data-file="+data, "-o", profile), env)
	if !converted.Passed() {
		run.Failure, run.Reason = CoverageFailure, "coverage.py 未完整生成 JSON: "+converted.Reason
		return run, report
	}
	for _, path := range report.Modules {
		if _, inside := copyFile(p.exec.Root, path); inside {
			continue
		}
		if p.exec.Workspace != "" {
			if relative, inside := copyFile(p.exec.Workspace, path); inside && p.request.Snapshot.Files[relative] != nil {
				run.Failure, run.Reason = CoverageFailure, "Python 从原工作区加载源码: "+relative
				return run, report
			}
		}
	}
	run.Coverage = p.ReadCoverage(ctx, run)
	return run, report
}

func (p *PythonRunner) Enumerate(ctx context.Context, tests []Case) ([]Case, Result) {
	selectors := p.command.selectors
	if len(selectors) == 0 {
		for _, test := range tests {
			selectors = append(selectors, test.Source.File)
		}
		slices.Sort(selectors)
		selectors = slices.Compact(selectors)
	}
	run, report := p.pytest(ctx, "enumerate", selectors, true)
	if !run.Passed() {
		return nil, run.Result
	}
	groups := map[string][]Case{}
	var keys []string
	for _, test := range tests {
		name, _, _ := strings.Cut(test.Source.Name, "[")
		key := test.Source.File + "::" + name
		if _, found := groups[key]; !found {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], test)
	}
	var result []Case
	for _, key := range keys {
		group := groups[key]
		var native []string
		for _, id := range report.Collected {
			if id == key || strings.HasPrefix(id, key+"[") {
				native = append(native, id)
			}
		}
		if len(native) == 0 {
			for _, test := range group {
				test.Unknown = append(test.Unknown, "pytest 未选中此用例，可能受项目选择或跳过配置影响")
				result = append(result, test)
			}
			continue
		}
		exact := len(native) == len(group)
		for _, test := range group {
			exact = exact && len(test.Source.Unknown) == 0 && slices.Contains(native, test.Source.File+"::"+test.Source.Name)
		}
		if exact {
			for _, test := range group {
				test.Package = test.Source.File
				test.Name = test.Source.Name
				test.Expected = []string{test.Source.File + "::" + test.Source.Name}
				result = append(result, test)
			}
		} else {
			name := strings.TrimPrefix(key, group[0].Source.File+"::")
			test := wholeCase(group, name, native, "pytest 原生参数名与源码不能唯一对应，保留外层整体")
			test.Package = test.Source.File
			result = append(result, test)
		}
	}
	return result, Result{}
}

func (p *PythonRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	run, _ := p.pytest(ctx, stage, []string{test.Source.File + "::" + test.Name}, false)
	if (run.Passed() || run.Failure == AssertionFailure) && !sameNames(run.Selected, test.Expected) {
		run.Failure, run.Reason = SelectionFailure, "pytest 实际执行的 node id 与选择不一致"
	}
	return run
}

func (p *PythonRunner) ReadCoverage(ctx context.Context, run RunResult) Coverage {
	if err := ctx.Err(); err != nil {
		return Coverage{Reason: err.Error()}
	}
	var report struct {
		Files map[string]struct {
			Executed []int `json:"executed_lines"`
			Missing  []int `json:"missing_lines"`
		} `json:"files"`
	}
	if err := readJSON(run.Profile, &report); err != nil {
		return Coverage{Reason: err.Error()}
	}
	if report.Files == nil {
		return Coverage{Reason: "coverage.py 报告缺少文件集合"}
	}
	result := Coverage{Valid: true}
	for path, covered := range report.Files {
		rel, inside := copyFile(p.exec.Root, path)
		if !inside {
			if p.exec.Workspace != "" {
				if outside, known := copyFile(p.exec.Workspace, path); known && p.request.Snapshot.Files[outside] != nil {
					return Coverage{Reason: "覆盖来自原工作区源码: " + outside}
				}
			}
			continue
		}
		file := p.request.Snapshot.Files[rel]
		if file == nil || file.Kind.Language != "python" {
			continue
		}
		for _, line := range append(slices.Clone(covered.Executed), covered.Missing...) {
			if line <= 0 || line > lineCount(file.Data) {
				return Coverage{Reason: fmt.Sprintf("覆盖行超出固定源码: %s:%d", rel, line)}
			}
		}
		if len(covered.Executed) > 0 {
			result.CopyExecuted = true
		}
		for _, line := range covered.Executed {
			result.Lines = append(result.Lines, Line{File: rel, Number: line})
		}
	}
	result.Lines = normalizedLines(result.Lines)
	if !result.CopyExecuted {
		result.Reason = "未取得副本源码执行证据"
	}
	return result
}

func (p *PythonRunner) Terminate() {
	if p.cancel != nil {
		p.cancel()
	}
}

func runnerContext(ctx, parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	if parent == nil {
		return ctx, cancel
	}
	stop := context.AfterFunc(parent, cancel)
	return ctx, func() { stop(); cancel() }
}
