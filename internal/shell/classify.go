package shell

import "strings"

func Classify(s Segment) (Kind, string) {
	if s.Kind == Context {
		return Context, ""
	}
	a := s.Args
	if len(a) == 0 {
		return Other, ""
	}
	name := Base(a[0])
	verb := ""
	if len(a) > 1 {
		verb = a[1]
	}
	has := func(values ...string) bool {
		for _, arg := range a[1:] {
			for _, value := range values {
				if arg == value {
					return true
				}
			}
		}
		return false
	}
	switch name {
	case "go":
		if oneOf(verb, "test", "build", "vet") {
			return Verification, "go"
		}
	case "golangci-lint", "staticcheck":
		return Verification, "go"
	case "npm", "pnpm", "yarn", "bun":
		if verb == "run" && len(a) > 2 {
			verb = a[2]
		}
		if oneOf(verb, "test", "build", "lint", "typecheck") {
			return Verification, "javascript"
		}
	case "tsc", "jest", "vitest", "eslint":
		return Verification, "javascript"
	case "deno":
		if verb == "test" {
			return Verification, "javascript"
		}
	case "pytest", "unittest", "tox", "nox", "mypy", "pyright":
		return Verification, "python"
	case "ruff":
		if verb == "check" {
			return Verification, "python"
		}
	case "cargo":
		args := cargoArguments(a)
		if len(args) > 0 && (oneOf(args[0], "test", "build", "check", "clippy") || args[0] == "nextest" && len(args) > 1 && args[1] == "run") {
			return Verification, "rust"
		}
	case "mvn", "mvnw", "gradle", "gradlew":
		if has("test", "verify", "package", "install", "compile", "build", "check", "assemble") {
			return Verification, "java"
		}
	case "dotnet", "swift":
		if oneOf(verb, "test", "build") {
			return Verification, ""
		}
	case "rspec", "phpunit", "ctest":
		return Verification, ""
	case "rake", "rails":
		if verb == "test" {
			return Verification, ""
		}
	case "cmake":
		if has("--build") {
			return Verification, ""
		}
	case "make":
		if has("test", "check", "lint", "build", "ci") {
			return Verification, ""
		}
	}
	if !s.Known || s.SideEffect {
		return Other, ""
	}
	switch name {
	case "pwd", "get-location", "cat", "get-content", "head", "tail", "less", "wc", "ls", "get-childitem", "get-item", "stat", "which", "where", "whereis":
		return ReadOnly, ""
	case "grep", "rg":
		writes := has("--pre")
		for _, arg := range a {
			writes = writes || strings.HasPrefix(arg, "--pre=")
		}
		if !writes {
			return ReadOnly, ""
		}
	case "sed":
		for _, arg := range a[1:] {
			if strings.HasPrefix(arg, "-i") || strings.HasPrefix(arg, "--in-place") || strings.ContainsAny(arg, "we") {
				return Other, ""
			}
		}
		return ReadOnly, ""
	case "find":
		if !has("-delete", "-exec", "-execdir", "-ok", "-okdir", "-fprint", "-fprint0", "-fprintf", "-fls") {
			return ReadOnly, ""
		}
	case "git":
		if oneOf(verb, "status", "diff", "log", "show", "rev-parse", "ls-files", "ls-tree", "cat-file") && !has("--output", "--ext-diff", "--textconv") {
			for _, arg := range a {
				if strings.HasPrefix(arg, "--output=") {
					return Other, ""
				}
			}
			return ReadOnly, ""
		}
	}
	return Other, ""
}

/**
 * cargoArguments 只为分类读取子命令，执行身份仍保留原参数中的工具链选择。
 */
func cargoArguments(args []string) []string {
	args = args[1:]
	if len(args) > 0 && len(args[0]) > 1 && strings.HasPrefix(args[0], "+") {
		return args[1:]
	}
	return args
}

func oneOf(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func ReusableCommand(s Segment) bool {
	if s.Kind != Verification || !s.Known || s.SideEffect || s.Background {
		return false
	}
	for i, part := range s.Prefix {
		name := Base(part)
		if name == "bunx" {
			return false
		}
		if name == "npx" && (i+1 >= len(s.Prefix) || s.Prefix[i+1] != "--no-install") {
			return false
		}
		if name == "uv" {
			return false
		}
	}
	for _, arg := range s.Args[1:] {
		key, _, _ := strings.Cut(arg, "=")
		if oneOf(key, "--fix", "--fix-only", "--fix-dry-run", "--update", "--updateSnapshot", "-u", "--write", "--watch", "--watchAll", "-w", "--output", "-o", "--emit", "--coverage", "--cache", "-coverprofile", "-cpuprofile", "-memprofile", "-trace", "-c", "--outDir") {
			return false
		}
	}
	if len(s.Args) > 0 && Base(s.Args[0]) == "tsc" {
		noEmit := false
		for _, arg := range s.Args[1:] {
			noEmit = noEmit || arg == "--noEmit"
		}
		if !noEmit {
			return false
		}
	}
	// 构建、安装和任意项目脚本通常有产物；Runner 也不能把它们变成无副作用命令。
	if len(s.Args) > 1 {
		args := s.Args[1:]
		if Base(s.Args[0]) == "cargo" {
			args = cargoArguments(s.Args)
		}
		if len(args) > 0 && oneOf(args[0], "build", "install", "package", "compile", "assemble") {
			return false
		}
	}
	return true
}
