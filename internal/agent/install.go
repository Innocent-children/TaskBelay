package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

type Runner func(context.Context, string, ...string) ([]byte, error)

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func installPath(env platform.Env, name string) (string, error) {
	switch name {
	case "claude":
		return filepath.Join(env.HomeDir, ".claude", "settings.json"), nil
	case "codex":
		return filepath.Join(env.HomeDir, ".codex", "hooks.json"), nil
	default:
		return "", fmt.Errorf("不支持 Agent %q", name)
	}
}

func hookGroups(name, exe string) []hookjson.Group {
	shell, edit := "Bash", "apply_patch"
	if name == "claude" {
		shell, edit = "Bash|PowerShell", "Edit|Write|NotebookEdit"
	}
	groups := []hookjson.Group{
		{Event: "SessionStart"}, {Event: "UserPromptSubmit"}, {Event: "Stop"},
		{Event: "PreToolUse", Matcher: shell}, {Event: "PreToolUse", Matcher: edit},
		{Event: "PostToolUse", Matcher: shell + "|" + edit},
	}
	if name == "claude" {
		groups = append(groups, hookjson.Group{Event: "PreToolUse", Matcher: "^mcp__taskbelay__.*$"})
		groups = append(groups, hookjson.Group{Event: "PostToolUseFailure", Matcher: shell + "|" + edit})
	}
	for i := range groups {
		hook := ""
		for command, native := range hookjson.Events {
			if native == groups[i].Event {
				hook = command
				break
			}
		}
		timeout := 30
		if hook == "stop" {
			timeout = 300
		}
		handler := hookjson.Object{}
		_ = handler.Set("type", "command")
		if name == "claude" {
			_ = handler.Set("command", exe)
			_ = handler.Set("args", []string{"hook", name, hook})
		} else {
			_ = handler.Set("command", platform.CodexHookCommand(exe, hook))
		}
		_ = handler.Set("timeout", timeout)
		groups[i].Handler = handler
	}
	return groups
}

func ownedExecutable(path string) bool {
	return filepath.IsAbs(path) && (filepath.Base(path) == "taskbelay" || filepath.Base(path) == "taskbelay.exe")
}

func ownedHook(name string) func(hookjson.Object) bool {
	return func(handler hookjson.Object) bool {
		var kind, command string
		if json.Unmarshal(handler.Get("type"), &kind) != nil || kind != "command" || json.Unmarshal(handler.Get("command"), &command) != nil {
			return false
		}
		if name == "claude" {
			var args []string
			if json.Unmarshal(handler.Get("args"), &args) != nil || len(args) != 3 {
				return false
			}
			_, valid := hookjson.Events[args[2]]
			return ownedExecutable(command) && args[0] == "hook" && args[1] == name && valid
		}
		// 只识别本程序生成的完整命令，不能用包含 taskbelay 的子串删除用户脚本。
		for hook := range hookjson.Events {
			for i := range command {
				if command[i] != ' ' {
					continue
				}
				prefix := command[:i]
				if strings.HasPrefix(prefix, "& ") {
					prefix = prefix[2:]
				}
				if len(prefix) < 2 || prefix[0] != '\'' || prefix[len(prefix)-1] != '\'' {
					continue
				}
				path := prefix[1 : len(prefix)-1]
				if platform.CodexShell() == "powershell" {
					path = strings.NewReplacer("''", "'", "‘‘", "‘", "’’", "’", "‚‚", "‚", "‛‛", "‛").Replace(path)
				} else {
					path = strings.ReplaceAll(path, "'\\''", "'")
				}
				if ownedExecutable(path) && platform.CodexHookCommand(path, hook) == command {
					return true
				}
			}
		}
		return false
	}
}

var versionPattern = regexp.MustCompile(`(?:^|\s)([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`)

