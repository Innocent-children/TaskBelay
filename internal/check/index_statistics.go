package check

import (
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"reflect"
)

func (x *repositoryIndex) adjustVariation(items []signals.Variation, sign int) {
	for _, v := range items {
		n := x.meta.VariationCounts[v.Kind]
		n[0] += sign
		if v.Count == 1 {
			n[1] += sign
		}
		x.meta.VariationCounts[v.Kind] = n
	}
}
func (x *repositoryIndex) refreshStatistics(before *repositoryIndex, changed []string) error {
	paths := map[string]bool{}
	for _, p := range changed {
		paths[p] = true
	}
	for _, row := range x.writes {
		if row.Kind == "syntax" {
			var old, next units.Syntax
			before.get("syntax", row.Key, &old)
			x.get("syntax", row.Key, &next)
			if reflect.DeepEqual(old, next) {
				continue
			}
		}
		if row.Kind == "head" || row.Kind == "name" || row.Kind == "calls" || row.Kind == "syntax" {
			for p := range before.read("signal_dep", rowKey(row.Kind, row.Key)) {
				paths[p] = true
			}
		}
		if row.Kind == "unit" && row.Data != "" {
			var u indexUnit
			if err := json.Unmarshal([]byte(row.Data), &u); err != nil {
				return err
			}
			paths[u.Path] = true
		}
	}
	sketches := x.sketches()
	for _, sketch := range sketches {
		var old, next units.Syntax
		before.get("syntax", path.Ext(sketch.Path), &old)
		x.get("syntax", path.Ext(sketch.Path), &next)
		if !reflect.DeepEqual(old.Keywords, next.Keywords) || !reflect.DeepEqual(old.Returns, next.Returns) || (old.CallRatio <= signals.CallPrecision) != (next.CallRatio <= signals.CallPrecision) {
			paths[sketch.Path] = true
		}
	}
	delete(paths, "")
	for _, p := range keys(paths) {
		for _, key := range before.fileInfo(p).Units {
			var old []signals.Variation
			before.get("indirection", key, &old)
			x.adjustVariation(old, -1)
			x.put("indirection", key, "", nil)
		}
	}
	if err := x.saveFacts(keys(paths)); err != nil {
		return err
	}
	x.updatePrior()
	var order []string
	for _, s := range sketches {
		for _, u := range s.Units {
			key := repostats.UnitKey(s.Path, u.Unit)
			if u.Callable {
				order = append(order, key)
			}
		}
	}
	var distribution []float64
	for i := 0; i < len(order) && len(distribution) < signals.SimilaritySamples; i++ {
		j := (i + len(order)/2) % max(1, len(order))
		if i != j {
			distribution = append(distribution, units.Similarity(x.unit(order[i]).Grams, x.unit(order[j]).Grams))
		}
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
	x.meta.SimilarityMean, x.meta.SimilarityVariance = mean, variance
	return x.err
}

func (x *repositoryIndex) updatePrior() {
	x.meta.Indirection = map[string]float64{}
	for k, n := range x.meta.VariationCounts {
		if n[0] > 0 {
			x.meta.Indirection[k] = float64(n[1]) / float64(n[0])
		}
	}
}

/**
 * 结构摘要和实际查询的依赖足以重算间接层先验，不需要未改文件的完整词流。
 */
func (x *repositoryIndex) saveFacts(paths []string) error {
	r := repostats.Repository{Files: map[string]units.File{}, Syntax: map[string]units.Syntax{}}
	dependencies := map[string]map[string]bool{}
	for _, p := range paths {
		var old []string
		x.get("signal_file", p, &old)
		for _, key := range old {
			x.put("signal_dep", key, p, nil)
		}
		f := units.File{Path: p}
		for _, key := range x.fileInfo(p).Units {
			f.Units = append(f.Units, x.unit(key).Unit)
		}
		r.Files[p] = f
		var syntax units.Syntax
		x.get("syntax", path.Ext(p), &syntax)
		r.Syntax[path.Ext(p)] = syntax
		dependencies[p] = map[string]bool{}
	}
	q := (indexedSnapshot{Origin: x, Current: x}).queries()
	q.Depend = func(p, kind, word string) {
		key := rowKey(kind, word)
		if dependencies[p][key] {
			return
		}
		dependencies[p][key] = true
		x.put("signal_dep", key, p, true)
	}
	facts, err := signals.OriginFacts(x.ctx, r, q)
	if err != nil {
		return err
	}
	for key, items := range facts {
		x.put("indirection", key, "", items)
		x.adjustVariation(items, 1)
	}
	for _, p := range paths {
		x.put("signal_file", p, "", keys(dependencies[p]))
	}
	return x.err
}
