package testrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func (s Scope) rustScope(ctx context.Context, workspace gitstate.Workspace, command shell.Segment, tree string) (VerificationScope, error) {
	result := VerificationScope{Reason: "Cargo workspace 依赖不包含构建脚本、过程宏和外部状态；V1 只提示"}
	if command.Dir != workspace.Root || len(command.Args) != 2 || command.Args[1] != "test" {
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
	executor.Env = mergeEnvironment(executor.Env, []string{"CARGO_TARGET_DIR=" + executor.Output, "CARGO_NET_OFFLINE=true", "RUSTUP_AUTO_INSTALL=0"})
	listed := executor.Execute(ctx, "scope", executor.Root, []string{command.Args[0], "metadata", "--format-version=1", "--offline"}, nil)
	var metadata cargoMetadata
	if !listed.Passed() || listed.Truncated || json.Unmarshal([]byte(listed.Output), &metadata) != nil {
		result.Reason = "Cargo 元数据没有完整返回 workspace 依赖"
		return result, nil
	}
	type member struct{ name, dir string }
	members := map[string]member{}
	for _, pkg := range metadata.Packages {
		if !slices.Contains(metadata.Members, pkg.ID) {
			continue
		}
		manifest, inside := copyFile(executor.Root, pkg.Manifest)
		if !inside {
			result.Reason = "workspace crate 清单位于固定副本之外"
			return result, nil
		}
		members[pkg.ID] = member{pkg.Name, filepath.ToSlash(filepath.Dir(manifest))}
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
			if !strings.HasSuffix(path, ".rs") || filepath.Base(path) == "build.rs" {
				result.Reason = "清单、构建脚本或共享配置变化，保留原 Cargo 验证范围"
				return result, nil
			}
			owner, length := "", -1
			for id, m := range members {
				if (m.dir == "." || strings.HasPrefix(path, m.dir+"/")) && len(m.dir) > length {
					owner, length = id, len(m.dir)
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
		for _, node := range metadata.Resolve.Nodes {
			if _, ok := members[node.ID]; !ok || affected[node.ID] {
				continue
			}
			for _, dependency := range node.Dependencies {
				if affected[dependency] {
					affected[node.ID], changed = true, true
					break
				}
			}
		}
	}
	var names []string
	for id := range affected {
		names = append(names, members[id].name)
	}
	slices.Sort(names)
	result.InputID = fmt.Sprintf("%x", sha256.Sum256([]byte(tree+"\n"+listed.Output)))
	version := executor.Execute(ctx, "scope", executor.Root, []string{command.Args[0], "--version"}, nil)
	if version.Passed() {
		result.ToolID = strings.TrimSpace(version.Output)
	}
	if len(names) > 0 && len(names) < len(members) {
		result.Suggestion = command.Args[0] + " test"
		for _, name := range names {
			result.Suggestion += " -p " + scopeQuote(name)
		}
		result.Evidence = append(result.Evidence, "Cargo 元数据的反向 workspace 依赖: "+strings.Join(names, ", "))
	}
	return result, nil
}
