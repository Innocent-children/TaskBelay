package check

import (
	"context"
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"sort"
	"strings"
)

func sources(ctx context.Context, w gitstate.Workspace, tree string) ([]units.Source, error) {
	files, err := w.TreeFiles(ctx, tree)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range keys(files) {
		if files[p].Mode != "160000" {
			ids = append(ids, files[p].Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []units.Source
	for _, p := range keys(files) {
		f := files[p]
		out = append(out, units.Source{Path: p, Blob: f.Blob, Mode: f.Mode, Data: blobs[f.Blob]})
	}
	return out, nil
}

type indexedSnapshot struct {
	Origin, Current, Graph *repositoryIndex
	Before, After          repostats.Repository
	Paths                  []string
}

func (x *repositoryIndex) overlay(tree string) *repositoryIndex {
	y := newIndex(x.ctx, x.s, x.w)
	y.id = x.id
	y.variant = x.variant
	if err := json.Unmarshal([]byte(encode(x.meta)), &y.meta); err != nil {
		y.err = err
	}
	y.meta.Tree = tree
	return y
}

func (x *repositoryIndex) apply(paths []string, files map[string]units.File, tree string) (repostats.Repository, error) {
	base := x.view(paths)
	for p, f := range files {
		var syn units.Syntax
		x.get("syntax", path.Ext(p), &syn)
		base.Syntax[path.Ext(p)] = syn
		for _, t := range f.Tokens {
			if t.Kind == "identifier" {
				var v float64
				x.get("test_model", t.Text, &v)
				base.Tests.Weights[t.Text] = v
			}
		}
	}
	after, err := repostats.Apply(x.ctx, base, tree, files)
	if err != nil {
		return after, err
	}
	return after, x.err
}

func snapshots(ctx context.Context, s *ledger.Store, w gitstate.Workspace, origin, tree string, runs []repostats.Run) (indexedSnapshot, error) {
	var out indexedSnapshot
	base, err := openIndex(ctx, s, w, origin)
	if err != nil {
		return out, err
	}
	base, err = observedIndex(base, runs)
	if err != nil {
		return out, err
	}
	out.Origin = base
	out.Current = base.overlay(tree)
	diff, err := w.Diff(ctx, origin, tree)
	if err != nil {
		return out, err
	}
	paths := map[string]bool{}
	entries := map[string]gitstate.File{}
	for _, d := range diff {
		paths[d.BeforePath] = true
		paths[d.Path] = true
		entries[d.BeforePath] = gitstate.File{}
		entries[d.Path] = d.After
	}
	out.Paths = keys(paths)
	out.Before = base.view(out.Paths)
	var ids []string
	for _, e := range entries {
		if e.Blob != "" && e.Mode != "160000" {
			ids = append(ids, e.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return out, err
	}
	files := map[string]units.File{}
	for _, p := range out.Paths {
		e := entries[p]
		if e.Blob == "" {
			continue
		}
		var syn units.Syntax
		base.get("syntax", path.Ext(p), &syn)
		key := units.Hash(Algorithm, e.Blob, syn.Key())
		var f units.File
		found, err := s.AnalysisCache(ctx, "file", key, &f)
		if err != nil {
			return out, err
		}
		if !found {
			f, err = units.Analyze(ctx, units.Source{Path: p, Blob: e.Blob, Mode: e.Mode, Data: blobs[e.Blob]}, syn)
			if err != nil {
				return out, err
			}
		}
		f.Path, f.Blob, f.Mode = p, e.Blob, e.Mode
		files[p] = f
	}
	out.After, err = base.apply(out.Paths, files, tree)
	if err != nil {
		return out, err
	}
	for _, p := range out.Paths {
		if f, ok := out.Before.Files[p]; ok {
			out.Current.changeFile(p, f, out.Before, -1)
		}
		if f, ok := out.After.Files[p]; ok {
			out.Current.changeFile(p, f, out.After, 1)
		}
	}
	out.Current.buildMatchers()
	if err = out.Current.updateValidation(base, out.Paths); err != nil {
		return out, err
	}
	// 删除侧仍参与变更簇的图定位；验证输入只包含当前 tree 的文件。
	out.Graph = out.Current
	for _, p := range out.Paths {
		if _, ok := out.After.Files[p]; !ok {
			if f, old := out.Before.Files[p]; old {
				if out.Graph == out.Current {
					out.Graph = out.Current.copyOverlay()
				}
				out.Graph.changeFile(p, f, out.Before, 1)
			}
		}
	}
	return out, out.err()
}

func (x *repositoryIndex) copyOverlay() *repositoryIndex {
	y := x.overlay(x.meta.Tree)
	for k, r := range x.rows {
		copy := map[string]string{}
		for m, v := range r {
			copy[m] = v
		}
		y.rows[k] = copy
		y.loaded[k] = x.loaded[k]
		kind, key, _ := strings.Cut(k, "\x00")
		if y.rowKinds[kind] == nil {
			y.rowKinds[kind] = map[string]bool{}
		}
		y.rowKinds[kind][key] = true
	}
	for p, f := range x.files {
		y.files[p] = f
	}
	for key, node := range x.volumes {
		copy := changes.VolumeNode{}
		for b, v := range node {
			copy[b] = v
		}
		y.volumes[key] = copy
		y.volumeDirty[key] = x.volumeDirty[key]
	}
	return y
}

func (s indexedSnapshot) err() error {
	for _, x := range []*repositoryIndex{s.Origin, s.Current, s.Graph} {
		if x != nil && x.err != nil {
			return x.err
		}
	}
	return nil
}

func keys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func runSamples(records []ledger.VerificationRecord) []repostats.Run {
	var out []repostats.Run
	for _, r := range records {
		if r.Algorithm == Algorithm && r.Executed && r.ExitCode != nil && !r.EndedAt.IsZero() {
			out = append(out, repostats.Run{ID: r.ID, Output: r.Output, ExitCode: r.ExitCode, Started: true})
		}
	}
	return out
}
