package signals

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/units"
)

func similarities(ctx context.Context, in Input) ([]Candidate, error) {
	type entry struct {
		path             string
		unit             units.Unit
		grams, signature []uint64
	}
	var old []entry
	buckets := map[string][]int{}
	for _, p := range keys(in.Before.Files) {
		for _, u := range in.Before.Files[p].Units {
			grams, err := units.Grams(ctx, units.Sequence(u.Tokens, true))
			if err != nil {
				return nil, err
			}
			signature, err := units.MinHash(ctx, grams)
			if err != nil {
				return nil, err
			}
			e := entry{p, u, grams, signature}
			idx := len(old)
			old = append(old, e)
			for i := 0; i < len(signature); i += LSHRows {
				key := fmt.Sprint(i, signature[i:min(i+LSHRows, len(signature))])
				buckets[key] = append(buckets[key], idx)
			}
		}
	}
	var distribution []float64
	for i := 0; i < len(old) && len(distribution) < SimilaritySamples; i++ {
		j := (i + len(old)/2) % max(1, len(old))
		if i != j {
			distribution = append(distribution, units.Similarity(old[i].grams, old[j].grams))
		}
	}
	mean, variance := 0.0, 0.0
	for _, s := range distribution {
		mean += s
	}
	if len(distribution) > 0 {
		mean /= float64(len(distribution))
	}
	for _, s := range distribution {
		variance += (s - mean) * (s - mean)
	}
	if len(distribution) > 1 {
		variance /= float64(len(distribution) - 1)
	}
	var out []Candidate
	for _, p := range keys(in.After.Files) {
		for _, u := range in.After.Files[p].Units {
			cluster, agent := clusterFor(in, p, u.Line, u.EndLine)
			if cluster == "" {
				continue
			}
			grams, err := units.Grams(ctx, units.Sequence(u.Tokens, true))
			if err != nil {
				return out, err
			}
			signature, err := units.MinHash(ctx, grams)
			if err != nil {
				return out, err
			}
			candidates := map[string]int{}
			for i := 0; i < len(signature); i += LSHRows {
				for _, j := range buckets[fmt.Sprint(i, signature[i:min(i+LSHRows, len(signature))])] {
					candidates[location(old[j].path, old[j].unit)] = j
				}
			}
			for _, at := range keys(candidates) {
				v := old[candidates[at]]
				if at == location(p, u) || v.unit.ID == u.ID && v.path == p {
					continue
				}
				similarity := units.Similarity(grams, v.grams)
				z := similarity - mean
				if variance > 0 {
					z /= math.Sqrt(variance)
				}
				out = append(out, Candidate{Kind: "duplicate", Object: units.Hash(u.ID, v.unit.ID), At: location(p, u), Cluster: cluster, AgentOnly: agent, Facts: []string{fmt.Sprintf("与 %s 相似度 %.3f；无关单元均值 %.3f", at, similarity, mean)}, Advice: "复用 " + at + "，合并 " + location(p, u), Features: map[string]float64{"similarity": similarity, "similarity_excess": z, "structure": in.After.Files[p].Confidence}, Benefit: float64(len(strings.Split(strings.Join(units.Sequence(u.Tokens, false), " "), "\n")))})
			}
		}
	}
	return out, nil
}
