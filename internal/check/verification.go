package check

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/units"
	"github.com/Innocent-children/taskbelay/internal/validation"
	"time"
)

func command(record ledger.VerificationRecord) validation.Command {
	// 分号连接的多条命令只返回最后一条的退出状态，不能据此确认前面各条都通过。
	c := validation.Command{Environment: record.EnvironmentID, Tool: record.Scope.Tool, Unknown: record.Command.Complex || record.Command.Sequential}
	count := 0
	for _, s := range record.Command.Segments {
		c.Unknown = c.Unknown || !s.Known || s.Background
		c.Writes = c.Writes || s.SideEffect
		if len(s.Args) == 0 {
			continue
		}
		args := append(append([]string(nil), s.Prefix...), s.Args...)
		if s.Input != "" {
			args = append(args, "\x00<"+s.Input)
		}
		if s.Piped {
			args = append(args, "\x00|")
		}
		if count == 0 {
			c.Program = args[0]
			c.Args = args[1:]
			c.Directory, c.Shell = s.Dir, string(s.Shell)
		} else {
			// 用 && 串联的命令整体作为一个比较单位，各段的目录和参数都进入命令标识。
			c.Args = append(append(c.Args, "\x00"+s.Dir), args...)
		}
		count++
	}
	c.Mixed = count != 1
	if c.Program == "" {
		c.Unknown = true
	}
	return c
}
func CommandKey(record ledger.VerificationRecord) string {
	return validation.CommandKey(command(record))
}

func validationRun(r ledger.VerificationRecord) validation.Run {
	return validation.Run{ID: r.ID, Command: command(r), Input: validation.Identity{Hash: r.Scope.Hash, Content: r.Scope.Content, Semantic: r.Scope.Semantic, Tool: r.Scope.Tool, Environment: r.EnvironmentID, Complete: r.Scope.Complete, Included: r.Scope.Included, Excluded: r.Scope.Excluded}, Before: r.Tree, After: r.AfterTree, State: r.State, Output: r.Output, Cases: r.Scope.Cases, Started: !r.StartedAt.IsZero(), Executed: r.Executed, EnvironmentChanged: r.Scope.EnvironmentChanged, Rerun: r.RerunNoteID != "", At: r.StartedAt, ReadOnly: r.Command.ReadOnly(), Validation: r.Scope.Validation}
}

func validationRuns(history []ledger.VerificationRecord, exclude string) []validation.Run {
	var runs []validation.Run
	for _, r := range history {
		if r.Algorithm == Algorithm && r.ID != exclude {
			runs = append(runs, validationRun(r))
		}
	}
	return runs
}

/**
 * Verification 在命令开始前给出这次运行的验证输入，并判断它是否重复。
 * 重复判断只使用同一命令的运行记录和期间的近期操作，跨命令分析按需读取历史输入和结果。
 */
func Verification(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord, record *ledger.VerificationRecord) error {
	history, truncated, err := s.VerificationHistory(ctx, w.ID, record.CommandKey)
	if err != nil {
		return err
	}
	summary, err := inputSummary(ctx, s, w, task.OriginTree, record.Tree)
	if err != nil {
		return err
	}
	identity, err := validation.Input(ctx, validation.InputData{Summary: summary, Set: task.OriginTree + "/" + record.Tree, Command: command(*record)})
	if err != nil {
		return err
	}
	record.Scope.Hash, record.Scope.Content, record.Scope.Semantic = identity.Hash, identity.Content, identity.Semantic
	record.Scope.Included, record.Scope.Excluded = identity.Included, identity.Excluded
	record.Scope.Set = identity.Set
	record.Scope.Complete = identity.Complete
	d, err := validation.Evaluate(ctx, validationRun(*record), validationRuns(history, record.ID), truncated, func() ([]validation.Run, error) {
		records, err := s.VerificationOutcomes(ctx, w.ID, Algorithm)
		return validationRuns(records, record.ID), err
	})
	if err != nil {
		return err
	}
	record.Scope.Validation, record.Scope.Unstable = d.Validation, d.Unstable
	record.Scope.Reason = d.Reason
	if d.Kind != "" {
		if d.Output == "" && d.PreviousID != "" {
			// 历史输入不带输出，只加载选中的那次运行。
			if d.Output, err = s.VerificationOutput(ctx, d.PreviousID); err != nil {
				return err
			}
		}
		level := "hint"
		if d.Block {
			level = "block"
		}
		advice := "有新的环境或外部状态变化时，用 note_add 指向上次验证记录并写明重跑理由"
		if d.Kind == "restored" {
			advice = "查看相同输入时的输出；有新的疑问或外部数据已变化时可以重跑"
		}
		record.Issue = &ledger.Issue{Algorithm: Algorithm, Selected: true, Rule: "validation_" + d.Kind, LogicalAt: record.CommandKey, At: record.CommandKey, FactKey: record.Scope.Hash, Status: "open", Level: level, Facts: []string{d.Reason, "上次记录：" + d.PreviousID, "上次输出：" + d.Output}, Advice: advice}
		if d.Kind != "repeat" {
			record.Scope.Suggestion = d.Reason
		}
	}
	return nil
}

