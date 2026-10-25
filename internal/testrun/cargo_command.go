package testrun

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

type cargoCommand struct {
	prefix, options, metadata, env []string
	toolchain, filter              string
	exact                          bool
}

func parseCargoCommand(text, root string) (cargoCommand, error) {
	c := cargoCommand{prefix: []string{"cargo"}}
	if strings.TrimSpace(text) == "" {
		return c, nil
	}
	var parser shell.Parser = posix.Parser{}
	if runtime.GOOS == "windows" {
		parser = pwsh.Parser{}
	}
	script := parser.Parse(text, root)
	if script.Complex || len(script.Segments) != 1 {
		return c, fmt.Errorf("Cargo 命令需为一个可核对的字面命令或前缀")
	}
	s := script.Segments[0]
	if !s.Known || s.Background || s.SideEffect || s.Dir != root || len(s.Args) == 0 {
		return c, fmt.Errorf("Cargo 命令的目录、参数或输出无法核对")
	}
	for key, value := range s.Environment {
		c.env = append(c.env, key+"="+value)
	}
	index := -1
	for i, arg := range s.Args {
		if shell.Base(arg) == "cargo" {
			index = i
			break
		}
	}
	if index < 0 {
		if err := checkOutputEnvironmentPrefix(s.Args); err != nil {
			return c, err
		}
		c.prefix = append(slices.Clone(s.Args), "cargo")
		c.prefix, c.env = prefixEnvironment(c.prefix, c.env)
		return c, nil
	}
	if err := checkOutputEnvironmentPrefix(s.Args[:index]); err != nil {
		return c, err
	}
	c.prefix = slices.Clone(s.Args[:index+1])
	c.prefix, c.env = prefixEnvironment(c.prefix, c.env)
	args := s.Args[index+1:]
	if len(args) > 0 && strings.HasPrefix(args[0], "+") {
		c.toolchain = args[0]
		args = args[1:]
	}
	if len(args) == 0 {
		return c, nil
	}
	if args[0] != "test" && args[0] != "llvm-cov" {
		return c, fmt.Errorf("Cargo 命令没有采用 test 或 llvm-cov")
	}
	args = args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			continue
		}
		if arg == "--exact" {
			c.exact = true
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			if c.filter != "" {
				return c, fmt.Errorf("Cargo 用例选择不唯一")
			}
			c.filter = arg
			continue
		}
		key, value, has := strings.Cut(arg, "=")
		take := func() bool {
			if has {
				return true
			}
			i++
			if i >= len(args) {
				return false
			}
			value = args[i]
			return true
		}
		switch key {
		case "-p", "--package", "--test", "--bin", "--features", "--profile", "--target", "--manifest-path", "--exclude":
			if !take() {
				return c, fmt.Errorf("Cargo 参数 %s 缺少值", key)
			}
			if key == "--manifest-path" && !filepath.IsLocal(filepath.FromSlash(value)) {
				return c, fmt.Errorf("Cargo 清单必须位于固定副本内")
			}
			c.options = append(c.options, key, value)
			if key == "--features" || key == "--manifest-path" {
				c.metadata = append(c.metadata, key, value)
			}
		case "--lib", "--tests", "--bins", "--workspace", "--release", "--all-features", "--no-default-features", "--locked", "--offline", "--frozen":
			c.options = append(c.options, arg)
			if key == "--all-features" || key == "--no-default-features" || key == "--locked" || key == "--offline" || key == "--frozen" {
				c.metadata = append(c.metadata, arg)
			}
		case "-q", "--quiet", "-v", "--verbose":
		default:
			return c, fmt.Errorf("Cargo 参数 %s 的执行范围尚不能核对", key)
		}
	}
	return c, nil
}

func (c cargoCommand) args(action string, args ...string) []string {
	result := slices.Clone(c.prefix)
	if c.toolchain != "" {
		result = append(result, c.toolchain)
	}
	result = append(result, action)
	return append(result, args...)
}

func (c cargoCommand) rustc(args ...string) []string {
	result := slices.Clone(c.prefix)
	program := result[len(result)-1]
	if strings.ContainsAny(program, `/\`) {
		result[len(result)-1] = filepath.Join(filepath.Dir(program), "rustc")
		if runtime.GOOS == "windows" {
			result[len(result)-1] += ".exe"
		}
	} else {
		result[len(result)-1] = "rustc"
	}
	if c.toolchain != "" {
		result = append(result, c.toolchain)
	}
	return append(result, args...)
}

func (c cargoCommand) matches(name string) bool {
	if c.filter == "" {
		return true
	}
	if c.exact {
		return name == c.filter
	}
	return strings.Contains(name, c.filter)
}
