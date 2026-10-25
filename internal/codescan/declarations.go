package codescan

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func span(node *ts.Node) Span {
	return Span{node.StartByte(), node.EndByte(), int(node.StartPoint().Row) + 1, int(node.EndPoint().Row) + 1}
}

func callable(nodeType string) bool {
	switch nodeType {
	case "function_declaration", "function_signature", "method_declaration", "function_definition", "method_definition", "constructor_declaration", "compact_constructor_declaration", "function_item", "function_signature_item", "function_expression", "arrow_function", "lambda", "lambda_expression", "func_literal", "method", "singleton_method", "local_function_statement":
		return true
	}
	return false
}

func declarations(ctx context.Context, file *File, tree *ts.Tree, query *ts.Query, language *ts.Language, parents syntaxParents) ([]Declaration, error) {
	cursor := query.Exec(tree.RootNode(), language, file.Data)
	var result []Declaration
	nodes := map[string][]*ts.Node{}
	documents := map[uint32][]Token{}
	indices := map[string]int{}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		var definition *ts.Node
		var declarator *ts.Node
		var names []*ts.Node
		kind := ""
		for _, capture := range match.Captures {
			if strings.HasPrefix(capture.Name, "definition.") {
				definition, kind = capture.Node, strings.TrimPrefix(capture.Name, "definition.")
			} else if capture.Name == "name" {
				names = append(names, capture.Node)
			} else if capture.Name == "binding" {
				names = append(names, bindingNames(capture.Node, language)...)
			} else if capture.Name == "declarator" {
				declarator = capture.Node
				if name := declaratorName(capture.Node, language); name != nil {
					names = append(names, name)
				}
			}
		}
		if definition == nil {
			continue
		}
		if file.Kind.Language == "rust" && definition.Type(language) == "use_as_clause" && !rustPublicAlias(definition, language, file.Data, parents) {
			continue
		}
		if kind == "variable" && declarator != nil && declarator.Type(language) == "function_declarator" {
			if inner := declarator.ChildByFieldName("declarator", language); inner != nil && inner.Type(language) != "parenthesized_declarator" {
				kind = "function"
			}
		}
		local := false
		for parent := parents.parent(definition); parent != nil; parent = parents.parent(parent) {
			if callable(parent.Type(language)) {
				local = true
				break
			}
		}
		if local {
			continue
		}
		definitionKind := kind
		for _, nameNode := range names {
			kind = definitionKind
			if file.Kind.Language == "ruby" && nameNode.Type(language) == "constant" {
				kind = "constant"
			}
			multipleNames := definition.Type(language) == "var_spec" || definition.Type(language) == "const_spec"
			if actual := definition.ChildByFieldName("name", language); !multipleNames && definition.Type(language) != "export_specifier" && actual != nil && (nameNode.StartByte() < actual.StartByte() || nameNode.EndByte() > actual.EndByte()) {
				continue
			}
			name := strings.TrimSpace(nameNode.Text(file.Data))
			if name == "" || (file.Kind.Language == "go" && name == "_") {
				continue
			}
			owner := declarationOwner(definition, language, file.Data, file.Kind.Language, parents)
			qualified := name
			if owner != "" {
				qualified = owner + "." + name
			}
			id := file.Path + ":" + qualified
			container := declarationContainer(definition, language, parents)
			duplicate := false
			for _, previous := range nodes[id] {
				if previous.StartByte() == container.StartByte() && previous.EndByte() == container.EndByte() {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			merge := false
			if len(nodes[id]) > 0 {
				previous := result[indices[id]]
				merge = previous.Kind == kind && previous.NodeType == definition.Type(language) && repeatedAssignment(file.Kind.Language, previous.NodeType)
				if !merge {
					// 内部标识只区分同名定义，不能作为对外实现位置。
					id += fmt.Sprintf("@%d", definition.StartByte())
					if len(nodes[id]) > 0 {
						continue
					}
				}
			}
			nodes[id] = append(nodes[id], container)
			comments := leadingComments(container, language, file.Data, parents)
			if file.Kind.Language == "rust" {
				var err error
				comments, err = rustLeadingTokens(ctx, container, language, file.Data, parents)
				if err != nil {
					return result, err
				}
			}
			documents[container.StartByte()] = comments
			region := span(container)
			var documentation []string
			for _, comment := range comments {
				if comment.Kind == "comment" {
					documentation = append(documentation, comment.Text)
				}
			}
			if len(comments) > 0 {
				region.Start, region.StartLine = comments[0].Span.Start, comments[0].Span.StartLine
			}
			if merge {
				d := &result[indices[id]]
				d.Segments = append(d.Segments, region)
				d.NameSpans = append(d.NameSpans, span(nameNode))
				d.Text += "\n" + string(file.Data[region.Start:region.End])
				if len(documentation) > 0 {
					d.Documentation += "\n" + strings.Join(documentation, "\n")
				}
			} else {
				indices[id] = len(result)
				result = append(result, Declaration{ID: id, File: file.Path, Name: name, Owner: owner, Kind: kind, NodeType: definition.Type(language), Span: region, NameSpan: span(nameNode), Segments: []Span{region}, NameSpans: []Span{span(nameNode)}, Test: file.Kind.Test || (file.Kind.Language == "rust" && rustTestRange(definition, language, file.Data)), Text: string(file.Data[region.Start:region.End]), Documentation: strings.Join(documentation, "\n")})
			}
		}
	}
	if cursor.DidExceedMatchLimit() {
		return result, fmt.Errorf("tags 查询达到匹配预算")
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Span.Start == result[j].Span.Start {
			return result[i].ID < result[j].ID
		}
		return result[i].Span.Start < result[j].Span.Start
	})
	shadowed, err := shapeBindings(ctx, tree.RootNode(), language, file.Data)
	if err != nil {
		return result, err
	}
	nameStarts := map[uint32]bool{}
	for _, declaration := range result {
		for _, name := range declaration.NameRanges() {
			nameStarts[name.Start] = true
		}
	}
	for i := range result {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		declaration := &result[i]
		var tokens []Token
		for partIndex, definition := range nodes[declaration.ID] {
			region := declaration.Segments[partIndex]
			var owned []Span
			for _, other := range result {
				for _, part := range other.Ranges() {
					if part.Start > region.Start && part.End <= region.End {
						owned = append(owned, part)
					}
				}
			}
			partTokens, err := leafTokens(ctx, definition, language, file.Data, owned)
			if err != nil {
				return result, err
			}
			tokens = append(tokens, documents[definition.StartByte()]...)
			tokens = append(tokens, partTokens...)
			if len(nodes[declaration.ID]) == 1 {
				declaration.Shape, err = functionShape(ctx, definition, language, file.Data, file.Kind.Language, shadowed)
				if err != nil {
					return result, err
				}
			}
		}
		declaration.Tokens = tokens
		refs := map[string]bool{}
		var full, code, shape, literals strings.Builder
		for _, token := range tokens {
			encoded := token.Kind + "\x00" + token.Text + "\x00"
			full.WriteString(encoded)
			if token.Kind == "comment" {
				continue
			}
			code.WriteString(encoded)
			if token.Kind == "identifier" {
				shape.WriteString("identifier\x00")
				if !nameStarts[token.Span.Start] {
					refs[token.Text] = true
				}
			} else {
				shape.WriteString(encoded)
			}
			if token.Kind == "literal" {
				literals.WriteString("literal\x00")
			} else {
				literals.WriteString(encoded)
			}
		}
		declaration.FullHash, declaration.CodeHash, declaration.ShapeHash, declaration.LiteralFreeHash = digest(full.String()), digest(code.String()), digest(shape.String()), digest(literals.String())
		for name := range refs {
			declaration.References = append(declaration.References, name)
		}
		sort.Strings(declaration.References)
	}
	return result, nil
}

