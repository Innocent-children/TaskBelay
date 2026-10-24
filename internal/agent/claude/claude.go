// Package claude 是 Claude Code 的适配器：把 Claude Code 的 hook 输入转成统一事件，
// 把决定写成 Claude Code 要的输出，安装和卸载 Claude Code 的 hook 条目。
package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/platform"
)

// Adapter 是 Claude Code 的适配器。
type Adapter struct{}

// Name 返回 claude，也是 hook 命令里的 <agent>。
func (Adapter) Name() string { return "claude" }

// input 是 Claude Code 的 hook 输入里 TaskBelay 用到的字段。
type input struct {
	SessionID string `json:"session_id"`
	PromptID  string `json:"prompt_id"`
	Cwd       string `json:"cwd"`
	Source    string `json:"source"`
	Prompt    string `json:"prompt"`
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolResponse json.RawMessage `json:"tool_response"`
	Error        string          `json:"error"`
	IsInterrupt  bool            `json:"is_interrupt"`
}

// exitCodeLine 匹配 PostToolUseFailure 的 error 第一行。
var exitCodeLine = regexp.MustCompile(`^Exit code (-?\d+)`)

// ParseEvent 把一次 hook 调用的输入转成统一事件。hookEvent 是 hook 命令里的事件名。
func (a Adapter) ParseEvent(hookEvent string, data []byte) (event.Event, error) {
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		return event.Event{}, fmt.Errorf("读取 Claude Code 的 %s 输入：%w", hookEvent, err)
	}
	if in.SessionID == "" || in.Cwd == "" {
		return event.Event{}, fmt.Errorf("Claude Code 的 %s 输入里没有 session_id 或 cwd", hookEvent)
	}
	ev := event.Event{Agent: a.Name(), Session: in.SessionID, Turn: in.PromptID, Dir: in.Cwd}

	switch hookEvent {
	case "session-start":
		ev.Kind = event.SessionStart
		ev.Source = in.Source
	case "user-prompt-submit":
		ev.Kind = event.UserPrompt
		ev.Prompt = in.Prompt
	case "pre-tool-use":
		switch in.ToolName {
		case "Bash", "PowerShell":
			ev.Kind = event.CommandRequest
			ev.Command = in.ToolInput.Command
			ev.Syntax = event.POSIX
			if in.ToolName == "PowerShell" {
				ev.Syntax = event.PowerShell
			}
			ev.ToolUseID = in.ToolUseID
		case "Edit", "Write":
			ev.Kind = event.EditRequest
			ev.Files = []string{in.ToolInput.FilePath}
		case "NotebookEdit":
			ev.Kind = event.EditRequest
			ev.Files = []string{in.ToolInput.NotebookPath}
		default:
			return event.Event{}, fmt.Errorf("PreToolUse 收到不处理的工具 %s", in.ToolName)
		}
	case "post-tool-use":
		ev.Kind = event.CommandDone
		ev.ToolUseID = in.ToolUseID
		var resp struct {
			Interrupted bool `json:"interrupted"`
		}
		// tool_response 不是对象时读不出 interrupted，按没中断处理。
		_ = json.Unmarshal(in.ToolResponse, &resp)
		if resp.Interrupted {
			ev.Result = event.Unknown
		} else {
			code := 0
			ev.Result, ev.ExitCode = event.Pass, &code
		}
	case "post-tool-use-failure":
		ev.Kind = event.CommandDone
		ev.ToolUseID = in.ToolUseID
		ev.Result = event.Unknown
		if m := exitCodeLine.FindStringSubmatch(in.Error); m != nil && !in.IsInterrupt {
			code, _ := strconv.Atoi(m[1])
			ev.Result, ev.ExitCode = event.Fail, &code
		}
	case "stop":
		ev.Kind = event.Stop
	default:
		return event.Event{}, fmt.Errorf("Claude Code 没有接 %s 这个 hook", hookEvent)
	}
	return ev, nil
}

// RenderDecision 把决定写成 Claude Code 的 hook 输出。
func (Adapter) RenderDecision(hookEvent string, d event.Decision) ([]byte, error) {
	return hookjson.Render(hookjson.EventName(hookEvent), d)
}

