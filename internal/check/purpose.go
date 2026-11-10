package check

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type quoteRange struct{ start, end, line, lastLine int }

// 片段可以从一行中部开始。即使指定了行号，同一行上有多个匹配也不能任选其一。
func locateQuote(content []byte, ref ledger.CodeQuote) (quoteRange, bool) {
	var found quoteRange
	quote := []byte(ref.Quote)
	if len(quote) == 0 {
		return found, false
	}
	offset, line := 0, 1
	for {
		next := bytes.Index(content[offset:], quote)
		if next < 0 {
			break
		}
		start := offset + next
		line += bytes.Count(content[offset:start], []byte{'\n'})
		if ref.Line > 0 && line > ref.Line {
			break
		}
		if ref.Line == 0 || line == ref.Line {
			if found.line != 0 {
				return quoteRange{}, false
			}
			found = quoteRange{start, start + len(quote), line, line + bytes.Count(quote[:len(quote)-1], []byte{'\n'})}
		}
		offset = start + 1
		if content[start] == '\n' {
			line++
		}
	}
	return found, found.line != 0
}

type purposeFiles struct {
	current, origin map[string]gitstate.File
	contents        map[string][]byte
}

func regular(file gitstate.File) bool { return file.Mode == "100644" || file.Mode == "100755" }

func (f purposeFiles) locate(ref ledger.CodeQuote, recorded map[string]gitstate.File) (quoteRange, string) {
	file := f.current[ref.Path]
	if !regular(file) || file != recorded[ref.Path] {
		return quoteRange{}, ref.Path + " 已变化或缺少可核对的普通文件"
	}
	r, ok := locateQuote(f.contents[file.Blob], ref)
	if !ok {
		return r, ref.Path + " 中原文缺失或位置不唯一"
	}
	return r, ""
}

func reviewObject(note ledger.Note) string {
	if note.Target.At != "" {
		return units.Hash(note.Target.ID, note.Review.Kind, note.Target.At)
	}
	subject := note.Review.Subject
	return units.Hash(note.Target.ID, note.Review.Kind, subject.Path, subject.Quote, strconv.Itoa(subject.Line))
}

// 固定快照只读取本次解释引用的路径。同一观察点或 blob 被多处引用时只读取一次。
func reviewPurposes(ctx context.Context, store *ledger.Store, w gitstate.Workspace, view ledger.TaskView, a *analysis) error {
	if !slices.ContainsFunc(view.Notes, func(n ledger.Note) bool { return n.Review != nil }) {
		return nil
	}
	latest := map[string]ledger.Note{}
	paths := map[string]bool{}
	changed := map[string]bool{}
	for _, h := range a.Changes.Hunks {
		if h.Span.AfterStart < h.Span.AfterEnd {
			changed[h.Path] = true
		}
	}
	requirements := map[string]ledger.Requirement{}
	sources := map[string]ledger.Source{}
	for _, req := range view.Requirements {
		if req.Status != "revoked" {
			requirements[req.ID] = req
		}
	}
	for _, source := range view.Sources {
		sources[source.ID] = source
	}
	for _, note := range view.Notes {
		if note.Review != nil && note.Target != nil && requirements[note.Target.ID].ID != "" && changed[note.Review.Subject.Path] {
			latest[reviewObject(note)] = note
		}
	}
	if len(latest) == 0 {
		return nil
	}
	for _, note := range latest {
		paths[note.Review.Subject.Path] = true
		for _, ref := range note.Review.Evidence {
			paths[ref.Path] = true
		}
		if note.Citation != nil && sources[note.Citation.SourceID].Kind == "project_convention" {
			paths[sources[note.Citation.SourceID].Path] = true
		}
	}
	names := keys(paths)
	trees := map[string]map[string]gitstate.File{}
	filesAt := func(tree string) (map[string]gitstate.File, error) {
		if files, ok := trees[tree]; ok {
			return files, nil
		}
		files, err := w.TreeFiles(ctx, tree, names...)
		trees[tree] = files
		return files, err
	}
	var files purposeFiles
	var err error
	if files.current, err = filesAt(a.After.Tree); err != nil {
		return err
	}
	if files.origin, err = filesAt(a.Before.Tree); err != nil {
		return err
	}
	var blobs []string
	addBlob := func(file gitstate.File) {
		if regular(file) {
			blobs = append(blobs, file.Blob)
		}
	}
	for _, note := range latest {
		addBlob(files.current[note.Review.Subject.Path])
		for _, ref := range note.Review.Evidence {
			addBlob(files.current[ref.Path])
			addBlob(files.origin[ref.Path])
		}
	}
	if files.contents, err = w.ReadBlobs(ctx, blobs); err != nil {
		return err
	}
	observed := map[int64]string{}
	for _, id := range keys(latest) {
		note := latest[id]
		tree, ok := observed[note.ObservationID]
		if !ok {
			observation, err := store.ObservationMetadata(ctx, note.ObservationID)
			if err != nil {
				return err
			}
			if !observation.Complete || !observation.Full || observation.WorkspaceID != w.ID {
				return fmt.Errorf("用途说明 %s 缺少完整观察点", note.ID)
			}
			tree = observation.Tree
			observed[note.ObservationID] = tree
		}
		recorded, err := filesAt(tree)
		if err != nil {
			return err
		}
		candidate, gap := assessPurpose(note, requirements[note.Target.ID], sources, view, files, recorded, a)
		if gap != "" {
			candidate.Facts = append(candidate.Facts, "缺口："+gap)
			a.PurposeGaps = append(a.PurposeGaps, ledger.CheckGap{At: candidate.At, Candidate: candidate.ID, Operation: "purpose", Kind: "unknown", Reason: gap})
		}
		a.Purposes = append(a.Purposes, candidate)
		for i := range a.Signals.Candidates {
			item := &a.Signals.Candidates[i]
			if item.At == candidate.At && item.At != note.Review.Subject.Path && item.AgentOnly && item.BlockFact == "" && gap == "" {
				item.Facts = append(item.Facts, fmt.Sprintf("同位置的用途说明 %s（%s）：%s", note.ID, note.Target.ID, candidate.Advice))
			}
		}
	}
	return ctx.Err()
}

