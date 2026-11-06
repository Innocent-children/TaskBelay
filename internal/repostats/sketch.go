package repostats

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/units"
	"math"
	"path"
	"strings"
)

type UnitSketch struct {
	Callable           bool
	Unit               units.Unit
	Identifiers, Words map[string]int
}
type FileSketch struct {
	Path, Blob, Mode   string
	Lexical            SyntaxCounts
	Units              []UnitSketch
	Heads, Ends, Uses  map[string]int
	Calls, Identifiers int
	Role               Role
	Classified         Role
	Tests, Seeds       map[string]bool
	Licensed           bool
}

/**
 * Sketch 保留约定学习所需计数及首行，正文词流仍留在 blob 级分析中。
 */
func Sketch(ctx context.Context, f units.File, data []byte) (FileSketch, error) {
	s := FileSketch{Path: f.Path, Blob: f.Blob, Mode: f.Mode, Heads: map[string]int{}, Ends: map[string]int{}, Uses: map[string]int{}, Role: classify(f)}
	var err error
	s.Lexical, err = CountSyntax(ctx, data)
	if err != nil {
		return s, err
	}
	for _, t := range f.Tokens {
		s.Uses[units.Literal(t)]++
	}
	for _, w := range units.Words(strings.Join(f.Lines, "")) {
		s.Licensed = s.Licensed || w == "license" || w == "licence" || w == "copyright"
	}
	for _, u := range f.Units {
		v := UnitSketch{Unit: u, Identifiers: map[string]int{}, Words: map[string]int{}}
		v.Unit.Tokens = nil
		heads := map[string]bool{}
		for i, t := range u.Tokens {
			if t.End <= u.HeaderEnd {
				v.Unit.Tokens = append(v.Unit.Tokens, t)
			}
			if t.Kind == "identifier" {
				v.Identifiers[t.Text]++
			}
			if t.Kind == "identifier" || t.Kind == "key" {
				v.Words[units.Literal(t)]++
				s.Identifiers++
				if t.End <= u.HeaderEnd {
					heads[units.Literal(t)] = true
				}
				if i+1 < len(u.Tokens) && len(u.Tokens[i+1].Text) == 1 && strings.Contains("([<", u.Tokens[i+1].Text) {
					s.Calls++
				}
			}
		}
		for w := range heads {
			s.Heads[w]++
		}
		last := u.EndLine
		for i := len(u.Tokens) - 1; i >= 0; i-- {
			t := u.Tokens[i]
			if t.Kind == "identifier" && t.Line < last {
				last = t.Line
				break
			}
		}
		for _, t := range u.Tokens {
			if t.Kind == "identifier" && t.Line == last {
				s.Ends[t.Text]++
				break
			}
		}
		s.Units = append(s.Units, v)
	}
	return s, ctx.Err()
}

func Relearn(ctx context.Context, tree string, sketches []FileSketch, runs []Run) (Repository, error) {
	r := Repository{Tree: tree, Files: map[string]units.File{}, Roles: map[string]Role{}, Frequency: map[string]int{}, HeadFrequency: map[string]int{}, IDF: map[string]float64{}, TestCounts: map[string]map[string]int{}}
	var sources []units.Source
	counts := map[string]SyntaxCounts{}
	groups := map[string][]FileSketch{}
	for _, s := range sketches {
		sources = append(sources, units.Source{Path: s.Path, Blob: s.Blob})
		counts[s.Blob] = s.Lexical
		groups[path.Ext(s.Path)] = append(groups[path.Ext(s.Path)], s)
	}
	var err error
	r.Syntax, err = LearnSyntax(ctx, sources, counts)
	if err != nil {
		return r, err
	}
	for _, ext := range sortedKeys(groups) {
		syn := r.Syntax[ext]
		heads, ends := map[string]int{}, map[string]int{}
		calls, identifiers := 0, 0
		for _, s := range groups[ext] {
			syn.Samples += len(s.Units)
			calls += s.Calls
			identifiers += s.Identifiers
			for w, n := range s.Heads {
				heads[w] += n
				r.HeadFrequency[w] += n
			}
			for w, n := range s.Ends {
				ends[w] += n
			}
			for _, u := range s.Units {
				r.UnitCount++
				for w := range u.Words {
					r.Frequency[w]++
				}
			}
		}
		if identifiers > 0 {
			syn.CallRatio = float64(calls) / float64(identifiers)
		}
		for _, w := range sortedKeys(heads) {
			if float64(heads[w]) > float64(syn.Samples)/2 {
				syn.Keywords = append(syn.Keywords, w)
			}
		}
		best := 0
		for _, n := range ends {
			best = max(best, n)
		}
		for _, w := range sortedKeys(ends) {
			if ends[w] == best {
				syn.Returns = append(syn.Returns, w)
			}
		}
		r.Syntax[ext] = syn
	}
	licensed, manifest := map[string]bool{}, map[string]bool{}
	for _, s := range sketches {
		f := units.File{Path: s.Path, Blob: s.Blob, Mode: s.Mode}
		for _, v := range s.Units {
			f.Units = append(f.Units, v.Unit)
			r.TestCounts[UnitKey(s.Path, v.Unit)] = v.Identifiers
		}
		f, err = units.Reclassify(ctx, f, r.SyntaxFor(s.Path))
		if err != nil {
			return r, err
		}
		for i := range f.Units {
			f.Units[i].ID = s.Units[i].Unit.ID
		}
		r.Files[s.Path] = f
		r.Roles[s.Path] = s.Role
		licensed[path.Dir(s.Path)] = licensed[path.Dir(s.Path)] || s.Licensed
		manifest[path.Dir(s.Path)] = manifest[path.Dir(s.Path)] || s.Role.Manifest
	}
	for w, n := range r.Frequency {
		r.IDF[w] = math.Log(float64(max(1, r.UnitCount)) / float64(max(1, n)))
	}
	if err = learnTests(ctx, &r, runs); err != nil {
		return r, err
	}
	learnEntries(&r)
	for _, s := range sketches {
		role := r.Roles[s.Path]
		for _, u := range r.Files[s.Path].Units {
			role.Test = role.Test || r.TestUnits[UnitKey(s.Path, u)]
		}
		for dir := path.Dir(s.Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			role.Dependency = role.Dependency || licensed[dir] && manifest[dir]
		}
		r.Roles[s.Path] = role
	}
	r.Results = LearnResults(runs, CaseNames(r))
	return r, ctx.Err()
}
