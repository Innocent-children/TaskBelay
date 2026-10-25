package codescan

import (
	"fmt"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

type jsField struct {
	item  *jsDeclaration
	index int
}
type jsFieldWrite struct {
	item     *jsDeclaration
	name     string
	file     *jsFile
	node     *ts.Node
	implicit bool
}
type jsFieldLink struct {
	item, method *jsDeclaration
	name         string
	parameter    int
	node         *ts.Node
}
type jsParameterWrite struct {
	item  *jsDeclaration
	index int
	file  *jsFile
	node  *ts.Node
}

func (a *jsAnalysis) functionType(p *jsFile, n *ts.Node, seen map[string]bool) bool {
	if n == nil {
		return false
	}
	if p.kind(n) == "function_type" {
		return true
	}
	if p.kind(n) == "type_annotation" && n.NamedChildCount() == 1 {
		return a.functionType(p, n.NamedChild(0), seen)
	}
	if p.kind(n) == "union_type" {
		for i := 0; i < n.ChildCount(); i++ {
			if child := n.Child(i); child.IsNamed() && a.functionType(p, child, seen) {
				return true
			}
		}
	}
	name := p.typeName(n, 0)
	if name == "global.Function" {
		return true
	}
	if name == "" || seen[name] {
		return false
	}
	seen[name] = true
	if item := a.byID[name]; item != nil && item.p.kind(item.n) == "type_alias_declaration" {
		return a.functionType(item.p, item.p.field(item.n, "value"), seen)
	}
	if item := a.byID[name]; item != nil && item.p.kind(item.n) == "interface_declaration" {
		if body := item.p.field(item.n, "body"); body != nil {
			for i := 0; i < body.ChildCount(); i++ {
				if item.p.kind(body.Child(i)) == "call_signature" {
					return true
				}
			}
		}
	}
	return false
}

func (a *jsAnalysis) contractMethod(item *jsDeclaration) bool {
	if item.function == nil {
		return false
	}
	if slices.Contains([]string{"method_signature", "abstract_method_signature"}, item.p.kind(item.function)) {
		return true
	}
	owner := item.p.thisClass(item.function)
	if owner == nil {
		return false
	}
	seen := map[string]bool{}
	var visit func(*jsDeclaration) bool
	visit = func(parent *jsDeclaration) bool {
		if seen[parent.d.ID] {
			return false
		}
		seen[parent.d.ID] = true
		if parent.d.Usage.Abstract && slices.Contains(parent.d.Usage.Methods, item.d.Name) {
			return true
		}
		for _, base := range parent.parents {
			if visit(base) {
				return true
			}
		}
		return false
	}
	return visit(owner)
}

func (a *jsAnalysis) parameters(item *jsDeclaration) {
	p, n := item.p, item.function
	if n == nil {
		return
	}
	parameters := p.field(n, "parameters")
	if parameters == nil {
		parameters = p.field(n, "parameter")
	}
	if parameters == nil {
		return
	}
	nodes := []*ts.Node{parameters}
	if p.kind(parameters) == "formal_parameters" {
		nodes = nil
		for i := 0; i < parameters.ChildCount(); i++ {
			child := parameters.Child(i)
			if child.IsNamed() && p.kind(child) != "comment" {
				nodes = append(nodes, child)
			}
		}
	}
	public := item.d.Usage.Public
	if owner := p.thisClass(n); owner != nil && owner.d.Usage.Public && !strings.HasPrefix(item.d.Name, "#") {
		public = true
		for i := 0; i < n.ChildCount(); i++ {
			if child := n.Child(i); p.kind(child) == "accessibility_modifier" && p.text(child) == "private" {
				public = false
			}
		}
	}
	for _, node := range nodes {
		pattern, defaultValue := node, (*ts.Node)(nil)
		if p.kind(node) == "required_parameter" || p.kind(node) == "optional_parameter" {
			pattern = p.field(node, "pattern")
			defaultValue = p.field(node, "value")
		}
		if p.kind(pattern) == "assignment_pattern" {
			defaultValue = p.field(pattern, "right")
			pattern = p.field(pattern, "left")
		}
		if p.text(pattern) == "this" {
			continue
		}
		parameter := jsParameter{pattern: pattern, defaultValue: defaultValue, variadic: p.kind(pattern) == "rest_pattern", optional: p.kind(node) == "optional_parameter"}
		for _, name := range bindingNames(pattern, p.syntax.language) {
			index := len(item.d.Usage.Parameters)
			parameter.slots = append(parameter.slots, index)
			slot := Slot{Name: p.text(name), Type: p.text(p.field(node, "type")), Span: span(name), Variadic: parameter.variadic, Public: public, Interface: a.contractMethod(item), Function: a.functionType(p, p.field(node, "type"), map[string]bool{})}
			item.d.Usage.Parameters = append(item.d.Usage.Parameters, slot)
			if !slices.Contains([]string{"identifier", "undefined", "rest_pattern"}, p.kind(pattern)) {
				a.unknown(item, &item.d.Usage.Parameters[index], p, pattern, "解构参数需要逐项核对，不能按整个实参推断各个绑定的值")
			}
		}
		if len(parameter.slots) == 0 {
			a.unknown(item, nil, p, node, "参数绑定名字不能静态核对")
		}
		item.parameters = append(item.parameters, parameter)
	}
}

func (a *jsAnalysis) field(item *jsDeclaration, name, annotation *ts.Node) *Slot {
	if name == nil {
		return nil
	}
	p := item.p
	key := p.propertyName(name)
	if p.kind(name) == "private_property_identifier" {
		key = p.text(name)
	}
	if key == "" {
		a.unknown(item, nil, p, name, "类字段的名字不能静态核对")
		return nil
	}
	for i := range item.d.Usage.Fields {
		if item.d.Usage.Fields[i].Name == key {
			return &item.d.Usage.Fields[i]
		}
	}
	if item.fieldTypes == nil {
		item.fieldTypes = map[string]*ts.Node{}
	}
	item.fieldTypes[key] = annotation
	item.d.Usage.Fields = append(item.d.Usage.Fields, Slot{Name: key, Type: p.text(annotation), Span: span(name), Public: item.d.Usage.Public, Function: a.functionType(p, annotation, map[string]bool{})})
	return &item.d.Usage.Fields[len(item.d.Usage.Fields)-1]
}

func (a *jsAnalysis) fields(item *jsDeclaration) {
	if !item.class || item.p.kind(item.n) == "interface_declaration" {
		return
	}
	p := item.p
	definition := item.n
	if p.kind(definition) == "variable_declarator" {
		definition = p.field(definition, "value")
	}
	err := walkSyntax(a.ctx, definition, func(n *ts.Node) {
		if p.thisClass(n) != item {
			return
		}
		switch p.kind(n) {
		case "public_field_definition", "field_definition":
			name := p.field(n, "name")
			if name == nil {
				name = p.field(n, "property")
			}
			slot := a.field(item, name, p.field(n, "type"))
			if slot == nil {
				return
			}
			if slot.Span != span(name) {
				a.unknown(item, slot, p, n, "同名类字段有多个声明，实例与静态存储不能合并判断")
			}
			slot.External = slot.External || jsHasDecorator(p, n)
			slot.Written = true
			if value := p.field(n, "value"); value != nil {
				a.fieldValue(item, slot, p, value, true)
			} else {
				a.undefined(item, slot, p, n, true)
			}
		case "assignment_expression", "augmented_assignment_expression":
			left := p.field(n, "left")
			if p.kind(left) != "member_expression" && p.kind(left) != "subscript_expression" {
				return
			}
			if p.kind(p.field(left, "object")) != "this" {
				return
			}
			name := p.field(left, "property")
			if p.kind(left) == "subscript_expression" {
				name = p.field(left, "index")
			}
			slot := a.field(item, name, nil)
			if slot == nil {
				return
			}
			slot.Written = true
			if slot.Span == span(name) {
				a.undefined(item, slot, p, left, true)
			}
			if p.kind(n) == "augmented_assignment_expression" {
				slot.Read = true
				a.unknown(item, slot, p, n, "复合赋值依赖先前字段值")
				return
			}
			a.fieldValue(item, slot, p, p.field(n, "right"), false)
		}
	})
	if err != nil {
		a.unknown(item, nil, p, definition, err.Error())
	}
	for _, method := range a.ownMethods(item) {
		if method.d.Name != "constructor" {
			continue
		}
		for _, parameter := range method.parameters {
			if len(parameter.slots) != 1 {
				continue
			}
			n := p.parent(parameter.pattern)
			if p.kind(n) != "required_parameter" && p.kind(n) != "optional_parameter" {
				continue
			}
			property := false
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				if p.kind(child) == "accessibility_modifier" || p.text(child) == "readonly" {
					property = true
				}
			}
			if !property {
				continue
			}
			slot := a.field(item, parameter.pattern, p.field(n, "type"))
			if slot != nil {
				slot.Written = true
				slot.External = jsHasDecorator(p, n)
				a.fieldLinks = append(a.fieldLinks, jsFieldLink{item, method, slot.Name, parameter.slots[0], parameter.pattern})
			}
		}
	}
}

