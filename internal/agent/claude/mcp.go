package claude

import (
	"fmt"

	"github.com/Innocent-children/taskbelay/internal/event"
)

/**
 * session 来自 MCP 子进程启动时的 CLAUDE_CODE_SESSION_ID，调用编号来自
 * 原生元数据。轮次只允许从这次调用的前置记录回读，不使用最近消息补齐。
 */
func MCPIdentity(meta map[string]any, session string) (event.Event, bool, error) {
	value, found := meta["claudecode/toolUseId"]
	if !found {
		return event.Event{}, false, nil
	}
	call, ok := value.(string)
	if !ok || call == "" || session == "" {
		return event.Event{}, true, fmt.Errorf("Claude Code MCP 缺少原生工具调用编号或 CLAUDE_CODE_SESSION_ID，无法核对写入身份")
	}
	return event.Event{Agent: "claude", SessionID: session, ToolCallID: call}, true, nil
}
