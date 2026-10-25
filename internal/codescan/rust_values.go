package codescan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

type rustSlot struct {
	item  *rustDeclaration
	index int
	field bool
}
type rustWrite struct {
	target rustSlot
	p      *rustFile
	node   *ts.Node
}

func (ref rustSlot) slot() *Slot {
	if ref.item == nil {
		return nil
	}
	if ref.field {
		return &ref.item.d.Usage.Fields[ref.index]
	}
	return &ref.item.d.Usage.Parameters[ref.index]
}

func (a *rustAnalysis) rustSlots(item *rustDeclaration) {
	p, n := item.p, item.n
	if callable(p.kind(n)) {
		for _, node := range p.named(p.field(n, "parameters")) {
			if p.kind(node) == "self_parameter" {
				continue
			}
			pattern := p.field(node, "pattern")
			if pattern == nil {
				a.unknown(item, nil, p, node, "参数模式不能核对")
				continue
			}
			contract := p.ancestor(n, "trait_item") != nil
			if owner := p.ancestor(n, "impl_item"); owner != nil && p.field(owner, "trait") != nil {
				contract = true
			}
			slot := Slot{Name: p.text(pattern), Type: p.text(p.field(node, "type")), Span: span(pattern), Public: item.d.Usage.Public, Interface: contract}
			slot.Function = p.functionType(p.field(node, "type"))
			for _, parameter := range p.named(p.field(n, "type_parameters")) {
				if p.text(p.field(parameter, "name")) != slot.Type {
					continue
				}
				_ = walkSyntax(a.ctx, parameter, func(child *ts.Node) {
					if p.kind(child) == "function_type" {
						slot.Function = true
					}
				})
			}
			for _, clause := range p.named(n) {
				if p.kind(clause) != "where_clause" {
					continue
				}
				for _, predicate := range p.named(clause) {
					if p.text(p.field(predicate, "left")) != slot.Type {
						continue
					}
					_ = walkSyntax(a.ctx, predicate, func(child *ts.Node) {
						if p.kind(child) == "function_type" {
							slot.Function = true
						}
					})
				}
			}
			item.d.Usage.Parameters = append(item.d.Usage.Parameters, slot)
			ref := rustSlot{item, len(item.d.Usage.Parameters) - 1, false}
			for _, name := range p.patternNames(pattern) {
				for _, b := range p.bindings[n][p.text(name)] {
					if b.node == node {
						a.slots[b] = ref
					}
				}
			}
			if p.kind(pattern) != "identifier" {
				a.unknown(item, ref.slot(), p, node, "解构参数的各部分值尚不能完整对应")
			}
			if slot.Public || slot.Interface || len(item.d.Usage.Implicit) > 0 {
				a.unknown(item, ref.slot(), p, node, "公开、trait 或隐式入口可由工作区外调用，参数值集合不完整")
			}
		}
	}
	if p.kind(n) != "struct_item" {
		return
	}
	index := 0
	publicTuple := false
	for _, node := range p.named(p.field(n, "body")) {
		if p.kind(node) == "visibility_modifier" {
			publicTuple = p.text(node) == "pub"
			continue
		}
		var name, typ *ts.Node
		if p.kind(node) == "field_declaration" {
			name, typ = p.field(node, "name"), p.field(node, "type")
		} else if p.kind(p.field(n, "body")) == "ordered_field_declaration_list" && p.kind(node) != "visibility_modifier" && p.kind(node) != "attribute_item" {
			typ = node
		} else {
			continue
		}
		fieldName := strconv.Itoa(index)
		index++
		region := span(node)
		if name != nil {
			fieldName = p.text(name)
			region = span(name)
		}
		slot := Slot{Name: fieldName, Type: p.text(typ), Span: region, Public: item.d.Usage.Public && p.public(node), External: p.derived(n, "Deserialize")}
		if name == nil {
			slot.Public = item.d.Usage.Public && publicTuple
			publicTuple = false
		}
		slot.Function = p.functionType(typ)
		item.d.Usage.Fields = append(item.d.Usage.Fields, slot)
		stored := &item.d.Usage.Fields[len(item.d.Usage.Fields)-1]
		if slot.External {
			a.unknown(item, stored, p, node, "Deserialize 字段可从外部数据写入")
		}
		if p.derived(n, "Default") {
			a.unknown(item, stored, p, node, "Default 派生生成的字段默认值未展开")
		}
		if slot.Public {
			a.unknown(item, stored, p, node, "公开字段可由工作区外写入")
		}
	}
}

