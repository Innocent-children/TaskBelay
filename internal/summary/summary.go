package summary

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

const Budget = 2000
const GuidanceBudget = 350
const Footer = "用 task_get 查看完整任务；在编辑工作区更新涉及的位置前，先读当前内容。"

type Input struct {
	View            ledger.TaskView
	Observation     ledger.Observation
	ContentRevision int64
	ChangedFiles    []string
	NextStepChanged bool
	Peers           []ledger.SessionActivity
	Sessionless     bool
	Partial         bool
}

/**
 * 用四分之一 token 为单位累计：ASCII 每四个字符一个，其余 Unicode 字符
 * 每字一个。截断按 rune 边界进行，不破坏中文或非 BMP 字符。
 */
func Tokens(text string) int {
	units := 0
	for _, r := range text {
		units++
		if r > 127 {
			units += 3
		}
	}
	return (units + 3) / 4
}

func Truncate(text string, budget int) string {
	if budget <= 0 {
		return ""
	}
	if Tokens(text) <= budget {
		return text
	}
	units, end := 0, 0
	for i, r := range text {
		cost := 1
		if r > 127 {
			cost = 4
		}
		if units+cost > (budget-1)*4 {
			break
		}
		units += cost
		end = i + len(string(r))
	}
	return text[:end] + "…"
}

func oneLine(text string) string { return strings.Join(strings.Fields(text), " ") }

func stamp(at time.Time) string {
	if at.IsZero() {
		return "时间未记录"
	}
	return at.UTC().Format("2006-01-02 15:04:05Z")
}

func taskState(state ledger.TaskState) string {
	switch state {
	case ledger.TaskActive:
		return "进行中"
	case ledger.TaskPending:
		return "待交付"
	case ledger.TaskPaused:
		return "已暂停"
	case ledger.TaskCompleted:
		return "已完成"
	case ledger.TaskCanceled:
		return "已中止"
	}
	return "状态未知"
}

func requirementState(state string) string {
	switch state {
	case "pending":
		return "未开始"
	case "in_progress":
		return "进行中"
	case "completed":
		return "已完成"
	case "revoked":
		return "已撤销"
	}
	return "状态未知"
}

func workflow(settings ledger.EffectiveSettings) string {
	if settings.Values["workflow"].Value == "semi" {
		return "半自动"
	}
	return "全自动"
}

func Guidance(settings ledger.EffectiveSettings) string {
	mode := "按当前需求进行全自动开发。"
	if settings.Values["workflow"].Value == "semi" {
		mode = "半自动先用 plan_save 提交完整方案，告诉用户要点、变更摘要和文件路径；明确确认后用 plan_confirm 记录原话。讨论或否定保持待确认，附带修改一并核对；已确认范围内可继续。"
	}
	enforcement := "过期编辑按设置退回，先读取当前内容后重新组织编辑。"
	if settings.Values["enforcement"].Value == "hint" {
		enforcement = "当前为只提示模式，收到冲突提示后先读取当前内容。"
	}
	tests := "关闭"
	if settings.Values["analysis.tests"].Value == "on" {
		tests = "开启"
	}
	return Truncate("事前提示："+mode+"开发前用 requirement_save 保存需求，完成、修改或撤销时更新状态；用 note_add 记录决定和未完成的下一步。工作区更新不新增授权，不恢复旧代码覆盖当前修改。"+enforcement+"需要可核对编辑记录时使用编辑工具；shell 写入按实际取得的信息记录。动态测试设置："+tests+"；未知或未完成的检查不能当作通过。", GuidanceBudget)
}

func GuidanceVersion(settings ledger.EffectiveSettings) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(Guidance(settings))))
}

/**
 * 观察编号、活动时间和新消息本身不产生重复摘要。任务修订、实际内容修订、
 * 分支/HEAD、有效设置、仍有效的决定、检查事实和活跃 session 集合才影响输入身份。
 */