func requireVersion(output []byte, minimum [3]int, label string) error {
	m := versionPattern.FindStringSubmatch(string(output))
	if m == nil {
		return fmt.Errorf("无法核对 %s 版本；需要 %d.%d.%d 或更高", label, minimum[0], minimum[1], minimum[2])
	}
	var actual [3]int
	for i := range actual {
		if m[i+1] != "" {
			n, err := strconv.Atoi(m[i+1])
			if err != nil {
				return err
			}
			actual[i] = n
		}
	}
	for i := range actual {
		if actual[i] > minimum[i] {
			return nil
		}
		if actual[i] < minimum[i] {
			return fmt.Errorf("%s 需要 %d.%d.%d 或更高，当前为 %d.%d.%d", label, minimum[0], minimum[1], minimum[2], actual[0], actual[1], actual[2])
		}
	}
	return nil
}

func mcpArgs(name, action, exe string) []string {
	args := []string{"mcp", action}
	if name == "claude" {
		args = append(args, "--scope", "user")
	}
	args = append(args, "taskbelay")
	if action == "add" {
		args = append(args, "--", exe, "mcp")
	}
	return args
}

func manual(name, exe string) string {
	data, err := hookjson.Merge([]byte("{}"), hookGroups(name, exe), ownedHook(name))
	if err != nil {
		return ""
	}
	args, _ := json.Marshal(append([]string{name}, mcpArgs(name, "add", exe)...))
	message := "请将以下 hooks 合并到用户配置（保留现有内容）：\n" + string(data) + "\nMCP 注册程序及参数：" + string(args) + "\n"
	if name == "claude" {
		mcp, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"taskbelay": map[string]any{"type": "stdio", "command": exe, "args": []string{"mcp"}}}}, "", "  ")
		message += "也可合并到 ~/.claude.json（保留其他 MCP）：\n" + string(mcp) + "\n"
	} else {
		command, _ := json.Marshal(exe)
		message += "也可合并到 ~/.codex/config.toml（保留其他 MCP）：\n[mcp_servers.taskbelay]\ncommand = " + string(command) + "\nargs = [\"mcp\"]\n随后用 /hooks 确认信任。\n"
	}
	return message
}

func checkConfigHome(env platform.Env, name string) error {
	key, dir := "CODEX_HOME", ".codex"
	if name == "claude" {
		key, dir = "CLAUDE_CONFIG_DIR", ".claude"
	}
	if value := os.Getenv(key); value != "" && (name == "claude" || filepath.Clean(value) != filepath.Join(env.HomeDir, dir)) {
		return fmt.Errorf("%s 指向其他配置目录；当前只安装文档规定的用户目录 %s，未修改配置", key, filepath.Join(env.HomeDir, dir))
	}
	return nil
}

func mcpOwned(ctx context.Context, env platform.Env, name string, run Runner) (exists, owned bool, err error) {
	var command string
	var args []string
	if name == "claude" {
		data, readErr := os.ReadFile(filepath.Join(env.HomeDir, ".claude.json"))
		if os.IsNotExist(readErr) {
			return false, false, nil
		}
		if readErr != nil {
			return false, false, readErr
		}
		var config struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return false, false, err
		}
		raw, ok := config.Servers["taskbelay"]
		if !ok {
			return false, false, nil
		}
		var entry struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return true, false, err
		}
		command, args = entry.Command, entry.Args
	} else {
		data, getErr := run(ctx, name, "mcp", "get", "taskbelay", "--json")
		if getErr != nil {
			if strings.Contains(string(data), "No MCP server named 'taskbelay' found") {
				return false, false, nil
			}
			return false, false, fmt.Errorf("无法核对已有 MCP 注册: %w", getErr)
		}
		var entry struct {
			Transport struct {
				Type    string   `json:"type"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"transport"`
		}
		if err := json.Unmarshal(data, &entry); err != nil {
			return true, false, err
		}
		if entry.Transport.Type != "stdio" {
			return true, false, nil
		}
		command, args = entry.Transport.Command, entry.Transport.Args
	}
	return true, ownedExecutable(command) && len(args) == 1 && args[0] == "mcp", nil
}

