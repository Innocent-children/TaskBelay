package signals

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
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
				} else if in.EditPreview && v.Path == p {
					if detail := booleanForwardingAdvice(f, u, v.Unit); detail != "" {
						advice = detail
					}
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

func booleanForwardingAdvice(f units.File, proposed, reference units.Unit) string {
	target, args := returnedCall(f, proposed)
	if target == "" {
		return ""
	}
	other, prior := returnedCall(f, reference)
	if other != target || len(args) != len(prior) {
		return ""
	}
	different := -1
	for i, arg := range args {
		if arg == prior[i] {
			continue
		}
		if different >= 0 || !(arg == "true" && prior[i] == "false" || arg == "false" && prior[i] == "true") {
			return ""
		}
		different = i
	}
	if different < 0 {
		return ""
	}
	fact := fmt.Sprintf("拟写入的 %s（第 %d 行）和 %s（第 %d 行）都直接返回 %s 的调用结果。第 %d 个实参分别为 %s 和 %s，其余实参写法相同。", proposed.Name, proposed.Line, reference.Name, reference.Line, target, different+1, args[different], prior[different])
	if name, line := loopFilterParameter(f, target, different, len(args)); line > 0 {
		return fact + fmt.Sprintf("同文件的 %s 在循环第 %d 行的 if 条件中使用形参 %s，分支内只执行 continue。若 %s 的结果保留了筛选所需的数据，且在调用处筛选能保持求值、副作用和入口约定，可省去这层转发", target, line, name, reference.Name)
	}
	return fact + fmt.Sprintf("核对这两个取值改变了什么。如果调用方用 %s 的结果就能保留所需行为和入口约定，可以省去这层转发", reference.Name)
}

/**
 * 只跟进已有转发建议。两个调用的实参位置须对应同一份声明，且这个形参在正文里只有一处可核对的使用。
 */
func loopFilterParameter(f units.File, target string, slot, arity int) (string, int) {
	callee := -1
	for i, u := range f.Units {
		if u.Name == target && declaration(f, u) && u.Tokens[len(u.Tokens)-1].Text == "}" {
			if callee >= 0 {
				return "", 0
			}
			callee = i
		}
	}
	if callee < 0 {
		return "", 0
	}
	u := f.Units[callee]
	params := declarationParams(u)
	if len(params) != arity {
		return "", 0
	}
	name, ok := strings.CutSuffix(params[slot].Text, " bool")
	if !ok {
		return "", 0
	}
	body := u.BodyTokens()
	isName := func(t units.Token) bool { return t.Kind == "identifier" && t.Text == name }
	use := slices.IndexFunc(body, isName)
	if use < 1 || slices.ContainsFunc(body[use+1:], isName) {
		return "", 0
	}
	// 排除字段选择、重新绑定等同名写法，只接受直接作为条件项的变量。
	if before := body[use-1].Text; before != "if" && before != "||" && before != "&&" {
		return "", 0
	}
	if after := body[use+1].Text; after != "&&" && after != "||" && after != "{" {
		return "", 0
	}
	for _, branch := range f.Units[callee+1:] {
		if branch.Start > body[use].Start {
			break
		}
		if branch.HeaderEnd < body[use].End || branch.Parent < 0 || branch.Tokens[0].Text != "if" {
			continue
		}
		loop := f.Units[branch.Parent]
		if loop.Parent != callee || loop.Tokens[0].Text != "for" {
			continue
		}
		statements := branch.BodyTokens()
		if (len(statements) == 2 || len(statements) == 3 && statements[1].Text == ";") &&
			statements[0].Text == "continue" && statements[len(statements)-1].Text == "}" {
			return name, branch.Line
		}
	}
	return "", 0
}

/**
 * 声明首行的类型括号也可能成为相似候选。按起点和名字找到完整的大括号主体，
 * 只接受直接返回一次调用且每个实参均为单个标识符的写法。
 */
func returnedCall(f units.File, at units.Unit) (string, []string) {
	if f.Layout != "brackets" {
		return "", nil
	}
	first := sort.Search(len(f.Units), func(i int) bool { return f.Units[i].Start >= at.Start })
	for _, u := range f.Units[first:] {
		if u.Start != at.Start {
			break
		}
		if u.Name != at.Name || u.End < at.End {
			continue
		}
		body := u.BodyTokens()
		if len(body) < 6 || body[len(body)-1].Text != "}" || !declaration(f, u) {
			continue
		}
		body = body[:len(body)-1]
		if body[len(body)-1].Text == ";" {
			body = body[:len(body)-1]
		}
		// 换行可能结束 return 语句，不能把下一条调用当作返回值。
		if body[0].Line != body[2].Line || body[0].Text != "return" || body[1].Kind != "identifier" ||
			body[2].Text != "(" || body[len(body)-1].Text != ")" {
			continue
		}
		fields := body[3 : len(body)-1]
		if len(fields)%2 != 1 {
			continue
		}
		var args []string
		for i := 0; i < len(fields); i += 2 {
			if fields[i].Kind != "identifier" || i > 0 && fields[i-1].Text != "," {
				return "", nil
			}
			args = append(args, fields[i].Text)
		}
		return body[1].Text, args
	}
	return "", nil
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
