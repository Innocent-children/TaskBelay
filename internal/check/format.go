package check

import (
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"sort"
	"strings"
)

func LimitFeedback(text string) string {
	r := []rune(text)
	if len(r) > FeedbackBudget {
		return string(r[:FeedbackBudget-len([]rune("\n其余内容见 task_get。"))]) + "\n其余内容见 task_get。"
	}
	return text
}

func Format(record ledger.CheckRecord, limit int) string {
	var b strings.Builder
	var selected []ledger.Issue
	blocks := 0
	for _, i := range record.Issues {
		if limit == 0 || i.Selected || i.BlockOnDelivery || i.Level == "block" {
			selected = append(selected, i)
		}
		if i.Level == "block" {
			blocks++
		}
	}
	depth := "交付检查"
	if record.Depth == "tests" {
		depth = "动态检查"
	}
	fmt.Fprintf(&b, "TaskBelay %s：%d 条提示", depth, len(record.Issues))
	if blocks > 0 {
		fmt.Fprintf(&b, "，其中 %d 处拦截", blocks)
	}
	if limit > 0 && len(selected) > limit {
		fmt.Fprintf(&b, "，以下列出最值得核对的 %d 条", limit)
	}
	fmt.Fprintf(&b, "。检查状态 %s；输入版本 %s\n", record.Status, record.InputVersion)
	if record.HistoricalVersion != "" {
		fmt.Fprintf(&b, "历史规则版本：%s（不参与当前判定）\n", record.HistoricalVersion)
	}
	if issues := FormatIssues(selected, limit); issues != "" {
		b.WriteString(issues + "\n")
	}
	counts := record.GapCounts()
	if len(record.Missing) > 0 {
		// 无法判断的部分只给数量和少量示例，完整清单在检查记录里。
		fmt.Fprintf(&b, "无法判断 %d 项；出错 %d 项；未完成 %d 项", counts["unknown"], counts["error"], counts["incomplete"])
		shown := len(record.Missing)
		if limit > 0 && shown > GapExamples {
			shown = GapExamples
		}
		for i, gap := range record.Missing[:shown] {
			if i == 0 {
				b.WriteString("。例如：")
			} else {
				b.WriteString("；")
			}
			b.WriteString(strings.TrimSpace(gap.At + " " + gap.Reason))
		}
		b.WriteString("。这些部分不能当作已通过。\n")
	}
	if len(record.Issues) == 0 {
		b.WriteString("本次没有给出建议，但仍不能据此认定改动都必要或测试已充分。\n")
	}
	if d := record.Report.Dynamic; d != nil {
		fmt.Fprintf(&b, "副本检查状态：%s；运行 %d 次；撤回集合是否整体确认通过：%t。\n", d.State, d.Runs, d.Confirmed)
	}
	for _, v := range record.Report.Verifications {
		if v.Algorithm == Algorithm && v.Scope.Validation {
			fmt.Fprintf(&b, "验证 %s：%s\n", v.ID, v.State)
		}
	}
	return strings.TrimSpace(b.String())
}

var ruleNames = map[string]string{
	"unrelated": "与需求的联系未找到", "noise": "只改了空白、注释或名字", "external": "对外新增项",
	"abstraction": "只有一个实现的抽象", "parameter": "取值固定的参数", "forwarding": "转发层", "old_name": "旧名兼容",
	"duplicate": "重复实现", "unused": "没人使用的代码", "recovery": "恢复处理", "mock_only": "只检查了 mock 调用",
	"assertion": "没有检查结果的用例", "test_equivalence": "相近用例", "test_budget": "超出测试预算", "test_gap": "测试缺口",
	"validation_repeat": "重复验证", "validation_contained": "范围已被覆盖的验证", "validation_implication": "可能没有新信息的验证", "validation_comments": "只改注释后的验证",
}

/**
 * FormatIssues 每条提示占三行：对象和类别、已知事实、下一步动作。
 * 拦截排在前面，可以直接动手的建议排在只报告现状的提示前面。
 */
