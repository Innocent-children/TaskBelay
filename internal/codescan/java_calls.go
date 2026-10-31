package codescan

import (
	"encoding/json"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (a *javaAnalysis) javaMethods(owner, name string, seen map[string]bool) []*javaDeclaration {
	if owner == "" || seen[owner] || a.ctx.Err() != nil {
		return nil
	}
	seen[owner] = true
	var targets []*javaDeclaration
	for _, binding := range a.methods[owner+"."+name] {
		if item := binding.file.usage[binding.node]; item != nil {
			targets = append(targets, item)
		}
	}
	for _, base := range a.parentTypes(owner, 0) {
		for _, item := range a.javaMethods(base, name, seen) {
			if !slices.Contains(targets, item) {
				targets = append(targets, item)
			}
		}
	}
	return targets
}

func (a *javaAnalysis) javaMethodReference(p *javaFile, n *ts.Node) (string, []*javaDeclaration) {
	children := p.named(n)
	if len(children) == 0 {
		return "", nil
	}
	ref := p.reference(children[0], 0)
	owner := ref.typ
	if owner == "" {
		owner = ref.qualified
	}
	if owner == "" {
		return "", nil
	}
	name := ""
	for i := 1; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if p.kind(child) == "identifier" || p.kind(child) == "new" {
			name = p.text(child)
		}
	}
	if name == "" {
		return "", nil
	}
	if name == "new" {
		return owner + ".new", a.javaMethods(owner, owner[strings.LastIndex(owner, ".")+1:], map[string]bool{})
	}
	return owner + "." + name, a.javaMethods(owner, name, map[string]bool{})
}

func (a *javaAnalysis) javaCallUnknown(item *javaDeclaration, p *javaFile, n *ts.Node, reason string) {
	a.unknown(item, nil, p, n, reason)
	if item.d.Usage.Generic {
		item.d.Usage.Inferred = true
	}
	for i := range item.d.Usage.Parameters {
		a.unknown(item, &item.d.Usage.Parameters[i], p, n, reason)
	}
}

func (a *javaAnalysis) javaCalls(p *javaFile, n *ts.Node) {
	switch p.kind(n) {
	case "method_reference":
		_, targets := a.javaMethodReference(p, n)
		for _, target := range targets {
			a.javaCallUnknown(target, p, n, "方法作为函数值传递，后续调用实参不能完整核对")
		}
	case "method_invocation", "object_creation_expression", "explicit_constructor_invocation":
		name, owner, target := "", "", ""
		args := p.named(p.field(n, "arguments"))
		switch p.kind(n) {
		case "method_invocation":
			name = p.text(p.field(n, "name"))
			target = p.methodTarget(n, 0)
			if index := strings.LastIndex(target, "."); index >= 0 {
				owner = target[:index]
			}
		case "object_creation_expression":
			owner = p.typeName(p.field(n, "type"), 0)
			name = owner[strings.LastIndex(owner, ".")+1:]
		case "explicit_constructor_invocation":
			owner = p.types[p.enclosingType(n)]
			if p.text(p.field(n, "constructor")) == "super" {
				parents := a.parentTypes(owner, 0)
				if len(parents) > 0 {
					owner = parents[0]
				} else {
					owner = ""
				}
			}
			name = owner[strings.LastIndex(owner, ".")+1:]
		}
		targets := a.javaMethods(owner, name, map[string]bool{})
		if len(targets) == 0 && p.kind(n) == "method_invocation" && p.field(n, "object") == nil {
			targets = a.javaLexicalPrivate(p, n, name)
		}
		if p.kind(n) == "method_invocation" && p.kind(p.field(n, "object")) != "super" {
			for _, method := range a.overrides[owner+"."+name] {
				if !slices.Contains(targets, method) {
					targets = append(targets, method)
				}
			}
		}
		if p.kind(n) != "method_invocation" {
			targets = slices.DeleteFunc(targets, func(item *javaDeclaration) bool {
				return !javaCallable(item.p.kind(item.n)) || item.p.kind(item.n) == "method_declaration" || item.p.types[item.p.enclosingType(item.n)] != owner
			})
		}
		if len(targets) == 1 {
			confirmed := true
			for _, base := range a.parentTypes(targets[0].p.types[targets[0].p.enclosingType(targets[0].n)], 0) {
				if inherited := a.apiMethod(base, name, 0); javaKnownMethod(inherited) {
					confirmed = false
					a.javaCallUnknown(targets[0], p, n, "同名方法与外部 API 存在继承或重载，实际调用参数不能唯一核对")
				}
			}
			if confirmed {
				use := p.use(n, targets[0].d.ID)
				use.Candidate = false
				addUse(&targets[0].d.Usage.References, use)
			}
			a.javaGeneric(targets[0], p, n, p.field(n, "type_arguments"))
			a.javaCallArguments(targets[0], p, n, args)
		} else if len(targets) > 1 {
			for _, item := range targets {
				a.javaCallUnknown(item, p, n, "调用存在重载或继承候选，参数签名不能唯一核对")
			}
		} else if owner == "" || len(a.types[owner]) > 0 {
			for _, item := range a.byName[name] {
				if javaCallable(item.p.kind(item.n)) {
					a.javaCallUnknown(item, p, n, "同名调用的接收者或声明位置不能唯一核对")
				}
			}
		}
		if p.kind(n) == "object_creation_expression" && len(targets) == 0 {
			if item := a.typeItem(owner); item != nil && item.p.kind(item.n) == "record_declaration" {
				if len(args) == len(item.d.Usage.Fields) {
					for i, arg := range args {
						ref := javaSlot{item, i, true}
						ref.slot().Written = true
						a.javaAssign(ref, p, arg)
					}
				} else {
					a.javaObjectEscape(p, n, owner, "记录类构造参数不能完整对应字段", map[string]bool{})
				}
			}
		}
		for _, arg := range args {
			if typ := p.reference(arg, 0).typ; typ != "" {
				a.javaObjectEscape(p, n, typ, "对象作为实参传递，别名或间接写入不能完整核对", map[string]bool{})
			}
		}
		if target == "java.lang.reflect.Method.invoke" || target == "java.lang.Class.forName" {
			p.missing("usage", n, "反射调用的实际声明与间接写入不能完整核对")
		}
		if owner != "" && len(a.types[owner]) > 0 && len(targets) == 0 && p.kind(n) == "method_invocation" {
			a.javaObjectEscape(p, n, owner, "接收者方法未能定位，可能存在间接字段写入", map[string]bool{})
		}
	case "return_statement", "array_initializer":
		for _, value := range p.named(n) {
			if typ := p.reference(value, 0).typ; typ != "" {
				a.javaObjectEscape(p, value, typ, "对象流入返回值或容器，外部别名写入不能完整核对", map[string]bool{})
			}
		}
	case "assignment_expression":
		value := p.field(n, "right")
		if p.kind(p.field(n, "left")) == "field_access" {
			if typ := p.reference(value, 0).typ; typ != "" {
				a.javaObjectEscape(p, n, typ, "对象存入字段，后续别名写入不能完整核对", map[string]bool{})
			}
		}
	}
}