// 差异区间从 0 开始、右端不含；引用行从 1 开始、两端均含。
// 删除位于两行之间，只有引用同时包含这两行才算相交。
func (r quoteRange) overlaps(s changes.Span) bool {
	return s.AfterStart < r.lastLine && r.line <= s.AfterEnd
}

func changedClusters(delta changes.Result, path string, subject quoteRange) ([]changes.Cluster, bool) {
	touched := map[string]bool{}
	changedLines := false
	for _, h := range delta.Hunks {
		if h.Path == path && subject.overlaps(h.Span) {
			touched[h.ID] = true
			changedLines = changedLines || h.Span.AfterStart < h.Span.AfterEnd
		}
	}
	var result []changes.Cluster
	for _, cluster := range delta.Clusters {
		if slices.ContainsFunc(cluster.Hunks, func(id string) bool { return touched[id] }) {
			result = append(result, cluster)
		}
	}
	return result, changedLines
}

func preservedQuote(hunks []changes.Hunk, ref ledger.CodeQuote, current quoteRange, origin []byte) bool {
	ref.Line = current.line
	for _, h := range hunks {
		if h.Path != ref.Path {
			continue
		}
		s := h.Span
		if current.overlaps(s) {
			return false
		}
		if s.AfterEnd < current.line {
			ref.Line += (s.BeforeEnd - s.BeforeStart) - (s.AfterEnd - s.AfterStart)
		}
	}
	_, found := locateQuote(origin, ref)
	return found
}

func currentContract(c ledger.Citation, source ledger.Source, req ledger.Requirement, plan *ledger.PlanRecord, files purposeFiles) bool {
	switch source.Kind {
	case "user_message":
		return req.Citation != nil && req.Citation.SourceID == c.SourceID && req.Citation.Start <= c.Start && c.End <= req.Citation.End
	case "confirmed_plan":
		return plan != nil && plan.Confirmation != nil && plan.Confirmation.SourceID == c.SourceID && len(plan.Revocations) == 0
	case "project_convention":
		return source.Blob != "" && files.current[source.Path].Blob == source.Blob && files.origin[source.Path].Blob == source.Blob
	}
	return false
}

// 只报告能定位的结构联系。同名调用并不证明接收者、运行值或入口保证相同。
func quoteRelation(subject, basis units.File, pos, ref quoteRange) string {
	snippet := func(file units.File, r quoteRange) []units.Token {
		var tokens []units.Token
		for _, t := range file.Tokens {
			if t.Start >= r.start && t.End <= r.end {
				tokens = append(tokens, t)
			}
		}
		return tokens
	}
	decl := signals.EnclosingDeclaration(subject, pos.start, pos.end)
	other := signals.EnclosingDeclaration(basis, ref.start, ref.end)
	if subject.Path == basis.Path && decl.ID != "" && decl.ID == other.ID {
		return "两处均位于声明 " + decl.Name + " 中"
	}
	subjectTokens, basisTokens := snippet(subject, pos), snippet(basis, ref)
	mentions := func(tokens []units.Token, name string) bool {
		return name != "" && slices.ContainsFunc(tokens, func(t units.Token) bool { return t.Kind == "identifier" && t.Text == name })
	}
	if mentions(basisTokens, decl.Name) {
		return "依据片段引用了与对象声明同名的标识符 " + decl.Name
	}
	if mentions(subjectTokens, other.Name) {
		return "对象片段引用了与依据声明同名的标识符 " + other.Name
	}
	first, second := units.Calls(subjectTokens), units.Calls(basisTokens)
	for _, called := range keys(first) {
		if _, ok := second[called]; ok {
			return "两处都有同名调用形状 " + called
		}
	}
	return ""
}

