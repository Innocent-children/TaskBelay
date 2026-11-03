package check

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"sort"
	"strings"
)

func ReuseAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, actor ledger.Actor, text string) (string, error) {
	v, err := s.TaskView(ctx, w.ID, "", actor)
	if err != nil || v.Task == nil {
		return "", err
	}
	r, err := repository(ctx, s, w, v.Task.OriginTree, v.Task.OriginTree, runSamples(v.Verifications))
	if err != nil {
		return "", err
	}
	regions, err := changes.Relate(ctx, changes.GraphInput{Before: r, After: r, Requirements: []changes.Requirement{{ID: "message", Quote: text}}})
	if err != nil || len(regions) == 0 {
		return "", err
	}
	type choice struct {
		at    string
		score float64
	}
	var choices []choice
	for _, p := range keys(r.Files) {
		for _, u := range r.Files[p].Units {
			score := regions[0].Scores["u:"+repostats.UnitKey(p, u)] * float64(1+r.Frequency[u.Name])
			if score > 0 && u.Name != "" {
				choices = append(choices, choice{p + ":" + u.Name, score})
			}
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
		return "", nil
	}
	return "可先核对已有实现：" + strings.Join(out, "、"), nil
}

func EditAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, contents map[string][]byte) (string, error) {
	task, _, err := s.CommandContext(ctx, w.ID)
	if err != nil || task.ID == "" {
		return "", err
	}
	var base repostats.Repository
	found, err := s.AnalysisCache(ctx, "repository", units.Hash(Algorithm, task.OriginTree), &base)
	if err != nil || !found {
		return "", err
	}
	files := map[string]units.File{}
	for p, f := range base.Files {
		files[p] = f
	}
	for _, p := range keys(contents) {
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: units.Hash(string(contents[p])), Data: contents[p]}, base.SyntaxFor(p))
		if err != nil {
			return "", err
		}
		files[p] = f
	}
	after, err := repostats.Apply(ctx, base, "preview", files)
	if err != nil {
		return "", err
	}
	delta, err := changes.Compare(ctx, changes.Input{Before: base, After: after})
	if err != nil {
		return "", err
	}
	result, err := signals.Analyze(ctx, signals.Input{Before: base, After: after, Changes: delta})
	if err != nil {
		return "", err
	}
	var advice []string
	for _, c := range result.Candidates {
		if c.Kind == "external" || c.Kind == "duplicate" {
			advice = append(advice, c.At+"："+c.Advice)
			if len(advice) >= CategoryLimit {
				break
			}
		}
	}
	return strings.Join(advice, "\n"), nil
}
