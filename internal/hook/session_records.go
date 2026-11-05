package hook

import (
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

func (t Tasks) recordSession(actor ledger.Actor, record event.SessionRecord) {
	if err := t.Store.AppendSessionRecord(t.Workspace.ID, actor, summary.SessionRecord(t.Workspace.Root, actor, record)); err != nil && t.ReportError != nil {
		t.ReportError(fmt.Errorf("会话记录未完成：%w", err))
	}
}

func (t Tasks) recordCheck(actor ledger.Actor, record ledger.CheckRecord) {
	if record.Static != nil {
		t.recordCheck(actor, *record.Static)
	}
	action := "检查已执行"
	if record.Reused {
		action = "检查结果已复用"
	}
	if record.ID == 0 {
		action = "检查结果未知"
	}
	counts := record.GapCounts()
	t.recordSession(actor, event.SessionRecord{TaskID: record.Input.TaskID, Action: action, Object: record.Depth + " 检查", Detail: fmt.Sprintf("%s；检查状态 %s；结论 %s；未知 %d，出错 %d，未完成 %d。", action, record.Status, record.Level, counts["unknown"], counts["error"], counts["incomplete"]), Sources: checkSource(record)})
}

func feedbackRecord(task, action, object, detail string, sources ...string) event.SessionRecord {
	return event.SessionRecord{TaskID: task, Action: action, Object: object, Detail: detail, Sources: sources}
}

func verificationRecord(record ledger.VerificationRecord) event.SessionRecord {
	state := map[string]string{"running": "运行中", "passed": "成功", "failed": "失败", "interrupted": "已中断", "unknown": "未知"}[record.State]
	if state == "" {
		state = "未知"
	}
	exit := "退出结果未知"
	if record.ExitCode != nil {
		exit = fmt.Sprintf("退出码 %d", *record.ExitCode)
	}
	return feedbackRecord(record.TaskID, "已观察到验证结果", verificationCommand(record.Command), "状态："+state+"；"+exit+"。", "verification:"+record.ID)
}

/**
 * Display 只保证环境值脱敏，参数和同一 shell 的其他命令仍可能含凭据。
 * 会话记录只保留已识别的验证工具名，具体调用由验证编号关联。
 */
func verificationCommand(script shell.Script) string {
	var names []string
	seen := map[string]bool{}
	for _, segment := range script.Segments {
		if len(segment.Args) == 0 {
			continue
		}
		name := shell.Base(segment.Args[0])
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	if len(names) == 0 {
		return "验证命令（参数已省略）"
	}
	return strings.Join(names, "、") + " 验证命令（参数和其他命令已省略）"
}

func editRecords(task string, record ledger.EditRecord) []event.SessionRecord {
	result := record.Decision.Records
	if record.PlanCheck != nil {
		for _, issue := range record.PlanCheck.Issues {
			action := "编辑检查提示已输出"
			if record.PlanCheck.Blocked {
				action = "编辑拒绝已输出"
			}
			result = append(result, feedbackRecord(task, action, issue.At, issue.Rule+"："+issue.Advice, issueSource(issue)...))
		}
	}
	return result
}

func checkFeedback(record ledger.CheckRecord, blocked []ledger.Issue) []event.SessionRecord {
	action := "检查反馈已输出"
	decision := "本次提供提示"
	if len(blocked) > 0 {
		decision = "本次已阻止结束"
	}
	records := []event.SessionRecord{feedbackRecord(record.Input.TaskID, action, record.Depth+" 检查", "已反馈检查状态："+string(record.Status)+"；结论："+record.Level+"；"+decision+"。", checkSource(record)...)}
	seen := map[string]bool{}
	hints := 0
	for _, issue := range record.Issues {
		key := issue.Rule + "\x00" + issue.At + "\x00" + issue.FactKey
		if seen[key] || issue.Status != "open" {
			continue
		}
		seen[key] = true
		if issue.Level != "block" {
			if hints >= 5 {
				continue
			}
			hints++
		}
		records = append(records, feedbackRecord(record.Input.TaskID, action, issue.At, issue.Rule+"："+strings.Join(issue.Facts, " ")+"；"+issue.Advice, append(checkSource(record), issueSource(issue)...)...))
	}
	return records
}

func checkSource(record ledger.CheckRecord) []string {
	if record.ID == 0 {
		return nil
	}
	return []string{fmt.Sprintf("check:%d", record.ID)}
}
func issueSource(issue ledger.Issue) []string {
	if issue.ID == "" {
		return nil
	}
	return []string{"issue:" + issue.ID}
}
