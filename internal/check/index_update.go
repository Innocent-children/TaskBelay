package check

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"reflect"
	"strings"
)

func (x *repositoryIndex) saveSketch(sketch repostats.FileSketch) {
	sketch.Units = append([]repostats.UnitSketch(nil), sketch.Units...)
	for i, u := range sketch.Units {
		sketch.Units[i].Callable = x.unit(repostats.UnitKey(sketch.Path, u.Unit)).Structure.Callable
	}
	x.put("learning", sketch.Path, "", sketch)
}

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
	x.meta.TestExtensions = r.Tests.Extensions
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
		x.saveSketch(sketch)
		delete(x.files, p)
	}
}

// ErrIndexPending 表示所需索引尚未发布，而当前入口不负责建立它。
var ErrIndexPending = errors.New("分析索引尚未就绪")

type publishedOnlyKey struct{}

// publishedOnly 用于 hook：只读取已发布的索引。建立起点索引、扫描新的运行输出和
// 重新学习测试内容的耗时超过 hook 预算，由 MCP 后台和交付检查完成。
func publishedOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, publishedOnlyKey{}, true)
}

func usesPublishedOnly(ctx context.Context) bool {
	only, _ := ctx.Value(publishedOnlyKey{}).(bool)
	return only
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

/**
 * observedNames 返回运行输出中出现过的单元名。每条输出只扫描一次，结果按起点索引
 * 和输出内容保存；整段历史的并集按运行编号保存，新增一条运行时只需处理这一条。
 * hook 中只查询整段历史的已有结果，known 为 false 表示还有运行记录没有并入。
 */
func observedNames(base *repositoryIndex, runs []repostats.Run) (names []string, known bool, err error) {
	var eligible []repostats.Run
	for _, run := range runs {
		if run.Started && run.ExitCode != nil {
			eligible = append(eligible, run)
		}
	}
	if len(eligible) == 0 {
		return []string{}, true, nil
	}
	history := func(n int) string {
		parts := []string{Algorithm, base.id}
		for _, run := range eligible[:n] {
			parts = append(parts, run.ID, units.Hash(run.Output))
		}
		return units.Hash(parts...)
	}
	found, err := base.s.AnalysisCache(base.ctx, "observed", history(len(eligible)), &names)
	if err != nil || found || usesPublishedOnly(base.ctx) {
		return names, found, err
	}
	done := 0
	if len(eligible) > 1 {
		found, err = base.s.AnalysisCache(base.ctx, "observed", history(len(eligible)-1), &names)
		if err != nil {
			return nil, false, err
		}
		if found {
			done = len(eligible) - 1
		}
	}
	seeds := map[string]bool{}
	for _, name := range names {
		seeds[name] = true
	}
	for _, run := range eligible[done:] {
		var matched []string
		key := units.Hash(Algorithm, base.id, units.Hash(run.Output))
		found, err := base.s.AnalysisCache(base.ctx, "run-names", key, &matched)
		if err != nil {
			return nil, false, err
		}
		if !found {
			matches, err := base.matcher("graph", strings.ToLower(run.Output))
			if err != nil {
				return nil, false, err
			}
			// 同一个名字在一条输出里会命中很多次，整词核对只做一次。
			own, patterns := map[string]bool{}, map[string]bool{}
			for _, match := range matches {
				if patterns[match.Pattern] {
					continue
				}
				patterns[match.Pattern] = true
				for _, raw := range base.read("fold_name", match.Pattern) {
					var name string
					if err := json.Unmarshal([]byte(raw), &name); err != nil {
						return nil, false, err
					}
					if _, checked := own[name]; !checked {
						own[name] = units.HasWord(run.Output, name)
					}
				}
			}
			if base.err != nil {
				return nil, false, base.err
			}
			matched = []string{}
			for _, name := range keys(own) {
				if own[name] {
					matched = append(matched, name)
				}
			}
			if err := base.s.SaveAnalysisCache(base.ctx, "run-names", key, matched); err != nil {
				return nil, false, err
			}
		}
		for _, name := range matched {
			seeds[name] = true
		}
	}
	names = keys(seeds)
	return names, true, base.s.SaveAnalysisCache(base.ctx, "observed", history(len(eligible)), names)
}

func observedIndex(base *repositoryIndex, runs []repostats.Run) (*repositoryIndex, error) {
	if len(runs) == 0 {
		return base, base.err
	}
	names, known, err := observedNames(base, runs)
	if err != nil {
		return base, err
	}
	if base.err != nil {
		return base, base.err
	}
	open := func(saved ledger.AnalysisIndex) (*repositoryIndex, error) {
		x := newIndex(base.ctx, base.s, base.w)
		x.id, x.variant = saved.ID, saved.Variant
		return x, json.Unmarshal(saved.Data, &x.meta)
	}
	variant := Algorithm + ":seeds:" + units.Hash(encode(names))
	if known {
		saved, found, err := base.s.FindIndex(base.ctx, base.w.ProjectID, base.meta.Tree, variant)
		if err != nil {
			return base, err
		}
		if found {
			return open(saved)
		}
	}
	if usesPublishedOnly(base.ctx) {
		// 最新的运行记录尚未并入时，沿用这个起点上最近一次发布的识别结果。
		saved, found, err := base.s.LatestDerivedIndex(base.ctx, base.w.ProjectID, base.id)
		if err != nil {
			return base, err
		}
		if !found {
			return base, ErrIndexPending
		}
		return open(saved)
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
		x.saveSketch(sketch)
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
		x.saveSketch(next)
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
