package check

import (
	"context"
	"errors"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"sort"
	"strings"
)

func ReuseAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, actor ledger.Actor, text string) (string, error) {
	v, err := s.TaskView(ctx, w.ID, "", actor)
	if err != nil || v.Task == nil {
		return "", err
	}
	// 用户消息只使用已发布的索引；索引未就绪时不提示，把时间留给接续摘要。
	ctx = publishedOnly(ctx)
	x, err := openIndex(ctx, s, w, v.Task.OriginTree)
	if err == nil {
		x, err = observedIndex(x, runSamples(v.Verifications))
	}
	if errors.Is(err, ErrIndexPending) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	regions, err := changes.Relate(ctx, changes.GraphInput{Graph: x.graph(), Requirements: []changes.Requirement{{ID: "message", Quote: text}}})
	if err != nil || len(regions) == 0 {
		return "", err
	}
	type choice struct {
		at    string
		score float64
	}
	var choices []choice
	for key, score := range regions[0].Scores {
		if !strings.HasPrefix(key, "u:") {
			continue
		}
		u := x.unit(key[2:])
		score *= float64(1 + x.stats(u.Unit.Name).DF)
		if score > 0 && u.Unit.Name != "" {
			choices = append(choices, choice{u.Path + ":" + u.Unit.Name, score})
		}
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].score != choices[j].score {
			return choices[i].score > choices[j].score
		}
		return choices[i].at < choices[j].at
	})
	var out []string
	for _, c := range choices[:min(ReuseHints, len(choices))] {
		out = append(out, c.at)
	}
	if len(out) == 0 {
		return "", x.err
	}
	return "可先核对已有实现：" + strings.Join(out, "、"), x.err
}

func EditAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, contents map[string][]byte) (string, error) {
	view, err := s.EditAnalysisContext(ctx, w.ID)
	if err != nil || view.Task == nil {
		return "", err
	}
	ctx = publishedOnly(ctx)
	base, err := taskIndex(ctx, s, w, *view.Task)
	if err != nil {
		return "", err
	}
	paths := keys(contents)
	before := base.view(paths)
	files := map[string]units.File{}
	for _, p := range paths {
		var syn units.Syntax
		base.get("syntax", path.Ext(p), &syn)
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: units.Hash(string(contents[p])), Mode: "100644", Data: contents[p]}, syn)
		if err != nil {
			return "", err
		}
		files[p] = f
	}
	after, err := base.apply(paths, files, "preview")
	if err != nil {
		return "", err
	}
	delta, err := changes.Compare(ctx, changes.Input{Before: before, After: after})
	if err != nil {
		return "", err
	}
	snapshot := indexedSnapshot{Origin: base, Current: base, Before: before, After: after, Paths: paths}
	result, err := analyzeSignals(ctx, view, signals.Input{EditPreview: true, Queries: snapshot.queries(), Before: snapshot.Before, After: snapshot.After, Changes: delta, Requirements: requirements(view)})
	if err != nil {
		return "", err
	}
	var advice []string
	for _, c := range result.Candidates {
		if c.Kind == "external" || c.Kind == "duplicate" {
			text := c.At + "："
			if c.Kind == "external" {
				text += strings.Join(c.Facts, "；") + "；"
			}
			advice = append(advice, text+c.Advice)
			if len(advice) >= CategoryLimit {
				break
			}
		}
	}
	return LimitFeedback(strings.Join(advice, "\n")), snapshot.err()
}
