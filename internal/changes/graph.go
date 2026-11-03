package changes

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type Requirement struct {
	ID, Quote, Plan, Content, Acceptance string
	Locations                            []string
}
type GraphInput struct {
	Before, After repostats.Repository
	Changes       Result
	Requirements  []Requirement
	Weights       []float64
}
type Graph struct {
	Edges   map[string]map[string]float64
	Degree  map[string]float64
	Matches map[string][]string
	Volume  float64
}
type Seed struct {
	Node, Source string
	Weight       float64
}
type Region struct {
	Requirement string
	Known       bool
	Seeds       []Seed
	Scores      map[string]float64
	Members     []string
	Clusters    []string
	Paths       map[string][]string
	Conductance float64
	Pushes      int
}

func BuildGraph(ctx context.Context, in GraphInput) (Graph, error) {
	g := Graph{Edges: map[string]map[string]float64{}, Degree: map[string]float64{}, Matches: map[string][]string{}}
	files := map[string]units.File{}
	for p, f := range in.After.Files {
		files[p] = f
	}
	for p, f := range in.Before.Files {
		if _, ok := files[p]; !ok {
			files[p] = f
		}
	}
	df := map[string]int{}
	total := 0
	for _, p := range sortedKeys(files) {
		for _, u := range files[p].Units {
			total++
			seen := map[string]bool{}
			for _, t := range u.Tokens {
				if t.Kind == "identifier" || t.Kind == "key" {
					seen[units.Literal(t)] = true
				}
			}
			for w := range seen {
				df[w]++
			}
		}
	}
	for _, p := range sortedKeys(files) {
		if err := ctx.Err(); err != nil {
			return g, err
		}
		g.Matches[strings.ToLower(p)] = append(g.Matches[strings.ToLower(p)], "f:"+p)
		for _, u := range files[p].Units {
			key := "u:" + repostats.UnitKey(p, u)
			g.add(key, "f:"+p, FileWeight)
			g.Matches[strings.ToLower(fmt.Sprintf("%s:%d", p, u.Line))] = append(g.Matches[strings.ToLower(fmt.Sprintf("%s:%d", p, u.Line))], key)
			if u.Name != "" {
				g.Matches[strings.ToLower(p+":"+u.Name)] = append(g.Matches[strings.ToLower(p+":"+u.Name)], key)
			}
			counts := map[string]int{}
			for _, t := range u.Tokens {
				if t.Kind == "identifier" || t.Kind == "key" {
					counts[units.Literal(t)]++
				}
			}
			for _, w := range sortedKeys(counts) {
				weight := math.Log(float64(max(1, total))/float64(df[w])) * math.Log1p(float64(counts[w]))
				if weight < Epsilon {
					continue
				}
				g.add(key, "w:"+w, weight)
				g.Matches[strings.ToLower(w)] = append(g.Matches[strings.ToLower(w)], "w:"+w)
				for _, sub := range units.Words(w) {
					g.Matches[sub] = append(g.Matches[sub], "w:"+w)
				}
			}
		}
	}
	hunks := map[string]Hunk{}
	for _, h := range in.Changes.Hunks {
		hunks[h.ID] = h
	}
	calls := map[string][]string{}
	for _, c := range in.Changes.Clusters {
		for _, id := range c.Hunks {
			h := hunks[id]
			for _, u := range h.Units {
				if _, ok := g.Edges["u:"+u]; ok {
					g.add("c:"+c.ID, "u:"+u, ClusterWeight)
				}
			}
			if h.Call != "" {
				calls[h.Call] = append(calls[h.Call], c.ID)
			}
		}
	}
	for _, call := range sortedKeys(calls) {
		ids := calls[call]
		for i, a := range ids {
			for _, b := range ids[i+1:] {
				if a != b {
					g.add("c:"+a, "c:"+b, ClusterWeight)
				}
			}
		}
	}
	for _, key := range sortedKeys(g.Edges) {
		for _, to := range sortedKeys(g.Edges[key]) {
			g.Degree[key] += g.Edges[key][to]
		}
		g.Volume += g.Degree[key]
	}
	return g, nil
}

func (g *Graph) add(a, b string, w float64) {
	if a == b {
		return
	}
	if g.Edges[a] == nil {
		g.Edges[a] = map[string]float64{}
	}
	if g.Edges[b] == nil {
		g.Edges[b] = map[string]float64{}
	}
	g.Edges[a][b] += w
	g.Edges[b][a] += w
}

func Relate(ctx context.Context, in GraphInput) ([]Region, error) {
	g, err := BuildGraph(ctx, in)
	if err != nil {
		return nil, err
	}
	matcher, err := units.NewMatcher(ctx, sortedKeys(g.Matches))
	if err != nil {
		return nil, err
	}
	weights := in.Weights
	if len(weights) != 3 {
		weights = []float64{QuoteWeight, ContentWeight, LocationWeight}
	}
	var regions []Region
	for _, req := range in.Requirements {
		region := Region{Requirement: req.ID, Paths: map[string][]string{}}
		seedMass := map[string]float64{}
		for i, text := range []string{req.Quote + " " + req.Plan, req.Content + " " + req.Acceptance, strings.Join(req.Locations, " ")} {
			text = strings.ToLower(text)
			matches, err := matcher.Find(ctx, text)
			if err != nil {
				return regions, err
			}
			seen := map[string]bool{}
			for _, match := range matches {
				if !units.WholeWord(text, match) {
					continue
				}
				for _, node := range g.Matches[match.Pattern] {
					if seen[node] {
						continue
					}
					seen[node] = true
					if i < 2 {
						region.Known = true
					}
					seedMass[node] += weights[i]
					region.Seeds = append(region.Seeds, Seed{node, []string{"quote", "content", "location"}[i], weights[i]})
				}
			}
		}
		if !region.Known {
			regions = append(regions, region)
			continue
		}
		region.Scores, region.Pushes, err = Push(ctx, g, seedMass)
		if err != nil {
			return regions, err
		}
		region.Members, region.Conductance = Sweep(g, region.Scores)
		for _, node := range region.Members {
			if strings.HasPrefix(node, "c:") {
				region.Clusters = append(region.Clusters, strings.TrimPrefix(node, "c:"))
			}
		}
		region.Paths, err = Strongest(ctx, g, seedMass, in.Changes.Clusters)
		if err != nil {
			return regions, err
		}
		regions = append(regions, region)
	}
	return regions, nil
}
