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
				cooccurrences := map[string]map[string]bool{}
				for _, m := range members {
					for _, v := range headsFor(m) {
						if err := ctx.Err(); err != nil {
							return out, gaps, err
						}
						if v.id == repostats.UnitKey(p, u) || v.name == u.Name {
							continue
						}
						for word := range v.words {
							if cooccurrences[word] == nil {
								cooccurrences[word] = map[string]bool{}
							}
							cooccurrences[word][m] = true
						}
					}
				}
				implementations := map[string]bool{}
				for word, present := range cooccurrences {
					if word == u.Name || ignored[word] || slices.Contains(members, word) {
						continue
					}
					if len(present) == len(members) && definedName(word) {
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
				if shape.BodyCalls == 1 && slices.Equal(body[1:], params) {
					out = append(out, indirection{p, u, "forwarding", 1, body[0]})
				}
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
			for _, site := range sites {
				for i, arg := range site.args {
					if site.path == p && arg.Start <= u.HeaderEnd && arg.End >= u.Start {
						continue
					}
					if positions[i] == nil {
						positions[i] = map[string]bool{}
					}
					positions[i][arg.Text] = true
				}
			}
			for i := 0; i < len(positions); i++ {
				values := positions[i]
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
		facts := []string{fmt.Sprintf("变化数 %d；%s", v.count, v.replacement)}
		if kind == "forwarding" && in.Before.FrequencyOf(v.unit.Name) > 0 {
			kind = "old_name"
			facts = append(facts, fmt.Sprintf("起点引用单元数 %d；外部调用方需核对发布约定", in.Before.FrequencyOf(v.unit.Name)))
		}
		out = append(out, Candidate{Kind: kind, Object: repostats.UnitKey(v.path, v.unit) + ":" + v.replacement, At: location(v.path, v.unit), Cluster: cluster, AgentOnly: agent, Facts: facts, Advice: "把 " + location(v.path, v.unit) + " 的调用改用 " + v.replacement + "，核对后删除该间接层", Features: map[string]float64{"variation": float64(v.count), "convention": prior[v.kind], "structure": in.After.Files[v.path].Confidence}, Benefit: float64(v.unit.EndLine - v.unit.Line + 1)})
	}
	return out, gaps, ctx.Err()
}

func unused(in Input) []Candidate {
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		for _, u := range f.Units {
			if u.Name == "" || in.Before.FrequencyOf(u.Name) > 0 || in.After.Entry(p, u) {
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
