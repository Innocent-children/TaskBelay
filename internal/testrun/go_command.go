package testrun

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

type goCommand struct {
	prefix, build, packages, runtime, env []string
	filter                                string
}

/**
 * 配置命令只拆解可核对的字面参数。保留运行前缀、构建参数及选择范围，
 * 不把未知 shell 程序重写成另一条测试命令。
 */
func parseGoCommand(text, root string) (goCommand, error) {
	result := goCommand{prefix: []string{"go"}, packages: []string{"./..."}}
	if strings.TrimSpace(text) == "" {
		return result, nil
	}
	var parser shell.Parser = posix.Parser{}
	if runtime.GOOS == "windows" {
		parser = pwsh.Parser{}
	}
	script := parser.Parse(text, root)
	if script.Complex || len(script.Segments) != 1 {
		return result, fmt.Errorf("Go 测试命令需能核对为一个字面命令或前缀")
	}
	segment := script.Segments[0]
	if !segment.Known || segment.Background || segment.SideEffect || segment.Dir != root || len(segment.Args) == 0 {
		return result, fmt.Errorf("Go 测试命令的目录、参数或重定向无法核对")
	}
	for key, value := range segment.Environment {
		result.env = append(result.env, key+"="+value)
	}
	if err := checkOutputEnvironmentPrefix(segment.Args); err != nil {
		return result, err
	}
	segment.Args, result.env = prefixEnvironment(segment.Args, result.env)
	index := -1
	for i, arg := range segment.Args {
		if shell.Base(arg) == "go" {
			index = i
			break
		}
	}
	if index < 0 {
		result.prefix = append(append([]string(nil), segment.Args...), "go")
		return result, nil
	}
	result.prefix = append([]string(nil), segment.Args[:index+1]...)
	args := segment.Args[index+1:]
	if len(args) == 0 {
		return result, nil
	}
	if args[0] != "test" {
		return result, fmt.Errorf("Go 测试命令没有使用 go test")
	}
	var packages []string
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			packages = append(packages, arg)
			continue
		}
		key, value, has := strings.Cut(arg, "=")
		take := func() bool {
			if has {
				return true
			}
			if i+1 >= len(args) {
				return false
			}
			i++
			value = args[i]
			return true
		}
		switch key {
		case "-tags", "-mod", "-modfile", "-gcflags", "-asmflags", "-ldflags", "-p":
			if !take() {
				return result, fmt.Errorf("Go 参数 %s 缺少值", key)
			}
			if key == "-modfile" && strings.ContainsAny(value, `/\`) {
				return result, fmt.Errorf("自定义 modfile 路径尚不能核对副本来源")
			}
			result.build = append(result.build, key+"="+value)
		case "-race", "-msan", "-asan", "-trimpath":
			result.build = append(result.build, arg)
		case "-run":
			if !take() {
				return result, fmt.Errorf("-run 缺少值")
			}
			result.filter = value
		case "-short", "-failfast":
			result.runtime = append(result.runtime, "-test."+strings.TrimPrefix(arg, "-"))
		case "-timeout":
			if !take() {
				return result, fmt.Errorf("-timeout 缺少值")
			}
			result.runtime = append(result.runtime, "-test.timeout="+value)
		case "-count":
			if !take() || value != "1" {
				return result, fmt.Errorf("重复执行参数不能表示单条用例")
			}
		case "-v", "-json":
		default:
			return result, fmt.Errorf("Go 参数 %s 的运行范围尚不能核对", key)
		}
	}
	if len(packages) > 0 {
		result.packages = packages
	}
	return result, nil
}

func (g goCommand) args(action string, args ...string) []string {
	return append(append(append([]string(nil), g.prefix...), action), args...)
}
