package codescan

import (
	"slices"
	"strings"
	"unicode"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *pyFile) analyzeTests() {
	defer finishTests(p.engine.ctx, p.file)
	if !p.file.Kind.Test {
		return
	}
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		if p.kind(n) != "function_definition" || !strings.HasPrefix(p.text(p.field(n, "name")), "test") {
			return
		}
		name := p.text(p.field(n, "name"))
		framework := "pytest"
		for parent := n.Parent(); parent != nil; parent = parent.Parent() {
			if p.kind(parent) == "function_definition" {
				return
			}
			if p.kind(parent) == "class_definition" {
				item := p.byNode[p.key(parent)]
				unittest := pyUnittest(item, map[*pyDeclaration]bool{})
				if !strings.HasPrefix(p.text(p.field(parent, "name")), "Test") && !unittest {
					return
				}
				if unittest {
					framework = "unittest"
				}
				name = p.text(p.field(parent, "name")) + "::" + name
			}
		}
		declaration := ""
		if owner := p.owner(n); owner != nil {
			declaration = owner.d.ID
		}
		test := sourceTest(p.engine.ctx, p.file, p.syntax, n, name, framework, declaration)
		if p.kind(n.Parent()) == "decorated_definition" {
			test.Context = append(test.Context, span(n.Parent()))
			test.CodeHash = digest(test.CodeHash + p.text(n.Parent()))
		}
		p.testAssertions(n, &test)
		cases := []TestCase{test}
		for _, decorator := range p.testDecorators(n) {
			if p.kind(decorator) != "call" || p.reference(p.field(decorator, "function"), 0).qualified != "pytest.mark.parametrize" {
				continue
			}
			names, rows := p.parameterRows(decorator)
			if len(names) == 0 {
				test.Unknown = append(test.Unknown, "参数组与 pytest node id 不能唯一对应，保留外层测试")
				cases = []TestCase{test}
				break
			}
			var expanded []TestCase
			for i, id := range names {
				for _, current := range cases {
					currentID := strings.TrimSuffix(strings.TrimPrefix(current.Name, name+"["), "]")
					if current.Name != name {
						idWithParent := currentID + "-" + id
						expanded = append(expanded, parameterTest(current, name+"["+idWithParent+"]", rows[i], p.file.Data))
					} else {
						expanded = append(expanded, parameterTest(current, name+"["+id+"]", rows[i], p.file.Data))
					}
				}
			}
			cases = expanded
		}
		p.file.Tests = append(p.file.Tests, cases...)
	})
}

func pyUnittest(item *pyDeclaration, seen map[*pyDeclaration]bool) bool {
	if item == nil || seen[item] {
		return false
	}
	seen[item] = true
	if slices.Contains(item.bases, "unittest.TestCase") || slices.Contains(item.bases, "unittest.case.TestCase") {
		return true
	}
	for _, parent := range item.parents {
		if pyUnittest(parent, seen) {
			return true
		}
	}
	return false
}

func (p *pyFile) testDecorators(n *ts.Node) []*ts.Node {
	var result []*ts.Node
	for node := n; node != nil; node = node.Parent() {
		if p.kind(node) != "decorated_definition" {
			continue
		}
		for _, child := range namedNodes(node, p.syntax.language) {
			if p.kind(child) == "decorator" && child.NamedChildCount() > 0 {
				result = append(result, child.NamedChild(0))
			}
		}
	}
	// pytest applies the innermost parameter decorator first.
	slices.Reverse(result)
	return result
}

