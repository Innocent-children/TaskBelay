package check

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/lang"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

const RulesVersion = "D1-D8-E1-S1-S2-T3-T6-v3"
const CapabilitiesVersion = "gotreesitter-0.56.0-facts-tests-v3"

type staticAnalysis struct {
	ctx         context.Context
	before      codescan.Snapshot
	after       codescan.Snapshot
	view        ledger.TaskView
	changes     []codescan.Change
	association codescan.Association
	sources     provenance
	result      ledger.CheckRecord
	nameUses    map[string][]string
	references  codescan.References
	conventions map[string]convention
	evaluated   map[string]bool
	missing     map[ledger.CheckGap]struct{}
	gapGroups   map[[2]string]*gapGroup
}

type gapGroup struct {
	count        int
	summaryIndex int
}

func excluded(file *codescan.File) bool {
	return file == nil || file.Kind.Generated || file.Kind.Dependency || file.Kind.Lock || file.Kind.Test || file.Kind.TestData
}

func staticCheck(ctx context.Context, input ledger.CheckInput, view ledger.TaskView, before, after codescan.Snapshot, changes []codescan.Change, association codescan.Association, sources provenance) (ledger.CheckRecord, error) {
	s := staticAnalysis{ctx: ctx, before: before, after: after, view: view, changes: changes, association: association, sources: sources,
		result: ledger.CheckRecord{Input: input, InputVersion: input.Version(), Depth: "static", Status: ledger.CheckComplete, Issues: []ledger.Issue{}, Missing: []ledger.CheckGap{}}, evaluated: map[string]bool{}}
	s.result.Report = report(view, changes)
	s.result.Report.ChangesByRequirement = map[string][]string{}
	for _, change := range changes {
		d := change.After
		if d == nil {
			d = change.Before
		}
		for _, id := range association.Requirements[d.ID] {
			s.result.Report.ChangesByRequirement[id] = append(s.result.Report.ChangesByRequirement[id], change.Kind+" "+change.Location())
		}
	}
	for _, note := range view.EffectiveDecisions {
		if note.Target != nil && note.Target.Effect == "pause_checks" {
			s.result.Status = ledger.CheckPartial
			s.result.Report.NotRun = append(s.result.Report.NotRun, "用户决定暂停检查："+note.ID)
			return s.result, nil
		}
	}
	var err error
	s.references, err = codescan.IndexReferences(ctx, after.Declarations())
	if err != nil {
		return s.result, err
	}
	s.nameUses = namedUses(after)
	s.conventions = baselineConventions(before)
	for _, snapshot := range []codescan.Snapshot{before, after} {
		for path, file := range snapshot.Files {
			if excluded(file) {
				continue
			}
			for _, missing := range file.Missing {
				if err := ctx.Err(); err != nil {
					return s.result, err
				}
				if missing.Operation != "tests" && missing.Operation != "risks" {
					s.gap(path, missing.Operation, missing.Reason)
				}
			}
		}
	}
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return s.result, err
		}
		d := change.After
		snapshot := after
		if d == nil {
			d, snapshot = change.Before, before
		}
		if d.Test || excluded(snapshot.Files[d.File]) {
			continue
		}
		if change.After != nil {
			s.compatibility(change)
		}
		if change.Detail == "format" {
			continue
		}
		s.requirementChange(change)
		if change.After == nil || change.Detail == "comment" {
			continue
		}
		if change.Kind == "add" {
			s.unused(*d)
			s.abstractions(*d)
			s.forwarder(*d)
		}
		s.slots(change)
	}
	s.entries()
	s.dependencies()
	s.otherFiles()
	s.tests()
	for rule, operation := range map[string]string{"D3": "entries", "D8": "dependencies"} {
		complete := true
		for _, snapshot := range []codescan.Snapshot{before, after} {
			for path, file := range snapshot.Files {
				if !excluded(file) && (rule == "D3" && file.Kind.Source || rule == "D8" && lang.Manifest(path)) && !file.Operations[operation].Complete {
					complete = false
				}
			}
		}
		if complete {
			s.scope(rule, "*")
		}
	}
	for scope := range s.evaluated {
		s.result.Evaluated = append(s.result.Evaluated, scope)
	}
	sort.Strings(s.result.Evaluated)
	sort.Slice(s.result.Issues, func(i, j int) bool {
		a, b := s.result.Issues[i], s.result.Issues[j]
		return a.Rule+"\x00"+a.At+"\x00"+a.FactKey < b.Rule+"\x00"+b.At+"\x00"+b.FactKey
	})
	sort.Slice(s.result.Missing, func(i, j int) bool {
		a, b := s.result.Missing[i], s.result.Missing[j]
		if a.At != b.At {
			return a.At < b.At
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		return a.Reason < b.Reason
	})
	s.result.Level = ledger.HighestLevel(s.result.Issues)
	if view.Task != nil && view.Task.State == ledger.TaskPending && view.Settings.Values["enforcement"].Value != "hint" {
		s.result = ledger.WithDelivery(s.result, true)
	}
	return s.result, ctx.Err()
}

