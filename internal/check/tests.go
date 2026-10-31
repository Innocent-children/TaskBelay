package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func validationNotes(view ledger.TaskView) []ledger.Note {
	var notes []ledger.Note
	for _, note := range view.Notes {
		if note.Kind == "decision" && note.InvalidatedBy == "" && note.Target != nil && (note.Target.Kind == "requirement" || note.Target.Kind == "verification" || note.Target.Kind == "issue") {
			notes = append(notes, note)
		}
	}
	return notes
}

func snapshotTests(snapshot codescan.Snapshot) []codescan.TestCase {
	var tests []codescan.TestCase
	for _, file := range snapshot.Files {
		tests = append(tests, file.Tests...)
	}
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].Location() != tests[j].Location() {
			return tests[i].Location() < tests[j].Location()
		}
		return tests[i].Span.Start < tests[j].Span.Start
	})
	return tests
}

func (s *staticAnalysis) tests() {
	old, current := snapshotTests(s.before), snapshotTests(s.after)
	oldByLocation := map[string][]codescan.TestCase{}
	oldByHash, currentByHash := map[string][]codescan.TestCase{}, map[string][]codescan.TestCase{}
	for _, test := range old {
		oldByLocation[test.Location()] = append(oldByLocation[test.Location()], test)
		oldByHash[test.Framework+"\x00"+test.CodeHash] = append(oldByHash[test.Framework+"\x00"+test.CodeHash], test)
	}
	for _, test := range current {
		currentByHash[test.Framework+"\x00"+test.CodeHash] = append(currentByHash[test.Framework+"\x00"+test.CodeHash], test)
	}
	declarations := s.after.Declarations()
	byName := map[string][]string{}
	productionLocations := map[string]bool{}
	for id, d := range declarations {
		byName[d.Name] = append(byName[d.Name], id)
		if !d.Test && !excluded(s.after.Files[d.File]) {
			productionLocations[d.Location()] = true
		}
	}
	complete := true
	needsTestInventory := false
	for _, change := range s.changes {
		d := change.After
		if d == nil {
			d = change.Before
		}
		if !d.Test && change.Detail != "format" && change.Detail != "comment" {
			needsTestInventory = true
			break
		}
	}
	for _, requirement := range s.view.Requirements {
		if requirement.Status == "revoked" {
			continue
		}
		for _, location := range requirement.ImplementedBy {
			if location.Pending == "" && productionLocations[location.At] {
				needsTestInventory = true
			}
		}
	}
	for _, snapshot := range []codescan.Snapshot{s.before, s.after} {
		for _, file := range snapshot.Files {
			if (file.Kind.Test || len(file.Tests) > 0) && !file.Kind.Generated && !file.Kind.Dependency && !file.Operations["tests"].Complete {
				complete = false
				if needsTestInventory || s.fileChanged(file.Path) {
					s.gap(file.Path, "tests", file.GapKind("tests"), file.Operations["tests"].Reason)
				}
			}
		}
	}
	for _, test := range current {
		if s.ctx.Err() != nil {
			return
		}
		change := "add"
		previous := oldByLocation[test.Location()]
		if len(previous) == 0 {
			key := test.Framework + "\x00" + test.CodeHash
			if len(oldByHash[key]) == 1 && len(currentByHash[key]) == 1 {
				previous = oldByHash[key]
			}
		}
		if len(previous) == 0 {
			previous = s.renamedTest(test, old)
		}
		if len(previous) == 1 {
			change = "modify"
			if previous[0].Location() == test.Location() && previous[0].CodeHash == test.CodeHash {
				change = "existing"
			}
		} else if len(previous) > 1 || len(test.Unknown) > 0 {
			change = "unknown"
		} else if file := s.before.Files[test.File]; file != nil && !file.Operations["tests"].Complete {
			change = "unknown"
		}
		fact := ledger.TestFact{At: test.Location(), Framework: test.Framework, Change: change, Line: test.Span.StartLine, Unknown: slices.Clone(test.Unknown)}
		if d, ok := declarations[test.Declaration]; ok {
			fact.Declaration = d.Location()
		}
		fact.Requirements = slices.Clone(s.association.Requirements[test.Location()])
		// 文件位置也可直接说明黑盒测试或删除入口验收的用途。
		fact.Requirements = append(fact.Requirements, s.association.Requirements[test.File]...)
		fact.RelatedCode = testRelated(s.ctx.Err, test, declarations, byName, s.references)
		spans := append([]codescan.Span{test.Span}, test.Parameters...)
		spans = append(spans, test.Context...)
		_, fact.Mixed, fact.Evidence = s.sources.at(test.File, spans)
		for _, scope := range spans {
			for line := scope.StartLine; line <= scope.EndLine && line <= len(s.sources[test.File].lines); line++ {
				if line > 0 && s.sources[test.File].lines[line-1].kind == "agent" {
					fact.AgentOnly = !fact.Mixed
				}
			}
		}
		for _, assertion := range test.Assertions {
			fact.Assertions = append(fact.Assertions, ledger.TestAssertion{Kind: assertion.Kind, Evidence: assertion.Evidence, Line: assertion.Span.StartLine})
		}
		fact.Retained, fact.Evidence = s.retainedTest(test, declarations, fact.Evidence)
		slices.Sort(fact.Requirements)
		fact.Requirements = slices.Compact(fact.Requirements)
		s.result.Report.Tests = append(s.result.Report.Tests, fact)
		s.scope("T4", fact.At)
		s.scope("T5", fact.At)
		if change != "add" || len(fact.Unknown) > 0 {
			continue
		}
		mockOnly := len(fact.Assertions) > 0
		for _, assertion := range fact.Assertions {
			mockOnly = mockOnly && assertion.Kind == "mock"
		}
		if mockOnly {
			issue := s.add("T5", fact.At, "direct_mock_assertions", "识别到的直接断言只有 mock 调用约束；间接帮助函数中的断言尚需核对。", "核对行为结果；若次数、顺序或参数本身就是需求，记录用途后保留。", assertionEvidence(fact)...)
			markTestRetention(issue, fact)
		}
		trivial := len(fact.RelatedCode) > 0
		for _, id := range fact.RelatedCode {
			d := declarations[id]
			trivial = trivial && d.Shape != nil && (d.Shape.SimpleKind == "field_accessor" || d.Shape.SimpleKind == "field_constructor")
		}
		if trivial {
			issue := s.add("T4", fact.At, "simple_declaration_candidates", "静态引用候选只指向字段取值、赋值或只填字段的构造函数，尚无实际覆盖数据。", "查看已有行为测试，以及默认值和公开约定是否需要单独守护；静态关系不能作为删除依据。", fact.RelatedCode...)
			markTestRetention(issue, fact)
		}
	}
	for _, requirement := range s.view.Requirements {
		if requirement.Status == "revoked" {
			continue
		}
		risk := s.testRisk(requirement)
		for _, test := range s.result.Report.Tests {
			if !slices.Contains(test.Requirements, requirement.ID) {
				continue
			}
			switch test.Change {
			case "add":
				risk.Added++
				if test.Retained {
					risk.Retained++
				}
			case "modify":
				risk.Modified++
			case "existing":
				risk.Existing++
			}
		}
		s.result.Report.TestRisk = append(s.result.Report.TestRisk, risk)
		if risk.Complete && complete {
			s.scope("T3", requirement.ID)
			s.scope("T6", requirement.ID)
		}
		if risk.Complete && risk.Added-risk.Retained > risk.Recommended {
			s.add("T3", requirement.ID, "risk_count", fmt.Sprintf("新增 %d 例，其中用户要求保留 %d 例；当前风险建议 %d 例。", risk.Added, risk.Retained, risk.Recommended), "建议数不是上限。逐条核对用途；安全默认值、配置和删除行为的回归用例有依据时保留。", riskEvidence(risk)...)
		}
		if risk.Logic && risk.Added+risk.Modified == 0 && !s.existingTestExplanation(requirement) {
			fact := "有实际逻辑的需求尚未关联新增或修改用例；静态关系不能确认覆盖缺失。"
			if !complete {
				fact += " 部分测试枚举未完成。"
			}
			s.add("T6", requirement.ID, "validation_basis", fact, "核对已有用例、黑盒测试及验证记录；可在需求位置或决定笔记说明验收依据，不要求为消除提示另写测试。", riskEvidence(risk)...)
		}
	}
	if complete {
		for _, rule := range []string{"T3", "T4", "T5", "T6"} {
			if rule != "T3" && rule != "T6" {
				s.scope(rule, "*")
			}
		}
	}
}

