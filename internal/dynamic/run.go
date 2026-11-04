package dynamic

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Run(ctx context.Context, in Input, publish func(Result) error) (result Result, failure error) {
	ctx, cancel := context.WithTimeout(ctx, Budget)
	defer cancel()
	end, _ := ctx.Deadline()
	search, stop := context.WithDeadline(ctx, time.Now().Add(time.Duration(float64(time.Until(end))*(1-ConfirmReserve))))
	defer stop()
	r := runner{input: in, result: Result{State: "running", Operators: map[string]Operator{}, Clusters: in.Changes.Clusters}, publish: publish, ctx: ctx, search: search, model: in.After.Results, samples: append([]repostats.Run(nil), in.Runs...)}
	defer func() {
		if failure != nil {
			r.result.Gaps = append(r.result.Gaps, failure.Error())
			if ctx.Err() != nil || search.Err() != nil {
				r.result.Incomplete = append(r.result.Incomplete, failure.Error())
			} else {
				r.result.Errors = append(r.result.Errors, failure.Error())
			}
		}
		if failure != nil || len(r.result.Gaps) > 0 {
			r.result.State = "partial"
		} else {
			r.result.State = "complete"
		}
		result = r.result
		if publish != nil {
			if err := publish(result); failure == nil {
				failure = err
			}
		}
	}()
	if len(in.Commands) == 0 {
		r.result.Gaps = append(r.result.Gaps, "没有本任务中已成功运行且输入完整的可信命令")
		return r.result, nil
	}
	root, err := os.MkdirTemp("", "taskbelay-analysis-")
	if err != nil {
		return r.result, err
	}
	r.root = root
	r.base = filepath.Join(root, "current")
	defer os.RemoveAll(root)
	if err = in.Workspace.ExportTree(ctx, in.Tree, r.base); err != nil {
		return r.result, err
	}
	baseline, err := r.commands(search, "current", r.base, nil, in.Commands)
	if err != nil {
		return r.result, err
	}
	var training []repostats.Run
	for _, c := range in.Commands {
		zero := 0
		training = append(training, repostats.Run{ID: c.ID, Output: c.Output, ExitCode: &zero, Started: true})
	}
	for _, run := range baseline {
		training = append(training, repostats.Run{ID: run.Command, Output: run.Output, ExitCode: run.ExitCode, Started: run.Started})
	}
	r.model, err = repostats.ResultsFor(in.After, training)
	if err != nil {
		return r.result, err
	}
	for _, b := range in.Basis {
		if basisState(b, baseline) == "passed" {
			r.result.Basis = append(r.result.Basis, b)
		} else {
			r.result.Gaps = append(r.result.Gaps, "验收依据在当前副本未通过或结果未知："+b.ID)
		}
	}
	if err = r.probes(search, baseline); err != nil {
		return r.result, err
	}
	files := map[string]bool{}
	for _, h := range in.Changes.Hunks {
		if !in.After.Files[h.Path].Binary && in.After.Files[h.Path].Mode != "120000" {
			files[h.Path] = true
		}
	}
	for _, p := range sorted(files) {
		for _, c := range in.Commands {
			key := units.Hash(in.Tree, p, in.After.Files[p].Blob, c.ID, c.Tool, c.Environment)
			cached := false
			if in.Resume != nil {
				for _, scope := range in.Resume.Scopes {
					if scope.Key == key && scope.State == "confirmed" {
						r.result.Scopes = append(r.result.Scopes, scope)
						cached = true
						break
					}
				}
			}
			if cached {
				continue
			}
			original := strings.Join(in.After.Files[p].Lines, "")
			runs, err := r.commands(search, "sentinel:"+p, r.base, []Patch{{Path: p, Before: original, After: "\x00\x00\xff\xfe", Mode: in.After.Files[p].Mode}}, []Command{c})
			if err != nil {
				return r.result, err
			}
			scope := Scope{Command: c.ID, Path: p, Key: key, State: "unknown"}
			for _, b := range baseline {
				if b.Command == c.ID {
					scope.Baseline = b
				}
			}
			if len(runs) > 0 {
				scope.Sentinel = runs[0]
				if scope.Baseline.State == "passed" && runs[0].State == "failed" {
					scope.State = "confirmed"
					scope.Reason = "改变该副本文件使已通过命令失败"
				} else {
					scope.Reason = "无法确认命令读取该文件"
				}
			}
			r.result.Scopes = append(r.result.Scopes, scope)
			if scope.State == "unknown" {
				r.result.Gaps = append(r.result.Gaps, scope.Reason+"："+p+" / "+c.ID)
			}
		}
	}
	if len(r.result.Basis) == 0 {
		r.result.Gaps = append(r.result.Gaps, "没有在当前副本通过的验收依据")
		return r.result, nil
	}
	eligible := r.eligible()
	if len(eligible) > 0 {
		if err = r.rollback(eligible); err != nil {
			return r.result, err
		}
	}
	if err = r.mutations(); err != nil {
		return r.result, err
	}
	return r.result, nil
}