func Install(env platform.Env, exe, name string, run Runner) (string, error) {
	path, err := installPath(env, name)
	if err != nil {
		return "", err
	}
	if !ownedExecutable(exe) {
		return "", fmt.Errorf("TaskBelay 必须使用名为 taskbelay 或 taskbelay.exe 的绝对程序路径")
	}
	if run == nil {
		if err := checkConfigHome(env, name); err != nil {
			return "", err
		}
		run = runCommand
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	version, err := run(ctx, "git", "--version")
	if err != nil {
		return "", fmt.Errorf("安装需要 git 2.5 或更高: %w", err)
	}
	if err := requireVersion(version, [3]int{2, 5, 0}, "git"); err != nil {
		return "", err
	}
	version, err = run(ctx, name, "--version")
	if err != nil {
		var missing *exec.Error
		if errors.As(err, &missing) && errors.Is(err, exec.ErrNotFound) {
			return manual(name, exe), fmt.Errorf("找不到 %s 命令: %w", name, err)
		}
		return "", fmt.Errorf("无法运行 %s: %w", name, err)
	}
	if name == "claude" {
		if err := requireVersion(version, [3]int{2, 1, 139}, "Claude Code"); err != nil {
			return "", err
		}
	}
	exists, owned, err := mcpOwned(ctx, env, name, run)
	if err != nil {
		return "", err
	}
	if exists && !owned {
		return "", fmt.Errorf("已有名为 taskbelay 的 MCP 不属于 TaskBelay，已保留原配置")
	}
	file, err := hookjson.Load(path)
	if err != nil {
		return "", err
	}
	data, err := hookjson.Merge(file.Before, hookGroups(name, exe), ownedHook(name))
	if err != nil {
		return "", err
	}
	if err := file.Write(data); err != nil {
		return "", err
	}
	partial := "TaskBelay hook 配置已写入；MCP 注册尚未完成，可修复原因后重试。\n"
	if exists {
		if _, err := run(ctx, name, mcpArgs(name, "remove", "")...); err != nil {
			return partial, fmt.Errorf("移除旧 MCP 注册: %w", err)
		}
	}
	if _, err := run(ctx, name, mcpArgs(name, "add", exe)...); err != nil {
		return partial, fmt.Errorf("注册 MCP: %w", err)
	}
	message := "已写入 TaskBelay hook 并注册 stdio MCP。\n"
	if name == "codex" {
		message += "请在 Codex 中用 /hooks 确认信任；未确认前 hook 不会运行。\n"
	} else {
		message += "请新建 Claude Code session 加载 hook。\n"
	}
	return message, nil
}

func Uninstall(env platform.Env, name string, run Runner) (string, error) {
	path, err := installPath(env, name)
	if err != nil {
		return "", err
	}
	if run == nil {
		if err := checkConfigHome(env, name); err != nil {
			return "", err
		}
		run = runCommand
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exists, owned, err := mcpOwned(ctx, env, name, run)
	if err != nil {
		return "", err
	}
	file, err := hookjson.Load(path)
	if err != nil {
		return "", err
	}
	if len(file.Before) > 0 {
		data, err := hookjson.Merge(file.Before, nil, ownedHook(name))
		if err != nil {
			return "", err
		}
		if err := file.Write(data); err != nil {
			return "", err
		}
	}
	if exists && owned {
		if _, err := run(ctx, name, mcpArgs(name, "remove", "")...); err != nil {
			return "hook 已移除，MCP 卸载未完成。\n", fmt.Errorf("移除 MCP: %w", err)
		}
	}
	message := "已移除 TaskBelay hook 和所属 MCP；账本数据保留。\n"
	if exists && !owned {
		message += "同名 MCP 属于其他程序，已保留。\n"
	}
	return message, nil
}
