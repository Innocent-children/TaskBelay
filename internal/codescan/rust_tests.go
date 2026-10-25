package codescan

import (
	"fmt"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *rustFile) analyzeTests() {
	defer finishTests(p.engine.ctx, p.file)
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		if p.kind(n) != "function_item" {
			return
		}
		ordinary, rstest := false, false
		var rows []*ts.Node
		var labels []string
		for _, attribute := range p.attributes(n) {
			name, _ := p.attributeParts(attribute)
			resolved := p.resolve(name, n, 0)
			ordinary = ordinary || name == "test" || resolved == "tokio::test"
			rstest = rstest || resolved == "rstest::rstest"
			if name == "case" || strings.HasPrefix(name, "case::") {
				rows = append(rows, attribute)
				labels = append(labels, strings.TrimPrefix(name, "case"))
			}
		}
		if !ordinary && !rstest {
			return
		}
		name := strings.TrimPrefix(p.moduleAt(n)+"::"+p.text(p.field(n, "name")), p.root+"::")
		if strings.HasPrefix(p.file.Path, "tests/") {
			name = strings.TrimPrefix(name, strings.TrimPrefix(p.module, p.root+"::")+"::")
		}
		test := sourceTest(p.engine.ctx, p.file, p.syntax, n, name, "rust", p.owner(n))
		p.testAssertions(n, &test)
		if rstest && len(rows) > 0 {
			for i, row := range rows {
				label := fmt.Sprintf("case_%d", i+1)
				if labels[i] != "" {
					label += "_" + strings.TrimPrefix(labels[i], "::")
				}
				p.file.Tests = append(p.file.Tests, parameterTest(test, name+"::"+label, []*ts.Node{row}, p.file.Data))
			}
			return
		}
		if rstest {
			parameters := p.field(n, "parameters")
			if parameters != nil && parameters.NamedChildCount() > 0 {
				test.Unknown = append(test.Unknown, "rstest 参数或 fixture 需由 Runner 枚举，保留外层函数")
			}
		}
		p.file.Tests = append(p.file.Tests, test)
	})
}

func (p *rustFile) testAssertions(function *ts.Node, test *TestCase) {
	if p.field(function, "body") == nil {
		return
	}
	_ = walkSyntax(p.engine.ctx, p.field(function, "body"), func(n *ts.Node) {
		for parent := p.parent(n); parent != nil && parent != function; parent = p.parent(parent) {
			if p.kind(parent) == "function_item" {
				return
			}
		}
		if p.kind(n) == "macro_invocation" {
			macro := p.field(n, "macro")
			name := p.text(macro)
			if slices.Contains([]string{"assert", "assert_eq", "assert_ne", "panic"}, name) {
				if _, shadowed := p.lookup(name, n); !shadowed {
					addAssertion(test, "result", n, p.file.Data)
				}
			}
		}
		if p.kind(n) == "call_expression" {
			if p.mockExpectation(n, 0) {
				addAssertion(test, "mock", n, p.file.Data)
			} else if fn := p.field(n, "function"); p.kind(fn) == "field_expression" && strings.HasPrefix(p.text(p.field(fn, "field")), "expect_") {
				test.Unknown = append(test.Unknown, "expect_ 调用的 mockall 接收者尚未核对")
			}
		}
	})
}

func (p *rustFile) mockExpectation(call *ts.Node, depth int) bool {
	if depth > 20 || p.kind(call) != "call_expression" {
		return false
	}
	fn := p.field(call, "function")
	if p.kind(fn) != "field_expression" {
		return false
	}
	method, receiver := p.text(p.field(fn, "field")), p.field(fn, "value")
	if method == "times" || method == "with" {
		return p.mockExpectation(receiver, depth+1)
	}
	return strings.HasPrefix(method, "expect_") && p.mockallReceiver(receiver, depth+1)
}

func (p *rustFile) mockallReceiver(n *ts.Node, depth int) bool {
	if n == nil || depth > 20 {
		return false
	}
	if p.kind(n) == "identifier" {
		if binding, ok := p.lookup(p.text(n), n); ok && binding != nil {
			if binding.annotation != nil {
				return binding.file.mockallType(binding.file.text(binding.annotation), n)
			}
			return binding.file.mockallReceiver(binding.value, depth+1)
		}
	}
	if p.kind(n) == "call_expression" {
		fn := p.field(n, "function")
		if p.kind(fn) == "scoped_identifier" && slices.Contains([]string{"new", "default"}, p.text(p.field(fn, "name"))) {
			return p.mockallType(p.text(p.field(fn, "path")), n)
		}
	}
	return false
}

func (p *rustFile) mockallType(name string, at *ts.Node) bool {
	want := p.moduleAt(at) + "::" + name
	if strings.Contains(name, "::") {
		want = p.resolve(name, at, 0)
	}
	if len(p.engine.symbols[want]) > 0 {
		return false
	}
	found := false
	for _, file := range p.engine.files {
		_ = walkSyntax(p.engine.ctx, file.syntax.tree.RootNode(), func(n *ts.Node) {
			if file.kind(n) == "macro_invocation" && file.resolve(file.text(file.field(n, "macro")), n, 0) == "mockall::mock" {
				for _, child := range file.named(n) {
					if file.kind(child) != "token_tree" {
						continue
					}
					for _, token := range file.named(child) {
						if file.kind(token) == "identifier" {
							found = found || file.moduleAt(n)+"::Mock"+file.text(token) == want
							break
						}
					}
				}
			}
			if file.kind(n) != "trait_item" {
				return
			}
			if file.moduleAt(n)+"::Mock"+file.text(file.field(n, "name")) != want {
				return
			}
			for _, attribute := range file.attributes(n) {
				name, _ := file.attributeParts(attribute)
				if file.resolve(name, n, 0) == "mockall::automock" {
					found = true
				}
			}
		})
	}
	return found
}
