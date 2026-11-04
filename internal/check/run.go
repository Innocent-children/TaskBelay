package check

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"strings"
	"time"
)

type RunInput struct {
	Depth   string `json:"depth,omitempty"`
	Refresh bool   `json:"refresh,omitempty"`
}
type analysis struct {
	Index         indexedSnapshot
	Before, After repostats.Repository
	Changes       changes.Result
	Regions       []changes.Region
	Signals       signals.Result
	Requirements  []changes.Requirement
	Edits         []ledger.EditRecord
}

func Run(ctx context.Context, store *ledger.Store, w gitstate.Workspace, actor ledger.Actor, view ledger.TaskView, observation ledger.Observation, options RunInput) (result ledger.CheckRecord, failure error) {
	if options.Depth == "" {
		options.Depth = "static"
	}
	if options.Depth != "static" && options.Depth != "tests" {
		return ledger.CheckRecord{}, fmt.Errorf("depth 只能是 static 或 tests")
	}
	if view.Task == nil {
		return ledger.CheckRecord{}, fmt.Errorf("当前没有任务")
	}
	input, err := store.CheckInput(ctx, w.ID, view.Task.ID, actor, Algorithm)
	if err != nil {
		return ledger.CheckRecord{}, err
	}
	if !options.Refresh {
		previous, err := store.LatestCheck(ctx, view.Task.ID, options.Depth, input.Version())
		if err != nil {
			return ledger.CheckRecord{}, err
		}
		if previous != nil && previous.ReusableStatic() {
			previous.Reused = true
			return *previous, nil
		}
	}
	r := ledger.CheckRecord{Depth: options.Depth, Input: input, InputVersion: input.Version(), Status: ledger.CheckComplete, StartedAt: time.Now().UTC(), Report: ledger.DeliveryReport{Analysis: map[string]json.RawMessage{}, Requirements: view.Requirements, Plans: view.Plans, Decisions: view.EffectiveDecisions, Verifications: view.Verifications, WorkspaceUpdates: view.WorkspaceUpdates, ChangesByRequirement: map[string][]string{}}}
	defer func() {
		if failure == nil || result.ID != 0 {
			return
		}
		publish, cancel := context.WithTimeout(context.WithoutCancel(ctx), PublishReserve)
		defer cancel()
		kind := "error"
		if ctx.Err() != nil {
			kind = "incomplete"
		}
		r.Status = ledger.CheckPartial
		r.Missing = append(r.Missing, ledger.CheckGap{Operation: options.Depth, Kind: kind, Reason: failure.Error()})
		r.Report.Counts = r.GapCounts()
		tree := ""
		if current, err := w.CaptureTree(publish); err == nil && current.Complete() {
			tree = current.Tree
		}
		if saved, err := store.SaveCheck(publish, w.ID, actor, tree, r); err == nil {
			result = saved
		}
	}()
	work := ctx
	cancel := func() {}
	if end, ok := ctx.Deadline(); ok {
		work, cancel = context.WithDeadline(ctx, end.Add(-PublishReserve))
	}
	defer cancel()
	data, err := analyze(work, store, w, view, observation)
	if err != nil {
		kind := "error"
		if work.Err() != nil {
			kind = "incomplete"
		}
		r.Missing = append(r.Missing, ledger.CheckGap{Operation: "static", Kind: kind, Reason: err.Error()})
		r.Status = ledger.CheckPartial
	} else {
		r.Relocations = data.Changes.Relocations
		r.Locations = resolveLocations(view.Requirements, data)
		if err := data.Index.err(); err != nil {
			return r, err
		}
		for _, gap := range data.Signals.Gaps {
			r.Missing = append(r.Missing, ledger.CheckGap{Operation: "signals", Kind: "unknown", Reason: gap})
		}
		for _, region := range data.Regions {
			r.Report.ChangesByRequirement[region.Requirement] = region.Clusters
		}
		if err = Calibrate(work, store, w, view, false); err != nil {
			return r, err
		}
		if err = rejectionDependencies(work, store, w, view, data); err != nil {
			return r, err
		}
		weights, err := fittedWeights(work, store, w.ProjectID)
		if err != nil {
			return r, err
		}
		if err = store.SaveWeights(work, w.ProjectID, weights); err != nil {
			return r, err
		}
		chosen, err := selectFeedback(work, data.Signals.Candidates, weights)
		if err != nil {
			return r, err
		}
		selected := map[string]bool{}
		for _, c := range chosen {
			selected[c.ID] = true
		}
		for _, c := range data.Signals.Candidates {
			issue := ledger.Issue{Algorithm: Algorithm, Rule: c.Kind, At: c.At, LogicalAt: c.Object, FactKey: c.Kind, Level: "hint", Status: "open", Facts: c.Facts, Advice: c.Advice, Confidence: probability(weights, c.Features), Signals: c.Features, Selected: selected[c.ID]}
			issue.ContentSignature, err = objectSignature(work, data, c)
			if err != nil {
				return r, err
			}
			if c.BlockFact == "external" {
				issue.BlockOnDelivery = true
				issue.Facts = append(issue.Facts, "可靠识别；全部来自 Agent 编辑；未找到用户原话、已确认方案或实现位置的对应项")
			}
			r.Issues = append(r.Issues, issue)
		}
		r.Issues = append(r.Issues, planIssues(w, view, data)...)
		for _, kind := range []string{"external", "unrelated", "noise", "abstraction", "forwarding", "old_name", "parameter", "duplicate", "unused", "test_equivalence", "test_budget", "assertion", "mock_only", "S1", "S2"} {
			r.Evaluated = append(r.Evaluated, ledger.CheckScope(kind, "*"))
		}
		for _, h := range data.Changes.Hunks {
			r.Report.Changes = append(r.Report.Changes, h.Path)
		}
		sections := map[string]any{"conventions": map[string]any{"tree": data.Before.Tree, "index": data.Index.Origin.id, "syntax": data.Before.Syntax, "test_converged": data.Before.Converged, "test_iterations": data.Before.TestIterations}, "changes": data.Changes, "regions": data.Regions, "signals": data.Signals, "weights": weights, "selected": chosen}
		for _, kind := range keys(sections) {
			raw, err := json.Marshal(sections[kind])
			if err != nil {
				return r, err
			}
			r.Report.Analysis[kind] = raw
			if err = store.SaveAnalysisRecord(work, view.Task.ID, input.Version(), kind, "complete", sections[kind]); err != nil {
				return r, err
			}
		}
	}
	for _, requirement := range view.Requirements {
		if requirement.Status == "revoked" {
			r.Report.Revoked = append(r.Report.Revoked, requirement)
		} else if requirement.Status != "completed" {
			r.Report.Unfinished = append(r.Report.Unfinished, requirement)
		}
	}
	if options.Depth == "tests" && data.After.Tree != "" {
		if err := runDynamic(work, store, w, actor, view, data, &r, options.Refresh); err != nil {
			kind := "error"
			if work.Err() != nil {
				kind = "incomplete"
			}
			r.Missing = append(r.Missing, ledger.CheckGap{Operation: "dynamic", Kind: kind, Reason: err.Error()})
		}
	}
	if len(r.Missing) > 0 {
		r.Status = ledger.CheckPartial
	}
	r.Report.Counts = r.GapCounts()
	for _, note := range view.EffectiveDecisions {
		if note.Target != nil && note.Target.Effect == "pause_checks" {
			for i := range r.Issues {
				r.Issues[i].BlockOnDelivery = false
				r.Issues[i].Level = "hint"
			}
		}
	}
	if view.Settings.Values["enforcement"].Value == "hint" {
		for i := range r.Issues {
			r.Issues[i].BlockOnDelivery = false
			r.Issues[i].Level = "hint"
		}
	}
	current, captureErr := w.CaptureTree(ctx)
	tree := ""
	if captureErr == nil && current.Complete() {
		tree = current.Tree
	}
	return store.SaveCheck(ctx, w.ID, actor, tree, r)
}

