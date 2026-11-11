package validation

import (
	"slices"
	"strings"
)

func Contains(a, b Command) bool {
	// 组合命令的路径分属不同命令段，放在一起比较会误判覆盖范围。
	if a.Mixed || b.Mixed || a.Program != b.Program || a.Directory != b.Directory || a.Shell != b.Shell {
		return false
	}
	scopes, targets := a.Args, b.Args
	for {
		// 非路径参数隔开的路径可能用途不同，例如测试包和传给测试的数据文件。
		scopePaths, scopeRest := cutPaths(scopes)
		targetPaths, targetRest := cutPaths(targets)
		if (len(scopePaths) == 0) != (len(targetPaths) == 0) {
			return false
		}
		for _, target := range targetPaths {
			if !slices.ContainsFunc(scopePaths, func(scope string) bool { return pathContains(scope, target) }) {
				return false
			}
		}
		if len(scopeRest) == 0 || len(targetRest) == 0 {
			return len(scopeRest) == len(targetRest)
		}
		if scopeRest[0] != targetRest[0] {
			return false
		}
		scopes, targets = scopeRest[1:], targetRest[1:]
	}
}

func cutPaths(args []string) (paths, rest []string) {
	for i, arg := range args {
		if !strings.HasPrefix(arg, "./") && !strings.HasPrefix(arg, "../") && !strings.HasPrefix(arg, "/") {
			return args[:i], args[i:]
		}
	}
	return args, nil
}

func pathContains(scope, target string) bool {
	if scope == target {
		return true
	}
	if !strings.HasSuffix(scope, "/...") {
		return false
	}
	base, parts := pathComponents(scope), pathComponents(target)
	return len(parts) >= len(base) && slices.Equal(base, parts[:len(base)]) && !slices.Contains(parts[len(base):], "..")
}

func pathComponents(name string) []string {
	if strings.HasSuffix(name, "/...") {
		name = strings.TrimSuffix(name, "...")
	}
	rest := strings.TrimLeft(name, "/")
	// 首项保留路径开头的斜杠，区分相对路径、绝对路径和双斜杠起点。
	parts := []string{strings.TrimSuffix(name, rest)}
	for part := range strings.SplitSeq(rest, "/") {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return parts
}
