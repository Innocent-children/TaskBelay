package codescan

import (
	"context"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func sourceTest(ctx context.Context, file *File, syntax *syntaxFile, node *ts.Node, name, framework, declaration string) TestCase {
	test := TestCase{File: file.Path, Name: name, Framework: framework, Declaration: declaration, Span: span(node)}
	tokens, err := leafTokens(ctx, node, syntax.language, file.Data, nil)
	if err != nil {
		test.Unknown = append(test.Unknown, err.Error())
	}
	var code strings.Builder
	for _, token := range tokens {
		if strings.Contains(token.Kind, "comment") {
			continue
		}
		code.WriteString(token.Kind + "\x00" + token.Text + "\x00")
		if strings.Contains(token.Kind, "identifier") {
			test.References = append(test.References, token.Text)
		}
	}
	test.CodeHash = digest(code.String())
	slices.Sort(test.References)
	test.References = slices.Compact(test.References)
	return test
}

func finishTests(ctx context.Context, file *File) {
	complete := file.CodeComplete && ctx.Err() == nil
	counts := map[string]int{}
	for _, test := range file.Tests {
		counts[test.Location()]++
	}
	for i := range file.Tests {
		test := &file.Tests[i]
		if counts[test.Location()] > 1 {
			test.Unknown = append(test.Unknown, "同一文件中的用例名字重复，不能唯一定位")
		}
		if len(test.Unknown) > 0 {
			complete = false
			file.Missing = append(file.Missing, Missing{file.Path, "tests", test.Name + ": " + strings.Join(test.Unknown, "；")})
		}
	}
	file.Operations["tests"] = Operation{Complete: complete}
	if !complete {
		file.Operations["tests"] = Operation{Reason: "部分用例身份或断言尚不能静态核对"}
	}
}

func namedNodes(node *ts.Node, language *ts.Language) []*ts.Node {
	var result []*ts.Node
	if node != nil {
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if !strings.Contains(child.Type(language), "comment") {
				result = append(result, child)
			}
		}
	}
	return result
}

func addAssertion(test *TestCase, kind string, node *ts.Node, data []byte) {
	if kind != "" {
		test.Assertions = append(test.Assertions, Assertion{Kind: kind, Evidence: node.Text(data), Span: span(node)})
	}
}

func parameterTest(test TestCase, name string, nodes []*ts.Node, data []byte) TestCase {
	test.Name = name
	test.Parameters = slices.Clone(test.Parameters)
	for _, node := range nodes {
		test.Parameters = append(test.Parameters, span(node))
		test.CodeHash = digest(test.CodeHash + node.Text(data))
	}
	return test
}
