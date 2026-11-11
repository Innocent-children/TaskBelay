package check

import (
	"context"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func ReuseAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, actor ledger.Actor, text string) (string, error) {
	v, err := s.TaskView(ctx, w.ID, "", actor)
	if err != nil || v.Task == nil {
		return "", err
	}
	// 用户消息只使用已发布的索引；索引未就绪时不提示，把时间留给接续摘要。
	x, err := publishedIndex(ctx, s, w, v.Task.OriginTree)
	if errors.Is(err, ErrIndexPending) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	regions, err := changes.Relate(ctx, changes.GraphInput{Graph: x.graph(), Requirements: []changes.Requirement{{ID: "message", Quote: text}}})
	if err != nil || len(regions) == 0 {
		return "", err
	}
	type choice struct {
		at    string
		score float64
	}
	var choices []choice
	for key, score := range regions[0].Scores {
		if !strings.HasPrefix(key, "u:") {
			continue
		}
		u := x.unit(key[2:])
		score *= float64(1 + x.stats(u.Unit.Name).DF)
		if score > 0 && u.Unit.Name != "" {
			choices = append(choices, choice{u.Path + ":" + u.Unit.Name, score})
		}
	}
	sort.Slice(choices, func(i, j int) bool {
		if choices[i].score != choices[j].score {
			return choices[i].score > choices[j].score
		}
		return choices[i].at < choices[j].at
	})
	var out []string
	for _, c := range choices[:min(ReuseHints, len(choices))] {
		out = append(out, c.at)
	}
	if err := x.readResult(); err != nil || len(out) == 0 {
		return "", err
	}
	return "可先核对已有实现：" + strings.Join(out, "、"), nil
}

func EditAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, current, contents map[string][]byte) (advice string, failure error) {
	started, stageStarted := time.Now(), time.Now()
	stage, scope, reason := "context", "none", "no_task"
	durations := map[string]float64{}
	details := map[string]any{"paths": keys(contents), "displayed": 0}
	var previewErr error
	advance := func(next string) {
		durations[stage] = float64(time.Since(stageStarted).Microseconds()) / 1000
		stage, stageStarted = next, time.Now()
	}
	defer func() {
		advance(stage)
		status := "complete"
		if scope == "none" {
			status = "skipped"
		} else if scope != "full" {
			status = "partial"
		}
		if failure != nil {
			previewErr = failure
			status = "failed"
		}
		if previewErr != nil {
			reason = "error"
			if errors.Is(previewErr, ErrIndexPending) {
				status, reason = "skipped", "index_pending"
			} else if errors.Is(previewErr, context.DeadlineExceeded) {
				reason = "deadline"
			} else if errors.Is(previewErr, context.Canceled) {
				reason = "canceled"
			}
			details["error"] = previewErr.Error()
		}
		details["status"], details["scope"], details["stage"], details["reason"] = status, scope, stage, reason
		details["durations_ms"], details["elapsed_ms"] = durations, float64(time.Since(started).Microseconds())/1000
		details["characters"] = utf8.RuneCountInString(advice)
		ledger.DebugRecord(ctx, "edit_preview.result", details)
	}()
	if deadline, ok := ctx.Deadline(); ok {
		details["remaining_ms"] = float64(time.Until(deadline).Microseconds()) / 1000
	}
	view, err := s.EditAnalysisContext(ctx, w.ID)
	if err != nil || view.Task == nil {
		return "", err
	}
	ctx = publishedOnly(ctx)
	advance("index")
	base, err := taskIndex(ctx, s, w, *view.Task)
	if err != nil {
		return "", err
	}
	details["index_id"], details["index_generation"] = base.id, base.meta.Generation
	details["index_variant"], details["index_tree"] = base.variant, base.meta.Tree
	details["converged"] = base.meta.Converged
	advance("targets")
	paths := keys(contents)
	beforeFiles := map[string]units.File{}
	files := map[string]units.File{}
	for _, p := range paths {
		var syn units.Syntax
		base.get("syntax", path.Ext(p), &syn)
		if data, exists := current[p]; exists {
			f, err := units.Analyze(ctx, units.Source{Path: p, Blob: units.Hash(string(data)), Mode: "100644", Data: data}, syn)
			if err != nil {
				return "", err
			}
			beforeFiles[p] = f
		}
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: units.Hash(string(contents[p])), Mode: "100644", Data: contents[p]}, syn)
		if err != nil {
			return "", err
		}
		files[p] = f
	}
	before, err := base.apply(paths, beforeFiles, "current")
	if err != nil {
		return "", err
	}
	after, err := base.apply(paths, files, "preview")
	if err != nil {
		return "", err
	}
	advance("diff")
	delta, err := changes.Compare(ctx, changes.Input{Before: before, After: after})
	if err != nil {
		return "", err
	}
	snapshot := indexedSnapshot{Origin: base, Current: base, Before: before, After: after, Paths: paths}
	advance("test_purposes")
	input := signals.Input{EditPreview: true, Before: before, After: after, Changes: delta, Requirements: requirements(view), Retained: retainedObjects(view)}
	purposes, err := signals.TestPurposePreview(ctx, input)
	if err != nil {
		return "", err
	}
	if err := snapshot.err(); err != nil {
		return "", err
	}
	scope = "test_purposes"
	details["purpose_candidates"], details["test_cases"] = len(purposes.Candidates), len(purposes.Cases)
	completed := purposes.Candidates
	partial := func(err error) (string, error) {
		previewErr = err
		suffix := "本次仅完成上述测试用途核对，其余编辑前核对未完成。"
		if scope == "targets" {
			suffix = "本次已完成目标内容的设计与测试用途核对，测试比较未完成。"
		} else if scope == "target_purposes" {
			suffix = "本次已完成目标内容的独立设计与测试用途核对，重复实现与测试比较未完成。"
		}
		text, selected := editAdvice(completed, suffix)
		details["displayed"] = selected
		return text, nil
	}
	advance("design")
	input.PreviewTestFiles = map[string]bool{}
	for _, c := range purposes.Cases {
		if after.Roles[c.Path].Test {
			input.PreviewTestFiles[c.Path] = true
		}
	}
	input.Queries = snapshot.editQueries(ctx, beforeFiles, files)
	design, err := signals.DesignPreview(ctx, input)
	if err != nil {
		return partial(err)
	}
	if err := snapshot.err(); err != nil {
		return partial(err)
	}
	design = requirementFacts(view, input, design)
	completed = append(slices.Clone(design.Candidates), purposes.Candidates...)
	scope = "target_purposes"
	details["target_design_candidates"] = len(design.Candidates)
	advance("similarities")
	design, err = signals.SimilarityPreview(ctx, input, design)
	if err != nil {
		return partial(err)
	}
	if err := snapshot.err(); err != nil {
		return partial(err)
	}
	scope = "targets"
	details["design_candidates"] = len(design.Candidates)
	compared := map[string]bool{}
	for _, c := range purposes.Cases {
		if c.Added && c.AgentOnly && !c.Retained {
			compared[repostats.UnitKey(c.Path, units.Unit{ID: c.Unit})] = true
		}
	}
	details["test_references_needed"] = len(compared) > 0
	result := design
	if len(compared) > 0 {
		completed = append(slices.Clone(design.Candidates), purposes.Candidates...)
		advance("references")
		if err := snapshot.editCaseNeighbourhood(compared); err != nil {
			return partial(err)
		}
		advance("test_comparisons")
		input.Before, input.After = snapshot.Before, snapshot.After
		result, err = signals.CompletePreview(ctx, input, design)
		if err != nil {
			return partial(err)
		}
		if err := snapshot.err(); err != nil {
			return partial(err)
		}
	}
	// 同文件副本只依赖目标原文，不需要为它准备跨文件参照。
	for _, c := range purposes.Candidates {
		if c.Kind == "test_copy" {
			result.Candidates = append(result.Candidates, c)
		}
	}
	scope, reason = "full", "no_candidates"
	details["candidates"] = len(result.Candidates)
	advance("feedback")
	advice, selected := editAdvice(result.Candidates, "")
	details["displayed"] = selected
	if selected > 0 {
		reason = "advice"
	} else if len(result.Candidates) > 0 {
		reason = "feedback_budget"
	}
	return advice, nil
}

