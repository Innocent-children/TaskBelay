package changes

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/units"
	"strings"
)

type Requirement struct {
	ID, Quote, Plan, Content, Acceptance string
	Locations                            []string
}
type GraphInput struct {
	Graph        Graph
	Changes      Result
	Requirements []Requirement
	Weights      []float64
}
type Graph struct {
	Neighbours func(context.Context, string) (map[string]float64, error)
	NodeDegree func(context.Context, string) (float64, error)
	Match      func(context.Context, string) (map[string][]string, error)
	Edges      map[string]map[string]float64
	Degree     map[string]float64
	Volume     float64
}
type Seed struct {
	Node, Source string
	Weight       float64
}
type Region struct {
	Requirement       string
	Known             bool
	Seeds             []Seed
	Scores            map[string]float64
	Members, Clusters []string
	Paths             map[string][]string
	Conductance       float64
	Pushes            int
}

func (g Graph) degree(ctx context.Context, node string) (float64, error) {
	if d, ok := g.Degree[node]; ok {
		return d, nil
	}
	d, err := g.NodeDegree(ctx, node)
	if err == nil {
		g.Degree[node] = d
	}
	return d, err
}
func (g Graph) neighbours(ctx context.Context, node string) (map[string]float64, error) {
	if e, ok := g.Edges[node]; ok {
		return e, nil
	}
	e, err := g.Neighbours(ctx, node)
	if err == nil {
		g.Edges[node] = e
	}
	return e, err
}

/**
 * BuildGraph 只增加本次变更簇的边；仓库邻接与度由调用方按节点查询。
 */
func BuildGraph(ctx context.Context, in GraphInput) (Graph, error) {
	g := in.Graph
	g.Edges = map[string]map[string]float64{}
	g.Degree = map[string]float64{}
	extra := map[string]map[string]float64{}
	add := func(a, b string, w float64) {
		if a == b {
			return
		}
		if extra[a] == nil {
			extra[a] = map[string]float64{}
		}
		if extra[b] == nil {
			extra[b] = map[string]float64{}
		}
		extra[a][b] += w
		extra[b][a] += w
		g.Volume += 2 * w
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
				d, err := g.NodeDegree(ctx, "u:"+u)
				if err != nil {
					return g, err
				}
				if d > 0 {
					add("c:"+c.ID, "u:"+u, ClusterWeight)
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
				add("c:"+a, "c:"+b, ClusterWeight)
			}
		}
	}
	baseNeighbours, baseDegree := g.Neighbours, g.NodeDegree
	g.Neighbours = func(ctx context.Context, node string) (map[string]float64, error) {
		e, err := baseNeighbours(ctx, node)
		if err != nil {
			return nil, err
		}
		out := map[string]float64{}
		for k, v := range e {
			out[k] = v
		}
		for k, v := range extra[node] {
			out[k] += v
		}
		return out, nil
	}
	g.NodeDegree = func(ctx context.Context, node string) (float64, error) {
		d, err := baseDegree(ctx, node)
		for _, v := range extra[node] {
			d += v
		}
		return d, err
	}
	return g, ctx.Err()
}

func Relate(ctx context.Context, in GraphInput) ([]Region, error) {
	g, err := BuildGraph(ctx, in)
	if err != nil {
		return nil, err
	}
	weights := in.Weights
	if len(weights) != 3 {
		weights = []float64{QuoteWeight, ContentWeight, LocationWeight}
	}
	var regions []Region
	for _, req := range in.Requirements {
		r := Region{Requirement: req.ID, Paths: map[string][]string{}}
		mass := map[string]float64{}
		for i, text := range []string{req.Quote + " " + req.Plan, req.Content + " " + req.Acceptance, strings.Join(req.Locations, " ")} {
			text = strings.ToLower(text)
			matches, err := g.Match(ctx, text)
			if err != nil {
				return regions, err
			}
			seen := map[string]bool{}
			for _, pattern := range sortedKeys(matches) {
				if !units.HasWord(text, pattern) {
					continue
				}
				for _, node := range matches[pattern] {
					if seen[node] {
						continue
					}
					seen[node] = true
					if i < 2 {
						r.Known = true
					}
					mass[node] += weights[i]
					r.Seeds = append(r.Seeds, Seed{node, []string{"quote", "content", "location"}[i], weights[i]})
				}
			}
		}
		if !r.Known {
			regions = append(regions, r)
			continue
		}
		r.Scores, r.Pushes, err = Push(ctx, g, mass)
		if err != nil {
			return regions, err
		}
		r.Members, r.Conductance, err = Sweep(ctx, g, r.Scores)
		if err != nil {
			return regions, err
		}
		for _, n := range r.Members {
			if strings.HasPrefix(n, "c:") {
				r.Clusters = append(r.Clusters, strings.TrimPrefix(n, "c:"))
			}
		}
		r.Paths, err = Strongest(ctx, g, mass, in.Changes.Clusters)
		if err != nil {
			return regions, err
		}
		regions = append(regions, r)
	}
	return regions, nil
}
