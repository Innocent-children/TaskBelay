package testrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func scopeCopy(ctx context.Context, workspace gitstate.Workspace, tree string) (*Executor, func(), error) {
	base, err := os.MkdirTemp("", "taskbelay-scope-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(base) }
	root, out := filepath.Join(base, "source"), filepath.Join(base, "output")
	if err := workspace.ExportTree(ctx, tree, root); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err := os.Mkdir(out, 0700); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return &Executor{Root: root, Output: out, Workspace: workspace.Root, Env: mergeEnvironment(os.Environ(), []string{"PYTHONDONTWRITEBYTECODE=1", "TMPDIR=" + out, "TMP=" + out, "TEMP=" + out})}, cleanup, nil
}

func scopeQuote(path string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

type pythonImports struct {
	Path    string `json:"path"`
	Imports []struct {
		Module string   `json:"module"`
		Level  int      `json:"level"`
		Names  []string `json:"names"`
	} `json:"imports"`
	Dynamic bool `json:"dynamic"`
}

const pythonImportGraph = `import ast,json,sys
result=[]
for path in json.load(open(sys.argv[1],encoding='utf-8')):
    with open(path,encoding='utf-8-sig') as f: tree=ast.parse(f.read(),filename=path)
    imports=[]; dynamic=False
    for node in ast.walk(tree):
        if isinstance(node,ast.Import):
            imports.extend(dict(module=n.name,level=0,names=[]) for n in node.names)
            dynamic |= any(n.name.split('.')[0] in ('importlib','runpy','pkgutil') for n in node.names)
        elif isinstance(node,ast.ImportFrom):
            imports.append(dict(module=node.module or '',level=node.level,names=[n.name for n in node.names]))
            dynamic |= any(n.name=='*' for n in node.names)
            dynamic |= (node.module or '').split('.')[0] in ('importlib','runpy','pkgutil')
        elif isinstance(node,ast.Name):
            dynamic |= node.id in ('__import__','exec','eval')
        elif isinstance(node,ast.Call):
            name=getattr(node.func,'id',getattr(node.func,'attr',''))
            dynamic |= name in ('__import__','import_module','exec','eval')
    result.append(dict(path=path,imports=imports,dynamic=dynamic))
print(json.dumps(dict(files=result,version=sys.version)))`

func (s Scope) pythonScope(ctx context.Context, workspace gitstate.Workspace, command shell.Segment, tree string) (VerificationScope, error) {
	result := VerificationScope{Reason: "Python 导入图不包含动态加载与外部状态；V1 只提示"}
	plain := len(command.Args) == 1 || len(command.Args) == 3 && command.Args[1] == "-m"
	if !plain || command.Dir != workspace.Root {
		return result, nil
	}
	executor, cleanup, err := scopeCopy(ctx, workspace, tree)
	if err != nil {
		return result, err
	}
	defer cleanup()
	for key, value := range command.Environment {
		executor.Env = mergeEnvironment(executor.Env, []string{key + "=" + value})
	}
	files, err := workspace.TreeFiles(ctx, tree)
	if err != nil {
		return result, err
	}
	var paths []string
	for path := range files {
		if strings.HasSuffix(path, ".py") {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	list := filepath.Join(executor.Output, "sources.json")
	data, _ := json.Marshal(paths)
	if err := os.WriteFile(list, data, 0600); err != nil {
		return result, err
	}
	python := "python3"
	for _, name := range []string{".venv", "venv"} {
		candidate := filepath.Join(workspace.Root, name, "bin", "python")
		if runtime.GOOS == "windows" {
			candidate = filepath.Join(workspace.Root, name, "Scripts", "python.exe")
		}
		if _, err := os.Stat(candidate); err == nil {
			python = candidate
			break
		}
	}
	if strings.HasPrefix(shell.Base(command.Args[0]), "python") {
		python = command.Args[0]
	}
	run := executor.Execute(ctx, "scope", executor.Root, []string{python, "-c", pythonImportGraph, list}, nil)
	if !run.Passed() || run.Truncated {
		result.Reason = "Python 导入分析未完成"
		return result, nil
	}
	var graph struct {
		Files   []pythonImports `json:"files"`
		Version string          `json:"version"`
	}
	if err := json.Unmarshal([]byte(run.Output), &graph); err != nil {
		return result, err
	}
	changes, err := workspace.Diff(ctx, s.OriginTree, tree)
	if err != nil {
		return result, err
	}
	changed := map[string]bool{}
	shared := false
	for _, change := range changes {
		for _, path := range []string{change.Path, change.BeforePath} {
			if path == "" {
				continue
			}
			changed[path] = true
			if !strings.HasSuffix(path, ".py") || filepath.Base(path) == "conftest.py" {
				shared = true
			}
		}
	}
	affected, unknown := affectedPythonTests(graph.Files, changed)
	var selected []string
	for _, path := range affected {
		selected = append(selected, scopeQuote(path))
	}
	total := 0
	for _, file := range graph.Files {
		if pythonTestPath(file.Path) {
			total++
		}
	}
	if len(selected) > 0 && len(selected) < total && !shared && !unknown {
		result.Suggestion = "pytest " + strings.Join(selected, " ")
	}
	result.InputID = fmt.Sprintf("%x", sha256.Sum256([]byte(tree+run.Output)))
	result.ToolID = fmt.Sprintf("%x", sha256.Sum256([]byte(graph.Version)))
	result.Complete = false
	result.Evidence = []string{"按固定根目录及 src/ 的普通导入与相对导入计算直接、间接受影响测试"}
	if unknown {
		result.Evidence = append(result.Evidence, "发现动态导入、通配导入或模块歧义，本次不缩小范围")
	}
	if shared {
		result.Evidence = append(result.Evidence, "共享配置或 conftest.py 变化，本次不缩小范围")
	}
	return result, nil
}

func pythonModule(path string) string {
	module := strings.TrimSuffix(path, ".py")
	module = strings.TrimSuffix(module, "/__init__")
	return strings.ReplaceAll(module, "/", ".")
}

func affectedPythonTests(files []pythonImports, changed map[string]bool) ([]string, bool) {
	modules := map[string][]string{}
	unknown := false
	for _, file := range files {
		for _, path := range []string{file.Path, strings.TrimPrefix(file.Path, "src/")} {
			name := pythonModule(path)
			if !slices.Contains(modules[name], file.Path) {
				modules[name] = append(modules[name], file.Path)
			}
		}
	}
	for _, paths := range modules {
		if len(paths) > 1 {
			unknown = true
		}
	}
	edges := map[string][]string{}
	for _, file := range files {
		unknown = unknown || file.Dynamic
		for _, imp := range file.Imports {
			base := imp.Module
			if imp.Level > 0 {
				owner := strings.Split(pythonModule(strings.TrimPrefix(file.Path, "src/")), ".")
				if !strings.HasSuffix(file.Path, "/__init__.py") {
					owner = owner[:len(owner)-1]
				}
				if imp.Level > len(owner) {
					unknown = true
					continue
				}
				base = strings.Join(owner[:len(owner)-imp.Level+1], ".")
				if imp.Module != "" {
					base += "." + imp.Module
				}
			}
			candidates := []string{base}
			for _, name := range imp.Names {
				candidates = append(candidates, base+"."+name)
			}
			for _, candidate := range candidates {
				parts := strings.Split(candidate, ".")
				for i := 1; i <= len(parts); i++ {
					edges[file.Path] = append(edges[file.Path], modules[strings.Join(parts[:i], ".")]...)
				}
			}
		}
	}
	for progress := true; progress; {
		progress = false
		for path, imports := range edges {
			if changed[path] {
				continue
			}
			for _, dependency := range imports {
				if changed[dependency] {
					changed[path] = true
					progress = true
					break
				}
			}
		}
	}
	var tests []string
	for _, file := range files {
		if changed[file.Path] && pythonTestPath(file.Path) {
			tests = append(tests, file.Path)
		}
	}
	slices.Sort(tests)
	return tests, unknown
}

func pythonTestPath(path string) bool {
	base := filepath.Base(path)
	return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || strings.HasPrefix(path, "tests/") || strings.HasPrefix(path, "test/")
}

func (s Scope) javascriptScope(ctx context.Context, workspace gitstate.Workspace, command shell.Segment, tree string) (VerificationScope, error) {
	result := VerificationScope{Reason: "框架依赖图不证明动态加载和外部环境一致；V1 只提示"}
	tool := shell.Base(command.Args[0])
	if command.Dir != workspace.Root || len(command.Args) > 1 && !(tool == "vitest" && len(command.Args) == 2 && command.Args[1] == "run") {
		return result, nil
	}
	changes, err := workspace.Diff(ctx, s.OriginTree, tree)
	if err != nil {
		return result, err
	}
	var changed []string
	for _, change := range changes {
		for _, path := range []string{change.Path, change.BeforePath} {
			if path == "" {
				continue
			}
			extension := filepath.Ext(path)
			name := filepath.Base(path)
			if strings.Contains(name, ".config.") || strings.HasPrefix(name, "jest.setup.") || strings.HasPrefix(name, "setupTests.") {
				result.Evidence = append(result.Evidence, "共享配置变化，本次不缩小范围")
				return result, nil
			}
			if !slices.Contains([]string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts"}, extension) {
				result.Evidence = append(result.Evidence, "共享配置变化，本次不缩小范围")
				return result, nil
			}
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	changed = slices.Compact(changed)
	if len(changed) == 0 {
		return result, nil
	}
	executor, cleanup, err := scopeCopy(ctx, workspace, tree)
	if err != nil {
		return result, err
	}
	defer cleanup()
	request := Request{Config: map[string]Config{"javascript": {Command: tool}}}
	for key, value := range command.Environment {
		executor.Env = mergeEnvironment(executor.Env, []string{key + "=" + value})
	}
	runner := NewJavaScriptRunner(executor, request)
	runner.related = changed
	defer runner.Terminate()
	if prepared := runner.prepare(ctx, false); !prepared.Passed() {
		result.Reason = "框架范围分析准备未完成: " + prepared.Reason
		return result, nil
	}
	var args []string
	path := filepath.Join(executor.Output, "related.json")
	if tool == "jest" {
		args = append(args, "--listTests", "--json", "--runInBand", "--cacheDirectory="+filepath.Join(executor.Output, "cache"), "--findRelatedTests")
		args = append(args, changed...)
	} else {
		args = append(args, "list", "--filesOnly", "--json="+path)
		args = append(args, runner.command.options...)
	}
	run := executor.Execute(ctx, "scope", executor.Root, runner.command.args(args...), runner.command.env)
	if !run.Passed() || run.Truncated {
		result.Reason = "框架受影响范围未完整返回"
		return result, nil
	}
	var native []string
	if tool == "jest" {
		err = json.Unmarshal([]byte(run.Output), &native)
	} else {
		// Vitest 的 filesOnly JSON 在不同版本可能返回路径或带 testFile 的记录。
		err = readJSON(path, &native)
		if err != nil {
			var entries []struct {
				File     string `json:"file"`
				TestFile string `json:"testFile"`
			}
			err = readJSON(path, &entries)
			if err == nil {
				for _, entry := range entries {
					if entry.File != "" {
						native = append(native, entry.File)
					} else {
						native = append(native, entry.TestFile)
					}
				}
			}
		}
	}
	if err != nil {
		result.Reason = "框架范围报告格式无法核对"
		return result, nil
	}
	var paths []string
	for _, name := range native {
		relative, inside := copyFile(executor.Root, name)
		if !inside {
			return result, nil
		}
		paths = append(paths, relative)
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	for _, path := range paths {
		result.Evidence = append(result.Evidence, "受影响测试: "+path)
	}
	var quoted []string
	for _, path := range changed {
		quoted = append(quoted, scopeQuote(path))
	}
	if len(paths) > 0 {
		if tool == "jest" {
			result.Suggestion = "jest --findRelatedTests " + strings.Join(quoted, " ")
		} else {
			result.Suggestion = "vitest related --run " + strings.Join(quoted, " ")
		}
	}
	identity, _ := json.Marshal(struct {
		Tree  string
		Tests []string
	}{tree, paths})
	result.InputID = fmt.Sprintf("%x", sha256.Sum256(identity))
	// 第三方配置、插件、动态依赖及共享状态未获证明，不能据此拦截重跑。
	result.Complete = false
	result.EnvironmentKnown = false
	return result, nil
}
