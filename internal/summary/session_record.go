package summary

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

/**
 * SessionRecord 只格式化调用方已经取得的事实，不读取账本或工具正文。
 */
func SessionRecord(workspace string, actor ledger.Actor, record event.SessionRecord) string {
	if record.At.IsZero() {
		record.At = time.Now()
	}
	task := record.TaskID
	if task == "" {
		task = "未关联"
	}
	sources := append([]string{}, record.Sources...)
	if actor.RoundID != "" {
		sources = append(sources, "轮次："+actor.RoundID)
	}
	if actor.CallID != "" {
		sources = append(sources, "工具调用："+actor.CallID)
	}
	text := fmt.Sprintf("\n## %s · %s\n\n- 工作区：%s\n- 会话：%s / %s；任务：%s\n- 对象：%s\n- 动作：%s\n", record.At.Format(time.RFC3339Nano), recordBrief(record.Action), recordText(workspace), recordText(actor.Agent), recordText(actor.SessionID), recordText(task), recordText(record.Object), recordBrief(record.Detail))
	if len(sources) > 0 {
		text += "- 依据：" + recordText(strings.Join(sources, "；")) + "\n"
	}
	return text
}

func recordText(value string) string {
	value = strings.Join(strings.FieldsFunc(value, unicode.IsControl), " ")
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "`", "\\`", "[", "\\[", "]", "\\]").Replace(value)
}

func recordBrief(value string) string {
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240]) + "…"
	}
	return recordText(value)
}
