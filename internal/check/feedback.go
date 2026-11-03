package check

import (
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

/**
 * hook 仅发送有界反馈；完整事实留在检查账本供 status 或 task_get 查询。
 * 按字符截断，避免中文 UTF-8 被切断。
 */
func LimitFeedback(text string) string { return shortText(text, 6000) }

func shortText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func compactReport(record ledger.CheckRecord, limit int) string {
	var text strings.Builder
	if record.Static != nil {
		fmt.Fprintln(&text, compactReport(*record.Static, limit))
	}
	conclusion := record.Level
	if conclusion == "pass" {
		conclusion = "已核对范围未发现问题"
	}
	fmt.Fprintf(&text, "TaskBelay %s：%s；任务=%s；%s；检查编号=%d\n", record.Depth, record.Status, record.Input.TaskID, conclusion, record.ID)
	if record.Reused {
		fmt.Fprintln(&text, "复用相同输入的检查，原状态与缺口保持不变。")
	}
	counts := record.GapCounts()
	fmt.Fprintf(&text, "检查缺口：无法判断 %d，错误 %d，未完成 %d。\n", counts["unsupported"], counts["error"], counts["incomplete"])
	levels := map[string]int{}
	for _, file := range record.Report.Coverage {
		levels[file.Level]++
	}
	fmt.Fprintf(&text, "累计变更覆盖：代码分析 %d，文件级 %d，按规则排除 %d。文件级不代表行为或测试风险已检查。\n", levels["code"], levels["file"], levels["excluded"])
	shown := 0
	for _, file := range record.Report.Coverage {
		if file.Level == "file" && shown < limit {
			fmt.Fprintf(&text, "  仅文件级：%s\n", shortText(file.Path, 200))
			shown++
		}
	}
	// 拦截原因优先，防止大量普通提示挤掉本次需要处理的事项。
	shown = 0
	for _, level := range []string{"block", "hint"} {
		for _, issue := range record.Issues {
			if issue.Status != "open" || issue.Level != level || shown >= limit {
				continue
			}
			fmt.Fprintf(&text, "[%s %s] %s：%s；%s\n", issue.Rule, issue.Level, shortText(issue.At, 200), shortText(strings.Join(issue.Facts, " "), 350), shortText(issue.Advice, 250))
			shown++
		}
	}
	for i, gap := range record.Missing {
		if i >= limit {
			break
		}
		fmt.Fprintf(&text, "缺口 [%s] %s %s：%s\n", gap.Kind, gap.Operation, shortText(gap.At, 200), shortText(gap.Reason, 300))
	}
	unknown := 0
	for _, risk := range record.Report.TestRisk {
		if !risk.Complete {
			unknown++
		}
	}
	if unknown > 0 {
		fmt.Fprintf(&text, "%d 项需求的风险未完整核对，不提供测试数量建议。\n", unknown)
	}
	if len(record.Unconfirmed) > 0 {
		fmt.Fprintf(&text, "%d 个历史问题本次尚未确认，不作为当前拦截依据。\n", len(record.Unconfirmed))
	}
	for i, reason := range record.Report.NotRun {
		if i >= limit {
			break
		}
		fmt.Fprintf(&text, "未运行：%s\n", shortText(reason, 300))
	}
	return shortText(strings.TrimSpace(text.String()), 5800) + "\n完整记录：taskbelay status 或 task_get（checks）。"
}
