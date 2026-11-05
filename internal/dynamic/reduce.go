package dynamic

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"math/bits"
	"sort"
	"strings"
)

func cover(ctx context.Context, rows []Discovery, cases []signals.Case) ([]string, []string, error) {
	ids := map[string]bool{}
	for _, c := range cases {
		ids[c.ID] = true
	}
	names := sorted(ids)
	indices := map[string]int{}
	for i, id := range names {
		indices[id] = i
	}
	var matrix [][]uint64
	for _, row := range rows {
		if row.State != "valid" || len(row.Cases) == 0 {
			continue
		}
		bits := make([]uint64, (len(names)+63)/64)
		nonempty := false
		for _, id := range row.Cases {
			if i, ok := indices[id]; ok {
				bits[i/64] |= 1 << uint(i%64)
				nonempty = true
			}
		}
		if nonempty {
			matrix = append(matrix, bits)
		}
	}
	var essential [][]uint64
	for i, a := range matrix {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		redundant := false
		for j, b := range matrix {
			if i == j {
				continue
			}
			subset, equal := true, true
			for k := range a {
				subset = subset && b[k]&^a[k] == 0
				equal = equal && b[k] == a[k]
			}
			if subset && (!equal || j < i) {
				redundant = true
				break
			}
		}
		if !redundant {
			essential = append(essential, a)
		}
	}
	keep := map[string]bool{}
	for _, c := range cases {
		if c.Retained || !c.Added || !c.AgentOnly {
			keep[c.ID] = true
		}
	}
	covered := make([]bool, len(essential))
	mark := func(id string) {
		i := indices[id]
		for row, b := range essential {
			covered[row] = covered[row] || b[i/64]&(1<<uint(i%64)) != 0
		}
	}
	for _, id := range sorted(keep) {
		mark(id)
	}
	for {
		best, bestGain := "", 0
		for _, id := range names {
			if keep[id] {
				continue
			}
			i := indices[id]
			gain := 0
			for row, b := range essential {
				if !covered[row] {
					gain += bits.OnesCount64(b[i/64] & (1 << uint(i%64)))
				}
			}
			if gain > bestGain {
				best, bestGain = id, gain
			}
		}
		if bestGain == 0 {
			break
		}
		keep[best] = true
		mark(best)
	}
	var removed []string
	for _, c := range cases {
		if !keep[c.ID] {
			removed = append(removed, c.ID)
		}
	}
	sort.Strings(removed)
	return sorted(keep), removed, nil
}

func (r *runner) reduce() error {
	// 未观察到单条结果的用例始终保留；整条命令不能代替其独立结果。
	cases := append([]signals.Case(nil), r.input.Signals.Cases...)
	for i := range cases {
		observed := false
		for _, run := range r.result.Runs {
			if run.Phase != "current" {
				continue
			}
			for _, c := range run.Cases {
				observed = observed || c.Name == cases[i].Name && c.State == "passed"
			}
		}
		cases[i].Retained = cases[i].Retained || !observed
	}
	confirmed := false
	defer func() {
		if !confirmed {
			r.result.Kept = nil
			for _, c := range cases {
				r.result.Kept = append(r.result.Kept, c.ID)
			}
			sort.Strings(r.result.Kept)
		}
	}()
	valid := false
	for _, row := range r.result.Matrix {
		valid = valid || row.State == "valid"
	}
	if !valid {
		for _, c := range r.input.Signals.Cases {
			r.result.Kept = append(r.result.Kept, c.ID)
		}
		if len(r.input.Signals.Cases) > 0 {
			r.result.Gaps = append(r.result.Gaps, "尚无有效的用例与变异对应结果，无法判断哪些测试可以精简")
		}
		return nil
	}
	keep, removed, err := cover(r.ctx, r.result.Matrix, cases)
	if err != nil {
		return err
	}
	r.result.Kept = keep
	if len(removed) == 0 {
		confirmed = true
		return nil
	}
	byPath := map[string][]signals.Case{}
	for _, c := range r.input.Signals.Cases {
		if contains(removed, c.ID) {
			byPath[c.Path] = append(byPath[c.Path], c)
		}
	}
	var patch []Patch
	for _, p := range sorted(byPath) {
		f := r.input.After.Files[p]
		text := strings.Join(f.Lines, "")
		after := text
		cases := byPath[p]
		sort.Slice(cases, func(i, j int) bool { return cases[i].Start > cases[j].Start })
		last := len(text)
		for _, c := range cases {
			if c.Start < 0 || c.End > last || c.Start > c.End {
				r.result.Gaps = append(r.result.Gaps, "用例范围重叠，无法独立删除："+c.ID)
				return nil
			}
			after = after[:c.Start] + after[c.End:]
			last = c.Start
		}
		patch = append(patch, Patch{Path: p, Before: text, After: after, Mode: f.Mode})
	}
	runs, err := r.commands(r.ctx, "reduction-confirm", r.base, patch, r.input.Commands)
	if err != nil {
		return err
	}
	passed := true
	for _, b := range r.result.Basis {
		if b.Primary || b.Existing {
			passed = passed && basisState(b, runs) == "passed"
		}
	}
	for _, c := range r.input.Signals.Cases {
		if !contains(keep, c.ID) {
			continue
		}
		found := false
		for _, run := range runs {
			for _, state := range run.Cases {
				if state.Name == c.Name && state.State == "passed" {
					found = true
				}
			}
		}
		passed = passed && found
	}
	if passed {
		confirmed = true
		r.result.ReductionConfirmed = true
		r.result.Removed = removed
		r.result.ReductionPatch = patch
	} else {
		r.result.Kept = nil
		for _, c := range r.input.Signals.Cases {
			r.result.Kept = append(r.result.Kept, c.ID)
		}
		r.result.Gaps = append(r.result.Gaps, "删除候选后保留用例没有全部通过，撤销测试精简结论")
	}
	return r.emit()
}
