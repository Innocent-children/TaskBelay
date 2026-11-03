package check

import (
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/ledger"
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
	fmt.Fprintf(&b, "TaskBelay %s：处理状态 %s；结论 %s；输入 %s\n", record.Depth, record.Status, record.Level, record.InputVersion)
	if record.HistoricalVersion != "" {
		fmt.Fprintf(&b, "历史规则版本：%s（不参与当前判定）\n", record.HistoricalVersion)
	}
	counts := record.GapCounts()
	fmt.Fprintf(&b, "未知 %d；出错 %d；未完成 %d。\n", counts["unknown"], counts["error"], counts["incomplete"])
	var selected []ledger.Issue
	for _, i := range record.Issues {
		if limit == 0 || i.Selected || i.BlockOnDelivery || i.Level == "block" {
			selected = append(selected, i)
		}
	}
	b.WriteString(FormatIssues(selected, limit))
	for i, gap := range record.Missing {
		if limit > 0 && i >= limit {
			break
		}
		fmt.Fprintf(&b, "%s %s：%s\n", gap.Kind, gap.At, gap.Reason)
	}
	if len(record.Issues) == 0 {
		b.WriteString("本次未形成建议；这不表示已证明没有多余改动或测试足够。\n")
	}
	if d := record.Report.Dynamic; d != nil {
		fmt.Fprintf(&b, "副本：%s；运行 %d 次；整组确认 %t。\n", d.State, d.Runs, d.Confirmed)
	}
	for _, v := range record.Report.Verifications {
		if v.Algorithm == Algorithm && v.Scope.Validation {
			fmt.Fprintf(&b, "验证 %s：%s\n", v.ID, v.State)
		}
	}
	return strings.TrimSpace(b.String())
}
func FormatIssues(issues []ledger.Issue, limit int) string {
	var b strings.Builder
	n := 0
	for _, i := range issues {
		if i.Status != "open" && i.Status != "retained" {
			continue
		}
		if limit > 0 && n >= limit {
			break
		}
		n++
		fmt.Fprintf(&b, "[%s %s] %s；编号=%s；状态=%s；可信度=%.3f\n%s\n%s\n", i.Rule, i.Level, i.At, i.ID, i.Status, i.Confidence, strings.Join(i.Facts, "；"), i.Advice)
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
func FormatVerification(r ledger.VerificationRecord) string {
	if r.Issue == nil && r.Scope.Suggestion == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "验证 %s：%s\n", r.ID, r.Command.Display)
	if r.Issue != nil {
		b.WriteString(FormatIssues([]ledger.Issue{*r.Issue}, 0))
		if r.PreviousID != "" {
			fmt.Fprintf(&b, "\n上次记录：%s\n", r.PreviousID)
		}
		b.WriteString(r.Issue.Advice)
	}
	if r.Scope.Suggestion != "" {
		b.WriteString("\n" + r.Scope.Suggestion)
	}
	return LimitFeedback(b.String())
}
