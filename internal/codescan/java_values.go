package codescan

import (
	"fmt"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

type javaSlot struct {
	item  *javaDeclaration
	index int
	field bool
}
type javaWrite struct {
	target javaSlot
	file   *javaFile
	node   *ts.Node
}
type javaFieldLink struct {
	target, source javaSlot
	node           *ts.Node
	file           *javaFile
}

func (ref javaSlot) slot() *Slot {
	if ref.item == nil {
		return nil
	}
	if ref.field {
		return &ref.item.d.Usage.Fields[ref.index]
	}
	return &ref.item.d.Usage.Parameters[ref.index]
}

func (a *javaAnalysis) javaFunctionType(p *javaFile, typ *ts.Node) bool {
	name := p.typeName(typ, 0)
	if strings.HasPrefix(name, "java.util.function.") || name == "java.lang.Runnable" || name == "java.util.concurrent.Callable" {
		return true
	}
	item := a.typeItem(name)
	if item == nil || item.p.kind(item.n) != "interface_declaration" {
		return false
	}
	for _, annotation := range item.p.annotations(item.n) {
		if item.p.typeName(item.p.field(annotation, "name"), 0) == "java.lang.FunctionalInterface" {
			return true
		}
	}
	methods := map[string]bool{}
	for _, methodName := range item.d.Usage.Methods {
		if slices.Contains([]string{"toString", "equals", "hashCode"}, methodName) {
			continue
		}
		for _, method := range a.javaMethods(name, methodName, map[string]bool{}) {
			if method.p.field(method.n, "body") == nil && !method.p.hasModifier(method.n, "static") && !method.p.hasModifier(method.n, "private") {
				methods[methodName+strings.Join(strings.Fields(method.p.text(method.p.field(method.n, "parameters"))), "")] = true
			}
		}
	}
	return len(methods) == 1
}

func (a *javaAnalysis) javaSlots(item *javaDeclaration) {
	p, n := item.p, item.n
	if javaCallable(p.kind(n)) {
		parameters := p.field(n, "parameters")
		if p.kind(n) == "compact_constructor_declaration" {
			parameters = p.field(p.enclosingType(n), "parameters")
		}
		for _, node := range p.named(parameters) {
			if p.kind(node) == "receiver_parameter" {
				continue
			}
			var binding *javaBinding
			for _, list := range p.bindings[n] {
				if len(list) == 1 && list[0].node == node {
					binding = list[0]
					break
				}
			}
			if binding == nil {
				a.unknown(item, nil, p, node, "参数绑定无法唯一核对")
				continue
			}
			name := p.field(node, "name")
			if name == nil {
				for _, child := range p.named(node) {
					if p.kind(child) == "variable_declarator" {
						name = p.field(child, "name")
					}
				}
			}
			slot := Slot{Name: binding.name, Type: p.text(binding.annotation), Span: span(name), Public: item.d.Usage.Public, Interface: a.javaContract(item), Function: a.javaFunctionType(p, binding.annotation), Variadic: p.kind(node) == "spread_parameter"}
			if dimensions := p.field(node, "dimensions"); dimensions != nil {
				slot.Type += strings.Join(strings.Fields(p.text(dimensions)), "")
			}
			if slot.Variadic {
				slot.Type += "[]"
			}
			item.d.Usage.Parameters = append(item.d.Usage.Parameters, slot)
			item.parameters = append(item.parameters, binding)
			a.slots[binding] = javaSlot{item, len(item.d.Usage.Parameters) - 1, false}
			if p.kind(n) == "compact_constructor_declaration" {
				if fields := p.bindings[p.enclosingType(n)][binding.name]; len(fields) == 1 {
					if target := a.slots[fields[0]]; target.item != nil {
						target.slot().Written = true
						a.fieldLinks = append(a.fieldLinks, javaFieldLink{target, a.slots[binding], n, p})
					}
				}
			}
		}
	}
	if !javaTypeNode(p.kind(n)) {
		return
	}
	nodes := p.named(p.field(n, "body"))
	if p.kind(n) == "enum_declaration" {
		for _, node := range nodes {
			if p.kind(node) == "enum_body_declarations" {
				nodes = append(nodes, p.named(node)...)
				break
			}
		}
	}
	for _, node := range nodes {
		if p.kind(node) != "field_declaration" && p.kind(node) != "constant_declaration" {
			continue
		}
		for _, field := range p.named(node) {
			if p.kind(field) != "variable_declarator" {
				continue
			}
			a.javaField(item, field, p.field(field, "name"), node)
		}
	}
	if p.kind(n) == "record_declaration" {
		for _, parameter := range p.named(p.field(n, "parameters")) {
			a.javaField(item, parameter, p.field(parameter, "name"), parameter)
		}
	}
}

func (a *javaAnalysis) javaField(item *javaDeclaration, field, name, modifiers *ts.Node) {
	p := item.p
	if name == nil {
		return
	}
	list := p.bindings[item.n][p.text(name)]
	if len(list) != 1 {
		a.unknown(item, nil, p, field, "字段绑定无法唯一核对")
		return
	}
	b := list[0]
	slot := Slot{Name: b.name, Type: p.text(b.annotation), Span: span(name), Public: item.d.Usage.Public && p.hasModifier(modifiers, "public"), External: len(p.annotations(modifiers)) > 0, Function: a.javaFunctionType(p, b.annotation)}
	if dimensions := p.field(field, "dimensions"); dimensions != nil {
		slot.Type += strings.Join(strings.Fields(p.text(dimensions)), "")
	}
	item.d.Usage.Fields = append(item.d.Usage.Fields, slot)
	ref := javaSlot{item, len(item.d.Usage.Fields) - 1, true}
	a.slots[b] = ref
	if p.kind(item.n) == "record_declaration" {
		a.members[p.types[item.n]+"."+b.name] = []*javaBinding{b}
		if item.d.Usage.Public {
			a.unknown(item, ref.slot(), p, field, "公开记录类的隐式构造参数可来自工作区外，字段值集合不能完整核对")
		}
	}
	value := javaZero(ref.slot().Type)
	value.At = p.use(field, b.name)
	ref.slot().Values = append(ref.slot().Values, value)
	if b.value != nil {
		ref.slot().Written = true
		a.writes = append(a.writes, javaWrite{ref, p, b.value})
	}
}

func javaZero(typ string) Value {
	switch typ {
	case "boolean":
		return Value{Kind: "boolean", Text: "false", Implicit: true}
	case "char":
		return Value{Kind: "char", Text: "\x00", Implicit: true}
	case "byte", "short", "int", "long", "float", "double":
		return Value{Kind: typ, Text: "0", Implicit: true}
	default:
		return Value{Kind: "null", Text: "null", Implicit: true}
	}
}

func (a *javaAnalysis) javaSlotAt(p *javaFile, n *ts.Node) javaSlot {
	var b *javaBinding
	if p.kind(n) == "identifier" {
		b, _ = p.lookup(p.text(n), n)
	} else if p.kind(n) == "field_access" {
		ref := p.reference(p.field(n, "object"), 0)
		owner := ref.typ
		if owner == "" {
			owner = ref.qualified
		}
		if bindings := a.memberBindings(owner, p.text(p.field(n, "field")), 0); len(bindings) == 1 {
			b = bindings[0]
		}
	}
	return a.slots[b]
}

func (a *javaAnalysis) javaReadWrite(p *javaFile, n *ts.Node) {
	if p.kind(n) != "identifier" && p.kind(n) != "field_access" {
		return
	}
	parent := p.parent(n)
	if p.kind(n) == "identifier" {
		if p.kind(parent) == "field_access" && span(p.field(parent, "field")) == span(n) {
			return
		}
		if name := p.field(parent, "name"); name != nil && span(name) == span(n) {
			return
		}
		if slices.Contains([]string{"package_declaration", "import_declaration", "annotation", "marker_annotation", "element_value_pair", "inferred_parameters"}, p.kind(parent)) {
			return
		}
	}
	ref := a.javaSlotAt(p, n)
	if ref.item == nil {
		return
	}
	slot := ref.slot()
	if p == ref.item.p && slot.Span == span(n) {
		return
	}
	read, write := true, false
	var value *ts.Node
	reason := ""
	element := false
	for node := n; p.parent(node) != nil; node = p.parent(node) {
		parent = p.parent(node)
		if p.kind(parent) == "parenthesized_expression" {
			continue
		}
		if p.kind(parent) == "array_access" && span(p.field(parent, "array")) == span(node) {
			element = true
			continue
		}
		if p.kind(parent) == "assignment_expression" && span(p.field(parent, "left")) == span(node) {
			write = true
			if p.text(p.field(parent, "operator")) == "=" {
				read = false
				value = p.field(parent, "right")
			} else {
				reason = "复合赋值依赖原值，不能将显式常量当作全部取值"
			}
		} else if p.kind(parent) == "update_expression" {
			write = true
			reason = "自增减依赖原值，完整值集合不能确定"
		}
		break
	}
	if element && write {
		read = true
		value = nil
		reason = "数组元素写入可能改变参数或字段内容，完整值集合不能确定"
	}
	slot.Read = slot.Read || read
	slot.Written = slot.Written || write
	if reason != "" {
		a.unknown(ref.item, slot, p, n, reason)
	}
	if value != nil {
		if ref.field && p.kind(value) == "identifier" {
			source := a.javaSlotAt(p, value)
			if source.item != nil && !source.field {
				a.fieldLinks = append(a.fieldLinks, javaFieldLink{ref, source, value, p})
				return
			}
		}
		a.writes = append(a.writes, javaWrite{ref, p, value})
	}
	if slot.Function && p.kind(parent) == "method_invocation" && span(p.field(parent, "object")) == span(n) {
		slot.Read = true
	}
}

func (a *javaAnalysis) javaPrepareValues() {
	for _, link := range a.fieldLinks {
		source, target := link.source.slot(), link.target.slot()
		source.Function = source.Function || target.Function
		target.Function = target.Function || source.Function
	}
	for _, write := range a.writes {
		a.javaAssign(write.target, write.file, write.node)
	}
}

func (a *javaAnalysis) javaAssign(target javaSlot, p *javaFile, n *ts.Node) {
	if n == nil || target.item == nil {
		return
	}
	slot := target.slot()
	if p.kind(n) == "lambda_expression" {
		slot.Function = true
		addUse(&slot.Functions, p.use(n, fmt.Sprintf("%s:lambda@%d", p.file.Path, n.StartByte())))
		return
	}
	if p.kind(n) == "method_reference" {
		slot.Function = true
		name, methods := a.javaMethodReference(p, n)
		if name != "" && len(methods) <= 1 {
			addUse(&slot.Functions, p.use(n, name))
		} else {
			a.unknown(target.item, slot, p, n, "方法引用的所属类型或重载签名不能唯一核对")
		}
		return
	}
	if p.kind(n) == "object_creation_expression" && slot.Function {
		for _, child := range p.named(n) {
			if p.kind(child) == "class_body" {
				addUse(&slot.Functions, p.use(n, fmt.Sprintf("%s:anonymous@%d", p.file.Path, n.StartByte())))
				return
			}
		}
	}
	if value, ok := p.literal(n, 0); ok {
		value = javaConvertValue(value, slot.Type)
		value.At = p.use(n, slot.Name)
		if !slices.Contains(slot.Values, value) {
			slot.Values = append(slot.Values, value)
		}
		return
	}
	a.unknown(target.item, slot, p, n, "赋值或实参不能解析为常量或唯一函数，完整值集合不能确定")
}

func (a *javaAnalysis) javaConstructorValues() {
	for _, link := range a.fieldLinks {
		source, target := link.source.slot(), link.target.slot()
		target.Function = target.Function || source.Function
		for _, value := range source.Values {
			value = javaConvertValue(value, target.Type)
			if !slices.Contains(target.Values, value) {
				target.Values = append(target.Values, value)
			}
		}
		for _, use := range source.Functions {
			addUse(&target.Functions, use)
		}
		if len(source.Unknown) > 0 || source.Public || source.Interface || (len(source.Values) == 0 && len(source.Functions) == 0) {
			a.unknown(link.target.item, target, link.file, link.node, "字段来源参数的调用、外部约定或写入不能完整核对")
		}
	}
}

func (a *javaAnalysis) javaObjectEscape(p *javaFile, n *ts.Node, name, reason string, seen map[string]bool) {
	if name == "" || seen[name] || a.ctx.Err() != nil {
		return
	}
	seen[name] = true
	if item := a.typeItem(name); item != nil {
		for i := range item.d.Usage.Fields {
			a.unknown(item, &item.d.Usage.Fields[i], p, n, reason)
		}
	}
	for _, parent := range a.parentTypes(name, 0) {
		a.javaObjectEscape(p, n, parent, reason, seen)
	}
}
