package codescan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/rust"
	ts "github.com/odvcencio/gotreesitter"
)

func (p *rustFile) typeName(n *ts.Node, depth int) string {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return ""
	}
	switch p.kind(n) {
	case "reference_type", "pointer_type":
		return p.typeName(p.field(n, "type"), depth+1)
	case "generic_type", "generic_type_with_turbofish":
		return p.typeName(p.field(n, "type"), depth+1)
	case "primitive_type":
		return p.text(n)
	}
	if p.text(n) == "Self" {
		if owner := p.ancestor(n, "impl_item"); owner != nil {
			return p.typeName(p.field(owner, "type"), depth+1)
		}
	}
	name := p.resolve(p.text(n), n, depth+1)
	if list := p.engine.symbols[name]; len(list) == 1 && list[0].file.kind(list[0].node) == "type_item" {
		b := list[0]
		return b.file.typeName(b.file.field(b.node, "type"), depth+1)
	}
	return name
}

func (p *rustFile) bindingReference(b *rustBinding, depth int) rustReference {
	if b == nil || b.changed || depth > 40 {
		return rustReference{}
	}
	if b.hoisted && !b.constant {
		return rustReference{qualified: b.qualified}
	}
	if b.annotation != nil {
		return rustReference{typ: b.file.typeName(b.annotation, depth+1)}
	}
	return b.file.reference(b.value, depth+1)
}

func (p *rustFile) reference(n *ts.Node, depth int) rustReference {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return rustReference{}
	}
	switch p.kind(n) {
	case "self":
		if owner := p.ancestor(n, "impl_item"); owner != nil {
			return rustReference{typ: p.typeName(p.field(owner, "type"), depth+1)}
		}
	case "identifier", "type_identifier":
		if b, bound := p.lookup(p.text(n), n); bound {
			return p.bindingReference(b, depth+1)
		}
		return rustReference{qualified: p.typeName(n, depth+1)}
	case "scoped_identifier", "scoped_type_identifier":
		if scope := p.field(n, "path"); scope != nil {
			if owner := p.typeName(scope, depth+1); owner != "" {
				return rustReference{qualified: owner + "::" + p.text(p.field(n, "name"))}
			}
		}
		return rustReference{qualified: p.resolve(p.text(n), n, depth+1)}
	case "generic_function":
		return p.reference(p.field(n, "function"), depth+1)
	case "generic_type", "generic_type_with_turbofish":
		return rustReference{qualified: p.typeName(n, depth+1)}
	case "string_literal", "raw_string_literal":
		return rustReference{typ: "str"}
	case "reference_expression":
		return p.reference(p.field(n, "value"), depth+1)
	case "parenthesized_expression", "try_expression":
		if children := p.named(n); len(children) == 1 {
			return p.reference(children[0], depth+1)
		}
	case "struct_expression":
		return rustReference{typ: p.typeName(p.field(n, "name"), depth+1)}
	case "field_expression":
		ref := p.reference(p.field(n, "value"), depth+1)
		if item := p.engine.item(ref.typ); item != nil {
			index := 0
			for _, field := range item.p.named(item.p.field(item.n, "body")) {
				if item.p.kind(field) == "field_declaration" {
					if item.p.text(item.p.field(field, "name")) == p.text(p.field(n, "field")) {
						return rustReference{typ: item.p.typeName(item.p.field(field, "type"), depth+1)}
					}
				} else if item.p.kind(item.p.field(item.n, "body")) == "ordered_field_declaration_list" && !slices.Contains([]string{"attribute_item", "visibility_modifier"}, item.p.kind(field)) {
					if p.text(p.field(n, "field")) == fmt.Sprint(index) {
						return rustReference{typ: item.p.typeName(field, depth+1)}
					}
					index++
				}
			}
		}
	case "call_expression":
		fn := p.field(n, "function")
		if p.kind(fn) == "generic_function" {
			fn = p.field(fn, "function")
		}
		// 目标和返回类型使用同一个接收者，避免长调用链递归展开两遍。
		var ref rustReference
		var target string
		if p.kind(fn) == "field_expression" {
			ref = p.reference(p.field(fn, "value"), depth+1)
			target = ref.methodTarget(p.text(p.field(fn, "field")))
		} else {
			target = p.reference(fn, depth+1).qualified
		}
		if typ := rust.Rules.ReceiverMethods[target]; typ != "" {
			return rustReference{typ: typ}
		}
		if p.kind(fn) == "field_expression" {
			if slices.Contains(rust.BuilderMethods[ref.typ], p.text(p.field(fn, "field"))) {
				return ref
			}
			if name := p.text(p.field(fn, "field")); name == "unwrap" || name == "expect" || name == "clone" || name == "to_owned" || name == "as_str" {
				return ref
			}
			if ref.typ == "str" && p.text(p.field(fn, "field")) == "to_string" {
				return rustReference{typ: "std::string::String"}
			}
		}
		if bindings := p.engine.symbols[target]; len(bindings) == 1 {
			b := bindings[0]
			if p.kind(b.node) == "struct_item" || p.kind(b.node) == "enum_item" {
				return rustReference{typ: b.qualified}
			}
			return rustReference{typ: b.file.typeName(b.file.field(b.node, "return_type"), depth+1)}
		}
	}
	return rustReference{}
}

func (ref rustReference) methodTarget(name string) string {
	owner := ref.typ
	if owner == "" {
		owner = ref.qualified
	}
	if owner == "" {
		return ""
	}
	return owner + "::" + name
}

func (p *rustFile) callTarget(n *ts.Node, depth int) string {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return ""
	}
	fn := p.field(n, "function")
	if p.kind(fn) == "generic_function" {
		fn = p.field(fn, "function")
	}
	if p.kind(fn) == "field_expression" {
		ref := p.reference(p.field(fn, "value"), depth+1)
		return ref.methodTarget(p.text(p.field(fn, "field")))
	}
	return p.reference(fn, depth+1).qualified
}

func (p *rustFile) literal(n *ts.Node, depth int) (string, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return "", false
	}
	if p.kind(n) == "string_literal" || p.kind(n) == "raw_string_literal" {
		return rustString(p.text(n))
	}
	if p.kind(n) == "reference_expression" {
		return p.literal(p.field(n, "value"), depth+1)
	}
	if p.kind(n) == "parenthesized_expression" {
		if children := p.named(n); len(children) == 1 {
			return p.literal(children[0], depth+1)
		}
	}
	if p.kind(n) == "identifier" {
		if b, bound := p.lookup(p.text(n), n); bound {
			if b == nil || !b.constant || b.changed {
				return "", false
			}
			return b.file.literal(b.value, depth+1)
		}
	}
	if p.kind(n) == "identifier" || p.kind(n) == "scoped_identifier" {
		if list := p.engine.symbols[p.resolve(p.text(n), n, depth+1)]; len(list) == 1 {
			b := list[0]
			if b.constant && !b.changed {
				return b.file.literal(b.value, depth+1)
			}
		}
	}
	return "", false
}

func rustRiskKinds(target string) []string {
	var result []string
	for _, rule := range rust.Rules.Risks {
		match := target == rule.Target
		if strings.HasSuffix(rule.Target, "::*") {
			base := strings.TrimSuffix(rule.Target, "::*")
			match = target == base || strings.HasPrefix(target, base+"::")
		}
		if match {
			found := false
			for _, kind := range result {
				found = found || kind == rule.Kind
			}
			if !found {
				result = append(result, rule.Kind)
			}
		}
	}
	return result
}