// settingsPath 是 Claude Code 用户级配置文件的位置。
func settingsPath(env platform.Env) string {
	return filepath.Join(env.Home, ".claude", "settings.json")
}

// handler 是 Claude Code 的一个 hook 处理器。用 exec 形式：有 args 时 Claude Code
// 直接启动 command 指向的程序，不经过 shell，路径不用加引号，三个平台写法相同。
type handler struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Timeout int      `json:"timeout"`
}

// entries 是 TaskBelay 在 Claude Code 里的 7 个 hook 条目。
func entries(exe string) []hookjson.Entry {
	const shellTools = "Bash|PowerShell"
	list := []struct {
		hook, matcher string
		timeout       int
	}{
		{"session-start", "", 30},
		{"user-prompt-submit", "", 30},
		{"pre-tool-use", shellTools, 30},
		{"pre-tool-use", "Edit|Write|NotebookEdit", 30},
		{"post-tool-use", shellTools, 30},
		{"post-tool-use-failure", shellTools, 30},
		{"stop", "", 300},
	}
	var out []hookjson.Entry
	for _, e := range list {
		out = append(out, hookjson.Entry{
			Event:   hookjson.EventName(e.hook),
			Matcher: e.matcher,
			Handler: handler{Type: "command", Command: exe, Args: []string{"hook", "claude", e.hook}, Timeout: e.timeout},
		})
	}
	return out
}

// ours 认出 TaskBelay 写入的处理器：command 是 taskbelay 程序，参数以 hook claude 开头。
func ours(raw json.RawMessage) bool {
	var h handler
	if json.Unmarshal(raw, &h) != nil {
		return false
	}
	return platform.PathKey(filepath.Base(h.Command)) == platform.PathKey(platform.ExeName) &&
		len(h.Args) >= 2 && h.Args[0] == "hook" && h.Args[1] == "claude"
}

// minVersion 是 TaskBelay 需要的最低 Claude Code 版本：hook 的 exec 形式（args 字段）
// 从 2.1.139 开始有。更早的版本不认识 args，会不带参数运行 taskbelay。
var minVersion = [3]int{2, 1, 139}

var versionNumber = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// checkVersion 检查本机的 Claude Code 版本够不够新。
func checkVersion() error {
	out, err := exec.Command("claude", "--version").Output()
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("找不到 claude 命令：先安装 Claude Code，或者把它所在的目录加进 PATH")
	}
	if err != nil {
		return fmt.Errorf("claude --version：%w", err)
	}
	m := versionNumber.FindStringSubmatch(string(out))
	if m == nil {
		return fmt.Errorf("看不懂 Claude Code 的版本：%s", out)
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	for i := range v {
		if v[i] != minVersion[i] {
			if v[i] < minVersion[i] {
				return fmt.Errorf("Claude Code 版本是 %d.%d.%d，TaskBelay 需要 %d.%d.%d 或更高版本",
					v[0], v[1], v[2], minVersion[0], minVersion[1], minVersion[2])
			}
			break
		}
	}
	return nil
}

// Install 在 Claude Code 的用户级配置里写入 TaskBelay 的 hook 条目，原有的 TaskBelay 条目
// 先删掉，用户自己的条目保持不变。exe 是 taskbelay 程序的绝对路径。
func (Adapter) Install(env platform.Env, exe string) (string, error) {
	if err := checkVersion(); err != nil {
		return "", err
	}
	path := settingsPath(env)
	if err := hookjson.EditHooks(path, ours, entries(exe)); err != nil {
		return "", err
	}
	return fmt.Sprintf("已在 %s 写入 TaskBelay 的 hook，之后新开的 Claude Code session 生效。", path), nil
}

// Uninstall 删除 Claude Code 配置里 TaskBelay 的 hook 条目。
func (Adapter) Uninstall(env platform.Env) (string, error) {
	path := settingsPath(env)
	if err := hookjson.EditHooks(path, ours, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("已从 %s 删除 TaskBelay 的 hook，任务账本里的数据保留。", path), nil
}
