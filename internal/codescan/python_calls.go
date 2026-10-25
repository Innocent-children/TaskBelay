package codescan

import (
	"encoding/json"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (a *pyAnalysis) methods(owner, name string, seen map[string]bool) []*pyDeclaration {
	if seen[owner] || a.ctx.Err() != nil {
		return nil
	}
	seen[owner] = true
	defer delete(seen, owner)
	var result []*pyDeclaration
	for _, item := range a.byName[owner] {
		var own []*pyDeclaration
		for _, method := range a.ownMethods(item) {
			if method.d.Name == name {
				own = append(own, method)
			}
		}
		if len(own) > 0 {
			result = append(result, own...)
			continue
		}
		for _, parent := range item.parents {
			result = append(result, a.methods(parent.d.ID, name, seen)...)
		}
	}
	return result
}

func (a *pyAnalysis) callable(p *pyFile, n *ts.Node) []*pyDeclaration {
	if targets := a.byName[p.reference(n, 0).qualified]; len(targets) > 0 {
		return targets
	}
	if p.kind(n) == "attribute" {
		owner := p.reference(p.field(n, "object"), 0)
		name := owner.typ
		if name == "" {
			name = owner.qualified
		}
		return a.methods(name, p.text(p.field(n, "attribute")), map[string]bool{})
	}
	return nil
}

func (a *pyAnalysis) calls(p *pyFile, n *ts.Node) {
	switch p.kind(n) {
	case "except_clause":
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child.IsNamed() && p.kind(child) != "block" {
				a.errorChecks(p, n, child)
			}
		}
	case "identifier", "attribute":
		a.functionEscape(p, n)
	case "call":
		callee := p.field(n, "function")
		args := p.arguments(n)
		targets := a.callable(p, callee)
		if len(targets) == 1 {
			target := targets[0]
			if target.p.kind(target.n) == "class_definition" {
				initializers := a.methods(target.d.ID, "__init__", map[string]bool{})
				if len(initializers) == 1 {
					a.callArguments(initializers[0], p, n, args)
				}
				if target.external {
					a.externalConstructor(target, p, n, args)
				}
			} else if target.p.kind(target.n) == "function_definition" {
				if target.self != "" && !target.classmethod {
					if p.kind(callee) == "attribute" && p.reference(p.field(callee, "object"), 0).typ == "" {
						if len(args) > 0 && args[0].name == "" && !args[0].spread {
							args = args[1:]
						} else {
							a.callUnknown(target, p, n, "非绑定方法的接收者不能核对")
							return
						}
					} else if p.kind(callee) != "attribute" {
						a.callUnknown(target, p, n, "方法别名是否已绑定接收者不能核对")
						return
					}
				}
				a.callArguments(target, p, n, args)
			}
		} else if len(targets) > 1 {
			for _, target := range targets {
				a.callUnknown(target, p, n, "调用目标有多个声明候选")
			}
		} else if p.reference(callee, 0).qualified == "" {
			name := p.text(callee)
			if p.kind(callee) == "attribute" {
				name = p.text(p.field(callee, "attribute"))
			}
			for _, candidate := range a.byName[name] {
				if candidate.p.kind(candidate.n) == "function_definition" {
					a.callUnknown(candidate, p, n, "同名调用的绑定、接收者或重绑定不能核对")
				}
			}
		}
		if p.reference(callee, 0).qualified == "builtins.isinstance" {
			a.errorChecks(p, n, pyArgumentAt(args, 1, "class_or_tuple"))
		}
		for _, arg := range args {
			ref := p.reference(arg.value, 0)
			if ref.typ != "" {
				a.objectEscape(p, n, ref.typ, map[string]bool{})
			}
		}
	}
}

func (a *pyAnalysis) callUnknown(item *pyDeclaration, p *pyFile, n *ts.Node, reason string) {
	for i := range item.d.Usage.Parameters {
		a.unknown(item, &item.d.Usage.Parameters[i], p, n, reason)
	}
}

func (a *pyAnalysis) callArguments(item *pyDeclaration, p *pyFile, call *ts.Node, args []pyArgument) {
	for _, arg := range args {
		if arg.spread {
			a.callUnknown(item, p, call, "调用包含 *args 或 **kwargs，完整实参集合不能核对")
			return
		}
	}
	assigned := map[int][]pyArgument{}
	position := 0
	for _, arg := range args {
		index := -1
		if arg.name != "" {
			for _, parameter := range item.parameters {
				if parameter.name == arg.name && !parameter.positionalOnly && parameter.rest == "" {
					index = parameter.index
					break
				}
			}
			if index < 0 {
				for _, parameter := range item.parameters {
					if parameter.rest == "keyword" {
						index = parameter.index
						break
					}
				}
			}
		} else {
			for position < len(item.parameters) {
				parameter := item.parameters[position]
				if parameter.rest == "positional" {
					index = parameter.index
					break
				}
				position++
				if !parameter.keywordOnly && parameter.rest == "" {
					index = parameter.index
					break
				}
			}
		}
		if index < 0 {
			a.callUnknown(item, p, call, "实参数量或名字与可核对签名不匹配")
			return
		}
		assigned[index] = append(assigned[index], arg)
	}
	for _, parameter := range item.parameters {
		slot := &item.d.Usage.Parameters[parameter.index]
		values := assigned[parameter.index]
		if parameter.rest != "" {
			a.variadicValues(item, slot, p, call, values, parameter.rest)
			continue
		}
		if len(values) > 1 {
			a.unknown(item, slot, p, call, "同一参数被重复传入")
			continue
		}
		if len(values) == 1 {
			a.assign(item, slot, p, values[0].value, false)
		} else if parameter.defaultValue == nil {
			a.unknown(item, slot, p, call, "调用缺少必需参数")
		}
	}
}