func report(view ledger.TaskView, changes []codescan.Change) ledger.DeliveryReport {
	result := ledger.DeliveryReport{Plans: view.Plans, Verifications: view.Verifications, Decisions: view.EffectiveDecisions, ValidationNotes: validationNotes(view), WorkspaceUpdates: view.WorkspaceUpdates, NotRun: []string{"动态测试未运行"}}
	for _, req := range view.Requirements {
		if req.Status == "revoked" {
			result.Revoked = append(result.Revoked, req)
		} else {
			result.Requirements = append(result.Requirements, req)
			if req.Status != "completed" {
				result.Unfinished = append(result.Unfinished, req)
			}
		}
	}
	for _, change := range changes {
		result.Changes = append(result.Changes, change.Kind+" "+change.Location())
	}
	return result
}

func (s *staticAnalysis) scope(rule, at string) {
	s.evaluated[ledger.CheckScope(rule, at)] = true
}

func (s *staticAnalysis) gap(at, operation, reason string) {
	gap := ledger.CheckGap{At: at, Operation: operation, Reason: reason}
	if s.missing == nil {
		s.missing = make(map[ledger.CheckGap]struct{})
		s.gapGroups = make(map[[2]string]*gapGroup)
	}
	if _, exists := s.missing[gap]; exists {
		return
	}
	s.missing[gap] = struct{}{}
	key := [2]string{at, operation}
	group := s.gapGroups[key]
	if group == nil {
		group = &gapGroup{}
		s.gapGroups[key] = group
	}
	group.count++
	if group.count <= 5 {
		s.result.Missing = append(s.result.Missing, gap)
	} else {
		// 保留具体原因和未完成数量，避免大量候选引用耗尽报告写入预算。
		if group.count == 6 {
			group.summaryIndex = len(s.result.Missing)
			s.result.Missing = append(s.result.Missing, gap)
		}
		s.result.Missing[group.summaryIndex].Reason = fmt.Sprintf("另有 %d 项未完成；已保留前 5 项原因，当前范围仍未完整核对", group.count-5)
	}
	s.result.Status = ledger.CheckPartial
}

func (s *staticAnalysis) add(rule, at, key, fact, advice string, evidence ...string) *ledger.Issue {
	s.scope(rule, at)
	issue := ledger.Issue{Rule: rule, Level: "hint", At: at, FactKey: key, Facts: []string{fact}, Advice: advice, Evidence: evidence, Status: "open"}
	s.result.Issues = append(s.result.Issues, issue)
	return &s.result.Issues[len(s.result.Issues)-1]
}

func (s *staticAnalysis) explicit(at, file string, sourced bool) (bool, []string) {
	var evidence []string
	for _, req := range s.view.Requirements {
		if req.Status == "revoked" {
			continue
		}
		for _, location := range req.ImplementedBy {
			if location.Pending != "" || (location.At != at && (sourced || location.At != file)) {
				continue
			}
			if sourced && (strings.TrimSpace(location.Purpose) == "" || !s.requestCitation(location.Citation, at) && !s.requestCitation(req.Citation, at)) {
				continue
			}
			evidence = append(evidence, req.ID+" "+location.At+" "+location.Purpose)
		}
	}
	return len(evidence) > 0, evidence
}

