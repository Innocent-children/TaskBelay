package testrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

func linkDependency(executor *Executor, name string) error {
	if executor.Workspace == "" {
		return nil
	}
	source := filepath.Join(executor.Workspace, name)
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("依赖位置不是目录: %s", name)
	}
	target := filepath.Join(executor.Root, name)
	if linked, err := os.Readlink(target); err == nil && linked == source {
		return nil
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return fmt.Errorf("副本已有 %s，不能覆盖为依赖链接", name)
	}
	return os.Symlink(source, target)
}

func checkCopy(executor *Executor, snapshot codescan.Snapshot, languages ...string) error {
	for path, file := range snapshot.Files {
		if !slices.Contains(languages, file.Kind.Language) {
			continue
		}
		actual, err := os.ReadFile(filepath.Join(executor.Root, filepath.FromSlash(path)))
		if err != nil || !bytes.Equal(actual, file.Data) {
			return fmt.Errorf("准备后源码与固定快照不同: %s", path)
		}
	}
	return nil
}

// 解析真实路径，避免依赖链接、editable 安装或 source map 将外部源码冒充副本。
func copyFile(root, name string) (string, bool) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(root, filepath.FromSlash(name))
	}
	actual, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, actual)
	return filepath.ToSlash(rel), err == nil && filepath.IsLocal(rel)
}

func readJSON(path string, out any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > outputLimit {
		return fmt.Errorf("框架报告超出输出预算")
	}
	return json.NewDecoder(file).Decode(out)
}

func wholeCase(cases []Case, name string, expected []string, reason string) Case {
	whole := cases[0]
	whole.Name, whole.Whole, whole.Expected = name, true, append([]string(nil), expected...)
	whole.At = "test:" + whole.Source.File + ":" + name
	whole.Unknown = append(slices.Clone(whole.Unknown), reason)
	for _, test := range cases {
		whole.AgentOnly = whole.AgentOnly && test.AgentOnly
		whole.Mixed = whole.Mixed || test.Mixed
		whole.Retained = whole.Retained || test.Retained
		whole.Requirements = append(whole.Requirements, test.Requirements...)
		whole.Assertions = append(whole.Assertions, test.Assertions...)
		if test.Change != "add" {
			whole.Change = test.Change
		}
	}
	slices.Sort(whole.Requirements)
	whole.Requirements = slices.Compact(whole.Requirements)
	slices.Sort(whole.Assertions)
	whole.Assertions = slices.Compact(whole.Assertions)
	return whole
}

func sameNames(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return len(a) > 0 && slices.Equal(a, b)
}

func lineCount(data []byte) int { return bytes.Count(data, []byte("\n")) + 1 }

func normalizedLines(lines []Line) []Line {
	slices.SortFunc(lines, func(a, b Line) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return a.Number - b.Number
	})
	return slices.Compact(lines)
}
