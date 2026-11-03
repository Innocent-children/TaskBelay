package check

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/dynamic"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
)

func runDynamic(ctx context.Context, s *ledger.Store, w gitstate.Workspace, actor ledger.Actor, view ledger.TaskView, a analysis, record *ledger.CheckRecord, refresh bool) error {
	var result dynamic.Result
	owner := rand.Text()
	claimed, err := s.ClaimDynamic(ctx, view.Task.ID, record.InputVersion, owner, time.Now().Add(dynamic.Budget+PublishReserve), refresh)
	if err != nil {
		return err
	}
	if !claimed {
		for {
			claimed, err = s.ClaimDynamic(ctx, view.Task.ID, record.InputVersion, owner, time.Now().Add(dynamic.Budget+PublishReserve), refresh)
			if err != nil {
				return err
			}
			if claimed {
				_ = s.FinishDynamic(ctx, view.Task.ID, owner, "retry")
				return runDynamic(ctx, s, w, actor, view, a, record, true)
			}
			found, err := s.AnalysisRecord(ctx, view.Task.ID, record.InputVersion, "dynamic", &result)
			if err != nil {
				return err
			}
			if found && result.State != "running" {
				break
			}
			timer := time.NewTimer(dynamic.PollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				result.Gaps = append(result.Gaps, "等待副本运行未完成："+ctx.Err().Error())
				result.Incomplete = append(result.Incomplete, "等待副本运行未完成："+ctx.Err().Error())
				result.State = "partial"
			case <-timer.C:
				continue
			}
			break
		}
	} else {
		defer func() {
			finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), PublishReserve)
			defer cancel()
			_ = s.FinishDynamic(finish, view.Task.ID, owner, "complete")
		}()
		input, err := dynamicInput(ctx, s, w, view, a, record)
		if err != nil {
			publish, cancel := context.WithTimeout(context.WithoutCancel(ctx), PublishReserve)
			defer cancel()
			_ = s.SaveAnalysisRecord(publish, view.Task.ID, record.InputVersion, "dynamic", "partial", dynamic.Result{State: "partial", Gaps: []string{err.Error()}, Errors: []string{err.Error()}})
			return err
		}
		var previous dynamic.Result
		if found, err := s.AnalysisRecord(ctx, view.Task.ID, record.InputVersion, "dynamic", &previous); err != nil {
			return err
		} else if found {
			input.Resume = &previous
		}
		input.Pause = func(ctx context.Context) (bool, error) { return s.ValidationRunning(ctx, w.ID) }
		result, err = dynamic.Run(ctx, input, func(progress dynamic.Result) error {
			publish, cancel := context.WithTimeout(context.WithoutCancel(ctx), PublishReserve)
			defer cancel()
			if err := s.SaveAnalysisRecord(publish, view.Task.ID, record.InputVersion, "dynamic", progress.State, progress); err != nil {
				return err
			}
			latest, err := s.CheckInput(publish, w.ID, view.Task.ID, actor, Algorithm)
			if err != nil {
				return err
			}
			if latest.Version() != record.InputVersion {
				return fmt.Errorf("任务输入已变化，停止旧输入的副本运行")
			}
			return nil
		})
		if err != nil {
			if !stringIn(result.Gaps, err.Error()) {
				result.Gaps = append(result.Gaps, err.Error())
				result.Errors = append(result.Errors, err.Error())
			}
			result.State = "partial"
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	record.Report.Analysis["dynamic"] = raw
	record.Report.Dynamic = &ledger.DynamicReport{State: result.State, Runs: len(result.Runs), Confirmed: result.Confirmed, Rollback: result.Rollback, Kept: result.Kept, Gaps: result.Gaps, Details: raw}
	for _, gap := range result.Gaps {
		kind := "unknown"
		if stringIn(result.Errors, gap) {
			kind = "error"
		}
		if stringIn(result.Incomplete, gap) {
			kind = "incomplete"
		}
		record.Missing = append(record.Missing, ledger.CheckGap{Operation: "dynamic", Kind: kind, Reason: gap})
	}
	if result.State == "running" {
		record.Missing = append(record.Missing, ledger.CheckGap{Operation: "dynamic", Kind: "incomplete", Reason: "副本运行尚未完成"})
	}
	for _, kind := range []string{"rollback", "test_reduction", "test_gap"} {
		record.Evaluated = append(record.Evaluated, ledger.CheckScope(kind, "*"))
	}
	if result.Confirmed {
		for _, c := range a.Signals.Candidates {
			if !stringIn(result.Rollback, c.Cluster) {
				continue
			}
			issue := ledger.Issue{Algorithm: Algorithm, Rule: "rollback", At: c.At, LogicalAt: c.Object, FactKey: "confirmed", Level: "hint", Status: "open", Selected: true, Facts: append(append([]string(nil), c.Facts...), "撤回集合已整组确认："+strings.Join(result.Rollback, ",")), Advice: "可使用报告中的撤回补丁；副本中的全部既有及主要验收依据仍通过", Signals: c.Features}
			issue.ContentSignature, err = objectSignature(ctx, a, c)
			if err != nil {
				return err
			}
			unrelated := len(a.Regions) > 0
			for _, region := range a.Regions {
				unrelated = unrelated && region.Known && !stringIn(region.Clusters, c.Cluster)
			}
			anchored := false
			for _, req := range view.Requirements {
				for _, at := range req.ImplementedBy {
					anchored = anchored || at.At == c.At || at.At == c.Object
				}
			}
			issue.BlockOnDelivery = unrelated && !anchored && c.AgentOnly && !c.Defensive
			record.Issues = append(record.Issues, issue)
		}
	}
	if result.ReductionConfirmed && len(result.Removed) > 0 {
		record.Issues = append(record.Issues, ledger.Issue{Algorithm: Algorithm, Rule: "test_reduction", LogicalAt: units.Hash(result.Removed...), At: strings.Join(result.Removed, ","), FactKey: "confirmed", Level: "hint", Status: "open", Selected: true, Facts: []string{"发现矩阵约简后，保留用例及验收依据已整组确认通过"}, Advice: "可删除报告中已确认的测试集合；这只覆盖本次已抽样的变异"})
	}
	for _, gap := range result.TestGaps {
		record.Issues = append(record.Issues, ledger.Issue{Algorithm: Algorithm, Rule: "test_gap", LogicalAt: gap, At: gap, FactKey: "survived", Level: "hint", Status: "open", Selected: true, Facts: []string{"有效变异未被已运行的用例发现"}, Advice: "核对该变异所代表的行为是否应被验收覆盖"})
	}
	var confirmed []string
	for _, probe := range result.Probes {
		if probe.State == "confirmed" {
			confirmed = append(confirmed, probe.Added...)
		}
	}
	if len(confirmed) > 0 {
		updated, err := signals.Analyze(ctx, signals.Input{Before: a.Before, After: a.After, Changes: a.Changes, Regions: a.Regions, Requirements: a.Requirements, Confirmed: confirmed})
		if err != nil {
			return err
		}
		for _, c := range updated.Candidates {
			if c.Kind != "external" || c.BlockFact != "external" {
				continue
			}
			found := false
			for i := range record.Issues {
				if record.Issues[i].Rule == c.Kind && record.Issues[i].LogicalAt == c.Object {
					record.Issues[i].BlockOnDelivery = true
					record.Issues[i].Facts = c.Facts
					found = true
				}
			}
			if !found {
				record.Issues = append(record.Issues, ledger.Issue{Algorithm: Algorithm, Rule: c.Kind, At: c.At, LogicalAt: c.Object, FactKey: c.Kind, Level: "hint", Status: "open", Selected: true, BlockOnDelivery: true, Facts: c.Facts, Advice: c.Advice})
			}
		}
	}
	return calibrateTrials(ctx, s, w, view, result)
}

