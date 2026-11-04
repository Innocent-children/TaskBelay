package check

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"strings"
)

func objectSignature(ctx context.Context, a analysis, c signals.Candidate) ([]uint64, error) {
	var words []string
	for _, cluster := range a.Changes.Clusters {
		if cluster.ID != c.Cluster {
			continue
		}
		set := map[string]bool{}
		for _, id := range cluster.Hunks {
			set[id] = true
		}
		for _, h := range a.Changes.Hunks {
			if set[h.ID] {
				var tokens []units.Token
				for _, token := range a.After.Files[h.Path].Tokens {
					if token.Line > h.Span.AfterStart && token.Line <= h.Span.AfterEnd {
						tokens = append(tokens, token)
					}
				}
				words = append(words, units.Sequence(tokens, true)...)
			}
		}
	}
	if len(words) == 0 {
		p := strings.SplitN(c.At, ":", 2)[0]
		for _, u := range a.After.Files[p].Units {
			if c.At == p || strings.HasSuffix(c.At, ":"+u.Name) || strings.HasSuffix(c.Object, u.ID) {
				words = append(words, units.Sequence(u.Tokens, true)...)
			}
		}
	}
	grams, err := units.Grams(ctx, words)
	if err != nil || len(grams) == 0 {
		return nil, err
	}
	return units.MinHash(ctx, grams)
}

func signatureMatch(ctx context.Context, signature []uint64, f units.File) (bool, error) {
	if len(signature) == 0 {
		return false, nil
	}
	var matches []int
	for _, u := range append(append([]units.Unit(nil), f.Units...), units.Unit{Tokens: f.Tokens}) {
		grams, err := units.Grams(ctx, units.Sequence(u.Tokens, true))
		if err != nil {
			return false, err
		}
		if len(grams) == 0 {
			continue
		}
		candidate, err := units.MinHash(ctx, grams)
		if err != nil {
			return false, err
		}
		same := 0
		for i, h := range candidate {
			if i < len(signature) && h == signature[i] {
				same++
			}
		}
		matches = append(matches, same)
	}
	best, total := 0, 0
	for _, n := range matches {
		best = max(best, n)
		total += n
	}
	if best == len(signature) {
		return true, nil
	}
	if len(matches) <= 1 {
		return false, nil
	}
	null := float64(total-best+1) / float64((len(matches)-1)*len(signature)+1)
	return repostats.BinomialTail(best, len(signature), null) < CalibrationSignificance/float64(len(matches)), nil
}

type feedbackSubject struct {
	Feedback                                               ledger.IssueFeedback
	Present, Followed, Restored, WorkspaceDeleted, Unknown bool
	Removals                                               []ledger.FileChange
}

func feedbackID(task string, f ledger.IssueFeedback) string {
	return units.Hash(task, f.Issue.ID)
}

/**
 * 从实际展示时的快照开始核对编辑链；混合来源、缺失记录或无法识别的内容不产生标签。
 * 是否采纳取决于 Agent 是否删除过对象，不能用最后快照或笔记措辞代替。
 */
