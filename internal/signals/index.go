package signals

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"strings"
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
	Callable         bool
}
type Queries struct {
	Syntax         func(string) units.Syntax
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
	Callable              bool
}

/**
 * 排除只有首行或闭合标点的版面节点，它们不能独立作为实现复用。
 */
func hasBody(u units.Unit) bool {
	for _, t := range u.Tokens {
		if t.Start >= u.HeaderEnd && (t.Kind == "identifier" || t.Kind == "number" || t.Kind == "key" || t.Kind == "string") {
			return true
		}
	}
	return false
}

/**
 * 要求参数括号在首行部分完整闭合，避免把调用的实参节点当作声明。
 */
func callable(u units.Unit) bool {
	if u.Name == "" || !hasBody(u) {
		return false
	}
	tokens := units.Significant(u.Tokens)
	depth := 0
	for _, t := range tokens {
		if t.End > u.HeaderEnd {
			break
		}
		if depth == 0 && strings.ContainsAny(t.Text, "=!") && t.Kind == "symbol" {
			return false
		}
		if t.Text == "(" {
			depth++
		}
		if t.Text == ")" {
			depth--
		}
	}
	for i, t := range tokens {
		if t.End > u.HeaderEnd {
			break
		}
		if t.Text != u.Name || i+1 >= len(tokens) || tokens[i+1].Text != "(" {
			continue
		}
		if i > 0 && tokens[i-1].Text == "." {
			return false
		}
		depth := 0
		for _, arg := range tokens[i+1:] {
			if arg.End > u.HeaderEnd {
				break
			}
			if arg.Text == "(" {
				depth++
			}
			if arg.Text == ")" {
				depth--
				if depth == 0 {
					return true
				}
			}
		}
	}
	return false
}

/**
 * 不将函数主体内的版面节点再次解释为独立声明。
 */
func declaration(f units.File, u units.Unit) bool {
	if !callable(u) {
		return false
	}
	for parent := u.Parent; parent >= 0; parent = f.Units[parent].Parent {
		if callable(f.Units[parent]) {
			return false
		}
	}
	return true
}

// outsideCallable 判断单元是否不在任何函数形状的单元内部。
func outsideCallable(f units.File, u units.Unit) bool {
	for parent := u.Parent; parent >= 0; parent = f.Units[parent].Parent {
		if callable(f.Units[parent]) {
			return false
		}
	}
	return true
}

// EnclosingDeclaration 返回包含整段引用的声明；类型声明可用自己的首行标识。
func EnclosingDeclaration(f units.File, start, end int) units.Unit {
	var best units.Unit
	for _, u := range f.Units {
		if u.Name == "" || u.Start > start || u.End < end || best.Name != "" && u.End-u.Start >= best.End-best.Start {
			continue
		}
		if declaration(f, u) || u.Parent < 0 && start < u.HeaderEnd && hasBody(u) {
			best = u
		}
	}
	return best
}

/**
 * 保留间接层计算需要的顺序和结构；学习约定变化后重新过滤词汇。
 */
func unitStructure(f units.File, u units.Unit) UnitStructure {
	s := UnitStructure{Abstract: true, Callable: declaration(f, u)}
	bodyStart := len(u.Tokens)
	for i, t := range u.Tokens {
		if bodyStart == len(u.Tokens) && t.Start >= u.HeaderEnd {
			bodyStart = i
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
	s.BodyCalls = units.CallNameCount(u.Tokens[bodyStart:])
	return s
}

func addMember(s *UnitStructure, v units.Unit) {
	if v.Name != "" {
		s.Members = append(s.Members, v.Name)
	}
	if s.Abstract {
		count := 0
		for _, t := range v.Tokens {
			if t.Start >= v.HeaderEnd && t.Kind == "identifier" {
				count++
				if count > 1 {
					s.Abstract = false
					break
				}
			}
		}
	}
}

func Structure(f units.File, u units.Unit) UnitStructure {
	s := unitStructure(f, u)
	for j, v := range f.Units {
		if v.Parent < 0 || f.Units[v.Parent].ID != u.ID || j == u.Parent {
			continue
		}
		addMember(&s, v)
	}
	return s
}

// 初建和重算整个文件时，沿已知的父子关系归集成员。
func FileStructures(f units.File) []UnitStructure {
	out := make([]UnitStructure, len(f.Units))
	for i, u := range f.Units {
		out[i] = unitStructure(f, u)
	}
	for j, child := range f.Units {
		if p := child.Parent; p >= 0 && j != f.Units[p].Parent {
			addMember(&out[p], child)
		}
	}
	return out
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
		for _, u := range r.Files[p].Units {
			if declaration(r.Files[p], u) {
				all = append(all, u)
			}
		}
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
