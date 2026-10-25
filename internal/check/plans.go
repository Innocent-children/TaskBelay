package check

import (
	"context"
	"fmt"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/lang"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/plan"
)

/**
 * S1 使用编辑前依据和实际后置观察时间；S2 使用当前方案和仍留在快照中的编辑来源。
 * 只有已核对的半自动编辑参与拦截，不能把用户更新或切换前的 auto 工作归入违规。
 */
func checkPlan(ctx context.Context, workspace gitstate.Workspace, view ledger.TaskView, before, after codescan.Snapshot, changes []ledger.FileChange, records []ledger.EditRecord, sources provenance, contents map[string][]byte, result *ledger.CheckRecord) error {
	byObservation := map[int64]ledger.EditRecord{}
	for _, record := range records {
		byObservation[record.AfterObservation] = record
	}
	semi := view.Settings.Values["workflow"].Value == "semi"
	scopeComplete := true
	if semi {
		for _, snapshot := range []codescan.Snapshot{before, after} {
			for path, file := range snapshot.Files {
				if lang.Manifest(path) && !file.Operations["dependencies"].Complete {
					scopeComplete = false
				}
			}
		}
	}
	historyComplete := true
	issues := map[string]*ledger.Issue{}
	add := func(rule, at, key, fact string, block bool, evidence ...string) {
		identity := rule + "\x00" + at + "\x00" + key
		issue := issues[identity]
		if issue == nil {
			issue = &ledger.Issue{Rule: rule, At: at, FactKey: key, Status: "open", Level: "hint", Advice: "保留当前工作区；仅核对可归责的 Agent 编辑及对应授权，新增范围需补充方案，不要求用户为手改补方案。"}
			issues[identity] = issue
		}
		issue.BlockOnDelivery = issue.BlockOnDelivery || block && semi
		issue.Facts = append(issue.Facts, fact)
		issue.Evidence = append(issue.Evidence, evidence...)
	}
	allowed := func(scope *ledger.PlanRecord, path string) (bool, error) {
		if scope == nil {
			return false, nil
		}
		for _, file := range scope.Files {
			ok, err := plan.Within(workspace, file.Path, path)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	semiObservations := map[int64]bool{}
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if change.Kind != "agent_edit" {
			continue
		}
		record, found := byObservation[change.AfterObservation]
		if found && record.PlanCheck == nil {
			historyComplete = false
			result.Status = ledger.CheckPartial
			result.Missing = append(result.Missing, ledger.CheckGap{At: change.Path, Operation: "S1", Reason: "旧编辑记录缺少当时的模式与授权依据，不能追溯判定为未授权写入"})
		}
		if !found || record.PlanCheck == nil || record.PlanCheck.Workflow != "semi" {
			continue
		}
		semiObservations[change.AfterObservation] = true
		beforeAllowed, err := allowed(view.Plans.At(record.PlanCheck.CheckedAt), change.Path)
		if err != nil {
			return err
		}
		afterAllowed, err := allowed(view.Plans.At(change.At), change.Path)
		if err != nil {
			return err
		}
		if beforeAllowed && !afterAllowed {
			add("S1", change.Path, "plan_scope", "编辑前已有授权，但在后置观察之前撤回了该范围；实际写入与撤回的先后无法核对。", false)
			result.Status = ledger.CheckPartial
			result.Missing = append(result.Missing, ledger.CheckGap{At: change.Path, Operation: "S1", Reason: "授权在编辑前后观察之间撤回，实际写入时刻未知"})
		}
		for _, problem := range record.PlanCheck.Issues {
			if problem.At != change.Path {
				continue
			}
			if beforeAllowed {
				continue
			}
			if afterAllowed {
				add("S1", change.Path, "plan_scope", "编辑前核对与后置观察之间取得授权，无法确定实际写入与授权的先后，保留时间窗口供核对。", false, problem.Evidence...)
				result.Status = ledger.CheckPartial
				result.Missing = append(result.Missing, ledger.CheckGap{At: change.Path, Operation: "S1", Reason: "授权在编辑前后观察之间发生，实际写入时刻未知"})
				continue
			}
			fact := fmt.Sprintf("编辑前未获该范围授权，观察点 %d 于 %s 核对到 Agent 实际写入；后续确认不改写此时的授权事实。", change.AfterObservation, change.At.Format("2006-01-02T15:04:05.999999999Z07:00"))
			add("S1", change.Path, "plan_scope", fact, true, problem.Evidence...)
		}
	}
	if historyComplete {
		result.Evaluated = append(result.Evaluated, ledger.CheckScope("S1", "*"))
	}
	if semi {
		paths := map[string]bool{}
		for _, change := range changes {
			if change.Kind == "agent_edit" && semiObservations[change.AfterObservation] {
				paths[change.Path] = true
			}
		}
		for path := range paths {
			if err := ctx.Err(); err != nil {
				return err
			}
			result.Evaluated = append(result.Evaluated, ledger.CheckScope("S2", path))
			old, current := before.Files[path], after.Files[path]
			file := current
			if file == nil {
				file = old
			}
			if excluded(file) || old == nil && current == nil || old != nil && current != nil && old.Blob == current.Blob && old.Mode == current.Mode {
				continue
			}
			ok, err := allowed(view.Plans.Effective, path)
			if err != nil {
				return err
			}
			for _, note := range view.EffectiveDecisions {
				if note.Target != nil && note.Target.Effect == "allow_edit" && note.Citation != nil {
					match, err := plan.Within(workspace, note.Target.At, path)
					if err != nil {
						return err
					}
					ok = ok || match
				}
			}
			if ok {
				continue
			}
			agent, mixed, evidence := planFileSource(path, current, changes, semiObservations, sources)
			if !agent {
				continue
			}
			fact := "当前仍有已核对的半自动 Agent 改动，生产文件不在有效方案范围内。"
			if mixed {
				fact += " 文件含有工作区更新或不能拆分的来源，只展示实际关系。"
			}
			add("S2", path, "plan_file", fact, !mixed, evidence...)
		}
		var old, current []codescan.Dependency
		for _, file := range before.Files {
			old = append(old, file.Dependencies...)
		}
		for _, file := range after.Files {
			current = append(current, file.Dependencies...)
		}
		for _, dep := range codescan.AddedDependencies(old, current) {
			if err := ctx.Err(); err != nil {
				return err
			}
			ok := false
			if scope := view.Plans.Effective; scope != nil {
				for _, candidate := range scope.Dependencies {
					ok = ok || candidate.Module == dep.Name
				}
			}
			if ok {
				result.Evaluated = append(result.Evaluated, ledger.CheckScope("S2", "dep:"+dep.Name))
				continue
			}
			agent, mixed, evidence := planFileSource(dep.Manifest, after.Files[dep.Manifest], changes, semiObservations, sources)
			if !agent {
				continue
			}
			introduced := false
			for _, change := range changes {
				if change.Path != dep.Manifest || change.Kind != "agent_edit" || !semiObservations[change.AfterObservation] {
					continue
				}
				one, err1 := codescan.Dependencies(ctx, dep.Manifest, contents[change.Before.Blob])
				two, err2 := codescan.Dependencies(ctx, dep.Manifest, contents[change.After.Blob])
				if err1 != nil || err2 != nil {
					scopeComplete = false
					result.Status = ledger.CheckPartial
					result.Missing = append(result.Missing, ledger.CheckGap{At: dep.Manifest, Operation: "S2", Reason: "历史清单解析不完整，新增依赖与具体半自动编辑的关系尚未核对"})
					mixed = true
					continue
				}
				for _, added := range codescan.AddedDependencies(one, two) {
					introduced = introduced || added.Name == dep.Name
				}
			}
			if !introduced {
				continue
			}
			file := after.Files[dep.Manifest]
			complete := file != nil && file.Operations["dependencies"].Complete
			if oldFile := before.Files[dep.Manifest]; oldFile != nil {
				complete = complete && oldFile.Operations["dependencies"].Complete
			}
			fact := "新增直接依赖 " + dep.Name + " 未列入有效方案的依赖清单。"
			if mixed || !complete {
				fact += " 清单含混合更新或解析不完整，只提供提示。"
			}
			add("S2", "dep:"+dep.Name, "plan_dependency:"+dep.Manifest, fact, !mixed && complete, evidence...)
		}
	}
	if scopeComplete {
		result.Evaluated = append(result.Evaluated, ledger.CheckScope("S2", "*"))
	}
	keys := make([]string, 0, len(issues))
	for key := range issues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result.Issues = append(result.Issues, *issues[key])
	}
	return ctx.Err()
}

