package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/skills"
)

type nativeFixture struct {
	home, gitVersion, claudeVersion, command string
	missing, failAdd                         bool
	calls                                    [][]string
}

func (n *nativeFixture) run(_ context.Context, name string, args ...string) ([]byte, error) {
	n.calls = append(n.calls, append([]string{name}, args...))
	if name == "git" {
		return []byte(n.gitVersion), nil
	}
	if n.missing {
		return nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	if args[0] == "--version" {
		if name == "claude" {
			return []byte(n.claudeVersion), nil
		}
		return []byte("codex-cli 0.163.0"), nil
	}
	switch args[1] {
	case "get":
		if n.command == "" {
			return []byte("Error: No MCP server named 'taskbelay' found."), errors.New("exit 1")
		}
		return json.Marshal(map[string]any{"transport": map[string]any{"type": "stdio", "command": n.command, "args": []string{"mcp"}}})
	case "remove":
		n.command = ""
	case "add":
		if n.failAdd {
			return nil, errors.New("registration failed")
		}
		n.command = args[len(args)-2]
	default:
		return nil, errors.New("unexpected native command")
	}
	if name == "claude" {
		servers := map[string]any{"user-server": map[string]any{"command": "user-command"}}
		if n.command != "" {
			servers["taskbelay"] = map[string]any{"command": n.command, "args": []string{"mcp"}}
		}
		data, _ := json.Marshal(map[string]any{"mcpServers": servers})
		if err := os.WriteFile(filepath.Join(n.home, ".claude.json"), data, 0o600); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func fixture(t *testing.T) (platform.Env, *nativeFixture, string) {
	t.Helper()
	home := t.TempDir()
	return platform.Env{HomeDir: home, DataDir: filepath.Join(home, ".taskbelay")}, &nativeFixture{home: home, gitVersion: "git version 2.50.1", claudeVersion: "2.1.309 (Claude Code)"}, filepath.Join(home, "中文 space ' ‘ dir", platform.ExecutableName())
}

func readConfig(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInstallRepeatAndUninstallPreserveUserConfiguration(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			env, native, exe := fixture(t)
			config, skillPath, _ := installPaths(env, name)
			if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
				t.Fatal(err)
			}
			original := []byte(`{"z-user":{"second":2,"first":1},"hooks":{"PreToolUse":[{"matcher":"user","extra":"keep","hooks":[{"type":"command","command":"echo taskbelay hook codex stop"}]}],"Stop":[{"hooks":[{"type":"command","command":"user-hook"}]}]},"a-user":true}`)
			if err := os.WriteFile(config, original, 0o640); err != nil {
				t.Fatal(err)
			}
			userSkill := filepath.Join(skillPath, "write-a-well-document")
			if err := os.MkdirAll(userSkill, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(userSkill, "SKILL.md"), []byte("user version"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(env.DataDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(env.DataDir, "keep"), []byte("ledger"), 0o600); err != nil {
				t.Fatal(err)
			}
			message, err := Install(env, exe, name, native.run)
			if err != nil {
				t.Fatal(message, err)
			}
			if name == "codex" && !strings.Contains(message, "/hooks") {
				t.Fatal(message)
			}
			first := readConfig(t, config)
			if _, err := Install(env, exe, name, native.run); err != nil {
				t.Fatal(err)
			}
			second := readConfig(t, config)
			if !bytes.Equal(first, second) {
				t.Fatal("repeat duplicated or reordered hooks")
			}
			if bytes.Index(second, []byte(`"z-user"`)) > bytes.Index(second, []byte(`"hooks"`)) || bytes.Index(second, []byte(`"hooks"`)) > bytes.Index(second, []byte(`"a-user"`)) || bytes.Index(second, []byte(`"second"`)) > bytes.Index(second, []byte(`"first"`)) {
				t.Fatal("user key order changed")
			}
			var root struct {
				Hooks map[string][]struct {
					Hooks []json.RawMessage `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(second, &root); err != nil {
				t.Fatal(err)
			}
			count := 0
			for event, groups := range root.Hooks {
				for _, group := range groups {
					for _, handler := range group.Hooks {
						object, _ := hookjson.ParseObject(handler)
						if !ownedHook(name)(object) {
							continue
						}
						count++
						var timeout int
						_ = json.Unmarshal(object.Get("timeout"), &timeout)
						want := 30
						if event == "Stop" {
							want = 300
						}
						if timeout != want {
							t.Fatal(timeout)
						}
					}
				}
			}
			want := 6
			if name == "claude" {
				want = 8
			}
			if count != want {
				t.Fatalf("hooks=%d", count)
			}
			if string(readConfig(t, filepath.Join(userSkill, "SKILL.md"))) != "user version" {
				t.Fatal("replaced user skill")
			}
			if _, err := os.Stat(filepath.Join(userSkill, ".taskbelay")); !os.IsNotExist(err) {
				t.Fatal("marked user skill")
			}
			bundle, _ := skills.Files.ReadFile("write-technical-plans/references/diagrams.md")
			if !bytes.Equal(readConfig(t, filepath.Join(skillPath, "write-technical-plans", "references", "diagrams.md")), bundle) {
				t.Fatal("missing embedded references")
			}
			if native.command != exe {
				t.Fatal("MCP did not receive exact executable path")
			}
			if _, err := Uninstall(env, name, native.run); err != nil {
				t.Fatal(err)
			}
			last := readConfig(t, config)
			if !bytes.Contains(last, []byte("echo taskbelay hook codex stop")) || !bytes.Contains(last, []byte("user-hook")) || !bytes.Contains(last, []byte(`"extra": "keep"`)) {
				t.Fatal("deleted user hooks")
			}
			if _, err := os.Stat(filepath.Join(skillPath, "write-technical-plans")); !os.IsNotExist(err) {
				t.Fatal("owned skill retained")
			}
			if string(readConfig(t, filepath.Join(userSkill, "SKILL.md"))) != "user version" || string(readConfig(t, filepath.Join(env.DataDir, "keep"))) != "ledger" {
				t.Fatal("deleted user data")
			}
			if _, err := Uninstall(env, name, native.run); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInstallPreflightDoesNotWriteAndMissingCommandShowsManualConfig(t *testing.T) {
	for _, test := range []string{"old git", "old claude", "missing", "foreign MCP", "malformed config", "add failure"} {
		t.Run(test, func(t *testing.T) {
			env, native, exe := fixture(t)
			config, _, _ := installPaths(env, "claude")
			switch test {
			case "old git":
				native.gitVersion = "git version 2.4.9"
			case "old claude":
				native.claudeVersion = "2.1.138 (Claude Code)"
			case "missing":
				native.missing = true
			case "foreign MCP":
				native.command = filepath.Join(env.HomeDir, "unrelated")
				_, _ = native.run(t.Context(), "claude", "mcp", "add", "taskbelay", "--", native.command, "mcp")
			case "malformed config":
				if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(config, []byte(`{"hooks": []}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "add failure":
				native.failAdd = true
			}
			before, _ := os.ReadFile(config)
			message, err := Install(env, exe, "claude", native.run)
			if err == nil {
				t.Fatal("expected preflight error")
			}
			after, _ := os.ReadFile(config)
			if test != "add failure" && !bytes.Equal(before, after) {
				t.Fatal("wrote config on failed preflight")
			}
			if test == "missing" && (!strings.Contains(message, `"SessionStart"`) || !strings.Contains(message, "MCP")) {
				t.Fatal(message)
			}
			if test == "add failure" && !strings.Contains(message, "尚未全部完成") {
				t.Fatal("hid partial installation")
			}
		})
	}
}

func TestConfigSymlinkAndOwnedHandlerMixedWithUserHandler(t *testing.T) {
	env, native, exe := fixture(t)
	path, _, _ := installPaths(env, "codex")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(env.HomeDir, "linked.json")
	data := []byte(`{"first":1,"hooks":{},"last":2}`)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Install(env, exe, "codex", native.run); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced config symlink")
	}
	if bytes.Equal(data, readConfig(t, target)) {
		t.Fatal("target not edited")
	}
	group := hookGroups("codex", exe)[0]
	mixed, _ := json.Marshal(map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"custom": true, "hooks": []any{group.Handler, map[string]string{"type": "command", "command": "keep"}}}}}})
	result, err := hookjson.Merge(mixed, nil, ownedHook("codex"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte("taskbelay")) || !bytes.Contains(result, []byte("keep")) || !bytes.Contains(result, []byte("custom")) {
		t.Fatal(string(result))
	}
}

