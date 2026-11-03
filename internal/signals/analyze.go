package signals

import (
	"context"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type Input struct {
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
	Reliable, AgentOnly                                    bool
}
type Case struct {
	ID, Name, Path, Unit, Class, Shape string
	Start, End                         int
	Added, AgentOnly, Retained         bool
	Targets                            []string
	Inputs                             []string
	Boundary                           []string
}
type Result struct {
	Candidates []Candidate
	External   []External
	Cases      []Case
	Gaps       []string
	TestBudget map[string]int
}

func Analyze(ctx context.Context, in Input) (Result, error) {
	r := Result{TestBudget: map[string]int{}}
	for _, c := range in.Changes.Clusters {
		if err := ctx.Err(); err != nil {
			return r, err
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
		}
		if related {
			continue
		}
		kind := "unrelated"
		var facts []string
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
		r.Candidates = append(r.Candidates, Candidate{Kind: kind, Object: c.ID, Cluster: c.ID, At: clusterPath(in, c.ID), Facts: facts, Advice: "核对并撤回变更簇 " + c.ID, Features: map[string]float64{"unrelated": boolFloat(known), "agent": boolFloat(c.Origin == "agent")}, Benefit: benefit, AgentOnly: c.Origin == "agent", Defensive: defensive})
	}
	r.External = external(in)
	for _, item := range r.External {
		if item.Anchor == "quote" {
			continue
		}
		c := Candidate{Kind: "external", Object: item.Kind + ":" + item.Name, At: item.At, Cluster: item.Cluster, AgentOnly: item.AgentOnly, Features: map[string]float64{"unanchored": boolFloat(item.Anchor == "none"), "reliable": boolFloat(item.Reliable), "agent": boolFloat(item.AgentOnly)}, Benefit: 1, Facts: []string{item.Method + "：" + item.Name, "锚定：" + item.Anchor}, Advice: "撤回该新增项，或用 implemented_by 登记它对应的需求和用途"}
		if item.Anchor == "none" && item.Reliable && item.AgentOnly {
			c.BlockFact = "external"
		}
		r.Candidates = append(r.Candidates, c)
	}
	indirect, gaps := indirections(in)
	r.Candidates = append(r.Candidates, indirect...)
	r.Gaps = append(r.Gaps, gaps...)
	duplicate, err := similarities(ctx, in)
	if err != nil {
		return r, err
	}
	r.Candidates = append(r.Candidates, duplicate...)
	r.Candidates = append(r.Candidates, unused(in)...)
	r.Cases, r.TestBudget, duplicate, gaps = cases(in)
	r.Candidates = append(r.Candidates, duplicate...)
	r.Gaps = append(r.Gaps, gaps...)
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
		c.ID = units.Hash(c.Kind, c.Object)
		if c.Features == nil {
			c.Features = map[string]float64{}
		}
		if f, ok := in.After.Files[c.At]; ok {
			c.Features["structure"] = f.Confidence
		}
		kept = append(kept, c)
	}
	r.Candidates = kept
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
		for _, h := range clusterHunks(in, c.ID) {
			if h.Path == p && h.Span.AfterStart < end && h.Span.AfterEnd >= start {
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
			return false
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