func analyze(ctx context.Context, s *ledger.Store, w gitstate.Workspace, view ledger.TaskView, observation ledger.Observation) (analysis, error) {
	var a analysis
	var err error
	a.Index, err = snapshots(ctx, s, w, view.Task.OriginTree, observation.Tree, runSamples(view.Verifications))
	if err != nil {
		return a, err
	}
	a.Before, a.After = a.Index.Before, a.Index.After
	a.Edits, err = s.TaskEdits(ctx, w.ID, view.Task.ID, observation.ID)
	if err != nil {
		return a, err
	}
	edits, err := provenance(ctx, s, w, view, observation, a.Before, a.After)
	if err != nil {
		return a, err
	}
	a.Changes, err = changes.Compare(ctx, changes.Input{Before: a.Before, After: a.After, Edits: edits})
	if err != nil {
		return a, err
	}
	a.Requirements = requirements(view)
	weights, err := fittedWeights(ctx, s, w.ProjectID)
	if err != nil {
		return a, err
	}
	a.Regions, err = changes.Relate(ctx, changes.GraphInput{Graph: a.Index.Graph.graph(), Changes: a.Changes, Requirements: a.Requirements, Weights: seedWeights(weights)})
	if err != nil {
		return a, err
	}
	var retained []string
	for _, note := range view.EffectiveDecisions {
		if note.Target != nil && note.Target.Effect == "retain" {
			retained = append(retained, note.Target.At)
		}
	}
	a.Index.caseNeighbourhood()
	a.Before, a.After = a.Index.Before, a.Index.After
	a.Signals, err = signals.Analyze(ctx, signals.Input{Queries: a.Index.queries(), Before: a.Before, After: a.After, Changes: a.Changes, Regions: a.Regions, Requirements: a.Requirements, Retained: retained})
	if err != nil {
		return a, err
	}
	return a, a.Index.err()
}