func (p *pyFile) parameterRows(call *ts.Node) ([]string, [][]*ts.Node) {
	args := p.arguments(call)
	argNames := pyArgumentAt(args, 0, "argnames")
	argCount, forceTuple := 0, false
	if names, known := p.literal(argNames, 0); known && names.Kind == "String" {
		for _, name := range strings.Split(names.Text, ",") {
			if strings.TrimSpace(name) != "" {
				argCount++
			}
		}
		forceTuple = argCount == 1
	} else if p.kind(argNames) == "list" || p.kind(argNames) == "tuple" {
		argCount = len(namedNodes(argNames, p.syntax.language))
	}
	if argCount == 0 {
		return nil, nil
	}
	values := pyArgumentAt(args, 1, "argvalues")
	if p.kind(values) != "list" && p.kind(values) != "tuple" {
		return nil, nil
	}
	var ids *ts.Node
	for _, arg := range args {
		if arg.name == "ids" {
			ids = arg.value
			break
		}
	}
	var idNodes []*ts.Node
	if ids != nil {
		if p.kind(ids) != "list" && p.kind(ids) != "tuple" {
			return nil, nil
		}
		idNodes = namedNodes(ids, p.syntax.language)
	}
	rows := namedNodes(values, p.syntax.language)
	if ids != nil && len(idNodes) != len(rows) {
		return nil, nil
	}
	var names []string
	var sources [][]*ts.Node
	seen := map[string]bool{}
	for i, row := range rows {
		if p.engine.ctx.Err() != nil {
			return nil, nil
		}
		parts := []*ts.Node{row}
		explicitID := (*ts.Node)(nil)
		if !forceTuple && (p.kind(row) == "tuple" || p.kind(row) == "list") {
			parts = namedNodes(row, p.syntax.language)
		} else if p.kind(row) == "call" && p.reference(p.field(row, "function"), 0).qualified == "pytest.param" {
			parts = nil
			for _, arg := range p.arguments(row) {
				if arg.name == "id" {
					explicitID = arg.value
				} else if arg.name == "" && !arg.spread {
					parts = append(parts, arg.value)
				}
			}
		}
		if explicitID == nil && len(idNodes) > 0 {
			explicitID = idNodes[i]
		}
		if p.kind(explicitID) == "none" {
			explicitID = nil
		}
		if len(parts) != argCount {
			return nil, nil
		}
		var namesOfParts []string
		if explicitID != nil {
			parts = []*ts.Node{explicitID}
		}
		for _, part := range parts {
			value, ok := p.literal(part, 0)
			if !ok || !slices.Contains([]string{"String", "Int", "Bool", "nil"}, value.Kind) {
				return nil, nil
			}
			if value.Kind == "Bool" {
				value.Text = strings.ToUpper(value.Text[:1]) + value.Text[1:]
			}
			if strings.ContainsFunc(value.Text, func(r rune) bool { return r > unicode.MaxASCII || unicode.IsControl(r) }) {
				return nil, nil
			}
			namesOfParts = append(namesOfParts, value.Text)
		}
		name := strings.Join(namesOfParts, "-")
		if name == "" || seen[name] {
			return nil, nil
		}
		seen[name] = true
		names = append(names, name)
		sources = append(sources, []*ts.Node{row})
	}
	return names, sources
}

func (p *pyFile) testAssertions(function *ts.Node, test *TestCase) {
	_ = walkSyntax(p.engine.ctx, p.field(function, "body"), func(n *ts.Node) {
		for parent := n.Parent(); parent != nil && parent != function; parent = parent.Parent() {
			if p.kind(parent) == "function_definition" || p.kind(parent) == "lambda" {
				return
			}
		}
		if p.kind(n) == "assert_statement" {
			kind := "mock"
			found := false
			_ = walkSyntax(p.engine.ctx, n, func(child *ts.Node) {
				if p.kind(child) == "call" {
					kind = "result"
				}
				if p.kind(child) == "attribute" {
					q := p.reference(child, 0).qualified
					member := p.text(p.field(child, "attribute"))
					if strings.HasPrefix(q, "unittest.mock.") && slices.Contains([]string{"called", "call_count", "call_args"}, member) {
						found = true
					} else if p.kind(child.Parent()) != "attribute" {
						kind = "result"
					}
				}
			})
			if !found {
				kind = "result"
			}
			addAssertion(test, kind, n, p.file.Data)
		}
		if p.kind(n) != "call" {
			return
		}
		fn := p.field(n, "function")
		q := p.reference(fn, 0).qualified
		member := p.text(p.field(fn, "attribute"))
		kind := ""
		if strings.HasPrefix(q, "unittest.mock.") && slices.Contains([]string{"assert_called", "assert_called_once", "assert_called_with", "assert_called_once_with", "assert_any_call", "assert_has_calls", "assert_not_called"}, member) {
			kind = "mock"
		}
		if q == "pytest.raises" || q == "pytest.fail" || (test.Framework == "unittest" && strings.HasPrefix(member, "assert") && p.text(p.field(fn, "object")) == "self") {
			kind = "result"
		}
		addAssertion(test, kind, n, p.file.Data)
	})
}
