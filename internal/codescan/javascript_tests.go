package codescan

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *jsFile) testAPI(n *ts.Node) (string, string) {
	q := p.reference(n, 0).qualified
	for _, module := range []string{"vitest", "@jest/globals"} {
		if strings.HasPrefix(q, module+".") {
			return strings.TrimPrefix(q, module+"."), module
		}
	}
	if p.kind(n) == "identifier" {
		name := p.text(n)
		if _, bound := p.lookup(name, n); !bound && slices.Contains([]string{"test", "it", "describe", "expect"}, name) {
			return name, "jest/vitest"
		}
	}
	if p.kind(n) == "member_expression" {
		base, framework := p.testAPI(p.field(n, "object"))
		if base != "" {
			return base + "." + p.text(p.field(n, "property")), framework
		}
	}
	return "", ""
}

func (p *jsFile) testDefinition(call *ts.Node) (string, string, *ts.Node) {
	fn := p.field(call, "function")
	var rows *ts.Node
	if p.kind(fn) == "call_expression" {
		name, framework := p.testAPI(p.field(fn, "function"))
		if !strings.HasSuffix(name, ".each") {
			return "", "", nil
		}
		if args := p.arguments(fn); len(args) == 1 {
			rows = args[0]
		}
		return name, framework, rows
	}
	name, framework := p.testAPI(fn)
	return name, framework, nil
}

func (p *jsFile) analyzeTests() {
	defer finishTests(p.engine.ctx, p.file)
	if !p.file.Kind.Test {
		return
	}
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(call *ts.Node) {
		if p.kind(call) != "call_expression" {
			return
		}
		api, framework, rows := p.testDefinition(call)
		base := strings.Split(api, ".")[0]
		if base != "test" && base != "it" {
			return
		}
		args := p.arguments(call)
		if len(args) < 2 || !jsCallable(p.kind(args[1])) {
			return
		}
		label, known := p.literal(args[0], 0)
		name := label.Text
		if !known || label.Kind != "String" {
			name = fmt.Sprintf("<动态名字:%d>", span(call).StartLine)
		}
		test := sourceTest(p.engine.ctx, p.file, p.syntax, call, name, framework, p.owner(call))
		if !known || label.Kind != "String" {
			test.Unknown = append(test.Unknown, "测试名字需由 Runner 核对")
		}
		var parents []string
		for parent := call.Parent(); parent != nil; parent = parent.Parent() {
			if p.kind(parent) == "call_expression" {
				parentAPI, _, _ := p.testDefinition(parent)
				if strings.Split(parentAPI, ".")[0] == "describe" {
					parentArgs := p.arguments(parent)
					if len(parentArgs) > 0 {
						value, ok := p.literal(parentArgs[0], 0)
						if !ok || value.Kind != "String" || strings.HasSuffix(parentAPI, ".each") {
							test.Unknown = append(test.Unknown, "describe 名字或参数组需由 Runner 核对")
						} else {
							parents = append(parents, value.Text)
						}
					}
				}
			}
			if slices.Contains([]string{"for_statement", "for_in_statement", "while_statement"}, p.kind(parent)) {
				test.Unknown = append(test.Unknown, "循环注册的测试需由 Runner 核对实际用例")
			}
		}
		slices.Reverse(parents)
		p.testAssertions(args[1], &test)
		if strings.HasSuffix(api, ".each") && len(test.Unknown) == 0 {
			cases := p.eachCases(test, rows)
			if len(cases) > 0 {
				for i := range cases {
					cases[i].Name = strings.Join(append(slices.Clone(parents), cases[i].Name), " > ")
				}
				p.file.Tests = append(p.file.Tests, cases...)
				return
			}
			test.Unknown = append(test.Unknown, "each 参数行与运行名不能唯一对应，保留外层测试")
		}
		test.Name = strings.Join(append(parents, test.Name), " > ")
		p.file.Tests = append(p.file.Tests, test)
	})
	counts := map[string]int{}
	for _, test := range p.file.Tests {
		counts[test.Name]++
	}
	for i := range p.file.Tests {
		if counts[p.file.Tests[i].Name] > 1 {
			p.file.Tests[i].Unknown = append(p.file.Tests[i].Unknown, "重复测试名字不能精确选择一例")
		}
	}
}