func (p *rustFile) functionType(n *ts.Node) bool {
	if n == nil {
		return false
	}
	found := false
	_ = walkSyntax(p.engine.ctx, n, func(child *ts.Node) {
		if p.kind(child) == "function_type" {
			found = true
		}
	})
	return found
}

func (a *rustAnalysis) field(item *rustDeclaration, name string) rustSlot {
	if item != nil {
		for i, slot := range item.d.Usage.Fields {
			if slot.Name == name {
				return rustSlot{item, i, true}
			}
		}
	}
	return rustSlot{}
}

func (a *rustAnalysis) slotRef(p *rustFile, n *ts.Node) rustSlot {
	if n == nil {
		return rustSlot{}
	}
	if p.kind(n) == "identifier" {
		if b, ok := p.lookup(p.text(n), n); ok && b != nil {
			return a.slots[b]
		}
	}
	if p.kind(n) == "field_expression" {
		ref := p.reference(p.field(n, "value"), 0)
		return a.field(a.item(ref.typ), p.text(p.field(n, "field")))
	}
	return rustSlot{}
}

func (a *rustAnalysis) indirectSlot(p *rustFile, n *ts.Node) rustSlot {
	if n == nil {
		return rustSlot{}
	}
	if ref := a.slotRef(p, n); ref.item != nil {
		return ref
	}
	if slices.Contains([]string{"reference_expression", "parenthesized_expression", "unary_expression", "index_expression"}, p.kind(n)) {
		for _, child := range p.named(n) {
			if p.kind(child) != "mutable_specifier" {
				return a.indirectSlot(p, child)
			}
		}
	}
	return rustSlot{}
}

func (p *rustFile) bindingName(n *ts.Node) bool {
	for parent := p.parent(n); parent != nil; parent = p.parent(parent) {
		if slices.Contains([]string{"parameter", "let_declaration", "let_condition", "for_expression", "match_arm"}, p.kind(parent)) {
			pattern := p.field(parent, "pattern")
			if pattern != nil && n.StartByte() >= pattern.StartByte() && n.EndByte() <= pattern.EndByte() {
				return true
			}
		}
		if p.kind(parent) == "closure_parameters" {
			return true
		}
		if slices.Contains([]string{"block", "source_file"}, p.kind(parent)) {
			break
		}
	}
	return false
}

func (a *rustAnalysis) rustReadWrite(p *rustFile, n *ts.Node) {
	switch p.kind(n) {
	case "identifier", "scoped_identifier", "field_expression":
		if p.bindingName(n) || p.ancestor(n, "attribute_item", "use_declaration", "token_tree") != nil {
			return
		}
		if ref := a.slotRef(p, n); ref.item != nil {
			parent := p.parent(n)
			if p.kind(parent) != "assignment_expression" || p.field(parent, "left") != n {
				ref.slot().Read = true
			}
		}
		if p.kind(n) == "identifier" || p.kind(n) == "scoped_identifier" {
			parent := p.parent(n)
			if p.field(parent, "name") == n || p.kind(parent) == "scoped_identifier" || (p.kind(parent) == "call_expression" && p.field(parent, "function") == n) || p.kind(parent) == "generic_function" {
				return
			}
			if item := a.item(p.reference(n, 0).qualified); item != nil && callable(p.kind(item.n)) {
				a.rustGeneric(item, p, n, nil)
				for i := range item.d.Usage.Parameters {
					a.unknown(item, &item.d.Usage.Parameters[i], p, n, "函数作为值传递后，后续调用实参不能完整核对")
				}
			}
		}
	case "assignment_expression", "compound_assignment_expr":
		left := p.field(n, "left")
		ref := a.indirectSlot(p, left)
		if ref.item != nil {
			ref.slot().Written = true
			if p.kind(n) == "compound_assignment_expr" || a.slotRef(p, left).item == nil {
				ref.slot().Read = true
				a.unknown(ref.item, ref.slot(), p, n, "复合或间接更新后的值不能静态确定")
			} else {
				a.writes = append(a.writes, rustWrite{ref, p, p.field(n, "right")})
			}
		} else {
			a.rustObjectUnknown(p, left, n, "对象整体赋值后的字段值不能完整核对")
		}
	case "reference_expression":
		for _, child := range p.named(n) {
			if p.kind(child) != "mutable_specifier" {
				continue
			}
			if ref := a.indirectSlot(p, p.field(n, "value")); ref.item != nil {
				ref.slot().Written = true
				a.unknown(ref.item, ref.slot(), p, n, "可变借用可能间接写入")
			}
			a.rustObjectUnknown(p, p.field(n, "value"), n, "对象的可变借用可能间接写入字段")
		}
	case "struct_expression":
		item := a.item(p.typeName(p.field(n, "name"), 0))
		if item == nil {
			return
		}
		if p.text(p.field(n, "name")) != "Self" {
			a.rustGeneric(item, p, n, p.field(p.field(n, "name"), "type_arguments"))
		}
		for _, field := range p.named(p.field(n, "body")) {
			name, value := p.text(p.field(field, "field")), p.field(field, "value")
			if p.kind(field) == "shorthand_field_initializer" {
				children := p.named(field)
				if len(children) == 1 {
					name = p.text(children[0])
					value = children[0]
				}
			}
			if ref := a.field(item, name); ref.item != nil && value != nil {
				ref.slot().Written = true
				a.writes = append(a.writes, rustWrite{ref, p, value})
			}
			if p.kind(field) == "base_field_initializer" {
				for i := range item.d.Usage.Fields {
					a.unknown(item, &item.d.Usage.Fields[i], p, field, "结构体更新语法的继承字段值不能完整核对")
				}
			}
		}
	case "macro_invocation":
		item := p.enclosingUsage(n)
		if item == nil {
			return
		}
		a.unknown(item, nil, p, n, "宏未展开，内部声明和字段读写不能完整核对")
		for i := range item.d.Usage.Parameters {
			a.unknown(item, &item.d.Usage.Parameters[i], p, n, "宏未展开，内部参数读写不能完整核对")
		}
	}
}

