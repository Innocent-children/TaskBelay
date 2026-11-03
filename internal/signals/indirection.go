package signals

import (
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

func measure(repo repostats.Repository) ([]indirection, []string) {
	var out []indirection
	var gaps []string
	type callSite struct {
		path string
		args []units.Token
	}
	allCalls := map[string][]callSite{}
	for _, p := range keys(repo.Files) {
		for name, sites := range units.Calls(repo.Files[p].Tokens) {
			for _, args := range sites {
				allCalls[name] = append(allCalls[name], callSite{p, args})
			}
		}
	}
	for _, p := range keys(repo.Files) {
		f := repo.Files[p]
		ignored := map[string]bool{}
		for _, w := range append(append([]string(nil), repo.SyntaxFor(p).Keywords...), repo.SyntaxFor(p).Returns...) {
			ignored[w] = true
		}
		for _, u := range f.Units {
			if u.Name == "" {
				continue
			}
			var body, header []string
			for _, t := range u.Tokens {
				if t.Kind != "identifier" || ignored[t.Text] {
					continue
				}
				if t.End <= u.HeaderEnd {
					header = append(header, t.Text)
				} else {
					body = append(body, t.Text)
				}
			}
			var members []string
			abstract := true
			for j, v := range f.Units {
				if v.Parent < 0 || f.Units[v.Parent].ID != u.ID || j == u.Parent {
					continue
				}
				if v.Name != "" {
					members = append(members, v.Name)
				}
				count := 0
				for _, t := range v.Tokens {
					if t.Start >= v.HeaderEnd && t.Kind == "identifier" {
						count++
					}
				}
				abstract = abstract && count <= 1
			}
			if abstract && len(members) > 0 {
				cooccurrences := map[string]map[string]bool{}
				for _, q := range keys(repo.Files) {
					for _, v := range repo.Files[q].Units {
						if v.ID == u.ID || v.Name == u.Name {
							continue
						}
						words := map[string]bool{}
						for _, t := range v.Tokens {
							if t.End <= v.HeaderEnd {
								words[t.Text] = true
							}
						}
						if v.Parent >= 0 {
							parent := repo.Files[q].Units[v.Parent]
							for _, t := range parent.Tokens {
								if t.End <= parent.HeaderEnd {
									words[t.Text] = true
								}
							}
						}
						for _, m := range members {
							if !words[m] {
								continue
							}
							for word := range words {
								if cooccurrences[word] == nil {
									cooccurrences[word] = map[string]bool{}
								}
								cooccurrences[word][m] = true
							}
						}
					}
				}
				implementations := map[string]bool{}
				for word, present := range cooccurrences {
					if word == u.Name || ignored[word] || slices.Contains(members, word) {
						continue
					}
					if len(present) == len(members) {
						for _, q := range keys(repo.Files) {
							for _, v := range repo.Files[q].Units {
								if v.Name == word {
									implementations[word] = true
								}
							}
						}
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
				if slices.Equal(body[1:], params) {
					out = append(out, indirection{p, u, "forwarding", 1, body[0]})
				}
			}
			if repo.SyntaxFor(p).CallRatio <= CallPrecision {
				gaps = append(gaps, "实参未知："+location(p, u))
				continue
			}
			sites := allCalls[u.Name]
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
	return out, gaps
}

func indirections(in Input) ([]Candidate, []string) {
	before, _ := measure(in.Before)
	after, gaps := measure(in.After)
	prior := map[string]float64{}
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
	var out []Candidate
	for _, v := range after {
		cluster, agent := clusterFor(in, v.path, v.unit.Line, v.unit.EndLine)
		if cluster == "" || v.count != 1 {
			continue
		}
		kind := v.kind
		facts := []string{fmt.Sprintf("变化数 %d；%s", v.count, v.replacement)}
		if kind == "forwarding" && in.Before.Frequency[v.unit.Name] > 0 {
			kind = "old_name"
			facts = append(facts, fmt.Sprintf("起点引用单元数 %d；外部调用方需核对发布约定", in.Before.Frequency[v.unit.Name]))
		}
		out = append(out, Candidate{Kind: kind, Object: v.unit.ID, At: location(v.path, v.unit), Cluster: cluster, AgentOnly: agent, Facts: facts, Advice: "把 " + location(v.path, v.unit) + " 的调用改用 " + v.replacement + "，核对后删除该间接层", Features: map[string]float64{"variation": float64(v.count), "convention": prior[v.kind], "structure": in.After.Files[v.path].Confidence}, Benefit: float64(v.unit.EndLine - v.unit.Line + 1)})
	}
	return out, gaps
}

func unused(in Input) []Candidate {
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		for _, u := range f.Units {
			if u.Name == "" || in.Before.Frequency[u.Name] > 0 || in.After.Entry(p, u) {
				continue
			}
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			if cluster == "" {
				continue
			}
			used := false
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
			if !used {
				out = append(out, Candidate{Kind: "unused", Object: u.ID, At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{"新名字 " + u.Name + " 在定义单元之外没有生产内容引用，也不符合起点入口模式"}, Advice: "撤回 " + location(p, u) + "，或核对配置和外部调用", Features: map[string]float64{"unused": 1, "structure": f.Confidence}, Benefit: float64(u.EndLine - u.Line + 1)})
			}
		}
	}
	return out
}
