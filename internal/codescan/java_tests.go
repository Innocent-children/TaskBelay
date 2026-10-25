package codescan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/java"
	ts "github.com/odvcencio/gotreesitter"
)

func (p *javaFile) analyzeTests() {
	defer finishTests(p.engine.ctx, p.file)
	if !p.file.Kind.Test {
		return
	}
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		if p.kind(n) != "method_declaration" {
			return
		}
		ordinary, parameterized := false, false
		for _, annotation := range p.annotations(n) {
			typ := p.typeName(p.field(annotation, "name"), 0)
			ordinary = ordinary || typ == "org.junit.Test" || typ == "org.junit.jupiter.api.Test"
			parameterized = parameterized || typ == "org.junit.jupiter.params.ParameterizedTest"
		}
		if !ordinary && !parameterized {
			return
		}
		name := p.types[p.enclosingType(n)] + "#" + p.text(p.field(n, "name"))
		test := sourceTest(p.engine.ctx, p.file, p.syntax, n, name, "junit", p.owner(n))
		if body := p.field(n, "body"); body != nil {
			_ = walkSyntax(p.engine.ctx, body, func(call *ts.Node) {
				if p.kind(call) == "method_invocation" {
					addAssertion(&test, java.TestAssertion(p.methodTarget(call, 0)), call, p.file.Data)
				}
			})
		}
		if parameterized {
			rows, known := p.testRows(n)
			if known && len(rows) > 0 {
				for i, row := range rows {
					p.file.Tests = append(p.file.Tests, parameterTest(test, fmt.Sprintf("%s[%d]", name, i+1), []*ts.Node{row}, p.file.Data))
				}
				return
			}
			test.Unknown = append(test.Unknown, "JUnit 参数源不能静态枚举，保留整个方法，单例筛选交由 Runner 核对")
		}
		p.file.Tests = append(p.file.Tests, test)
	})
}

func (p *javaFile) testRows(n *ts.Node) ([]*ts.Node, bool) {
	var result []*ts.Node
	for _, annotation := range p.annotations(n) {
		name := p.typeName(p.field(annotation, "name"), 0)
		if !strings.HasPrefix(name, "org.junit.jupiter.params.provider.") {
			continue
		}
		switch strings.TrimPrefix(name, "org.junit.jupiter.params.provider.") {
		case "NullSource", "EmptySource":
			result = append(result, annotation)
		case "ValueSource", "CsvSource":
			args := p.annotationArguments(annotation)
			var value *ts.Node
			for key, candidate := range args {
				if slices.Contains([]string{"value", "strings", "ints", "longs", "shorts", "bytes", "floats", "doubles", "chars", "booleans", "classes"}, key) {
					if value != nil {
						return nil, false
					}
					value = candidate
				}
				if key == "textBlock" || key == "useHeadersInDisplayName" {
					return nil, false
				}
			}
			if value == nil {
				return nil, false
			}
			rows := []*ts.Node{value}
			if p.kind(value) == "element_value_array_initializer" {
				rows = p.named(value)
			}
			for _, row := range rows {
				if _, ok := p.literal(row, 0); !ok && p.kind(row) != "class_literal" {
					return nil, false
				}
				result = append(result, row)
			}
		default:
			return nil, false
		}
	}
	return result, len(result) > 0
}
