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

func cases(ctx context.Context, in Input, purposesOnly bool) ([]Case, map[string]int, []Candidate, []string, error) {
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
		if !ok || c.checksUnknown {
			prior, ok = seen[identity]
		}
		if ok && c.Added && c.AgentOnly && !c.Retained {
			priorAt := prior.Path + ":" + prior.Name
			facts := []string{"与 " + priorAt + " 的被测单元、等价类和形状相同", "被测单元：" + strings.Join(c.Targets, "|") + "；等价类：" + c.Class}
			advice := "比较两处的输入、输出约定、副作用、权限、时序和共享状态，仅在没有独特检查时合并"
			object := "测试侧调用"
			if row {
				object = "表格行内容和用例的测试侧调用"
			}
			switch {
			case c.checksUnknown || prior.checksUnknown || len(c.checks) == 0 || len(prior.checks) == 0:
				facts = append(facts, "未能完整定位两处的测试侧调用，断言是否重复未知")
				advice = "保留两处检查，先核对实际断言及其副作用"
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
			reviewOnly := c.checksUnknown || prior.checksUnknown || len(c.checks) == 0 || !slices.Equal(c.checks, prior.checks) || prior.Path > c.Path || prior.Path == c.Path && prior.Start >= c.Start
			out = append(out, Candidate{Kind: "test_equivalence", Object: c.ID, At: c.Path + ":" + c.Name, Cluster: c.Cluster, AgentOnly: true, ReviewOnly: reviewOnly, Facts: facts, Advice: advice, Features: features, Benefit: float64(benefit)})
		}
		if _, exists := seen[identity]; !exists {
			seen[identity] = c
		}
		if _, exists := seenExact[key]; !exists && !c.checksUnknown && len(c.checks) > 0 {
			seenExact[key] = c
		}
	}
	type target struct {
		path string
		unit units.Unit
	}
	targets := map[string][]target{}
	for _, p := range keys(in.After.Files) {
		if purposesOnly || in.After.Roles[p].Test {
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
			// 预览差异只包含已核对请求的拟写入内容，不代表编辑已经发生。
			agent = agent || in.EditPreview && cluster != ""
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
			var body []units.Token
			for _, t := range u.Tokens {
				if t.Start >= u.HeaderEnd {
					body = append(body, t)
				}
			}
			var mockExample string
			c.checks, mockExample, c.checksUnknown = assertionExpressions(in.After, u.Name, body)
			if len(constants) > 0 || len(stringsSet) > 0 {
				comparisons = append(comparisons, comparison{c, key + "\x00" + c.Class + "\x00" + c.Shape, false, u.EndLine - u.Line + 1})
			}
			if mockExample != "" && c.Added && (!in.EditPreview || !c.Retained) {
				candidate := Candidate{Kind: "mock_only", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"测试侧调用的参数只包含从测试中学到的词：" + mockExample}, Advice: "核对 " + c.Name + " 中的这处调用检查了什么，以及删去用例会漏掉哪种错误；若次数或顺序属于需求，应保留对应检查", Features: map[string]float64{"mock_only": 1}, Benefit: 1}
				if in.EditPreview {
					candidate.Facts = append(candidate.Facts, "词汇匹配不能确认这是 mock，也不能证明用例只检查内部实现或没有独特作用")
					candidate.Advice = "先核对 " + c.Name + " 承担的当前需求、实际输入、期望结果或副作用，以及删去会漏掉哪种错误；调用次数或顺序属于约定时保留；确认没有独特检查后才考虑省去本次拟新增用例，依据不足时保留必要检查，不因此追加断言、辅助设施或验证"
				}
				out = append(out, candidate)
			}
			if len(c.checks) == 0 && !c.checksUnknown && c.Added && (!in.EditPreview || !c.Retained) {
				candidate := Candidate{Kind: "assertion", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"用例中未出现从仓库识别出的断言词"}, Advice: "核对 " + c.Name + " 是否检查了行为结果", Features: map[string]float64{"assertion_missing": 1}, Benefit: 1}
				if in.EditPreview {
					candidate.Facts = append(candidate.Facts, "未识别到断言词不证明没有结果检查，测试运行器或其他写法也可能承担检查")
					candidate.Advice = "先核对 " + c.Name + " 承担的当前需求、实际输入、期望结果或副作用，以及删去会漏掉哪种错误；确认没有独特检查后才考虑省去本次拟新增用例，依据不足时保留必要检查，不因此追加断言、辅助设施或验证"
				}
				out = append(out, candidate)
			}
			all = append(all, c)
			if purposesOnly {
				continue
			}
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
					child.checks = append([]string{strings.Join(units.Sequence(row.Tokens, false), " ")}, c.checks...)
					child.checksUnknown = c.checksUnknown || len(c.checks) == 0
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
			candidate := Candidate{Kind: "test_budget", Object: c.ID, At: c.Path + ":" + c.Name, Cluster: c.Cluster, AgentOnly: true, Facts: []string{fmt.Sprintf("新增用例数超过按等价类和边界点计算的建议数量 %d；等价类 %s", limit, c.Class)}, Advice: "核对 " + c.Name + " 是否增加了不同的行为检查；建议数量只用于提示", Features: map[string]float64{"budget_excess": float64(used[key] - limit)}, Benefit: 1}
			if in.EditPreview {
				candidate.Facts = append(candidate.Facts, "被测单元："+strings.Join(c.Targets, "|"), "建议数量不是上限，超过不证明用例多余或断言重复")
				candidate.Advice = "先核对 " + c.Name + " 承担的当前需求、不同输入、期望结果或副作用，以及省去会失去的独特检查；确认没有独特检查后才考虑省去本次拟新增用例或表格行，依据不足时保留必要检查，不因此追加断言、辅助设施或验证"
			}
			out = append(out, candidate)
		}
	}
	return all, budget, out, gaps, ctx.Err()
}