func repeatedAssignment(language, nodeType string) bool {
	return (language == "python" || language == "ruby" || language == "php") && (nodeType == "assignment" || nodeType == "assignment_expression")
}

func digest(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func declarationContainer(node *ts.Node, language *ts.Language, parents syntaxParents) *ts.Node {
	if node.Type(language) == "use_as_clause" {
		for parent := parents.parent(node); parent != nil; parent = parents.parent(parent) {
			if parent.Type(language) == "use_declaration" {
				return parent
			}
		}
	}
	for parents.parent(node) != nil {
		parent := parents.parent(node)
		switch parent.Type(language) {
		case "decorated_definition", "export_statement":
			node = parent
		case "var_declaration", "const_declaration", "type_declaration", "lexical_declaration", "variable_declaration":
			if parent.NamedChildCount() != 1 {
				return node
			}
			node = parent
		default:
			return node
		}
	}
	return node
}

func leadingComments(node *ts.Node, language *ts.Language, source []byte, parents syntaxParents) []Token {
	parent := parents.parent(node)
	if parent == nil {
		return nil
	}
	var comments []Token
	for i := 0; i < parent.ChildCount(); i++ {
		if parent.Child(i).StartByte() != node.StartByte() {
			continue
		}
		line := node.StartPoint().Row
		for j := i - 1; j >= 0; j-- {
			previous := parent.Child(j)
			if !previous.IsNamed() {
				continue
			}
			if !strings.Contains(previous.Type(language), "comment") || previous.EndPoint().Row+1 < line {
				break
			}
			comments = append(comments, Token{Kind: "comment", Text: previous.Text(source), Span: span(previous)})
			line = previous.StartPoint().Row
		}
		break
	}
	for i, j := 0, len(comments)-1; i < j; i, j = i+1, j-1 {
		comments[i], comments[j] = comments[j], comments[i]
	}
	return comments
}

func declarationOwner(node *ts.Node, language *ts.Language, source []byte, name string, parents syntaxParents) string {
	if name == "go" && node.Type(language) == "method_declaration" {
		receiver := node.ChildByFieldName("receiver", language)
		if receiver != nil && receiver.NamedChildCount() > 0 {
			return typeName(receiver.NamedChild(0).ChildByFieldName("type", language), language, source)
		}
	}
	var owners []string
	if name == "cpp" {
		for declarator := node.ChildByFieldName("declarator", language); declarator != nil; declarator = declarator.ChildByFieldName("declarator", language) {
			if declarator.Type(language) == "qualified_identifier" {
				if scope := declarator.ChildByFieldName("scope", language); scope != nil {
					owners = append(owners, cppScopeName(scope, language, source))
				}
				break
			}
		}
	}
	for parent := parents.parent(node); parent != nil; parent = parents.parent(parent) {
		kind := parent.Type(language)
		if name == "rust" && kind == "impl_item" {
			owners = append(owners, typeName(parent.ChildByFieldName("type", language), language, source))
			continue
		}
		switch kind {
		case "class", "class_definition", "class_declaration", "abstract_class_declaration", "class_specifier", "interface_declaration", "trait_item", "struct_specifier", "struct_item", "mod_item", "module", "type_spec", "record_declaration", "enum_declaration", "annotation_type_declaration", "namespace_definition", "namespace_declaration", "internal_module":
			if n := parent.ChildByFieldName("name", language); n != nil {
				owners = append(owners, n.Text(source))
			} else if kind == "class" && parents.parent(parent) != nil && parents.parent(parent).Type(language) == "variable_declarator" {
				if n := parents.parent(parent).ChildByFieldName("name", language); n != nil && n.Type(language) == "identifier" {
					owners = append(owners, n.Text(source))
				}
			}
		}
	}
	for i, j := 0, len(owners)-1; i < j; i, j = i+1, j-1 {
		owners[i], owners[j] = owners[j], owners[i]
	}
	return strings.Join(owners, ".")
}

func typeName(node *ts.Node, language *ts.Language, source []byte) string {
	if node == nil {
		return ""
	}
	if node.Type(language) == "type_identifier" || node.Type(language) == "identifier" || node.Type(language) == "namespace_identifier" {
		return node.Text(source)
	}
	if child := node.ChildByFieldName("type", language); child != nil {
		return typeName(child, language, source)
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if value := typeName(node.NamedChild(i), language, source); value != "" {
			return value
		}
	}
	return ""
}

func cppScopeName(node *ts.Node, language *ts.Language, source []byte) string {
	if node != nil && node.Type(language) == "qualified_identifier" {
		left := cppScopeName(node.ChildByFieldName("scope", language), language, source)
		right := cppScopeName(node.ChildByFieldName("name", language), language, source)
		if left != "" {
			return left + "." + right
		}
		return right
	}
	return typeName(node, language, source)
}

func rustTestRange(node *ts.Node, language *ts.Language, source []byte) bool {
	for current := node; current != nil; current = current.Parent() {
		parent := current.Parent()
		if parent == nil {
			break
		}
		for i := 0; i < parent.ChildCount(); i++ {
			if parent.Child(i).StartByte() != current.StartByte() {
				continue
			}
			for j := i - 1; j >= 0; j-- {
				previous := parent.Child(j)
				if !previous.IsNamed() {
					continue
				}
				if previous.Type(language) != "attribute_item" {
					break
				}
				text := strings.Join(strings.Fields(previous.Text(source)), "")
				if text == "#[cfg(test)]" {
					return true
				}
			}
			break
		}
	}
	return false
}

func leafTokens(ctx context.Context, node *ts.Node, language *ts.Language, source []byte, excluded []Span) ([]Token, error) {
	var tokens []Token
	stack := []*ts.Node{node}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return tokens, err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		skip := false
		for _, region := range excluded {
			if current.StartByte() >= region.Start && current.EndByte() <= region.End {
				skip = true
				break
			}
		}
		if skip || current.EndByte() <= current.StartByte() {
			continue
		}
		kind := current.Type(language)
		category := "syntax"
		switch {
		case strings.Contains(kind, "comment"):
			category = "comment"
		case strings.Contains(kind, "string") || strings.Contains(kind, "char_literal") || strings.Contains(kind, "integer") || strings.Contains(kind, "float") || kind == "int_literal" || kind == "rune_literal" || kind == "imaginary_literal" || kind == "number" || kind == "true" || kind == "false" || kind == "nil" || kind == "null" || kind == "none":
			category = "literal"
		case strings.Contains(kind, "identifier") || kind == "constant" || kind == "name" || kind == "variable_name":
			category = "identifier"
		}
		if category == "literal" && hasInterpolation(current, language) {
			category = "syntax"
		}
		if current.ChildCount() == 0 || category == "comment" || category == "literal" || kind == "variable_name" {
			tokens = append(tokens, Token{Kind: category, Text: current.Text(source), Span: span(current)})
			continue
		}
		for i := current.ChildCount() - 1; i >= 0; i-- {
			stack = append(stack, current.Child(i))
		}
	}
	return tokens, nil
}

func hasInterpolation(node *ts.Node, language *ts.Language) bool {
	stack := []*ts.Node{node}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kind := current.Type(language)
		if kind == "interpolation" || kind == "template_substitution" || kind == "embedded_expression" {
			return true
		}
		for i := 0; i < current.NamedChildCount(); i++ {
			stack = append(stack, current.NamedChild(i))
		}
	}
	return false
}
