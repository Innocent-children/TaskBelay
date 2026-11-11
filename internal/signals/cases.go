package signals

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
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
		firstCase := len(all)
		caseUnits := repostats.CaseUnits(in.After, p)
		callers := map[string]map[string]bool{}
		for _, u := range caseUnits {
			if err := ctx.Err(); err != nil {
				return all, budget, out, gaps, err
			}
			if !declaration(f, u) {
				continue
			}
			body := 0
			for body < len(u.Tokens) && u.Tokens[body].Start < u.HeaderEnd {
				body++
			}
			for name := range units.Calls(u.Tokens[body:]) {
				if callers[name] == nil {
					callers[name] = map[string]bool{}
				}
				callers[name][u.ID] = true
			}
		}
		for _, u := range caseUnits {
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
			// 测试内容也包含构造和执行辅助逻辑；其他声明直接调用它不证明它是独立用例。
			if len(callers[u.Name]) > 1 || len(callers[u.Name]) == 1 && !callers[u.Name][u.ID] {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			// 预览差异只包含已核对请求的拟写入内容，不代表编辑已经发生。
			agent = agent || in.EditPreview && cluster != ""
			old := false
			for _, v := range in.Before.Files[p].Units {
				old = old || v.Name == u.Name
			}
			c := Case{ID: units.Hash(p, u.ID), Name: u.Name, Path: p, Unit: u.ID, Cluster: cluster, Start: u.Start, End: u.End, Added: !old, AgentOnly: agent, Retained: !agent, Shape: strings.Join(units.Sequence(u.Tokens, true), " ")}
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
			if purposesOnly && !c.Retained && f.Layout == "brackets" {
				out = append(out, copiedTestLines(ctx, in, f, u, c)...)
			}
			c.Retained = c.Retained || old
			var numbers []float64
			stringsSet := map[string]bool{}
			names := map[string]bool{}
			for _, t := range u.Tokens {
				if t.Kind == "identifier" {
					names[t.Text] = true
				}
			}
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
			boundaryPoints := map[float64]bool{}
			for _, n := range constants {
				boundaryPoints[n-1] = true
				boundaryPoints[n] = true
				boundaryPoints[n+1] = true
			}
			c.Inputs, c.Class, c.Boundary = equivalence(u.Tokens, constants, boundaryPoints, stringsSet)
			key := strings.Join(c.Targets, "|")
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
			c.checks, c.checksUnknown = assertionExpressions(in.After, u.Name, body)
			if len(constants) > 0 || len(stringsSet) > 0 {
				comparisons = append(comparisons, comparison{c, key + "\x00" + c.Class + "\x00" + c.Shape, false, u.EndLine - u.Line + 1})
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
					child.Inputs, child.Class, child.Boundary = equivalence(row.Tokens, constants, boundaryPoints, stringsSet)
					identity := strings.Join(child.Targets, "|") + "\x00" + child.Class + "\x00" + child.Shape
					if len(constants) > 0 || len(stringsSet) > 0 {
						comparisons = append(comparisons, comparison{child, "row\x00" + identity, true, 1})
					}
					all = append(all, child)
				}
			}
		}
		if purposesOnly {
			out = append(out, copiedTestCases(ctx, f, caseUnits, all[firstCase:])...)
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

/**
 * 既有用例只核对纯插入。首次插入之前的行差可定位原声明，沿用按原用例编号作出的保留决定。
 */
func caseInsertions(in Input, path string, test units.Unit) []changes.Span {
	var insertions []changes.Span
	for _, h := range in.Changes.Hunks {
		s := h.Span
		if h.Path != path || s.AfterEnd < test.Line || s.AfterStart >= test.EndLine {
			continue
		}
		if s.BeforeStart != s.BeforeEnd || h.Kind == "move" || s.AfterStart < test.Line {
			return nil
		}
		insertions = append(insertions, s)
	}
	if len(in.Retained) > 0 && len(insertions) > 0 {
		beforeLine := test.Line + insertions[0].BeforeStart - insertions[0].AfterStart
		for _, u := range in.Before.Files[path].Units {
			if u.Line == beforeLine && u.Name == test.Name && slices.Contains(in.Retained, units.Hash(path, u.ID)) {
				return nil
			}
		}
	}
	return insertions
}

/**
 * 在同一张表中比较数据行，在用例主体中比较连续的相同调用。
 * 优先以未改动的行为参照，位置、次数和执行顺序的用途仍需由 Agent 核对。
 */
func copiedTestLines(ctx context.Context, in Input, f units.File, test units.Unit, c Case) []Candidate {
	var insertions []changes.Span
	if !c.Added {
		insertions = caseInsertions(in, c.Path, test)
		if len(insertions) == 0 {
			return nil
		}
	}
	type rowText struct {
		parent, callStart int
		text              string
	}
	type proposedRow struct {
		key   rowText
		index int
	}
	first := map[rowText]int{}
	var proposed []proposedRow
	lastCall, callStart := 0, 0
	start := sort.Search(len(f.Units), func(i int) bool { return f.Units[i].Start > test.HeaderEnd })
	for i := start; i < len(f.Units); i++ {
		row := f.Units[i]
		if row.Start >= test.End || ctx.Err() != nil {
			break
		}
		if row.Parent < 0 || row.Line != row.EndLine {
			continue
		}
		parent := f.Units[row.Parent]
		line := strings.TrimSpace(f.Lines[row.Line-1])
		key := rowText{parent: row.Parent, text: line}
		if body, comma := strings.CutSuffix(line, ","); comma {
			if parent.Tokens[len(parent.Tokens)-1].Text == ")" || len(strings.TrimSpace(body)) != row.End-row.Start || !literalRow(row.Tokens) {
				continue
			}
		} else {
			body := strings.TrimSpace(strings.TrimSuffix(line, ";"))
			if parent.ID != test.ID || len(body) != row.End-row.Start || len(row.Tokens) < 4 ||
				row.Tokens[0].Kind != "identifier" || row.Tokens[1].Text != "(" || !literalRow(row.Tokens[1:]) {
				continue
			}
			if lastCall == 0 || lastCall+1 != row.Line || f.Lines[lastCall-1] != f.Lines[row.Line-1] {
				callStart = row.Line
			}
			lastCall, key.callStart = row.Line, callStart
		}
		// 先收集未改动的行，即使它排在新行之后，也优先作为参照。
		for len(insertions) > 0 && insertions[0].AfterEnd < row.Line {
			insertions = insertions[1:]
		}
		inserted := len(insertions) > 0 && insertions[0].AfterStart < row.Line
		if inserted || c.Added && first[key] != 0 {
			proposed = append(proposed, proposedRow{key, i})
		} else if first[key] == 0 {
			first[key] = row.Line
		}
	}
	var out []Candidate
	for _, item := range proposed {
		if ctx.Err() != nil {
			return nil
		}
		row := f.Units[item.index]
		prior := first[item.key]
		if prior == 0 {
			first[item.key] = row.Line
			continue
		}
		cluster, _ := clusterFor(in, c.Path, row.Line, row.Line)
		if cluster != "" {
			var fact, advice string
			if item.key.callStart == 0 {
				fact = fmt.Sprintf("同一组表格数据中，第 %d 行与第 %d 行的字面量和排列完全相同", row.Line, prior)
				advice = fmt.Sprintf("核对两行是否各自承担一次必要检查。行的位置、次数、顺序或共享状态有意义时保留；确认重复后可省去第 %d 行", row.Line)
			} else {
				fact = fmt.Sprintf("第 %d 行与第 %d 行属于一段连续的相同调用，调用名 %s 和字面量实参均相同", row.Line, prior, row.Tokens[0].Text)
				advice = fmt.Sprintf("核对再次调用检查了什么。依赖状态变化、幂等性或执行次数时保留；确认只重复同一项检查后可省去第 %d 行", row.Line)
			}
			out = append(out, Candidate{
				Kind: "test_copy", Object: units.Hash(c.ID, row.ID), At: fmt.Sprintf("%s:%s/%d", c.Path, c.Name, row.Line), Cluster: cluster, AgentOnly: true,
				Facts:  []string{fact},
				Advice: advice,
			})
		}
	}
	return out
}

func literalRow(tokens []units.Token) bool {
	if len(tokens) < 3 {
		return false
	}
	switch tokens[0].Text + tokens[len(tokens)-1].Text {
	case "{}", "[]", "()":
	default:
		return false
	}
	fields := tokens[1 : len(tokens)-1]
	inRecord := false
	for len(fields) > 0 {
		// 一列可以是 Name{Field: literal, ...}，字段值仍只接受字面量。
		if !inRecord && len(fields) > 1 && fields[0].Kind == "identifier" && fields[1].Text == "{" {
			inRecord = true
			fields = fields[2:]
		}
		if inRecord {
			if len(fields) < 3 || fields[0].Kind != "identifier" || fields[1].Text != ":" {
				return false
			}
			fields = fields[2:]
		}
		if len(fields) > 1 && fields[0].Text == "-" && fields[1].Kind == "number" {
			fields = fields[1:]
		}
		switch fields[0].Kind {
		case "number", "string", "key":
		default:
			return false
		}
		fields = fields[1:]
		if inRecord && len(fields) > 0 && fields[0].Text == "}" {
			inRecord = false
			fields = fields[1:]
		}
		if len(fields) == 0 {
			return !inRecord
		}
		if fields[0].Text != "," {
			return false
		}
		fields = fields[1:]
	}
	return false
}

/**
 * 只比较本文件已识别的独立用例。跳过声明名，其余原文逐字比较，
 * 避免把不同实参、断言、注释或缩进当作重复。
 */
func copiedTestCases(ctx context.Context, f units.File, caseUnits []units.Unit, cases []Case) []Candidate {
	if len(cases) < 2 || !slices.ContainsFunc(cases, func(c Case) bool { return c.Added && c.AgentOnly && !c.Retained }) {
		return nil
	}
	byUnit := make(map[string]Case, len(cases))
	for _, c := range cases {
		byUnit[c.Unit] = c
	}
	type declarationText struct {
		parent         int
		prefix, suffix string
	}
	source := strings.Join(f.Lines, "")
	groups := map[declarationText][]Case{}
	for _, u := range caseUnits {
		if ctx.Err() != nil {
			return nil
		}
		c, ok := byUnit[u.ID]
		if !ok {
			continue
		}
		for i, token := range u.Tokens {
			if token.End > u.HeaderEnd {
				break
			}
			if token.Kind == "identifier" && token.Text == u.Name && i+1 < len(u.Tokens) && u.Tokens[i+1].Text == "(" {
				key := declarationText{u.Parent, source[u.Start:token.Start], source[token.End:u.End]}
				groups[key] = append(groups[key], c)
				break
			}
		}
	}
	var out []Candidate
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		reference := group[0]
		for _, c := range group {
			if c.Retained {
				reference = c
				break
			}
		}
		for _, c := range group {
			if c.Name == reference.Name || !c.Added || !c.AgentOnly || c.Retained {
				continue
			}
			out = append(out, Candidate{
				Kind: "test_copy", Object: c.ID, At: c.Path + ":" + c.Name, Cluster: c.Cluster, AgentOnly: true,
				Facts:  []string{"与 " + reference.Path + ":" + reference.Name + " 同属一个文件和上级节点，两处声明原文只有用例名不同"},
				Advice: "名称、标注或夹具选择了不同场景，或需要重复执行来检查时序和共享状态时，两条都保留。确认它们只承担同一项检查后，保留 " + reference.Name + "，省去 " + c.Name + " 及其单独运行",
			})
		}
	}
	return out
}

