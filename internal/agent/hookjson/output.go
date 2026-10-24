package hookjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/event"
)

// EventName 把 hook 命令里的事件名换成 Agent 配置和输出里用的名字，
// 如 pre-tool-use 换成 PreToolUse。
func EventName(arg string) string {
	parts := strings.Split(arg, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

// output 是 hook 的输出。Claude Code 和 Codex 用的字段相同；Codex 不接受多余的字段，
// 所以空的字段一律不写出来。
type output struct {
	Decision      string    `json:"decision,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	SystemMessage string    `json:"systemMessage,omitempty"`
	Specific      *specific `json:"hookSpecificOutput,omitempty"`
}

type specific struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

// Render 把决定写成 hook 的输出。name 是 Agent 那边的事件名，如 PreToolUse。
// 决定为空时返回 nil，hook 什么都不输出。决定里有这个事件做不到的事时返回错误：
// 只有 PreToolUse 能拒绝工具调用，只有 Stop 能阻止结束，Stop 不补充上下文。
func Render(name string, d event.Decision) ([]byte, error) {
	if d.Empty() {
		return nil, nil
	}
	if d.Deny != "" && name != "PreToolUse" {
		return nil, fmt.Errorf("%s 不能拒绝工具调用", name)
	}
	if d.Block != "" && name != "Stop" {
		return nil, fmt.Errorf("%s 不能阻止结束", name)
	}
	if d.Context != "" && name == "Stop" {
		return nil, fmt.Errorf("Stop 不补充上下文")
	}

	out := output{SystemMessage: d.Notice}
	if d.Block != "" {
		out.Decision = "block"
		out.Reason = d.Block
	}
	if d.Deny != "" || d.Context != "" {
		out.Specific = &specific{HookEventName: name, AdditionalContext: d.Context}
		if d.Deny != "" {
			out.Specific.PermissionDecision = "deny"
			out.Specific.PermissionDecisionReason = d.Deny
		}
	}
	return MarshalASCII(out)
}

// MarshalASCII 把 v 编码成只含 ASCII 字符的 JSON：非 ASCII 字符一律写成 \u 转义，
// 超出基本平面的字符写成一对代理项。这样输出不管经过哪种 shell 都不会被转码。
func MarshalASCII(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]
		if r < utf8.RuneSelf {
			b.WriteByte(byte(r))
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&b, `\u%04x`, u)
		}
	}
	return b.Bytes(), nil
}