func requirements(view ledger.TaskView) []changes.Requirement {
	var result []changes.Requirement
	planText := ""
	if p := view.Plans.Effective; p != nil {
		planText = p.Content
		for _, f := range p.Files {
			planText += " " + f.Path
		}
		for _, e := range p.Entries {
			planText += " " + e.At
		}
		for _, d := range p.Dependencies {
			planText += " " + d.Module
		}
	}
	for _, r := range view.Requirements {
		if r.Status == "revoked" {
			continue
		}
		q := changes.Requirement{ID: r.ID, Content: r.Content, Acceptance: r.Acceptance, Plan: planText}
		if r.Citation != nil {
			q.Quote = r.Citation.Quote
		}
		for _, l := range r.ImplementedBy {
			q.Locations = append(q.Locations, l.At)
		}
		result = append(result, q)
	}
	if len(result) == 0 {
		for _, message := range view.Messages {
			result = append(result, changes.Requirement{ID: message.ID, Quote: message.Text, Plan: planText})
		}
	}
	return result
}

func resolveLocations(requirements []ledger.Requirement, a analysis) []ledger.Requirement {
	out := append([]ledger.Requirement(nil), requirements...)
	for i := range out {
		out[i].ImplementedBy = append([]ledger.Location(nil), out[i].ImplementedBy...)
		for j := range out[i].ImplementedBy {
			l := &out[i].ImplementedBy[j]
			l.AnalysisTree = a.After.Tree
			at := l.At
			if to := a.Changes.Relocations[at]; to != "" {
				l.BeforeAt, l.At = at, to
				at = to
			}
			file := strings.SplitN(at, ":", 2)[0]
			if f := a.Index.Current.fileInfo(file); f.Blob != "" {
				l.File, l.Blob, l.Pending = file, f.Blob, ""
			}
			for _, external := range a.Signals.External {
				if at == external.Kind+":"+external.Name {
					l.File = external.At
					l.Pending = ""
				}
			}
		}
	}
	return out
}