func (a *jsAnalysis) undefined(item *jsDeclaration, slot *Slot, p *jsFile, n *ts.Node, implicit bool) {
	value := Value{Kind: "undefined", Text: "undefined", Implicit: implicit, At: p.use(n, slot.Name)}
	if !slices.Contains(slot.Values, value) {
		slot.Values = append(slot.Values, value)
	}
}

func (a *jsAnalysis) fieldValue(item *jsDeclaration, slot *Slot, p *jsFile, n *ts.Node, implicit bool) {
	if n == nil {
		return
	}
	if p.kind(n) == "identifier" || p.kind(n) == "undefined" {
		if method, index := a.parameterAt(p, n); method != nil {
			for _, link := range a.fieldLinks {
				if link.item == item && link.name == slot.Name && link.method == method && link.parameter == index && span(link.node) == span(n) {
					return
				}
			}
			a.fieldLinks = append(a.fieldLinks, jsFieldLink{item, method, slot.Name, index, n})
			return
		}
	}
	a.fieldWrites = append(a.fieldWrites, jsFieldWrite{item, slot.Name, p, n, implicit})
}

func (a *jsAnalysis) parameterAt(p *jsFile, n *ts.Node) (*jsDeclaration, int) {
	for node := n; p.parent(node) != nil; node = p.parent(node) {
		if p.kind(p.parent(node)) == "formal_parameters" {
			pattern := node
			if p.kind(node) == "required_parameter" || p.kind(node) == "optional_parameter" {
				pattern = p.field(node, "pattern")
			}
			for _, name := range bindingNames(pattern, p.syntax.language) {
				if span(name) == span(n) {
					return nil, 0
				}
			}
			break
		}
		if p.kind(p.parent(node)) == "arrow_function" && p.field(p.parent(node), "parameter") != nil && span(p.field(p.parent(node), "parameter")) == span(n) {
			return nil, 0
		}
		if jsScopeKind(p.kind(p.parent(node))) {
			break
		}
	}
	for node := p.parent(n); node != nil; node = p.parent(node) {
		if !jsScopeKind(p.kind(node)) {
			continue
		}
		if len(p.bindings[p.key(node)][p.text(n)]) == 0 {
			continue
		}
		if item := p.usage[p.key(node)]; item != nil && item.function == node {
			for i, slot := range item.d.Usage.Parameters {
				if slot.Name == p.text(n) {
					return item, i
				}
			}
		}
		return nil, 0
	}
	return nil, 0
}

