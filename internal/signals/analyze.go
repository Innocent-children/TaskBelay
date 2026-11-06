package signals

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type Input struct {
	EditPreview   bool
	Queries       *Queries
	Before, After repostats.Repository
	Changes       changes.Result
	Regions       []changes.Region
	Requirements  []changes.Requirement
	Retained      []string
	Confirmed     []string
}
type Candidate struct {
	ID, Kind, Object, At, Cluster string
	Facts                         []string
	Advice                        string
	Features                      map[string]float64
	Benefit                       float64
	AgentOnly, Defensive          bool
	BlockFact                     string
}
type External struct {
	Kind, Method, Name, Value, At, Cluster, Anchor, Source string
	Location                                               string
	Reliable, AgentOnly                                    bool
}
type Case struct {
	ID, Name, Path, Unit, Class, Shape, Cluster string
	Start, End                                  int
	Added, AgentOnly, Retained                  bool
	Targets                                     []string
	Inputs                                      []string
	Boundary                                    []string
}
type Result struct {
	Candidates []Candidate
	External   []External
	Cases      []Case
	Gaps       []string
	Defensive  []string
	TestBudget map[string]int
}

func Analyze(ctx context.Context, in Input) (Result, error) {
	r := Result{TestBudget: map[string]int{}}
	if len(in.Changes.Hunks) == 0 {
		return r, ctx.Err()
	}
	for _, c := range in.Changes.Clusters {
		if in.EditPreview {
			continue
		}
		if err := ctx.Err(); err != nil {
			return r, err
		}
		for _, h := range clusterHunks(in, c.ID) {
			if isDefensive(in.After, h) {
				r.Defensive = append(r.Defensive, c.ID)
				break
			}
		}
		known, related := false, false
		for _, region := range in.Regions {
			known = known || region.Known
			for _, id := range region.Clusters {
				related = related || id == c.ID
			}
		}
		if !known {
			r.Gaps = append(r.Gaps, "需求相关区未知："+c.ID)
			continue
		}
		if related {
			continue
		}
		registered := true
		for _, h := range clusterHunks(in, c.ID) {
			found := false
			for _, req := range in.Requirements {
				for _, at := range req.Locations {
					found = found || at == h.Path
				}
			}
			registered = registered && found
		}
		if registered {
			continue
		}
		kind := "unrelated"
		facts := []string{"未落入已知需求相关区；词汇关联不足不证明实现多余"}
		advice := "先核对变更簇 " + c.ID + " 是否维护当前需求或既有行为；确认不需要后再撤回"
		cosmetic, defensive := true, false
		benefit := 0.0
		for _, h := range clusterHunks(in, c.ID) {
			cosmetic = cosmetic && (h.Kind == "cosmetic" || h.Kind == "rename" || h.Kind == "move")
			benefit += float64(len(strings.Split(h.After, "\n")) + len(h.NewWords))
			defensive = defensive || isDefensive(in.After, h)
		}
		for _, region := range in.Regions {
			if path := region.Paths[c.ID]; len(path) > 0 {
				facts = append(facts, region.Requirement+"："+strings.Join(path, " → "))
			}
		}
		if cosmetic {
			kind = "noise"
		}
		r.Candidates = append(r.Candidates, Candidate{Kind: kind, Object: c.ID, Cluster: c.ID, At: clusterPath(in, c.ID), Facts: facts, Advice: advice, Features: map[string]float64{"unrelated": boolFloat(known), "agent": boolFloat(c.Origin == "agent")}, Benefit: benefit, AgentOnly: c.Origin == "agent", Defensive: defensive})
	}
	r.External = external(in)
	for _, item := range r.External {
		if item.Anchor == "quote" {
			continue
		}
		c := Candidate{Kind: "external", Object: item.Object(), At: item.At, Cluster: item.Cluster, AgentOnly: item.AgentOnly, Features: map[string]float64{"unanchored": boolFloat(item.Anchor == "none"), "reliable": boolFloat(item.Reliable), "agent": boolFloat(item.AgentOnly)}, Benefit: 1, Facts: []string{item.Method + "：" + item.Name, "需求对应关系：没有对应说明"}, Advice: "撤回该新增项，或用 implemented_by 登记它对应的需求和用途"}
		if !item.Reliable {
			c.Facts = append(c.Facts, "仅按代码形状识别，尚未由探针确认是否为对外新增项")
			c.Advice = "先核对它是否确为对外新增项；确有新增时再核对需求，撤回多余内容或用 implemented_by 登记必要用途"
		}
		if item.Anchor == "agent" {
			c.Facts[1] = "需求对应关系：Agent 已说明对应需求 " + item.Source
			c.Facts = append(c.Facts, "登记只说明关联，不能据此确认实现有必要或已获用户授权")
			c.Advice = "沿用已有登记，无需重复登记；是否需要精简，仍须结合当前需求和实际用途判断"
		}
		if item.Anchor == "none" && item.Reliable && item.AgentOnly {
			c.BlockFact = "external"
		}
		r.Candidates = append(r.Candidates, c)
	}
	if !in.EditPreview {
		indirect, gaps, err := indirections(ctx, in)
		if err != nil {
			return r, err
		}
		r.Candidates = append(r.Candidates, indirect...)
		r.Gaps = append(r.Gaps, gaps...)
	}
	duplicate, err := similarities(ctx, in)
	if err != nil {
		return r, err
	}
	r.Candidates = append(r.Candidates, duplicate...)
	if !in.EditPreview {
		var gaps []string
		if in.Before.Converged && in.After.Converged {
			r.Candidates = append(r.Candidates, unused(in)...)
			r.Cases, r.TestBudget, duplicate, gaps, err = cases(ctx, in)
			if err != nil {
				return r, err
			}
			r.Candidates = append(r.Candidates, duplicate...)
			r.Gaps = append(r.Gaps, gaps...)
		} else {
			r.Gaps = append(r.Gaps, "按词汇识别测试内容的分类结果尚未稳定，未生成依赖该分类的建议")
		}
	}
	var kept []Candidate
	for _, c := range r.Candidates {
		retain := false
		for _, at := range in.Retained {
			if at == c.Object || at == c.At || at == c.Cluster {
				retain = true
			}
		}
		if retain {
			continue
		}
		for _, id := range r.Defensive {
			c.Defensive = c.Defensive || c.Cluster == id
		}
		if c.Defensive {
			c.Facts = append(c.Facts, "涉及按结构识别的防御性短分支，不参加撤回试验；结构不能证明失败不可能发生")
			if c.Kind == "unrelated" || c.Kind == "noise" {
				c.Advice = "先保留必要的错误传递和资源处理；核对失败条件、需求或调用约定及负责处理的层次，无法确认时不删除、不因此补测试"
			} else {
				c.Advice += "；涉及的错误分支先保留，另行核对失败条件和处理职责"
			}
		}
		if !c.AgentOnly && !in.EditPreview {
			c.Facts = append(c.Facts, "含工作区更新或无法核对为全部由 Agent 编辑，只报告现状")
			c.Advice = "保留当前内容，不据此撤回、内联或合并用户修改"
		}
		if in.EditPreview {
			c.Advice += "；仅核对本次拟编辑内容，不据此覆盖或撤回已有用户修改"
		}
		c.ID = units.Hash(c.Kind, c.Object)
		if c.Features == nil {
			c.Features = map[string]float64{}
		}
		for _, region := range in.Regions {
			for _, id := range region.Clusters {
				if id == c.Cluster {
					c.Facts = append(c.Facts, "位于需求 "+region.Requirement+" 的相关区；关联不证明实现有必要")
					break
				}
			}
			path := region.Paths[c.Cluster]
			if len(path) == 0 {
				continue
			}
			total := 0.0
			mass := map[string]float64{}
			for _, seed := range region.Seeds {
				if seed.Node == path[0] {
					mass[seed.Source] += seed.Weight
					total += seed.Weight
				}
			}
			for _, source := range keys(mass) {
				if total > 0 {
					c.Features["seed_"+source] = max(c.Features["seed_"+source], mass[source]/total)
				}
			}
		}
		if f, ok := in.After.Files[c.At]; ok {
			c.Features["structure"] = f.Confidence
		}
		kept = append(kept, c)
	}
	distinct := map[string]Candidate{}
	for _, c := range kept {
		key := c.Kind + "\x00" + c.Object + "\x00" + c.At + "\x00" + strings.Join(c.Facts, "\x00")
		prior, ok := distinct[key]
		if !ok || c.Benefit > prior.Benefit {
			distinct[key] = c
		}
	}
	r.Candidates = nil
	for _, key := range keys(distinct) {
		r.Candidates = append(r.Candidates, distinct[key])
	}
	sort.Slice(r.Candidates, func(i, j int) bool { return r.Candidates[i].ID < r.Candidates[j].ID })
	sort.Strings(r.Gaps)
	return r, ctx.Err()
}

