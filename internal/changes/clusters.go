package changes

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type Edit struct {
	Path, BeforePath, Before, After, Call string
	AgentOnly                             bool
	Exact                                 bool
	Span                                  Span
}
type Input struct {
	Before, After repostats.Repository
	Edits         []Edit
}
type Hunk struct {
	ID, Path, BeforePath, Kind, Origin, Call string
	Span                                     Span
	Before, After                            string
	Words, NewWords, Names, Units            []string
	Tokens                                   []units.Token
	WordEdits                                []Span
}
type Cluster struct {
	ID          string
	Hunks       []string
	Depends     []string
	Origin      string
	NewWords    []string
	MergeReason string
}
type Result struct {
	Hunks       []Hunk
	Clusters    []Cluster
	Relocations map[string]string
}

func Compare(ctx context.Context, in Input) (Result, error) {
	r := Result{Relocations: map[string]string{}}
	paths := map[string]bool{}
	for p := range in.Before.Files {
		paths[p] = true
	}
	for p := range in.After.Files {
		paths[p] = true
	}
	keys := sortedKeys(paths)
	beforeByHash, afterByHash := map[string][]string{}, map[string][]string{}
	for _, p := range keys {
		a, aok := in.Before.Files[p]
		b, bok := in.After.Files[p]
		if aok && !bok {
			key := units.Hash(strings.Join(units.Sequence(a.Tokens, false), "\x00"))
			beforeByHash[key] = append(beforeByHash[key], p)
		}
		if bok && !aok {
			key := units.Hash(strings.Join(units.Sequence(b.Tokens, false), "\x00"))
			afterByHash[key] = append(afterByHash[key], p)
		}
	}
	moved := map[string]bool{}
	for _, key := range sortedKeys(beforeByHash) {
		a, b := beforeByHash[key], afterByHash[key]
		if len(a) == 1 && len(b) == 1 {
			r.Relocations[a[0]] = b[0]
			moved[a[0]] = true
		}
	}
	reverse := map[string]string{}
	for a, b := range r.Relocations {
		reverse[b] = a
	}
	for _, p := range keys {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if moved[p] {
			continue
		}
		old := p
		if reverse[p] != "" {
			old = reverse[p]
		}
		a, b := in.Before.Files[old], in.After.Files[p]
		if a.Blob != "" && a.Blob == b.Blob && old == p {
			continue
		}
		spans, err := Diff(ctx, a.Lines, b.Lines)
		if err != nil {
			return r, err
		}
		if old != p {
			spans = []Span{{0, len(a.Lines), 0, len(b.Lines)}}
		}
		if a.Binary || b.Binary {
			if a.Blob == b.Blob {
				continue
			}
			spans = []Span{{}}
		}
		for _, span := range spans {
			before, after := tokensIn(a, span.BeforeStart, span.BeforeEnd), tokensIn(b, span.AfterStart, span.AfterEnd)
			h := Hunk{Path: p, BeforePath: old, Span: span, Kind: classify(before, after), Origin: "workspace", Before: lineText(a.Lines, span.BeforeStart, span.BeforeEnd), After: lineText(b.Lines, span.AfterStart, span.AfterEnd), Tokens: append(append([]units.Token(nil), before...), after...)}
			h.WordEdits, err = Diff(ctx, units.Sequence(before, false), units.Sequence(after, false))
			if err != nil {
				return r, err
			}
			if old != p {
				h.Kind = "move"
			}
			if a.Binary || b.Binary {
				h.Kind = "binary"
			}
			h.ID = units.Hash(old, strings.Join(units.Sequence(before, false), "\x00"), strings.Join(units.Sequence(after, false), "\x00"))
			words, newWords, names, objects := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
			for _, t := range h.Tokens {
				if t.Kind == "identifier" || t.Kind == "key" {
					words[units.Literal(t)] = true
				}
			}
			for _, t := range after {
				if t.Kind == "identifier" && in.Before.Frequency[t.Text] == 0 {
					newWords[t.Text] = true
				}
			}
			for _, side := range []struct {
				file       units.File
				start, end int
			}{{a, span.BeforeStart, span.BeforeEnd}, {b, span.AfterStart, span.AfterEnd}} {
				for _, u := range side.file.Units {
					if u.Line <= side.end && u.EndLine > side.start {
						objects[repostats.UnitKey(side.file.Path, u)] = true
						if u.Line > side.start && u.Line <= side.end && u.Name != "" {
							names[u.Name] = true
						}
					}
				}
			}
			h.Words, h.NewWords, h.Names, h.Units = sortedKeys(words), sortedKeys(newWords), sortedKeys(names), sortedKeys(objects)
			// 同内容的多个片段以所属单元及出现次序区分，其他文件变化不改变身份。
			identity := units.Hash(h.ID, strings.Join(h.Units, "\x00"))
			occurrence := 0
			for _, prior := range r.Hunks {
				if prior.BeforePath == old && prior.Before == h.Before && prior.After == h.After {
					occurrence++
				}
			}
			h.ID = units.Hash(identity, strconv.Itoa(occurrence))
			for _, edit := range in.Edits {
				if edit.Path != p && edit.BeforePath != old {
					continue
				}
				matches := edit.Exact && edit.Span == span && edit.Before == h.Before && edit.After == h.After
				matches = matches || !edit.Exact && edit.Before == strings.Join(a.Lines, "") && edit.After == strings.Join(b.Lines, "")
				if !matches && !edit.Exact && h.Before != "" && h.After != "" {
					matches = strings.Contains(edit.Before, h.Before) && strings.Contains(edit.After, h.After)
				}
				if matches && edit.AgentOnly {
					h.Origin = "agent"
					h.Call = edit.Call
					break
				}
				if strings.Contains(edit.Before, h.Before) && h.Before != "" || strings.Contains(edit.After, h.After) && h.After != "" {
					h.Origin = "mixed"
				}
			}
			r.Hunks = append(r.Hunks, h)
		}
	}
	// 移动只有在完整内容和所选窗口都对应时才迁移身份。
	for i := range r.Hunks {
		a := &r.Hunks[i]
		if a.Before == "" || a.After != "" {
			continue
		}
		aw, _ := units.Winnow(ctx, units.Sequence(a.Tokens, false))
		var matches []int
		for j, b := range r.Hunks {
			if b.Before != "" || b.After == "" {
				continue
			}
			bw, err := units.Winnow(ctx, units.Sequence(b.Tokens, false))
			if err != nil {
				return r, err
			}
			if len(aw) > 0 && slices.Equal(aw, bw) && a.Before == b.After {
				matches = append(matches, j)
			}
		}
		if len(matches) == 1 {
			j := matches[0]
			a.Kind = "move"
			r.Hunks[j].Kind = "move"
			r.Relocations[a.BeforePath] = r.Hunks[j].Path
		}
	}
	edges := make([][]int, len(r.Hunks))
	suppliers := map[string][]int{}
	users := map[string][]int{}
	for i, h := range r.Hunks {
		for _, w := range h.NewWords {
			suppliers[w] = append(suppliers[w], i)
		}
		for _, w := range h.Words {
			users[w] = append(users[w], i)
		}
	}
	for i, h := range r.Hunks {
		set := map[int]bool{}
		for _, w := range h.Words {
			for _, j := range suppliers[w] {
				if i != j {
					set[j] = true
				}
			}
		}
		for _, w := range h.Names {
			for _, j := range users[w] {
				if i != j {
					set[j] = true
					edges[j] = append(edges[j], i)
				}
			}
		}
		for j := range set {
			edges[i] = append(edges[i], j)
		}
	}
	components, err := components(ctx, edges)
	if err != nil {
		return r, err
	}
	owner := make([]int, len(r.Hunks))
	for i, group := range components {
		for _, j := range group {
			owner[j] = i
		}
		c := Cluster{Origin: "agent"}
		words := map[string]bool{}
		for _, j := range group {
			h := r.Hunks[j]
			c.Hunks = append(c.Hunks, h.ID)
			if h.Origin != c.Origin {
				if len(c.Hunks) == 1 {
					c.Origin = h.Origin
				} else {
					c.Origin = "mixed"
				}
			}
			for _, w := range h.NewWords {
				words[w] = true
			}
		}
		sort.Strings(c.Hunks)
		c.ID = units.Hash(c.Hunks...)
		c.NewWords = sortedKeys(words)
		r.Clusters = append(r.Clusters, c)
	}
	for i, group := range components {
		deps := map[string]bool{}
		for _, j := range group {
			for _, k := range edges[j] {
				if owner[k] != i {
					deps[r.Clusters[owner[k]].ID] = true
				}
			}
		}
		r.Clusters[i].Depends = sortedKeys(deps)
	}
	sort.Slice(r.Clusters, func(i, j int) bool { return r.Clusters[i].ID < r.Clusters[j].ID })
	return r, ctx.Err()
}

