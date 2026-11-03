package signals

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func cases(in Input) ([]Case, map[string]int, []Candidate, []string) {
	var all []Case
	budget := map[string]int{}
	var out []Candidate
	var gaps []string
	seen := map[string]Case{}
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		for _, u := range f.Units {
			if !in.After.TestUnits[repostats.UnitKey(p, u)] {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			old := false
			for _, v := range in.Before.Files[p].Units {
				old = old || v.Name == u.Name
			}
			c := Case{ID: units.Hash(p, u.ID), Name: u.Name, Path: p, Unit: u.ID, Start: u.Start, End: u.End, Added: !old, AgentOnly: agent, Retained: old || !agent, Shape: strings.Join(units.Sequence(u.Tokens, true), " ")}
			var assertionShape []string
			for _, token := range u.Tokens {
				if token.End > u.HeaderEnd {
					assertionShape = append(assertionShape, units.Sequence([]units.Token{token}, true)...)
				}
			}
			c.Shape = strings.Join(assertionShape, " ")
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
				if t.Kind == "number" || t.Kind == "key" || t.Kind == "string" {
					c.Inputs = append(c.Inputs, units.Literal(t))
				}
			}
			for _, q := range keys(in.After.Files) {
				if in.After.Roles[q].Test {
					continue
				}
				for _, v := range in.After.Files[q].Units {
					if !names[v.Name] || v.Name == "" {
						continue
					}
					c.Targets = append(c.Targets, location(q, v))
					for i, t := range v.Tokens {
						switch t.Text {
						case "<", "<=", "==", "!=", ">", ">=":
							for _, j := range []int{i - 1, i + 1} {
								if j < 0 || j >= len(v.Tokens) {
									continue
								}
								adj := v.Tokens[j]
								if adj.Kind == "number" {
									if n, err := strconv.ParseFloat(adj.Text, 64); err == nil {
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
			budget[key] = len(constants)*3 + len(constants) + 1 + len(stringsSet) + 1
			if len(constants) == 0 && len(stringsSet) == 0 {
				gaps = append(gaps, "等价类未知："+location(p, u))
			} else {
				identity := key + "\x00" + c.Class + "\x00" + c.Shape
				if prev, ok := seen[identity]; ok && c.Added && c.AgentOnly && !c.Retained {
					out = append(out, Candidate{Kind: "test_equivalence", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: true, Facts: []string{"与 " + prev.Name + " 的被测单元、等价类和断言形状相同"}, Advice: "核对断言后合并用例 " + c.Name + " 与 " + prev.Name, Features: map[string]float64{"equivalent": 1, "structure": f.Confidence}, Benefit: float64(u.EndLine - u.Line + 1)})
				}
				seen[identity] = c
			}
			asserted := false
			for _, t := range u.Tokens {
				if in.After.TestWords[t.Text] > 0 {
					asserted = true
				}
			}
			if !asserted && c.Added {
				out = append(out, Candidate{Kind: "assertion", Object: c.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"用例中没有学到的断言词"}, Advice: "核对 " + c.Name + " 是否检查了行为结果", Features: map[string]float64{"assertion_missing": 1}, Benefit: 1})
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
					child.Retained = existed || !child.AgentOnly
					child.Shape = strings.Join(units.Sequence(row.Tokens, true), " ")
					child.Inputs = nil
					for _, t := range row.Tokens {
						if t.Kind == "number" || t.Kind == "key" || t.Kind == "string" {
							child.Inputs = append(child.Inputs, units.Literal(t))
						}
					}
					child.Class, child.Boundary = equivalence(child.Inputs, constants, stringsSet)
					identity := strings.Join(child.Targets, "|") + "\x00" + child.Class + "\x00" + child.Shape
					if previous, ok := seen[identity]; ok && child.Added && !child.Retained && (len(constants) > 0 || len(stringsSet) > 0) {
						out = append(out, Candidate{Kind: "test_equivalence", Object: child.ID, At: p + ":" + child.Name, Cluster: cluster, AgentOnly: child.AgentOnly, Facts: []string{"与 " + previous.Name + " 的表格骨架和等价类相同"}, Advice: "核对断言后合并表格行 " + child.Name + " 与 " + previous.Name, Features: map[string]float64{"equivalent": 1}, Benefit: 1})
					}
					seen[identity] = child
					all = append(all, child)
				}
			}
		}
	}
	return all, budget, out, gaps
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
