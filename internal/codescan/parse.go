package codescan

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"time"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type syntaxParents map[*ts.Node]*ts.Node

func (parents syntaxParents) parent(node *ts.Node) *ts.Node {
	if parents != nil {
		return parents[node]
	}
	return node.Parent()
}

/**
 * 固定版本 JavaScript 的部分父链仍指向解析时的包装节点。
 * 从最终树建立一次父级索引，保证声明、绑定和用法使用同一棵树。
 */
func indexSyntaxParents(ctx context.Context, tree *ts.Tree, language string) (syntaxParents, error) {
	if !slices.Contains([]string{"javascript", "typescript", "tsx"}, language) {
		return nil, nil
	}
	parents := syntaxParents{}
	err := walkSyntax(ctx, tree.RootNode(), func(node *ts.Node) {
		for i := 0; i < node.ChildCount(); i++ {
			parents[node.Child(i)] = node
		}
	})
	return parents, err
}

func parse(ctx context.Context, entry *grammars.LangEntry, source []byte) (*ts.Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if entry == nil || entry.Language == nil {
		return nil, fmt.Errorf("没有内置语法")
	}
	language := entry.Language()
	parser := ts.NewParser(language)
	if deadline, ok := ctx.Deadline(); ok {
		left := time.Until(deadline)
		if left <= 0 {
			return nil, context.DeadlineExceeded
		}
		parser.SetTimeoutMicros(uint64(max(1, left.Microseconds())))
	}
	var canceled uint32
	parser.SetCancellationFlag(&canceled)
	stop := context.AfterFunc(ctx, func() { atomic.StoreUint32(&canceled, 1) })
	defer stop()
	var tree *ts.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSourceStrict(source, entry.TokenSourceFactory(source, language))
	} else {
		tree, err = parser.ParseStrict(source)
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err == nil && (tree == nil || tree.RootNode() == nil || tree.RootNode().HasErrorOrMissing()) {
		err = fmt.Errorf("语法树包含无法解析的内容")
	}
	return tree, err
}

func walkSyntax(ctx context.Context, node *ts.Node, visit func(*ts.Node)) error {
	stack := []*ts.Node{node}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visit(current)
		for i := current.ChildCount() - 1; i >= 0; i-- {
			if child := current.Child(i); child.IsNamed() {
				stack = append(stack, child)
			}
		}
	}
	return nil
}

func syntaxNodeAt(node *ts.Node, language *ts.Language, kind string, name Span) *ts.Node {
	if node == nil || node.StartByte() > name.Start || node.EndByte() < name.End {
		return nil
	}
	for i := 0; i < node.ChildCount(); i++ {
		if found := syntaxNodeAt(node.Child(i), language, kind, name); found != nil {
			return found
		}
	}
	if node.Type(language) == kind {
		return node
	}
	return nil
}
