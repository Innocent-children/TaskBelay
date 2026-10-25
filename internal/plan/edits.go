package plan

import (
	"strings"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/lang"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func Within(workspace gitstate.Workspace, scope, file string) (bool, error) {
	if scope == "./" {
		return true, nil
	}
	if strings.HasSuffix(scope, "/") {
		parts := strings.Split(strings.TrimSuffix(scope, "/"), "/")
		target := strings.Split(file, "/")
		if len(target) <= len(parts) {
			return false, nil
		}
		file = strings.Join(target[:len(parts)], "/")
		scope = strings.TrimSuffix(scope, "/")
	}
	one, err := workspace.TargetKey(scope)
	if err != nil {
		return false, err
	}
	two, err := workspace.TargetKey(file)
	return one == two, err
}

func BeforeEdit(workspace gitstate.Workspace, context ledger.PlanEditContext, paths []string, contents map[string]string) (ledger.PlanEditCheck, error) {
	check := ledger.PlanEditCheck{TaskID: context.TaskID, Workflow: context.Workflow, SettingsRevision: context.SettingsRevision, PlanRevision: context.PlanRevision, AuthorizationRevision: context.AuthorizationRevision, Paths: paths}
	if context.Workflow != "semi" {
		return check, nil
	}
	for _, file := range paths {
		allowed := false
		if effective := context.Plans.Effective; effective != nil {
			kind := lang.Classify(file, []byte(contents[file]))
			allowed = len(effective.Files) > 0 && (kind.Test || kind.TestData || kind.Generated || kind.Lock)
			for _, scope := range effective.Files {
				match, err := Within(workspace, scope.Path, file)
				if err != nil {
					return check, err
				}
				allowed = allowed || match
			}
		}
		for _, note := range context.Decisions {
			if note.Target != nil && note.Target.Effect == "allow_edit" && note.Citation != nil {
				match, err := Within(workspace, note.Target.At, file)
				if err != nil {
					return check, err
				}
				allowed = allowed || match
			}
		}
		if allowed {
			continue
		}
		evidence := []string{"尚无已确认的有效方案"}
		if effective := context.Plans.Effective; effective != nil {
			evidence = []string{effective.ID, effective.Path}
		}
		check.Issues = append(check.Issues, ledger.Issue{Rule: "S1", At: file, FactKey: "plan_scope", Level: "block", Status: "open", Facts: []string{"本次 Agent 编辑目标不在已确认的有效范围内。"}, Advice: "先提交并确认该范围的方案；已确认范围继续开发，用户手动修改照常保留。", Evidence: evidence})
	}
	return check, nil
}
