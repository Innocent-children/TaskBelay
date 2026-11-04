package repostats

import (
	"context"
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
	FrequencyLookup          func(string) int               `json:"-"`
	EntryLookup              func(string, units.Unit) bool  `json:"-"`
	TestWordLookup           func(string, bool) float64     `json:"-"`
	NamesLookup              func(string) ([]string, error) `json:"-"`
	Tree                     string
	Syntax                   map[string]units.Syntax
	Files                    map[string]units.File
	Roles                    map[string]Role
	Frequency, HeadFrequency map[string]int
	IDF                      map[string]float64
	TestWords                map[string]float64
	TestUnits                map[string]bool
	TestSeeds                map[string]bool
	TestCounts               map[string]map[string]int `json:"-"`
	Tests                    TestModel
	TestIterations           int
	Entries                  []Pattern
	UnitCount                int
	Converged                bool
	Results                  ResultModel
}

func Learn(ctx context.Context, in Input) (Repository, error) {
	sources := append([]units.Source(nil), in.Sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	syntax := in.Syntax
	var err error
	if syntax == nil {
		syntax, err = LearnSyntax(ctx, sources, nil)
		if err != nil {
			return Repository{}, err
		}
	}
	files := map[string]units.File{}
	var sketches []FileSketch
	for _, src := range sources {
		syn := syntax[path.Ext(src.Path)]
		key := units.Hash(src.Blob, syn.Key())
		f, found := in.Cached[key]
		if !found {
			f, err = units.Analyze(ctx, src, syn)
			if err != nil {
				return Repository{}, err
			}
		}
		f.Path, f.Blob, f.Mode = src.Path, src.Blob, src.Mode
		files[src.Path] = f
		sketch, err := Sketch(ctx, f, src.Data)
		if err != nil {
			return Repository{}, err
		}
		sketches = append(sketches, sketch)
	}
	r, err := Relearn(ctx, in.Tree, sketches, in.Runs)
	if err != nil {
		return r, err
	}
	for _, p := range sortedKeys(files) {
		f, err := units.Reclassify(ctx, files[p], r.SyntaxFor(p))
		if err != nil {
			return r, err
		}
		r.Files[p] = f
	}
	r.TestCounts = nil
	return r, ctx.Err()
}

func UnitKey(p string, u units.Unit) string          { return p + ":" + u.ID }
func (r Repository) SyntaxFor(p string) units.Syntax { return r.Syntax[path.Ext(p)] }
func (r Repository) Entry(p string, u units.Unit) bool {
	if r.EntryLookup != nil {
		return r.EntryLookup(p, u)
	}
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

func (r Repository) FrequencyOf(word string) int {
	if r.FrequencyLookup != nil {
		return r.FrequencyLookup(word)
	}
	return r.Frequency[word]
}
func (r Repository) TestWord(word string, fold bool) float64 {
	if r.TestWordLookup != nil {
		return r.TestWordLookup(word, fold)
	}
	if !fold {
		return r.TestWords[word]
	}
	best := 0.0
	for w, v := range r.TestWords {
		if strings.EqualFold(word, w) {
			best = max(best, v)
		}
	}
	return best
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