func Version(input Input) string {
	type taskVersion struct {
		ID       string
		Revision int64
	}
	var task taskVersion
	if input.View.Task != nil {
		task = taskVersion{input.View.Task.ID, input.View.Task.Revision}
	}
	var candidates []taskVersion
	if input.View.Task == nil {
		for _, candidate := range Candidates(input) {
			candidates = append(candidates, taskVersion{candidate.ID, candidate.Revision})
		}
	}
	var decisions []string
	for _, note := range input.View.EffectiveDecisions {
		decisions = append(decisions, note.ID)
	}
	type issueVersion struct {
		ID, Rule, At, FactKey, Status string
		Facts                         []string
	}
	type checkVersion struct {
		Depth, Input string
		Status       ledger.CheckStatus
		Issues       []issueVersion
		Unconfirmed  []issueVersion
		Missing      []ledger.CheckGap
	}
	var checks []checkVersion
	for _, record := range input.View.Checks {
		check := checkVersion{Depth: record.Depth, Input: record.InputVersion, Status: record.Status, Missing: record.Missing}
		for _, issue := range record.Issues {
			check.Issues = append(check.Issues, issueVersion{issue.ID, issue.Rule, issue.At, issue.FactKey, issue.Status, issue.Facts})
		}
		for _, issue := range record.Unconfirmed {
			check.Unconfirmed = append(check.Unconfirmed, issueVersion{issue.ID, issue.Rule, issue.At, issue.FactKey, issue.Status, issue.Facts})
		}
		checks = append(checks, check)
	}
	type verificationVersion struct {
		ID, State        string
		Denied, Executed bool
	}
	var verifications []verificationVersion
	for _, record := range input.View.Verifications {
		verifications = append(verifications, verificationVersion{record.ID, record.State, record.Denied, record.Executed})
	}
	data, _ := json.Marshal(struct {
		Task           taskVersion
		Candidates     []taskVersion
		Content        int64
		Branch         string
		HEAD           string
		Settings       string
		Guidance       string
		Decisions      []string
		Peers          []ledger.SessionActivity
		Check          string
		Checks         []checkVersion
		Verifications  []verificationVersion
		Partial        bool
		PendingActions []string
	}{task, candidates, input.ContentRevision, input.Observation.Context.Branch, input.Observation.Context.HEAD, input.View.Settings.Revision, GuidanceVersion(input.View.Settings), decisions, input.Peers, input.View.LatestCheck, checks, verifications, input.Partial, input.View.PendingActions})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func Candidates(input Input) []ledger.TaskRecord {
	var candidates []ledger.TaskRecord
	for _, task := range input.View.Candidates {
		if task.ID != input.View.CurrentTaskID && task.State != ledger.TaskCanceled {
			candidates = append(candidates, task)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		matchingA, matchingB := a.Context.Branch == input.Observation.Context.Branch, b.Context.Branch == input.Observation.Context.Branch
		if matchingA != matchingB {
			return matchingA
		}
		if a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.ID < b.ID
		}
		return a.UpdatedAt.After(b.UpdatedAt)
	})
	return candidates
}

func NextStep(view ledger.TaskView) *ledger.Note {
	var latest *ledger.Note
	for _, note := range view.Notes {
		if note.Kind == "next_step" && (latest == nil || !note.At.Before(latest.At)) {
			copy := note
			latest = &copy
		}
	}
	return latest
}

func requirements(view ledger.TaskView) string {
	items := append([]ledger.Requirement(nil), view.Requirements...)
	unfinished := func(item ledger.Requirement) bool { return item.Status == "pending" || item.Status == "in_progress" }
	sort.SliceStable(items, func(i, j int) bool {
		if unfinished(items[i]) != unfinished(items[j]) {
			return unfinished(items[i])
		}
		return items[i].At.After(items[j].At)
	})
	var lines []string
	for _, item := range items {
		if len(lines) == 8 {
			break
		}
		line := item.ID + " " + requirementState(item.Status) + "：" + Truncate(oneLine(item.Content), 65)
		if Tokens(strings.Join(lines, "\n")+"\n"+line) > 450 {
			break
		}
		lines = append(lines, line)
	}
	return Truncate(fmt.Sprintf("当前需求：共 %d 条，省略 %d 条。\n", len(items), len(items)-len(lines))+strings.Join(lines, "\n"), 500)
}

