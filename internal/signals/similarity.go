package signals

import (
	"context"
	"fmt"
	"math"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
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
			target := location(p, u)
			for _, v := range in.Queries.Similar(sig) {
				if !v.Callable {
					continue
				}
				if v.Path == p && (v.Unit.ID == u.ID || v.Unit.Line <= u.EndLine && u.Line <= v.Unit.EndLine) {
					continue
				}
				at := location(v.Path, v.Unit)
				// 只有编辑预览已将同文件参照核对到拟写入的位置。
				if at == target && !in.EditPreview {
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
				advice := "比较 " + target + " 与 " + at + " 的输入输出、副作用和依赖；行为与职责一致时复用已有单元，否则保留必要差异"
				if at == target {
					advice = fmt.Sprintf("拟写入内容中，第 %d 行的声明与第 %d 行已有的同名声明相似。核对两处的输入输出、副作用和依赖，保留各自调用约定需要的入口，只复用行为一致的实现", u.Line, v.Unit.Line)
				}
				out = append(out, Candidate{
					Kind:      "duplicate",
					Object:    units.Hash(p, u.ID, v.Path, v.Unit.ID),
					At:        target,
					Cluster:   cluster,
					AgentOnly: agent,
					Facts:     []string{fmt.Sprintf("与 %s 相似度 %.3f；无关单元均值 %.3f", at, similarity, in.Queries.Mean), "词序列相似不证明用途和行为相同"},
					Advice:    advice,
					Features:  map[string]float64{"similarity": similarity, "similarity_excess": z, "structure": in.After.Files[p].Confidence},
					Benefit:   1,
				})
			}
		}
	}
	return out, ctx.Err()
}
