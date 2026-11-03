package check

import (
	"context"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/validation"
	"strings"
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
	r, err := repository(ctx, s, w, task.OriginTree, record.Tree, runSamples(history))
	if err != nil {
		return err
	}
	identity, err := validation.Input(ctx, validation.InputData{Files: r.Files, Roles: r.Roles, Command: command(*record)})
	if err != nil {
		return err
	}
	record.Scope.Hash, record.Scope.Content, record.Scope.Semantic = identity.Hash, identity.Content, identity.Semantic
	record.Scope.Included, record.Scope.Excluded = identity.Included, identity.Excluded
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

func CompleteVerification(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord, record ledger.VerificationRecord, after, output string) error {
	scope := record.Scope
	if task.ID != "" && after != "" {
		repo, err := repository(ctx, s, w, task.OriginTree, after, nil)
		if err != nil {
			return err
		}
		names := repostats.CaseNames(repo)
		matcherNames := map[string]bool{}
		for _, name := range names {
			if strings.Contains(output, name) {
				matcherNames[name] = true
			}
		}
		scope.Cases = keys(matcherNames)
	}
	if record.Tree != after {
		scope.Complete = false
		scope.Reason = "命令前后内容变化，结果不可用于重复拦截"
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

func CommandResultReason(err error) string { return fmt.Sprintf("验证输入未完成：%v", err) }
