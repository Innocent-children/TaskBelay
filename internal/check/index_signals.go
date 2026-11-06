package check

import (
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
)

func (s indexedSnapshot) queries() *signals.Queries {
	x, origin := s.Current, s.Origin
	q := &signals.Queries{Prior: origin.meta.Indirection, Mean: origin.meta.SimilarityMean, Variance: origin.meta.SimilarityVariance}
	callsByName := map[string][]signals.CallSite{}
	headsByWord := map[string][]signals.Head{}
	syntaxByExtension := map[string]units.Syntax{}
	q.Syntax = func(p string) units.Syntax {
		ext := path.Ext(p)
		if syntax, ok := syntaxByExtension[ext]; ok {
			return syntax
		}
		var syntax units.Syntax
		x.get("syntax", ext, &syntax)
		syntaxByExtension[ext] = syntax
		return syntax
	}
	q.Structure = func(key string) signals.UnitStructure { return x.unit(key).Structure }
	q.Calls = func(name string) []signals.CallSite {
		if calls, ok := callsByName[name]; ok {
			return calls
		}
		var out []signals.CallSite
		rows := x.read("calls", name)
		for _, p := range keys(rows) {
			var calls [][]units.Token
			if err := json.Unmarshal([]byte(rows[p]), &calls); err != nil {
				x.err = err
			}
			for _, args := range calls {
				out = append(out, signals.CallSite{Path: p, Args: args})
			}
		}
		callsByName[name] = out
		return out
	}
	q.Heads = func(word string) []signals.Head {
		if heads, ok := headsByWord[word]; ok {
			return heads
		}
		var out []signals.Head
		for _, id := range keys(x.read("head", word)) {
			u := x.unit(id)
			out = append(out, signals.Head{ID: id, Name: u.Unit.Name, Words: u.Head})
		}
		headsByWord[word] = out
		return out
	}
	q.Defined = func(name string) bool { return len(x.read("name", name)) > 0 }
	q.Uses = func(name string) int { return x.stats(name).Uses }
	q.Similar = func(sig []uint64) []signals.SimilarUnit {
		set := map[string]bool{}
		for _, bucket := range signals.SimilarityBuckets(sig) {
			for key := range origin.read("lsh", bucket) {
				set[key] = true
			}
		}
		var out []signals.SimilarUnit
		for _, key := range keys(set) {
			u := origin.unit(key)
			out = append(out, signals.SimilarUnit{Path: u.Path, Unit: u.Unit, Grams: u.Grams, Signature: u.Signature, Callable: u.Structure.Callable})
		}
		return out
	}
	return q
}

/**
 * 用例只补齐被改动声明及其被测对象的直接引用邻域；其余计算查询计数和首行汇总。
 */
func (s *indexedSnapshot) caseNeighbourhood() {
	paths := map[string]bool{}
	names := map[string]bool{}
	for _, p := range s.Paths {
		paths[p] = true
		for _, u := range s.After.Files[p].Units {
			if !s.After.Roles[p].Test && u.Name != "" {
				names[u.Name] = true
			}
			if s.After.TestUnits[repostats.UnitKey(p, u)] {
				for _, t := range u.Tokens {
					if t.Kind == "identifier" {
						for key := range s.Current.read("name", t.Text) {
							target := s.Current.unit(key)
							if !s.Current.fileInfo(target.Path).Role.Test {
								names[t.Text] = true
								paths[target.Path] = true
							}
						}
					}
				}
			}
		}
	}
	for _, name := range keys(names) {
		for key := range s.Current.read("word", name) {
			u := s.Current.unit(key)
			if s.Current.fileInfo(u.Path).Role.Test {
				paths[u.Path] = true
			}
		}
	}
	// 新纳入的既有用例可能还引用其他被测声明，边界与等价类必须用全部直接目标。
	for _, p := range keys(paths) {
		f := s.Current.load(p)
		if !s.Current.fileInfo(p).Role.Test {
			continue
		}
		for _, t := range f.Tokens {
			if t.Kind == "identifier" {
				for key := range s.Current.read("name", t.Text) {
					u := s.Current.unit(key)
					if !s.Current.fileInfo(u.Path).Role.Test {
						paths[u.Path] = true
					}
				}
			}
		}
	}
	s.Before = s.Origin.view(keys(paths))
	s.After = s.Current.view(keys(paths))
}