func planFileSource(path string, current *codescan.File, changes []ledger.FileChange, semi map[int64]bool, sources provenance) (agent, mixed bool, evidence []string) {
	markers := map[string]bool{}
	var last *ledger.FileChange
	for i := range changes {
		change := &changes[i]
		if change.Path != path {
			continue
		}
		last = change
		if change.Kind == "agent_edit" && semi[change.AfterObservation] {
			markers[fmt.Sprintf("agent_edit:%d:%s", change.AfterObservation, change.ToolCallID)] = true
		}
	}
	seen := map[string]bool{}
	for _, line := range sources[path].lines {
		agent = agent || markers[line.evidence]
		mixed = mixed || line.kind == "workspace" || line.kind == "unknown"
		if line.evidence != "" && !seen[line.evidence] {
			evidence = append(evidence, line.evidence)
			seen[line.evidence] = true
		}
	}
	if current == nil || len(sources[path].lines) == 0 {
		if last != nil {
			agent = last.Kind == "agent_edit" && semi[last.AfterObservation]
			mixed = last.Kind != "agent_edit"
			evidence = append(evidence, fmt.Sprintf("%s:%d:%s", last.Kind, last.AfterObservation, last.ToolCallID))
		}
	}
	sort.Strings(evidence)
	return
}
