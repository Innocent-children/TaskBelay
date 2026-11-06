package repostats

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
)

// Observe 用已结束的运行更新起点测试约定，复制统计映射以保持内容缓存不可变。
func Observe(ctx context.Context, origin Repository, runs []Run) (Repository, error) {
	if len(runs) == 0 {
		return origin, ctx.Err()
	}
	r := origin
	r.Roles = map[string]Role{}
	r.TestUnits = map[string]bool{}
	r.TestWords = map[string]float64{}
	r.Converged = false
	for p, role := range origin.Roles {
		role.Test = false
		r.Roles[p] = role
	}
	if err := learnTests(ctx, &r, runs); err != nil {
		return r, err
	}
	for p, f := range r.Files {
		role := r.Roles[p]
		for _, u := range f.Units {
			role.Test = role.Test || r.TestUnits[UnitKey(p, u)]
		}
		r.Roles[p] = role
	}
	r.Results = LearnResults(runs, CaseNames(r))
	return r, ctx.Err()
}

// Apply 只把起点约定应用于当前内容，新增代码不能改变自己的判定约定。
func Apply(ctx context.Context, origin Repository, tree string, files map[string]units.File) (Repository, error) {
	r := origin
	r.Tree, r.Files, r.Roles, r.TestUnits = tree, files, map[string]Role{}, map[string]bool{}
	for _, p := range sortedKeys(files) {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		f := files[p]
		role := classify(f)
		if old, ok := origin.Roles[p]; ok {
			role.Dependency = old.Dependency
		}
		for _, u := range f.Units {
			if u.Parent >= 0 {
				continue
			}
			key := UnitKey(p, u)
			if origin.Converged && origin.Tests.Extensions[path.Ext(p)] && (origin.TestSeeds[key] || origin.Tests.TestCounts(TestFeatures(p, u), origin.TestUnits[key])) {
				r.TestUnits[UnitKey(p, u)] = true
				role.Test = true
			}
		}
		for _, u := range f.Units {
			if r.TestUnits[UnitKey(p, testRoot(f, u))] {
				r.TestUnits[UnitKey(p, u)] = true
			}
		}
		r.Roles[p] = role
	}
	return r, nil
}
