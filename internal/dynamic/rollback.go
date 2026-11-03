package dynamic

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"math"
	"sort"
	"strings"
)

func rollbackPatch(ctx context.Context, in Input, ids []string) ([]Patch, error) {
	set := map[string]bool{}
	for _, c := range in.Changes.Clusters {
		if contains(ids, c.ID) {
			if c.Origin != "agent" {
				return nil, fmt.Errorf("不能撤回非 Agent 变更簇")
			}
			for _, h := range c.Hunks {
				set[h] = true
			}
		}
	}
	byPath := map[string][]changes.Hunk{}
	for _, h := range in.Changes.Hunks {
		if set[h.ID] {
			byPath[h.Path] = append(byPath[h.Path], h)
		}
	}
	var out []Patch
	for _, p := range sorted(byPath) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f := in.After.Files[p]
		text := strings.Join(f.Lines, "")
		lines := append([]string(nil), f.Lines...)
		hunks := byPath[p]
		sort.Slice(hunks, func(i, j int) bool { return hunks[i].Span.AfterStart > hunks[j].Span.AfterStart })
		oldPath := p
		for _, h := range hunks {
			if h.Kind == "binary" {
				return nil, fmt.Errorf("二进制撤回未知：%s", p)
			}
			oldPath = h.BeforePath
			start, end := h.Span.AfterStart, h.Span.AfterEnd
			if start < 0 || end > len(lines) || strings.Join(lines[start:end], "") != h.After {
				return nil, fmt.Errorf("撤回片段不对应：%s", p)
			}
			replacement := split(h.Before)
			lines = append(append(append([]string(nil), lines[:start]...), replacement...), lines[end:]...)
		}
		before, existed := in.Before.Files[oldPath]
		if !existed {
			before.Mode = f.Mode
		}
		restore := strings.Join(lines, "")
		if oldPath != p {
			out = append(out, Patch{Path: p, Before: text, Remove: true}, Patch{Path: oldPath, After: restore, Mode: before.Mode})
		} else {
			out = append(out, Patch{Path: p, Before: text, After: restore, Remove: !existed && restore == "", Mode: before.Mode})
		}
	}
	return out, nil
}
func split(text string) []string {
	if text == "" {
		return nil
	}
	out := strings.SplitAfter(text, "\n")
	if out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func (r *runner) rollback(ids []string) error {
	// 全部候选一起通过时只需搜索运行和整组确认。
	first := r.test(r.search, ids)
	var removable, required []string
	if first.Passed {
		removable = ids
	} else if first.State == "unknown" {
		return unknown(first.Reason)
	} else {
		var err error
		removable, required, err = groupSearch(r.search, ids, func(group []string) Trial { return r.test(r.search, group) })
		if err != nil {
			r.result.Gaps = append(r.result.Gaps, err.Error())
			r.result.Incomplete = append(r.result.Incomplete, err.Error())
		}
	}
	r.result.Required = required
	if len(removable) == 0 {
		return r.emit()
	}
	confirmed := r.test(r.ctx, removable)
	if !confirmed.Passed {
		sort.SliceStable(removable, func(i, j int) bool { return r.input.Confidence[removable[i]] > r.input.Confidence[removable[j]] })
		var accepted []string
		for _, id := range removable {
			trial := r.test(r.ctx, append(append([]string(nil), accepted...), id))
			if trial.Passed {
				accepted = append(accepted, id)
				confirmed = trial
			} else if trial.State == "unknown" {
				break
			}
		}
		removable = accepted
	}
	if len(removable) > 0 && confirmed.Passed {
		r.result.Rollback = confirmed.IDs
		r.result.Confirmed = true
		patch, err := rollbackPatch(r.ctx, r.input, confirmed.IDs)
		if err != nil {
			return err
		}
		r.result.Patch = patch
		r.result.UnifiedPatch = unified(patch)
	}
	return r.emit()
}

// Hwang 广义二分根据尚余候选和缺陷估计选择二的幂大小的组；失败组中二分定位。
func groupSearch(ctx context.Context, ids []string, test func([]string) Trial) ([]string, []string, error) {
	pending := append([]string(nil), ids...)
	var removable, required []string
	defects := 1
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return removable, required, err
		}
		n := len(pending)
		size := 1
		if n > 2*defects-2 {
			exponent := int(math.Floor(math.Log2(float64(n-defects+1) / float64(max(1, defects)))))
			if exponent > 0 {
				size = 1 << exponent
			}
			size = min(n, size)
		}
		group := append([]string(nil), pending[:size]...)
		pending = pending[size:]
		trial := test(group)
		if trial.State == "unknown" || trial.State == "invalid" {
			return removable, required, unknown(trial.Reason)
		}
		if trial.Passed {
			removable = append(removable, group...)
			defects = max(1, defects/2)
			continue
		}
		for len(group) > 1 {
			middle := len(group) / 2
			left, right := group[:middle], group[middle:]
			trial = test(left)
			if trial.State == "unknown" || trial.State == "invalid" {
				return removable, required, unknown(trial.Reason)
			}
			if trial.Passed {
				removable = append(removable, left...)
				group = right
			} else {
				pending = append(append([]string(nil), right...), pending...)
				group = left
			}
		}
		required = append(required, group[0])
		defects = min(len(pending), max(1, defects+1))
	}
	sort.Strings(removable)
	sort.Strings(required)
	return removable, required, nil
}

func unified(patches []Patch) string {
	var b strings.Builder
	for _, p := range patches {
		before, after := split(p.Before), split(p.After)
		from, to := "a/"+p.Path, "b/"+p.Path
		if p.Remove {
			to = "/dev/null"
		}
		startBefore, startAfter := 1, 1
		if len(before) == 0 {
			startBefore = 0
		}
		if len(after) == 0 {
			startAfter = 0
		}
		fmt.Fprintf(&b, "--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", from, to, startBefore, len(before), startAfter, len(after))
		for _, line := range before {
			b.WriteString("-" + line)
			if !strings.HasSuffix(line, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
		for _, line := range after {
			b.WriteString("+" + line)
			if !strings.HasSuffix(line, "\n") {
				b.WriteString("\n\\ No newline at end of file\n")
			}
		}
	}
	return b.String()
}
