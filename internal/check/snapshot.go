package check

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"sort"
)

func sources(ctx context.Context, w gitstate.Workspace, tree string) ([]units.Source, error) {
	files, err := w.TreeFiles(ctx, tree)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range keys(files) {
		ids = append(ids, files[p].Blob)
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

func repository(ctx context.Context, s *ledger.Store, w gitstate.Workspace, origin, tree string, runs []repostats.Run) (repostats.Repository, error) {
	var base repostats.Repository
	key := units.Hash(Algorithm, origin)
	if found, err := s.AnalysisCache(ctx, "repository", key, &base); err != nil {
		return base, err
	} else if !found {
		src, err := sources(ctx, w, origin)
		if err != nil {
			return base, err
		}
		counts := map[string]repostats.SyntaxCounts{}
		for _, file := range src {
			key := units.Hash(Algorithm, file.Blob)
			var count repostats.SyntaxCounts
			found, err := s.AnalysisCache(ctx, "syntax", key, &count)
			if err != nil {
				return base, err
			}
			if !found {
				count, err = repostats.CountSyntax(ctx, file.Data)
				if err != nil {
					return base, err
				}
				if err = s.SaveAnalysisCache(ctx, "syntax", key, count); err != nil {
					return base, err
				}
			}
			counts[file.Blob] = count
		}
		syntax, err := repostats.LearnSyntax(ctx, src, counts)
		if err != nil {
			return base, err
		}
		cached := map[string]units.File{}
		for _, file := range src {
			key := units.Hash(file.Blob, syntax[path.Ext(file.Path)].Key())
			var f units.File
			found, err := s.AnalysisCache(ctx, "lexed", units.Hash(Algorithm, key), &f)
			if err != nil {
				return base, err
			}
			if found {
				cached[key] = f
			}
		}
		base, err = repostats.Learn(ctx, repostats.Input{Tree: origin, Sources: src, Syntax: syntax, Cached: cached})
		if err != nil {
			return base, err
		}
		if err = s.SaveAnalysisCache(ctx, "repository", key, base); err != nil {
			return base, err
		}
		for _, p := range keys(base.Files) {
			f := base.Files[p]
			lexedKey := units.Hash(f.Blob, syntax[path.Ext(p)].Key())
			if _, found := cached[lexedKey]; !found {
				if err = s.SaveAnalysisCache(ctx, "lexed", units.Hash(Algorithm, lexedKey), f); err != nil {
					return base, err
				}
			}
			if err = s.SaveAnalysisCache(ctx, "file", units.Hash(Algorithm, f.Blob, base.SyntaxFor(p).Key()), f); err != nil {
				return base, err
			}
		}
	}
	if tree == origin {
		return repostats.Observe(ctx, base, runs)
	}
	currentKey := units.Hash(key, tree)
	var current repostats.Repository
	if found, err := s.AnalysisCache(ctx, "repository", currentKey, &current); found || err != nil {
		if err != nil {
			return current, err
		}
		return observedRepository(ctx, base, current, runs)
	}
	entries, err := w.TreeFiles(ctx, tree)
	if err != nil {
		return current, err
	}
	files := map[string]units.File{}
	var missing []string
	var paths []string
	for _, p := range keys(entries) {
		entry := entries[p]
		f, ok := base.Files[p]
		if ok && f.Blob == entry.Blob && f.Mode == entry.Mode {
			files[p] = f
			continue
		}
		found, err := s.AnalysisCache(ctx, "file", units.Hash(Algorithm, entry.Blob, base.SyntaxFor(p).Key()), &f)
		if err != nil {
			return current, err
		}
		if found {
			f.Path, f.Blob, f.Mode = p, entry.Blob, entry.Mode
			files[p] = f
		} else {
			paths = append(paths, p)
			missing = append(missing, entry.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, missing)
	if err != nil {
		return current, err
	}
	for _, p := range paths {
		entry := entries[p]
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: entry.Blob, Mode: entry.Mode, Data: blobs[entry.Blob]}, base.SyntaxFor(p))
		if err != nil {
			return current, err
		}
		files[p] = f
		if err = s.SaveAnalysisCache(ctx, "file", units.Hash(Algorithm, entry.Blob, base.SyntaxFor(p).Key()), f); err != nil {
			return current, err
		}
	}
	current, err = repostats.Apply(ctx, base, tree, files)
	if err == nil {
		err = s.SaveAnalysisCache(ctx, "repository", currentKey, current)
	}
	if err != nil {
		return current, err
	}
	return observedRepository(ctx, base, current, runs)
}

func observedRepository(ctx context.Context, base, current repostats.Repository, runs []repostats.Run) (repostats.Repository, error) {
	if len(runs) == 0 {
		return current, ctx.Err()
	}
	observed, err := repostats.Observe(ctx, base, runs)
	if err != nil {
		return current, err
	}
	return repostats.Apply(ctx, observed, current.Tree, current.Files)
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
		if r.Algorithm == Algorithm && r.Executed {
			out = append(out, repostats.Run{ID: r.ID, Output: r.Output, ExitCode: r.ExitCode, Started: true})
		}
	}
	return out
}