func (a *rustAnalysis) rustObjectUnknown(p *rustFile, value, evidence *ts.Node, reason string) {
	if value == nil {
		return
	}
	ref := p.reference(value, 0)
	if ref.typ == "" && p.kind(value) == "identifier" {
		if b, ok := p.lookup(p.text(value), value); ok && b != nil {
			if b.annotation != nil {
				ref.typ = b.file.typeName(b.annotation, 0)
			} else if b.value != nil {
				ref = b.file.reference(b.value, 0)
			}
		}
	}
	if item := a.item(ref.typ); item != nil {
		for i := range item.d.Usage.Fields {
			a.unknown(item, &item.d.Usage.Fields[i], p, evidence, reason)
		}
	}
}

func (a *rustAnalysis) rustCalls(p *rustFile, n *ts.Node) {
	if p.kind(n) != "call_expression" {
		return
	}
	fn := p.field(n, "function")
	args := p.named(p.field(n, "arguments"))
	if ref := a.slotRef(p, fn); ref.item != nil {
		ref.slot().Read = true
		ref.slot().Function = true
	}
	var generics *ts.Node
	if p.kind(fn) == "generic_function" {
		generics = p.field(fn, "type_arguments")
		fn = p.field(fn, "function")
	}
	if p.kind(fn) == "field_expression" && p.text(p.field(fn, "field")) == "downcast_ref" {
		for _, arg := range p.named(generics) {
			a.rustErrorName(p, p.typeName(arg, 0), n)
		}
	}
	target := p.callTarget(n, 0)
	if p.kind(fn) == "scoped_identifier" {
		scope := p.field(fn, "path")
		if p.text(scope) != "Self" {
			if owner := a.item(p.typeName(scope, 0)); owner != nil {
				a.rustGeneric(owner, p, n, p.field(scope, "type_arguments"))
			}
		}
	}
	item := a.item(target)
	if item == nil {
		for _, arg := range args {
			a.rustObjectUnknown(p, arg, n, "对象传给不能核对的调用，字段写入范围未知")
		}
		for _, candidate := range a.byTarget[target] {
			for i := range candidate.d.Usage.Parameters {
				a.unknown(candidate, &candidate.d.Usage.Parameters[i], p, n, "同名或 trait 方法无法唯一分派")
			}
		}
		return
	}
	a.rustGeneric(item, p, n, generics)
	if p.kind(item.n) == "struct_item" {
		if p.kind(item.p.field(item.n, "body")) == "ordered_field_declaration_list" {
			for i, arg := range args {
				if i < len(item.d.Usage.Fields) {
					ref := rustSlot{item, i, true}
					ref.slot().Written = true
					a.writes = append(a.writes, rustWrite{ref, p, arg})
				}
			}
		}
		return
	}
	if !callable(p.kind(item.n)) {
		return
	}
	if p.kind(fn) != "field_expression" {
		for _, parameter := range item.p.named(item.p.field(item.n, "parameters")) {
			if item.p.kind(parameter) == "self_parameter" && len(args) > 0 {
				args = args[1:]
				break
			}
		}
	}
	if len(args) != len(item.d.Usage.Parameters) {
		for i := range item.d.Usage.Parameters {
			a.unknown(item, &item.d.Usage.Parameters[i], p, n, "调用实参个数与参数位置不能对应")
		}
		return
	}
	for i, arg := range args {
		a.writes = append(a.writes, rustWrite{rustSlot{item, i, false}, p, arg})
	}
}

