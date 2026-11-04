package signals

import (
	"context"
	"fmt"
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
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			if cluster == "" {
				continue
			}
			grams, err := units.Grams(ctx, units.Sequence(u.Tokens, true))
			if err != nil {
				return out, err
			}
			sig, err := units.MinHash(ctx, grams)
			if err != nil {
				return out, err
			}
			for _, v := range in.Queries.Similar(sig) {
				at := location(v.Path, v.Unit)
				if at == location(p, u) || v.Unit.ID == u.ID && v.Path == p {
					continue
				}
				similarity := units.Similarity(grams, v.Grams)
				z := similarity - in.Queries.Mean
				if in.Queries.Variance > 0 {
					z /= math.Sqrt(in.Queries.Variance)
				}
				out = append(out, Candidate{Kind: "duplicate", Object: units.Hash(p, u.ID, v.Path, v.Unit.ID), At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{fmt.Sprintf("与 %s 相似度 %.3f；无关单元均值 %.3f", at, similarity, in.Queries.Mean)}, Advice: "复用 " + at + "，合并 " + location(p, u), Features: map[string]float64{"similarity": similarity, "similarity_excess": z, "structure": in.After.Files[p].Confidence}, Benefit: 1})
			}
		}
	}
	return out, ctx.Err()
}