func (a *jsAnalysis) matchingFields(owner, name string, seen map[string]bool) []jsField {
	if seen[owner] || a.ctx.Err() != nil {
		return nil
	}
	seen[owner] = true
	defer delete(seen, owner)
	item := a.byID[owner]
	if item == nil {
		return nil
	}
	for i, slot := range item.d.Usage.Fields {
		if slot.Name == name {
			return []jsField{{item, i}}
		}
	}
	var result []jsField
	for _, parent := range item.parents {
		result = append(result, a.matchingFields(parent.d.ID, name, seen)...)
	}
	return result
}

func jsAccess(p *jsFile, n *ts.Node) (read, write bool, value *ts.Node, unknown string) {
	for node := n; p.parent(node) != nil; node = p.parent(node) {
		parent := p.parent(node)
		switch p.kind(parent) {
		case "assignment_expression", "augmented_assignment_expression":
			left := p.field(parent, "left")
			if left == nil || n.StartByte() < left.StartByte() || n.EndByte() > left.EndByte() {
				return true, false, nil, ""
			}
			if p.kind(parent) == "augmented_assignment_expression" {
				return true, true, nil, "复合赋值依赖先前值"
			}
			if span(n) != span(left) {
				return true, true, nil, "解构、索引或间接写入不能逐项核对"
			}
			return false, true, p.field(parent, "right"), ""
		case "update_expression":
			return true, true, nil, "自增减依赖先前值"
		case "unary_expression":
			if p.text(p.field(parent, "operator")) == "delete" {
				return false, true, nil, "字段被删除"
			}
			return true, false, nil, ""
		case "for_in_statement":
			if left := p.field(parent, "left"); left != nil && n.StartByte() >= left.StartByte() && n.EndByte() <= left.EndByte() {
				return false, true, nil, "循环赋值依赖迭代值"
			}
			return true, false, nil, ""
		case "subscript_expression":
			if index := p.field(parent, "index"); index != nil && n.StartByte() >= index.StartByte() {
				return true, false, nil, ""
			}
		case "member_expression", "parenthesized_expression", "array_pattern", "object_pattern", "assignment_pattern", "pair_pattern":
		default:
			return true, false, nil, ""
		}
	}
	return true, false, nil, ""
}

