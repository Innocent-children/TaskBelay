package codescan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *genericFile) shadowed(item *genericDeclaration, name string, n *ts.Node) bool {
	for _, local := range p.locals[item] {
		if local.name == name && local.node.StartByte() <= n.StartByte() && local.scope.StartByte() <= n.StartByte() && n.EndByte() <= local.scope.EndByte() {
			return true
		}
	}
	return false
}

func (a *genericAnalysis) typeItem(p *genericFile, typ string) *genericDeclaration {
	typ = strings.TrimSpace(strings.Trim(typ, "*&?"))
	typ = strings.TrimPrefix(typ, "const ")
	typ = strings.TrimPrefix(typ, "struct ")
	var result *genericDeclaration
	for _, item := range a.byName[fileKey(p.file.Kind.Language, typ)] {
		if item.typ {
			if result != nil {
				return nil
			}
			result = item
		}
	}
	return result
}

func (a *genericAnalysis) slotRef(p *genericFile, n *ts.Node) genericSlot {
	if n == nil {
		return genericSlot{}
	}
	item := p.enclosing(n, false)
	if slices.Contains([]string{"identifier", "field_identifier", "variable_name", "instance_variable", "name"}, p.kind(n)) {
		name := p.text(n)
		if item != nil && !p.shadowed(item, name, n) {
			for i, slot := range item.d.Usage.Parameters {
				if slot.Name == name {
					return genericSlot{item, i, false}
				}
			}
		}
		if item != nil && p.shadowed(item, name, n) {
			return genericSlot{}
		}
		return a.field(p.enclosing(n, true), name)
	}
	if slices.Contains([]string{"field_expression", "member_access_expression", "member_access", "member_expression"}, p.kind(n)) {
		var object, field *ts.Node
		for _, key := range []string{"object", "value", "argument", "expression"} {
			if object = p.field(n, key); object != nil {
				break
			}
		}
		for _, key := range []string{"field", "property", "name", "member"} {
			if field = p.field(n, key); field != nil {
				break
			}
		}
		if object == nil || field == nil {
			return genericSlot{}
		}
		if slices.Contains([]string{"this", "self", "$this"}, p.text(object)) {
			return a.field(p.enclosing(n, true), p.text(field))
		}
		if ref := a.slotRef(p, object); ref.item != nil {
			return a.field(a.typeItem(p, ref.slot().Type), p.text(field))
		}
		if typ := a.typeItem(p, p.text(object)); typ != nil {
			return a.field(typ, p.text(field))
		}
	}
	return genericSlot{}
}

func (a *genericAnalysis) indirect(p *genericFile, n *ts.Node) genericSlot {
	if ref := a.slotRef(p, n); ref.item != nil {
		return ref
	}
	if slices.Contains([]string{"pointer_expression", "unary_expression", "reference_expression", "parenthesized_expression", "subscript_expression", "element_access_expression", "index_expression"}, p.kind(n)) {
		if children := p.named(n); len(children) > 0 {
			return a.indirect(p, children[0])
		}
	}
	return genericSlot{}
}

func (p *genericFile) declarationName(n *ts.Node) bool {
	return p.definitions[span(n)]
}

