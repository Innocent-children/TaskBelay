package testrun

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type goPackage struct {
	ImportPath, Dir, Name, ForTest               string
	Standard, DepOnly, Incomplete                bool
	GoFiles, CgoFiles, TestGoFiles, XTestGoFiles []string
	SFiles, CFiles, CXXFiles, SwigFiles          []string
	Imports, TestImports, XTestImports           []string
	Error                                        *struct{ Err string }
	Module                                       *struct {
		Path, Dir string
		Main      bool
	}
}

type GoRunner struct {
	exec          *Executor
	request       Request
	config        Config
	command       goCommand
	packages      map[string]goPackage
	byFile        map[string]string
	coverageFiles map[string]string
	binaries      map[string]string
	coverPackages []string
	version       int
	mu            sync.Mutex
	profiles      int
	cancel        context.CancelFunc
	context       context.Context
}

func NewGoRunner(executor *Executor, request Request) *GoRunner {
	return &GoRunner{exec: executor, request: request, config: request.Config["go"], packages: map[string]goPackage{}, byFile: map[string]string{}, coverageFiles: map[string]string{}, binaries: map[string]string{}}
}

func failure(kind Failure, reason string) Result {
	return Result{Failure: kind, Reason: reason, ExitCode: -1}
}

func (g *GoRunner) Prepare(ctx context.Context) Result {
	ctx, g.cancel = context.WithCancel(ctx)
	g.context = ctx
	var err error
	g.exec.Root, err = filepath.EvalSymlinks(g.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	g.command, err = parseGoCommand(g.config.Command, g.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	cache, tmp := filepath.Join(g.exec.Output, "go-cache"), filepath.Join(g.exec.Output, "go-tmp")
	for _, path := range []string{cache, tmp} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return failure(EnvironmentFailure, err.Error())
		}
	}
	g.command.env = mergeEnvironment(g.command.env, []string{"GOCACHE=" + cache, "GOTMPDIR=" + tmp, "GOPROXY=off", "PWD=" + g.exec.Root})
	g.exec.Env = mergeEnvironment(g.exec.Env, g.command.env)
	if result := g.exec.Prepare(ctx, g.config.Prepare); !result.Passed() {
		return result
	}
	environment := g.exec.Execute(ctx, "prepare", g.exec.Root, g.command.args("env", "-json", "GOCACHE", "GOTMPDIR"), g.command.env)
	if !environment.Passed() {
		return environment
	}
	var destinations map[string]string
	if json.Unmarshal([]byte(environment.Output), &destinations) != nil || filepath.Clean(destinations["GOCACHE"]) != filepath.Clean(cache) || filepath.Clean(destinations["GOTMPDIR"]) != filepath.Clean(tmp) {
		return failure(EnvironmentFailure, "项目命令覆盖了临时缓存或构建输出位置，不能保证分析产物隔离")
	}
	version := g.exec.Execute(ctx, "prepare", g.exec.Root, g.command.args("version"), g.command.env)
	if !version.Passed() {
		return version
	}
	args := append([]string{"-json", "-deps", "-test"}, g.command.build...)
	args = append(args, g.command.packages...)
	listed := g.exec.Execute(ctx, "prepare", g.exec.Root, g.command.args("list", args...), g.command.env)
	if !listed.Passed() {
		return listed
	}
	if listed.Truncated {
		return failure(EnvironmentFailure, "Go 包清单超出输出预算，不能确认全部范围")
	}
	decoder := json.NewDecoder(strings.NewReader(listed.Output))
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return failure(EnvironmentFailure, "Go 包清单无法读取: "+err.Error())
		}
		if pkg.Error != nil || pkg.Incomplete {
			return failure(EnvironmentFailure, "Go 包输入不完整: "+pkg.ImportPath)
		}
		if pkg.ForTest != "" || strings.HasSuffix(pkg.ImportPath, ".test") {
			continue
		}
		if pkg.Standard {
			continue
		}
		rel, err := filepath.Rel(g.exec.Root, pkg.Dir)
		inside := err == nil && (rel == "." || filepath.IsLocal(rel))
		if !inside {
			if pkg.Module != nil && pkg.Module.Main {
				return failure(EnvironmentFailure, "Go 主模块实际位于副本之外: "+pkg.ImportPath)
			}
			continue
		}
		g.packages[pkg.ImportPath] = pkg
		for _, name := range append(append(slices.Clone(pkg.GoFiles), pkg.CgoFiles...), append(slices.Clone(pkg.TestGoFiles), pkg.XTestGoFiles...)...) {
			path := filepath.ToSlash(filepath.Join(rel, name))
			file := g.request.Snapshot.Files[path]
			if file == nil {
				return failure(EnvironmentFailure, "实际编译源码不在固定快照: "+path)
			}
			data, err := os.ReadFile(filepath.Join(pkg.Dir, name))
			if err != nil || !bytes.Equal(data, file.Data) {
				return failure(EnvironmentFailure, "准备后源码与固定快照不同: "+path)
			}
			g.byFile[path] = pkg.ImportPath
			g.coverageFiles[pkg.ImportPath+"/"+name] = path
			g.coverageFiles[filepath.ToSlash(filepath.Join(pkg.Dir, name))] = path
		}
	}
	for _, lines := range g.request.Requirements {
		for _, line := range lines {
			if pkg := g.byFile[line.File]; pkg != "" {
				g.coverPackages = append(g.coverPackages, pkg)
			}
		}
	}
	sort.Strings(g.coverPackages)
	g.coverPackages = slices.Compact(g.coverPackages)
	if len(g.coverPackages) == 0 {
		return failure(CoverageFailure, "没有可定位到 Go 副本包的当前需求代码；删除入口等验证保留原用途")
	}
	return Result{}
}

