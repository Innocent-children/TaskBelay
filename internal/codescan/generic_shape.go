package codescan

import (
	"slices"

	ts "github.com/odvcencio/gotreesitter"
)

func (a *genericAnalysis) genericShape(item *genericDeclaration) {
	shape := item.d.Shape
	if shape == nil || shape.Simple || len(shape.Branches) > 0 {
		return
	}
	p := item.p
	body := p.field(item.n, "body")
	if body == nil {
		return
	}
	simple := true
	err := walkSyntax(a.ctx, body, func(n *ts.Node) {
		if !simple {
			return
		}
		kind := p.kind(n)
		if !slices.Contains([]string{
			"block", "compound_statement", "body_statement", "statement_list", "return_statement", "expression_statement",
			"assignment_expression", "assignment", "operator_assignment", "binary_expression", "binary", "unary_expression", "unary",
			"update_expression", "prefix_unary_expression", "postfix_unary_expression", "parenthesized_expression",
			"field_expression", "member_access_expression", "member_access", "member_expression",
			"identifier", "field_identifier", "variable_name", "instance_variable", "name", "this", "self",
			"integer", "integer_literal", "number_literal", "float", "float_literal", "real_literal", "decimal_integer_literal",
			"boolean", "boolean_literal", "true", "false", "null", "nil", "null_literal", "string", "string_literal", "char_literal", "character_literal", "encapsed_string", "string_content", "string_fragment", "escape_sequence",
			"comment", "line_comment", "block_comment",
		}, kind) {
			simple = false
			return
		}
		if slices.Contains([]string{"field_expression", "member_access_expression", "member_access", "member_expression"}, kind) {
			simple = a.slotRef(p, n).item != nil
			return
		}
		if !slices.Contains([]string{"identifier", "field_identifier", "variable_name", "instance_variable", "name"}, kind) {
			return
		}
		parent := p.parent(n)
		if p.kind(parent) == "variable_name" || slices.Contains([]string{"this", "self", "$this"}, p.text(n)) {
			return
		}
		if slices.Contains([]string{"field_expression", "member_access_expression", "member_access", "member_expression"}, p.kind(parent)) {
			for _, key := range []string{"field", "property", "name", "member"} {
				if p.field(parent, key) == n {
					return
				}
			}
		}
		simple = a.slotRef(p, n).item != nil
	})
	shape.Simple = simple && err == nil
}
