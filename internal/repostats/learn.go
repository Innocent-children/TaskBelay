package repostats

import (
	"context"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/units"
)

type Input struct {
	Tree    string
	Sources []units.Source
	Runs    []Run
	Cached  map[string]units.File
	Syntax  map[string]units.Syntax
}
type Role struct{ Test, Generated, Lock, Manifest, Dependency, Document, Config bool }
type Pattern struct {
	Kind, Value string
	Probability float64
}
type Repository struct {
	Tree                     string
	Syntax                   map[string]units.Syntax
	Files                    map[string]units.File
	Roles                    map[string]Role
	Frequency, HeadFrequency map[string]int
	IDF                      map[string]float64
	TestWords                map[string]float64
	TestUnits                map[string]bool
	TestSeeds                map[string]bool
	Tests                    TestModel
	TestIterations           int
	Entries                  []Pattern
	UnitCount                int
	Converged                bool
	Results                  ResultModel
}

func Learn(ctx context.Context, in Input) (Repository, error) {
	r := Repository{Tree: in.Tree, Syntax: map[string]units.Syntax{}, Files: map[string]units.File{}, Roles: map[string]Role{}, Frequency: map[string]int{}, HeadFrequency: map[string]int{}, IDF: map[string]float64{}, TestWords: map[string]float64{}, TestUnits: map[string]bool{}}
	sources := append([]units.Source(nil), in.Sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	groups := map[string][]units.Source{}
	for _, src := range sources {
		groups[path.Ext(src.Path)] = append(groups[path.Ext(src.Path)], src)
	}
	if in.Syntax == nil {
		var err error
		in.Syntax, err = LearnSyntax(ctx, sources, nil)
		if err != nil {
			return r, err
		}
	}
	for ext, syntax := range in.Syntax {
		r.Syntax[ext] = syntax
	}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		syn := r.Syntax[path.Ext(src.Path)]
		key := units.Hash(src.Blob, syn.Key())
		f, ok := in.Cached[key]
		if !ok {
			var err error
			f, err = units.Analyze(ctx, src, syn)
			if err != nil {
				return r, err
			}
		}
		f.Path, f.Blob, f.Mode = src.Path, src.Blob, src.Mode
		r.Files[src.Path] = f
	}
	for _, ext := range sortedKeys(groups) {
		syn := r.Syntax[ext]
		headDF := map[string]int{}
		ends := map[string]int{}
		calls, identifiers := 0, 0
		for _, source := range groups[ext] {
			f := r.Files[source.Path]
			for _, u := range f.Units {
				r.UnitCount++
				seen := map[string]bool{}
				heads := map[string]bool{}
				for i, t := range u.Tokens {
					if t.Kind == "identifier" || t.Kind == "key" {
						w := units.Literal(t)
						seen[w] = true
						if t.End <= u.HeaderEnd {
							heads[w] = true
						}
						identifiers++
						if i+1 < len(u.Tokens) && strings.Contains("([<", u.Tokens[i+1].Text) && len(u.Tokens[i+1].Text) == 1 {
							calls++
						}
					}
				}
				for w := range seen {
					r.Frequency[w]++
				}
				for w := range heads {
					r.HeadFrequency[w]++
					headDF[w]++
				}
				lastLine := u.EndLine
				for i := len(u.Tokens) - 1; i >= 0; i-- {
					t := u.Tokens[i]
					if t.Kind == "identifier" && t.Line < lastLine {
						lastLine = t.Line
						break
					}
				}
				for _, t := range u.Tokens {
					if t.Line == lastLine && t.Kind == "identifier" {
						ends[t.Text]++
						break
					}
				}
			}
			syn.Samples += len(f.Units)
		}
		if identifiers > 0 {
			syn.CallRatio = float64(calls) / float64(identifiers)
		}
		for _, w := range sortedKeys(headDF) {
			if float64(headDF[w]) > float64(syn.Samples)/2 {
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
		syn.Keywords = unique(syn.Keywords)
		syn.Returns = unique(syn.Returns)
		r.Syntax[ext] = syn
	}
	// 用已收敛的首行统计再次确定名字，缓存键包含约定，避免跨项目误用。
	for _, src := range sources {
		syn := r.Syntax[path.Ext(src.Path)]
		f, err := units.Reclassify(ctx, r.Files[src.Path], syn)
		if err != nil {
			return r, err
		}
		f.Path, f.Blob, f.Mode = src.Path, src.Blob, src.Mode
		r.Files[src.Path] = f
		r.Roles[src.Path] = classify(f)
	}
	for w, n := range r.Frequency {
		r.IDF[w] = math.Log(float64(max(1, r.UnitCount)) / float64(max(1, n)))
	}
	if err := learnTests(ctx, &r, in.Runs); err != nil {
		return r, err
	}
	learnEntries(&r)
	for _, p := range sortedKeys(r.Files) {
		role := r.Roles[p]
		for _, u := range r.Files[p].Units {
			if r.TestUnits[UnitKey(p, u)] {
				role.Test = true
			}
		}
		r.Roles[p] = role
	}
	for _, p := range sortedKeys(r.Roles) {
		if !r.Roles[p].Manifest {
			continue
		}
		dir := path.Dir(p)
		if dir == "." {
			continue
		}
		licensed := false
		for _, q := range sortedKeys(r.Files) {
			if path.Dir(q) == dir {
				for _, w := range units.Words(strings.Join(r.Files[q].Lines, "")) {
					if w == "license" || w == "licence" || w == "copyright" {
						licensed = true
						break
					}
				}
			}
		}
		if licensed {
			for _, q := range sortedKeys(r.Files) {
				if strings.HasPrefix(q, dir+"/") {
					role := r.Roles[q]
					role.Dependency = true
					r.Roles[q] = role
				}
			}
		}
	}
	r.Results = LearnResults(in.Runs, CaseNames(r))
	return r, ctx.Err()
}

func UnitKey(p string, u units.Unit) string          { return p + ":" + u.ID }
func (r Repository) SyntaxFor(p string) units.Syntax { return r.Syntax[path.Ext(p)] }
func (r Repository) Entry(p string, u units.Unit) bool {
	for _, pattern := range r.Entries {
		switch pattern.Kind {
		case "prefix":
			if strings.HasPrefix(u.Name, pattern.Value) {
				return true
			}
		case "suffix":
			if strings.HasSuffix(u.Name, pattern.Value) {
				return true
			}
		case "path":
			if strings.Contains(p, pattern.Value) {
				return true
			}
		case "header":
			for _, t := range u.Tokens {
				if t.End <= u.HeaderEnd && t.Text == pattern.Value {
					return true
				}
			}
		}
	}
	return false
}

var versionPattern = regexp.MustCompile(`(^|[^[:alnum:]])[[:digit:]]+\.[[:digit:]]+(\.[[:digit:]]+)?`)
var hashPattern = regexp.MustCompile(`\b[[:xdigit:]]{` + strconv.Itoa(HashCharacters) + `,}\b`)

func classify(f units.File) Role {
	role := Role{}
	if f.Binary {
		return role
	}
	count, versions, hashes, keys, prose, symbols := 0, 0, 0, 0, 0, 0
	for _, line := range f.Lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		count++
		if versionPattern.MatchString(trim) {
			versions++
		}
		if hashPattern.MatchString(trim) {
			hashes++
		}
		if i := strings.IndexAny(trim, ":="); i > 0 && len(strings.Fields(trim[:i])) == 1 && !strings.ContainsAny(trim, "()") {
			keys++
		}
		prose += len(strings.Fields(trim))
		for _, c := range trim {
			if unicode.IsPunct(c) || unicode.IsSymbol(c) {
				symbols++
			}
		}
	}
	role.Lock = versions > 0 && versions == hashes && (float64(versions)+Prior)/float64(count+1) > Prior
	role.Manifest = versions > 0 && hashes == 0 && (float64(versions)+Prior)/float64(count+1) > Prior
	role.Config = keys > 0 && (float64(keys)+Prior)/float64(count+1) > Prior
	// 只有明确的自然语言句子且没有执行结构才排除；代码信息少时宁可计入验证输入。
	sentences, executable := 0, false
	for _, line := range f.Lines {
		trim := strings.TrimSpace(line)
		if strings.HasSuffix(trim, "。") || strings.HasSuffix(trim, ".") || strings.HasSuffix(trim, "！") || strings.HasSuffix(trim, "？") {
			sentences++
		}
	}
	for _, t := range f.Tokens {
		if t.Kind == "symbol" && strings.ContainsAny(t.Text, "=(){};") {
			executable = true
		}
	}
	role.Document = sentences > 0 && !executable && prose > symbols && !role.Config && !role.Manifest && !role.Lock
	if len(f.Lines) > 0 {
		words := units.Words(f.Lines[0])
		for _, w := range words {
			if w == "generated" || w == "生成" {
				role.Generated = true
			}
		}
	}
	return role
}

func sortedKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func unique(xs []string) []string {
	sort.Strings(xs)
	out := xs[:0]
	for _, x := range xs {
		if len(out) == 0 || out[len(out)-1] != x {
			out = append(out, x)
		}
	}
	return out
}