func (a *rustAnalysis) rustValue(p *rustFile, n *ts.Node, depth int) ([]Value, bool) {
	if n == nil || depth > 40 || a.ctx.Err() != nil {
		return nil, false
	}
	value := Value{At: p.use(n, "")}
	switch p.kind(n) {
	case "string_literal", "raw_string_literal":
		text, ok := p.literal(n, 0)
		value.Kind, value.Text = "string", text
		return []Value{value}, ok
	case "integer_literal", "float_literal":
		value.Kind, value.Text = p.kind(n), strings.ReplaceAll(p.text(n), "_", "")
		return []Value{value}, true
	case "boolean_literal", "char_literal":
		value.Kind, value.Text = p.kind(n), p.text(n)
		return []Value{value}, true
	case "closure_expression":
		value.Kind, value.Text = "function", fmt.Sprintf("%s:%d", p.file.Path, n.StartByte())
		return []Value{value}, true
	case "reference_expression", "parenthesized_expression":
		for _, child := range p.named(n) {
			if p.kind(child) != "mutable_specifier" {
				return a.rustValue(p, child, depth+1)
			}
		}
	case "unary_expression":
		children := p.named(n)
		if len(children) == 1 && strings.HasPrefix(p.text(n), "-") {
			values, ok := a.rustValue(p, children[0], depth+1)
			if ok && len(values) == 1 && slices.Contains([]string{"integer_literal", "float_literal"}, values[0].Kind) {
				values[0].Text = "-" + values[0].Text
				return values, true
			}
		}
	case "identifier", "scoped_identifier", "field_expression":
		if ref := a.slotRef(p, n); ref.item != nil {
			return slices.Clone(ref.slot().Values), len(ref.slot().Values) > 0 && len(ref.slot().Unknown) == 0
		}
		if p.kind(n) == "identifier" {
			if b, ok := p.lookup(p.text(n), n); ok && b != nil {
				if b.changed || p.kind(b.node) == "static_item" {
					return nil, false
				}
				if b.value != nil {
					return a.rustValue(b.file, b.value, depth+1)
				}
			}
		}
		target := p.reference(n, 0).qualified
		if item := a.item(target); item != nil && callable(p.kind(item.n)) {
			value.Kind, value.Text = "function", item.d.ID
			return []Value{value}, true
		}
		if list := a.symbols[target]; len(list) == 1 && list[0].constant {
			return a.rustValue(list[0].file, list[0].value, depth+1)
		}
	}
	return nil, false
}

func (a *rustAnalysis) rustValues() {
	done := make([]bool, len(a.writes))
	for pass := 0; pass <= len(a.writes) && a.ctx.Err() == nil; pass++ {
		changed := false
		for i, write := range a.writes {
			values, ok := a.rustValue(write.p, write.node, 0)
			slot := write.target.slot()
			for _, value := range values {
				value.At = write.p.use(write.node, value.Text)
				if !slices.Contains(slot.Values, value) {
					slot.Values = append(slot.Values, value)
					changed = true
				}
				if value.Kind == "function" {
					slot.Function = true
					addUse(&slot.Functions, value.At)
				}
			}
			done[i] = ok
		}
		if !changed {
			break
		}
	}
	for pass := 0; pass <= len(a.writes) && a.ctx.Err() == nil; pass++ {
		changed := false
		for i, write := range a.writes {
			_, complete := a.rustValue(write.p, write.node, 0)
			if !done[i] || !complete {
				before := len(write.target.slot().Unknown)
				a.unknown(write.target.item, write.target.slot(), write.p, write.node, "值来源包含未知、外部或间接写入，不能断定为常量")
				changed = changed || len(write.target.slot().Unknown) > before
			}
		}
		if !changed {
			break
		}
	}
	sortRustMethods(a.declarations)
}
