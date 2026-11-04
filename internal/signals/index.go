package signals

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type CallSite struct {
	Path string
	Args []units.Token
}
type Head struct {
	ID, Name string
	Words    map[string]bool
}
type SimilarUnit struct {
	Path             string
	Unit             units.Unit
	Grams, Signature []uint64
}
type Queries struct {
	Structure      func(string) UnitStructure
	Depend         func(path, kind, word string)
	Calls          func(string) []CallSite
	Heads          func(string) []Head
	Defined        func(string) bool
	Uses           func(string) int
	Similar        func([]uint64) []SimilarUnit
	Prior          map[string]float64
	Mean, Variance float64
}

type UnitStructure struct {
	Header, Body, Members []string
	Abstract              bool
	BodyCalls             int
}

/**
 * 保留间接层计算需要的顺序和结构；学习约定变化后重新过滤词汇。
 */
func Structure(f units.File, u units.Unit) UnitStructure {
	s := UnitStructure{Abstract: true}
	var bodyTokens []units.Token
	for _, t := range u.Tokens {
		if t.Start >= u.HeaderEnd {
			bodyTokens = append(bodyTokens, t)
		}
		if t.Kind != "identifier" {
			continue
		}
		if t.End <= u.HeaderEnd {
			s.Header = append(s.Header, t.Text)
		} else {
			s.Body = append(s.Body, t.Text)
		}
	}
	s.BodyCalls = len(units.Calls(bodyTokens))
	for j, v := range f.Units {
		if v.Parent < 0 || f.Units[v.Parent].ID != u.ID || j == u.Parent {
			continue
		}
		if v.Name != "" {
			s.Members = append(s.Members, v.Name)
		}
		count := 0
		for _, t := range v.Tokens {
			if t.Start >= v.HeaderEnd && t.Kind == "identifier" {
				count++
			}
		}
		s.Abstract = s.Abstract && count <= 1
	}
	return s
}

type Variation struct {
	Kind  string
	Count int
}

func OriginFacts(ctx context.Context, r repostats.Repository, q *Queries) (map[string][]Variation, error) {
	items, _, err := measure(ctx, r, nil, q)
	out := map[string][]Variation{}
	for _, v := range items {
		key := repostats.UnitKey(v.path, v.unit)
		out[key] = append(out[key], Variation{v.kind, v.count})
	}
	return out, err
}

func SimilarityBuckets(signature []uint64) []string {
	var out []string
	for i := 0; i < len(signature); i += LSHRows {
		out = append(out, fmt.Sprint(i, signature[i:min(i+LSHRows, len(signature))]))
	}
	return out
}

func SimilarityStatistics(ctx context.Context, r repostats.Repository) (float64, float64, error) {
	var all []units.Unit
	for _, p := range keys(r.Files) {
		all = append(all, r.Files[p].Units...)
	}
	var distribution []float64
	for i := 0; i < len(all) && len(distribution) < SimilaritySamples; i++ {
		j := (i + len(all)/2) % max(1, len(all))
		if i == j {
			continue
		}
		a, err := units.Grams(ctx, units.Sequence(all[i].Tokens, true))
		if err != nil {
			return 0, 0, err
		}
		b, err := units.Grams(ctx, units.Sequence(all[j].Tokens, true))
		if err != nil {
			return 0, 0, err
		}
		distribution = append(distribution, units.Similarity(a, b))
	}
	mean, variance := 0.0, 0.0
	for _, v := range distribution {
		mean += v
	}
	if len(distribution) > 0 {
		mean /= float64(len(distribution))
	}
	for _, v := range distribution {
		variance += (v - mean) * (v - mean)
	}
	if len(distribution) > 1 {
		variance /= float64(len(distribution) - 1)
	}
	return mean, variance, ctx.Err()
}
