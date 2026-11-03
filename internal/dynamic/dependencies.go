package dynamic

import (
	"context"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/units"
)

// 没有单条用例结果时，失败输出中同时出现的新增声明补充静态依赖；合并后重新运行。
func (r *runner) mergeFailure(ctx context.Context, removed []string, runs []RunResult) (bool, error) {
	words := map[string]bool{}
	for _, run := range runs {
		if run.State != "failed" {
			continue
		}
		named := false
		for _, c := range run.Cases {
			named = named || c.Name != run.Command && (c.State == "passed" || c.State == "failed")
		}
		if named {
			continue
		}
		for _, w := range units.Words(run.Output) {
			words[w] = true
		}
	}
	ids := []string{}
	inside, outside := false, false
	for _, c := range r.result.Clusters {
		if c.Origin != "agent" {
			continue
		}
		matched := false
		for _, word := range c.NewWords {
			matched = matched || words[strings.ToLower(word)]
		}
		if !matched {
			continue
		}
		ids = append(ids, c.ID)
		if contains(removed, c.ID) {
			inside = true
		} else {
			outside = true
		}
	}
	if !inside || !outside {
		return false, nil
	}
	merged, err := changes.Merge(ctx, r.result.Clusters, ids, "失败输出同时提及撤回簇与保留簇的新增声明")
	if err != nil {
		return false, err
	}
	newID := ""
	for _, c := range merged {
		if c.MergeReason != "" && !containsCluster(r.result.Clusters, c.ID) {
			newID = c.ID
		}
	}
	if newID == "" {
		return false, nil
	}
	if r.merged == nil {
		r.merged = map[string]string{}
	}
	if r.input.Confidence == nil {
		r.input.Confidence = map[string]float64{}
	}
	for _, id := range ids {
		r.merged[id] = newID
		r.input.Confidence[newID] = max(r.input.Confidence[newID], r.input.Confidence[id])
	}
	r.result.Clusters = merged
	r.input.Changes.Clusters = merged
	for i := range r.input.Regions {
		for j, id := range r.input.Regions[i].Clusters {
			if contains(ids, id) {
				r.input.Regions[i].Clusters[j] = newID
			}
		}
		r.input.Regions[i].Clusters = unique(r.input.Regions[i].Clusters)
	}
	for i := range r.input.Signals.Candidates {
		if contains(ids, r.input.Signals.Candidates[i].Cluster) {
			r.input.Signals.Candidates[i].Cluster = newID
		}
	}
	if !contains(r.eligible(), newID) {
		return false, unknown("失败关联的合并簇缺少撤回条件")
	}
	return true, r.emit()
}
func containsCluster(clusters []changes.Cluster, id string) bool {
	for _, c := range clusters {
		if c.ID == id {
			return true
		}
	}
	return false
}