func (s *staticAnalysis) requestCitation(citation *ledger.Citation, at string) bool {
	if citation == nil {
		return false
	}
	for _, source := range s.view.Sources {
		if source.ID != citation.SourceID {
			continue
		}
		if source.Kind == "user_message" {
			return true
		}
		if source.Kind == "confirmed_plan" && s.view.Plans.Effective != nil {
			for _, entry := range s.view.Plans.Effective.Entries {
				if entry.At == at {
					return true
				}
			}
		}
	}
	return false
}

func (s *staticAnalysis) requirementChange(change codescan.Change) {
	d := change.After
	if d == nil {
		d = change.Before
	}
	at := d.Location()
	s.scope("D2", at)
	if found, _ := s.explicit(at, d.File, false); found {
		return
	}
	fact := "累计" + change.Kind + "尚未关联有效需求位置。"
	if candidates := s.association.Requirements[d.ID]; len(candidates) > 0 {
		return
	}
	s.add("D2", at, "requirement:"+change.Kind, fact, "补充原需求的位置或当前用途；没有用途时只精简能核对为自己的编辑。")
}

func (s *staticAnalysis) unused(d codescan.Declaration) {
	at := d.Location()
	s.scope("D1", at)
	if found, _ := s.explicit(at, d.File, false); found {
		return
	}
	decls := s.after.Declarations()
	locationCount := 0
	for _, candidate := range decls {
		if candidate.Location() == at {
			locationCount++
		}
	}
	for _, id := range s.references.Backward[d.ID] {
		if other := decls[id]; !other.Test && !excluded(s.after.Files[other.File]) {
			return
		}
	}
	for _, file := range s.after.Files {
		if !excluded(file) && slices.Contains(file.References, d.Name) {
			return
		}
	}
	if len(s.nameUses[d.Name]) > 0 {
		return
	}
	if d.Usage != nil && len(d.Usage.Implicit) > 0 {
		return
	}
	for _, file := range s.after.Files {
		for _, entry := range file.Entries {
			if entry.Declaration == d.ID {
				return
			}
		}
	}
	agent, mixed, evidence := s.sources.at(d.File, d.Ranges())
	complete := d.Usage != nil && len(d.Usage.Unknown) == 0 && locationCount == 1
	for _, file := range s.after.Files {
		if !excluded(file) && file.Kind.Source && (!file.CodeComplete || !file.Operations["references"].Complete || !file.Operations["usage"].Complete || !file.Operations["entries"].Complete) {
			complete = false
		}
	}
	issue := s.add("D1", at, "unused", "在已核对范围内未找到这个新增声明的非测试使用。", "核对当前需求、公开 API 或间接调用；仅在来源和用法均明确时精简自己的声明。", evidence...)
	if agent && !mixed && complete && !d.Usage.Public {
		issue.BlockOnDelivery = true
	} else {
		issue.Facts = append(issue.Facts, "公开范围、间接用法、来源或混合更新尚不满足确定性拦截条件。")
		if locationCount != 1 {
			issue.Facts = append(issue.Facts, "这个位置包含多个同名声明，无法唯一核对。")
		}
	}
}