// 断言比较需要完整的调用文本。大小比较保留在实参中，不用尖括号截断表达式。
func assertionExpressions(repo repostats.Repository, caseName string, body []units.Token) (checks []string, mock string, unknown bool) {
	ts := units.Significant(body)
	ends := make([]int, len(ts))
	var stack []int
	for i, t := range ts {
		switch t.Text {
		case "(", "[", "{":
			stack = append(stack, i)
		case ")", "]", "}":
			if len(stack) == 0 {
				continue
			}
			open := stack[len(stack)-1]
			switch ts[open].Text + t.Text {
			case "()", "[]", "{}":
				ends[open] = i
				stack = stack[:len(stack)-1]
			default:
				stack = stack[:0]
			}
		}
	}
	covered := -1
	for i, t := range ts {
		if t.Kind != "identifier" || t.Text == caseName || i+1 >= len(ts) {
			continue
		}
		open := ts[i+1].Text
		if (open != "(" && open != "[" && open != "<") || repo.TestWord(t.Text, false) <= 0 {
			continue
		}
		end := ends[i+1]
		for end > 0 && end+1 < len(ts) && (ts[end+1].Text == "(" || ts[end+1].Text == "[") {
			end = ends[end+1]
		}
		if end == 0 {
			// 已核对的外层调用包含这段文本；内部的比较不构成另一处未读全的断言。
			unknown = unknown || i > covered
			continue
		}
		covered = max(covered, end)
		args := strings.Join(units.Sequence(ts[i+2:end], false), " ")
		expression := t.Text + open + args + ts[end].Text
		checks = append(checks, expression)
		if mock == "" {
			words := units.Words(args)
			if len(words) > 0 && !slices.ContainsFunc(words, func(w string) bool { return repo.TestWord(w, true) <= 0 }) {
				mock = expression
			}
		}
	}
	return checks, mock, unknown
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