// InputReserve 是命令开始前为保存记录和输出决定留出的时间，摘要计算不能占用。
const InputReserve = 350 * time.Millisecond

/**
 * inputSummary 返回某个工作区状态的验证输入摘要，每个状态只确定一次。
 * 内容没有变化时直接取已保存的结果。新状态先按已发布的索引去掉未被引用的文档；
 * 索引未就绪或来不及算完时，改用该状态自身的标识：内容完全相同仍能识别为重复，
 * 只是不再把“只改了无关文档”视为相同输入。
 */
func inputSummary(ctx context.Context, s *ledger.Store, w gitstate.Workspace, origin, tree string) (validation.Summary, error) {
	var summary validation.Summary
	if tree == "" {
		return summary, fmt.Errorf("没有可用的工作区状态")
	}
	key := units.Hash(Algorithm, "input", w.ProjectID, origin, tree)
	found, err := s.AnalysisCache(ctx, "validation-input", key, &summary)
	if err != nil || found {
		return summary, err
	}
	limited := ctx
	if deadline, ok := ctx.Deadline(); ok {
		var cancel context.CancelFunc
		limited, cancel = context.WithDeadline(ctx, deadline.Add(-InputReserve))
		defer cancel()
	}
	if r, err := snapshots(publishedOnly(limited), s, w, origin, tree, nil); err == nil {
		summary = r.Current.meta.Validation
	} else if ctx.Err() != nil {
		return summary, ctx.Err()
	} else {
		label := sha256.Sum256([]byte("tree\x00" + tree))
		summary = validation.Summary{Content: label, Semantic: label}
	}
	if err := s.SaveAnalysisCache(ctx, "validation-input", key, summary); err != nil {
		return summary, err
	}
	// 并发的另一次调用可能先保存了结果，以已保存的为准。
	_, err = s.AnalysisCache(ctx, "validation-input", key, &summary)
	return summary, err
}

/**
 * CompleteVerification 在命令结束后补全记录。执行后的状态和输出先保存；
 * 用例名只在已发布的索引里查找，是否属于验证按同一命令的运行记录判断。
 */
func CompleteVerification(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord, record ledger.VerificationRecord, after, output string) error {
	scope := record.Scope
	if record.Tree != after {
		scope.Complete = false
		scope.Reason = "命令执行前后文件内容发生变化，不能用这次结果拦截后续的重复验证"
	}
	if err := s.UpdateVerificationInput(ctx, record.ID, scope, after, output); err != nil {
		return err
	}
	if record.Command.ReadOnly() {
		return nil
	}
	if task.ID != "" && after != "" && output != "" {
		x, err := publishedIndex(ctx, s, w, task.OriginTree)
		// 起点索引由 MCP 后台或交付检查建立；尚未就绪时仍按反复运行的记录判断。
		if err != nil && !errors.Is(err, ErrIndexPending) {
			return err
		}
		if err == nil {
			found := map[string]bool{}
			for _, name := range x.namesIn(output) {
				if _, checked := found[name]; !checked {
					found[name] = units.HasWord(output, name)
				}
			}
			var present []string
			for _, name := range keys(found) {
				if found[name] {
					present = append(present, name)
				}
			}
			if x.err != nil {
				return x.err
			}
			// 输出只是照抄源码时，其中的名字不说明运行过测试。
			if scope.Cases, err = executed(x, output, present); err != nil {
				return err
			}
			if err := x.readResult(); err != nil {
				return err
			}
		}
	}
	scope.Validation = scope.Validation || len(scope.Cases) > 0 && record.Tree == after && after != ""
	if !scope.Validation {
		history, _, err := s.VerificationHistory(ctx, w.ID, record.CommandKey)
		if err != nil {
			return err
		}
		current := record
		current.Scope = scope
		current.AfterTree = after
		current.Output = output
		runs := append(validationRuns(history, record.ID), validationRun(current))
		scope.Validation, err = validation.IsValidation(ctx, validationRun(current), runs)
		if err != nil {
			return err
		}
	}
	return s.UpdateVerificationInput(ctx, record.ID, scope, after, output)
}

func CommandResultReason(err error) string {
	return fmt.Sprintf("未能完整核对验证输入：%v", err)
}
