package signals

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type indirection struct {
	path        string
	unit        units.Unit
	kind        string
	count       int
	replacement string
}

func measure(ctx context.Context, repo repostats.Repository, affected map[string]bool, query *Queries) ([]indirection, []string, error) {
	var out []indirection
	var gaps []string
	type callSite struct {
		path string
		args []units.Token
	}
	allCalls := map[string][]callSite{}
	type head struct {
		id, name string
		words    map[string]bool
	}
	var heads []head
	byWord := map[string][]int{}
	defined := map[string]bool{}
	currentPath := ""
	depend := func(kind, word string) {
		if query != nil && query.Depend != nil {
			query.Depend(currentPath, kind, word)
		}
	}
	if query == nil {
		for _, p := range keys(repo.Files) {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			for name, sites := range units.Calls(repo.Files[p].Tokens) {
				for _, args := range sites {
					allCalls[name] = append(allCalls[name], callSite{p, args})
				}
			}
			f := repo.Files[p]
			for _, u := range f.Units {
				defined[u.Name] = true
				words := map[string]bool{}
				add := func(v units.Unit) {
					for _, t := range v.Tokens {
						if t.End <= v.HeaderEnd {
							words[t.Text] = true
						}
					}
				}
				add(u)
				if u.Parent >= 0 {
					add(f.Units[u.Parent])
				}
				for word := range words {
					byWord[word] = append(byWord[word], len(heads))
				}
				heads = append(heads, head{repostats.UnitKey(p, u), u.Name, words})
			}
		}
	}
	callsFor := func(name string) []callSite {
		depend("calls", name)
		if query == nil {
			return allCalls[name]
		}
		var out []callSite
		for _, s := range query.Calls(name) {
			out = append(out, callSite{s.Path, s.Args})
		}
		return out
	}
	headsFor := func(word string) []head {
		depend("head", word)
		var out []head
		if query == nil {
			for _, i := range byWord[word] {
				out = append(out, heads[i])
			}
		} else {
			for _, h := range query.Heads(word) {
				out = append(out, head{h.ID, h.Name, h.Words})
			}
		}
		return out
	}
	definedName := func(name string) bool {
		depend("name", name)
		if query != nil {
			return query.Defined(name)
		}
		return defined[name]
	}
	for _, p := range keys(repo.Files) {
		currentPath = p
		f := repo.Files[p]
		ignored := map[string]bool{}
		for _, w := range append(append([]string(nil), repo.SyntaxFor(p).Keywords...), repo.SyntaxFor(p).Returns...) {
			ignored[w] = true
		}
		for _, u := range f.Units {
			if err := ctx.Err(); err != nil {
				return out, gaps, err
			}
			if affected != nil && !affected[repostats.UnitKey(p, u)] {
				continue
			}
			if u.Name == "" {
				continue
			}
			var shape UnitStructure
			if query != nil && query.Structure != nil {
				shape = query.Structure(repostats.UnitKey(p, u))
			} else {
				shape = Structure(f, u)
			}
			var body, header []string
			for _, word := range shape.Header {
				if !ignored[word] {
					header = append(header, word)
				}
			}
			for _, word := range shape.Body {
				if !ignored[word] {
					body = append(body, word)
				}
			}
			members := shape.Members
			if shape.Abstract && len(members) > 0 {
				var common map[string]bool
				memberSet := map[string]bool{}
				unitKey := repostats.UnitKey(p, u)
				for _, m := range members {
					memberSet[m] = true
					matches := headsFor(m)
					present := map[string]bool{}
					for _, v := range matches {
						if err := ctx.Err(); err != nil {
							return out, gaps, err
						}
						if v.id == unitKey || v.name == u.Name || common != nil && len(common) == 0 {
							continue
						}
						for word := range v.words {
							if common == nil || common[word] {
								present[word] = true
							}
						}
					}
					common = present
				}
				implementations := map[string]bool{}
				for word := range common {
					if word == u.Name || ignored[word] || slices.Contains(members, word) {
						continue
					}
					if len(memberSet) == len(members) && definedName(word) {
						implementations[word] = true
					}
				}
				out = append(out, indirection{p, u, "abstraction", len(implementations), strings.Join(keys(implementations), ", ")})
			}
			if len(body) > 0 {
				var params []string
				for _, w := range header {
					if w != u.Name {
						params = append(params, w)
					}
				}
				if shape.Callable && shape.BodyCalls == 1 && slices.Equal(body[1:], params) {
					out = append(out, indirection{p, u, "forwarding", 1, body[0]})
				}
			}
			if !shape.Callable {
				continue
			}
			if repo.SyntaxFor(p).CallRatio <= CallPrecision {
				gaps = append(gaps, "实参未知："+location(p, u))
				continue
			}
			sites := callsFor(u.Name)
			if len(sites) == 0 {
				continue
			}
			positions := map[int]map[string]bool{}
			unknown := map[int]bool{}
			for _, site := range sites {
				for i, arg := range site.args {
					if site.path == p && arg.Start <= u.HeaderEnd && arg.End >= u.Start {
						continue
					}
					if positions[i] == nil {
						positions[i] = map[string]bool{}
					}
					positions[i][arg.Text] = true
					value, err := units.Analyze(ctx, units.Source{Data: []byte(arg.Text)}, repo.SyntaxFor(site.path))
					if err != nil {
						return out, gaps, err
					}
					tokens := units.Significant(value.Tokens)
					if len(tokens) == 2 && tokens[0].Text == "-" && tokens[1].Kind == "number" {
						tokens = tokens[1:]
					}
					unknown[i] = unknown[i] || len(tokens) != 1 || (tokens[0].Kind != "number" && tokens[0].Kind != "string" && tokens[0].Kind != "key")
				}
			}
			for i := 0; i < len(positions); i++ {
				values := positions[i]
				if unknown[i] {
					continue
				}
				if len(values) > 0 {
					out = append(out, indirection{p, u, "parameter", len(values), fmt.Sprintf("参数 %d = %s", i+1, strings.Join(keys(values), ", "))})
				}
			}
		}
	}
	return out, gaps, ctx.Err()
}

