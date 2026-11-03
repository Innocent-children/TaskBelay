package validation

import (
	"path"
	"slices"
	"strings"
)

func Contains(a, b Command) bool {
	if a.Program != b.Program || a.Directory != b.Directory || a.Shell != b.Shell {
		return false
	}
	ap, ao := arguments(a.Args)
	bp, bo := arguments(b.Args)
	if !slices.Equal(ao, bo) {
		return false
	}
	if len(ap) == 0 {
		return true
	}
	if len(bp) == 0 {
		return false
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
	if strings.HasSuffix(scope, "/...") {
		prefix := path.Clean(strings.TrimSuffix(scope, "/..."))
		target = path.Clean(strings.TrimSuffix(target, "/..."))
		return prefix == "." || target == prefix || strings.HasPrefix(target, prefix+"/")
	}
	return false
}
