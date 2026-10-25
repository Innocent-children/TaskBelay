package testrun

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/shell/posix"
	"github.com/Innocent-children/taskbelay/internal/shell/pwsh"
)

type javaCommand struct {
	prefix, options, env []string
	tool, filter         string
}

func prefixEnvironment(prefix, env []string) ([]string, []string) {
	result := slices.Clone(prefix)
	if len(result) == 0 || shell.Base(result[0]) != "env" {
		return result, env
	}
	i := 1
	if i < len(result) && result[i] == "--" {
		i++
	}
	for i < len(result) {
		key, value, ok := shell.Assignment(result[i])
		if !ok {
			break
		}
		env = mergeEnvironment(env, []string{key + "=" + value})
		result = append(result[:i], result[i+1:]...)
	}
	return result, env
}

func javaTool(name string) string {
	switch strings.TrimSuffix(strings.TrimSuffix(shell.Base(name), ".cmd"), ".bat") {
	case "mvn", "mvnw":
		return "maven"
	case "gradle", "gradlew":
		return "gradle"
	}
	return ""
}

func parseJavaCommand(text, root string) (javaCommand, error) {
	c := javaCommand{}
	var parser shell.Parser = posix.Parser{}
	if runtime.GOOS == "windows" {
		parser = pwsh.Parser{}
	}
	var args []string
	if strings.TrimSpace(text) != "" {
		script := parser.Parse(text, root)
		if script.Complex || len(script.Segments) != 1 {
			return c, fmt.Errorf("Java 命令需为一个可核对的字面命令或前缀")
		}
		s := script.Segments[0]
		if !s.Known || s.Background || s.SideEffect || s.Dir != root {
			return c, fmt.Errorf("Java 命令目录或输出无法核对")
		}
		args = s.Args
		for key, value := range s.Environment {
			c.env = append(c.env, key+"="+value)
		}
	}
	index := -1
	for i, arg := range args {
		if tool := javaTool(arg); tool != "" {
			c.tool, index = tool, i
			break
		}
	}
	if index >= 0 {
		c.prefix, args = slices.Clone(args[:index+1]), args[index+1:]
	} else {
		c.prefix, args = slices.Clone(args), nil
	}
	if c.tool == "" {
		_, maven := os.Stat(filepath.Join(root, "pom.xml"))
		_, groovy := os.Stat(filepath.Join(root, "build.gradle"))
		_, kotlin := os.Stat(filepath.Join(root, "build.gradle.kts"))
		if maven == nil && (groovy == nil || kotlin == nil) {
			return c, fmt.Errorf("同时存在 Maven 与 Gradle，请使用已有配置明确选择")
		}
		if maven == nil {
			c.tool = "maven"
		} else if groovy == nil || kotlin == nil {
			c.tool = "gradle"
		} else {
			return c, fmt.Errorf("未找到 Java 构建清单")
		}
		program := "mvn"
		if c.tool == "gradle" {
			program = "gradle"
		}
		c.prefix = append(c.prefix, program)
	}
	if err := checkOutputEnvironmentPrefix(c.prefix[:len(c.prefix)-1]); err != nil {
		return c, err
	}
	c.prefix, c.env = prefixEnvironment(c.prefix, c.env)
	wrapper := "mvnw"
	if c.tool == "gradle" {
		wrapper = "gradlew"
	}
	if runtime.GOOS == "windows" {
		if c.tool == "maven" {
			wrapper += ".cmd"
		} else {
			wrapper += ".bat"
		}
	}
	if info, err := os.Stat(filepath.Join(root, wrapper)); err == nil && !info.IsDir() {
		c.prefix[len(c.prefix)-1] = filepath.Join(root, wrapper)
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "test" || arg == "-q" || arg == "--quiet" || arg == "--offline" || arg == "-o" || arg == "--no-daemon" {
			continue
		}
		key, value, has := strings.Cut(arg, "=")
		if strings.HasPrefix(arg, "-Dtest=") || key == "--tests" {
			if !has {
				i++
				if i == len(args) {
					return c, fmt.Errorf("Java 用例选择缺少值")
				}
				value = args[i]
			}
			if c.filter != "" {
				return c, fmt.Errorf("Java 用例选择不唯一")
			}
			c.filter = value
			continue
		}
		if strings.HasPrefix(arg, "-D") || c.tool == "gradle" && strings.HasPrefix(arg, "-P") {
			c.options = append(c.options, arg)
			continue
		}
		if c.tool == "maven" && (arg == "-s" || arg == "--settings" || arg == "-P" || arg == "--activate-profiles") {
			i++
			if i == len(args) {
				return c, fmt.Errorf("Java 参数缺少值: %s", arg)
			}
			c.options = append(c.options, arg, args[i])
			continue
		}
		return c, fmt.Errorf("Java 参数 %s 的实际范围尚未核对", arg)
	}
	return c, nil
}

func (c javaCommand) args(args ...string) []string {
	result := append(slices.Clone(c.prefix), c.options...)
	if c.tool == "maven" {
		result = append(result, "--offline", "--batch-mode", "-Dstyle.color=never")
	} else {
		result = append(result, "--offline", "--no-daemon", "--no-configuration-cache", "--console=plain")
	}
	return append(result, args...)
}