func (a *jsAnalysis) readWrite(p *jsFile, n *ts.Node) {
	if slices.Contains([]string{"identifier", "undefined", "shorthand_property_identifier"}, p.kind(n)) {
		if parent := p.parent(n); parent != nil {
			if key := p.field(parent, "key"); key != nil && p.kind(parent) == "pair" && span(key) == span(n) {
				return
			}
		}
		if item, index := a.parameterAt(p, n); item != nil {
			slot := &item.d.Usage.Parameters[index]
			if parent := p.parent(n); parent != nil && jsCallable(p.kind(parent)) {
				if name := p.field(parent, "name"); name != nil && span(name) == span(n) {
					slot.Written = true
					a.unknown(item, slot, p, n, "参数被局部定义重新绑定")
					return
				}
			}
			read, write, value, unknown := jsAccess(p, n)
			slot.Read = slot.Read || read
			slot.Written = slot.Written || write
			if parent := p.parent(n); parent != nil && p.kind(parent) == "call_expression" {
				if callee := p.field(parent, "function"); callee != nil && span(callee) == span(n) {
					slot.Function = true
				}
			}
			if unknown != "" {
				a.unknown(item, slot, p, n, unknown)
			} else if write {
				a.parameterWrites = append(a.parameterWrites, jsParameterWrite{item, index, p, value})
			}
		}
		return
	}
	if p.kind(n) != "member_expression" && p.kind(n) != "subscript_expression" {
		return
	}
	object := p.reference(p.field(n, "object"), 0)
	owner := object.typ
	if owner == "" {
		owner = object.qualified
	}
	name := p.propertyName(p.field(n, "property"))
	property := p.field(n, "property")
	if p.kind(n) == "subscript_expression" {
		property = p.field(n, "index")
		value, ok := p.literal(property, 0)
		if ok && value.Kind == "String" {
			name = value.Text
		}
	}
	if name == "" {
		if owner != "" {
			a.objectEscape(p, n, owner, map[string]bool{}, "计算字段名不能逐项核对")
		}
		return
	}
	if _, write, _, _ := jsAccess(p, n); write {
		for _, method := range a.methods(owner, name, map[string]bool{}) {
			a.callUnknown(method, p, n, "方法属性被写入，后续调用不能按原签名完整核对")
		}
	}
	fields := a.matchingFields(owner, name, map[string]bool{})
	if len(fields) == 1 {
		field := fields[0]
		slot := &field.item.d.Usage.Fields[field.index]
		if property != nil && slot.Span == span(property) && field.item.p == p {
			return
		}
		read, write, value, unknown := jsAccess(p, n)
		slot.Read = slot.Read || read
		slot.Written = slot.Written || write
		if parent := p.parent(n); parent != nil && p.kind(parent) == "call_expression" {
			if callee := p.field(parent, "function"); callee != nil && span(callee) == span(n) {
				slot.Function = true
			}
		}
		if unknown != "" {
			a.unknown(field.item, slot, p, n, unknown)
		} else if write {
			a.fieldValue(field.item, slot, p, value, false)
		}
	} else if owner == "" || a.byID[owner] == nil {
		for _, field := range a.fieldsByName[name] {
			a.unknown(field.item, &field.item.d.Usage.Fields[field.index], p, n, "同名字段的接收者或别名不能核对")
		}
	}
}

