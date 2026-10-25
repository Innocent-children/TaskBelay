package lang

import (
	"bytes"
	"path"
	"regexp"
	"strings"

	"github.com/odvcencio/gotreesitter/grammars"
)

type FileKind struct {
	Language   string
	Source     bool
	Generated  bool
	Dependency bool
	Lock       bool
	Test       bool
	TestData   bool
	Stub       bool
}

var testSuffix = regexp.MustCompile(`(^|[._-])(test|spec)([._-]|$)`)

func Classify(name string, data []byte) FileKind {
	base := path.Base(name)
	result := FileKind{}
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "vendor", "node_modules", ".venv", "venv":
			result.Dependency = true
		case "test", "tests", "__tests__", "spec":
			result.Test = true
		case "testdata":
			result.TestData = true
		}
	}
	result.Lock = IsLock(base)
	result.Stub = strings.HasSuffix(name, ".pyi") || strings.HasSuffix(name, ".d.ts")
	if entry := grammars.DetectLanguage(base); entry != nil {
		result.Language = entry.Name
		result.Source = !result.Stub && !result.Lock
		switch entry.Name {
		case "json", "json5", "jsonc", "toml", "yaml", "xml", "markdown", "markdown_inline", "rst", "html", "css", "ini", "gomod", "gosum":
			result.Source = false
		}
	}
	if result.Source {
		result.Test = result.Test || testSuffix.MatchString(base)
		if result.Language == "python" {
			result.Test = result.Test || strings.HasPrefix(base, "test_") || base == "conftest.py"
		}
	} else if result.Test {
		result.TestData = true
		result.Test = false
	}
	lines := bytes.SplitN(data, []byte{'\n'}, 6)
	if len(lines) > 5 {
		lines = lines[:5]
	}
	header := strings.ToLower(string(bytes.Join(lines, []byte{'\n'})))
	result.Generated = strings.Contains(header, "@generated") || (strings.Contains(header, "generated") && strings.Contains(header, "do not edit"))
	return result
}

func IsLock(base string) bool {
	switch base {
	case "go.sum", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", "bun.lock", "poetry.lock", "uv.lock", "pdm.lock", "Cargo.lock", "gradle.lockfile", "Gemfile.lock", "composer.lock", "packages.lock.json":
		return true
	}
	return false
}

func Manifest(name string) bool {
	base := path.Base(name)
	switch base {
	case "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts", "Gemfile", "composer.json":
		return true
	}
	return strings.HasSuffix(base, ".csproj") || (strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"))
}
