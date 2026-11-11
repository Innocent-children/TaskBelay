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
	// ReadOnly 表示只查看内容的命令；Validation 表示已按运行事实归为验证。
	ReadOnly, Validation bool
	At                   time.Time
}

/**
 * 前置事件可确认命令已启动，只有后置事件时也可确认命令执行过。
 * 被拦截且未执行的请求不算一次运行。
 */
func (r Run) attempted() bool {
	return r.Started || r.Executed
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

func Evaluate(ctx context.Context, current Run, history []Run, truncated bool, relatedHistory func() ([]Run, error)) (Decision, error) {
	d := Decision{}
	is, err := IsValidation(ctx, current, history)
	if err != nil {
		return d, err
	}
	d.Validation = is
	if !is {
		return firstScopeAdvice(ctx, current, history)
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
		// 其间运行过别的命令时，服务、数据库等工作区之外的状态可能已经变化。
		// 只查看内容的命令和输出里有用例名的验证除外。
		if r.EnvironmentChanged || r.Executed && !r.ReadOnly && !(r.Validation && !r.Command.Mixed && len(r.Cases) > 0) && CommandKey(r.Command) != key {
			envChanged = true
		}
		if CommandKey(r.Command) != key {
			continue
		}
		if r.attempted() {
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
	if current.Rerun || current.Command.Writes || current.Command.Unknown || previous.Before != previous.After || previous.Command.Writes || previous.State != "passed" || !current.Input.Complete || !previous.Input.Complete || d.Unstable {
		d.Reason = "已有重跑决定、输入信息不全、命令会写入或包含多项操作、结果不稳定，或上次未通过，因此本次照常运行"
		return d, nil
	}
	if current.Input.Hash == previous.Input.Hash {
		d.Kind = "repeat"
		switch {
		case current.Command.Mixed:
			d.Reason = "验证输入与上次通过时相同，组合命令可能还要执行其他操作，这里只作提示"
		case truncated:
			d.Reason = "验证输入与上次通过时相同，两次验证之间的操作记录未读全，这里只作提示"
		case envChanged || current.EnvironmentChanged:
			d.Reason = "验证输入与上次通过时相同，但其间运行过其他命令或调整过环境变量，这里只作提示"
		default:
			d.Block = true
			d.Reason = "验证输入与上次通过时相同；环境变量或外部服务变化时，用 note_add 指向上次验证写明重跑理由"
		}
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
	history, err = relatedHistory()
	if err != nil {
		return d, err
	}
	seen := map[string]bool{}
	for i := len(history) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		r := history[i]
		if !takeLatestAttempt(r, seen) {
			continue
		}
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

/**
 * firstScopeAdvice 只用调用方已读取的近期记录提醒首次请求的较小范围命令。
 * 路径包含不证明本次命令属于验证；同一命令已经启动过就不再是首次请求。
 */
func firstScopeAdvice(ctx context.Context, current Run, history []Run) (Decision, error) {
	d := Decision{}
	if current.ReadOnly || current.Rerun || current.Command.Mixed || current.Command.Writes || current.Command.Unknown || !current.Input.Complete {
		return d, nil
	}
	key := CommandKey(current.Command)
	for _, r := range history {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		if r.attempted() && CommandKey(r.Command) == key {
			return d, nil
		}
	}
	seen := map[string]bool{}
	for i := len(history) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		r := history[i]
		if !takeLatestAttempt(r, seen) {
			continue
		}
		if !r.Validation || !r.Executed || r.State != "passed" || r.Before == "" || r.Before != r.After || !r.Input.Complete || r.Command.Mixed || r.Command.Writes || r.Command.Unknown {
			continue
		}
		if r.Input.Content == current.Input.Content && sameRuntime(current, r) && Contains(r.Command, current.Command) {
			d.Kind, d.PreviousID, d.Output = "contained", r.ID, r.Output
			d.Reason = "近期已通过的验证按路径参数包含本次范围；本次命令尚无执行记录，这里只作提示，不阻止运行，也不据此将其归为验证"
			return d, nil
		}
	}
	return d, nil
}

/**
 * 倒序查找提示依据时，先记住每条命令最近的尝试，再判断那次结果能否引用。
 */
func takeLatestAttempt(r Run, seen map[string]bool) bool {
	if !r.attempted() {
		return false
	}
	key := CommandKey(r.Command)
	if seen[key] {
		return false
	}
	seen[key] = true
	return true
}

func sameRuntime(a, b Run) bool {
	return a.Input.Environment == b.Input.Environment && a.Input.Tool == b.Input.Tool && a.Command.Directory == b.Command.Directory && a.Command.Shell == b.Command.Shell
}
func known(state string) bool { return state == "passed" || state == "failed" }
func credible(agreements, counter int) bool {
	return counter == 0 && agreements > 0 && math.Pow(1-Epsilon, float64(agreements+1)) < TailProbability
}

func implication(a, b Run, history []Run) (int, int) {
	type input struct{ content, environment string }
	sourceKey, targetKey := CommandKey(a.Command), CommandKey(b.Command)
	sourcePassed := map[input]bool{}
	targetResult := map[input]string{}
	for _, r := range history {
		if !r.Executed || !known(r.State) || !r.Input.Complete || r.Before != r.After {
			continue
		}
		key := input{r.Input.Content, r.Input.Environment}
		switch CommandKey(r.Command) {
		case sourceKey:
			if r.State == "passed" && sameRuntime(r, a) {
				sourcePassed[key] = true
			}
		case targetKey:
			// 同一输入上的一次失败就能构成反例，重跑成功也要保留它。
			if sameRuntime(r, b) && targetResult[key] != "failed" {
				targetResult[key] = r.State
			}
		}
	}
	yes, no := 0, 0
	for key := range sourcePassed {
		switch targetResult[key] {
		case "passed":
			yes++
		case "failed":
			no++
		}
	}
	return yes, no
}