/**
 * 嵌套类型可调用外层的 private 成员；存在继承候选或静态边界时不猜接收者。
 */
func (a *javaAnalysis) javaLexicalPrivate(p *javaFile, call *ts.Node, name string) []*javaDeclaration {
	for typ := p.enclosingType(call); typ != nil; {
		if p.field(typ, "superclass") != nil || p.field(typ, "interfaces") != nil || p.hasModifier(typ, "static") {
			return nil
		}
		typ = p.enclosingType(p.parent(typ))
		if typ == nil {
			return nil
		}
		var targets []*javaDeclaration
		for _, item := range a.byName[name] {
			if item.p == p && p.enclosingType(item.n) == typ && p.hasModifier(item.n, "private") && javaCallable(p.kind(item.n)) {
				targets = append(targets, item)
			}
		}
		if len(targets) > 0 {
			return targets
		}
	}
	return nil
}

func (a *javaAnalysis) javaCallArguments(item *javaDeclaration, p *javaFile, n *ts.Node, args []*ts.Node) {
	parameters := item.d.Usage.Parameters
	variadic := len(parameters) > 0 && parameters[len(parameters)-1].Variadic
	if (!variadic && len(parameters) != len(args)) || (variadic && len(args) < len(parameters)-1) {
		a.javaCallUnknown(item, p, n, "调用实参个数与声明参数不能对应")
		return
	}
	for i := range parameters {
		ref := javaSlot{item, i, false}
		if parameters[i].Variadic {
			a.javaVariadic(ref, p, n, args[i:])
			break
		}
		a.javaAssign(ref, p, args[i])
	}
}

func (a *javaAnalysis) javaVariadic(ref javaSlot, p *javaFile, n *ts.Node, args []*ts.Node) {
	slot := ref.slot()
	if len(args) == 1 {
		arg := args[0]
		if p.kind(arg) == "array_creation_expression" {
			value := p.field(arg, "value")
			if value == nil {
				for _, child := range p.named(arg) {
					if p.kind(child) == "array_initializer" {
						value = child
					}
				}
			}
			if value != nil {
				args = p.named(value)
			} else {
				a.unknown(ref.item, slot, p, n, "可变参数数组不是显式初始化，完整序列不能核对")
				return
			}
		} else if p.kind(arg) == "null_literal" {
			a.javaAssign(ref, p, arg)
			return
		} else if binding, found := p.lookup(p.text(arg), arg); found && binding != nil && p.kind(binding.annotation) == "array_type" {
			a.unknown(ref.item, slot, p, n, "可变参数传入数组引用，完整序列不能核对")
			return
		}
	}
	type element struct{ Kind, Text string }
	values := []element{}
	for _, arg := range args {
		value, ok := p.literal(arg, 0)
		if !ok {
			a.unknown(ref.item, slot, p, arg, "可变参数含不能解析的值，完整序列不能核对")
			return
		}
		value = javaConvertValue(value, strings.TrimSuffix(slot.Type, "[]"))
		values = append(values, element{value.Kind, value.Text})
	}
	data, _ := json.Marshal(values)
	value := Value{Kind: "arguments", Text: string(data), At: p.use(n, slot.Name)}
	if !slices.Contains(slot.Values, value) {
		slot.Values = append(slot.Values, value)
	}
}