func indirections(ctx context.Context, in Input) ([]Candidate, []string, error) {
	prior := map[string]float64{}
	if in.Queries != nil {
		prior = in.Queries.Prior
	} else {
		before, _, err := measure(ctx, in.Before, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		total := map[string]int{}
		for _, v := range before {
			total[v.kind]++
			if v.count == 1 {
				prior[v.kind]++
			}
		}
		for k, n := range total {
			prior[k] /= float64(n)
		}
	}
	affected := map[string]bool{}
	for _, p := range keys(in.After.Files) {
		for _, u := range in.After.Files[p].Units {
			if cluster, _ := clusterFor(in, p, u.Line, u.EndLine); cluster != "" {
				affected[repostats.UnitKey(p, u)] = true
			}
		}
	}
	after, gaps, err := measure(ctx, in.After, affected, in.Queries)
	if err != nil {
		return nil, gaps, err
	}
	var out []Candidate
	for _, v := range after {
		cluster, agent := clusterFor(in, v.path, v.unit.Line, v.unit.EndLine)
		if cluster == "" || v.count != 1 {
			continue
		}
		kind := v.kind
		if kind == "parameter" {
			unchanged := false
			for _, old := range in.Before.Files[v.path].Units {
				if old.Name == v.unit.Name && slices.Equal(Structure(in.Before.Files[v.path], old).Header, Structure(in.After.Files[v.path], v.unit).Header) {
					unchanged = true
					break
				}
			}
			if unchanged {
				continue
			}
		}
		facts := []string{fmt.Sprintf("变化数 %d；%s", v.count, v.replacement)}
		facts = append(facts, "仅统计当前仓库可见结构；变化数为 1 不证明该间接层多余")
		advice := "先核对入口约定；若只需原样转发，把 " + location(v.path, v.unit) + " 的调用改用 " + v.replacement + "，再删除转发单元"
		switch kind {
		case "abstraction":
			advice = "先核对该抽象隔离的实际依赖或变化；若无需隔离，可将 " + location(v.path, v.unit) + " 的使用处改为 " + v.replacement + "，再删除抽象定义"
		case "parameter":
			advice = "先核对所有调用方及实参求值的副作用；若该位置确实恒定，可在 " + location(v.path, v.unit) + " 内使用 " + v.replacement + "，同步删除形参和各调用处对应的实参"
		}
		if kind == "forwarding" && in.Before.FrequencyOf(v.unit.Name) > 0 {
			kind = "old_name"
			facts = append(facts, fmt.Sprintf("起点引用单元数 %d；外部调用方需核对发布约定", in.Before.FrequencyOf(v.unit.Name)))
			advice = "先核对 " + location(v.path, v.unit) + " 的外部调用和兼容承诺；存在兼容要求时保留旧名，不能只凭转发结构建议删除"
		}
		out = append(out, Candidate{Kind: kind, Object: repostats.UnitKey(v.path, v.unit) + ":" + v.replacement, At: location(v.path, v.unit), Cluster: cluster, AgentOnly: agent, Facts: facts, Advice: advice, Features: map[string]float64{"variation": float64(v.count), "convention": prior[v.kind], "structure": in.After.Files[v.path].Confidence}, Benefit: float64(v.unit.EndLine - v.unit.Line + 1)})
	}
	return out, gaps, ctx.Err()
}

func unused(in Input) []Candidate {
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		for _, u := range f.Units {
			if !declaration(f, u) || in.Before.FrequencyOf(u.Name) > 0 || in.After.Entry(p, u) {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			if cluster == "" {
				continue
			}
			used := false
			if in.Queries != nil {
				count := in.Queries.Uses(u.Name)
				if !in.After.Roles[p].Test {
					for _, t := range f.Tokens {
						if t.Start >= u.Start && t.End <= u.End && units.Literal(t) == u.Name {
							count--
						}
					}
				}
				used = count > 0
			} else {
				for _, q := range keys(in.After.Files) {
					g := in.After.Files[q]
					for _, t := range g.Tokens {
						if p == q && t.Start >= u.Start && t.End <= u.End {
							continue
						}
						if units.Literal(t) == u.Name && !in.After.Roles[q].Test {
							used = true
						}
					}
				}
			}
			if !used {
				out = append(out, Candidate{Kind: "unused", Object: repostats.UnitKey(p, u), At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"新增标识符 " + u.Name + " 在定义它的单元之外未被其他非测试内容引用，也未匹配任务开始时识别出的入口写法"}, Advice: "撤回 " + location(p, u) + "，或核对配置和外部调用", Features: map[string]float64{"unused": 1, "structure": f.Confidence}, Benefit: float64(u.EndLine - u.Line + 1)})
			}
		}
	}
	return out
}
