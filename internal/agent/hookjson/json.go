package hookjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf16"

	"github.com/Innocent-children/taskbelay/internal/event"
)

var Events = map[string]string{
	"session-start": "SessionStart", "user-prompt-submit": "UserPromptSubmit",
	"pre-tool-use": "PreToolUse", "post-tool-use": "PostToolUse",
	"post-tool-use-failure": "PostToolUseFailure", "stop": "Stop",
}

type Input struct {
	SessionID   string          `json:"session_id"`
	PromptID    string          `json:"prompt_id"`
	TurnID      string          `json:"turn_id"`
	CWD         string          `json:"cwd"`
	Event       string          `json:"hook_event_name"`
	Source      string          `json:"source"`
	Prompt      string          `json:"prompt"`
	Tool        string          `json:"tool_name"`
	CallID      string          `json:"tool_use_id"`
	ToolInput   json.RawMessage `json:"tool_input"`
	Response    json.RawMessage `json:"tool_response"`
	Error       string          `json:"error"`
	Interrupted bool            `json:"is_interrupt"`
}

func Read(hook string, input []byte) (Input, error) {
	var in Input
	name, ok := Events[hook]
	if !ok {
		return in, fmt.Errorf("未知 hook 事件 %q", hook)
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	if err := dec.Decode(&in); err != nil {
		return in, fmt.Errorf("读取 hook JSON: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return in, fmt.Errorf("hook 输入必须只有一个 JSON 对象")
	}
	if in.SessionID == "" || in.CWD == "" || in.Event != name {
		return in, fmt.Errorf("hook 缺少 session、工作目录或事件名称不匹配")
	}
	return in, nil
}

func Base(in Input, agent, round string) (event.Event, error) {
	e := event.Event{Agent: agent, SessionID: in.SessionID, RoundID: round, WorkingDir: in.CWD, ToolCallID: in.CallID}
	switch in.Event {
	case "SessionStart":
		e.Kind = event.SessionStart
		switch in.Source {
		case "startup":
			e.StartMode = event.NewSession
		case "resume":
			e.StartMode = event.Resume
		case "clear":
			e.StartMode = event.Clear
		case "compact":
			e.StartMode = event.Compact
		case "fork":
			e.StartMode = event.Fork
		default:
			return e, fmt.Errorf("无法识别 session 来源 %q", in.Source)
		}
	case "UserPromptSubmit":
		if round == "" {
			return e, fmt.Errorf("用户消息缺少原生轮次编号")
		}
		e.Kind, e.Message, e.MessageID = event.UserPrompt, in.Prompt, round
	case "Stop":
		e.Kind = event.Stop
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		if in.CallID == "" || in.Tool == "" {
			return e, fmt.Errorf("工具事件缺少工具名或调用编号")
		}
	}
	return e, nil
}

func Render(hook string, d event.Decision, failureSupported bool) ([]byte, error) {
	name, ok := Events[hook]
	if !ok || hook == "post-tool-use-failure" && !failureSupported {
		return nil, fmt.Errorf("不支持 hook %q", hook)
	}
	if d.DenyTool != "" && hook != "pre-tool-use" || d.BlockStop != "" && hook != "stop" {
		return nil, fmt.Errorf("决定不适用于当前 hook")
	}
	out := make(map[string]any)
	if d.UserMessage != "" {
		out["systemMessage"] = d.UserMessage
	}
	if d.BlockStop != "" {
		out["decision"], out["reason"] = "block", d.BlockStop
	}
	if hook != "stop" && (d.AdditionalContext != "" || d.DenyTool != "") {
		specific := map[string]string{"hookEventName": name}
		if d.AdditionalContext != "" {
			specific["additionalContext"] = d.AdditionalContext
		}
		if d.DenyTool != "" {
			specific["permissionDecision"], specific["permissionDecisionReason"] = "deny", d.DenyTool
		}
		out["hookSpecificOutput"] = specific
	}
	if len(out) == 0 {
		return nil, nil
	}
	return Marshal(out)
}

/**
 * JSON 标准允许 Unicode 转义。统一输出 ASCII，避免 hook 宿主按本地代码页
 * 解码中文；代理对保证非 BMP 字符在两个 Agent 中也能无损回读。
 */
func Marshal(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for _, r := range string(data) {
		if r < 128 {
			out.WriteByte(byte(r))
			continue
		}
		if r <= 0xffff {
			fmt.Fprintf(&out, "\\u%04x", r)
			continue
		}
		high, low := utf16.EncodeRune(r)
		fmt.Fprintf(&out, "\\u%04x\\u%04x", high, low)
	}
	return out.Bytes(), nil
}
