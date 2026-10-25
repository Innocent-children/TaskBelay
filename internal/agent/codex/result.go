package codex

import (
	"encoding/json"
	"regexp"
	"strconv"

	"github.com/Innocent-children/taskbelay/internal/event"
)

var commandHeader = regexp.MustCompile(`^(?:Chunk ID: [A-Za-z0-9_-]+\r?\n)?Wall time: [0-9]+(?:\.[0-9]+)? seconds\r?\n(?:Process exited with code (-?[0-9]+)|Exit code: (-?[0-9]+)|Process running with session ID ([0-9]+))\r?\n(?:Original token count: [0-9]+\r?\n)?(?:Output|Final output):(?:\r?\n|$)`)

/**
 * PostToolUse 可以传结构化结果，也可以传宿主生成的模型可见结果。
 * 文本只读取完整宿主头部，不从命令输出中的成功字样或退出码推断结果。
 */
func commandResult(raw json.RawMessage) *event.ToolResult {
	result := &event.ToolResult{State: event.Unknown, Output: string(raw)}
	var text string
	encoded := json.Unmarshal(raw, &text) == nil
	if encoded {
		result.Output = text
		raw = json.RawMessage(text)
	}
	var response struct {
		ExitCode *int            `json:"exit_code"`
		Session  json.RawMessage `json:"session_id"`
		Output   *string         `json:"output"`
		WallTime *float64        `json:"wall_time_seconds"`
	}
	if json.Unmarshal(raw, &response) == nil && (!encoded || response.Output != nil && response.WallTime != nil) {
		result.ExitCode = response.ExitCode
		if response.ExitCode == nil && len(response.Session) > 0 && string(response.Session) != "null" && string(response.Session) != `""` {
			result.State = event.Running
		}
	} else if encoded {
		match := commandHeader.FindStringSubmatch(text)
		if match == nil {
			return result
		}
		if match[3] != "" {
			result.State = event.Running
			return result
		}
		code, err := strconv.Atoi(match[1] + match[2])
		if err != nil {
			return result
		}
		result.ExitCode = &code
	}
	if result.ExitCode != nil {
		result.State = event.Completed
		if *result.ExitCode != 0 {
			result.State = event.Failed
		}
	}
	return result
}