func components(ctx context.Context, edges [][]int) ([][]int, error) {
	index, low := make([]int, len(edges)), make([]int, len(edges))
	for i := range index {
		index[i] = -1
	}
	var stack []int
	on := make([]bool, len(edges))
	serial := 0
	var out [][]int
	var visit func(int) error
	visit = func(v int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		index[v], low[v] = serial, serial
		serial++
		stack = append(stack, v)
		on[v] = true
		sort.Ints(edges[v])
		for _, w := range edges[v] {
			if index[w] < 0 {
				if err := visit(w); err != nil {
					return err
				}
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			var group []int
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[w] = false
				group = append(group, w)
				if w == v {
					break
				}
			}
			sort.Ints(group)
			out = append(out, group)
		}
		return nil
	}
	for v := range edges {
		if index[v] < 0 {
			if err := visit(v); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func Closure(ctx context.Context, clusters []Cluster, selected []string) ([]string, error) {
	set := map[string]bool{}
	for _, id := range selected {
		set[id] = true
	}
	reverse := map[string][]string{}
	for _, c := range clusters {
		for _, d := range c.Depends {
			reverse[d] = append(reverse[d], c.ID)
		}
	}
	queue := append([]string(nil), selected...)
	for i := 0; i < len(queue); i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, id := range reverse[queue[i]] {
			if !set[id] {
				set[id] = true
				queue = append(queue, id)
			}
		}
	}
	return sortedKeys(set), nil
}

func Merge(ctx context.Context, clusters []Cluster, ids []string, reason string) ([]Cluster, error) {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	merged := Cluster{Origin: "agent", MergeReason: reason}
	deps := map[string]bool{}
	var out []Cluster
	for _, c := range clusters {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !set[c.ID] {
			out = append(out, c)
			continue
		}
		merged.Hunks = append(merged.Hunks, c.Hunks...)
		merged.NewWords = append(merged.NewWords, c.NewWords...)
		if c.Origin != "agent" {
			merged.Origin = "mixed"
		}
		for _, d := range c.Depends {
			if !set[d] {
				deps[d] = true
			}
		}
	}
	sort.Strings(merged.Hunks)
	merged.ID = units.Hash(merged.Hunks...)
	merged.Depends = sortedKeys(deps)
	for i := range out {
		d := map[string]bool{}
		for _, id := range out[i].Depends {
			if set[id] {
				id = merged.ID
			}
			d[id] = true
		}
		out[i].Depends = sortedKeys(d)
	}
	out = append(out, merged)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
