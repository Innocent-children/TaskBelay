package codescan

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
)

func (g *goFile) analyzeTests() {
	defer finishTests(g.ctx, g.file)
	if !g.file.Kind.Test {
		g.file.Operations["tests"] = Operation{Complete: true}
		return
	}
	root := g.syntax.tree.RootNode()
	for i := 0; i < root.NamedChildCount(); i++ {
		function := root.NamedChild(i)
		if g.kind(function) != "function_declaration" {
			continue
		}
		name := g.text(g.field(function, "name"))
		if !strings.HasPrefix(name, "Test") {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(strings.TrimPrefix(name, "Test")); r != utf8.RuneError && unicode.IsLower(r) {
			continue
		}
		parameters := g.field(function, "parameters")
		if parameters == nil || parameters.NamedChildCount() != 1 || g.field(function, "result") != nil || g.field(function, "type_parameters") != nil {
			continue
		}
		parameter := parameters.NamedChild(0)
		if parameter.NamedChildCount() > 2 || g.kind(g.field(parameter, "type")) != "pointer_type" || g.typeName(g.field(parameter, "type")) != "testing.T" {
			continue
		}
		cases, reason := g.testCases(function, name, g.owner(function))
		if reason != "" {
			one := g.testCase(function, name, g.owner(function))
			one.Unknown = append(one.Unknown, reason)
			cases = []TestCase{one}
			g.missing("tests", function, reason)
		}
		g.file.Tests = append(g.file.Tests, cases...)
	}
	complete := g.file.CodeComplete && g.ctx.Err() == nil
	for _, missing := range g.file.Missing {
		if missing.Operation == "tests" {
			complete = false
		}
	}
	g.file.Operations["tests"] = Operation{Complete: complete}
	if !complete {
		g.file.Operations["tests"] = Operation{Reason: "存在未能唯一核对的测试用例或语法"}
	}
}

func (g *goFile) testCases(function *ts.Node, prefix, declaration string) ([]TestCase, string) {
	if g.field(function, "body") == nil {
		return nil, "测试函数缺少可分析函数体"
	}
	var runs []*ts.Node
	err := walkSyntax(g.ctx, g.field(function, "body"), func(n *ts.Node) {
		if g.kind(n) != "call_expression" || g.enclosingCallable(n) != function {
			return
		}
		target, _ := g.target(g.field(n, "function"), 0)
		if target == "testing.T.Run" {
			runs = append(runs, n)
		}
	})
	if err != nil {
		return nil, err.Error()
	}
	if len(runs) == 0 {
		return []TestCase{g.testCase(function, prefix, declaration)}, ""
	}
	var result []TestCase
	shared := g.testCase(function, prefix, declaration)
	seen := map[string]bool{}
	for _, run := range runs {
		args := g.arguments(run)
		if len(args) != 2 || g.kind(args[1]) != "func_literal" {
			return nil, "子测试回调不能定位到当前源码，保留外层整体"
		}
		names, rows := []string{}, []Span{}
		value := g.value(args[0], 0)
		if value.constant {
			for parent := run.Parent(); parent != nil && parent != function; parent = parent.Parent() {
				if g.kind(parent) == "for_statement" {
					return nil, "循环中的固定子测试标签可能重复，保留外层整体"
				}
			}
			names = append(names, value.text)
		} else {
			names, rows = g.tableNames(function, run, args[0])
		}
		if len(names) == 0 {
			return nil, "子测试名字与参数行不能唯一对应，保留外层整体"
		}
		for i, name := range names {
			if name == "" || seen[name] {
				return nil, "子测试标签为空或重复，运行名需由 Runner 核对"
			}
			seen[name] = true
			children, reason := g.testCases(args[1], prefix+"/"+name, declaration)
			if reason != "" {
				return nil, reason
			}
			for j := range children {
				children[j].CodeHash = digest(shared.CodeHash + children[j].CodeHash)
				children[j].Context = append(children[j].Context, shared.Span)
				children[j].Assertions = append(append([]Assertion(nil), shared.Assertions...), children[j].Assertions...)
				children[j].Unknown = append(children[j].Unknown, shared.Unknown...)
				children[j].References = append(children[j].References, shared.References...)
				slices.Sort(children[j].References)
				children[j].References = slices.Compact(children[j].References)
			}
			if len(rows) > 0 {
				for j := range children {
					children[j].Parameters = append(children[j].Parameters, rows[i])
					children[j].CodeHash = digest(children[j].CodeHash + string(g.file.Data[rows[i].Start:rows[i].End]))
				}
			}
			result = append(result, children...)
		}
	}
	seen = map[string]bool{}
	for _, test := range result {
		if seen[test.Name] {
			return nil, "展开后的子测试名字重复，保留外层整体"
		}
		seen[test.Name] = true
	}
	return result, ""
}

func (g *goFile) testCase(function *ts.Node, name, declaration string) TestCase {
	result := TestCase{File: g.file.Path, Name: name, Framework: "go", Declaration: declaration, Span: span(function)}
	tokens, err := leafTokens(g.ctx, function, g.syntax.language, g.file.Data, nil)
	if err != nil {
		result.Unknown = append(result.Unknown, err.Error())
	}
	var code strings.Builder
	for _, token := range tokens {
		if token.Kind != "comment" {
			code.WriteString(token.Kind + "\x00" + token.Text + "\x00")
		}
	}
	result.CodeHash = digest(code.String())
	if g.field(function, "body") == nil {
		return result
	}
	names := map[string]bool{}
	_ = walkSyntax(g.ctx, g.field(function, "body"), func(n *ts.Node) {
		if g.enclosingCallable(n) != function {
			return
		}
		if slices.Contains([]string{"identifier", "field_identifier", "type_identifier"}, g.kind(n)) {
			names[g.text(n)] = true
		}
		if g.kind(n) != "call_expression" {
			return
		}
		target, _ := g.target(g.field(n, "function"), 0)
		kind := ""
		fn := g.field(n, "function")
		if g.kind(fn) == "selector_expression" && g.text(g.field(fn, "field")) == "EXPECT" && g.mockReceiver(g.field(fn, "operand"), 0) {
			kind = "mock"
		}
		for _, prefix := range []string{"github.com/stretchr/testify/assert.", "github.com/stretchr/testify/require."} {
			if strings.HasPrefix(target, prefix) && target != prefix+"New" {
				kind = "result"
			}
		}
		if strings.HasPrefix(target, "testing.T.") && slices.Contains([]string{"Error", "Errorf", "Fatal", "Fatalf", "Fail", "FailNow"}, strings.TrimPrefix(target, "testing.T.")) {
			kind = "result"
		}
		if (strings.HasPrefix(target, "github.com/golang/mock/gomock.") || strings.HasPrefix(target, "go.uber.org/mock/gomock.")) && strings.HasSuffix(target, ".Finish") {
			kind = "mock"
		}
		if strings.HasPrefix(target, "github.com/stretchr/testify/mock.") && slices.Contains([]string{"AssertExpectations", "AssertCalled", "AssertNotCalled", "AssertNumberOfCalls"}, target[strings.LastIndex(target, ".")+1:]) {
			kind = "mock"
		}
		if kind == "" && (target == "" || strings.HasPrefix(target, "local:")) {
			method := g.text(g.field(g.field(n, "function"), "field"))
			if method == "EXPECT" || method == "AssertExpectations" || method == "AssertCalled" || method == "AssertNotCalled" || method == "AssertNumberOfCalls" {
				result.Unknown = append(result.Unknown, "调用 "+g.text(n)+" 的替身接收者尚未核对")
			}
		}
		if kind != "" {
			result.Assertions = append(result.Assertions, Assertion{Kind: kind, Evidence: g.text(n), Span: span(n)})
		}
	})
	for name := range names {
		result.References = append(result.References, name)
	}
	slices.Sort(result.References)
	return result
}

/**
 * 只拆分当前函数内、单次定义的字面表格。运行时重命名和动态参数交由 Runner，
 * 这里不执行测试，也不把不能对应的表格行当作独立可删除用例。
 */
func (g *goFile) tableNames(function, run, name *ts.Node) ([]string, []Span) {
	if g.kind(name) != "selector_expression" {
		return nil, nil
	}
	rowName, fieldName := g.text(g.field(name, "operand")), g.text(g.field(name, "field"))
	var loop *ts.Node
	for parent := run.Parent(); parent != nil && parent != function; parent = parent.Parent() {
		if g.kind(parent) == "for_statement" {
			loop = parent
			break
		}
	}
	if loop == nil {
		return nil, nil
	}
	var clause *ts.Node
	for i := 0; i < loop.NamedChildCount(); i++ {
		if g.kind(loop.NamedChild(i)) == "range_clause" {
			clause = loop.NamedChild(i)
		}
	}
	if clause == nil {
		return nil, nil
	}
	left, right := g.field(clause, "left"), g.field(clause, "right")
	if left == nil || left.NamedChildCount() != 2 || g.text(left.NamedChild(1)) != rowName || g.kind(right) != "identifier" {
		return nil, nil
	}
	var literal *ts.Node
	definitions := 0
	_ = walkSyntax(g.ctx, g.field(function, "body"), func(n *ts.Node) {
		if n.StartByte() >= loop.StartByte() {
			return
		}
		if g.enclosingCallable(n) != function {
			return
		}
		if g.kind(n) == "var_spec" && g.text(g.field(n, "name")) == g.text(right) {
			definitions++
			value := g.field(n, "value")
			if value != nil && value.NamedChildCount() == 1 && g.kind(value.NamedChild(0)) == "composite_literal" {
				literal = value.NamedChild(0)
			}
			return
		}
		if g.kind(n) != "short_var_declaration" && g.kind(n) != "assignment_statement" {
			return
		}
		lhs, rhs := g.field(n, "left"), g.field(n, "right")
		if lhs == nil || rhs == nil || lhs.NamedChildCount() != 1 || g.text(lhs.NamedChild(0)) != g.text(right) {
			return
		}
		definitions++
		if rhs.NamedChildCount() == 1 && g.kind(rhs.NamedChild(0)) == "composite_literal" {
			literal = rhs.NamedChild(0)
		}
	})
	if definitions != 1 || literal == nil {
		return nil, nil
	}
	body := g.field(literal, "body")
	if body == nil {
		return nil, nil
	}
	var names []string
	var rows []Span
	fieldIndex := -1
	typeNode := g.field(literal, "type")
	if typeNode != nil && g.kind(typeNode) == "slice_type" {
		element := g.field(typeNode, "element")
		if element != nil && g.kind(element) == "struct_type" {
			index := 0
			_ = walkSyntax(g.ctx, element, func(n *ts.Node) {
				if g.kind(n) != "field_declaration" {
					return
				}
				for i := 0; i < n.NamedChildCount(); i++ {
					child := n.NamedChild(i)
					if g.kind(child) == "field_identifier" {
						if g.text(child) == fieldName {
							fieldIndex = index
						}
						index++
					}
				}
			})
		}
	}
	for i := 0; i < body.NamedChildCount(); i++ {
		row := body.NamedChild(i)
		if g.kind(row) == "comment" {
			continue
		}
		for g.kind(row) == "literal_element" && row.NamedChildCount() == 1 {
			row = row.NamedChild(0)
		}
		if g.kind(row) == "composite_literal" {
			row = g.field(row, "body")
		}
		if row == nil || g.kind(row) != "literal_value" {
			return nil, nil
		}
		found := ""
		position := 0
		for j := 0; j < row.NamedChildCount(); j++ {
			entry := row.NamedChild(j)
			if g.kind(entry) == "comment" {
				continue
			}
			if g.kind(entry) == "keyed_element" && g.text(g.field(entry, "key")) == fieldName {
				value := g.value(g.field(entry, "value"), 0)
				if !value.constant {
					return nil, nil
				}
				found = value.text
			}
			if g.kind(entry) != "keyed_element" && position == fieldIndex {
				value := g.value(entry, 0)
				if !value.constant {
					return nil, nil
				}
				found = value.text
			}
			position++
		}
		if found == "" {
			return nil, nil
		}
		names = append(names, found)
		rows = append(rows, span(row))
	}
	return names, rows
}