func editAdvice(candidates []signals.Candidate, suffix string) (string, int) {
	var advice []string
	characters := utf8.RuneCountInString(suffix)
	copies := map[string]bool{}
	for _, kind := range []string{"test_copy", "test_equivalence", "test_budget", "assertion", "field_overwrite", "recovery", "forwarding", "abstraction", "parameter", "external", "duplicate"} {
		for _, c := range candidates {
			if c.Kind != kind || len(advice) >= CategoryLimit || copies[c.Object] {
				continue
			}
			text := c.At + "："
			if c.Kind == "test_copy" {
				text = "拟新增测试副本 " + text
			} else if c.Kind == "test_equivalence" {
				text = "拟新增相近测试 " + text
			} else if c.Kind == "test_budget" {
				text = "拟新增测试数量 " + text
			} else if c.Kind == "assertion" {
				text = "拟新增测试用途 " + text
			} else if c.Kind == "field_overwrite" {
				text = "拟编辑字段初值 " + text
			} else if c.Kind == "recovery" {
				text = "拟编辑分支调用 " + text
			} else if c.Kind == "forwarding" {
				text = "拟编辑转发包装 " + text
			} else if c.Kind == "abstraction" {
				text = "拟编辑抽象 " + text
			} else if c.Kind == "parameter" {
				text = "拟编辑参数 " + text
			}
			if c.Kind != "duplicate" {
				text += strings.Join(c.Facts, "；") + "；"
			}
			text += c.Advice
			cost := utf8.RuneCountInString(text)
			if len(advice) > 0 || suffix != "" {
				cost++
			}
			if !slices.Contains(advice, text) && characters+cost <= FeedbackBudget {
				advice = append(advice, text)
				characters += cost
				if c.Kind == "test_copy" {
					copies[c.Object] = true
				}
			}
		}
	}
	if len(advice) == 0 {
		return "", 0
	}
	text := strings.Join(advice, "\n")
	if suffix != "" {
		text += "\n" + suffix
	}
	return text, len(advice)
}

/**
 * 参数只用本批目标中的调用形状核对，不沿用索引中的旧调用。
 * 起点索引只用于找相似候选；预览不能把已变化的实现当作当前可复用的参照。
 * 同批目标直接核对两份内容，其余文件按路径复用本次读取结果。
 */
func (s indexedSnapshot) editQueries(ctx context.Context, current, proposed map[string]units.File) *signals.Queries {
	q := s.queries()
	// 仅用原始目标收集调用，排除索引旧调用及为测试比较补齐的参照。
	calls := map[string][]signals.CallSite{}
	for _, p := range s.Paths {
		if err := ctx.Err(); err != nil {
			s.Origin.err = err
			break
		}
		for name, sites := range units.Calls(proposed[p].Tokens) {
			for _, args := range sites {
				calls[name] = append(calls[name], signals.CallSite{Path: p, Args: args})
			}
		}
	}
	q.Calls = func(name string) []signals.CallSite { return calls[name] }
	similar := q.Similar
	references := map[string]units.File{}
	q.Similar = func(signature []uint64, sameFileKeys []string) []signals.SimilarUnit {
		var out []signals.SimilarUnit
		for _, candidate := range similar(signature, sameFileKeys) {
			if err := ctx.Err(); err != nil {
				s.Origin.err = err
				return nil
			}
			if !candidate.Callable {
				continue
			}
			p := candidate.Path
			f, editing := proposed[p]
			if !editing {
				info := s.Origin.fileInfo(p)
				ref, known := references[p]
				if !known {
					var err error
					ref, err = s.Origin.readPreviewReference(p, gitstate.File{Blob: info.Blob, Mode: info.Mode}, true)
					if err != nil {
						s.Origin.err = err
						return nil
					}
					references[p] = ref
				}
				if ref.Blob != "" && ref.Blob == info.Blob {
					out = append(out, candidate)
					continue
				}
				f = ref
				if len(f.Units) == 0 {
					continue
				}
			}
			// 索引单元省略了词流，从已有文件分析中取回原声明及所在各行。
			origin := s.Origin.load(p)
			for _, u := range origin.Units {
				if u.ID != candidate.Unit.ID || u.Start != candidate.Unit.Start {
					continue
				}
				lines := origin.Lines[u.Line-1 : u.EndLine]
				if editing {
					if _, ok := previewDeclaration(current[p], u, lines, true); !ok {
						break
					}
				}
				if matched, ok := previewDeclaration(f, u, lines, editing); ok {
					candidate.Unit = matched
					out = append(out, candidate)
				}
				break
			}
		}
		return out
	}
	return q
}

