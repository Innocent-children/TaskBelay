package validation

import (
	"context"
	"fmt"
	"iter"
	"math"
	"slices"
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
	StartedAt, EndedAt   time.Time
}

/**
 * 前置事件可确认命令已启动，只有后置事件时也可确认命令执行过。
 * 被拦截且未执行的请求不算一次运行。
 */
func (r Run) attempted() bool {
	return r.Started || r.Executed
}

func (r Run) reusableSingleRun() bool {
	return r.Executed && r.State == "passed" && r.Before != "" && r.Before == r.After &&
		r.Input.Complete && !r.Command.Mixed && !r.Command.Writes && !r.Command.Unknown
}

func (r Run) reportsAll(names []string) bool {
	// 后置事件保存的用例名已排序。
	for _, name := range names {
		if _, found := slices.BinarySearch(r.Cases, name); !found {
			return false
		}
	}
	return true
}

func (r Run) mayChangeExternalState() bool {
	reportedCases := r.Validation && !r.Command.Mixed && len(r.Cases) > 0
	return r.attempted() && (r.EnvironmentChanged || !r.ReadOnly && !reportedCases)
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
		return recentScopeAdvice(ctx, current, history, d, nil)
	}
	key := CommandKey(current.Command)
	previousIndex := -1
	var matchingID string
	possibleExternalChange := false
	states := map[string]string{}
	for i := range history {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		r := history[i]
		if CommandKey(r.Command) != key {
			if r.mayChangeExternalState() {
				possibleExternalChange = true
			}
			continue
		}
		if r.attempted() {
			previousIndex = i
			possibleExternalChange = false
			// 同命令中途失败或结果无法核对时，不能越过这次运行引用更早的成功。
			if !r.reusableSingleRun() || r.EnvironmentChanged {
				matchingID = ""
			} else if r.Input.Hash == current.Input.Hash {
				matchingID = r.ID
			}
		}
		if r.Executed && r.Input.Hash != "" {
			if state, ok := states[r.Input.Hash]; ok && state != r.State {
				d.Unstable = true
			}
			states[r.Input.Hash] = r.State
		}
	}
	if previousIndex < 0 {
		return d, nil
	}
	previous := &history[previousIndex]
	d.PreviousID, d.Output = previous.ID, previous.Output
	if current.Rerun || current.Command.Writes || current.Command.Unknown || previous.Before != previous.After || previous.Command.Writes || !current.Input.Complete || !previous.Input.Complete || d.Unstable {
		d.Reason = "已有重跑决定、输入信息不全、命令会写入或包含多项操作、结果不稳定，或上次未通过，因此本次照常运行"
		return d, nil
	}
	if previous.State != "passed" {
		d.Reason = "上次验证未通过或尚无结果，本次照常运行"
		if previous.State == "failed" && previous.Executed && !previous.EndedAt.IsZero() && previous.Before != "" && len(previous.Cases) > 0 &&
			previous.Input.Content != current.Input.Content && states[current.Input.Hash] != "failed" && !current.EnvironmentChanged {
			return recentScopeAdvice(ctx, current, history[previousIndex+1:], d, previous)
		}
		return d, nil
	}
	if current.Input.Hash == previous.Input.Hash {
		d.Kind = "repeat"
		switch {
		case current.Command.Mixed:
			d.Reason = "验证输入与上次通过时相同，组合命令可能还要执行其他操作，这里只作提示"
		case truncated:
			d.Reason = "验证输入与上次通过时相同，两次验证之间的操作记录未读全，这里只作提示"
		case possibleExternalChange || current.EnvironmentChanged:
			d.Reason = "验证输入与上次通过时相同，但其他命令可能已改变外部状态，或本次命令调整了环境变量，本次照常运行"
		default:
			d.Block = true
			d.Reason = "验证输入与上次通过时相同；环境变量或外部服务变化时，用 note_add 指向上次验证写明重跑理由"
		}
		return d, nil
	}
	var last *Run
	for i := range history {
		r := &history[i]
		if CommandKey(r.Command) != key || !r.Executed || !sameRuntime(current, *r) {
			continue
		}
		if last != nil && last.Input.Semantic == r.Input.Semantic && last.Input.Content != r.Input.Content {
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
		d.Reason = fmt.Sprintf("在当前工具和环境下，过去 %d 次只改注释或空白后，重跑这条命令的结果都与改动前一致。本次重跑可能没有新信息", d.Agreements)
		return d, nil
	}
	history, err = relatedHistory()
	if err != nil {
		return d, err
	}
	seen := map[string]bool{}
	for r := range runsSinceExternalChange(history) {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		if matchingID != "" && r.ID == matchingID && !current.Command.Mixed && !current.EnvironmentChanged {
			d.Kind, d.PreviousID, d.Output = "restored", r.ID, r.Output
			d.Reason = "当前验证输入已恢复到这条命令此前通过时的状态。如果仍要核对同一问题，可以查看那次输出"
			return d, nil
		}
		if !takeLatestAttempt(r, seen) {
			continue
		}
		if r.State != "passed" || !r.Executed || r.Before != r.After || r.Input.Content != current.Input.Content || current.Input.Environment != r.Input.Environment || current.Command.Directory != r.Command.Directory || current.Command.Shell != r.Command.Shell {
			continue
		}
		if sameRuntime(current, r) && Contains(r.Command, current.Command) {
			d.Kind, d.PreviousID, d.Output, d.Reason = "contained", r.ID, r.Output, "此前较大路径范围的验证已通过。请从那次输出核对是否已检查本次关心的目标，本次仍可运行"
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
 * 首次请求只按路径核对。失败后的重跑只看失败之后的记录，
 * 较大范围的输出还须报告上次识别出的全部用例名。
 */
func recentScopeAdvice(ctx context.Context, current Run, history []Run, d Decision, failed *Run) (Decision, error) {
	if current.ReadOnly || current.Rerun || current.Command.Mixed || current.Command.Writes || current.Command.Unknown || !current.Input.Complete {
		return d, nil
	}
	if failed == nil {
		key := CommandKey(current.Command)
		for _, r := range history {
			if err := ctx.Err(); err != nil {
				return d, err
			}
			if r.attempted() && CommandKey(r.Command) == key {
				return d, nil
			}
		}
	}
	seen := map[string]bool{}
	for r := range runsSinceExternalChange(history) {
		if err := ctx.Err(); err != nil {
			return d, err
		}
		if !takeLatestAttempt(r, seen) {
			continue
		}
		if !r.Validation || !r.reusableSingleRun() {
			continue
		}
		if r.Input.Content != current.Input.Content || !sameRuntime(current, r) || !Contains(r.Command, current.Command) {
			continue
		}
		if failed != nil && (r.StartedAt.Before(failed.EndedAt) || !r.reportsAll(failed.Cases)) {
			continue
		}
		d.Kind, d.PreviousID, d.Output = "contained", r.ID, r.Output
		if failed != nil {
			d.Reason = "这条验证上次在改动前失败。改动后，较大范围的验证已通过，输出中包含上次识别出的全部用例名。请核对原先失败的检查，本次仍可运行"
		} else {
			d.Reason = "近期较大路径范围的验证已通过。请从那次输出核对是否已检查本次关心的目标。本次命令尚无执行记录，照常运行"
		}
		return d, nil
	}
	return d, nil
}

/**
 * 倒序查找可引用的结果。可能改变外部状态的命令也可以有自己的验证结果，
 * 但更早的成功无法说明这条命令启动后的状态。
 */
func runsSinceExternalChange(history []Run) iter.Seq[Run] {
	return func(yield func(Run) bool) {
		for _, r := range slices.Backward(history) {
			if !yield(r) || r.mayChangeExternalState() {
				return
			}
		}
	}
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
