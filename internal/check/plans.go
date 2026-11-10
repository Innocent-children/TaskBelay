package check

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/plan"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"strings"
)

func ClassifyTargets(ctx context.Context, s *ledger.Store, w gitstate.Workspace, contents map[string]string) (map[string]bool, error) {
	out := map[string]bool{}
	task, _, err := s.CommandContext(ctx, w.ID)
	if err != nil || task.ID == "" {
		return out, err
	}
	index, err := taskIndex(publishedOnly(ctx), s, w, task)
	if err != nil {
		return out, err
	}
	base := index.view(keys(contents))
	for _, p := range keys(contents) {
		var syn units.Syntax
		index.get("syntax", path.Ext(p), &syn)
		base.Syntax[path.Ext(p)] = syn
	}
	if !base.Converged {
		return out, fmt.Errorf("按词汇识别测试内容的分类结果尚未稳定，无法确定编辑目标类别")
	}
	files := map[string]units.File{}
	for _, p := range keys(contents) {
		f, err := units.Analyze(ctx, units.Source{Path: p, Data: []byte(contents[p])}, base.SyntaxFor(p))
		if err != nil {
			return out, err
		}
		files[p] = f
	}
	r, err := index.apply(keys(contents), files, "")
	if err != nil {
		return out, err
	}
	for p, role := range r.Roles {
		out[p] = role.Test || role.Generated || role.Lock
	}
	return out, index.readResult()
}

func planIssues(w gitstate.Workspace, v ledger.TaskView, a analysis) []ledger.Issue {
	var out []ledger.Issue
	seen := map[string]bool{}
	for _, edit := range a.Edits {
		if edit.PlanCheck == nil || edit.PlanCheck.Workflow != "semi" {
			continue
		}
		for _, h := range a.Changes.Hunks {
			if h.Origin != "agent" || !strings.Contains(","+h.Call+",", ","+edit.ToolCallID+",") {
				continue
			}
			for _, issue := range edit.PlanCheck.Issues {
				if issue.At == h.Path && !seen["S1:"+h.Path] {
					issue.Algorithm = Algorithm
					issue.Selected = true
					issue.Level = "hint"
					issue.BlockOnDelivery = true
					out = append(out, issue)
					seen["S1:"+h.Path] = true
				}
			}
			if v.Settings.Values["workflow"].Value != "semi" || seen["S2:"+h.Path] {
				continue
			}
			role := a.After.Roles[h.Path]
			aux := role.Test || role.Generated || role.Lock
			allowed := false
			if p := v.Plans.Effective; p != nil {
				allowed = aux && len(p.Files) > 0
				for _, f := range p.Files {
					match, err := plan.Within(w, f.Path, h.Path)
					allowed = allowed || err == nil && match
				}
			}
			if !allowed && a.After.Converged {
				out = append(out, ledger.Issue{Algorithm: Algorithm, Selected: true, Rule: "S2", At: h.Path, LogicalAt: h.Path, FactKey: "plan_scope", Status: "open", Level: "hint", BlockOnDelivery: true, Facts: []string{"半自动模式下的 Agent 改动超出当前有效方案范围"}, Advice: "核对需要保留的行为，补充方案并取得确认"})
				seen["S2:"+h.Path] = true
			}
		}
	}
	if v.Settings.Values["workflow"].Value == "semi" {
		verified := make(map[string]bool, len(a.Changes.Clusters))
		for _, cluster := range a.Changes.Clusters {
			verified[cluster.ID] = cluster.Origin == "agent"
		}
		for _, e := range a.Signals.External {
			if e.Kind != "dep" || !e.Reliable || !e.AgentOnly {
				continue
			}
			allowed := false
			if p := v.Plans.Effective; p != nil {
				for _, d := range p.Dependencies {
					allowed = allowed || d.Module == e.Name
				}
			}
			if !allowed {
				out = append(out, ledger.Issue{Algorithm: Algorithm, Selected: true, Rule: "S2", At: e.At, LogicalAt: "dep:" + e.Name, FactKey: "dependency", Status: "open", Level: "hint", BlockOnDelivery: verified[e.Cluster], Facts: []string{"直接依赖未列入有效方案：" + e.Name}, Advice: "核对依赖必要性并更新方案"})
			}
		}
	}
	return out
}
