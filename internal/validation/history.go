package validation

import (
	"context"
	"math"
	"time"
)

type Run struct {
	ID                                           string
	Command                                      Command
	Input                                        Identity
	Before, After, State, Output                 string
	Cases                                        []string
	Started, Executed, EnvironmentChanged, Rerun bool
	At                                           time.Time
}
type Decision struct {
	Kind, PreviousID, Output, Reason                string
	Block, Validation, Unstable, CommentInsensitive bool
	Agreements, Counterexamples                     int
}

func IsValidation(ctx context.Context, current Run, history []Run) (bool, error) {
	key := CommandKey(current.Command)
	trees := map[string]bool{}
	observed := false
	for _, r := range history {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if CommandKey(r.Command) != key || !r.Executed || r.Before == "" || r.After != r.Before {
			continue
		}
		if len(r.Cases) > 0 {
			return true, nil
		}
		trees[r.Before] = true
		observed = true
	}
	return observed && len(trees) > 1, nil
}

func Evaluate(ctx context.Context, current Run, history []Run) (Decision, error) {
	d := Decision{}
	is, err := IsValidation(ctx, current, history)
	if err != nil {
		return d, err
	}
	d.Validation = is
	if !is {
		return d, nil
	}
	key := CommandKey(current.Command)
	var previous *Run
	envChanged := false
	states := map[string]string{}
	for i := range history {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		r := history[i]
		if r.EnvironmentChanged {
			envChanged = true
		}
		if CommandKey(r.Command) != key {
			continue
		}
		if r.Executed {
			previous = &history[i]
			envChanged = false
		}
		if r.Executed && r.Input.Hash != "" {
			if state, ok := states[r.Input.Hash]; ok && state != r.State {
				d.Unstable = true
			}
			states[r.Input.Hash] = r.State
		}
	}
	if previous == nil {
		return d, nil
	}
	d.PreviousID, d.Output = previous.ID, previous.Output
	if current.Rerun || current.Command.Mixed || current.Command.Writes || current.Command.Unknown || previous.Before != previous.After || previous.Command.Writes || previous.State != "passed" || !current.Input.Complete || !previous.Input.Complete || d.Unstable {
		d.Reason = "已有重跑决定、输入信息不全、命令会写入或包含多项操作、结果不稳定，或上次未通过，因此本次照常运行"
		return d, nil
	}
	if current.Input.Hash == previous.Input.Hash {
		d.Kind = "repeat"
		d.Block = !envChanged && !current.EnvironmentChanged
		d.Reason = "验证输入与上次通过时相同；环境变量或外部服务变化时，用 note_add 指向上次验证写明重跑理由"
		return d, nil
	}
	var last *Run
	for i := range history {
		r := &history[i]
		if CommandKey(r.Command) != key || !r.Executed {
			continue
		}
		if last != nil && last.Input.Semantic == r.Input.Semantic && last.Input.Content != r.Input.Content && last.Input.Environment == r.Input.Environment && last.Input.Tool == r.Input.Tool {
			if last.State == r.State && known(last.State) {
				d.Agreements++
			} else {
				d.Counterexamples++
			}
		}
		last = r
	}
	d.CommentInsensitive = credible(d.Agreements, d.Counterexamples)
	if d.CommentInsensitive && current.Input.Semantic == previous.Input.Semantic && sameRuntime(current, *previous) {
		d.Kind = "comments"
		d.Reason = "从历史结果看，仅修改注释后重跑该命令很少提供新信息"
		return d, nil
	}
	for i := len(history) - 1; i >= 0; i-- {
		r := history[i]
		if r.State != "passed" || !r.Executed || r.Before != r.After || r.Input.Content != current.Input.Content || current.Input.Environment != r.Input.Environment || current.Command.Directory != r.Command.Directory || current.Command.Shell != r.Command.Shell {
			continue
		}
		if sameRuntime(current, r) && Contains(r.Command, current.Command) {
			d.Kind, d.PreviousID, d.Output, d.Reason = "contained", r.ID, r.Output, "此前已通过的验证包含本次验证范围；这里只作提示，不阻止运行"
			return d, nil
		}
		agreements, counter := implication(r, current, history)
		if credible(agreements, counter) {
			d.Kind, d.PreviousID, d.Output, d.Reason = "implication", r.ID, r.Output, "从历史通过情况看，本次验证可能不会提供新信息；这里只作提示"
			d.Agreements, d.Counterexamples = agreements, counter
			return d, nil
		}
	}
	return d, nil
}

func sameRuntime(a, b Run) bool {
	return a.Input.Environment == b.Input.Environment && a.Input.Tool == b.Input.Tool && a.Command.Directory == b.Command.Directory && a.Command.Shell == b.Command.Shell
}
func known(state string) bool { return state == "passed" || state == "failed" }
func credible(agreements, counter int) bool {
	return counter == 0 && agreements > 0 && math.Pow(1-Epsilon, float64(agreements+1)) < TailProbability
}

func implication(a, b Run, history []Run) (int, int) {
	byInput := map[string]map[string]string{}
	for _, r := range history {
		if !r.Executed || !known(r.State) || !r.Input.Complete {
			continue
		}
		keyCommand := CommandKey(r.Command)
		if keyCommand != CommandKey(a.Command) && keyCommand != CommandKey(b.Command) {
			continue
		}
		if keyCommand == CommandKey(a.Command) && !sameRuntime(r, a) || keyCommand == CommandKey(b.Command) && !sameRuntime(r, b) || r.Before != r.After {
			continue
		}
		key := r.Input.Content + "\x00" + r.Input.Environment
		row := byInput[key]
		if row == nil {
			row = map[string]string{}
			byInput[key] = row
		}
		row[CommandKey(r.Command)] = r.State
	}
	yes, no := 0, 0
	for _, key := range sorted(byInput) {
		row := byInput[key]
		if row[CommandKey(a.Command)] == "passed" {
			if row[CommandKey(b.Command)] == "passed" {
				yes++
			} else if row[CommandKey(b.Command)] == "failed" {
				no++
			}
		}
	}
	return yes, no
}