func (a *genericAnalysis) genericUsage(p *genericFile, n *ts.Node) {
	kind := p.kind(n)
	if slices.Contains([]string{"identifier", "field_identifier", "name", "variable_name", "instance_variable", "field_expression", "member_access_expression", "member_access", "member_expression"}, kind) {
		parent := p.parent(n)
		if p.declarationName(n) || p.kind(parent) == "variable_name" {
			return
		}
		if p.kind(parent) == "name_colon" || p.kind(parent) == "argument" && p.field(parent, "name") == n {
			return
		}
		if slices.Contains([]string{"field_expression", "member_access_expression", "member_access", "member_expression"}, p.kind(parent)) && (p.field(parent, "field") == n || p.field(parent, "name") == n || p.field(parent, "property") == n || p.field(parent, "member") == n) {
			return
		}
		if ref := a.slotRef(p, n); ref.item != nil {
			if !slices.Contains([]string{"assignment_expression", "assignment"}, p.kind(parent)) || p.field(parent, "left") != n || p.text(p.field(parent, "operator")) != "=" && p.field(parent, "operator") != nil {
				ref.slot().Read = true
			}
		}
	}
	if slices.Contains([]string{"assignment_expression", "assignment", "operator_assignment", "augmented_assignment_expression"}, kind) {
		left, right := p.field(n, "left"), p.field(n, "right")
		if ref := a.indirect(p, left); ref.item != nil {
			ref.slot().Written = true
			op := p.text(p.field(n, "operator"))
			if a.slotRef(p, left).item == nil || kind == "operator_assignment" || kind == "augmented_assignment_expression" || op != "" && op != "=" {
				ref.slot().Read = true
				a.unknown(ref.item, ref.slot(), p, n, "复合或间接更新后的值尚不能确定")
			} else if right != nil {
				a.writes = append(a.writes, genericWrite{ref, p, right})
			}
		}
	}
	if slices.Contains([]string{"update_expression", "postfix_unary_expression", "prefix_unary_expression"}, kind) {
		argument := p.field(n, "argument")
		if argument == nil {
			argument = p.field(n, "operand")
		}
		if argument == nil {
			children := p.named(n)
			if len(children) > 0 {
				argument = children[0]
			}
		}
		if strings.Contains(p.text(n), "++") || strings.Contains(p.text(n), "--") {
			if ref := a.indirect(p, argument); ref.item != nil {
				ref.slot().Read, ref.slot().Written = true, true
				a.unknown(ref.item, ref.slot(), p, n, "自增减后的值尚不能确定")
			}
		}
	}
	if slices.Contains([]string{"pointer_expression", "unary_expression", "reference_expression"}, kind) && strings.HasPrefix(p.text(n), "&") {
		if ref := a.indirect(p, n); ref.item != nil {
			a.unknown(ref.item, ref.slot(), p, n, "取址或引用传递后的间接写入范围未知")
		}
	}
}

func (p *genericFile) callParts(n *ts.Node) (*ts.Node, *ts.Node) {
	if !slices.Contains([]string{"call_expression", "call", "invocation_expression", "function_call_expression", "member_call_expression", "scoped_call_expression", "object_creation_expression"}, p.kind(n)) {
		return nil, nil
	}
	var fn *ts.Node
	for _, key := range []string{"function", "method", "name", "type"} {
		if fn = p.field(n, key); fn != nil {
			break
		}
	}
	return fn, p.field(n, "arguments")
}

