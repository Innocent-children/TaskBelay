package codescan

import (
	"context"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func rustPublicAlias(n *ts.Node, language *ts.Language, source []byte, parents syntaxParents) bool {
	for n = parents.parent(n); n != nil; n = parents.parent(n) {
		if n.Type(language) != "use_declaration" {
			continue
		}
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child.Type(language) == "visibility_modifier" {
				return child.Text(source) == "pub"
			}
		}
		return false
	}
	return false
}

func rustLeadingTokens(ctx context.Context, node *ts.Node, language *ts.Language, source []byte, parents syntaxParents) ([]Token, error) {
	parent := parents.parent(node)
	if parent == nil {
		return nil, nil
	}
	var nodes []*ts.Node
	for i := 0; i < parent.ChildCount(); i++ {
		if parent.Child(i) != node {
			continue
		}
		line := node.StartPoint().Row
		for j := i - 1; j >= 0; j-- {
			previous := parent.Child(j)
			if !previous.IsNamed() {
				continue
			}
			kind := previous.Type(language)
			if kind != "attribute_item" && (!strings.Contains(kind, "comment") || previous.EndPoint().Row+1 < line) {
				break
			}
			nodes = append(nodes, previous)
			line = previous.StartPoint().Row
		}
		break
	}
	slices.Reverse(nodes)
	var result []Token
	for _, n := range nodes {
		tokens, err := leafTokens(ctx, n, language, source, nil)
		if err != nil {
			return result, err
		}
		result = append(result, tokens...)
	}
	return result, nil
}

func (p *rustFile) ancestor(n *ts.Node, kinds ...string) *ts.Node {
	for n = p.parent(n); n != nil; n = p.parent(n) {
		if slices.Contains(kinds, p.kind(n)) {
			return n
		}
	}
	return nil
}

func (p *rustFile) public(n *ts.Node) bool {
	for _, child := range p.named(n) {
		if p.kind(child) == "visibility_modifier" {
			return p.text(child) == "pub"
		}
	}
	return false
}

func (a *rustAnalysis) indexMethods() {
	var methods []*rustBinding
	for _, list := range a.symbols {
		for _, b := range list {
			if b.file.ancestor(b.node, "impl_item", "trait_item") != nil {
				methods = append(methods, b)
			}
		}
	}
	for _, b := range methods {
		p := b.file
		owner := p.ancestor(b.node, "impl_item", "trait_item")
		typ := ""
		if p.kind(owner) == "trait_item" {
			typ = p.moduleAt(owner) + "::" + p.text(p.field(owner, "name"))
		} else {
			typ = p.typeName(p.field(owner, "type"), 0)
		}
		if typ == "" {
			continue
		}
		old := b.qualified
		b.qualified = typ + "::" + b.name
		a.symbols[old] = slices.DeleteFunc(a.symbols[old], func(item *rustBinding) bool { return item == b })
		a.symbols[b.qualified] = append(a.symbols[b.qualified], b)
	}
}