func (s *staticAnalysis) renamedTest(test codescan.TestCase, old []codescan.TestCase) []codescan.TestCase {
	for _, change := range s.changes {
		if change.Before == nil || change.After == nil || change.AmbiguousLocation || change.After.ID != test.Declaration {
			continue
		}
		name := strings.Replace(test.Name, change.After.Name, change.Before.Name, 1)
		var matches []codescan.TestCase
		for _, previous := range old {
			if previous.Declaration == change.Before.ID && previous.Name == name {
				matches = append(matches, previous)
			}
		}
		return matches
	}
	return nil
}

func testRelated(canceled func() error, test codescan.TestCase, declarations map[string]codescan.Declaration, byName map[string][]string, references codescan.References) []string {
	var queue, result []string
	for _, name := range test.References {
		queue = append(queue, byName[name]...)
	}
	seen := map[string]bool{}
	for len(queue) > 0 && canceled() == nil {
		id := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		d := declarations[id]
		if !d.Test && d.Shape != nil {
			result = append(result, id)
		}
		queue = append(queue, references.Forward[id]...)
	}
	slices.Sort(result)
	return result
}

func (s *staticAnalysis) retainedTest(test codescan.TestCase, declarations map[string]codescan.Declaration, evidence []string) (bool, []string) {
	positions := []string{test.Location()}
	if d, ok := declarations[test.Declaration]; ok {
		positions = append(positions, d.Location(), "test:"+d.Location())
	}
	retained := false
	for _, at := range positions {
		if yes, proof := s.explicit(at, test.File, true); yes {
			retained = true
			evidence = append(evidence, proof...)
		}
		for _, note := range s.view.EffectiveDecisions {
			if note.Target != nil && note.Target.At == at && note.Target.Effect == "retain" {
				retained = true
				evidence = append(evidence, note.ID+": "+note.Text)
			}
		}
	}
	return retained, evidence
}

