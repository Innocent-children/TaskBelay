package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
)

type abstractUse struct {
	at, kind                   string
	production, tests, guesses []string
	unknown                    []codescan.Use
	threshold                  int
}

type convention struct {
	total, similar int
	evidence       []string
}

func uniqueUses(uses []codescan.Use, locations bool) (production, tests, guesses []string) {
	for _, use := range uses {
		value := use.Value
		if locations {
			value = fmt.Sprintf("%s:%d:%d", use.File, use.Span.Start, use.Span.End)
		}
		if value == "" {
			value = use.Declaration
		}
		if use.Test {
			tests = append(tests, value)
		} else if use.Candidate {
			guesses = append(guesses, value)
		} else {
			production = append(production, value)
		}
	}
	for _, list := range []*[]string{&production, &tests, &guesses} {
		sort.Strings(*list)
		*list = slices.Compact(*list)
	}
	return
}

func abstractionUses(d codescan.Declaration) []abstractUse {
	if d.Usage == nil || d.Test {
		return nil
	}
	u := d.Usage
	var result []abstractUse
	add := func(at, kind string, uses, unknown []codescan.Use, threshold int, locations bool) {
		production, tests, guesses := uniqueUses(uses, locations)
		result = append(result, abstractUse{at, kind, production, tests, guesses, unknown, threshold})
	}
	if u.Abstract {
		add(d.Location(), "接口实现", u.Implementations, u.Unknown, 2, false)
	}
	if u.Generic {
		unknown := append([]codescan.Use(nil), u.Unknown...)
		if u.Inferred {
			unknown = append(unknown, codescan.Use{Reason: "存在推断用法，不能完整统计显式类型组合"})
		}
		add(d.Location(), "泛型组合", u.TypeArguments, unknown, 2, false)
	}
	if u.Error || u.Sentinel {
		add(d.Location(), "错误区分", u.ErrorChecks, u.Unknown, 1, true)
	}
	for _, slot := range append(append([]codescan.Slot(nil), u.Parameters...), u.Fields...) {
		if slot.Function {
			unknown := append(append([]codescan.Use(nil), u.Unknown...), slot.Unknown...)
			add(d.Location()+"."+slot.Name, "函数值", slot.Functions, unknown, 2, false)
		}
	}
	return result
}

func baselineConventions(before codescan.Snapshot) map[string]convention {
	result := map[string]convention{}
	for _, file := range before.Files {
		if excluded(file) || !file.CodeComplete {
			continue
		}
		for _, d := range file.Declarations {
			for _, usage := range abstractionUses(d) {
				key := file.Kind.Language + ":" + usage.kind
				pattern := result[key]
				pattern.total++
				if len(usage.unknown) == 0 && len(usage.production)+len(usage.guesses) < usage.threshold {
					pattern.similar++
					pattern.evidence = append(pattern.evidence, usage.at)
				}
				result[key] = pattern
			}
		}
	}
	for key, pattern := range result {
		sort.Strings(pattern.evidence)
		result[key] = pattern
	}
	return result
}

func (s *staticAnalysis) abstractions(d codescan.Declaration) {
	for _, use := range abstractionUses(d) {
		count := len(use.production) + len(use.guesses)
		if count >= use.threshold {
			s.scope("D4", use.at)
			continue
		}
		if len(use.unknown) > 0 {
			for _, unknown := range use.unknown {
				s.gap(use.at, "D4", unknown.Reason)
			}
			continue
		}
		s.scope("D4", use.at)
		fact := fmt.Sprintf("%s：生产用法 %d，候选 %d，测试用法 %d；建议核对的用法数为 %d。", use.kind, len(use.production), len(use.guesses), len(use.tests), use.threshold)
		evidence := append([]string(nil), use.production...)
		for _, candidate := range use.guesses {
			evidence = append(evidence, "候选："+candidate)
		}
		for _, test := range use.tests {
			evidence = append(evidence, "测试："+test)
		}
		issue := s.add("D4", use.at, fmt.Sprintf("%s:%d:%d", use.kind, len(use.production), len(use.guesses)), fact, "核对隔离 I/O、依赖方向、接口约定等当前用途；一种用法本身不说明抽象无价值。", evidence...)
		pattern := s.conventions[s.after.Files[d.File].Kind.Language+":"+use.kind]
		if pattern.total >= 3 && pattern.similar*2 >= pattern.total {
			issue.Status = "convention"
			issue.HandledBy = append(issue.HandledBy, fmt.Sprintf("E1：起点同类 %d 个，其中 %d 个采用相同写法", pattern.total, pattern.similar))
			issue.Evidence = append(issue.Evidence, pattern.evidence...)
		}
	}
}

