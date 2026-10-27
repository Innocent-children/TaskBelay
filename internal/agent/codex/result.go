package codex

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"

	"github.com/Innocent-children/taskbelay/internal/agent/hookjson"
	"github.com/Innocent-children/taskbelay/internal/event"
)

var commandHeader = regexp.MustCompile(`^(?:Chunk ID: [A-Za-z0-9_-]+\r?\n)?Wall time: [0-9]+(?:\.[0-9]+)? seconds\r?\n(?:Process exited with code (-?[0-9]+)|Exit code: (-?[0-9]+)|Process running with session ID ([0-9]+))\r?\n(?:Original token count: [0-9]+\r?\n)?(?:Output|Final output):(?:\r?\n|$)`)

/**
 * PostToolUse 的字符串可能只是 stdout；带会话路径时核对宿主完成记录。
 * 无路径的旧输入才读取完整宿主头部，不从成功字样或零散退出码推断结果。
 */
func commandResult(in hookjson.Input) *event.ToolResult {
	raw := in.Response
	result := &event.ToolResult{State: event.Unknown, Output: string(raw)}
	var text string
	encoded := json.Unmarshal(raw, &text) == nil
	if encoded {
		result.Output = text
		if in.Transcript != "" {
			// 当前 Codex 的 Bash hook 只给 stdout；不能把 stdout 当作宿主结果头部。
			result.ExitCode = transcriptExitCode(in, text)
			if result.ExitCode != nil {
				result.State = event.Completed
				if *result.ExitCode != 0 {
					result.State = event.Failed
				}
			}
			return result
		}
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

/**
 * 会话格式不是稳定协议，只接受已知的宿主完成事件并逐项核对调用身份与输出。
 * 仅读取文件末尾 4 MiB 的完整行；记录缺失、尚未刷盘或格式变化时保持未知。
 */
func transcriptExitCode(in hookjson.Input, output string) *int {
	if in.SessionID == "" || in.TurnID == "" || in.CallID == "" {
		return nil
	}
	file, err := os.Open(in.Transcript)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	const limit = 4 << 20
	start := max(int64(0), info.Size()-limit)
	data, err := io.ReadAll(io.NewSectionReader(file, start, info.Size()-start))
	if err != nil {
		return nil
	}
	if start > 0 {
		_, data, _ = bytes.Cut(data, []byte{'\n'})
	}
	var code *int
	for len(data) > 0 {
		line, rest, complete := bytes.Cut(data, []byte{'\n'})
		if !complete {
			break
		}
		data = rest
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				Type     string `json:"type"`
				ThreadID string `json:"thread_id"`
				TurnID   string `json:"turn_id"`
				Item     struct {
					Type     string  `json:"type"`
					ID       string  `json:"id"`
					Status   string  `json:"status"`
					Output   *string `json:"aggregated_output"`
					ExitCode *int    `json:"exit_code"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &record) != nil || record.Type != "event_msg" {
			continue
		}
		p := record.Payload
		if p.Type != "item_completed" || p.ThreadID != in.SessionID || p.TurnID != in.TurnID || p.Item.Type != "CommandExecution" || p.Item.ID != in.CallID {
			continue
		}
		item := p.Item
		if item.ExitCode == nil || item.Output == nil || *item.Output != output || (*item.ExitCode == 0 && item.Status != "completed") || (*item.ExitCode != 0 && item.Status != "failed") {
			return nil
		}
		if code != nil && *code != *item.ExitCode {
			return nil
		}
		code = item.ExitCode
	}
	return code
}
