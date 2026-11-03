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

// 处理说明只有和后续内容变化对应时才产生标签；缺少因果记录时不猜测。
func Calibrate(ctx context.Context, s *ledger.Store, w gitstate.Workspace, v ledger.TaskView, complete bool) error {
	if v.Task == nil {
		return nil
	}
	records, err := s.TaskChanges(ctx, w.ID, v.Task.ID, v.Task.LastObservationID)
	if err != nil {
		return err
	}
	var ids []string
	for _, r := range records {
		if r.Before.Blob != "" {
			ids = append(ids, r.Before.Blob)
		}
		if r.After.Blob != "" {
			ids = append(ids, r.After.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return err
	}
	var origin repostats.Repository
	if found, err := s.AnalysisCache(ctx, "repository", units.Hash(Algorithm, v.Task.OriginTree), &origin); err != nil || !found {
		return err
	}
	issues, err := s.Issues(ctx, v.Task.ID)
	if err != nil {
		return err
	}
	for _, note := range v.Notes {
		if note.Target == nil || note.Target.Kind != "issue" {
			continue
		}
		accepted := strings.HasPrefix(strings.TrimSpace(note.Text), "采纳")
		rejected := strings.HasPrefix(strings.TrimSpace(note.Text), "拒绝") || note.Target.Effect == "retain"
		if !accepted && !rejected {
			continue
		}
		for _, issue := range issues {
			if issue.Algorithm != Algorithm || issue.ID != note.Target.ID {
				continue
			}
			reason := ""
			label := false
			if rejected && note.Citation != nil {
				object := strings.TrimPrefix(issue.LogicalAt, "file:")
				if strings.Contains(note.Citation.Quote, object) || strings.Contains(note.Citation.Quote, strings.TrimPrefix(object, "dep:")) {
					reason = "拒绝理由中的用户原话明确点名该对象"
				}
			}
			gone, restored, userDeleted, acceptanceFailed := false, false, false, false
			for _, r := range records {
				if !r.At.After(note.At) || r.Path != strings.SplitN(issue.At, ":", 2)[0] {
					continue
				}
				f, err := units.Analyze(ctx, units.Source{Path: r.Path, Data: blobs[r.After.Blob]}, origin.SyntaxFor(r.Path))
				if err != nil {
					return err
				}
				present, err := signatureMatch(ctx, issue.ContentSignature, f)
				if err != nil {
					return err
				}
				prior, err := units.Analyze(ctx, units.Source{Path: r.Path, Data: blobs[r.Before.Blob]}, origin.SyntaxFor(r.Path))
				if err != nil {
					return err
				}
				wasPresent, err := signatureMatch(ctx, issue.ContentSignature, prior)
				if err != nil {
					return err
				}
				if accepted && r.Kind == "agent_edit" && wasPresent && !present {
					gone = true
					failed, err := removalFailure(ctx, w, v, r, origin)
					if err != nil {
						return err
					}
					acceptanceFailed = acceptanceFailed || failed
				}
				if gone && r.Kind == "workspace_update" && present {
					restored = true
				}
				if rejected && r.Kind == "workspace_update" && wasPresent && !present {
					userDeleted = true
				}
			}
			if restored {
				reason = "Agent 采纳后，工作区更新恢复了相似内容"
			} else if acceptanceFailed {
				reason = "Agent 采纳后，在只有该文件撤回变化的快照上，既有验收由通过变为失败"
			} else if accepted && gone && complete {
				reason = "Agent 采纳，到任务完成时没有恢复"
				label = true
			} else if rejected && userDeleted {
				reason = "Agent 拒绝后，工作区更新删除了对应内容"
				label = true
			}
			if reason != "" {
				if err = s.SaveCalibration(ctx, ledger.CalibrationLabel{ID: units.Hash(v.Task.ID, issue.ID, note.ID), ProjectID: w.ProjectID, IssueID: issue.ID, Kind: issue.Rule, Reason: reason, Signals: issue.Signals, Accepted: label}); err != nil {
					return err
				}
			}
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
	for _, note := range v.Notes {
		if note.Target == nil || note.Target.Kind != "issue" || !strings.HasPrefix(strings.TrimSpace(note.Text), "拒绝") {
			continue
		}
		for _, issue := range v.Issues {
			if issue.ID != note.Target.ID || issue.Algorithm != Algorithm {
				continue
			}
			closure, err := changes.Closure(ctx, a.Changes.Clusters, []string{issue.LogicalAt})
			if err != nil {
				return err
			}
			for _, region := range a.Regions {
				for _, needed := range region.Clusters {
					for _, dependent := range closure {
						if dependent != issue.LogicalAt && dependent == needed && strings.Contains(note.Text, dependent) {
							if err := s.SaveCalibration(ctx, ledger.CalibrationLabel{ID: units.Hash(v.Task.ID, issue.ID, note.ID), ProjectID: w.ProjectID, IssueID: issue.ID, Kind: issue.Rule, Reason: "被需求相关区内的保留簇依赖", Signals: issue.Signals, Accepted: false}); err != nil {
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
