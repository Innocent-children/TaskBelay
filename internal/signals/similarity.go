package signals

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func similarities(ctx context.Context, in Input) ([]Candidate, error) {
	if in.Queries == nil {
		return nil, fmt.Errorf("相似内容索引未提供")
	}
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		var source string
		var bodies map[[2]string][]string
		for _, u := range f.Units {
			if in.EditPreview && in.After.TestUnits[repostats.UnitKey(p, u)] {
				continue
			}
			if !declaration(f, u) {
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
			var sameFileKeys []string
			if in.EditPreview && u.Line < u.EndLine && len(in.Before.Files[p].Units) > 0 {
				if bodies == nil {
					source = strings.Join(f.Lines, "")
					bodies, err = previewBodyReferences(ctx, in, f, source)
					if err != nil {
						return out, err
					}
				}
				sameFileKeys = bodies[[2]string{u.Name, source[u.HeaderEnd:u.End]}]
			}
			for _, v := range in.Queries.Similar(sig, sameFileKeys) {
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
					fact := "两段实现的词序列相似"
					if len(sameFileKeys) > 0 && v.Path == p && source[u.HeaderEnd:u.End] == source[v.Unit.HeaderEnd:v.Unit.End] {
						fact = "两处声明的主体原文相同"
					}
					advice = fmt.Sprintf("在拟写入内容中，本次改动的声明在第 %d 行，已有声明在第 %d 行。%s。核对两处的输入输出、副作用和依赖，保留各自调用约定需要的入口，只复用行为一致的实现", u.Line, v.Unit.Line, fact)
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

/**
 * 从拟写入内容中按声明名和主体原文查找参照。只补查当前文件已有、含跨行子块的实现，
 * 避免为相同的单行返回或转发额外提示。候选仍须经过起点索引和两份内容的声明核对。
 */
func previewBodyReferences(ctx context.Context, in Input, f units.File, source string) (map[[2]string][]string, error) {
	existing := map[string]bool{}
	for _, u := range in.Before.Files[f.Path].Units {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		existing[u.ID] = true
	}
	bodies := map[[2]string][]string{}
	seen := map[int]bool{}
	for _, child := range f.Units {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if child.Parent < 0 || child.Line == child.EndLine || seen[child.Parent] {
			continue
		}
		u := f.Units[child.Parent]
		if child.Start < u.HeaderEnd {
			continue
		}
		seen[child.Parent] = true
		key := repostats.UnitKey(f.Path, u)
		if existing[u.ID] && !in.After.TestUnits[key] && declaration(f, u) {
			body := [2]string{u.Name, source[u.HeaderEnd:u.End]}
			bodies[body] = append(bodies[body], key)
		}
	}
	return bodies, nil
}
