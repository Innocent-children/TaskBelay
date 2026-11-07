package signals

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func cases(ctx context.Context, in Input) ([]Case, map[string]int, []Candidate, []string, error) {
	var all []Case
	budget := map[string]int{}
	var out []Candidate
	var gaps []string
	type comparison struct {
		c       Case
		key     string
		row     bool
		benefit int
	}
	var comparisons []comparison
	seen := map[string]Case{}
	seenExact := map[string]Case{}
	compare := func(c Case, identity string, row bool, benefit int) {
		key := identity + "\x00" + units.Hash(c.checks...)
		prior, ok := seenExact[key]
		if !ok {
			prior, ok = seen[identity]
		}
		if ok && c.Added && c.AgentOnly && !c.Retained {
			priorAt := prior.Path + ":" + prior.Name
			facts := []string{"与 " + priorAt + " 的被测单元、等价类和形状相同", "被测单元：" + strings.Join(c.Targets, "|") + "；等价类：" + c.Class}
			advice := "比较两处的输入、输出约定、副作用、权限、时序和共享状态，仅在没有独特检查时合并"
			object := "测试侧调用"
			if row {
				object = "表格行内容"
			}
			switch {
			case len(c.checks) == 0 && len(prior.checks) == 0:
				facts = append(facts, "尚未定位可比较的测试侧调用，断言是否重复未知")
			case slices.Equal(prior.checks, c.checks):
				facts = append(facts, object+"相同："+strings.Join(c.checks, "、"))
			default:
				facts = append(facts, priorAt+" 的"+object+"："+strings.Join(prior.checks, "、"), c.Name+" 的"+object+"："+strings.Join(c.checks, "、"))
				advice = "保留这两处检查，先核对上述差异对应的实际输入、期望结果或副作用"
			}
			features := map[string]float64{"equivalent": 1}
			if !row {
				features["structure"] = in.After.Files[c.Path].Confidence
			}
			// 新找到的后置参照和不同断言只供核对，不能增加撤回试验或据删除动作校准。
			reviewOnly := len(c.checks) == 0 || !slices.Equal(c.checks, prior.checks) || prior.Path > c.Path || prior.Path == c.Path && prior.Start >= c.Start
			out = append(out, Candidate{Kind: "test_equivalence", Object: c.ID, At: c.Path + ":" + c.Name, Cluster: c.Cluster, AgentOnly: true, ReviewOnly: reviewOnly, Facts: facts, Advice: advice, Features: features, Benefit: float64(benefit)})
		}
		if _, exists := seen[identity]; !exists {
			seen[identity] = c
		}
		if _, exists := seenExact[key]; !exists {
			seenExact[key] = c
		}
	}
	type target struct {
		path string
		unit units.Unit
	}
	targets := map[string][]target{}
	for _, p := range keys(in.After.Files) {
		if in.After.Roles[p].Test {
			continue
		}
		for _, u := range in.After.Files[p].Units {
			if u.Name != "" {
				targets[u.Name] = append(targets[u.Name], target{p, u})
			}
		}
	}
	for _, p := range keys(in.After.Files) {
		if err := ctx.Err(); err != nil {
			return all, budget, out, gaps, err
		}
		f := in.After.Files[p]
		for _, u := range repostats.CaseUnits(in.After, p) {
			if !in.After.TestUnits[repostats.UnitKey(p, u)] {
				continue
			}
			if !declaration(f, u) {
				if cluster, _ := clusterFor(in, p, u.Line, u.EndLine); cluster != "" && u.Parent < 0 {
					gaps = append(gaps, "用例结构未知："+location(p, u))
				}
				continue
			}
			contained := false
			for _, parent := range f.Units {
				if parent.Name == u.Name && parent.Start <= u.Start && parent.End > u.End {
					contained = true
					break
				}
			}
			if contained {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			old := false
			for _, v := range in.Before.Files[p].Units {
				old = old || v.Name == u.Name
			}
			c := Case{ID: units.Hash(p, u.ID), Name: u.Name, Path: p, Unit: u.ID, Cluster: cluster, Start: u.Start, End: u.End, Added: !old, AgentOnly: agent, Retained: old || !agent, Shape: strings.Join(units.Sequence(u.Tokens, true), " ")}
			var assertionShape []string
			for _, token := range u.Tokens {
				if token.End > u.HeaderEnd {
					assertionShape = append(assertionShape, units.Sequence([]units.Token{token}, true)...)
				}
			}
			c.Shape = strings.Join(assertionShape, " ")
			for _, at := range in.Retained {
				c.Retained = c.Retained || at == c.ID || at == location(p, u) || at == "test:"+location(p, u)
			}
			for _, req := range in.Requirements {
				if u.Name != "" && (strings.Contains(req.Quote, u.Name) || strings.Contains(req.Acceptance, u.Name)) {
					c.Retained = true
				}
			}
			var numbers []float64
			stringsSet := map[string]bool{}
			names := map[string]bool{}
			for _, t := range u.Tokens {
				if t.Kind == "identifier" {
					names[t.Text] = true
				}
			}
			c.Inputs = inputValues(u.Tokens)
			for _, name := range keys(names) {
				for _, target := range targets[name] {
					q, v := target.path, target.unit
					if err := ctx.Err(); err != nil {
						return all, budget, out, gaps, err
					}
					c.Targets = append(c.Targets, location(q, v))
					tokens := units.Significant(v.Tokens)
					for i, t := range tokens {
						switch t.Text {
						case "<", "<=", "==", "!=", ">", ">=":
							for _, j := range []int{i - 1, i + 1} {
								if j < 0 || j >= len(tokens) {
									continue
								}
								adj := tokens[j]
								negative := false
								if adj.Text == "-" && j+1 < len(tokens) && j > i {
									adj = tokens[j+1]
									negative = true
								} else if j > 0 && tokens[j-1].Text == "-" {
									negative = true
								}
								if adj.Kind == "number" {
									if n, err := strconv.ParseFloat(adj.Text, 64); err == nil {
										if negative {
											n = -n
										}
										numbers = append(numbers, n)
									}
								} else if adj.Kind == "key" || adj.Kind == "string" {
									stringsSet[units.Literal(adj)] = true
								}
							}
						}
					}
				}
			}
			sort.Float64s(numbers)
			var constants []float64
			for _, n := range numbers {
				if len(constants) == 0 || constants[len(constants)-1] != n {
					constants = append(constants, n)
				}
			}
			sort.Strings(c.Targets)
			c.Class, c.Boundary = equivalence(c.Inputs, constants, stringsSet)
			key := strings.Join(c.Targets, "|")
			boundaryPoints := map[float64]bool{}
			for _, n := range constants {
				boundaryPoints[n-1] = true
				boundaryPoints[n] = true
				boundaryPoints[n+1] = true
			}
			if len(constants) == 0 && len(stringsSet) == 0 {
				if c.Added || cluster != "" {
					gaps = append(gaps, "等价类未知："+location(p, u))
				}
			} else {
				classes := 0
				if len(constants) > 0 {
					classes += 2*len(constants) + 1
				}
				if len(stringsSet) > 0 {
					classes += len(stringsSet) + 1
				}
				budget[key] = len(boundaryPoints) + len(stringsSet) + classes
			}
			asserted, mockExample := false, ""
			var body []units.Token
			for _, t := range u.Tokens {
				if t.Start >= u.HeaderEnd {
					body = append(body, t)
				}
			}
			calls := units.Calls(body)
			for _, name := range keys(calls) {
				if in.After.TestWord(name, false) <= 0 || name == u.Name {
					continue
				}
				asserted = true
				for _, args := range calls[name] {
					onlyTest := len(args) > 0
					seenWord := false
					var expressions []string
					for _, arg := range args {
						expressions = append(expressions, arg.Text)
						for _, word := range units.Words(arg.Text) {
							found := in.After.TestWord(word, true) > 0
							onlyTest = onlyTest && found
							seenWord = true
						}
					}
					expression := name + "(" + strings.Join(expressions, ", ") + ")"
					c.checks = append(c.checks, expression)
					if onlyTest && seenWord && mockExample == "" {
						mockExample = expression
					}
				}
			}
			if len(constants) > 0 || len(stringsSet) > 0 {
				comparisons = append(comparisons, comparison{c, key + "\x00" + c.Class + "\x00" + c.Shape, false, u.EndLine - u.Line + 1})
			}
			if mockExample != "" && c.Added {
				out = append(out, Candidate{Kind: "mock_only", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"测试侧调用的参数只包含从测试中学到的词：" + mockExample}, Advice: "核对 " + c.Name + " 中的这处调用检查了什么，以及删去用例会漏掉哪种错误；若次数或顺序属于需求，应保留对应检查", Features: map[string]float64{"mock_only": 1}, Benefit: 1})
			}
			if !asserted && c.Added {
				out = append(out, Candidate{Kind: "assertion", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"用例中未出现从仓库识别出的断言词"}, Advice: "核对 " + c.Name + " 是否检查了行为结果", Features: map[string]float64{"assertion_missing": 1}, Benefit: 1})
			}
			all = append(all, c)
			// 同一列表中骨架相同的兄弟元素逐行参与，避免把整张表当成一条用例。
			groups := map[string][]units.Unit{}
			for _, row := range f.Units {
				if row.Parent >= 0 && row.Start > u.HeaderEnd && row.End <= u.End {
					key := fmt.Sprint(row.Parent) + ":" + strings.Join(units.Sequence(row.Tokens, true), " ")
					groups[key] = append(groups[key], row)
				}
			}
			for _, key := range keys(groups) {
				rows := groups[key]
				if len(rows) < 2 {
					continue
				}
				for _, row := range rows {
					child := c
					child.ID = units.Hash(c.ID, row.ID)
					child.Name = fmt.Sprintf("%s/%d", c.Name, row.Line)
					child.Start, child.End = row.Start, row.End
					existed := false
					for _, previous := range in.Before.Files[p].Units {
						existed = existed || previous.ID == row.ID
					}
					child.Added = !existed
					child.Retained = c.Retained || existed || !child.AgentOnly
					child.Shape = strings.Join(units.Sequence(row.Tokens, true), " ")
					child.checks = []string{strings.Join(units.Sequence(row.Tokens, false), " ")}
					child.Inputs = inputValues(row.Tokens)
					child.Class, child.Boundary = equivalence(child.Inputs, constants, stringsSet)
					identity := strings.Join(child.Targets, "|") + "\x00" + child.Class + "\x00" + child.Shape
					if len(constants) > 0 || len(stringsSet) > 0 {
						comparisons = append(comparisons, comparison{child, "row\x00" + identity, true, 1})
					}
					all = append(all, child)
				}
			}
		}
	}
	// 先登记不能精简的用例，避免新文件排在既有用例前面时漏掉可比较的断言。
	for _, eligible := range []bool{false, true} {
		for _, item := range comparisons {
			if err := ctx.Err(); err != nil {
				return nil, nil, nil, nil, err
			}
			if (item.c.Added && item.c.AgentOnly && !item.c.Retained) == eligible {
				compare(item.c, item.key, item.row, item.benefit)
			}
		}
	}
	used := map[string]int{}
	for _, c := range all {
		if !c.Added {
			continue
		}
		parent := false
		for _, child := range all {
			parent = parent || child.Path == c.Path && child.Start > c.Start && child.End <= c.End
		}
		if parent {
			continue
		}
		key := strings.Join(c.Targets, "|")
		limit, known := budget[key]
		used[key]++
		if known && used[key] > limit && c.AgentOnly && !c.Retained {
			out = append(out, Candidate{Kind: "test_budget", Object: c.ID, At: c.Path + ":" + c.Name, Cluster: c.Cluster, AgentOnly: true, Facts: []string{fmt.Sprintf("新增用例数超过按等价类和边界点计算的建议数量 %d；等价类 %s", limit, c.Class)}, Advice: "核对 " + c.Name + " 是否增加了不同的行为检查；建议数量只用于提示", Features: map[string]float64{"budget_excess": float64(used[key] - limit)}, Benefit: 1})
		}
	}
	return all, budget, out, gaps, ctx.Err()
}

func equivalence(inputs []string, constants []float64, stringsSet map[string]bool) (string, []string) {
	var classes, boundary []string
	for _, value := range inputs {
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			at := sort.SearchFloat64s(constants, n)
			label := fmt.Sprintf("interval:%d", at)
			if at < len(constants) && constants[at] == n {
				label = "equal:" + value
			}
			classes = append(classes, label)
			for _, c := range constants {
				if n == c || n == c-1 || n == c+1 {
					boundary = append(boundary, value)
				}
			}
		} else if stringsSet[value] {
			classes = append(classes, "equal:"+value)
			boundary = append(boundary, value)
		} else {
			classes = append(classes, "string:other")
		}
	}
	return strings.Join(classes, "|"), boundary
}

func inputValues(tokens []units.Token) []string {
	tokens = units.Significant(tokens)
	var out []string
	for i, t := range tokens {
		if t.Kind != "number" && t.Kind != "key" && t.Kind != "string" {
			continue
		}
		value := units.Literal(t)
		if t.Kind == "number" && i > 0 && tokens[i-1].Text == "-" && (i < 2 || tokens[i-2].Kind == "symbol") {
			value = "-" + value
		}
		out = append(out, value)
	}
	return out
}
