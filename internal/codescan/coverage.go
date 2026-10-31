package codescan

import (
	"context"
	"fmt"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

/**
 * 查询有结果不等于所有绑定都已提取。已知定义节点里的漏项必须使声明
 * 操作保留未完成状态，否则一次解构赋值就可能造成虚假的删除结论。
 */
func auditDefinitions(ctx context.Context, file *File, tree *ts.Tree, language *ts.Language, parents syntaxParents) error {
	covered := map[[2]uint32]bool{}
	for _, d := range file.Declarations {
		for _, name := range d.NameRanges() {
			covered[[2]uint32{name.Start, name.End}] = true
		}
	}
	return walkSyntax(ctx, tree.RootNode(), func(n *ts.Node) {
		for parent := parents.parent(n); parent != nil; parent = parents.parent(parent) {
			if callable(parent.Type(language)) {
				return
			}
		}
		kind := n.Type(language)
		var names []*ts.Node
		switch kind {
		case "assignment", "assignment_expression":
			if file.Kind.Language != "python" && file.Kind.Language != "ruby" && file.Kind.Language != "php" {
				return
			}
			names = bindingNames(n.ChildByFieldName("left", language), language)
		case "variable_declarator":
			names = bindingNames(n.ChildByFieldName("name", language), language)
		case "declaration", "type_definition":
			if file.Kind.Language == "c" || file.Kind.Language == "cpp" {
				for i := 0; i < n.ChildCount(); i++ {
					if n.FieldNameForChild(i, language) == "declarator" {
						if name := declaratorName(n.Child(i), language); name != nil {
							names = append(names, name)
						}
					}
				}
			}
		case "var_spec", "const_spec":
			for i := 0; i < n.NamedChildCount(); i++ {
				child := n.NamedChild(i)
				if child.Type(language) == "identifier" {
					names = append(names, child)
				}
			}
		case "function_definition":
			if name := n.ChildByFieldName("name", language); name != nil {
				names = []*ts.Node{name}
			} else {
				if name := declaratorName(n.ChildByFieldName("declarator", language), language); name != nil {
					names = []*ts.Node{name}
				}
			}
		case "export_specifier":
			if name := n.ChildByFieldName("alias", language); name != nil {
				names = []*ts.Node{name}
			}
		case "function_declaration", "function_signature", "method_declaration", "method_definition", "abstract_method_signature", "class_definition", "class_declaration", "abstract_class_declaration", "interface_declaration", "record_declaration", "enum_declaration", "annotation_type_declaration", "annotation_type_element_declaration", "constructor_declaration", "compact_constructor_declaration", "struct_item", "enum_item", "trait_item", "function_item", "const_item", "static_item", "type_item", "type_spec", "type_alias", "type_alias_declaration", "class_specifier", "struct_specifier", "method", "singleton_method":
			if name := n.ChildByFieldName("name", language); name != nil {
				names = []*ts.Node{name}
			}
		}
		for _, name := range names {
			if file.Kind.Language == "go" && name.Text(file.Data) == "_" {
				continue
			}
			if !covered[[2]uint32{name.StartByte(), name.EndByte()}] {
				file.Missing = append(file.Missing, Missing{file.Path, "declarations", fmt.Sprintf("第 %d 行：tags 未覆盖具名绑定 %s", name.StartPoint().Row+1, name.Text(file.Data)), "unsupported"})
			}
		}
	})
}

func declaratorName(node *ts.Node, language *ts.Language) *ts.Node {
	for node != nil {
		switch node.Type(language) {
		case "identifier", "field_identifier", "type_identifier":
			return node
		case "qualified_identifier":
			node = node.ChildByFieldName("name", language)
		case "parenthesized_declarator", "reference_declarator":
			if node.NamedChildCount() != 1 {
				return nil
			}
			node = node.NamedChild(0)
		default:
			node = node.ChildByFieldName("declarator", language)
		}
	}
	return nil
}

func bindingNames(node *ts.Node, language *ts.Language) []*ts.Node {
	if node == nil {
		return nil
	}
	switch node.Type(language) {
	case "identifier", "undefined", "constant", "variable_name", "shorthand_property_identifier_pattern":
		return []*ts.Node{node}
	case "pair_pattern":
		return bindingNames(node.ChildByFieldName("value", language), language)
	case "assignment_pattern", "object_assignment_pattern":
		return bindingNames(node.ChildByFieldName("left", language), language)
	case "pattern_list", "tuple_pattern", "list_pattern", "object_pattern", "array_pattern", "rest_pattern", "list_splat_pattern", "tuple", "list", "left_assignment_list", "rest_assignment", "list_literal":
		var names []*ts.Node
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			if !strings.Contains(child.Type(language), "comment") {
				names = append(names, bindingNames(child, language)...)
			}
		}
		return names
	}
	return nil
}