func dynamicInput(ctx context.Context, s *ledger.Store, w gitstate.Workspace, view ledger.TaskView, a analysis, record *ledger.CheckRecord) (dynamic.Input, error) {
	in := dynamic.Input{Workspace: w, Tree: a.After.Tree, Before: a.Before, After: a.After, Changes: a.Changes, Signals: a.Signals, Regions: a.Regions, Runs: runSamples(view.Verifications), Confidence: map[string]float64{}}
	in.Parallel, _ = strconv.Atoi(view.Settings.Values["analysis.parallel"].Value)
	for _, c := range a.Signals.Candidates {
		for _, i := range record.Issues {
			if i.LogicalAt == c.Object {
				in.Confidence[c.Cluster] = max(in.Confidence[c.Cluster], i.Confidence)
			}
		}
	}
	// 以相关簇第一次可核对的 Agent 编辑为界，不能靠事后补填验收制造主要依据。
	first := map[string]time.Time{}
	for _, edit := range a.Edits {
		observed, err := s.Observation(ctx, edit.BeforeObservation)
		if err != nil {
			return in, err
		}
		for _, region := range a.Regions {
			for _, h := range a.Changes.Hunks {
				if !stringIn(strings.Split(h.Call, ","), edit.ToolCallID) {
					continue
				}
				related := false
				for _, c := range a.Changes.Clusters {
					if stringIn(region.Clusters, c.ID) && stringIn(c.Hunks, h.ID) {
						related = true
					}
				}
				if related && (first[region.Requirement].IsZero() || observed.At.Before(first[region.Requirement])) {
					first[region.Requirement] = observed.At
				}
			}
		}
	}
	existing := map[string]bool{}
	for _, name := range repostats.CaseNames(a.Before) {
		existing[name] = true
	}
	seen := map[string]bool{}
	for n := len(view.Verifications) - 1; n >= 0; n-- {
		r := view.Verifications[n]
		c := command(r)
		if r.Algorithm != Algorithm || r.TaskID != view.Task.ID || r.State != "passed" || !r.Executed || !r.Scope.Complete || !r.Scope.Validation || r.Tree != r.AfterTree || c.Unknown || c.Mixed || c.Writes || c.Tool == "" || seen[r.CommandKey] {
			continue
		}
		seen[r.CommandKey] = true
		in.Commands = append(in.Commands, dynamic.Command{ID: r.CommandKey, Program: c.Program, Args: c.Args, Directory: c.Directory, Shell: c.Shell, Environment: c.Environment, Tool: c.Tool, Output: r.Output, Cases: r.Scope.Cases, Trusted: true})
		var old []string
		for _, name := range r.Scope.Cases {
			if existing[name] {
				old = append(old, name)
			}
		}
		if len(old) > 0 {
			in.Basis = append(in.Basis, dynamic.Basis{ID: "existing:" + r.ID, Command: r.CommandKey, Cases: old, Existing: true, Source: r.ID})
		}
		if len(old) == 0 {
			for _, past := range view.Verifications {
				if past.Algorithm == Algorithm && past.CommandKey == r.CommandKey && past.Executed && past.State == "passed" && past.Tree == a.Before.Tree && past.AfterTree == a.Before.Tree {
					in.Basis = append(in.Basis, dynamic.Basis{ID: "existing:" + past.ID, Command: r.CommandKey, Existing: true, Source: past.ID})
					break
				}
			}
		}
		for _, req := range view.Requirements {
			if req.Status == "revoked" {
				continue
			}
			text := req.Acceptance
			quote := ""
			if req.Citation != nil {
				quote = req.Citation.Quote
			}
			var names []string
			explicit := false
			for _, name := range r.Scope.Cases {
				if strings.Contains(text, name) || strings.Contains(quote, name) {
					names = append(names, name)
					explicit = explicit || strings.Contains(quote, name)
				}
			}
			commandMatch := strings.Contains(text, strings.Join(append([]string{c.Program}, c.Args...), " "))
			if len(names) == 0 && !commandMatch {
				continue
			}
			primary := explicit
			for _, past := range view.History {
				if past.ID == req.ID && past.Acceptance == req.Acceptance && !first[req.ID].IsZero() && past.At.Before(first[req.ID]) {
					primary = true
				}
			}
			if p := view.Plans.Effective; p != nil && p.Confirmation != nil && !first[req.ID].IsZero() && p.Confirmation.AuthorizedAt.Before(first[req.ID]) && strings.Contains(p.Content, text) && text != "" {
				primary = true
			}
			in.Basis = append(in.Basis, dynamic.Basis{ID: fmt.Sprintf("%s:%s", req.ID, r.ID), Command: r.CommandKey, Requirement: req.ID, Source: req.ID, Cases: names, Primary: primary})
		}
	}
	return in, nil
}

func stringIn(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func calibrateTrials(ctx context.Context, s *ledger.Store, w gitstate.Workspace, view ledger.TaskView, r dynamic.Result) error {
	for _, note := range view.Notes {
		if note.Target == nil || note.Target.Kind != "issue" || !strings.HasPrefix(strings.TrimSpace(note.Text), "拒绝") {
			continue
		}
		for _, issue := range view.Issues {
			if issue.ID != note.Target.ID || issue.Algorithm != Algorithm {
				continue
			}
			for _, trial := range r.Trials {
				if trial.State != "failed" || !stringIn(trial.IDs, issue.LogicalAt) {
					continue
				}
				label := ledger.CalibrationLabel{ID: units.Hash(issue.ID, note.ID, "acceptance_failure"), ProjectID: w.ProjectID, IssueID: issue.ID, Kind: "acceptance_failure", Reason: "撤回该建议指向的簇后既有或主要验收失败", Signals: issue.Signals}
				if err := s.SaveCalibration(ctx, label); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}
