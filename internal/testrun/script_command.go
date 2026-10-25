package testrun

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

type scriptCommand struct {
	prefix, env, options, selectors []string
}

func parseScriptCommand(text, root, tool string, program []string) (scriptCommand, error) {
	c := scriptCommand{prefix: append([]string(nil), program...)}
	if strings.TrimSpace(text) == "" {
		return c, nil
	}
	var parser shell.Parser = posix.Parser{}
	if runtime.GOOS == "windows" {
		parser = pwsh.Parser{}
	}
	s := parser.Parse(text, root)
	if s.Complex || len(s.Segments) != 1 {
		return c, fmt.Errorf("测试命令需能核对为一个字面命令或运行前缀")
	}
	segment := s.Segments[0]
	if !segment.Known || segment.Background || segment.SideEffect || segment.Dir != root || len(segment.Args) == 0 {
		return c, fmt.Errorf("测试命令的参数、目录或输出无法核对")
	}
	for key, value := range segment.Environment {
		c.env = append(c.env, key+"="+value)
	}
	if err := checkOutputEnvironmentPrefix(segment.Args); err != nil {
		return c, err
	}
	segment.Args, c.env = prefixEnvironment(segment.Args, c.env)
	index := -1
	for i, arg := range segment.Args {
		if shell.Base(arg) == tool {
			index = i
			break
		}
	}
	if index < 0 {
		if err := checkOutputEnvironmentPrefix(segment.Args); err != nil {
			return c, err
		}
		c.prefix = append(append([]string(nil), segment.Args...), program...)
		return c, nil
	}
	before := append([]string(nil), segment.Args[:index]...)
	if err := checkOutputEnvironmentPrefix(before); err != nil {
		return c, err
	}
	if tool == "pytest" && len(before) >= 2 && before[len(before)-1] == "-m" && strings.HasPrefix(shell.Base(before[len(before)-2]), "python") {
		c.prefix = append([]string(nil), before[:len(before)-1]...)
	} else {
		c.prefix = append(before, program...)
	}
	args := segment.Args[index+1:]
	if tool == "vitest" && len(args) > 0 && args[0] == "run" {
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			path, _, _ := strings.Cut(arg, "::")
			if !filepath.IsLocal(filepath.FromSlash(path)) && path != "." {
				return c, fmt.Errorf("测试选择必须位于副本内: %s", arg)
			}
			c.selectors = append(c.selectors, arg)
			continue
		}
		key, value, has := strings.Cut(arg, "=")
		allowed := false
		if tool == "pytest" {
			allowed = key == "-k" || key == "-m" || key == "--tb"
		}
		if tool != "pytest" {
			allowed = key == "--config" || key == "--testNamePattern" || key == "-t"
		}
		if allowed {
			if !has {
				i++
				if i >= len(args) {
					return c, fmt.Errorf("%s 缺少值", key)
				}
				value = args[i]
			}
			if key == "--config" && !filepath.IsLocal(filepath.FromSlash(value)) {
				return c, fmt.Errorf("配置文件必须位于副本内")
			}
			c.options = append(c.options, key, value)
			continue
		}
		if arg == "-q" || arg == "-v" || arg == "--runInBand" || arg == "--run" {
			continue
		}
		return c, fmt.Errorf("%s 参数 %s 的运行范围尚未核对", tool, key)
	}
	return c, nil
}

func checkOutputEnvironmentPrefix(args []string) error {
	for i, arg := range args {
		if shell.Base(arg) != "env" {
			continue
		}
		for _, option := range args[i+1:] {
			if option == "--" {
				break
			}
			if _, _, assignment := shell.Assignment(option); assignment {
				continue
			}
			if strings.HasPrefix(option, "-") {
				return fmt.Errorf("清空或撤销环境变量的命令前缀尚不能核对临时输出约束")
			}
			break
		}
	}
	return nil
}

func (c scriptCommand) args(args ...string) []string {
	return append(append([]string(nil), c.prefix...), args...)
}
