package check

import (
	"context"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"slices"
	"sort"
	"strings"
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

func EditAdvice(ctx context.Context, s *ledger.Store, w gitstate.Workspace, current, contents map[string][]byte) (string, error) {
	view, err := s.EditAnalysisContext(ctx, w.ID)
	if err != nil || view.Task == nil {
		return "", err
	}
	ctx = publishedOnly(ctx)
	base, err := taskIndex(ctx, s, w, *view.Task)
	if err != nil {
		return "", err
	}
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
	delta, err := changes.Compare(ctx, changes.Input{Before: before, After: after})
	if err != nil {
		return "", err
	}
	snapshot := indexedSnapshot{Origin: base, Current: base, Before: before, After: after, Paths: paths}
	if err := snapshot.editCaseNeighbourhood(ctx, w, beforeFiles, files); err != nil {
		return "", err
	}
	result, err := analyzeSignals(ctx, view, signals.Input{EditPreview: true, Queries: snapshot.editQueries(ctx, w, beforeFiles, files), Before: snapshot.Before, After: snapshot.After, Changes: delta, Requirements: requirements(view)})
	if err != nil {
		return "", err
	}
	var advice []string
	for _, kind := range []string{"test_equivalence", "recovery", "forwarding", "abstraction", "parameter", "external", "duplicate"} {
		for _, c := range result.Candidates {
			if c.Kind != kind || len(advice) >= CategoryLimit {
				continue
			}
			text := c.At + "："
			if c.Kind == "test_equivalence" {
				text = "拟新增相近测试 " + text
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
			if !slices.Contains(advice, text) {
				advice = append(advice, text)
			}
		}
	}
	return LimitFeedback(strings.Join(advice, "\n")), snapshot.err()
}

/**
 * 参数只用本批目标中的调用形状核对，不沿用索引中的旧调用。
 * 起点索引只用于找相似候选；预览不能把已变化的实现当作当前可复用的参照。
 * 同批目标直接核对两份内容，其余文件按路径复用本次读取结果。
 */
func (s indexedSnapshot) editQueries(ctx context.Context, w gitstate.Workspace, current, proposed map[string]units.File) *signals.Queries {
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
	checked := map[string]bool{}
	q.Similar = func(signature []uint64) []signals.SimilarUnit {
		var out []signals.SimilarUnit
		for _, candidate := range similar(signature) {
			if err := ctx.Err(); err != nil {
				s.Origin.err = err
				return nil
			}
			if !candidate.Callable {
				continue
			}
			p := candidate.Path
			if f, editing := proposed[p]; editing {
				// 索引单元省略了词流，从已有文件分析中取回，不能拿空词流比较。
				for _, u := range s.Origin.load(p).Units {
					if u.ID == candidate.Unit.ID && u.Start == candidate.Unit.Start {
						if samePreviewDeclaration(current[p], u) && samePreviewDeclaration(f, u) {
							out = append(out, candidate)
						}
						break
					}
				}
				continue
			}
			valid, known := checked[p]
			if !known {
				f := s.Origin.fileInfo(p)
				indexed := gitstate.File{Blob: f.Blob, Mode: f.Mode}
				observed, _, err := w.ReadObservedFile(ctx, p, indexed)
				if err != nil {
					s.Origin.err = err
					return nil
				}
				valid = indexed.Blob != "" && observed == indexed
				checked[p] = valid
			}
			if valid {
				out = append(out, candidate)
			}
		}
		return out
	}
	return q
}

/**
 * 位置和完整词流都要保留，避免用相似度规整后相同的字面量或旧位置确认参照。
 */
func samePreviewDeclaration(f units.File, indexed units.Unit) bool {
	for _, u := range f.Units {
		if u.ID == indexed.ID && u.Start == indexed.Start && u.End == indexed.End &&
			u.HeaderEnd == indexed.HeaderEnd && u.Line == indexed.Line && u.EndLine == indexed.EndLine &&
			slices.Equal(u.Tokens, indexed.Tokens) && signals.Structure(f, u).Callable {
			return true
		}
	}
	return false
}

/**
 * 预览只分析请求中的当前与预期内容。直接参照仍来自已发布索引，内容已变化时
 * 略过测试比较，避免把旧断言或旧分段点当作当前事实。
 */
func (s *indexedSnapshot) editCaseNeighbourhood(ctx context.Context, w gitstate.Workspace, beforeFiles, files map[string]units.File) error {
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
	before, after := s.Before, s.After
	s.caseNeighbourhood()
	neighbours := s.After.Files
	s.Before, s.After = before, after
	if err := s.err(); err != nil {
		return err
	}
	for _, p := range keys(neighbours) {
		if _, editing := files[p]; editing {
			continue
		}
		f := neighbours[p]
		indexed := gitstate.File{Blob: f.Blob, Mode: f.Mode}
		current, _, err := w.ReadObservedFile(ctx, p, indexed)
		if err != nil {
			return err
		}
		if current != indexed {
			return fmt.Errorf("测试参照 %s 已变化，略过编辑前提示", p)
		}
	}
	for p, f := range neighbours {
		if _, editing := files[p]; !editing {
			beforeFiles[p], files[p] = f, f
		}
	}
	paths := keys(files)
	var err error
	s.Before, err = s.Origin.apply(paths, beforeFiles, "current")
	if err != nil {
		return err
	}
	s.After, err = s.Origin.apply(paths, files, "preview")
	return err
}