func namedUses(snapshot codescan.Snapshot) map[string][]string {
	result := map[string][]string{}
	add := func(text, at string) {
		for _, word := range strings.FieldsFunc(text, func(r rune) bool { return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			result[word] = append(result[word], at)
		}
	}
	for path, file := range snapshot.Files {
		if excluded(file) || !utf8.Valid(file.Data) || bytes.ContainsRune(file.Data, 0) {
			continue
		}
		if !file.Kind.Source {
			add(string(file.Data), path)
			continue
		}
		for _, literal := range file.Literals {
			add(literal, path)
		}
		for _, d := range file.Declarations {
			if d.Test {
				continue
			}
			for _, token := range d.Tokens {
				if token.Kind == "literal" {
					add(token.Text, d.Location())
				}
			}
		}
	}
	return result
}

func (s *staticAnalysis) entries() {
	old := map[string]bool{}
	for _, file := range s.before.Files {
		for _, entry := range file.Entries {
			old[entry.Location()] = true
		}
	}
	decls := s.after.Declarations()
	for path, file := range s.after.Files {
		if excluded(file) {
			continue
		}
		for _, entry := range file.Entries {
			at := entry.Location()
			s.scope("D3", at)
			if old[at] {
				continue
			}
			if found, _ := s.explicit(at, "", true); found {
				continue
			}
			if effective := s.view.Plans.Effective; effective != nil {
				authorized := false
				for _, planned := range effective.Entries {
					authorized = authorized || planned.At == at && strings.TrimSpace(planned.Note) != ""
				}
				if authorized {
					continue
				}
			}
			agent, mixed, evidence := s.sources.at(path, []codescan.Span{entry.Span})
			if d, ok := decls[entry.Declaration]; ok {
				_, wholeMixed, _ := s.sources.at(path, d.Ranges())
				mixed = mixed || wholeMixed
			}
			evidence = append(evidence, path+": "+entry.Evidence)
			issue := s.add("D3", at, "entry:"+at, "新增对外入口尚未列出有效需求或方案来源及当前用途。", "属于当前需求时补入口位置、用途和来源；独立行为先由用户决定；仅精简可核对的 Agent 编辑。", evidence...)
			complete := file.CodeComplete && file.Operations["entries"].Complete
			for _, prior := range s.before.Files {
				if !excluded(prior) && prior.Kind.Language == file.Kind.Language && prior.Kind.Source && !prior.Operations["entries"].Complete {
					complete = false
				}
			}
			if agent && !mixed && complete {
				issue.BlockOnDelivery = true
			}
		}
	}
}

func (s *staticAnalysis) dependencies() {
	var old, current []codescan.Dependency
	var existing []string
	for _, file := range s.before.Files {
		old = append(old, file.Dependencies...)
		for _, dep := range file.Dependencies {
			existing = append(existing, dep.Name)
		}
	}
	for _, file := range s.after.Files {
		current = append(current, file.Dependencies...)
	}
	sort.Strings(existing)
	existing = slices.Compact(existing)
	for _, dep := range codescan.AddedDependencies(old, current) {
		at := "dep:" + dep.Name
		s.scope("D8", at)
		if oldFile := s.before.Files[dep.Manifest]; oldFile != nil && !oldFile.Operations["dependencies"].Complete {
			s.gap(dep.Manifest, "D8", "起点清单解析不完整，不能确认新增依赖")
			continue
		}
		_, evidence := s.explicit(at, dep.Manifest, false)
		evidence = append(evidence, dep.Manifest)
		fact := "新增直接依赖 " + dep.Name + "。"
		if len(existing) > 0 {
			fact += "起点已有依赖：" + strings.Join(existing, ", ") + "。"
		}
		s.add("D8", at, "dependency:"+dep.Manifest+":"+dep.Name, fact, "说明当前用途，并核对标准库、已有实现和已有依赖是否可满足；半自动授权由 S2 检查。", evidence...)
	}
}

func (s *staticAnalysis) otherFiles() {
	paths := map[string]bool{}
	for path := range s.before.Files {
		paths[path] = true
	}
	for path := range s.after.Files {
		paths[path] = true
	}
	for path := range paths {
		old, current := s.before.Files[path], s.after.Files[path]
		file := current
		kind := "modify"
		if file == nil {
			file, kind = old, "delete"
		} else if old == nil {
			kind = "add"
		}
		if excluded(file) || file.Kind.Source && file.CodeComplete || old != nil && current != nil && old.Blob == current.Blob && old.Mode == current.Mode {
			continue
		}
		s.result.Report.Changes = append(s.result.Report.Changes, kind+" "+path)
		s.scope("D2", path)
		if found, _ := s.explicit(path, path, false); found {
			continue
		}
		if lang.Manifest(path) && len(file.Dependencies) > 0 {
			continue
		}
		s.add("D2", path, "file:"+kind, fmt.Sprintf("文件%s尚未关联需求；没有完整代码级结果的文件只按文件核对。", kind), "补充这个文件与当前需求的关系；未知来源保留当前内容。", file.Blob)
	}
}