func (g *GoRunner) Compile(ctx context.Context) Result {
	ctx, end := g.operationContext(ctx)
	defer end()
	g.version++
	var packages []string
	for _, test := range g.request.Cases {
		if pkg := g.byFile[test.Source.File]; pkg != "" && !g.packages[pkg].DepOnly && !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	sort.Strings(packages)
	for i, pkg := range packages {
		name := fmt.Sprintf("go-%d-%d.test", g.version, i)
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		binary := filepath.Join(g.exec.Output, name)
		args := append(slices.Clone(g.command.build), "-c", "-cover", "-covermode=set", "-coverpkg="+strings.Join(g.coverPackages, ","), "-buildvcs=false", "-o", binary, pkg)
		result := g.exec.Execute(ctx, "compile", g.exec.Root, g.command.args("test", args...), g.command.env)
		if !result.Passed() {
			return result
		}
		if _, err := os.Stat(binary); err != nil {
			return failure(CompileFailure, "Go 未生成预期测试程序: "+pkg)
		}
		g.binaries[pkg] = binary
	}
	return Result{}
}

func (g *GoRunner) Terminate() {
	if g.cancel != nil {
		g.cancel()
	}
}

func (g *GoRunner) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	child, cancel := context.WithCancel(ctx)
	if g.context == nil {
		return child, cancel
	}
	stop := context.AfterFunc(g.context, cancel)
	return child, func() { stop(); cancel() }
}

func exactGoPattern(name string) string {
	parts := strings.Split(name, "/")
	for i := range parts {
		parts[i] = "^" + regexp.QuoteMeta(parts[i]) + "$"
	}
	return strings.Join(parts, "/")
}

func selectedGoNames(output string) []string {
	var names []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "=== RUN   ") {
			names = append(names, strings.TrimSpace(strings.TrimPrefix(line, "=== RUN   ")))
		}
	}
	sort.Strings(names)
	return slices.Compact(names)
}

func goLeaves(names []string) []string {
	var result []string
	for _, name := range names {
		child := false
		for _, other := range names {
			if strings.HasPrefix(other, name+"/") {
				child = true
				break
			}
		}
		if !child {
			result = append(result, name)
		}
	}
	return result
}

func (g *GoRunner) execute(ctx context.Context, test Case, stage string) RunResult {
	ctx, end := g.operationContext(ctx)
	defer end()
	binary := g.binaries[test.Package]
	if binary == "" {
		return RunResult{Result: failure(EnvironmentFailure, "用例没有已编译的副本测试程序")}
	}
	g.mu.Lock()
	g.profiles++
	sequence := g.profiles
	g.mu.Unlock()
	profile := filepath.Join(g.exec.Output, "go-cover-"+strconv.Itoa(sequence)+".out")
	args := append([]string{binary}, g.command.runtime...)
	pattern := exactGoPattern(test.Name)
	if stage == "enumerate" && g.command.filter != "" {
		parts := strings.Split(g.command.filter, "/")
		if len(parts) > 1 {
			pattern += "/" + strings.Join(parts[1:], "/")
		}
	}
	args = append(args, "-test.v", "-test.count=1", "-test.parallel=1", "-test.run="+pattern, "-test.coverprofile="+profile)
	result := g.exec.Execute(ctx, stage, g.packages[test.Package].Dir, args, g.command.env)
	names := selectedGoNames(result.Output)
	if result.Truncated {
		result.Failure, result.Reason = SelectionFailure, "Go 输出不完整，不能核对实际用例"
	}
	if result.ExitCode != 0 && result.Failure == EnvironmentFailure && strings.Contains(result.Output, "--- FAIL: ") && !strings.Contains(result.Output, "panic:") && !strings.Contains(result.ErrorOutput, "panic:") {
		result.Failure, result.Reason = AssertionFailure, "选中测试报告失败；须结合已提取的断言核对"
	}
	return RunResult{Result: result, Selected: names, Profile: profile}
}

