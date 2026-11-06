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
	sites       []string
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
				out = append(out, indirection{path: p, unit: u, kind: "abstraction", count: len(implementations), replacement: strings.Join(keys(implementations), ", ")})
			}
			if len(body) > 0 {
				var params []string
				for _, w := range header {
					if w != u.Name {
						params = append(params, w)
					}
				}
				if shape.Callable && shape.BodyCalls == 1 && slices.Equal(body[1:], params) {
					out = append(out, indirection{path: p, unit: u, kind: "forwarding", count: 1, replacement: body[0]})
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
			locations := map[int]map[string]bool{}
			for _, site := range sites {
				for i, arg := range site.args {
					if site.path == p && arg.Start <= u.HeaderEnd && arg.End >= u.Start {
						continue
					}
					if positions[i] == nil {
						positions[i] = map[string]bool{}
						locations[i] = map[string]bool{}
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
					if affected != nil {
						locations[i][fmt.Sprintf("%s:%d", site.path, arg.Line)] = true
					}
				}
			}
			for i := 0; i < len(positions); i++ {
				values := positions[i]
				if unknown[i] {
					continue
				}
				if len(values) > 0 {
					out = append(out, indirection{path: p, unit: u, kind: "parameter", count: len(values), replacement: fmt.Sprintf("参数 %d = %s", i+1, strings.Join(keys(values), ", ")), sites: keys(locations[i])})
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
		at := location(v.path, v.unit)
		var facts []string
		var advice string
		switch kind {
		case "abstraction":
			facts = append(facts, "按成员名与首行共现识别出一个实现候选："+v.replacement+"；未确认类型关系或隔离用途")
			advice = "核对 " + at + " 是否承担当前需求中的依赖隔离或替换约定；确认不需要、且调用方可直接使用 " + v.replacement + " 后，再调整引用并删除该抽象"
		case "parameter":
			facts = append(facts, "按同名调用形状统计，该位置只有一种实参写法："+v.replacement+"；相同写法不代表运行值固定", fmt.Sprintf("调用形状共记录 %d 处位置；位置示例：%s", len(v.sites), v.sites[0]))
			advice = "核对 " + at + " 的" + v.replacement + " 是否需要由调用方提供；确认调用属于该声明、取值固定、求值行为不变且调用范围完整后，才将值移入实现并删除对应形参和各调用实参，保留函数及其他参数"
		case "forwarding":
			facts = append(facts, "主体标识符序列符合转发形状，目标名字为 "+v.replacement+"；未核对运算符、字面量或调用语义")
			advice = "核对 " + at + " 的独立职责、返回结果和副作用；确认直接调用 " + v.replacement + " 行为等价且无需保留该层后，再修改调用并删除此包装"
		}
		if kind == "forwarding" && in.Before.FrequencyOf(v.unit.Name) > 0 {
			kind = "old_name"
			facts = append(facts, fmt.Sprintf("该名字在起点出现于 %d 个单元；外部调用与兼容承诺尚未确认", in.Before.FrequencyOf(v.unit.Name)))
			advice = "先核对 " + at + " 的仓库内引用、外部调用方和发布兼容约定；仍需兼容时保留旧名，确认无需兼容且行为等价后，才将调用迁移到 " + v.replacement + " 并删除旧名包装"
		}
		related, unknown := map[string]bool{}, map[string]bool{}
		known := false
		for _, region := range in.Regions {
			if !region.Known {
				unknown[region.Requirement] = true
				continue
			}
			known = true
			if slices.Contains(region.Clusters, cluster) {
				related[region.Requirement] = true
			}
		}
		if len(related) > 0 {
			facts = append(facts, "位于需求相关区："+strings.Join(keys(related), "、")+"；词汇关联不证明实现有必要或已获授权")
		} else if known {
			facts = append(facts, "未落入已知需求相关区；不能据此认定没有实际用途")
		}
		if len(unknown) > 0 {
			facts = append(facts, "需求相关区未知："+strings.Join(keys(unknown), "、"))
		} else if !known {
			facts = append(facts, "未取得需求相关区，实际用途待核对")
		}
		facts = append(facts, "以上仅为静态结构信号，尚未确认可以精简")
		if !agent {
			advice = "该变更簇不能全部核对为 Agent 编辑，保留当前内容；仅结合以上结构信号核对实际用途"
		}
		out = append(out, Candidate{Kind: kind, Object: repostats.UnitKey(v.path, v.unit) + ":" + v.replacement, At: at, Cluster: cluster, AgentOnly: agent, Facts: facts, Advice: advice, Features: map[string]float64{"variation": float64(v.count), "convention": prior[v.kind], "structure": in.After.Files[v.path].Confidence}, Benefit: float64(v.unit.EndLine - v.unit.Line + 1)})
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