func retainedPurpose(decisions []ledger.Note, a *analysis, path string, pos quoteRange, clusters []changes.Cluster) bool {
	for _, note := range decisions {
		target := note.Target
		if target == nil || target.Effect != "retain" {
			continue
		}
		at := target.At
		if at == path || at == fmt.Sprintf("%s:%d", path, pos.line) || slices.ContainsFunc(clusters, func(c changes.Cluster) bool { return c.ID == at }) {
			return true
		}
		for _, u := range a.After.Files[path].Units {
			if u.Start <= pos.start && pos.end <= u.End && (at == repostats.UnitKey(path, u) || strings.TrimPrefix(at, "test:") == path+":"+u.Name) {
				return true
			}
		}
		for _, c := range a.Signals.Cases {
			if c.Path == path && c.Start < pos.end && pos.start < c.End && (at == c.ID || at == "test:"+path+":"+c.Name) {
				return true
			}
		}
		for _, item := range a.Signals.External {
			if item.At != path || at != item.Object() && at != item.Kind+":"+item.Name {
				continue
			}
			if item.Kind == "file" || slices.ContainsFunc(a.After.Files[path].Tokens, func(t units.Token) bool {
				return t.Start < pos.end && pos.start < t.End && units.Literal(t) == item.Name
			}) {
				return true
			}
		}
	}
	return false
}

func assessPurpose(note ledger.Note, req ledger.Requirement, sources map[string]ledger.Source, view ledger.TaskView, files purposeFiles, recorded map[string]gitstate.File, a *analysis) (signals.Candidate, string) {
	review := note.Review
	subject := review.Subject
	object := reviewObject(note)
	c := signals.Candidate{Kind: "purpose", Object: object, ID: units.Hash("purpose", object), At: subject.Path, Benefit: 1, ReviewOnly: true,
		Facts:  []string{"需求 " + req.ID + "：" + req.Content, "Agent 用途说明 " + note.ID + "（" + review.Kind + " / " + review.Assessment + "）：" + note.Text, "登记片段 " + subject.Path + "：" + subject.Quote},
		Advice: "用途尚无法判断，保留当前行为"}
	position, missing := files.locate(subject, recorded)
	if missing != "" {
		return c, missing
	}
	c.At = fmt.Sprintf("%s:%d", subject.Path, position.line)
	c.Facts = append(c.Facts, "已在固定快照定位 "+c.At)
	if at := note.Target.At; at != "" {
		if at != c.At && at != subject.Path+":"+signals.EnclosingDeclaration(a.After.Files[subject.Path], position.start, position.end).Name {
			return c, "登记的声明或行号与对象片段不符"
		}
		c.At = at
	}
	clusters, changedLines := changedClusters(a.Changes, subject.Path, position)
	if !changedLines || len(clusters) == 0 {
		return c, "片段没有触及本次改动"
	}
	agent := true
	for _, cluster := range clusters {
		agent = agent && changes.ByAgent(cluster.Origin)
	}
	if !agent {
		c.Advice = "片段涉及工作区更新，只报告现状并保留当前内容"
		return c, ""
	}
	if retainedPurpose(view.EffectiveDecisions, a, subject.Path, position, clusters) {
		c.Advice = "沿用此处仍然有效的保留决定"
		return c, ""
	}
	matches := slices.ContainsFunc(view.History, func(old ledger.Requirement) bool {
		return old.ID == req.ID && old.Revision == note.ReviewedRevision && old.SameScope(req)
	})
	if !matches {
		return c, "用途说明对应的需求或原话已变化"
	}
	supported := false
	if citation := note.Citation; citation != nil {
		if !currentContract(*citation, sources[citation.SourceID], req, view.Plans.Effective, files) {
			return c, "原话或项目约定无法对应当前需求、有效方案或起点约定"
		}
		c.Facts = append(c.Facts, "已核对出处 "+citation.SourceID+"："+citation.Quote)
		supported = true
	}
	for _, ref := range review.Evidence {
		where, gap := files.locate(ref, recorded)
		if gap != "" {
			return c, gap
		}
		if ref.Path == subject.Path && where.start < position.end && position.start < where.end {
			return c, "依据与被核对片段重叠"
		}
		origin := files.origin[ref.Path]
		existed := regular(origin) && preservedQuote(a.Changes.Hunks, ref, where, files.contents[origin.Blob])
		basis := "未确认是起点保留的原文"
		if existed {
			basis = "任务起点也有这段原文"
		}
		c.Facts = append(c.Facts, fmt.Sprintf("%s:%d（%s）：%s", ref.Path, where.line, basis, ref.Quote))
		relation := quoteRelation(a.After.Files[subject.Path], a.Index.Current.load(ref.Path), position, where)
		if relation != "" {
			c.Facts = append(c.Facts, relation)
			supported = supported || existed
		} else {
			c.Facts = append(c.Facts, "尚未从代码核对到这段依据与对象之间的联系")
		}
	}
	if review.Assessment == "unknown" || !supported {
		return c, "尚缺能够说明当前用途的独立依据"
	}
	c.Facts = append(c.Facts, "出处和位置已核对；业务前提由 Agent 说明")
	if review.Assessment == "keep" {
		c.Advice = "按所引约定，若上述用途成立，应保留这处行为"
	} else {
		c.Advice = "在上述业务前提成立且必要行为仍可保留时，可按说明精简本次 Agent 改动"
	}
	return c, ""
}
