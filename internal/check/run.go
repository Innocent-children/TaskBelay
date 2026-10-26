package check

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type RunInput struct {
	Depth   string `json:"depth"`
	Refresh bool   `json:"refresh,omitempty"`
}

/**
 * 静态分析、历史 blob 回读和工作区复核均不占数据库写事务。
 * 到期时仍尝试保存未完成范围，发布和复核共用调用方剩余预算。
 */
func runStatic(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, actor ledger.Actor, view ledger.TaskView, observation ledger.Observation, options RunInput) (ledger.CheckRecord, error) {
	ctx, end := context.WithTimeout(ctx, event.StaticBudget)
	defer end()
	if options.Depth == "" {
		options.Depth = "static"
	}
	if view.Task == nil {
		return ledger.CheckRecord{}, fmt.Errorf("当前没有任务")
	}
	input, err := store.CheckInput(ctx, workspace.ID, view.Task.ID, actor, RulesVersion, CapabilitiesVersion)
	if err != nil {
		return ledger.CheckRecord{}, err
	}
	ledger.DebugRecord(ctx, "check.input", input)
	if !observation.Full || !observation.Complete || observation.Tree != input.Tree || ledger.NewCheckInput(*view.Task, observation.Tree, view.Settings, view.EffectiveDecisions, RulesVersion, CapabilitiesVersion).Version() != input.Version() {
		return ledger.CheckRecord{}, ledger.ErrCheckInputChanged
	}
	if !options.Refresh {
		cached, err := store.LatestCheck(ctx, input.TaskID, "static", input.Version())
		if err != nil {
			return ledger.CheckRecord{}, err
		}
		if cached != nil && cached.Status == ledger.CheckComplete {
			cached.Report.Verifications = view.Verifications
			cached.Report.ValidationNotes = validationNotes(view)
			ledger.DebugRecord(ctx, "check.cache", map[string]any{"depth": "static", "reused": true, "record_id": cached.ID, "input_version": input.Version()})
			return ledger.WithDelivery(*cached, view.Task.State == ledger.TaskPending && view.Settings.Values["enforcement"].Value != "hint"), nil
		}
	}
	ledger.DebugRecord(ctx, "check.cache", map[string]any{"depth": "static", "reused": false, "refresh": options.Refresh})
	started := time.Now().UTC()
	deadline, _ := ctx.Deadline()
	analysisCtx, cancel := context.WithDeadline(ctx, deadline.Add(-event.MCPBudget))
	defer cancel()
	result, analyzeErr := analyze(analysisCtx, store, workspace, view, observation, input)
	ledger.DebugRecord(ctx, "check.analysis.end", map[string]any{"record": result, "returned": analyzeErr == nil})
	result.Input, result.InputVersion, result.Depth, result.StartedAt = input, input.Version(), "static", started
	if analyzeErr != nil {
		result.Status = ledger.CheckFailed
		if errors.Is(analyzeErr, context.DeadlineExceeded) || errors.Is(analyzeErr, context.Canceled) {
			result.Status = ledger.CheckPartial
		}
		result.Missing = append(result.Missing, ledger.CheckGap{Operation: "static", Reason: analyzeErr.Error()})
		// 未完成操作不提供确定性拦截证据，也不把旧问题标为解决。
		result.Evaluated = nil
		for i := range result.Issues {
			result.Issues[i].Level = "hint"
			result.Issues[i].BlockOnDelivery = false
		}
	}
	publishDeadline := minTime(deadline, time.Now().Add(event.MCPBudget))
	publish, endPublish := context.WithDeadline(context.WithoutCancel(ctx), publishDeadline)
	defer endPublish()
	// 工作区复核不能用完发布预算，否则连未完成报告也无法写入。
	capture, endCapture := context.WithDeadline(publish, publishDeadline.Add(-event.MCPBudget/2))
	current, captureErr := workspace.Capture(capture)
	endCapture()
	ledger.DebugRecord(ctx, "check.workspace_recheck", map[string]any{"snapshot": current, "returned": captureErr == nil})
	currentTree := current.Tree
	if captureErr != nil || !current.Complete() {
		currentTree = ""
		result.Missing = append(result.Missing, ledger.CheckGap{Operation: "publish", Reason: "当前工作区复核未完成"})
	}
	return store.SaveCheck(publish, workspace.ID, actor, currentTree, result)
}