func (a *jsAnalysis) assign(item *jsDeclaration, slot *Slot, p *jsFile, n *ts.Node, implicit bool) {
	if n == nil {
		return
	}
	if slot.Function {
		if jsFunction(p.kind(n)) {
			value := fmt.Sprintf("%s:function@%d", p.file.Path, n.StartByte())
			if target := p.usage[p.key(n)]; target != nil {
				value = target.d.ID
			}
			addUse(&slot.Functions, p.use(n, value))
			return
		}
		if targets := a.callable(p, n); len(targets) == 1 && targets[0].function != nil {
			addUse(&slot.Functions, p.use(n, targets[0].d.ID))
			return
		}
		if target := p.reference(n, 0).qualified; slices.Contains([]string{"global.Number", "global.String", "global.Boolean", "global.parseInt", "global.parseFloat", "global.Array.from"}, target) {
			addUse(&slot.Functions, p.use(n, target))
			return
		}
		if p.kind(n) != "null" && p.kind(n) != "undefined" {
			a.unknown(item, slot, p, n, "函数值不能唯一对应到具名函数或匿名函数")
			return
		}
	}
	value, ok := p.literal(n, 0)
	if !ok {
		a.unknown(item, slot, p, n, "值不能解析为字面量或顶层常量")
		return
	}
	value.At = p.use(n, slot.Name)
	value.Implicit = implicit
	if !slices.Contains(slot.Values, value) {
		slot.Values = append(slot.Values, value)
	}
}

func (a *jsAnalysis) objectEscape(p *jsFile, n *ts.Node, owner string, seen map[string]bool, reason string) {
	if seen[owner] || a.ctx.Err() != nil {
		return
	}
	seen[owner] = true
	if item := a.byID[owner]; item != nil {
		for i := range item.d.Usage.Fields {
			a.unknown(item, &item.d.Usage.Fields[i], p, n, reason)
		}
		for _, method := range a.ownMethods(item) {
			if method.d.Name != "constructor" {
				a.callUnknown(method, p, n, reason)
			}
		}
		for _, parent := range item.parents {
			a.objectEscape(p, n, parent.d.ID, seen, reason)
		}
	}
}
