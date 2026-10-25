package testrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

type Scope struct{ OriginTree string }

func (s Scope) VerificationScope(ctx context.Context, workspace gitstate.Workspace, script shell.Script, tree string) (VerificationScope, error) {
	result := VerificationScope{Reason: "命令的输入或外部环境未完整核对"}
	if script.Mixed() {
		return result, nil
	}
	var command *shell.Segment
	for i := range script.Segments {
		segment := &script.Segments[i]
		if segment.Kind == shell.Verification {
			if command != nil {
				return result, nil
			}
			command = segment
		}
	}
	if command == nil || len(command.Args) == 0 || len(command.Prefix) > 0 || !shell.ReusableCommand(*command) || s.OriginTree == "" {
		return result, nil
	}
	program := shell.Base(command.Args[0])
	if program == "pytest" || strings.HasPrefix(program, "python") && len(command.Args) >= 3 && command.Args[1] == "-m" && command.Args[2] == "pytest" {
		return s.pythonScope(ctx, workspace, *command, tree)
	}
	if program == "jest" || program == "vitest" {
		return s.javascriptScope(ctx, workspace, *command, tree)
	}
	if program == "cargo" {
		return s.rustScope(ctx, workspace, *command, tree)
	}
	if javaTool(program) == "maven" {
		return s.mavenScope(ctx, workspace, *command, tree)
	}
	if len(command.Args) < 2 || program != "go" || command.Args[1] != "test" {
		return result, nil
	}
	for _, arg := range command.Args[2:] {
		if strings.HasPrefix(arg, "-") {
			result.Reason = "带额外选择或构建参数的包范围尚未在快速路径核对"
			return result, nil
		}
	}
	changes, err := workspace.Diff(ctx, s.OriginTree, tree)
	if err != nil {
		return result, err
	}
	base, err := os.MkdirTemp("", "taskbelay-scope-*")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(base)
	environment := os.Environ()
	for key, value := range command.Environment {
		environment = append(environment, key+"="+value)
	}
	executor := &Executor{Root: workspace.Root, Output: base, Env: environment}
	listed := executor.Execute(ctx, "scope", workspace.Root, []string{command.Args[0], "list", "-json", "-deps", "-test", "./..."}, nil)
	if !listed.Passed() || listed.Truncated {
		result.Reason = "go list 未完整返回包及测试导入关系"
		return result, nil
	}
	root, err := filepath.EvalSymlinks(workspace.Root)
	if err != nil {
		return result, err
	}
	packages := map[string]goPackage{}
	outside := false
	decoder := json.NewDecoder(strings.NewReader(listed.Output))
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return result, err
		}
		if pkg.Standard || pkg.ForTest != "" || strings.HasSuffix(pkg.ImportPath, ".test") {
			continue
		}
		rel, err := filepath.Rel(root, pkg.Dir)
		if err != nil || rel != "." && !filepath.IsLocal(rel) {
			outside = true
			continue
		}
		if pkg.Incomplete || pkg.Error != nil {
			return result, nil
		}
		packages[pkg.ImportPath] = pkg
	}
	changed := map[string]bool{}
	shared := false
	for _, change := range changes {
		for _, path := range []string{change.Path, change.BeforePath} {
			if path == "" {
				continue
			}
			if !strings.HasSuffix(path, ".go") {
				shared = true
				continue
			}
			for name, pkg := range packages {
				rel, _ := filepath.Rel(root, pkg.Dir)
				if filepath.Clean(filepath.Dir(filepath.FromSlash(path))) == filepath.Clean(rel) {
					changed[name] = true
				}
			}
		}
	}
	affected := affectedGoPackages(packages, changed)
	commandDir, dirErr := filepath.EvalSymlinks(command.Dir)
	broad := dirErr == nil && commandDir == root && len(command.Args) == 3 && command.Args[2] == "./..."
	if len(affected) > 0 && len(affected) < len(packages) && !shared && broad {
		result.Suggestion = "go test " + strings.Join(affected, " ")
		result.Evidence = append(result.Evidence, "按改动包以及直接、间接导入它的包计算，包含 TestImports、XTestImports")
	} else if shared {
		result.Evidence = append(result.Evidence, "存在共享配置或非 Go 文件变化，本次不缩小验证范围")
	}
	input, _ := json.Marshal(struct {
		Tree     string
		Packages map[string]goPackage
	}{tree, packages})
	result.InputID = fmt.Sprintf("%x", sha256.Sum256(input))
	result.Complete = !outside
	version := executor.Execute(ctx, "scope", command.Dir, []string{command.Args[0], "version"}, nil)
	if version.Passed() {
		result.ToolID = fmt.Sprintf("%x", sha256.Sum256([]byte(version.Output)))
	} else {
		result.Complete = false
	}
	// 包关系无法证明数据库、端口及测试外部状态相同；V1 仍需完整环境依据。
	result.EnvironmentKnown = false
	result.Reason = "Go 包及测试导入范围已记录；外部运行环境和工具链内容未在快速路径完整核对，V1 只提示"
	if result.Complete && len(command.Environment) == 0 && !command.ClearEnvironment && len(command.Unset) == 0 {
		files, err := workspace.TreeFiles(ctx, tree)
		if err != nil {
			return result, err
		}
		var ids, paths []string
		for path, file := range files {
			if strings.HasSuffix(path, ".go") && (file.Mode == "100644" || file.Mode == "100755") {
				paths = append(paths, path)
				ids = append(ids, file.Blob)
			}
		}
		sort.Strings(paths)
		contents, err := workspace.ReadBlobs(ctx, ids)
		if err != nil {
			return result, err
		}
		var sources []codescan.Source
		for _, path := range paths {
			sources = append(sources, codescan.Source{Path: path, Blob: files[path].Blob, Mode: files[path].Mode, Data: contents[files[path].Blob]})
		}
		snapshot := codescan.Analyze(ctx, tree, sources)
		known := ctx.Err() == nil
		for _, file := range snapshot.Files {
			known = known && file.CodeComplete
		}
		runner := GoRunner{packages: packages, request: Request{Snapshot: snapshot}}
		if known && runner.Reusable() {
			identity, err := ExecutionIdentity(ctx, command.Dir)
			if err == nil {
				result.ToolID = identity
				result.EnvironmentID = identity
				result.EnvironmentKnown = true
				result.Reason = "固定源码、Go 包范围、纯本地运行依赖及工具链内容已核对"
			}
		}
	}
	return result, nil
}

func affectedGoPackages(packages map[string]goPackage, changed map[string]bool) []string {
	for progress := true; progress; {
		progress = false
		for name, pkg := range packages {
			if changed[name] {
				continue
			}
			imports := append(append(slices.Clone(pkg.Imports), pkg.TestImports...), pkg.XTestImports...)
			for _, dependency := range imports {
				if changed[dependency] {
					changed[name] = true
					progress = true
					break
				}
			}
		}
	}
	var result []string
	for name := range changed {
		if _, exists := packages[name]; exists {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}