// 断言比较需要完整的调用文本。大小比较保留在实参中，不用尖括号截断表达式。
func assertionExpressions(repo repostats.Repository, caseName string, body []units.Token) (checks []string, unknown bool) {
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
	}
	return checks, unknown
}

func equivalence(tokens []units.Token, constants []float64, boundaryPoints map[float64]bool, stringsSet map[string]bool) (inputs []string, class string, boundary []string) {
	tokens = units.Significant(tokens)
	var classes []string
	for i, t := range tokens {
		switch t.Kind {
		case "key", "string":
			value := units.Literal(t)
			inputs = append(inputs, value)
			if stringsSet[value] {
				classes = append(classes, "equal:"+value)
				boundary = append(boundary, value)
			} else {
				classes = append(classes, "string:other")
			}
		case "number":
			value := t.Text
			if i > 0 && tokens[i-1].Text == "-" && (i < 2 || tokens[i-2].Kind == "symbol") {
				value = "-" + value
			}
			inputs = append(inputs, value)
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				classes = append(classes, "literal:"+value)
				continue
			}
			at, exact := slices.BinarySearch(constants, n)
			switch {
			case exact:
				classes = append(classes, "equal:"+value)
			case boundaryPoints[n]:
				classes = append(classes, "boundary:"+value)
			default:
				classes = append(classes, fmt.Sprintf("interval:%d", at))
				continue
			}
			boundary = append(boundary, value)
		}
	}
	return inputs, strings.Join(classes, "|"), boundary
}
