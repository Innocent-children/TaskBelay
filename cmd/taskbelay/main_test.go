package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

func configCLI(t *testing.T, env platform.Env, dir string, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := run(t.Context(), args, env, dir, &output); err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return output.String()
}

func TestConfigCLIResolvesScopesAndPreservesCommands(t *testing.T) {
	base := t.TempDir()
	env := platform.Env{HomeDir: base, DataDir: filepath.Join(base, ".taskbelay")}
	configCLI(t, env, base, "config", "set", "workflow", "semi")
	if got := configCLI(t, env, base, "config", "get", "workflow"); !strings.Contains(got, "workflow = semi\n来源: 用户\n设置修订:") {
		t.Fatalf("user settings: %s", got)
	}
	repo := filepath.Join(base, "项目 space")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	configCLI(t, env, repo, "config", "set", "workflow", "auto", "--project")
	if got := configCLI(t, env, repo, "config", "get", "workflow"); !strings.Contains(got, "workflow = auto\n来源: 项目 (") {
		t.Fatalf("project settings: %s", got)
	}
	var output bytes.Buffer
	if err := run(t.Context(), []string{"config", "set", "workflow", "semi", "--task"}, env, repo, &output); err == nil || !strings.Contains(err.Error(), "没有当前任务") {
		t.Fatalf("task scope without a task: %v", err)
	}
	marker := filepath.Join(base, "must-not-be-created")
	command := "printf 'two  spaces'; touch '" + marker + "'"
	configCLI(t, env, repo, "config", "set", "--project", "test.python.command", command)
	configCLI(t, env, repo, "config", "set", "test.python.prepare", command, "--project")
	configCLI(t, env, repo, "config", "set", "analysis.tests", "on", "--project")
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("saving settings executed a command: %v", err)
	}
	got := configCLI(t, env, repo, "config", "get", "test.python.command")
	if !strings.Contains(got, "two  spaces") || !strings.Contains(got, "来源: 项目") {
		t.Fatalf("command text or source missing: %s", got)
	}
}

func TestConfigReadDoesNotNeedAWriteLock(t *testing.T) {
	base := t.TempDir()
	env := platform.Env{DataDir: filepath.Join(base, "data")}
	repo := filepath.Join(base, "repo")
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	configCLI(t, env, repo, "config", "set", "workflow", "semi", "--project")
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(env.DataDir, "taskbelay.db"))}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(t.Context(), "ROLLBACK")
	if got := configCLI(t, env, repo, "config", "get", "workflow"); !strings.Contains(got, "workflow = semi") {
		t.Fatalf("read under write lock: %s", got)
	}
}

func TestCLIRejectsUnsupportedRequestsWithoutOpeningLedger(t *testing.T) {
	base := t.TempDir()
	env := platform.Env{DataDir: filepath.Join(base, "data")}
	for _, args := range [][]string{
		{"config", "set", "workflow", "other"},
		{"config", "set", "test.go.parallel", "5"},
		{"config", "get", "unknown"},
		{"config", "get", "workflow", "--project"},
		{"config", "set", "workflow", "semi", "--task", "--project"},
		{"config", "set", "workflow"},
		{"check"},
	} {
		var output bytes.Buffer
		if err := run(t.Context(), args, env, base, &output); err == nil {
			t.Errorf("accepted invalid command: %v", args)
		}
	}
	configCLI(t, env, base, "--help")
	if _, err := os.Stat(env.DataDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("help or invalid request opened the ledger: %v", err)
	}
}

func TestScopeOptionDoesNotConsumeCommandValue(t *testing.T) {
	name, value, scope, err := parseSet([]string{"test.go.command", "--task", "--project"})
	if err != nil || name != "test.go.command" || value != "--task" || scope != ledger.Project {
		t.Fatalf("parseSet = %q, %q, %q, %v", name, value, scope, err)
	}
}

func TestErrorLogStoresOneRecordPerFailure(t *testing.T) {
	env := platform.Env{DataDir: t.TempDir()}
	for _, message := range []string{"database busy", "git failed\nsecond line"} {
		if err := logError(env, errors.New(message)); err != nil {
			t.Fatal(err)
		}
	}
	files, err := filepath.Glob(filepath.Join(env.DataDir, "logs", "*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("daily logs: %v, %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("error message split log records: %s", data)
	}
	for _, line := range lines {
		var record map[string]string
		if err := json.Unmarshal(line, &record); err != nil || record["error"] == "" || record["time"] == "" {
			t.Fatalf("invalid log record: %s, %v", line, err)
		}
	}
}