/**
 * 文件与索引一致时直接使用索引单元。只有同模式的普通文件内容变化时，
 * 才按调用方需要分析已读到的字节；无法采用的参照返回空文件。
 */
func (x *repositoryIndex) readPreviewReference(p string, indexed gitstate.File, analyzeChanged bool) (units.File, error) {
	observed, data, err := x.w.ReadObservedFile(x.ctx, p, indexed)
	if err != nil || indexed.Blob == "" || observed.Mode != indexed.Mode {
		return units.File{}, err
	}
	if observed == indexed {
		return units.File{Path: p, Blob: observed.Blob, Mode: observed.Mode}, nil
	}
	if !analyzeChanged || !regular(observed) {
		return units.File{}, nil
	}
	var syntax units.Syntax
	x.get("syntax", path.Ext(p), &syntax)
	source := units.Source{Path: p, Blob: observed.Blob, Mode: observed.Mode, Data: data}
	return units.Analyze(x.ctx, source, syntax)
}

/**
 * 同批目标在原位置沿用完整词流核对。其余情况核对声明所在各行的原文，
 * 返回实际找到的声明，让重叠判断使用同一份拟写入内容的坐标。
 */
func previewDeclaration(f units.File, indexed units.Unit, lines []string, editTarget bool) (units.Unit, bool) {
	for _, u := range f.Units {
		if u.ID != indexed.ID || u.Name != indexed.Name || u.End-u.Start != indexed.End-indexed.Start ||
			u.HeaderEnd-u.Start != indexed.HeaderEnd-indexed.Start || u.EndLine-u.Line != indexed.EndLine-indexed.Line {
			continue
		}
		if editTarget && u.Start == indexed.Start && u.Line == indexed.Line {
			if !slices.Equal(u.Tokens, indexed.Tokens) {
				continue
			}
		} else if !slices.Equal(f.Lines[u.Line-1:u.EndLine], lines) {
			continue
		}
		if signals.Structure(f, u).Callable {
			return u, true
		}
	}
	return units.Unit{}, false
}

/**
 * 索引按生产声明名和测试文件中的引用决定参照范围。只有这些依据仍有效，
 * 才能在原范围内用当前实现比较用例。
 */
func (s *indexedSnapshot) editCaseNeighbourhood(compared map[string]bool) error {
	if !s.Before.Converged || !s.After.Converged {
		return nil
	}
	tests := false
	for _, test := range s.After.TestUnits {
		tests = tests || test
	}
	if !tests {
		return nil
	}
	paths := s.casePaths(compared)
	if err := s.err(); err != nil {
		return err
	}
	var references []string
	changed := map[string]units.File{}
	for _, p := range paths {
		if _, editing := s.After.Files[p]; editing {
			continue
		}
		f := s.Origin.fileInfo(p)
		if f.Blob == "" {
			continue
		}
		indexed := gitstate.File{Blob: f.Blob, Mode: f.Mode}
		current, err := s.Origin.readPreviewReference(p, indexed, !f.Role.Test)
		if err != nil {
			return err
		}
		if current.Blob == "" || current.Binary {
			return fmt.Errorf("测试参照 %s 已变化，略过编辑前提示", p)
		}
		if current.Blob != indexed.Blob {
			changed[p] = current
		}
		references = append(references, p)
	}
	neighbours := s.Origin.view(references)
	if err := s.err(); err != nil {
		return err
	}
	if len(changed) > 0 {
		current, err := s.Origin.apply(keys(changed), changed, "current")
		if err != nil {
			return err
		}
		for p, file := range changed {
			if current.Roles[p].Test || !maps.Equal(signals.DeclarationNames(file), signals.DeclarationNames(neighbours.Files[p])) {
				return fmt.Errorf("生产参照 %s 的测试身份或声明名已变化，略过编辑前提示", p)
			}
			neighbours.Files[p], neighbours.Roles[p] = file, current.Roles[p]
		}
	}
	for _, repo := range []*repostats.Repository{&s.Before, &s.After} {
		maps.Copy(repo.Files, neighbours.Files)
		maps.Copy(repo.Roles, neighbours.Roles)
		maps.Copy(repo.Syntax, neighbours.Syntax)
		for key, test := range neighbours.TestUnits {
			if test {
				repo.TestUnits[key] = true
			}
		}
		maps.Copy(repo.TestSeeds, neighbours.TestSeeds)
	}
	return nil
}
