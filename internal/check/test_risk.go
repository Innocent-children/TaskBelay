package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func (s *staticAnalysis) testRisk(requirement ledger.Requirement) ledger.TestRisk {
	result := ledger.TestRisk{Requirement: requirement.ID, Complete: true}
	changes := map[string]codescan.Change{}
	for _, change := range s.changes {
		d := change.After
		if d == nil {
			d = change.Before
		}
		if !d.Test && slices.Contains(s.association.Requirements[d.ID], requirement.ID) {
			changes[d.ID] = change
		}
	}
	// 专门要求给未修改声明补测试时，按整个声明核对风险。
	for _, location := range requirement.ImplementedBy {
		if location.Pending != "" {
			continue
		}
		for id, d := range s.after.Declarations() {
			if d.Location() != location.At || d.Test {
				continue
			}
			if _, exists := changes[id]; !exists {
				copy := d
				changes[id] = codescan.Change{After: &copy, Kind: "named", Detail: "behavior"}
			}
		}
	}
	var ids []string
	for id := range changes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	categories, sources := map[string]bool{}, map[string]bool{}
	for _, id := range ids {
		if s.ctx.Err() != nil {
			result.Complete = false
			break
		}
		change := changes[id]
		if slices.Contains([]string{"format", "comment", "names"}, change.Detail) {
			result.Evidence = append(result.Evidence, change.Location()+"：仅格式、注释或名字变化，通常不额外要求测试")
			continue
		}
		var oldLines, newLines map[int]bool
		if change.Before != nil && change.After != nil {
			var known bool
			oldLines, newLines, known = codescan.BehaviorLines(s.ctx, *change.Before, *change.After)
			if !known {
				result.Complete = false
				s.gap(change.Location(), "test_risk", "incomplete", "行为变化行比较超过预算或被取消")
				continue
			}
		}
		branchCount := 0
		for side, d := range []*codescan.Declaration{change.Before, change.After} {
			if d == nil {
				continue
			}
			snapshot, lines := s.before, oldLines
			if side == 1 {
				snapshot, lines = s.after, newLines
			}
			file := snapshot.Files[d.File]
			if excluded(file) || d.Test {
				continue
			}
			if !file.CodeComplete || !file.Operations["risks"].Complete {
				result.Complete = false
				s.gap(d.Location(), "test_risk", file.GapKind("declarations", "risks"), "声明语法或风险调用尚未完整核对")
			}
			if d.Shape != nil && d.Shape.SimpleKind == "" {
				result.Logic = true
			}
			if d.Shape != nil {
				count := 0
				for _, branch := range d.Shape.Branches {
					if touchesLines(branch, lines) {
						count++
					}
				}
				branchCount = max(branchCount, count)
			}
			for _, risk := range file.Risks {
				if risk.Declaration != d.ID || !touchesLines(risk.Span, lines) {
					continue
				}
				if !slices.Contains([]string{"concurrency", "write", "input", "command"}, risk.Kind) {
					continue
				}
				categories[risk.Kind] = true
				key := risk.Kind + "\x00" + d.Location() + "\x00" + risk.Evidence
				if !sources[key] {
					evidence := risk.Evidence
					if change.After == nil {
						evidence = "删除前：" + evidence
					}
					result.Sources = append(result.Sources, ledger.TestRiskSource{Kind: risk.Kind, At: d.Location(), Line: risk.Span.StartLine, Evidence: evidence})
					sources[key] = true
				}
			}
		}
		result.Branches += branchCount
		if branchCount > 0 {
			result.Evidence = append(result.Evidence, fmt.Sprintf("%s：行为范围内 %d 个分支点", change.Location(), branchCount))
		}
	}
	result.Logic = result.Logic || len(categories) > 0 || result.Branches > 0
	if result.Logic {
		result.Recommended = 1
		for category := range categories {
			if category == "command" {
				result.Recommended++
			} else {
				result.Recommended += 2
			}
		}
		result.Recommended = min(8, result.Recommended+result.Branches/4)
	} else {
		result.Evidence = append(result.Evidence, "当前可定位范围只有简单取值或非行为变化，通常不额外增加测试；有明确回归目的的用例仍可保留")
	}
	for _, location := range requirement.ImplementedBy {
		if strings.TrimSpace(location.Purpose) != "" {
			result.Evidence = append(result.Evidence, location.At+"："+location.Purpose)
		}
	}
	sort.Slice(result.Sources, func(i, j int) bool {
		a, b := result.Sources[i], result.Sources[j]
		return a.Kind+"\x00"+a.At+"\x00"+a.Evidence < b.Kind+"\x00"+b.At+"\x00"+b.Evidence
	})
	return result
}

func touchesLines(scope codescan.Span, lines map[int]bool) bool {
	if lines == nil {
		return true
	}
	for line := scope.StartLine; line <= scope.EndLine; line++ {
		if lines[line] {
			return true
		}
	}
	return false
}
