package codescan

import (
	"encoding/json"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (a *jsAnalysis) defaults() {
	for _, link := range a.fieldLinks {
		fields := a.matchingFields(link.item.d.ID, link.name, map[string]bool{})
		if len(fields) == 1 {
			field := fields[0]
			slot := &field.item.d.Usage.Fields[field.index]
			parameter := &link.method.d.Usage.Parameters[link.parameter]
			slot.Function = slot.Function || parameter.Function
			parameter.Function = parameter.Function || slot.Function
		}
	}
	for _, item := range a.declarations {
		for _, parameter := range item.parameters {
			if parameter.defaultValue != nil {
				for _, index := range parameter.slots {
					a.assign(item, &item.d.Usage.Parameters[index], item.p, parameter.defaultValue, true)
				}
			}
		}
	}
	for _, write := range a.parameterWrites {
		a.assign(write.item, &write.item.d.Usage.Parameters[write.index], write.file, write.node, false)
	}
}

func (a *jsAnalysis) callUnknown(item *jsDeclaration, p *jsFile, n *ts.Node, reason string) {
	if item.class {
		for _, constructor := range a.methods(item.d.ID, "constructor", map[string]bool{}) {
			a.callUnknown(constructor, p, n, reason)
		}
		return
	}
	a.unknown(item, nil, p, n, reason)
	if item.d.Usage.Generic {
		item.d.Usage.Inferred = true
	}
	for i := range item.d.Usage.Parameters {
		a.unknown(item, &item.d.Usage.Parameters[i], p, n, reason)
	}
}

func (a *jsAnalysis) calls(p *jsFile, n *ts.Node) {
	if slices.Contains([]string{"identifier", "this", "member_expression", "subscript_expression", "new_expression"}, p.kind(n)) {
		parent := p.parent(n)
		if slices.Contains([]string{"return_statement", "yield_expression", "pair", "array", "spread_element", "assignment_expression"}, p.kind(parent)) {
			if left := p.field(parent, "left"); left == nil || n.StartByte() >= left.EndByte() {
				if ref := p.reference(n, 0); ref.typ != "" {
					a.objectEscape(p, n, ref.typ, map[string]bool{}, "对象流入返回值或其他容器，后续别名写入不能完整核对")
				}
			}
		}
	}
	switch p.kind(n) {
	case "identifier", "member_expression", "subscript_expression":
		a.functionEscape(p, n)
	case "call_expression", "new_expression":
		callee := p.field(n, "function")
		if callee == nil {
			callee = p.field(n, "constructor")
		}
		args := p.arguments(n)
		targets := a.callable(p, callee)
		if len(targets) == 1 {
			target := targets[0]
			use := p.use(n, target.d.ID)
			use.Candidate = false
			addUse(&target.d.Usage.References, use)
			if target.class {
				constructors := a.methods(target.d.ID, "constructor", map[string]bool{})
				if len(constructors) == 1 {
					a.callArguments(constructors[0], p, n, args)
				} else if len(constructors) > 1 {
					a.callUnknown(target, p, n, "构造签名不能唯一核对")
				}
			} else if target.function != nil {
				a.callArguments(target, p, n, args)
			}
		} else if len(targets) > 1 {
			for _, target := range targets {
				a.callUnknown(target, p, n, "调用签名有多个候选")
			}
		} else if ref := p.reference(callee, 0); ref.qualified == "" || strings.Contains(ref.qualified, ":") {
			name := p.text(callee)
			if p.kind(callee) == "member_expression" {
				name = p.propertyName(p.field(callee, "property"))
			}
			for _, target := range a.byName[name] {
				if target.function != nil || target.class {
					a.callUnknown(target, p, n, "同名调用的接收者、遮蔽或重绑定不能核对")
				}
			}
		}
		for _, arg := range args {
			if ref := p.reference(arg, 0); ref.typ != "" {
				a.objectEscape(p, n, ref.typ, map[string]bool{}, "对象作为实参传递，别名或间接写入不能完整核对")
			}
		}
	case "variable_declarator":
		if p.kind(p.field(n, "name")) != "object_pattern" {
			return
		}
		owner := p.reference(p.field(n, "value"), 0).typ
		if owner == "" {
			return
		}
		pattern := p.field(n, "name")
		for i := 0; i < pattern.ChildCount(); i++ {
			child := pattern.Child(i)
			if !child.IsNamed() {
				continue
			}
			name := p.propertyName(child)
			if p.kind(child) == "pair_pattern" {
				name = p.propertyName(p.field(child, "key"))
			}
			if name == "" {
				a.objectEscape(p, n, owner, map[string]bool{}, "对象解构含有不能逐项核对的字段")
				continue
			}
			for _, field := range a.matchingFields(owner, name, map[string]bool{}) {
				field.item.d.Usage.Fields[field.index].Read = true
			}
		}
	}
}

func (a *jsAnalysis) callArguments(item *jsDeclaration, p *jsFile, call *ts.Node, args []*ts.Node) {
	for _, arg := range args {
		if p.kind(arg) == "spread_element" {
			a.callUnknown(item, p, call, "调用含展开实参，完整值集合不能核对")
			return
		}
	}
	position := 0
	for _, parameter := range item.parameters {
		if parameter.variadic {
			for _, index := range parameter.slots {
				a.variadicValues(item, &item.d.Usage.Parameters[index], p, call, args[min(position, len(args)):])
			}
			position = len(args)
			continue
		}
		var arg *ts.Node
		if position < len(args) {
			arg = args[position]
		}
		position++
		for _, index := range parameter.slots {
			slot := &item.d.Usage.Parameters[index]
			if len(parameter.slots) != 1 || !slices.Contains([]string{"identifier", "undefined"}, item.p.kind(parameter.pattern)) {
				a.unknown(item, slot, p, call, "解构参数的实参不能逐个绑定到字段或元素")
				continue
			}
			if arg == nil {
				if parameter.defaultValue == nil {
					a.undefined(item, slot, p, call, true)
				}
				continue
			}
			if value, ok := p.literal(arg, 0); ok && value.Kind == "undefined" && parameter.defaultValue != nil {
				continue
			}
			a.assign(item, slot, p, arg, false)
			annotation := item.p.parent(parameter.pattern)
			if item.p.kind(annotation) == "required_parameter" || item.p.kind(annotation) == "optional_parameter" {
				a.objectImplementation(p, arg, item.p.typeName(item.p.field(annotation, "type"), 0))
			}
		}
	}
	if position < len(args) {
		a.callUnknown(item, p, call, "调用带有签名之外的实参，arguments 等间接用法不能完整核对")
	}
}

func (a *jsAnalysis) variadicValues(item *jsDeclaration, slot *Slot, p *jsFile, call *ts.Node, args []*ts.Node) {
	type element struct{ Kind, Text string }
	values := make([]element, 0, len(args))
	for _, arg := range args {
		value, ok := p.literal(arg, 0)
		if !ok {
			a.unknown(item, slot, p, arg, "可变参数包含不能静态核对的值")
			return
		}
		values = append(values, element{value.Kind, value.Text})
	}
	encoded, _ := json.Marshal(values)
	value := Value{Kind: "Sequence", Text: string(encoded), At: p.use(call, slot.Name)}
	if !slices.Contains(slot.Values, value) {
		slot.Values = append(slot.Values, value)
	}
}

func (p *jsFile) stableAlias(n *ts.Node) bool {
	parent := p.parent(n)
	if p.kind(parent) != "variable_declarator" || p.field(parent, "value") != n {
		return false
	}
	name := p.text(p.field(parent, "name"))
	bindings := p.bindings[p.key(p.scope(parent, p.kind(p.parent(parent)) == "variable_declaration"))][name]
	return len(bindings) == 1 && bindings[0].value == n && bindings[0].constant
}

func (a *jsAnalysis) functionEscape(p *jsFile, n *ts.Node) {
	parent := p.parent(n)
	for slices.Contains([]string{"parenthesized_expression", "instantiation_expression", "as_expression", "non_null_expression"}, p.kind(parent)) {
		n = parent
		parent = p.parent(n)
	}
	if parent == nil {
		return
	}
	if p.kind(parent) == "member_expression" || p.kind(parent) == "subscript_expression" {
		return
	}
	if p.kind(parent) == "call_expression" || p.kind(parent) == "new_expression" {
		callee := p.field(parent, "function")
		if callee == nil {
			callee = p.field(parent, "constructor")
		}
		if callee != nil && span(callee) == span(n) {
			return
		}
	}
	if p.stableAlias(n) {
		return
	}
	for node := parent; node != nil; node = p.parent(node) {
		if slices.Contains([]string{"type_annotation", "type_arguments", "import_statement", "export_clause", "class_heritage", "type_alias_declaration"}, p.kind(node)) {
			return
		}
		if jsScopeKind(p.kind(node)) {
			break
		}
	}
	for _, item := range a.callable(p, n) {
		if item.function == nil && !item.class {
			continue
		}
		if item.p == p && item.d.NameSpan == span(n) {
			continue
		}
		a.callUnknown(item, p, n, "函数或构造器作为值传递，间接调用实参不能完整核对")
	}
}

func (a *jsAnalysis) constructorValues() {
	for _, write := range a.fieldWrites {
		fields := a.matchingFields(write.item.d.ID, write.name, map[string]bool{})
		if len(fields) == 1 {
			field := fields[0]
			a.assign(field.item, &field.item.d.Usage.Fields[field.index], write.file, write.node, write.implicit)
		}
	}
	for _, link := range a.fieldLinks {
		fields := a.matchingFields(link.item.d.ID, link.name, map[string]bool{})
		if len(fields) != 1 {
			continue
		}
		field := fields[0]
		slot := &field.item.d.Usage.Fields[field.index]
		parameter := &link.method.d.Usage.Parameters[link.parameter]
		if len(parameter.Values) == 0 && len(parameter.Functions) == 0 {
			a.unknown(field.item, slot, link.method.p, link.node, "字段来自参数，但没有可核对的默认值或调用实参")
		}
		for _, value := range parameter.Values {
			if !slices.Contains(slot.Values, value) {
				slot.Values = append(slot.Values, value)
			}
		}
		for _, use := range parameter.Functions {
			addUse(&slot.Functions, use)
		}
		slot.Function = slot.Function || parameter.Function
		for _, use := range parameter.Unknown {
			addUse(&slot.Unknown, use)
			field.item.p.missing("usage", field.item.n, "字段 "+slot.Name+" 的来源参数未能完整核对："+use.Reason)
		}
	}
}