func (g *GoRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	result := g.execute(ctx, test, stage)
	leaves := goLeaves(result.Selected)
	expected := test.Expected
	if len(expected) == 0 {
		expected = []string{test.Name}
	}
	if !slices.Equal(leaves, expected) {
		result.Failure, result.Reason = SelectionFailure, "实际运行用例与选中范围不一致"
	}
	for _, line := range strings.Split(result.Output, "\n") {
		if strings.Contains(line, "--- SKIP:") {
			result.Failure, result.Reason = SelectionFailure, "选中用例包含跳过，不能作为通过的基线"
		}
	}
	if result.Passed() {
		result.Coverage = g.ReadCoverage(ctx, result)
	}
	return result
}

func (g *GoRunner) Enumerate(ctx context.Context, tests []Case) ([]Case, Result) {
	var result []Case
	groups := map[string][]Case{}
	for _, test := range tests {
		if g.command.filter != "" {
			matches, err := matchGoFilter(g.command.filter, test.Source.Name)
			if err != nil {
				return result, failure(SelectionFailure, err.Error())
			}
			if !matches {
				test.Unknown = append(test.Unknown, "项目 -run 选择范围未包含本用例")
				result = append(result, test)
				continue
			}
		}
		pkg := g.byFile[test.Source.File]
		if pkg == "" || g.packages[pkg].DepOnly {
			test.Unknown = append(test.Unknown, "用例未被项目的 Go 构建与包范围选中")
			result = append(result, test)
			continue
		}
		test.Package = pkg
		outer := strings.Split(test.Source.Name, "/")[0]
		groups[pkg+"\x00"+outer] = append(groups[pkg+"\x00"+outer], test)
	}
	var keys []string
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cases := groups[key]
		parts := strings.Split(key, "\x00")
		outer := cases[0]
		outer.Name = parts[1]
		outer.Whole = true
		observed := g.execute(ctx, outer, "enumerate")
		if !observed.Passed() {
			return result, observed.Result
		}
		leaves := goLeaves(observed.Selected)
		if len(leaves) == 0 {
			return result, failure(SelectionFailure, "外层测试没有实际运行: "+outer.Name)
		}
		unique := len(cases) == len(leaves)
		for _, test := range cases {
			if len(test.Source.Unknown) > 0 || !slices.Contains(leaves, test.Source.Name) {
				unique = false
			}
		}
		if !unique {
			outer.At = "test:" + outer.Source.File + ":" + outer.Name
			outer.Expected = leaves
			outer.Unknown = append(outer.Unknown, "运行名字不能逐行对应，保留外层整体，不拆删除候选")
			for _, test := range cases {
				outer.Mixed = outer.Mixed || test.Mixed
				outer.Retained = outer.Retained || test.Retained
				outer.AgentOnly = outer.AgentOnly && test.AgentOnly
				outer.Requirements = append(outer.Requirements, test.Requirements...)
			}
			sort.Strings(outer.Requirements)
			outer.Requirements = slices.Compact(outer.Requirements)
			result = append(result, outer)
			continue
		}
		for _, test := range cases {
			test.Name = test.Source.Name
			test.Expected = []string{test.Name}
			if g.command.filter != "" {
				match, err := matchGoFilter(g.command.filter, test.Name)
				if err != nil {
					return result, failure(SelectionFailure, err.Error())
				}
				if !match {
					test.Unknown = append(test.Unknown, "项目 -run 选择范围未包含本用例")
					test.Package = ""
				}
			}
			result = append(result, test)
		}
	}
	return result, Result{}
}

func matchGoFilter(pattern, name string) (bool, error) {
	// Go 在斜杠处分段匹配。字符类中的斜杠有特殊语义，留待原生选择核对。
	if strings.ContainsAny(pattern, "[]") {
		return false, fmt.Errorf("带字符类的 Go -run 尚不能核对分段范围")
	}
	parts, names := strings.Split(pattern, "/"), strings.Split(name, "/")
	for i, p := range parts {
		if i >= len(names) {
			return false, nil
		}
		ok, err := regexp.MatchString(p, names[i])
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}
