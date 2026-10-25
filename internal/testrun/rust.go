package testrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell/posix"
)

type cargoMetadata struct {
	Packages  []cargoPackage `json:"packages"`
	Members   []string       `json:"workspace_members"`
	Defaults  []string       `json:"workspace_default_members"`
	TargetDir string         `json:"target_directory"`
	Resolve   struct {
		Nodes []struct {
			ID           string   `json:"id"`
			Dependencies []string `json:"dependencies"`
		} `json:"nodes"`
	} `json:"resolve"`
}

type cargoPackage struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Manifest string `json:"manifest_path"`
	Targets  []struct {
		Name   string   `json:"name"`
		Kind   []string `json:"kind"`
		Source string   `json:"src_path"`
		Test   bool     `json:"test"`
	} `json:"targets"`
}

type rustTarget struct{ Key, Package, Directory, Source, Kind, Name string }

type RustRunner struct {
	exec     *Executor
	request  Request
	command  cargoCommand
	metadata cargoMetadata
	targets  []rustTarget
	names    map[string][]string
	context  context.Context
	cancel   context.CancelFunc
	serial   chan struct{}
}

func NewRustRunner(executor *Executor, request Request) *RustRunner {
	return &RustRunner{exec: executor, request: request, names: map[string][]string{}, serial: make(chan struct{}, 1)}
}