func constantValues(slot codescan.Slot) []string {
	var result []string
	for _, value := range slot.Values {
		result = append(result, value.Kind+":"+value.Text)
	}
	sort.Strings(result)
	return slices.Compact(result)
}

func slotFact(slot codescan.Slot, parameter bool) string {
	if slot.Interface || slot.Public || slot.External || len(slot.Unknown) > 0 {
		return ""
	}
	if parameter && !slot.Read {
		return "unused-parameter"
	}
	values := constantValues(slot)
	if len(values) == 1 {
		return "constant:" + values[0]
	}
	return ""
}

func (s *staticAnalysis) slots(change codescan.Change) {
	d := *change.After
	if d.Usage == nil {
		return
	}
	if len(d.Usage.Unknown) > 0 {
		for _, unknown := range d.Usage.Unknown {
			s.gap(d.Location(), "D5", unknown.Reason)
		}
		return
	}
	for group, slots := range [][]codescan.Slot{d.Usage.Parameters, d.Usage.Fields} {
		parameter := group == 0
		old := map[string]codescan.Slot{}
		if change.Before != nil && change.Before.Usage != nil {
			previous := change.Before.Usage.Fields
			if parameter {
				previous = change.Before.Usage.Parameters
			}
			for _, slot := range previous {
				old[slot.Name] = slot
			}
		}
		for _, slot := range slots {
			at := d.Location() + "." + slot.Name
			if slot.Interface || slot.Public || slot.External {
				s.scope("D5", at)
				continue
			}
			if len(slot.Unknown) > 0 {
				for _, use := range slot.Unknown {
					s.gap(at, "D5", use.Reason)
				}
				continue
			}
			s.scope("D5", at)
			key := slotFact(slot, parameter)
			if key == "" {
				continue
			}
			if before, exists := old[slot.Name]; exists && slotFact(before, parameter) == key {
				continue
			}
			kind := "字段"
			if parameter {
				kind = "参数"
			}
			fact := "已核对的" + kind + "值为 " + strings.Join(constantValues(slot), ", ") + "。"
			if key == "unused-parameter" {
				fact = "这个参数在函数体内没有被读取。"
			}
			var evidence []string
			for _, value := range slot.Values {
				evidence = append(evidence, fmt.Sprintf("%s:%d %s:%s；隐式=%t", value.At.File, value.At.Span.StartLine, value.Kind, value.Text, value.Implicit))
			}
			s.add("D5", at, key, fact, "核对接口签名、公开调用和外部映射；准备内联或移除时同时核对调用约定与行为。", evidence...)
		}
	}
}

func (s *staticAnalysis) forwarder(d codescan.Declaration) {
	s.scope("D6", d.Location())
	if d.Shape == nil || d.Shape.Forwarder == "" {
		return
	}
	s.add("D6", d.Location(), "forward:"+d.Shape.Forwarder, "这个新增函数仅向 "+d.Shape.Forwarder+" 原样顺序转发参数。", "说明接口实现、适配第三方、稳定公开入口或统一命名等当前用途，再决定是否保留这一层。", d.Location())
}

func (s *staticAnalysis) compatibility(change codescan.Change) {
	d := *change.After
	s.scope("D7", d.Location())
	if d.Usage == nil {
		return
	}
	if d.Usage.Deprecated && (change.Before == nil || change.Before.Usage == nil || !change.Before.Usage.Deprecated) {
		s.add("D7", d.Location(), "deprecated", "本次新增了弃用标记。", "核对发布约定和现有调用方，再决定迁移还是保留兼容；工作区之外的调用方仍需核对。", d.Documentation)
	}
	oldName := change.Before != nil
	if !oldName {
		if file := s.before.Files[d.File]; file != nil {
			for _, prior := range file.Declarations {
				if prior.Name == d.Name && prior.Owner == d.Owner {
					oldName = true
				}
			}
		}
	}
	if oldName && d.Usage.Alias != "" && (change.Before == nil || change.Before.Usage == nil || change.Before.Usage.Alias != d.Usage.Alias) {
		s.add("D7", d.Location(), "alias:"+d.Usage.Alias, "保留起点旧名并改为 "+d.Usage.Alias+" 的别名。", "核对现有调用方和发布约定，说明兼容用途或一并迁移。", d.Location())
	}
	if change.Before != nil && d.Shape != nil && d.Shape.Forwarder != "" && (change.Before.Shape == nil || change.Before.Shape.Forwarder != d.Shape.Forwarder) {
		s.add("D7", d.Location(), "old-forward:"+d.Shape.Forwarder, "起点已有函数改为向 "+d.Shape.Forwarder+" 转发。", "核对旧调用方和发布约定，再选择保留兼容或迁移；不假定所有调用方都在工作区内。", change.Before.Location(), d.Location())
	}
}