func (a *genericAnalysis) genericCall(p *genericFile, n *ts.Node) {
	fn, argNode := p.callParts(n)
	if fn == nil {
		return
	}
	if ref := a.slotRef(p, fn); ref.item != nil {
		ref.slot().Read, ref.slot().Function = true, true
		return
	}
	name := p.text(fn)
	qualified := false
	if slices.Contains([]string{"member_access_expression", "member_expression", "field_expression", "qualified_identifier", "scoped_property_access_expression"}, p.kind(fn)) {
		qualified = true
		for _, key := range []string{"name", "field", "property"} {
			if member := p.field(fn, key); member != nil {
				name = p.text(member)
				break
			}
		}
	}
	if p.field(n, "receiver") != nil || p.field(n, "object") != nil {
		qualified = true
	}
	caller := p.enclosing(n, false)
	shadowed := caller != nil && p.shadowed(caller, name, n)
	var candidates []*genericDeclaration
	for _, item := range a.byName[fileKey(p.file.Kind.Language, name)] {
		if callable(item.p.kind(item.n)) {
			candidates = append(candidates, item)
		}
	}
	if !qualified && caller != nil {
		var local []*genericDeclaration
		for _, item := range candidates {
			if item.p == p && item.d.Owner == caller.d.Owner {
				local = append(local, item)
			}
		}
		if len(local) > 0 {
			candidates = local
		}
	}
	if len(candidates) == 0 {
		return
	}
	for _, item := range candidates {
		unknown := len(candidates) > 1 || qualified || shadowed || item.p != p
		if item.d.Owner != "" && (caller == nil || caller.d.Owner != item.d.Owner) {
			unknown = true
		}
		if unknown {
			for i := range item.d.Usage.Parameters {
				a.unknown(item, &item.d.Usage.Parameters[i], p, n, "名字调用候选的接收者、遮蔽、重载或跨文件连接范围不能唯一核对")
			}
			continue
		}
		assigned := map[int]*ts.Node{}
		next := 0
		valid := true
		for _, argument := range p.named(argNode) {
			value, key := argument, ""
			if p.kind(argument) == "argument" {
				key = p.text(p.field(argument, "name"))
				children := p.named(argument)
				if len(children) > 0 {
					value = children[len(children)-1]
				}
			} else if p.kind(argument) == "pair" {
				key = p.text(p.field(argument, "key"))
				value = p.field(argument, "value")
			}
			index := next
			if key != "" {
				index = -1
				for i, slot := range item.d.Usage.Parameters {
					if strings.TrimPrefix(slot.Name, "$") == key {
						index = i
						break
					}
				}
			} else {
				next++
			}
			if index < 0 || index >= len(item.d.Usage.Parameters) || assigned[index] != nil || value == nil {
				valid = false
				break
			}
			assigned[index] = value
		}
		for i := range item.d.Usage.Parameters {
			if assigned[i] == nil {
				assigned[i] = item.defaults[i]
				if assigned[i] == nil {
					valid = false
				}
			}
		}
		if !valid {
			for i := range item.d.Usage.Parameters {
				a.unknown(item, &item.d.Usage.Parameters[i], p, n, "调用参数的位置、名字、展开或缺省值不能完整对应")
			}
			continue
		}
		for i := range item.d.Usage.Parameters {
			if item.indirectParameters[i] {
				if ref := a.indirect(p, assigned[i]); ref.item != nil {
					a.unknown(ref.item, ref.slot(), p, n, "传入指针、数组或引用参数后，写入范围不能完整核对")
				}
			}
			a.writes = append(a.writes, genericWrite{genericSlot{item, i, false}, p, assigned[i]})
		}
	}
}

func (p *genericFile) initializer(item *genericDeclaration) *ts.Node {
	if value := p.defaultValue(item.n); value != nil {
		return value
	}
	if right := p.field(item.n, "right"); right != nil {
		return right
	}
	for _, child := range p.named(item.n) {
		if child.StartByte() <= item.d.NameSpan.Start && child.EndByte() >= item.d.NameSpan.End {
			if value := p.defaultValue(child); value != nil {
				return value
			}
		}
	}
	return nil
}
func (p *genericFile) constant(item *genericDeclaration) bool {
	if item.d.Kind == "constant" {
		return true
	}
	for _, child := range p.named(item.n) {
		if slices.Contains([]string{"type_qualifier", "modifier", "storage_class_specifier"}, p.kind(child)) && slices.Contains([]string{"const", "constexpr"}, p.text(child)) {
			return true
		}
	}
	return false
}