func (a *pyAnalysis) variadicValues(item *pyDeclaration, slot *Slot, p *pyFile, call *ts.Node, args []pyArgument, rest string) {
	type element struct{ Name, Kind, Text string }
	values := make([]element, 0, len(args))
	for _, arg := range args {
		value, ok := p.literal(arg.value, 0)
		if !ok {
			a.unknown(item, slot, p, arg.value, "可变参数包含不能核对的值")
			return
		}
		values = append(values, element{arg.name, value.Kind, value.Text})
	}
	if rest == "keyword" {
		slices.SortFunc(values, func(a, b element) int { return strings.Compare(a.Name, b.Name) })
	}
	encoded, _ := json.Marshal(values)
	value := Value{Kind: "Sequence", Text: string(encoded), At: p.use(call, slot.Name)}
	for _, old := range slot.Values {
		if old.At.File == value.At.File && old.At.Span == value.At.Span && old.Kind == value.Kind && old.Text == value.Text {
			return
		}
	}
	slot.Values = append(slot.Values, value)
}

func (a *pyAnalysis) externalConstructor(item *pyDeclaration, p *pyFile, call *ts.Node, args []pyArgument) {
	for _, arg := range args {
		if arg.spread {
			a.objectEscape(p, call, item.d.ID, map[string]bool{})
			continue
		}
		if arg.name == "" {
			continue
		}
		fields := a.matchingFields(item.d.ID, arg.name, map[string]bool{})
		if len(fields) == 1 {
			field := fields[0]
			a.assign(field.item, &field.item.d.Usage.Fields[field.index], p, arg.value, false)
		}
	}
}

func (a *pyAnalysis) errorChecks(p *pyFile, check, expression *ts.Node) {
	if expression == nil {
		return
	}
	if targets := a.byName[p.reference(expression, 0).qualified]; len(targets) == 1 && targets[0].d.Usage.Error {
		addUse(&targets[0].d.Usage.ErrorChecks, p.use(check, targets[0].d.ID))
		return
	}
	if slices.Contains([]string{"tuple", "as_pattern", "parenthesized_expression"}, p.kind(expression)) {
		for i := 0; i < expression.ChildCount(); i++ {
			if child := expression.Child(i); child.IsNamed() && p.kind(child) != "as_pattern_target" {
				a.errorChecks(p, check, child)
			}
		}
	} else if p.reference(expression, 0).qualified == "" {
		p.missing("usage", expression, "异常区分中的类型不能静态核对")
	}
}

func (a *pyAnalysis) functionEscape(p *pyFile, n *ts.Node) {
	parent := n.Parent()
	if parent == nil {
		return
	}
	if p.kind(parent) == "attribute" || p.kind(parent) == "decorator" {
		return
	}
	if fn := p.field(parent, "function"); p.kind(parent) == "call" && fn != nil && fn.StartByte() == n.StartByte() {
		return
	}
	for node := parent; node != nil; node = node.Parent() {
		if slices.Contains([]string{"import_statement", "import_from_statement", "parameters", "type"}, p.kind(node)) {
			return
		}
		if pyScopeKind(p.kind(node)) {
			break
		}
	}
	for _, item := range a.callable(p, n) {
		if item.p.kind(item.n) != "function_definition" || (item.d.File == p.file.Path && item.d.NameSpan == span(n)) {
			continue
		}
		a.callUnknown(item, p, n, "函数作为值传递，间接调用的实参不能完整核对")
	}
}

func (a *pyAnalysis) objectEscape(p *pyFile, n *ts.Node, owner string, seen map[string]bool) {
	if seen[owner] || a.ctx.Err() != nil {
		return
	}
	seen[owner] = true
	for _, item := range a.byName[owner] {
		for i := range item.d.Usage.Fields {
			a.unknown(item, &item.d.Usage.Fields[i], p, n, "对象作为实参传递，别名或间接写入不能完整核对")
		}
		for _, parent := range item.parents {
			a.objectEscape(p, n, parent.d.ID, seen)
		}
	}
}

func (a *pyAnalysis) constructorValues() {
	for _, link := range a.fieldLinks {
		fields := a.matchingFields(link.item.d.ID, link.field, map[string]bool{})
		if len(fields) != 1 {
			continue
		}
		field := fields[0]
		slot := &field.item.d.Usage.Fields[field.index]
		parameter := &link.method.d.Usage.Parameters[link.parameter]
		if len(parameter.Values) == 0 && len(parameter.Functions) == 0 {
			a.unknown(field.item, slot, link.method.p, link.node, "字段来自参数，但尚无可核对的默认值或调用实参")
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