func (p *jsFile) eachCases(test TestCase, rows *ts.Node) []TestCase {
	if p.kind(rows) != "array" {
		return nil
	}
	var cases []TestCase
	seen := map[string]bool{}
	for i, row := range namedNodes(rows, p.syntax.language) {
		parts := []*ts.Node{row}
		if p.kind(row) == "array" {
			parts = namedNodes(row, p.syntax.language)
		}
		position := 0
		var name strings.Builder
		for index := 0; index < len(test.Name); index++ {
			c := test.Name[index]
			if c == '$' {
				end := index + 1
				for end < len(test.Name) && ((test.Name[end] >= 'a' && test.Name[end] <= 'z') || (test.Name[end] >= 'A' && test.Name[end] <= 'Z') || (test.Name[end] >= '0' && test.Name[end] <= '9') || test.Name[end] == '_') {
					end++
				}
				key := test.Name[index+1 : end]
				var found *ts.Node
				if p.kind(row) == "object" && key != "" {
					for _, pair := range namedNodes(row, p.syntax.language) {
						if p.propertyName(p.field(pair, "key")) == key {
							found = p.field(pair, "value")
						}
					}
				}
				value, ok := p.literal(found, 0)
				if !ok || !slices.Contains([]string{"String", "Number", "Bool", "null"}, value.Kind) {
					return nil
				}
				name.WriteString(value.Text)
				index = end - 1
				continue
			}
			if c != '%' {
				name.WriteByte(c)
				continue
			}
			index++
			if index >= len(test.Name) {
				return nil
			}
			switch test.Name[index] {
			case '%':
				name.WriteByte('%')
			case '#':
				name.WriteString(strconv.Itoa(i))
			case 's', 'd', 'i', 'f':
				if position >= len(parts) {
					return nil
				}
				value, ok := p.literal(parts[position], 0)
				position++
				if !ok || (test.Name[index] != 's' && value.Kind != "Number") || !slices.Contains([]string{"String", "Number", "Bool"}, value.Kind) {
					return nil
				}
				if test.Name[index] == 'i' || test.Name[index] == 'd' {
					if _, err := strconv.ParseInt(value.Text, 10, 64); err != nil {
						return nil
					}
				}
				name.WriteString(value.Text)
			default:
				return nil
			}
		}
		if name.Len() == 0 || seen[name.String()] {
			return nil
		}
		seen[name.String()] = true
		cases = append(cases, parameterTest(test, name.String(), []*ts.Node{row}, p.file.Data))
	}
	return cases
}

func (p *jsFile) testAssertions(function *ts.Node, test *TestCase) {
	_ = walkSyntax(p.engine.ctx, p.field(function, "body"), func(n *ts.Node) {
		if p.kind(n) != "call_expression" {
			return
		}
		fn := p.field(n, "function")
		q := p.reference(fn, 0).qualified
		if q == "node:assert" || q == "assert" || strings.HasPrefix(q, "node:assert.") || strings.HasPrefix(q, "assert.") {
			addAssertion(test, "result", n, p.file.Data)
			return
		}
		if p.kind(fn) != "member_expression" {
			return
		}
		matcher := p.text(p.field(fn, "property"))
		base := p.field(fn, "object")
		for p.kind(base) == "member_expression" {
			base = p.field(base, "object")
		}
		if p.kind(base) != "call_expression" {
			return
		}
		api, _ := p.testAPI(p.field(base, "function"))
		if api != "expect" {
			return
		}
		kind := "result"
		if slices.Contains([]string{"toHaveBeenCalled", "toHaveBeenCalledTimes", "toHaveBeenCalledWith", "toHaveBeenLastCalledWith", "toHaveBeenNthCalledWith", "toBeCalled", "toBeCalledTimes", "toBeCalledWith", "lastCalledWith", "nthCalledWith"}, matcher) {
			kind = "mock"
		}
		addAssertion(test, kind, n, p.file.Data)
	})
}