func analyze(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, view ledger.TaskView, observation ledger.Observation, input ledger.CheckInput) (ledger.CheckRecord, error) {
	result := ledger.CheckRecord{Report: report(view, nil)}
	for _, note := range view.EffectiveDecisions {
		if note.Target != nil && note.Target.Effect == "pause_checks" {
			result.Status = ledger.CheckPartial
			result.Report.NotRun = append(result.Report.NotRun, "用户决定暂停检查："+note.ID)
			return result, nil
		}
	}
	ledger.DebugRecord(ctx, "check.origin_tree.start", map[string]any{"tree": input.OriginTree})
	before, err := AnalyzeTree(ctx, workspace, input.OriginTree)
	if err != nil {
		return result, err
	}
	ledger.DebugRecord(ctx, "check.current_tree.start", map[string]any{"tree": input.Tree})
	after, err := AnalyzeTree(ctx, workspace, input.Tree)
	if err != nil {
		return result, err
	}
	edits, err := store.Changes(ctx, workspace.ID, view.Task.OriginObservationID)
	if err != nil {
		return result, err
	}
	bounded := edits[:0]
	for _, edit := range edits {
		if edit.AfterObservation <= observation.ID {
			bounded = append(bounded, edit)
		}
	}
	edits = bounded
	load := snapshotLoader(ctx, workspace)
	relocations, err := verifiedRelocations(ctx, load, edits, observation, after)
	if err != nil {
		return result, err
	}
	changes, err := codescan.Compare(ctx, before, after, relocations)
	if err != nil {
		return result, err
	}
	view, association, err := resolveLocations(ctx, store, workspace, view, observation, before, after)
	if err != nil {
		return result, err
	}
	// 撤销需求继续显示在报告中，但不能为当前改动提供归属。
	var positions []codescan.Position
	for _, requirement := range view.Requirements {
		if requirement.Status == "revoked" {
			continue
		}
		for _, location := range requirement.ImplementedBy {
			if location.Pending == "" {
				positions = append(positions, codescan.Position{Requirement: requirement.ID, At: location.At, Purpose: location.Purpose, Change: location.Change})
			}
		}
	}
	association, err = codescan.Associate(ctx, before, after, changes, positions)
	if err != nil {
		return result, err
	}
	ledger.DebugRecord(ctx, "check.association", map[string]any{"changes": changes, "association": association, "edits": edits})
	var blobs []string
	for _, edit := range edits {
		if edit.Before.Blob != "" && edit.Before.Mode != "160000" {
			blobs = append(blobs, edit.Before.Blob)
		}
		if edit.After.Blob != "" && edit.After.Mode != "160000" {
			blobs = append(blobs, edit.After.Blob)
		}
	}
	contents, err := workspace.ReadBlobs(ctx, blobs)
	if err != nil {
		return result, err
	}
	sources, err := buildProvenance(ctx, before, after, edits, contents)
	if err != nil {
		return result, err
	}
	result, err = staticCheck(ctx, input, view, before, after, changes, association, sources)
	result.Locations = view.Requirements
	if err != nil {
		return result, err
	}
	records, err := store.TaskEdits(ctx, workspace.ID, input.TaskID, observation.ID)
	if err != nil {
		return result, err
	}
	if err := checkPlan(ctx, workspace, view, before, after, edits, records, sources, contents, &result); err != nil {
		return result, err
	}
	result = ledger.WithDelivery(result, view.Task.State == ledger.TaskPending && view.Settings.Values["enforcement"].Value != "hint")
	previous, err := store.PreviousCheck(ctx, input.TaskID, "static")
	if err != nil {
		return result, err
	}
	if previous != nil && previous.Input.Tree != "" && previous.Input.Tree != input.Tree {
		prior, err := AnalyzeTree(ctx, workspace, previous.Input.Tree)
		if err != nil {
			return result, err
		}
		migration, err := codescan.Compare(ctx, prior, after, relocations)
		if err != nil {
			return result, err
		}
		result.Relocations = codescan.Migrations(migration)
		for from, to := range codescan.TestMigrations(prior, after, migration) {
			result.Relocations[from] = to
		}
		for _, change := range migration {
			if change.After == nil {
				for _, issue := range previous.Issues {
					at := change.Before.Location()
					if issue.At == at || strings.HasPrefix(issue.At, at+".") {
						result.Evaluated = append(result.Evaluated, ledger.CheckScope(issue.Rule, issue.At))
					}
				}
			}
		}
		for _, issue := range previous.Issues {
			at := issue.At
			if moved := result.Relocations[at]; moved != "" {
				at = moved
			}
			_, file, _ := splitPosition(at)
			if file == "" {
				continue
			}
			oldFile, currentFile := before.Files[file], after.Files[file]
			if currentFile == nil || !currentFile.CodeComplete || oldFile != nil && !oldFile.CodeComplete {
				continue
			}
			stillChanged := false
			for _, change := range changes {
				if change.Location() == at || strings.HasPrefix(at, change.Location()+".") {
					stillChanged = true
					break
				}
			}
			if !stillChanged {
				result.Evaluated = append(result.Evaluated, ledger.CheckScope(issue.Rule, at))
			}
		}
	}
	sort.Strings(result.Report.Changes)
	return result, nil
}