func FormatIssues(issues []ledger.Issue, limit int) string {
	var open []ledger.Issue
	for _, i := range issues {
		if i.Status == "open" || i.Status == "retained" {
			open = append(open, i)
		}
	}
	rank := func(i ledger.Issue) int {
		switch {
		case i.Level == "block":
			return 0
		case strings.HasPrefix(i.Advice, "保留当前内容"):
			return 2
		}
		return 1
	}
	sort.SliceStable(open, func(a, b int) bool { return rank(open[a]) < rank(open[b]) })
	// 只报告现状的提示没有可执行的动作，限量展示时只给出数量。
	kept := 0
	if limit > 0 {
		for len(open) > 0 && rank(open[len(open)-1]) == 2 {
			open = open[:len(open)-1]
			kept++
		}
		if len(open) > limit {
			open = open[:limit]
		}
	}
	var b strings.Builder
	for _, i := range open {
		level := "建议"
		if i.Level == "block" {
			level = "拦截"
		}
		name := ruleNames[i.Rule]
		if name == "" {
			name = i.Rule
		}
		fmt.Fprintf(&b, "[%s] %s：%s（编号 %s）\n  %s\n  → %s\n", level, name, i.At, i.ID, strings.Join(i.Facts, "；"), i.Advice)
	}
	if kept > 0 {
		fmt.Fprintf(&b, "另有 %d 条涉及工作区更新或来源无法核对的内容，只记录现状，保留当前内容即可。\n", kept)
	}
	return strings.TrimSpace(b.String())
}
func FormatPlans(p ledger.PlanView) string {
	if p.Effective == nil && p.Pending == nil {
		return ""
	}
	var b strings.Builder
	if p.Effective != nil {
		fmt.Fprintf(&b, "有效方案：%s %s\n", p.Effective.ID, p.Effective.Path)
	}
	if p.Pending != nil {
		fmt.Fprintf(&b, "待确认方案：%s %s\n", p.Pending.ID, p.Pending.Path)
	}
	return b.String()
}

/**
 * FormatVerification 生成命令前的验证反馈。拦截时先说明命令没有运行，再给出上次输出和重跑方式；
 * 输出过长时保留开头和结尾，测试汇总通常在结尾。
 */
func FormatVerification(r ledger.VerificationRecord) string {
	if r.Issue == nil && r.Scope.Suggestion == "" {
		return ""
	}
	var b strings.Builder
	if r.Issue == nil {
		fmt.Fprintf(&b, "TaskBelay 提示（%s）：%s", r.Command.Display, r.Scope.Suggestion)
		return LimitFeedback(b.String())
	}
	reason, previous, output := "", r.PreviousID, ""
	for i, fact := range r.Issue.Facts {
		if rest, ok := strings.CutPrefix(fact, "上次记录："); ok {
			previous = rest
		} else if rest, ok := strings.CutPrefix(fact, "上次输出："); ok {
			output = rest
		} else if i == 0 {
			reason = fact
		}
	}
	if r.Denied {
		fmt.Fprintf(&b, "TaskBelay 没有运行这条命令：%s\n", r.Command.Display)
		b.WriteString("代码、命令、环境变量和工具都与上次通过时相同，再运行一次不会得到新信息。直接使用下面的上次结果。\n")
	} else {
		fmt.Fprintf(&b, "TaskBelay 提示（%s）：%s\n", r.Command.Display, reason)
	}
	if previous != "" {
		fmt.Fprintf(&b, "上次输出（记录 %s）：\n%s\n", previous, clipOutput(output))
	}
	if r.Denied {
		fmt.Fprintf(&b, "外部服务、数据或环境确有变化、需要重跑时，先调用 note_add 写明理由（kind 为 decision，target 的 kind 为 verification、id 为 %s、effect 为 rerun_once、scope 为 once），再运行同一条命令。", previous)
	}
	return LimitFeedback(b.String())
}

func clipOutput(output string) string {
	r := []rune(strings.TrimRight(output, "\n"))
	if len(r) <= OutputHead+OutputTail {
		return string(r)
	}
	return string(r[:OutputHead]) + "\n……（中间省略）……\n" + string(r[len(r)-OutputTail:])
}