func updates(view ledger.TaskView) string {
	items := append([]ledger.FileChange(nil), view.WorkspaceUpdates...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].AfterObservation == items[j].AfterObservation {
			return items[i].Path < items[j].Path
		}
		return items[i].AfterObservation > items[j].AfterObservation
	})
	seen := map[string]bool{}
	lines := []string{"最新工作区更新（先读当前内容）："}
	for _, item := range items {
		if seen[item.Path] {
			continue
		}
		seen[item.Path] = true
		if len(lines) == 6 {
			break
		}
		position := oneLine(item.Path)
		if item.After.Blob == "" {
			position += "（已删除）"
		} else if len(item.Fragments) > 0 {
			position += fmt.Sprintf(":%d", max(1, item.Fragments[0].AfterStart))
		}
		if Tokens(position) > 35 {
			position = "位置较长，请用 task_get 查看"
		}
		lines = append(lines, position+"；观察于 "+stamp(item.At))
	}
	if len(lines) == 1 {
		return "工作区更新：暂无记录。"
	}
	return Truncate(strings.Join(lines, "\n"), 250)
}

func Build(input Input, guidance bool) string {
	view := input.View
	parts := make([]string, 7)
	if view.Task == nil {
		parts[0] = "当前没有任务；工作模式：" + workflow(view.Settings) + "。"
		lines := []string{"可恢复的最近任务（优先当前分支）："}
		candidates := Candidates(input)
		for i, task := range candidates {
			if i == 5 {
				break
			}
			lines = append(lines, task.ID+" "+taskState(task.State)+"："+Truncate(oneLine(task.Title), 65))
		}
		if len(candidates) == 0 {
			lines = append(lines, "暂无候选；根据用户请求选择恢复或开始任务。")
		} else {
			lines = append(lines, fmt.Sprintf("共 %d 项候选；根据用户请求用 task_start 选择，未自动开始任务。", len(candidates)))
		}
		parts[1] = Truncate(strings.Join(lines, "\n"), 500)
	} else {
		task := view.Task
		fixed := fmt.Sprintf("任务 %s；%s；工作模式：%s；更新于 %s。", task.ID, taskState(task.State), workflow(view.Settings), stamp(task.UpdatedAt))
		parts[0] = fixed + "\n" + Truncate(oneLine(task.Title), max(0, 120-Tokens(fixed)-1))
		parts[1] = requirements(view)
	}
	if len(view.PendingActions) > 0 {
		parts[0] += "\n待处理事项：" + strings.Join(view.PendingActions, "\n")
	}
	parts[2] = "方案状态：全自动模式，无需方案确认。"
	if view.Settings.Values["workflow"].Value == "semi" {
		parts[2] = "方案状态：尚无有效方案。"
		if effective := view.Plans.Effective; effective != nil {
			parts[2] = fmt.Sprintf("方案状态：有效 v%d。", effective.Version)
		}
		if pending := view.Plans.Pending; pending != nil {
			parts[2] += fmt.Sprintf("待确认 v%d；已确认范围仍可继续。", pending.Version)
		}
		if effective := view.Plans.Effective; effective != nil {
			var scopes []string
			for _, file := range effective.Files {
				scopes = append(scopes, file.Path)
			}
			parts[2] += "\n有效路径：" + Truncate(effective.Path, 45) + "；范围：" + Truncate(strings.Join(scopes, ", "), 25)
		}
		if pending := view.Plans.Pending; pending != nil {
			parts[2] += "\n待确认路径：" + Truncate(pending.Path, 45) + "；变更：" + Truncate(strings.Join(pending.Changes, "；"), 25)
		}
		parts[2] = Truncate(parts[2], 180)
	}
	parts[3] = updates(view)
	parts[4] = "下一步：暂无笔记，按当前需求和代码变化接续。"
	if note := NextStep(view); note != nil {
		state := "记录后未观察到内容变化。"
		if input.NextStepChanged {
			state = "笔记后工作区已有变化，先核对当前内容，再判断下一步是否适用。"
		}
		prefix := "下一步（" + stamp(note.At) + "）：" + state + "\n"
		parts[4] = prefix + Truncate(oneLine(note.Text), max(0, 120-Tokens(prefix)))
	}
	lines := []string{"仍有效的用户决定："}
	remainingItems := 5
	for _, note := range view.EffectiveDecisions {
		if remainingItems == 0 {
			break
		}
		target := ""
		if note.Target != nil {
			target = note.Target.ID + " " + note.Target.At + " "
		}
		lines = append(lines, Truncate(oneLine(target+note.Text), 33))
		remainingItems--
	}
	if len(lines) == 1 {
		lines = append(lines, "暂无记录。")
	}
	if len(view.Checks) == 0 {
		lines = append(lines, "未解决问题：尚未检查。")
	} else {
		var issues []string
		expired := false
		for _, record := range view.Checks {
			counts := record.GapCounts()
			lines = append(lines, fmt.Sprintf("%s %s：无法判断 %d，错误 %d，未完成 %d。", record.Depth, record.Status, counts["unsupported"], counts["error"], counts["incomplete"]))
			if len(record.Unconfirmed) > 0 {
				lines = append(lines, fmt.Sprintf("另有 %d 处历史问题本次未确认，不按已解决处理。", len(record.Unconfirmed)))
			}
			if record.Status == ledger.CheckExpired {
				expired = true
				continue
			}
			for _, issue := range record.Issues {
				if issue.Status == "open" {
					issues = append(issues, issue.Rule+" "+issue.At)
				}
			}
		}
		lines = append(lines, fmt.Sprintf("未解决问题：已完成的检查范围内有 %d 处。", len(issues)))
		for _, issue := range issues {
			if remainingItems == 0 {
				break
			}
			lines = append(lines, Truncate(oneLine(issue), 25))
			remainingItems--
		}
		if expired {
			lines = append(lines, "旧检查已过期，按当前输入重新核对。")
		}
	}
	parts[5] = Truncate(strings.Join(lines, "\n"), 200)
	lines = []string{"累计改动文件："}
	for i, path := range input.ChangedFiles {
		if i == 5 {
			break
		}
		if Tokens(path) > 19 {
			lines = append(lines, "文件路径较长，见 task_get")
		} else {
			lines = append(lines, oneLine(path))
		}
	}
	if len(input.ChangedFiles) == 0 {
		lines = append(lines, "暂无累计改动。")
	}
	if input.Partial {
		lines = []string{"累计改动文件：本次只观察目标文件；完整改动待下次完整观察核对。"}
	}
	check := "尚未检查"
	if view.LatestCheck != "" && view.LatestCheck != "not_checked" {
		check = oneLine(view.LatestCheck)
	}
	verification := "最近验证：尚无验证记录。"
	if len(view.Verifications) > 0 {
		record := view.Verifications[len(view.Verifications)-1]
		state := record.State
		if record.Denied {
			state = "被拦截，未执行"
		}
		verification = "最近验证：" + record.ID + " " + state + "。"
	}
	lines = append(lines, verification, "最近检查："+check+"。")
	parts[6] = Truncate(strings.Join(lines, "\n"), 150)
	prelude := ""
	if guidance {
		prelude = Guidance(view.Settings)
	}
	if len(input.Peers) > 0 {
		count := len(input.Peers)
		if !input.Sessionless {
			count++
		}
		shared := fmt.Sprintf("最近十分钟有 %d 个 session 活动，正在共享当前工作区和任务；独立开发可使用 worktree。", count)
		prelude = Truncate(prelude, GuidanceBudget-Tokens(shared)-1)
		prelude = strings.TrimSpace(prelude + "\n" + shared)
	}
	compose := func() string { return strings.TrimSpace(prelude + "\n" + strings.Join(parts, "\n") + "\n" + Footer) }
	text := compose()
	// 固定身份、模式、方案状态和完整入口始终保留，从最后的可变项开始缩减。
	for _, i := range []int{6, 5, 4, 3, 1} {
		if Tokens(text) <= Budget {
			break
		}
		parts[i] = Truncate(parts[i], max(0, Tokens(parts[i])-(Tokens(text)-Budget)-1))
		text = compose()
	}
	return text
}
