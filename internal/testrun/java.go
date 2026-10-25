package testrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type javaModule struct{ ID, Source, TestSource, Classes, TestClasses, Build, Reports, ArgLine string }

type JavaRunner struct {
	exec                                    *Executor
	request                                 Request
	command                                 javaCommand
	modules                                 []javaModule
	caches                                  []string
	agent, javaHome, readerPath, gradleInit string
	context                                 context.Context
	cancel                                  context.CancelFunc
	serial                                  chan struct{}
}

func NewJavaRunner(executor *Executor, request Request) *JavaRunner {
	return &JavaRunner{exec: executor, request: request, serial: make(chan struct{}, 1)}
}

func (r *JavaRunner) Prepare(ctx context.Context) Result {
	r.context, r.cancel = context.WithCancel(ctx)
	ctx = r.context
	root, err := filepath.EvalSymlinks(r.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	r.exec.Root = root
	r.command, err = parseJavaCommand(r.request.Config["java"].Command, root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	r.exec.Env = mergeEnvironment(r.exec.Env, r.command.env)
	if result := r.exec.Prepare(ctx, r.request.Config["java"].Prepare); !result.Passed() {
		return result
	}
	if err := checkCopy(r.exec, r.request.Snapshot, "java"); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	r.caches = []string{filepath.Join(home, ".m2", "repository"), filepath.Join(home, ".gradle")}
	for _, env := range r.exec.Env {
		if value, ok := strings.CutPrefix(env, "GRADLE_USER_HOME="); ok {
			r.caches = append(r.caches, value)
		}
		if value, ok := strings.CutPrefix(env, "MAVEN_USER_HOME="); ok {
			r.caches = append(r.caches, value)
		}
	}
	for _, option := range r.command.options {
		if value, ok := strings.CutPrefix(option, "-Dmaven.repo.local="); ok {
			r.caches = append(r.caches, value)
		}
	}
	var version string
	r.agent, version, err = cachedJaCoCoAgent(r.caches, r.exec.Output)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	var result Result
	if r.command.tool == "maven" {
		result = r.prepareMaven(ctx)
	} else {
		result = r.prepareGradle(ctx)
	}
	if !result.Passed() {
		return result
	}
	if r.javaHome == "" {
		return failure(EnvironmentFailure, "项目实际 JDK 未确认")
	}
	var jars []string
	for _, artifact := range []string{"org.jacoco.core", "org.jacoco.report"} {
		path, _ := cachedJavaArtifact(r.caches, "org.jacoco", artifact, version, "")
		if path == "" {
			return failure(EnvironmentFailure, "已有缓存缺少 JaCoCo "+version+" 的 "+artifact)
		}
		jars = append(jars, path)
	}
	_, asmVersion := cachedJavaArtifact(r.caches, "org.ow2.asm", "asm", "*", "")
	for _, artifact := range []string{"asm", "asm-commons", "asm-tree"} {
		path, _ := cachedJavaArtifact(r.caches, "org.ow2.asm", artifact, asmVersion, "")
		if path == "" {
			return failure(EnvironmentFailure, "已有缓存缺少 JaCoCo 的 ASM 依赖")
		}
		jars = append(jars, path)
	}
	r.readerPath = strings.Join(append([]string{r.exec.Output}, jars...), string(os.PathListSeparator))
	path := filepath.Join(r.exec.Output, "TaskBelayCoverage.java")
	if err := os.WriteFile(path, []byte(javaCoverageReader), 0600); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	return r.exec.Execute(ctx, "prepare", r.exec.Output, r.jdk("javac", "-encoding", "UTF-8", "-cp", r.readerPath, "-d", r.exec.Output, path), r.command.env)
}

func (r *JavaRunner) jdk(tool string, args ...string) []string {
	if runtime.GOOS == "windows" {
		tool += ".exe"
	}
	command := append(slices.Clone(r.command.prefix[:len(r.command.prefix)-1]), filepath.Join(r.javaHome, "bin", tool))
	return append(command, args...)
}

func (r *JavaRunner) Compile(ctx context.Context) Result {
	ctx, done := runnerContext(ctx, r.context)
	defer done()
	if r.command.tool == "maven" {
		return r.mavenCompile(ctx)
	}
	return r.gradleCompile(ctx)
}

func (r *JavaRunner) Enumerate(ctx context.Context, tests []Case) ([]Case, Result) {
	type selection struct {
		module javaModule
		name   string
		cases  []Case
	}
	groups := map[string]*selection{}
	var keys []string
	var result []Case
	for _, test := range tests {
		name, _, _ := strings.Cut(test.Source.Name, "[")
		class, method, ok := strings.Cut(name, "#")
		var matches []javaModule
		qualified := ""
		for _, module := range r.modules {
			fileClass, err := javaClassName(test.Source.File, module, r.exec.Root)
			if err == nil && (fileClass == class || strings.HasSuffix(fileClass, "."+class)) {
				matches = append(matches, module)
				qualified = fileClass + "#" + method
			}
		}
		if !ok || len(matches) != 1 {
			test.Unknown = append(test.Unknown, "Java 用例不能唯一对应模块与类")
			result = append(result, test)
			continue
		}
		if r.command.filter != "" && r.command.filter != qualified && r.command.filter != name && r.command.filter != class && r.command.filter != strings.Replace(qualified, "#", ".", 1) {
			test.Unknown = append(test.Unknown, "Java 配置的选择范围未包括或不能核对该方法")
			result = append(result, test)
			continue
		}
		key := matches[0].ID + ":" + qualified
		if groups[key] == nil {
			groups[key] = &selection{module: matches[0], name: qualified}
			keys = append(keys, key)
		}
		groups[key].cases = append(groups[key].cases, test)
	}
	slices.Sort(keys)
	for _, key := range keys {
		group := groups[key]
		test := group.cases[0]
		test.Package = group.module.ID
		test.Selector = group.name
		run, suites := r.executeCase(ctx, test, "enumerate")
		if !run.Passed() {
			return result, run.Result
		}
		for _, suite := range suites {
			for _, actual := range suite.Cases {
				test.Expected = append(test.Expected, junitName(actual))
			}
		}
		if len(test.Expected) == 0 {
			return result, failure(SelectionFailure, "JUnit 方法没有实际运行")
		}
		for _, actual := range test.Expected {
			if r.command.tool != "gradle" && !javaMethodMatches(actual, group.name) {
				return result, failure(SelectionFailure, "JUnit 实际执行超出所选方法: "+actual)
			}
		}
		if len(group.cases) > 1 || len(test.Source.Parameters) > 0 || len(test.Source.Unknown) > 0 || len(test.Expected) > 1 {
			// JUnit 的报告序号与构建工具选择语法没有统一契约，整个方法保留。
			whole := wholeCase(group.cases, group.name, test.Expected, "JUnit 参数项未证明能独立选择，按方法整体保留")
			whole.Package, whole.Selector = test.Package, test.Selector
			test = whole
		}
		result = append(result, test)
	}
	return result, Result{}
}

func javaMethodMatches(actual, selected string) bool {
	if actual == selected {
		return true
	}
	return strings.HasPrefix(actual, selected+"(") || strings.HasPrefix(actual, selected+"[")
}

func (r *JavaRunner) executeCase(ctx context.Context, test Case, stage string) (RunResult, []junitSuite) {
	ctx, done := runnerContext(ctx, r.context)
	defer done()
	select {
	case r.serial <- struct{}{}:
		defer func() { <-r.serial }()
	case <-ctx.Done():
		return RunResult{Result: contextResult(ctx.Err())}, nil
	}
	var module javaModule
	for _, candidate := range r.modules {
		if candidate.ID == test.Package {
			module = candidate
			break
		}
	}
	if module.ID == "" {
		return RunResult{Result: failure(SelectionFailure, "Java 测试模块未核对")}, nil
	}
	dir, err := os.MkdirTemp(r.exec.Output, "java-run-*")
	if err != nil {
		return RunResult{Result: failure(EnvironmentFailure, err.Error())}, nil
	}
	profile := filepath.Join(dir, "coverage.exec")
	reportDir := module.Reports
	var result Result
	if r.command.tool == "maven" {
		result = r.mavenRun(ctx, module, test.Selector, profile, stage)
	} else {
		result = r.gradleRun(ctx, module, test.Selector, dir, stage)
		reportDir = filepath.Join(dir, "junit")
	}
	run := RunResult{Result: result, Profile: filepath.Join(dir, "coverage.xml")}
	if result.Failure == Timeout || result.Failure == Canceled {
		return run, nil
	}
	suites, err := readJUnit(reportDir)
	if err != nil {
		if run.Passed() {
			run.Result = failure(SelectionFailure, err.Error())
		}
		return run, nil
	}
	if r.command.tool == "maven" {
		for _, other := range r.modules {
			if other.ID == module.ID {
				continue
			}
			extra, _ := readJUnit(other.Reports)
			for _, suite := range extra {
				if len(suite.Cases) > 0 {
					run.Result = failure(SelectionFailure, "Maven 同时执行了其他模块测试")
					return run, suites
				}
			}
		}
	}
	assertion, other, skipped := false, false, false
	var classpath []string
	for _, suite := range suites {
		for _, property := range suite.Properties {
			if property.Name == "java.class.path" || property.Name == "surefire.test.class.path" {
				classpath = append(classpath, filepath.SplitList(property.Value)...)
			}
		}
		for _, actual := range suite.Cases {
			run.Selected = append(run.Selected, junitName(actual))
			skipped = skipped || actual.Skipped != nil
			if actual.Failure != nil {
				if javaAssertionFailure(actual.Failure.Type) {
					assertion = true
				} else {
					other = true
				}
			}
			other = other || actual.Error != nil
		}
	}
	if len(test.Expected) > 0 && !sameNames(test.Expected, run.Selected) {
		run.Result = failure(SelectionFailure, "JUnit 实际执行名字与枚举不一致")
		return run, suites
	}
	if skipped || len(run.Selected) == 0 {
		run.Result = failure(SelectionFailure, "JUnit 用例跳过或未执行")
		return run, suites
	}
	if r.command.tool == "gradle" {
		if err := checkGradleSelection(dir, test.Selector, len(run.Selected)); err != nil {
			run.Result = failure(SelectionFailure, err.Error())
			return run, suites
		}
	}
	if !run.Passed() {
		if assertion && !other {
			run.Failure, run.Reason = AssertionFailure, "JUnit 断言失败"
		}
		return run, suites
	}
	if assertion || other {
		run.Result = failure(EnvironmentFailure, "JUnit 报告与进程状态不一致")
		return run, suites
	}
	if r.command.tool == "gradle" {
		var actual struct {
			Classpath  []string
			WorkingDir string
		}
		data, err := os.ReadFile(filepath.Join(dir, "actual.json"))
		if err != nil || len(data) > outputLimit || json.Unmarshal(data, &actual) != nil {
			run.Result = failure(CoverageFailure, "Gradle 实际 classpath 缺失")
			return run, suites
		}
		if err := r.javaClasspath(actual.Classpath, actual.WorkingDir); err != nil {
			run.Result = failure(CoverageFailure, err.Error())
			return run, suites
		}
		classpath = actual.Classpath
	}
	if len(classpath) == 0 {
		run.Result = failure(CoverageFailure, "JUnit 缺少实际 classpath")
		return run, suites
	}
	if err := r.javaClasspath(classpath, r.exec.Root); err != nil {
		run.Result = failure(CoverageFailure, err.Error())
		return run, suites
	}
	if !slices.ContainsFunc(classpath, func(path string) bool { return sameDirectory(path, module.Classes) }) {
		run.Result = failure(CoverageFailure, "Java 测试未采用副本生产 class 目录")
		return run, suites
	}
	reported := r.exec.Execute(ctx, "coverage", r.exec.Root, r.jdk("java", "-cp", r.readerPath, "TaskBelayCoverage", profile, module.Classes, module.Source, run.Profile), r.command.env)
	if !reported.Passed() {
		run.Result = reported
		if run.Failure != Canceled && run.Failure != Timeout {
			run.Failure = CoverageFailure
		}
		return run, suites
	}
	run.Coverage = readJaCoCo(ctx, run.Profile, module.Source, r.exec.Root, r.request.Snapshot)
	if !run.Coverage.Valid {
		run.Failure, run.Reason = CoverageFailure, run.Coverage.Reason
	}
	return run, suites
}

func (r *JavaRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	run, _ := r.executeCase(ctx, test, stage)
	return run
}
func (r *JavaRunner) ReadCoverage(ctx context.Context, run RunResult) Coverage {
	if err := ctx.Err(); err != nil {
		return Coverage{Reason: err.Error()}
	}
	return run.Coverage
}
func (r *JavaRunner) Terminate() {
	if r.cancel != nil {
		r.cancel()
	}
}

var _ Runner = (*JavaRunner)(nil)
