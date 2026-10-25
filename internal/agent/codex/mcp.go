package codex

import (
	"encoding/json"
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/event"
)

/**
 * Codex 0.163.0 把原生调用编号放在 callId，把 hook 使用的 session 和轮次
 * 放在 x-codex-turn-metadata；threadId 不是 hook 的 session_id。
 * 字段缺失或相互矛盾时不回退到最近活动，也不使用 JSON-RPC 请求编号。
 */
func MCPIdentity(meta map[string]any) (event.Event, bool, error) {
	value, ok := meta["x-codex-turn-metadata"]
	if !ok {
		return event.Event{}, false, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return event.Event{}, true, err
	}
	var identity struct {
		Session string `json:"session_id"`
		Round   string `json:"turn_id"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		return event.Event{}, true, fmt.Errorf("无法读取 Codex MCP 身份: %w", err)
	}
	call, ok := meta["callId"].(string)
	if !ok || call == "" || identity.Session == "" || identity.Round == "" {
		return event.Event{}, true, fmt.Errorf("Codex MCP 缺少原生 session、轮次或工具调用编号")
	}
	if raw, exists := meta["sessionId"]; exists {
		session, ok := raw.(string)
		if !ok || session != identity.Session {
			return event.Event{}, true, fmt.Errorf("Codex MCP 的 session 身份不一致")
		}
	}
	return event.Event{Agent: "codex", SessionID: identity.Session, RoundID: identity.Round, ToolCallID: call}, true, nil
}