func basisState(b Basis, runs []RunResult) string {
	for _, r := range runs {
		if r.Command != b.Command {
			continue
		}
		if r.State != "passed" && r.State != "failed" {
			return "unknown"
		}
		if len(b.Cases) == 0 {
			return r.State
		}
		state := "passed"
		for _, name := range b.Cases {
			found := false
			for _, c := range r.Cases {
				if c.Name == name {
					found = true
					if c.State == "failed" {
						return "failed"
					}
					if c.State != "passed" {
						state = "unknown"
					}
				}
			}
			if !found {
				state = "unknown"
			}
		}
		return state
	}
	return "unknown"
}

func (r *runner) test(ctx context.Context, ids []string) Trial {
	for i, id := range ids {
		for r.merged[id] != "" {
			id = r.merged[id]
		}
		ids[i] = id
	}
	ids = unique(ids)
	closure, err := changes.Closure(ctx, r.result.Clusters, ids)
	if err != nil {
		return Trial{IDs: ids, State: "unknown", Reason: err.Error()}
	}
	patches, err := rollbackPatch(ctx, r.input, closure)
	if err != nil {
		return Trial{IDs: closure, State: "invalid", Reason: err.Error()}
	}
	runs, err := r.commands(ctx, "rollback:"+units.Hash(closure...), r.base, patches, r.input.Commands)
	trial := Trial{IDs: closure, Runs: runs, State: "passed", Passed: true}
	if err != nil {
		trial.State, trial.Passed, trial.Reason = "unknown", false, err.Error()
	} else {
		for _, b := range r.result.Basis {
			state := basisState(b, runs)
			if !b.Existing && !b.Primary {
				if state != "passed" {
					trial.Supplemental = append(trial.Supplemental, b.ID)
				}
				continue
			}
			if state != "passed" {
				trial.Passed = false
				if state == "failed" {
					trial.State = "failed"
				} else if trial.State != "failed" {
					trial.State = "unknown"
				}
			}
		}
	}
	r.result.Trials = append(r.result.Trials, trial)
	if trial.State == "failed" {
		merged, mergeErr := r.mergeFailure(ctx, closure, runs)
		if mergeErr != nil {
			return Trial{IDs: closure, State: "unknown", Reason: mergeErr.Error()}
		}
		if merged {
			return r.test(ctx, ids)
		}
	}
	return trial
}

func (r *runner) supported() map[string]bool {
	own := map[string]bool{}
	for _, b := range r.result.Basis {
		if b.Primary && b.Requirement != "" {
			own[b.Requirement] = true
		}
	}
	allowed := map[string]bool{}
	for _, c := range r.result.Clusters {
		ok := c.Origin == "agent"
		for _, region := range r.input.Regions {
			if !region.Known {
				ok = false
			}
			if contains(region.Clusters, c.ID) && !own[region.Requirement] {
				ok = false
			}
		}
		for _, h := range r.input.Changes.Hunks {
			if !contains(c.Hunks, h.ID) {
				continue
			}
			sensitive := false
			for _, scope := range r.result.Scopes {
				if scope.Path == h.Path && scope.State == "confirmed" {
					for _, b := range r.result.Basis {
						sensitive = sensitive || b.Command == scope.Command && (b.Primary || b.Existing)
					}
				}
			}
			ok = ok && sensitive
		}
		allowed[c.ID] = ok
	}
	return allowed
}

func (r *runner) eligible() []string {
	scores := map[string]float64{}
	defensive := map[string]bool{}
	for _, id := range r.input.Signals.Defensive {
		defensive[id] = true
	}
	for _, c := range r.input.Signals.Candidates {
		if c.AgentOnly && c.Cluster != "" {
			scores[c.Cluster] = max(scores[c.Cluster], c.Benefit)
		}
		defensive[c.Cluster] = defensive[c.Cluster] || c.Defensive
	}
	allowed := r.supported()
	var out []string
	for _, id := range sorted(scores) {
		closure, err := changes.Closure(r.ctx, r.result.Clusters, []string{id})
		if err != nil {
			continue
		}
		ok := true
		for _, dep := range closure {
			ok = ok && allowed[dep] && !defensive[dep]
		}
		if ok {
			out = append(out, id)
		} else {
			r.result.Gaps = append(r.result.Gaps, "撤回候选缺少自身验收、文件作用范围或包含不可撤回内容："+id)
		}
	}
	return out
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (r *runner) emit() error {
	if r.publish != nil {
		return r.publish(r.result)
	}
	return nil
}
func unknown(reason string) error { return fmt.Errorf("动态计算未完成：%s", reason) }