func (r *RustRunner) Prepare(ctx context.Context) Result {
	r.context, r.cancel = context.WithCancel(ctx)
	ctx = r.context
	root, err := filepath.EvalSymlinks(r.exec.Root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	r.exec.Root = root
	r.command, err = parseCargoCommand(r.request.Config["rust"].Command, root)
	if err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	target := filepath.Join(r.exec.Output, "rust-target")
	if err := os.MkdirAll(target, 0700); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	r.command.env = mergeEnvironment(r.command.env, []string{"CARGO_TARGET_DIR=" + target, "CARGO_LLVM_COV_TARGET_DIR=" + target, "CARGO_LLVM_COV_BUILD_DIR=" + target, "CARGO_NET_OFFLINE=true", "RUSTUP_AUTO_INSTALL=0", "CARGO_TERM_COLOR=never", "PWD=" + root})
	r.exec.Env = mergeEnvironment(r.exec.Env, r.command.env)
	if result := r.exec.Prepare(ctx, r.request.Config["rust"].Prepare); !result.Passed() {
		return result
	}
	if err := checkCopy(r.exec, r.request.Snapshot, "rust"); err != nil {
		return failure(EnvironmentFailure, err.Error())
	}
	version := r.exec.Execute(ctx, "prepare", root, r.command.rustc("-vV"), r.command.env)
	if !version.Passed() {
		return version
	}
	host := ""
	for _, line := range strings.Split(version.Output, "\n") {
		if value, ok := strings.CutPrefix(line, "host: "); ok {
			host = strings.TrimSpace(value)
		}
	}
	sysroot := r.exec.Execute(ctx, "prepare", root, r.command.rustc("--print", "sysroot"), r.command.env)
	if !sysroot.Passed() || host == "" {
		return failure(EnvironmentFailure, "Rust 工具链实际位置与主机目标无法核对")
	}
	for _, name := range []string{"llvm-cov", "llvm-profdata"} {
		binary := name
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		path := filepath.Join(strings.TrimSpace(sysroot.Output), "lib", "rustlib", host, "bin", binary)
		key := "LLVM_COV"
		if name == "llvm-profdata" {
			key = "LLVM_PROFDATA"
		}
		for _, entry := range mergeEnvironment(r.exec.Env, r.command.env) {
			if value, ok := strings.CutPrefix(entry, key+"="); ok {
				path = value
			}
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return failure(EnvironmentFailure, "项目 Rust 工具链缺少 "+name+"，不自动安装")
		}
	}
	capability := r.exec.Execute(ctx, "prepare", root, r.command.args("llvm-cov", "--version"), r.command.env)
	if !capability.Passed() {
		return capability
	}
	args := append([]string{"--format-version=1", "--offline"}, r.command.metadata...)
	listed := r.exec.Execute(ctx, "prepare", root, r.command.args("metadata", args...), r.command.env)
	if !listed.Passed() {
		return listed
	}
	if listed.Truncated || json.Unmarshal([]byte(listed.Output), &r.metadata) != nil {
		return failure(EnvironmentFailure, "Cargo 元数据不完整")
	}
	if !sameDirectory(r.metadata.TargetDir, target) {
		return failure(EnvironmentFailure, "Cargo 构建输出被配置到临时目录之外")
	}
	shown := r.exec.Execute(ctx, "prepare", root, r.command.args("llvm-cov", append([]string{"show-env"}, r.command.metadata...)...), r.command.env)
	if !shown.Passed() {
		return shown
	}
	actual := map[string]string{}
	parsed := (posix.Parser{}).Parse(shown.Output, root)
	if parsed.Complex {
		return failure(EnvironmentFailure, "cargo-llvm-cov 的实际输出位置无法解析")
	}
	for _, segment := range parsed.Segments {
		if !segment.Known || len(segment.Args) > 0 {
			return failure(EnvironmentFailure, "cargo-llvm-cov 环境报告不是字面赋值")
		}
		for key, value := range segment.Environment {
			actual[key] = value
		}
	}
	actualTarget := actual["CARGO_LLVM_COV_TARGET_DIR"]
	if actualTarget == "" {
		actualTarget = actual["CARGO_TARGET_DIR"]
	}
	if !sameDirectory(actualTarget, target) || !sameDirectory(filepath.Dir(actual["LLVM_PROFILE_FILE"]), target) {
		return failure(EnvironmentFailure, "cargo-llvm-cov 覆盖或构建位置不在本次临时目录")
	}
	if build := actual["CARGO_LLVM_COV_BUILD_DIR"]; build != "" && !sameDirectory(build, target) {
		return failure(EnvironmentFailure, "cargo-llvm-cov 中间构建位置被覆盖")
	}
	for _, pkg := range r.metadata.Packages {
		if !slices.Contains(r.metadata.Members, pkg.ID) || !r.selectedPackage(pkg) {
			continue
		}
		manifest, inside := copyFile(root, pkg.Manifest)
		if !inside {
			return failure(EnvironmentFailure, "Cargo workspace 清单位于副本之外")
		}
		for _, target := range pkg.Targets {
			kind := ""
			for _, candidate := range []string{"lib", "proc-macro", "bin", "test"} {
				if slices.Contains(target.Kind, candidate) {
					kind = candidate
					break
				}
			}
			if kind == "proc-macro" {
				kind = "lib"
			}
			if kind == "" || !target.Test || !r.selectedTarget(kind, target.Name) {
				continue
			}
			source, inside := copyFile(root, target.Source)
			if !inside || r.request.Snapshot.Files[source] == nil {
				return failure(EnvironmentFailure, "Cargo 测试 target 源码不在固定副本: "+target.Name)
			}
			r.targets = append(r.targets, rustTarget{Key: pkg.Name + "::" + kind + "::" + target.Name, Package: pkg.Name, Directory: filepath.ToSlash(filepath.Dir(manifest)), Source: source, Kind: kind, Name: target.Name})
		}
	}
	slices.SortFunc(r.targets, func(a, b rustTarget) int { return strings.Compare(a.Key, b.Key) })
	if len(r.targets) == 0 {
		return failure(SelectionFailure, "项目配置没有选中可核对的 Cargo 测试 target")
	}
	return Result{}
}

func sameDirectory(a, b string) bool {
	left, e1 := filepath.EvalSymlinks(a)
	right, e2 := filepath.EvalSymlinks(b)
	return e1 == nil && e2 == nil && left == right
}

func (r *RustRunner) selectedPackage(pkg cargoPackage) bool {
	var packages, excluded []string
	workspace := false
	for i := 0; i < len(r.command.options); i++ {
		switch r.command.options[i] {
		case "-p", "--package":
			i++
			packages = append(packages, r.command.options[i])
		case "--exclude":
			i++
			excluded = append(excluded, r.command.options[i])
		case "--workspace":
			workspace = true
		}
	}
	if slices.Contains(excluded, pkg.Name) {
		return false
	}
	if len(packages) > 0 {
		return slices.Contains(packages, pkg.Name)
	}
	if workspace {
		return true
	}
	return slices.Contains(r.metadata.Defaults, pkg.ID)
}

func (r *RustRunner) selectedTarget(kind, name string) bool {
	filtered, matched := false, false
	for i := 0; i < len(r.command.options); i++ {
		switch r.command.options[i] {
		case "--lib":
			filtered = true
			matched = matched || kind == "lib"
		case "--bins":
			filtered = true
			matched = matched || kind == "bin"
		case "--tests":
			filtered = true
			matched = true
		case "--test", "--bin":
			flag := r.command.options[i]
			i++
			filtered = true
			matched = matched || (kind == strings.TrimPrefix(flag, "--") && name == r.command.options[i])
		}
	}
	return !filtered || matched
}

func (r *RustRunner) targetArgs(target rustTarget) []string {
	var args []string
	for i := 0; i < len(r.command.options); i++ {
		switch r.command.options[i] {
		case "-p", "--package", "--test", "--bin", "--exclude":
			i++
		case "--lib", "--tests", "--bins", "--workspace":
		default:
			args = append(args, r.command.options[i])
		}
	}
	args = append(args, "--package", target.Package)
	if target.Kind == "lib" {
		args = append(args, "--lib")
	} else {
		args = append(args, "--"+target.Kind, target.Name)
	}
	return args
}

func (r *RustRunner) Compile(ctx context.Context) Result {
	ctx, done := runnerContext(ctx, r.context)
	defer done()
	for _, target := range r.targets {
		args := append(r.targetArgs(target), "--no-report", "--", "--list")
		listed := r.exec.Execute(ctx, "compile", r.exec.Root, r.command.args("llvm-cov", args...), r.command.env)
		if !listed.Passed() {
			return listed
		}
		if listed.Truncated {
			return failure(SelectionFailure, "Rust 测试列表超出输出预算")
		}
		var names []string
		for _, line := range strings.Split(listed.Output, "\n") {
			if name, ok := strings.CutSuffix(strings.TrimSpace(line), ": test"); ok {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		r.names[target.Key] = names
	}
	return Result{}
}

func (r *RustRunner) Enumerate(ctx context.Context, tests []Case) ([]Case, Result) {
	var result []Case
	for _, test := range tests {
		if err := ctx.Err(); err != nil {
			return result, contextResult(err)
		}
		var matches []struct {
			target rustTarget
			names  []string
		}
		for _, target := range r.targets {
			directTarget := false
			for _, candidate := range r.targets {
				if candidate.Source == test.Source.File {
					directTarget = true
					break
				}
			}
			if directTarget && target.Source != test.Source.File {
				continue
			}
			prefix := target.Directory
			if prefix == "." {
				prefix = ""
			} else {
				prefix += "/"
			}
			local := strings.TrimPrefix(test.Source.File, prefix)
			if prefix != "" && local == test.Source.File {
				continue
			}
			if strings.HasPrefix(local, "tests/") && target.Kind != "test" {
				continue
			}
			if !strings.HasPrefix(local, "tests/") && target.Kind == "test" {
				continue
			}
			if strings.HasSuffix(local, "/lib.rs") && target.Kind != "lib" {
				continue
			}
			if strings.HasSuffix(local, "/main.rs") && target.Kind != "bin" {
				continue
			}
			var names []string
			for _, name := range r.names[target.Key] {
				if !r.command.matches(name) {
					continue
				}
				if name == test.Source.Name || len(test.Source.Unknown) > 0 && strings.HasPrefix(name, test.Source.Name+"::") {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				matches = append(matches, struct {
					target rustTarget
					names  []string
				}{target, names})
			}
		}
		if len(matches) != 1 {
			test.Unknown = append(test.Unknown, "Rust 源码与所选 package/target 的原生名字不能唯一对应")
			result = append(result, test)
			continue
		}
		test.Package = matches[0].target.Key
		test.Expected = matches[0].names
		if len(test.Expected) != 1 || len(test.Source.Unknown) > 0 {
			test = wholeCase([]Case{test}, test.Source.Name, test.Expected, "原生参数名字无法逐例对应，保留外层整体")
		}
		result = append(result, test)
	}
	return result, Result{}
}

func contextResult(err error) Result {
	if err == context.DeadlineExceeded {
		return failure(Timeout, err.Error())
	}
	return failure(Canceled, err.Error())
}

func (r *RustRunner) Run(ctx context.Context, test Case, stage string) RunResult {
	ctx, done := runnerContext(ctx, r.context)
	defer done()
	select {
	case r.serial <- struct{}{}:
		defer func() { <-r.serial }()
	case <-ctx.Done():
		return RunResult{Result: contextResult(ctx.Err())}
	}
	var target rustTarget
	for _, candidate := range r.targets {
		if candidate.Key == test.Package {
			target = candidate
			break
		}
	}
	if target.Key == "" {
		return RunResult{Result: failure(SelectionFailure, "Rust 用例没有可核对的测试 target")}
	}
	dir, err := os.MkdirTemp(r.exec.Output, "rust-run-*")
	if err != nil {
		return RunResult{Result: failure(EnvironmentFailure, err.Error())}
	}
	profile := filepath.Join(dir, "coverage.lcov")
	args := append(r.targetArgs(target), "--lcov", "--output-path", profile, "--", test.Name)
	if !test.Whole {
		args = append(args, "--exact")
	}
	args = append(args, "--test-threads=1")
	run := RunResult{Result: r.exec.Execute(ctx, stage, r.exec.Root, r.command.args("llvm-cov", args...), r.command.env), Profile: profile}
	if run.Failure == Canceled || run.Failure == Timeout {
		return run
	}
	pattern := regexp.MustCompile(`(?m)^test (.+) \.\.\. (ok|FAILED|ignored)[^\r\n]*`)
	failed, ignored := false, false
	for _, match := range pattern.FindAllStringSubmatch(run.Output, -1) {
		run.Selected = append(run.Selected, match[1])
		failed = failed || match[2] == "FAILED"
		ignored = ignored || match[2] == "ignored"
	}
	if strings.Contains(run.ErrorOutput, "could not compile") {
		run.Failure, run.Reason = CompileFailure, "Rust 编译失败"
		return run
	}
	if !run.Passed() && failed && (strings.Contains(run.Output, "panicked at") || strings.Contains(run.ErrorOutput, "panicked at")) {
		run.Failure, run.Reason = AssertionFailure, "Rust 测试断言或 panic 失败"
	}
	if (run.Passed() || run.Failure == AssertionFailure) && (ignored || !sameNames(run.Selected, test.Expected)) {
		run.Failure, run.Reason = SelectionFailure, "Rust 实际执行范围或跳过状态与选择不一致"
	}
	if run.Passed() {
		run.Coverage = r.ReadCoverage(ctx, run)
	}
	return run
}

func (r *RustRunner) ReadCoverage(ctx context.Context, run RunResult) Coverage {
	return readLCOV(ctx, r.exec.Root, r.exec.Workspace, run.Profile, r.request.Snapshot)
}
func (r *RustRunner) Terminate() {
	if r.cancel != nil {
		r.cancel()
	}
}
