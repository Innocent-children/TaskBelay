package check

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/shell"
)

func Verification(current ledger.VerificationRecord, previous *ledger.VerificationRecord) *ledger.Issue {
	if previous == nil {
		return nil
	}
	issue := &ledger.Issue{Rule: "V1", Level: "hint", At: current.CommandKey, Status: "open", Advice: "按当前输入和运行条件判断是否需要重跑；可用 note_add 指向上一条验证并记录一次重跑原因。", Evidence: []string{previous.ID}}
	issue.FactKey = fmt.Sprintf("%x", sha256.Sum256([]byte(current.Tree+current.Scope.InputID+current.Scope.ToolID+current.EnvironmentID+current.Scope.EnvironmentID)))
	issue.Facts = []string{"命令：" + current.Command.Display, "上一条相同验证 " + previous.ID + " 的结果为 " + previous.State + "。"}
	if previous.Denied || !previous.Executed || previous.State != "passed" {
		return issue
	}
	if current.Command.Mixed() || previous.Command.Mixed() {
		issue.Facts = append(issue.Facts, "调用含有其他命令或无法核对的执行结构，不复用整次调用。")
		return issue
	}
	for _, segment := range current.Command.Segments {
		if segment.Kind == shell.Verification && !shell.ReusableCommand(segment) {
			issue.Facts = append(issue.Facts, "命令有副作用、后台执行或未解析参数，这次照常运行。")
			return issue
		}
	}
	if current.Scope.Unstable || previous.Scope.Unstable {
		issue.Facts = append(issue.Facts, "正在核对不稳定结果，这次照常运行。")
		return issue
	}
	if current.Scope.SideEffects || previous.Scope.SideEffects {
		issue.Facts = append(issue.Facts, "Runner 确认命令还有其他动作，这次照常运行。")
		return issue
	}
	if current.Tree == "" || current.Tree != previous.Tree || current.Scope.InputID != previous.Scope.InputID || current.EnvironmentID != previous.EnvironmentID || current.Scope.ToolID != previous.Scope.ToolID || current.Scope.EnvironmentID != previous.Scope.EnvironmentID {
		issue.Facts = append(issue.Facts, "工作区、输入、工具或环境已有变化，这次照常运行。")
		return issue
	}
	if !current.Scope.Complete || !previous.Scope.Complete || !current.Scope.EnvironmentKnown || !previous.Scope.EnvironmentKnown || current.Scope.InputID == "" || current.Scope.ToolID == "" || current.EnvironmentID == "" || current.Scope.EnvironmentID == "" {
		issue.Facts = append(issue.Facts, "尚不能完整核对输入范围和运行环境，只提示上次结果。")
		return issue
	}
	issue.Level = "block"
	issue.Facts = append(issue.Facts, "同一 session 的上次验证已通过；目录、参数、工作区、Runner 输入、工具和环境均相同，可复用通过结果。")
	return issue
}

func FormatVerification(record ledger.VerificationRecord) string {
	var lines []string
	if record.Issue != nil {
		lines = append(lines, FormatIssues([]ledger.Issue{*record.Issue}, 0))
	}
	if record.Scope.Suggestion != "" {
		lines = append(lines, "[V2 提示] "+record.Scope.Suggestion+"；项目全量验收约定、共享配置和未覆盖的动态依赖仍需按实际要求处理。")
	}
	if record.RerunNoteID != "" {
		lines = append(lines, "本次采用一次重跑决定 "+record.RerunNoteID+"，原命令照常运行。")
	}
	return strings.Join(lines, "\n")
}