func feedbackSubjects(ctx context.Context, s *ledger.Store, w gitstate.Workspace, v ledger.TaskView) ([]feedbackSubject, repostats.Repository, error) {
	var origin repostats.Repository
	if v.Task == nil {
		return nil, origin, nil
	}
	feedback, err := s.IssueFeedback(ctx, v.Task.ID)
	if err != nil || len(feedback) == 0 {
		return nil, origin, err
	}
	task, current, err := s.CommandContext(ctx, w.ID)
	if err != nil || task.ID != v.Task.ID || current.ID != v.Task.LastObservationID {
		return nil, origin, err
	}
	records, err := s.TaskChanges(ctx, w.ID, v.Task.ID, v.Task.LastObservationID)
	if err != nil {
		return nil, origin, err
	}
	if found, err := s.AnalysisCache(ctx, "repository", units.Hash(Algorithm, v.Task.OriginTree), &origin); err != nil || !found {
		return nil, origin, err
	}
	var out []feedbackSubject
	for _, feedback := range feedback {
		issue := feedback.Issue
		if issue.Algorithm != Algorithm {
			continue
		}
		x := feedbackSubject{Feedback: feedback}
		path := strings.SplitN(issue.At, ":", 2)[0]
		start, err := w.TreeFiles(ctx, feedback.Tree, path)
		if err != nil {
			return nil, origin, err
		}
		end, err := w.TreeFiles(ctx, current.Tree, path)
		if err != nil {
			return nil, origin, err
		}
		var events []ledger.FileChange
		ids := []string{}
		add := func(blob string) {
			if blob != "" {
				ids = append(ids, blob)
			}
		}
		add(start[path].Blob)
		add(end[path].Blob)
		for _, record := range records {
			if record.Path == path && record.AfterObservation > feedback.ObservationID && record.At.After(feedback.At) {
				events = append(events, record)
				add(record.Before.Blob)
				add(record.After.Blob)
			}
		}
		blobs, err := w.ReadBlobs(ctx, ids)
		if err != nil {
			return nil, origin, err
		}
		match := func(file gitstate.File) (bool, error) {
			if file.Blob == "" {
				return false, nil
			}
			if file.Mode != "100644" && file.Mode != "100755" {
				x.Unknown = true
				return false, nil
			}
			f, err := units.Analyze(ctx, units.Source{Path: path, Data: blobs[file.Blob]}, origin.SyntaxFor(path))
			if err != nil {
				return false, err
			}
			if f.Binary {
				x.Unknown = true
				return false, nil
			}
			return signatureMatch(ctx, issue.ContentSignature, f)
		}
		prior := start[path]
		x.Present, err = match(prior)
		if err != nil {
			return nil, origin, err
		}
		x.Unknown = x.Unknown || !x.Present || len(issue.ContentSignature) == 0
		for i, event := range events {
			if event.Before != prior || (i > 0 && events[i-1].AfterObservation == event.AfterObservation) || (i+1 < len(events) && events[i+1].AfterObservation == event.AfterObservation) {
				x.Unknown = true
			}
			present, err := match(event.After)
			if err != nil {
				return nil, origin, err
			}
			if x.Present && !present {
				switch event.Kind {
				case "agent_edit":
					x.Followed = true
					x.Removals = append(x.Removals, event)
				case "workspace_update":
					x.WorkspaceDeleted = true
				default:
					x.Unknown = true
				}
			}
			if !x.Present && present && x.Followed && event.Kind == "workspace_update" {
				x.Restored = true
			}
			x.Present, prior = present, event.After
		}
		x.Unknown = x.Unknown || prior != end[path]
		out = append(out, x)
	}
	return out, origin, nil
}

func retentionNote(note ledger.Note, subject feedbackSubject) bool {
	return !subject.Unknown && !subject.Followed && subject.Present && note.Target != nil && note.Target.Kind == "issue" && note.Target.ID == subject.Feedback.Issue.ID && note.At.After(subject.Feedback.At)
}

func Calibrate(ctx context.Context, s *ledger.Store, w gitstate.Workspace, v ledger.TaskView, complete bool) error {
	subjects, origin, err := feedbackSubjects(ctx, s, w, v)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		issue := subject.Feedback.Issue
		id := feedbackID(v.Task.ID, subject.Feedback)
		if subject.Unknown || (subject.Followed && subject.Present && !subject.Restored) {
			if err := s.RemoveCalibration(ctx, id); err != nil {
				return err
			}
			continue
		}
		reason, label := "", false
		failed := false
		for _, removal := range subject.Removals {
			ok, err := removalFailure(ctx, w, v, removal, origin)
			if err != nil {
				return err
			}
			failed = failed || ok
		}
		switch {
		case subject.Restored:
			reason = "Agent 删除后，工作区更新恢复了相似内容"
		case failed:
			reason = "Agent 删除后，在只有该文件撤回变化的快照上，既有验收由通过变为失败"
		case subject.Followed && !subject.Present && complete:
			reason, label = "Agent 删除，到任务完成时没有恢复或可核对的验收失败", true
		case !subject.Followed && subject.WorkspaceDeleted && !subject.Present:
			reason, label = "Agent 未删除，后续工作区更新删除了对应内容", true
		default:
			for _, note := range v.Notes {
				if !retentionNote(note, subject) || note.Citation == nil {
					continue
				}
				object := strings.TrimPrefix(issue.LogicalAt, "file:")
				if object != "" && (strings.Contains(note.Citation.Quote, object) || strings.Contains(note.Citation.Quote, strings.TrimPrefix(object, "dep:"))) {
					reason = "保留理由中的用户原话明确点名该对象"
				}
			}
		}
		if reason == "" {
			continue
		}
		if err := s.SaveCalibration(ctx, ledger.CalibrationLabel{ID: id, ProjectID: w.ProjectID, IssueID: issue.ID, Kind: issue.Rule, Reason: reason, Signals: issue.Signals, Accepted: label}); err != nil {
			return err
		}
	}
	return nil
}

