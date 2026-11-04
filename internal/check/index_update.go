package check

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"reflect"
	"strings"
)

func (x *repositoryIndex) sketches() []repostats.FileSketch {
	rows := x.prefix("learning", "")
	var out []repostats.FileSketch
	for _, p := range keys(rows) {
		raw, ok := rows[p][""]
		if !ok {
			continue
		}
		var s repostats.FileSketch
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			x.err = err
		}
		out = append(out, s)
	}
	return out
}

func (x *repositoryIndex) replaceValues(kind string, values map[string]string) {
	old := x.prefix(kind, "")
	for _, k := range keys(old) {
		if _, ok := values[k]; !ok {
			x.put(kind, k, "", nil)
		}
	}
	for _, k := range keys(values) {
		var v any
		if err := json.Unmarshal([]byte(values[k]), &v); err != nil {
			x.err = err
			return
		}
		x.put(kind, k, "", v)
	}
}

func (x *repositoryIndex) conventions(sketches []repostats.FileSketch, r repostats.Repository) {
	defer func() { x.files = map[string]units.File{} }()
	x.meta.Converged, x.meta.Iterations, x.meta.LogPrior = r.Converged, r.TestIterations, r.Tests.LogPrior
	syntax := map[string]string{}
	for k, v := range r.Syntax {
		syntax[k] = encode(v)
	}
	x.replaceValues("syntax", syntax)
	model, words, fold, entries := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	for k, v := range r.Tests.Weights {
		model[k] = encode(v)
	}
	lower := map[string]float64{}
	for k, v := range r.TestWords {
		words[k] = encode(v)
		lower[strings.ToLower(k)] = max(lower[strings.ToLower(k)], v)
	}
	for k, v := range lower {
		fold[k] = encode(v)
	}
	for _, e := range r.Entries {
		entries[e.Kind+"\x00"+e.Value] = encode(e)
	}
	x.replaceValues("test_model", model)
	x.replaceValues("test_word", words)
	x.replaceValues("test_fold", fold)
	x.replaceValues("entry", entries)
	for _, sketch := range sketches {
		p := sketch.Path
		next := r.Files[p]
		tests, seeds := map[string]bool{}, map[string]bool{}
		namesChanged := false
		for i, u := range next.Units {
			key := repostats.UnitKey(p, u)
			tests[key], seeds[key] = r.TestUnits[key], r.TestSeeds[key]
			namesChanged = namesChanged || u.Name != sketch.Units[i].Unit.Name
		}
		if !namesChanged && sketch.Classified == r.Roles[p] && reflect.DeepEqual(sketch.Tests, tests) && reflect.DeepEqual(sketch.Seeds, seeds) {
			continue
		}
		info := x.fileInfo(p)
		rows := map[string]ledger.IndexRow{}
		for _, row := range info.Rows {
			rows[rowKey(row.Kind, row.Key)+"\x00"+row.Member] = row
		}
		update := func(kind, key, member string, v any) {
			x.put(kind, key, member, v)
			k := rowKey(kind, key) + "\x00" + member
			if v == nil {
				delete(rows, k)
			} else {
				rows[k] = ledger.IndexRow{Kind: kind, Key: key, Member: member}
			}
		}
		for i, u := range next.Units {
			key := repostats.UnitKey(p, u)
			old := sketch.Units[i].Unit.Name
			v := x.unit(key)
			members := []string(nil)
			for j, child := range next.Units {
				if child.Parent >= 0 && next.Units[child.Parent].ID == u.ID && j != u.Parent && child.Name != "" {
					members = append(members, child.Name)
				}
			}
			if v.Unit.Name != u.Name || !reflect.DeepEqual(v.Structure.Members, members) {
				v.Unit.Name, v.Structure.Members = u.Name, members
				x.put("unit", key, "", v)
			}
			if old != u.Name {
				if old != "" {
					update("name", old, key, nil)
					update("fold_name", strings.ToLower(old), key, nil)
					update("match", strings.ToLower(p+":"+old), key, nil)
				}
				if u.Name != "" {
					update("name", u.Name, key, true)
					update("fold_name", strings.ToLower(u.Name), key, u.Name)
					update("match", strings.ToLower(p+":"+u.Name), key, "u:"+key)
				}
			}
			if old != "" && sketch.Tests[key] && (old != u.Name || !tests[key]) {
				update("case_name", old, key, nil)
			}
			if u.Name != "" && tests[key] {
				update("case_name", u.Name, key, true)
			}
			sketch.Units[i].Unit.Name = u.Name
		}
		if info.Role.Test != r.Roles[p].Test {
			sign := 1
			if r.Roles[p].Test {
				sign = -1
			}
			for w, n := range sketch.Uses {
				x.statDelta(w, nil, sign*n)
			}
		}
		info.Role, info.Tests, info.Seeds = r.Roles[p], tests, seeds
		info.Rows = nil
		for _, k := range keys(rows) {
			info.Rows = append(info.Rows, rows[k])
		}
		x.put("file", p, "", info)
		sketch.Classified, sketch.Tests, sketch.Seeds = info.Role, tests, seeds
		x.put("learning", p, "", sketch)
		delete(x.files, p)
	}
}

func taskIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord) (*repositoryIndex, error) {
	base, err := openIndex(ctx, s, w, task.OriginTree)
	if err != nil {
		return nil, err
	}
	records, err := s.Verifications(ctx, w.ID, task.ID)
	if err != nil {
		return nil, err
	}
	return observedIndex(base, runSamples(records))
}

func observedIndex(base *repositoryIndex, runs []repostats.Run) (*repositoryIndex, error) {
	if len(runs) == 0 {
		return base, base.err
	}
	seeds := map[string]bool{}
	for _, run := range runs {
		if !run.Started || run.ExitCode == nil {
			continue
		}
		matches, err := base.matcher("graph", strings.ToLower(run.Output))
		if err != nil {
			return base, err
		}
		for _, match := range matches {
			for _, raw := range base.read("fold_name", match.Pattern) {
				var name string
				if err := json.Unmarshal([]byte(raw), &name); err != nil {
					return base, err
				}
				if units.HasWord(run.Output, name) {
					seeds[name] = true
				}
			}
		}
	}
	names := keys(seeds)
	if base.err != nil {
		return base, base.err
	}
	variant := Algorithm + ":seeds:" + units.Hash(encode(names))
	saved, found, err := base.s.FindIndex(base.ctx, base.w.ProjectID, base.meta.Tree, variant)
	if err != nil {
		return base, err
	}
	if found {
		x := newIndex(base.ctx, base.s, base.w)
		x.id, x.variant = saved.ID, variant
		err = json.Unmarshal(saved.Data, &x.meta)
		return x, err
	}
	x := base.overlay(base.meta.Tree)
	x.variant = variant
	sketches := x.sketches()
	if x.err != nil {
		return x, x.err
	}
	zero := 0
	r, err := repostats.Relearn(x.ctx, x.meta.Tree, sketches, []repostats.Run{{Output: strings.Join(names, "\n"), Started: true, ExitCode: &zero}})
	if err != nil {
		return x, err
	}
	x.conventions(sketches, r)
	x.buildMatchers()
	id := units.Hash(variant, x.w.ProjectID, x.meta.Tree)
	if err = x.publish(id, base.id); err != nil {
		return x, err
	}
	x.writes = map[string]ledger.IndexRow{}
	return x, x.err
}

func buildNextIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, parent ledger.AnalysisIndex, tree string) (*repositoryIndex, error) {
	base := newIndex(ctx, s, w)
	base.id = parent.ID
	if err := json.Unmarshal(parent.Data, &base.meta); err != nil {
		return base, err
	}
	x := base.overlay(tree)
	diff, err := w.Diff(ctx, parent.Tree, tree)
	if err != nil {
		return x, err
	}
	changed := map[string]bool{}
	entries := map[string]gitstate.File{}
	for _, d := range diff {
		changed[d.BeforePath] = true
		changed[d.Path] = true
		entries[d.BeforePath] = gitstate.File{}
		entries[d.Path] = d.After
	}
	var ids []string
	for _, e := range entries {
		if e.Blob != "" && e.Mode != "160000" {
			ids = append(ids, e.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return x, err
	}
	for _, p := range keys(changed) {
		if old := base.load(p); old.Blob != "" {
			x.changeFile(p, old, base.view([]string{p}), -1)
		}
		x.put("learning", p, "", nil)
		e := entries[p]
		if e.Blob == "" {
			continue
		}
		var syntax units.Syntax
		base.get("syntax", path.Ext(p), &syntax)
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: e.Blob, Mode: e.Mode, Data: blobs[e.Blob]}, syntax)
		if err != nil {
			return x, err
		}
		r, err := base.apply([]string{p}, map[string]units.File{p: f}, tree)
		if err != nil {
			return x, err
		}
		x.changeFile(p, f, r, 1)
		sketch, err := repostats.Sketch(ctx, f, blobs[e.Blob])
		if err != nil {
			return x, err
		}
		sketch.Classified = r.Roles[p]
		info := x.fileInfo(p)
		sketch.Tests, sketch.Seeds = info.Tests, info.Seeds
		x.put("learning", p, "", sketch)
	}
	sketches := x.sketches()
	if x.err != nil {
		return x, x.err
	}
	learned, err := repostats.Relearn(ctx, tree, sketches, nil)
	if err != nil {
		return x, err
	}
	invalid := map[string]bool{}
	for ext, syntax := range learned.Syntax {
		var old units.Syntax
		base.get("syntax", ext, &old)
		if !reflect.DeepEqual(old.Quotes, syntax.Quotes) || !reflect.DeepEqual(old.Comments, syntax.Comments) {
			invalid[ext] = true
		}
	}
	for _, sketch := range sketches {
		p := sketch.Path
		if !invalid[path.Ext(p)] {
			continue
		}
		old := x.load(p)
		view := x.view([]string{p})
		data, err := w.ReadBlob(ctx, sketch.Blob)
		if err != nil {
			return x, err
		}
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: sketch.Blob, Mode: sketch.Mode, Data: data}, learned.SyntaxFor(p))
		if err != nil {
			return x, err
		}
		r, err := repostats.Apply(ctx, learned, tree, map[string]units.File{p: f})
		if err != nil {
			return x, err
		}
		x.changeFile(p, old, view, -1)
		x.changeFile(p, f, r, 1)
		next, err := repostats.Sketch(ctx, f, data)
		if err != nil {
			return x, err
		}
		next.Classified = r.Roles[p]
		info := x.fileInfo(p)
		next.Tests, next.Seeds = info.Tests, info.Seeds
		x.put("learning", p, "", next)
		changed[p] = true
	}
	if len(invalid) > 0 {
		sketches = x.sketches()
		learned, err = repostats.Relearn(ctx, tree, sketches, nil)
		if err != nil {
			return x, err
		}
	}
	x.conventions(sketches, learned)
	x.buildMatchers()
	if err = x.updateValidation(base, keys(changed)); err != nil {
		return x, err
	}
	id := units.Hash(Algorithm, w.ProjectID, tree, base.id)
	for _, p := range keys(changed) {
		if info := x.fileInfo(p); info.Blob != "" && !info.Role.Document {
			if err = x.saveText(id, x.load(p)); err != nil {
				return x, err
			}
		}
	}
	if err = x.refreshStatistics(base, keys(changed)); err != nil {
		return x, err
	}
	if err = x.publish(id, base.id); err != nil {
		return x, err
	}
	x.writes = map[string]ledger.IndexRow{}
	return x, x.err
}