func TestInspectInstalledResourcesWithoutChangingConfiguration(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			env, native, exe := fixture(t)
			if _, err := Install(env, exe, name, native.run); err != nil {
				t.Fatal(err)
			}
			if name == "codex" {
				if err := os.WriteFile(filepath.Join(env.HomeDir, ".codex", "config.toml"), []byte("# fixture\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			config, _, _ := installPaths(env, name)
			before := readConfig(t, config)
			native.calls = nil
			state, err := Inspect(t.Context(), env, name, native.run)
			want := 6
			if name == "claude" {
				want = 8
			}
			if err != nil || state.Hooks != want || state.Skills != 2 || !state.MCP || state.ForeignMCP {
				t.Fatalf("inspection = %+v, err = %v", state, err)
			}
			if !bytes.Equal(before, readConfig(t, config)) {
				t.Fatal("inspection changed hooks")
			}
			for _, call := range native.calls {
				if len(call) < 3 || call[1] != "mcp" || call[2] != "get" {
					t.Fatalf("inspection executed a write command: %v", call)
				}
			}
		})
	}
}

func TestUninstallPreservesForeignMCPAndUnmarkedSkills(t *testing.T) {
	env, native, _ := fixture(t)
	native.command = filepath.Join(env.HomeDir, "foreign")
	_, skills, _ := installPaths(env, "codex")
	target := filepath.Join(skills, "write-technical-plans")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".taskbelay"), []byte("user file"), 0o600); err != nil {
		t.Fatal(err)
	}
	message, err := Uninstall(env, "codex", native.run)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "同名 MCP") || native.command == "" {
		t.Fatal(message)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("removed unowned skill")
	}
}
