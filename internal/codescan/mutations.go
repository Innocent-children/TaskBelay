package codescan

import (
	"context"
	"math/big"
	"sort"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type Mutation struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Start  uint32 `json:"-"`
	End    uint32 `json:"-"`
	Before string `json:"before"`
	After  string `json:"after"`
}

/**
 * 只在调用方提供的行为变化行生成文档列出的五类变异。
 * 候选是源码替换位置，编译及实际断言结果仍需 Runner 另行核对。
 */
func Mutations(ctx context.Context, file *File, lines map[int]bool) ([]Mutation, error) {
	if file == nil || file.Kind.Test || file.Kind.Generated || !file.CodeComplete || len(lines) == 0 {
		return nil, nil
	}
	entry := grammars.DetectLanguageByName(file.Kind.Language)
	tree, err := parse(ctx, entry, file.Data)
	if tree != nil {
		defer tree.Release()
	}
	if err != nil {
		return nil, err
	}
	language := entry.Language()
	result := map[string]Mutation{}
	add := func(node *ts.Node, replacement, kind string) {
		if node == nil || node.EndByte() <= node.StartByte() {
			return
		}
		scope := span(node)
		allowed := true
		for line := scope.StartLine; line <= scope.EndLine; line++ {
			allowed = allowed && lines[line]
		}
		if !allowed {
			return
		}
		old := node.Text(file.Data)
		if old == replacement {
			return
		}
		id := digest(file.Path + "\x00" + string(file.Data[:scope.Start]) + "\x00" + old + "\x00" + replacement)
		result[id] = Mutation{ID: id, File: file.Path, Line: scope.StartLine, Kind: kind, Start: scope.Start, End: scope.End, Before: old, After: replacement}
	}
	err = walkSyntax(ctx, tree.RootNode(), func(node *ts.Node) {
		kind := node.Type(language)
		if strings.Contains(kind, "comment") {
			return
		}
		if kind == "binary_expression" || kind == "binary_operator" || kind == "comparison_operator" {
			for i := 0; i < node.ChildCount(); i++ {
				operator := node.Child(i)
				if operator.IsNamed() {
					continue
				}
				if replacement := map[string]string{"==": "!=", "!=": "==", "===": "!==", "!==": "===", "<": "<=", "<=": "<", ">": ">=", ">=": ">", "+": "-", "-": "+"}[operator.Text(file.Data)]; replacement != "" {
					category := "comparison"
					if operator.Text(file.Data) == "+" || operator.Text(file.Data) == "-" {
						category = "arithmetic"
					}
					add(operator, replacement, category)
				}
			}
		}
		if kind == "unary_expression" || kind == "unary_operator" || kind == "not_operator" {
			for i := 0; i < node.ChildCount(); i++ {
				child := node.Child(i)
				if child.Text(file.Data) == "!" || child.Text(file.Data) == "not" {
					add(child, "", "remove_not")
				}
			}
		}
		if kind == "if_statement" || kind == "if_expression" || kind == "while_statement" || kind == "for_clause" {
			condition := node.ChildByFieldName("condition", language)
			if condition != nil && !strings.Contains(condition.Type(language), "let") {
				not := "!"
				if file.Kind.Language == "python" {
					not = "not "
				}
				add(condition, "("+not+"("+condition.Text(file.Data)+"))", "negate_condition")
			}
		}
		if kind == "int_literal" || kind == "integer" || kind == "integer_literal" || kind == "decimal_integer_literal" || kind == "number" {
			text := strings.ReplaceAll(node.Text(file.Data), "_", "")
			number, ok := new(big.Int).SetString(text, 0)
			if !ok {
				return
			}
			for _, delta := range []int64{-1, 1} {
				add(node, new(big.Int).Add(number, big.NewInt(delta)).String(), "integer")
			}
		}
	})
	var mutations []Mutation
	for _, mutation := range result {
		mutations = append(mutations, mutation)
	}
	sort.Slice(mutations, func(i, j int) bool {
		a, b := mutations[i], mutations[j]
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		if a.End != b.End {
			return a.End < b.End
		}
		return a.After < b.After
	})
	return mutations, err
}
