package check

import (
	"context"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"github.com/Innocent-children/taskbelay/internal/validation"
)

func command(record ledger.VerificationRecord) validation.Command {
	c := validation.Command{Environment: record.EnvironmentID, Tool: record.Scope.Tool, Unknown: record.Command.Complex, Mixed: len(record.Command.Segments) != 1}
	for i, s := range record.Command.Segments {
		if len(s.Args) == 0 {
			continue
		}
		args := append(append([]string(nil), s.Prefix...), s.Args...)
		if i == 0 {
			c.Program = args[0]
			c.Args = args[1:]
			c.Directory, c.Shell = s.Dir, string(s.Shell)
		}
		c.Unknown = c.Unknown || !s.Known || s.Background
		c.Writes = c.Writes || s.SideEffect
	}
	if c.Program == "" {
		c.Unknown = true
	}
	return c
}
func CommandKey(record ledger.VerificationRecord) string {
	return validation.CommandKey(command(record))
}
func KnownValidation(ctx context.Context, s *ledger.Store, workspace string, record ledger.VerificationRecord) (bool, error) {
	previous, err := s.PreviousVerification(ctx, workspace, record.Actor, CommandKey(record))
	return previous != nil && previous.Algorithm == Algorithm && (previous.Scope.Validation || len(previous.Scope.Cases) > 0), err
}

func validationRun(r ledger.VerificationRecord) validation.Run {
	return validation.Run{ID: r.ID, Command: command(r), Input: validation.Identity{Hash: r.Scope.Hash, Content: r.Scope.Content, Semantic: r.Scope.Semantic, Tool: r.Scope.Tool, Environment: r.EnvironmentID, Complete: r.Scope.Complete, Included: r.Scope.Included, Excluded: r.Scope.Excluded}, Before: r.Tree, After: r.AfterTree, State: r.State, Output: r.Output, Cases: r.Scope.Cases, Started: !r.StartedAt.IsZero(), Executed: r.Executed, EnvironmentChanged: r.Scope.EnvironmentChanged, Rerun: r.RerunNoteID != "", At: r.StartedAt}
}

func Verification(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord, record *ledger.VerificationRecord) error {
	history, err := s.Verifications(ctx, w.ID, "")
	if err != nil {
		return err
	}
	var runs []validation.Run
	for _, r := range history {
		if r.Algorithm == Algorithm {
			runs = append(runs, validationRun(r))
		}
	}
	summary, origin, err := validationSummary(publishedOnly(ctx), s, w, task.OriginTree, record.Tree, runSamples(history))
	if err != nil {
		return err
	}
	identity, err := validation.Input(ctx, validation.InputData{Summary: summary, Set: origin + "/" + record.Tree, Command: command(*record)})
	if err != nil {
		return err
	}
	record.Scope.Hash, record.Scope.Content, record.Scope.Semantic = identity.Hash, identity.Content, identity.Semantic
	record.Scope.Included, record.Scope.Excluded = identity.Included, identity.Excluded
	record.Scope.Set = identity.Set
	record.Scope.Complete = identity.Complete
	d, err := validation.Evaluate(ctx, validationRun(*record), runs)
	if err != nil {
		return err
	}
	record.Scope.Validation, record.Scope.Unstable = d.Validation, d.Unstable
	record.Scope.Reason = d.Reason
	if d.Kind != "" {
		level := "hint"
		if d.Block {
			level = "block"
		}
		record.Issue = &ledger.Issue{Algorithm: Algorithm, Selected: true, Rule: "validation_" + d.Kind, LogicalAt: record.CommandKey, At: record.CommandKey, FactKey: record.Scope.Hash, Status: "open", Level: level, Facts: []string{d.Reason, "上次记录：" + d.PreviousID, "上次输出：" + d.Output}, Advice: "有新的环境或外部状态变化时，用 note_add 指向上次验证记录并写明重跑理由"}
		if d.Kind != "repeat" {
			record.Scope.Suggestion = d.Reason
		}
	}
	return nil
}

/**
 * validationSummary 返回某个工作区状态的验证输入摘要。命令结束后已经为当时的状态
 * 算过一次；内容没有再变时，下一条命令开始前直接取用，不重新叠加整个任务的改动。
 */
func validationSummary(ctx context.Context, s *ledger.Store, w gitstate.Workspace, origin, tree string, runs []repostats.Run) (validation.Summary, string, error) {
	var summary validation.Summary
	base, err := openIndex(ctx, s, w, origin)
	if err == nil {
		base, err = observedIndex(base, runs)
	}
	if err != nil {
		return summary, "", err
	}
	key := units.Hash(Algorithm, base.id, tree)
	found, err := s.AnalysisCache(ctx, "validation-summary", key, &summary)
	if err != nil || found {
		return summary, base.id, err
	}
	r, err := snapshots(ctx, s, w, origin, tree, runs)
	if err != nil {
		return summary, "", err
	}
	return r.Current.meta.Validation, r.Origin.id, s.SaveAnalysisCache(ctx, "validation-summary", units.Hash(Algorithm, r.Origin.id, tree), r.Current.meta.Validation)
}

func CompleteVerification(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord, record ledger.VerificationRecord, after, output string) error {
	scope := record.Scope
	if record.Tree != after {
		scope.Complete = false
		scope.Reason = "命令执行前后文件内容发生变化，不能用这次结果拦截后续的重复验证"
	}
	// 先保存执行后的状态和输出；后面的用例识别失败或超时不影响这条运行记录。
	if err := s.UpdateVerificationInput(ctx, record.ID, scope, after, output); err != nil {
		return err
	}
	if record.Command.ReadOnly() {
		return nil
	}
	if task.ID != "" && after != "" {
		history, err := s.Verifications(ctx, w.ID, "")
		if err != nil {
			return err
		}
		samples := runSamples(history)
		if record.Executed && record.ExitCode != nil {
			present := false
			for _, sample := range samples {
				present = present || sample.ID == record.ID
			}
			if !present {
				samples = append(samples, repostats.Run{ID: record.ID, Output: output, ExitCode: record.ExitCode, Started: true})
			}
		}
		repo, err := snapshots(publishedOnly(ctx), s, w, task.OriginTree, after, samples)
		// 起点索引由 MCP 后台或交付检查建立；尚未就绪时仍按反复运行的记录判断。
		if err != nil && !errors.Is(err, ErrIndexPending) {
			return err
		}
		if err == nil {
			names := repo.Current.namesIn(output)
			matcherNames := map[string]bool{}
			for _, name := range names {
				if units.HasWord(output, name) {
					matcherNames[name] = true
				}
			}
			scope.Cases = keys(matcherNames)
			if err := repo.err(); err != nil {
				return err
			}
			if err := s.SaveAnalysisCache(ctx, "validation-summary", units.Hash(Algorithm, repo.Origin.id, after), repo.Current.meta.Validation); err != nil {
				return err
			}
		}
	}
	scope.Validation = scope.Validation || len(scope.Cases) > 0 && record.Tree == after && after != ""
	if !scope.Validation {
		history, err := s.Verifications(ctx, w.ID, "")
		if err != nil {
			return err
		}
		var runs []validation.Run
		for _, r := range history {
			if r.Algorithm == Algorithm && r.ID != record.ID {
				runs = append(runs, validationRun(r))
			}
		}
		current := record
		current.Scope = scope
		current.AfterTree = after
		current.Output = output
		runs = append(runs, validationRun(current))
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
