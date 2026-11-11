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
	ap, ao := arguments(a.Args)
	bp, bo := arguments(b.Args)
	if !slices.Equal(ao, bo) {
		return false
	}
	if len(ap) == 0 || len(bp) == 0 {
		// 省略路径时，默认运行范围由程序决定。
		return len(ap) == len(bp)
	}
	for _, target := range bp {
		covered := false
		for _, scope := range ap {
			covered = covered || pathContains(scope, target)
		}
		if !covered {
			return false
		}
	}
	return true
}
func arguments(args []string) ([]string, []string) {
	var paths, other []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || strings.HasPrefix(arg, "/") {
			paths = append(paths, arg)
		} else {
			other = append(other, arg)
		}
	}
	return paths, other
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
