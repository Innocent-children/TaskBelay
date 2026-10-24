// Package codex 是 Codex 的适配器：把 Codex 的 hook 输入转成统一事件，把决定写成
// Codex 要的输出，安装和卸载 Codex 的 hook 条目。
package codex

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// Adapter 是 Codex 的适配器。
type Adapter struct{}

// Name 返回 codex，也是 hook 命令里的 <agent>。
func (Adapter) Name() string { return "codex" }

// input 是 Codex 的 hook 输入里 TaskBelay 用到的字段。
type input struct {
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
	Cwd       string `json:"cwd"`
	Source    string `json:"source"`
	Prompt    string `json:"prompt"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"` // Bash 是命令文本，apply_patch 是补丁文本
	} `json:"tool_input"`
	ToolUseID string `json:"tool_use_id"`
}

// patchPathPrefixes 是 apply_patch 补丁里写着文件路径的行的开头。
var patchPathPrefixes = []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "}

// ParseEvent 把一次 hook 调用的输入转成统一事件。hookEvent 是 hook 命令里的事件名。
func (a Adapter) ParseEvent(hookEvent string, data []byte) (event.Event, error) {
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		return event.Event{}, fmt.Errorf("读取 Codex 的 %s 输入：%w", hookEvent, err)
	}
	if in.SessionID == "" || in.Cwd == "" {
		return event.Event{}, fmt.Errorf("Codex 的 %s 输入里没有 session_id 或 cwd", hookEvent)
	}
	ev := event.Event{Agent: a.Name(), Session: in.SessionID, Turn: in.TurnID, Dir: in.Cwd}

	switch hookEvent {
	case "session-start":
		ev.Kind = event.SessionStart
		ev.Source = in.Source
	case "user-prompt-submit":
		ev.Kind = event.UserPrompt
		ev.Prompt = in.Prompt
	case "pre-tool-use":
		switch in.ToolName {
		case "Bash":
			ev.Kind = event.CommandRequest
			ev.Command = in.ToolInput.Command
			ev.Syntax = event.POSIX
			if platform.CodexPowerShell {
				ev.Syntax = event.PowerShell
			}
			ev.ToolUseID = in.ToolUseID
		case "apply_patch":
			ev.Kind = event.EditRequest
			ev.Files = patchFiles(in.ToolInput.Command)
		default:
			return event.Event{}, fmt.Errorf("PreToolUse 收到不处理的工具 %s", in.ToolName)
		}
	case "post-tool-use":
		// Codex 的 tool_response 只有命令的输出，没有退出码，结果一律记为未知。
		ev.Kind = event.CommandDone
		ev.ToolUseID = in.ToolUseID
		ev.Result = event.Unknown
	case "stop":
		ev.Kind = event.Stop
	default:
		return event.Event{}, fmt.Errorf("Codex 没有接 %s 这个 hook", hookEvent)
	}
	return ev, nil
}

// patchFiles 从 apply_patch 的补丁文本里取出要新增、修改、删除和改名到的文件。
func patchFiles(patch string) []string {
	var files []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimRight(line, "\r")
		for _, prefix := range patchPathPrefixes {
			if path, ok := strings.CutPrefix(line, prefix); ok {
				files = append(files, strings.TrimSpace(path))
			}
		}
	}
	return files
}

// RenderDecision 把决定写成 Codex 的 hook 输出。
func (Adapter) RenderDecision(hookEvent string, d event.Decision) ([]byte, error) {
	return hookjson.Render(hookjson.EventName(hookEvent), d)
}

// hooksPath 是 Codex 用户级 hook 配置文件的位置。
func hooksPath(env platform.Env) string {
	return filepath.Join(env.Home, ".codex", "hooks.json")
}

// handler 是 Codex 的一个 hook 处理器。Codex 用自己运行命令的 shell 运行 command：
// macOS 和 Linux 上是 bash 或 zsh 的 -c，Windows 上是 PowerShell 的 -Command。
type handler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// entries 是 TaskBelay 在 Codex 里的 6 个 hook 条目。
func entries(exe string) []hookjson.Entry {
	list := []struct {
		hook, matcher string
		timeout       int
	}{
		{"session-start", "", 30},
		{"user-prompt-submit", "", 30},
		{"pre-tool-use", "Bash", 30},
		{"pre-tool-use", "apply_patch", 30},
		{"post-tool-use", "Bash", 30},
		{"stop", "", 300},
	}
	var out []hookjson.Entry
	for _, e := range list {
		out = append(out, hookjson.Entry{
			Event:   hookjson.EventName(e.hook),
			Matcher: e.matcher,
			Handler: handler{Type: "command", Command: platform.CodexCommand(exe, "hook", "codex", e.hook), Timeout: e.timeout},
		})
	}
	return out
}

// ours 认出 TaskBelay 写入的处理器：命令里是加了单引号的 taskbelay 程序，后面跟 hook codex。
func ours(raw json.RawMessage) bool {
	var h handler
	if json.Unmarshal(raw, &h) != nil {
		return false
	}
	return strings.Contains(platform.PathKey(h.Command), platform.PathKey(platform.ExeName+"' hook codex "))
}

// Install 在 Codex 的用户级 hook 配置里写入 TaskBelay 的 hook 条目，原有的 TaskBelay 条目
// 先删掉，用户自己的条目保持不变。exe 是 taskbelay 程序的绝对路径。
func (Adapter) Install(env platform.Env, exe string) (string, error) {
	path := hooksPath(env)
	if err := hookjson.EditHooks(path, ours, entries(exe)); err != nil {
		return "", err
	}
	return fmt.Sprintf("已在 %s 写入 TaskBelay 的 hook。请打开 Codex，用 /hooks 确认信任这些 hook，确认之前它们不会运行。", path), nil
}

// Uninstall 删除 Codex 配置里 TaskBelay 的 hook 条目。
func (Adapter) Uninstall(env platform.Env) (string, error) {
	path := hooksPath(env)
	if err := hookjson.EditHooks(path, ours, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("已从 %s 删除 TaskBelay 的 hook，任务账本里的数据保留。", path), nil
}
