package signals

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"math"
)

func similarities(ctx context.Context, in Input) ([]Candidate, error) {
	if in.Queries == nil {
		return nil, fmt.Errorf("相似内容索引未提供")
	}
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		for _, u := range in.After.Files[p].Units {
			if in.EditPreview && in.After.TestUnits[repostats.UnitKey(p, u)] {
				continue
			}
			if !declaration(in.After.Files[p], u) {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			if cluster == "" {
				continue
			}
			grams, err := units.Grams(ctx, units.Sequence(u.Tokens, true))
			if err != nil {
				return out, err
			}
			if len(grams) == 0 {
				continue
			}
			sig, err := units.MinHash(ctx, grams)
			if err != nil {
				return out, err
			}
			for _, v := range in.Queries.Similar(sig) {
				if !v.Callable {
					continue
				}
				if v.Path == p && v.Unit.Line <= u.EndLine && u.Line <= v.Unit.EndLine {
					continue
				}
				at := location(v.Path, v.Unit)
				if at == location(p, u) || v.Unit.ID == u.ID && v.Path == p {
					continue
				}
				similarity := units.Similarity(grams, v.Grams)
				if similarity == 0 {
					continue
				}
				z := similarity - in.Queries.Mean
				if in.Queries.Variance > 0 {
					z /= math.Sqrt(in.Queries.Variance)
				}
				out = append(out, Candidate{Kind: "duplicate", Object: units.Hash(p, u.ID, v.Path, v.Unit.ID), At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{fmt.Sprintf("与 %s 相似度 %.3f；无关单元均值 %.3f", at, similarity, in.Queries.Mean), "词序列相似不证明用途和行为相同"}, Advice: "比较 " + location(p, u) + " 与 " + at + " 的输入输出、副作用和依赖；行为与职责一致时复用已有单元，否则保留必要差异", Features: map[string]float64{"similarity": similarity, "similarity_excess": z, "structure": in.After.Files[p].Confidence}, Benefit: 1})
			}
		}
	}
	return out, ctx.Err()
}