func markTestRetention(issue *ledger.Issue, test ledger.TestFact) {
	if test.Retained {
		issue.Status = "retained"
		issue.HandledBy = append(issue.HandledBy, test.Evidence...)
	}
	if test.Mixed {
		issue.Facts = append(issue.Facts, "用例含工作区更新或未知来源，不能列入整例删除候选。")
	}
}

func assertionEvidence(test ledger.TestFact) []string {
	result := slices.Clone(test.Evidence)
	for _, assertion := range test.Assertions {
		result = append(result, fmt.Sprintf("第 %d 行 %s", assertion.Line, assertion.Evidence))
	}
	return result
}

func (s *staticAnalysis) existingTestExplanation(requirement ledger.Requirement) bool {
	for _, location := range requirement.ImplementedBy {
		if location.Pending != "" || strings.TrimSpace(location.Purpose) == "" {
			continue
		}
		for _, test := range s.result.Report.Tests {
			if (test.At == location.At || test.Declaration == location.At || "test:"+test.Declaration == location.At) && len(test.Unknown) == 0 {
				return true
			}
		}
	}
	return false
}

func riskEvidence(risk ledger.TestRisk) []string {
	result := slices.Clone(risk.Evidence)
	for _, source := range risk.Sources {
		result = append(result, fmt.Sprintf("%s %s:%d %s", source.Kind, source.At, source.Line, source.Evidence))
	}
	return result
}
