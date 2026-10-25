package codescan

import (
	"context"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang"
	"github.com/Innocent-children/taskbelay/internal/lang/golang"
	"github.com/Innocent-children/taskbelay/internal/lang/java"
	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	"github.com/Innocent-children/taskbelay/internal/lang/python"
	"github.com/Innocent-children/taskbelay/internal/lang/rust"
	ts "github.com/odvcencio/gotreesitter"
)

func languageRules(name string) lang.Rules {
	switch name {
	case "go":
		return golang.Rules
	case "python":
		return python.Rules
	case "javascript", "typescript", "tsx":
		return javascript.Rules
	case "java":
		return java.Rules
	case "rust":
		return rust.Rules
	}
	return lang.Rules{}
}

func functionShape(ctx context.Context, node *ts.Node, language *ts.Language, source []byte, name string, bindings shapeEnvironment) (*FunctionShape, error) {
	kind := func(n *ts.Node) string {
		if n == nil {
			return ""
		}
		return n.Type(language)
	}
	text := func(n *ts.Node) string {
		if n == nil {
			return ""
		}
		return n.Text(source)
	}
	field := func(n *ts.Node, f string) *ts.Node {
		if n == nil {
			return nil
		}
		return n.ChildByFieldName(f, language)
	}
	function := node
	for !callable(kind(function)) {
		switch kind(function) {
		case "decorated_definition", "export_statement":
			var next *ts.Node
			for i := 0; i < function.NamedChildCount(); i++ {
				child := function.NamedChild(i)
				if !strings.Contains(kind(child), "comment") && kind(child) != "decorator" {
					next = child
				}
			}
			function = next
		case "lexical_declaration", "variable_declaration":
			if function.NamedChildCount() != 1 {
				return nil, nil
			}
			function = function.NamedChild(0)
		case "variable_declarator":
			function = field(function, "value")
		case "init_declarator":
			function = field(function, "value")
		default:
			return nil, nil
		}
		if function == nil {
			return nil, nil
		}
	}
	body := field(function, "body")
	if body == nil {
		return nil, nil
	}
	rules := languageRules(name)
	shape := &FunctionShape{Simple: true}
	allowed := map[string]bool{}
	params := field(function, "parameters")
	if params == nil {
		declarator := field(function, "declarator")
		for declarator != nil {
			if params = field(declarator, "parameters"); params != nil {
				break
			}
			declarator = field(declarator, "declarator")
		}
	}
	if params == nil {
		params = field(function, "parameter")
	}
	if params != nil {
		var items []*ts.Node
		if kind(params) == "identifier" {
			items = append(items, params)
		} else {
			for i := 0; i < params.NamedChildCount(); i++ {
				items = append(items, params.NamedChild(i))
			}
		}
		for _, item := range items {
			var names []*ts.Node
			if kind(item) == "identifier" {
				names = append(names, item)
			} else if n := field(item, "name"); n != nil {
				names = append(names, n)
			} else if n := field(item, "pattern"); n != nil {
				names = append(names, n)
			} else if n := field(item, "declarator"); n != nil {
				if name := declaratorName(n, language); name != nil {
					names = append(names, name)
				}
			} else if n := field(item, "left"); n != nil {
				names = append(names, n)
			}
			if name == "go" {
				names = nil
				for i := 0; i < item.NamedChildCount(); i++ {
					if kind(item.NamedChild(i)) == "identifier" {
						names = append(names, item.NamedChild(i))
					}
				}
			}
			if name == "python" && len(names) == 0 && item.NamedChildCount() > 0 {
				names = append(names, item.NamedChild(0))
			}
			if name == "rust" && kind(item) == "self_parameter" {
				allowed["self"] = true
				continue
			}
			for _, n := range names {
				if kind(n) != "identifier" && kind(n) != "variable_name" {
					shape.Simple = false
					continue
				}
				shape.Parameters = append(shape.Parameters, text(n))
				allowed[text(n)] = true
			}
		}
	}
	if receiver := field(function, "receiver"); receiver != nil {
		_ = walkSyntax(ctx, receiver, func(n *ts.Node) {
			if kind(n) == "identifier" {
				allowed[text(n)] = true
			}
		})
	}
	callTarget := func(n *ts.Node) *ts.Node {
		for _, f := range []string{"function", "name", "method"} {
			if target := field(n, f); target != nil {
				return target
			}
		}
		return nil
	}
	isCall := func(n *ts.Node) bool {
		switch kind(n) {
		case "call_expression", "call", "method_invocation", "invocation_expression", "function_call_expression", "member_call_expression", "scoped_call_expression", "object_creation_expression", "new_expression":
			return true
		}
		return false
	}
	var calls []*ts.Node
	fields, construction, computation := false, false, false
	conversions := map[*ts.Node]bool{}
	allowedTargets := []Span{}
	err := walkSyntax(ctx, body, func(n *ts.Node) {
		switch kind(n) {
		case "selector_expression", "member_expression", "field_expression", "attribute", "field_access":
			fields = true
		case "composite_literal", "struct_expression", "object", "object_creation_expression":
			construction = true
		case "binary_expression", "binary_operator", "update_expression", "inc_statement", "dec_statement", "augmented_assignment", "augmented_assignment_expression":
			computation = true
		case "unary_expression", "unary_operator":
			computation = computation || text(field(n, "operator")) != "&"
		}
		branch := false
		switch kind(n) {
		case "if_statement", "if_expression", "for_statement", "for_in_statement", "for_expression", "for_each_statement", "foreach_statement", "while_statement", "while_expression", "do_statement", "do_while_statement", "case_statement", "switch_case", "case_clause", "expression_case", "type_case", "communication_case", "match_arm", "conditional_expression", "ternary_expression", "if", "elsif", "unless", "for", "while", "until", "when", "case_switch_label", "default_switch_label":
			branch = true
		}
		branch = branch || slices.Contains(rules.BranchNodes, kind(n))
		operator := text(field(n, "operator"))
		if operator == "&&" || operator == "||" || operator == "and" || operator == "or" || slices.Contains(rules.BranchOperators, operator) {
			branch = true
		}
		if branch {
			branchSpan := span(n)
			if condition := field(n, "condition"); condition != nil {
				branchSpan.End, branchSpan.EndLine = condition.EndByte(), int(condition.EndPoint().Row)+1
			} else if branchBody := field(n, "body"); branchBody != nil {
				branchSpan.End, branchSpan.EndLine = branchBody.StartByte(), int(branchBody.StartPoint().Row)+1
			}
			shape.Branches = append(shape.Branches, branchSpan)
			shape.Simple = false
		}
		if isCall(n) {
			calls = append(calls, n)
			target := text(callTarget(n))
			allowedCall := slices.Contains(rules.SimpleCalls, target)
			if name == "rust" {
				last := target[strings.LastIndex(target, ".")+1:]
				last = last[strings.LastIndex(last, ":")+1:]
				allowedCall = slices.Contains(rules.SimpleCalls, last)
			}
			if name == "go" && slices.Contains([]string{"string", "bool", "byte", "rune", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64", "complex64", "complex128"}, target) {
				allowedCall = true
			}
			if name == "go" && bindings.types[target] {
				allowedCall = true
			}
			base := strings.Split(target, ".")[0]
			if name != "rust" && (allowed[base] || bindings.shadowed[base] || bindings.shadowed["*"]) {
				allowedCall = false
			}
			if name == "go" && allowedCall {
				conversions[n] = true
			}
			if !allowedCall {
				shape.Simple = false
			} else if target := callTarget(n); target != nil {
				allowedTargets = append(allowedTargets, span(target))
			}
		}
		if (kind(n) == "identifier" || kind(n) == "variable_name") && !allowed[text(n)] {
			parent := n.Parent()
			isField := false
			if name == "go" {
				key := n
				if kind(parent) == "literal_element" {
					key, parent = parent, parent.Parent()
				}
				if kind(parent) == "keyed_element" && field(parent, "key") == key {
					for owner := parent.Parent(); owner != nil; owner = owner.Parent() {
						if kind(owner) == "composite_literal" {
							typ := field(owner, "type")
							isField = kind(typ) == "struct_type" || bindings.structs[text(typ)]
							break
						}
						if callable(kind(owner)) {
							break
						}
					}
				}
				parent = n.Parent()
			}
			for _, f := range []string{"field", "property", "attribute", "name"} {
				if p := field(parent, f); p != nil && p.StartByte() == n.StartByte() && p.EndByte() == n.EndByte() {
					switch kind(parent) {
					case "selector_expression", "member_expression", "field_expression", "attribute", "field_access":
						isField = true
					}
				}
			}
			conversion := parent != nil && isCall(parent) && callTarget(parent) == n && slices.Contains(rules.SimpleCalls, text(n))
			for _, target := range allowedTargets {
				if target.Start <= n.StartByte() && target.End >= n.EndByte() {
					conversion = true
				}
			}
			if name == "go" && parent != nil && (isCall(parent) || kind(parent) == "type_conversion_expression") {
				conversion = true
			}
			if !isField && !conversion {
				shape.Simple = false
			}
		}
		switch kind(n) {
		case "throw_statement", "raise_statement", "yield", "yield_statement", "yield_expression", "await_expression", "go_statement", "send_statement", "receive_statement", "select_statement", "defer_statement", "try_statement", "with_statement", "synchronized_statement", "unsafe_block", "asm_statement", "include_expression", "include_once_expression", "require_expression", "require_once_expression", "eval_expression", "delete_expression":
			shape.Simple = false
		}
		if callable(kind(n)) {
			shape.Simple = false
		}
	})
	if err != nil {
		return nil, err
	}
	if shape.Simple && !computation {
		if construction || (fields && (kind(function) == "constructor_declaration" || text(field(function, "name")) == "__init__" || text(field(function, "name")) == "constructor")) {
			shape.SimpleKind = "field_constructor"
		} else if fields {
			shape.SimpleKind = "field_accessor"
		} else {
			shape.SimpleKind = "value"
		}
	}
	if len(calls) != 1 {
		return shape, nil
	}
	if conversions[calls[0]] {
		return shape, nil
	}
	statement := body
	for slices.Contains([]string{"block", "compound_statement", "statement_block", "statement_list", "body_statement"}, kind(statement)) {
		var statements []*ts.Node
		for i := 0; i < statement.NamedChildCount(); i++ {
			child := statement.NamedChild(i)
			if !strings.Contains(kind(child), "comment") {
				statements = append(statements, child)
			}
		}
		if len(statements) != 1 {
			return shape, nil
		}
		statement = statements[0]
	}
	switch kind(statement) {
	case "return_statement", "expression_statement":
		if statement.NamedChildCount() != 1 {
			return shape, nil
		}
		statement = statement.NamedChild(0)
	}
	if kind(statement) == "expression_list" {
		if statement.NamedChildCount() != 1 {
			return shape, nil
		}
		statement = statement.NamedChild(0)
	}
	if statement.StartByte() != calls[0].StartByte() || statement.EndByte() != calls[0].EndByte() {
		return shape, nil
	}
	args := field(calls[0], "arguments")
	if args == nil {
		return shape, nil
	}
	var arguments []*ts.Node
	for i := 0; i < args.NamedChildCount(); i++ {
		child := args.NamedChild(i)
		if !strings.Contains(kind(child), "comment") {
			arguments = append(arguments, child)
		}
	}
	if len(arguments) != len(shape.Parameters) {
		return shape, nil
	}
	for i, arg := range arguments {
		if kind(arg) == "argument" && arg.NamedChildCount() == 1 {
			arg = arg.NamedChild(0)
		}
		if (kind(arg) != "identifier" && kind(arg) != "variable_name") || text(arg) != shape.Parameters[i] {
			return shape, nil
		}
	}
	shape.Forwarder = text(callTarget(calls[0]))
	return shape, nil
}

type shapeEnvironment struct{ shadowed, types, structs map[string]bool }

func shapeBindings(ctx context.Context, root *ts.Node, language *ts.Language, source []byte) (shapeEnvironment, error) {
	shadowed := map[string]bool{}
	types := map[string]bool{}
	structs := map[string]bool{}
	err := walkSyntax(ctx, root, func(n *ts.Node) {
		switch n.Type(language) {
		case "import_statement", "import_from_statement", "import_spec":
			_ = walkSyntax(ctx, n, func(child *ts.Node) {
				switch child.Type(language) {
				case "identifier", "package_identifier":
					shadowed[child.Text(source)] = true
				case "wildcard_import", "dot":
					shadowed["*"] = true
				}
			})
		case "type_spec", "type_alias":
			if child := n.ChildByFieldName("name", language); child != nil {
				types[child.Text(source)] = true
				if typ := n.ChildByFieldName("type", language); typ != nil && typ.Type(language) == "struct_type" {
					structs[child.Text(source)] = true
				}
			}
		case "function_definition", "function_declaration", "variable_declarator", "parameter_declaration", "formal_parameter", "required_parameter", "optional_parameter", "aliased_import", "import_specifier", "namespace_import", "import_clause":
			for _, field := range []string{"name", "alias", "pattern"} {
				if child := n.ChildByFieldName(field, language); child != nil {
					shadowed[child.Text(source)] = true
				}
			}
		case "var_spec", "const_spec":
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child.Type(language) == "identifier" {
					shadowed[child.Text(source)] = true
				}
			}
		case "assignment", "assignment_expression", "short_var_declaration":
			left := n.ChildByFieldName("left", language)
			if left != nil {
				if left.Type(language) == "identifier" {
					shadowed[left.Text(source)] = true
				} else {
					for i := 0; i < left.NamedChildCount(); i++ {
						child := left.NamedChild(i)
						if child.Type(language) == "identifier" {
							shadowed[child.Text(source)] = true
						}
					}
				}
			}
		}
	})
	return shapeEnvironment{shadowed: shadowed, types: types, structs: structs}, err
}