func (a *genericAnalysis) value(p *genericFile, n *ts.Node, depth int) ([]Value, bool) {
	if n == nil || depth > 40 || a.ctx.Err() != nil {
		return nil, false
	}
	v := Value{At: p.use(n, "")}
	switch p.kind(n) {
	case "integer", "integer_literal", "number_literal", "float", "float_literal", "real_literal", "decimal_integer_literal":
		v.Kind, v.Text = "number", p.text(n)
		return []Value{v}, true
	case "true", "false", "boolean_literal", "boolean":
		v.Kind, v.Text = "boolean", p.text(n)
		return []Value{v}, true
	case "null", "nil", "null_literal":
		v.Kind, v.Text = "null", p.text(n)
		return []Value{v}, true
	case "string", "string_literal", "char_literal", "character_literal", "encapsed_string":
		text := p.text(n)
		if strings.Contains(text, "\\") {
			return nil, false
		}
		decoded, err := strconv.Unquote(text)
		if err != nil && len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' && !strings.Contains(text, "\\") {
			decoded = text[1 : len(text)-1]
			err = nil
		}
		if err != nil {
			return nil, false
		}
		for _, child := range p.named(n) {
			if strings.Contains(p.kind(child), "interpolation") || p.kind(child) == "variable_name" {
				return nil, false
			}
		}
		v.Kind, v.Text = "string", decoded
		if p.kind(n) == "char_literal" || p.kind(n) == "character_literal" {
			v.Kind = "char"
		}
		return []Value{v}, true
	case "lambda_expression", "lambda", "anonymous_function", "arrow_function", "block":
		v.Kind, v.Text = "function", fmt.Sprintf("%s:%d", p.file.Path, n.StartByte())
		return []Value{v}, true
	case "parenthesized_expression", "argument":
		if children := p.named(n); len(children) == 1 {
			return a.value(p, children[0], depth+1)
		}
	case "unary_expression":
		if children := p.named(n); len(children) == 1 && strings.HasPrefix(p.text(n), "-") {
			values, ok := a.value(p, children[0], depth+1)
			if ok && len(values) == 1 && values[0].Kind == "number" {
				values[0].Text = "-" + values[0].Text
				return values, true
			}
		}
	case "identifier", "name", "constant", "variable_name", "instance_variable", "field_expression", "member_access_expression":
		if ref := a.slotRef(p, n); ref.item != nil {
			return slices.Clone(ref.slot().Values), len(ref.slot().Values) > 0 && len(ref.slot().Unknown) == 0
		}
		if owner := p.enclosing(n, false); owner != nil && p.shadowed(owner, p.text(n), n) {
			return nil, false
		}
		candidates := a.byName[fileKey(p.file.Kind.Language, p.text(n))]
		if len(candidates) == 1 {
			item := candidates[0]
			if callable(item.p.kind(item.n)) {
				v.Kind, v.Text = "function", item.d.ID
				return []Value{v}, true
			}
			if item.p.constant(item) {
				return a.value(item.p, item.p.initializer(item), depth+1)
			}
		}
	}
	return nil, false
}

func (a *genericAnalysis) genericValues() {
	complete := make([]bool, len(a.writes))
	for pass := 0; pass <= len(a.writes) && a.ctx.Err() == nil; pass++ {
		changed := false
		for i, write := range a.writes {
			values, ok := a.value(write.p, write.n, 0)
			complete[i] = ok
			for _, value := range values {
				value.At = write.p.use(write.n, value.Text)
				if !slices.Contains(write.slot.slot().Values, value) {
					write.slot.slot().Values = append(write.slot.slot().Values, value)
					changed = true
				}
				if value.Kind == "function" {
					write.slot.slot().Function = true
					addUse(&write.slot.slot().Functions, value.At)
				}
			}
		}
		if !changed {
			break
		}
	}
	for pass := 0; pass <= len(a.writes) && a.ctx.Err() == nil; pass++ {
		changed := false
		for i, write := range a.writes {
			_, ok := a.value(write.p, write.n, 0)
			if !complete[i] || !ok {
				before := len(write.slot.slot().Unknown)
				a.unknown(write.slot.item, write.slot.slot(), write.p, write.n, "值来源含有未核对的表达式或间接写入")
				changed = changed || len(write.slot.slot().Unknown) > before
			}
		}
		if !changed {
			break
		}
	}
}
