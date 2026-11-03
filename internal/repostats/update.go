package repostats

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/units"
)

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
			score := 0.0
			for _, t := range u.Tokens {
				if t.Kind == "identifier" {
					score += origin.TestWords[t.Text]
				}
			}
			if origin.TestUnits[UnitKey(p, u)] || score > 0 {
				r.TestUnits[UnitKey(p, u)] = true
				role.Test = true
			}
		}
		r.Roles[p] = role
	}
	return r, nil
}
