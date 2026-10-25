package check

import (
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func FormatIssues(issues []ledger.Issue, hintLimit int) string {
	var text strings.Builder
	hints, omitted := 0, 0
	seen := map[string]bool{}
	for _, issue := range issues {
		key := issue.Rule + "\x00" + issue.At + "\x00" + issue.FactKey
		if seen[key] || issue.Status != "open" {
			continue
		}
		seen[key] = true
		if issue.Level != "block" {
			if hintLimit > 0 && hints >= hintLimit {
				omitted++
				continue
			}
			hints++
		}
		fmt.Fprintf(&text, "[%s %s] %s；状态=%s；编号=%s\n  %s\n  %s\n", issue.Rule, issue.Level, issue.At, issue.Status, issue.ID, strings.Join(issue.Facts, " "), issue.Advice)
		if len(issue.Evidence) > 0 {
			fmt.Fprintln(&text, "  依据："+strings.Join(issue.Evidence, "; "))
		}
	}
	if omitted > 0 {
		fmt.Fprintf(&text, "另有 %d 条提示，可用 taskbelay status 查看。\n", omitted)
	}
	return strings.TrimSpace(text.String())
}

func Format(record ledger.CheckRecord, hintLimit int) string {
	var text strings.Builder
	if record.Static != nil {
		fmt.Fprintln(&text, Format(*record.Static, hintLimit))
	}
	fmt.Fprintf(&text, "TaskBelay %s 检查：%s；任务=%s；结论=%s；输入=%s\n", record.Depth, record.Status, record.Input.TaskID, record.Level, record.InputVersion)
	fmt.Fprintf(&text, "检查时间：%s 至 %s\n", record.StartedAt.Format("2006-01-02T15:04:05Z07:00"), record.FinishedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintln(&text, FormatIssues(record.Issues, hintLimit))
	fmt.Fprint(&text, FormatPlans(record.Report.Plans))
	if dynamic := record.Report.Dynamic; dynamic != nil {
		fmt.Fprintf(&text, "动态执行：完整=%t；耗时=%s；环境可复用=%t\n", dynamic.Complete, dynamic.Duration, dynamic.Reusable)
		counts := map[string]int{}
		for _, stage := range dynamic.Stages {
			counts[stage.Name]++
		}
		fmt.Fprintf(&text, "  命令次数：准备=%d，编译=%d，枚举=%d，基线=%d，变异=%d\n", counts["prepare"], counts["compile"], counts["enumerate"], counts["baseline"], counts["mutation"])
		if hintLimit == 0 {
			for _, stage := range dynamic.Stages {
				fmt.Fprintf(&text, "  执行阶段：%s；成功=%t；耗时=%s；失败类别=%s；%s\n", stage.Name, stage.Result.Passed(), stage.Result.Duration, stage.Result.Failure, stage.Result.Reason)
			}
		}
		for _, observation := range dynamic.Observations {
			fmt.Fprintf(&text, "动态用例：%s；基线=%t；失败类别=%s；有效覆盖=%t；执行副本=%t；发现=%s\n", observation.Case.At, observation.Baseline.Passed(), observation.Baseline.Failure, observation.Baseline.Coverage.Valid, observation.Baseline.Coverage.CopyExecuted, strings.Join(observation.Killed, ", "))
			for _, assertion := range observation.Case.Assertions {
				fmt.Fprintln(&text, "  断言："+assertion)
			}
			for _, unknown := range observation.Case.Unknown {
				fmt.Fprintln(&text, "  待核对："+unknown)
			}
		}
		for _, group := range dynamic.Groups {
			fmt.Fprintf(&text, "变异分组 %s：用例=%s；完整=%t；%s\n", group.Requirement, strings.Join(group.Cases, ", "), group.Complete, group.Reason)
			var lines []string
			for _, line := range group.Coverage {
				lines = append(lines, fmt.Sprintf("%s:%d", line.File, line.Number))
			}
			fmt.Fprintln(&text, "  共同覆盖："+strings.Join(lines, ", "))
			for _, mutation := range group.Mutations {
				fmt.Fprintf(&text, "  变异 %s：%s:%d %s → %s\n", mutation.ID, mutation.File, mutation.Line, mutation.Before, mutation.After)
			}
		}
	}
	for _, risk := range record.Report.TestRisk {
		fmt.Fprintf(&text, "测试投入 %s：新增 %d（用户保留 %d）、修改 %d、既有 %d；风险建议 %d，风险核对完整=%t。建议数不是上限。\n", risk.Requirement, risk.Added, risk.Retained, risk.Modified, risk.Existing, risk.Recommended, risk.Complete)
		for _, source := range risk.Sources {
			fmt.Fprintf(&text, "  风险：%s %s:%d %s\n", source.Kind, source.At, source.Line, source.Evidence)
		}
	}
	for _, test := range record.Report.Tests {
		if test.Change == "existing" && len(test.Requirements) == 0 {
			continue
		}
		fmt.Fprintf(&text, "用例：%s [%s]；需求=%s；混合或未知来源=%t；用户保留=%t\n", test.At, test.Change, strings.Join(test.Requirements, ", "), test.Mixed, test.Retained)
		for _, assertion := range test.Assertions {
			fmt.Fprintf(&text, "  直接断言：%s 第 %d 行 %s\n", assertion.Kind, assertion.Line, assertion.Evidence)
		}
		for _, unknown := range test.Unknown {
			fmt.Fprintln(&text, "  待核对："+unknown)
		}
	}
	for _, note := range record.Report.ValidationNotes {
		fmt.Fprintf(&text, "验证或用途说明：%s %s；记录时观察点=%d，需按当前内容核对\n", note.ID, note.Text, note.ObservationID)
	}
	for _, issue := range record.Issues {
		if issue.Status == "retained" || issue.Status == "convention" {
			fmt.Fprintf(&text, "[%s 已处理] %s；%s\n", issue.Rule, issue.At, strings.Join(issue.HandledBy, "; "))
		}
	}
	for _, req := range record.Report.Requirements {
		fmt.Fprintf(&text, "%s [%s] %s\n", req.ID, req.Status, req.Content)
		for _, location := range req.ImplementedBy {
			fmt.Fprintf(&text, "  %s %s；%s\n", location.Change, location.At, location.Purpose)
			if location.Pending != "" {
				fmt.Fprintln(&text, "  待核对："+location.Pending)
			}
		}
		for _, change := range record.Report.ChangesByRequirement[req.ID] {
			fmt.Fprintln(&text, "  累计改动："+change)
		}
	}
	if len(record.Report.Unfinished) > 0 {
		var ids []string
		for _, req := range record.Report.Unfinished {
			ids = append(ids, req.ID)
		}
		fmt.Fprintln(&text, "未完成需求："+strings.Join(ids, ", "))
	}
	for _, req := range record.Report.Revoked {
		fmt.Fprintf(&text, "已撤销需求：%s %s\n", req.ID, req.Content)
	}
	for _, change := range record.Report.Changes {
		fmt.Fprintln(&text, "累计改动："+change)
	}
	for _, note := range record.Report.Decisions {
		fmt.Fprintf(&text, "用户决定：%s %s\n", note.ID, note.Text)
	}
	for _, update := range record.Report.WorkspaceUpdates {
		fmt.Fprintf(&text, "工作区更新：%s；观察点=%d，保留当前内容\n", update.Path, update.AfterObservation)
	}
	missingLimit := len(record.Missing)
	if hintLimit > 0 {
		missingLimit = min(missingLimit, hintLimit)
	}
	for _, missing := range record.Missing[:missingLimit] {
		fmt.Fprintf(&text, "未完成检查：%s %s；%s\n", missing.Operation, missing.At, missing.Reason)
	}
	if missingLimit < len(record.Missing) {
		fmt.Fprintf(&text, "另有 %d 项未完成检查，可用 taskbelay status 查看。\n", len(record.Missing)-missingLimit)
	}
	for _, notRun := range record.Report.NotRun {
		fmt.Fprintln(&text, "未运行："+notRun)
	}
	for _, verification := range record.Report.Verifications {
		state := verification.State
		if verification.Denied {
			state = "被拦截，未执行"
		}
		fmt.Fprintf(&text, "验证：%s %s；%s\n", verification.ID, state, verification.Command.Display)
	}
	return strings.TrimSpace(text.String())
}

func FormatPlans(plans ledger.PlanView) string {
	var text strings.Builder
	if len(plans.Versions) > 0 && plans.Effective == nil {
		fmt.Fprintln(&text, "方案：当前没有有效确认范围，历史版本仍保留。")
	}
	if plans.Effective != nil {
		fmt.Fprintf(&text, "有效方案：v%d；%s；授权时间=%s\n", plans.Effective.Version, plans.Effective.Path, plans.Effective.Confirmation.AuthorizedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if plans.Pending != nil {
		fmt.Fprintf(&text, "待确认增补：v%d；%s\n", plans.Pending.Version, plans.Pending.Path)
	}
	return text.String()
}
