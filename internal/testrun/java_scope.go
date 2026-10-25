package testrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func (s Scope) mavenScope(ctx context.Context, workspace gitstate.Workspace, command shell.Segment, tree string) (VerificationScope, error) {
	result := VerificationScope{Reason: "Maven 模块依赖不包含运行时加载与外部环境；V1 只提示"}
	if command.Dir != workspace.Root || len(command.Args) != 2 || command.Args[1] != "test" {
		return result, nil
	}
	executor, cleanup, err := scopeCopy(ctx, workspace, tree)
	if err != nil {
		return result, err
	}
	defer cleanup()
	root, err := filepath.EvalSymlinks(executor.Root)
	if err != nil {
		return result, err
	}
	executor.Root = root
	for key, value := range command.Environment {
		executor.Env = mergeEnvironment(executor.Env, []string{key + "=" + value})
	}
	parsed, err := parseJavaCommand(scopeQuote(command.Args[0])+" test", executor.Root)
	if err != nil {
		result.Reason = err.Error()
		return result, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return result, err
	}
	if err := parsed.prepareWrapper(ctx, executor, []string{filepath.Join(home, ".m2")}, filepath.Join(executor.Output, "maven-home")); err != nil {
		result.Reason = err.Error()
		return result, nil
	}
	models, run := mavenModels(ctx, executor, parsed, []string{filepath.Join(home, ".m2", "repository")})
	if !run.Passed() {
		result.Reason = "Maven 有效模块和依赖未完整返回"
		return result, nil
	}
	paths := map[string]string{}
	for _, model := range models {
		id := model.Group + ":" + model.Artifact
		if _, exists := paths[id]; exists {
			return result, nil
		}
		dir := model.Build.Source
		for {
			if _, err := os.Stat(filepath.Join(dir, "pom.xml")); err == nil {
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir || !strings.HasPrefix(parent, executor.Root) {
				return result, nil
			}
			dir = parent
		}
		rel, err := filepath.Rel(executor.Root, dir)
		if err != nil || rel != "." && !filepath.IsLocal(rel) {
			return result, nil
		}
		paths[id] = filepath.ToSlash(rel)
	}
	changes, err := workspace.Diff(ctx, s.OriginTree, tree)
	if err != nil {
		return result, err
	}
	affected := map[string]bool{}
	for _, change := range changes {
		for _, path := range []string{change.Path, change.BeforePath} {
			if path == "" {
				continue
			}
			if !strings.HasSuffix(path, ".java") {
				result.Reason = "Maven 清单、构建或共享配置变化，保留原验证范围"
				return result, nil
			}
			owner, length := "", -1
			for id, dir := range paths {
				if (dir == "." || strings.HasPrefix(path, dir+"/")) && len(dir) > length {
					owner, length = id, len(dir)
				}
			}
			if owner == "" {
				return result, nil
			}
			affected[owner] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, model := range models {
			id := model.Group + ":" + model.Artifact
			if affected[id] {
				continue
			}
			for _, dependency := range model.Dependencies {
				if affected[dependency.Group+":"+dependency.Artifact] {
					affected[id], changed = true, true
					break
				}
			}
		}
	}
	var selected []string
	for id := range affected {
		selected = append(selected, paths[id])
	}
	slices.Sort(selected)
	if len(selected) > 0 && len(selected) < len(paths) {
		result.Suggestion = command.Args[0] + " test -pl " + scopeQuote(strings.Join(selected, ",")) + " -amd"
		result.Evidence = []string{"有效 Maven 模型的反向模块依赖: " + strings.Join(selected, ", ")}
	}
	data, _ := json.Marshal(models)
	result.InputID = fmt.Sprintf("%x", sha256.Sum256(append([]byte(tree), data...)))
	version := executor.Execute(ctx, "scope", executor.Root, parsed.args("--version"), nil)
	if version.Passed() {
		result.ToolID = fmt.Sprintf("%x", sha256.Sum256([]byte(version.Output)))
	}
	return result, nil
}