func clusterHunks(in Input, id string) []changes.Hunk {
	set := map[string]bool{}
	for _, c := range in.Changes.Clusters {
		if c.ID == id {
			for _, h := range c.Hunks {
				set[h] = true
			}
		}
	}
	var out []changes.Hunk
	for _, h := range in.Changes.Hunks {
		if set[h.ID] {
			out = append(out, h)
		}
	}
	return out
}
func clusterPath(in Input, id string) string {
	h := clusterHunks(in, id)
	if len(h) > 0 {
		return h[0].Path
	}
	return ""
}
func clusterFor(in Input, p string, start, end int) (string, bool) {
	found := ""
	agent := true
	for _, c := range in.Changes.Clusters {
		for _, h := range in.Changes.Hunks {
			if h.Path == p && h.Span.AfterStart < end && h.Span.AfterEnd >= start && slices.Contains(c.Hunks, h.ID) {
				if found != "" && found != c.ID {
					agent = false
				}
				found = c.ID
				agent = agent && c.Origin == "agent"
			}
		}
	}
	return found, agent && found != ""
}
func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func isDefensive(repo repostats.Repository, h changes.Hunk) bool {
	f := repo.Files[h.Path]
	returns := repo.SyntaxFor(h.Path).Returns
	for _, u := range f.Units {
		if u.Line > h.Span.AfterEnd || u.EndLine <= h.Span.AfterStart || u.Parent < 0 {
			continue
		}
		parent := f.Units[u.Parent]
		body := units.Significant(u.Tokens)
		var first string
		for _, t := range body {
			if t.Start >= u.HeaderEnd && t.Kind == "identifier" {
				first = t.Text
				break
			}
		}
		matches := false
		for _, w := range returns {
			matches = matches || w == first
		}
		if !matches || u.EndLine-u.Line >= parent.EndLine-parent.Line {
			continue
		}
		previous := map[string]bool{}
		for _, t := range parent.Tokens {
			if t.Line < u.Line && t.Kind == "identifier" {
				previous[t.Text] = true
			}
		}
		for _, t := range u.Tokens {
			if t.End <= u.HeaderEnd && previous[t.Text] {
				return true
			}
		}
	}
	return false
}
