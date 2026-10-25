package testrun

import (
	"runtime"
	"slices"
	"testing"
)

func TestRunnerCommandsPreserveParsedExecutionPrefixes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("这些输入是 POSIX 命令；PowerShell 原生前缀另行验证")
	}
	for _, item := range []struct {
		command, tool string
		want          []string
	}{
		{"uv run pytest -q", "pytest", []string{"uv", "run", "python-picked"}},
		{"poetry run python3.14 -m pytest", "pytest", []string{"poetry", "run", "python3.14"}},
		{"env TASKBELAY_SAMPLE=present pytest", "pytest", []string{"env", "python-picked"}},
		{"pnpm exec vitest run", "vitest", []string{"pnpm", "exec", "python-picked"}},
	} {
		command, err := parseScriptCommand(item.command, "/tmp/copy", item.tool, []string{"python-picked"})
		if err != nil || !slices.Equal(command.prefix, item.want) {
			t.Fatal(item.command, command.prefix, err)
		}
		if item.want[0] == "env" && !slices.Contains(command.env, "TASKBELAY_SAMPLE=present") {
			t.Fatal("lost explicit environment", command.env)
		}
	}
	goCommand, err := parseGoCommand("env TASKBELAY_SAMPLE=present go test ./...", "/tmp/copy")
	if err != nil || !slices.Equal(goCommand.prefix, []string{"env", "go"}) || !slices.Contains(goCommand.env, "TASKBELAY_SAMPLE=present") {
		t.Fatal(goCommand, err)
	}
	if _, err := parseScriptCommand("env -i pytest", "/tmp/copy", "pytest", []string{"python-picked"}); err == nil {
		t.Fatal("environment reset silently ignored")
	}
}
