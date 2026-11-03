package check

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"strings"
)

type lineOrigin struct {
	original int
	agent    bool
	call     string
}

func provenance(ctx context.Context, s *ledger.Store, w gitstate.Workspace, v ledger.TaskView, o ledger.Observation, before, after repostats.Repository) ([]changes.Edit, error) {
	history, err := s.TaskChanges(ctx, w.ID, v.Task.ID, o.ID)
	if err != nil {
		return nil, err
	}
	byPath := map[string][]ledger.FileChange{}
	var ids []string
	for _, h := range history {
		byPath[h.Path] = append(byPath[h.Path], h)
		if h.Before.Blob != "" {
			ids = append(ids, h.Before.Blob)
		}
		if h.After.Blob != "" {
			ids = append(ids, h.After.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return nil, err
	}
	var result []changes.Edit
	for _, p := range keys(byPath) {
		base := before.Files[p]
		current := append([]string(nil), base.Lines...)
		tags := make([]lineOrigin, len(current))
		for i := range tags {
			tags[i].original = i
		}
		deleted := map[int]lineOrigin{}
		rows := byPath[p]
		for start := 0; start < len(rows); {
			end := start + 1
			for end < len(rows) && rows[end].AfterObservation == rows[start].AfterObservation {
				end++
			}
			row := rows[start]
			old := splitLines(string(blobs[row.Before.Blob]))
			next := splitLines(string(blobs[row.After.Blob]))
			if strings.Join(old, "") != strings.Join(current, "") {
				tags = make([]lineOrigin, len(old))
				for i := range tags {
					tags[i].original = -1
				}
			}
			spans, err := changes.Diff(ctx, old, next)
			if err != nil {
				return nil, err
			}
			offset := 0
			for _, span := range spans {
				cause := lineOrigin{original: -1}
				for _, h := range rows[start:end] {
					if h.Kind != "agent_edit" {
						continue
					}
					for _, f := range h.Fragments {
						bs := f.BeforeStart
						if f.BeforeLines > 0 {
							bs--
						}
						as := f.AfterStart
						if f.AfterLines > 0 {
							as--
						}
						if bs == span.BeforeStart && bs+f.BeforeLines == span.BeforeEnd && as == span.AfterStart && as+f.AfterLines == span.AfterEnd {
							cause.agent, cause.call = true, h.ToolCallID
						}
					}
				}
				a, b := span.BeforeStart+offset, span.BeforeEnd+offset
				for _, oldTag := range tags[a:b] {
					if oldTag.original >= 0 {
						deleted[oldTag.original] = cause
					}
				}
				insert := make([]lineOrigin, span.AfterEnd-span.AfterStart)
				for i := range insert {
					insert[i] = cause
				}
				tags = append(append(append([]lineOrigin(nil), tags[:a]...), insert...), tags[b:]...)
				offset += (span.AfterEnd - span.AfterStart) - (span.BeforeEnd - span.BeforeStart)
			}
			current = next
			start = end
		}
		if strings.Join(current, "") != strings.Join(after.Files[p].Lines, "") {
			continue
		}
		spans, err := changes.Diff(ctx, base.Lines, current)
		if err != nil {
			return nil, err
		}
		for _, span := range spans {
			agent, has := true, false
			calls := map[string]bool{}
			for i := span.BeforeStart; i < span.BeforeEnd; i++ {
				t := deleted[i]
				agent = agent && t.agent
				has = true
				calls[t.call] = true
			}
			for _, t := range tags[span.AfterStart:span.AfterEnd] {
				agent = agent && t.agent
				has = true
				calls[t.call] = true
			}
			if !agent || !has {
				continue
			}
			call := strings.Join(keys(calls), ",")
			result = append(result, changes.Edit{Path: p, BeforePath: p, Before: strings.Join(base.Lines[span.BeforeStart:span.BeforeEnd], ""), After: strings.Join(current[span.AfterStart:span.AfterEnd], ""), Call: call, AgentOnly: true, Exact: true, Span: span})
		}
	}
	return result, nil
}
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
