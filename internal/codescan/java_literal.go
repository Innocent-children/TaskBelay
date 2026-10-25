package codescan

import (
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *javaFile) literal(n *ts.Node, depth int) (Value, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return Value{}, false
	}
	switch p.kind(n) {
	case "true", "false", "boolean_literal":
		return Value{Kind: "boolean", Text: p.text(n)}, true
	case "null_literal":
		return Value{Kind: "null", Text: "null"}, true
	case "character_literal":
		value, err := strconv.Unquote(p.text(n))
		return Value{Kind: "char", Text: value}, err == nil
	case "decimal_integer_literal", "hex_integer_literal", "octal_integer_literal", "binary_integer_literal", "decimal_floating_point_literal", "hex_floating_point_literal":
		return javaNumber(p.text(n))
	case "unary_expression":
		operator := p.text(p.field(n, "operator"))
		operand := p.field(n, "operand")
		if operator == "-" || operator == "+" {
			if strings.HasSuffix(p.kind(operand), "_literal") {
				return javaNumber(operator + p.text(operand))
			}
		}
	case "string_literal":
		value, err := strconv.Unquote(p.text(n))
		return Value{Kind: "String", Text: value}, err == nil
	case "identifier":
		if binding, bound := p.lookup(p.text(n), n); bound {
			return p.literalBinding(binding, depth+1)
		}
		if names := p.staticImports[p.text(n)]; len(names) == 1 {
			if list := p.engine.members[names[0]]; len(list) == 1 {
				return list[0].file.literalBinding(list[0], depth+1)
			}
		}
		var matches []*javaBinding
		for _, prefix := range p.staticStars {
			if !p.engine.knownTypes[prefix] && len(p.engine.types[prefix]) != 1 {
				return Value{}, false
			}
			matches = append(matches, p.engine.members[prefix+"."+p.text(n)]...)
		}
		if len(matches) == 1 {
			return matches[0].file.literalBinding(matches[0], depth+1)
		}
	case "field_access":
		ref := p.reference(p.field(n, "object"), depth+1)
		owner := ref.qualified
		if ref.typ != "" {
			owner = ref.typ
		}
		if list := p.engine.memberBindings(owner, p.text(p.field(n, "field")), depth+1); len(list) == 1 {
			return list[0].file.literalBinding(list[0], depth+1)
		}
	case "binary_expression":
		if p.text(p.field(n, "operator")) == "+" {
			left, lok := p.literal(p.field(n, "left"), depth+1)
			right, rok := p.literal(p.field(n, "right"), depth+1)
			if lok && rok && left.Kind == "String" && right.Kind == "String" {
				return Value{Kind: "String", Text: left.Text + right.Text}, true
			}
		}
	case "parenthesized_expression":
		if children := p.named(n); len(children) == 1 {
			return p.literal(children[0], depth+1)
		}
	}
	return Value{}, false
}
func (p *javaFile) literalBinding(binding *javaBinding, depth int) (Value, bool) {
	if binding == nil || !binding.constant || depth > 40 {
		return Value{}, false
	}
	return binding.file.literal(binding.value, depth+1)
}
func (p *javaFile) strings(n *ts.Node) ([]string, bool) {
	if p.kind(n) == "element_value_array_initializer" || p.kind(n) == "array_initializer" {
		var values []string
		complete := true
		for _, child := range p.named(n) {
			value, ok := p.literal(child, 0)
			if !ok || value.Kind != "String" {
				complete = false
				continue
			}
			values = append(values, value.Text)
		}
		return values, complete
	}
	value, ok := p.literal(n, 0)
	if !ok || value.Kind != "String" {
		return nil, false
	}
	return []string{value.Text}, true
}

func javaConfigNames(value string) ([]string, bool) {
	var result []string
	for {
		start := strings.Index(value, "${")
		if start < 0 {
			return result, true
		}
		value = value[start+2:]
		end := strings.IndexByte(value, '}')
		if end < 0 {
			return result, false
		}
		body := value[:end]
		if strings.Contains(body, "${") {
			return result, false
		}
		name, _, _ := strings.Cut(body, ":")
		if name == "" {
			return result, false
		}
		result = append(result, name)
		value = value[end+1:]
	}
}