// 两次命令的输入只相差已记录的撤回文件时，才把验收失败关联到该反馈。
func removalFailure(ctx context.Context, w gitstate.Workspace, view ledger.TaskView, edit ledger.FileChange, origin repostats.Repository) (bool, error) {
	model := repostats.LearnResults(runSamples(view.Verifications), repostats.CaseNames(origin))
	for _, failed := range view.Verifications {
		if failed.Algorithm != Algorithm || !failed.Executed || failed.State != "failed" || !failed.StartedAt.After(edit.At) || failed.Tree != failed.AfterTree || !failed.Scope.Complete {
			continue
		}
		commandText := strings.Join(append([]string{command(failed).Program}, command(failed).Args...), " ")
		parsed, err := repostats.ParseResults(ctx, model, repostats.Run{ID: failed.ID, Output: failed.Output, ExitCode: failed.ExitCode, Started: true})
		if err != nil {
			return false, err
		}
		acceptance := false
		for _, req := range view.History {
			if req.Status == "revoked" || !req.At.Before(edit.At) {
				continue
			}
			acceptance = acceptance || commandText != "" && strings.Contains(req.Acceptance, commandText)
			for _, result := range parsed {
				acceptance = acceptance || result.Name != failed.ID && result.State == "failed" && strings.Contains(req.Acceptance, result.Name)
			}
		}
		if !acceptance {
			continue
		}
		for _, passed := range view.Verifications {
			if passed.Algorithm != Algorithm || !passed.Executed || passed.State != "passed" || passed.CommandKey != failed.CommandKey || passed.EnvironmentID != failed.EnvironmentID || passed.Scope.Tool != failed.Scope.Tool || !passed.Scope.Complete || !passed.EndedAt.Before(edit.At) || passed.Tree != passed.AfterTree {
				continue
			}
			before, err := w.TreeFiles(ctx, passed.Tree)
			if err != nil {
				return false, err
			}
			after, err := w.TreeFiles(ctx, failed.Tree)
			if err != nil {
				return false, err
			}
			if before[edit.Path].Blob != edit.Before.Blob || after[edit.Path].Blob != edit.After.Blob {
				continue
			}
			paths := map[string]bool{}
			for p := range before {
				paths[p] = true
			}
			for p := range after {
				paths[p] = true
			}
			only := true
			for p := range paths {
				if p != edit.Path && (before[p].Blob != after[p].Blob || before[p].Mode != after[p].Mode) {
					only = false
					break
				}
			}
			if only {
				return true, nil
			}
		}
	}
	return false, ctx.Err()
}

func rejectionDependencies(ctx context.Context, s *ledger.Store, w gitstate.Workspace, v ledger.TaskView, a analysis) error {
	subjects, _, err := feedbackSubjects(ctx, s, w, v)
	if err != nil {
		return err
	}
	for _, note := range v.Notes {
		for _, subject := range subjects {
			if !retentionNote(note, subject) {
				continue
			}
			issue := subject.Feedback.Issue
			closure, err := changes.Closure(ctx, a.Changes.Clusters, []string{issue.LogicalAt})
			if err != nil {
				return err
			}
			for _, region := range a.Regions {
				for _, needed := range region.Clusters {
					for _, dependent := range closure {
						if dependent != issue.LogicalAt && dependent == needed && strings.Contains(note.Text, dependent) {
							if err := s.SaveCalibration(ctx, ledger.CalibrationLabel{ID: feedbackID(v.Task.ID, subject.Feedback), ProjectID: w.ProjectID, IssueID: issue.ID, Kind: issue.Rule, Reason: "被需求相关区内的保留簇依赖", Signals: issue.Signals, Accepted: false}); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}
	return nil
}
