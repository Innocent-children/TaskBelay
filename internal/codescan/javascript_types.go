package codescan

import (
	"fmt"
	"slices"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (a *jsAnalysis) methods(owner, name string, seen map[string]bool) []*jsDeclaration {
	if seen[owner] || a.ctx.Err() != nil {
		return nil
	}
	seen[owner] = true
	defer delete(seen, owner)
	item := a.byID[owner]
	if item == nil {
		return nil
	}
	var methods []*jsDeclaration
	for _, method := range a.ownMethods(item) {
		if method.d.Name == name {
			methods = append(methods, method)
		}
	}
	if len(methods) > 0 {
		return methods
	}
	for _, parent := range item.parents {
		methods = append(methods, a.methods(parent.d.ID, name, seen)...)
	}
	return methods
}

func (a *jsAnalysis) callable(p *jsFile, n *ts.Node) []*jsDeclaration {
	if n == nil {
		return nil
	}
	if item := a.byID[p.reference(n, 0).qualified]; item != nil {
		return []*jsDeclaration{item}
	}
	if p.kind(n) == "member_expression" || p.kind(n) == "subscript_expression" {
		ref := p.reference(p.field(n, "object"), 0)
		owner := ref.typ
		if owner == "" {
			owner = ref.qualified
		}
		name := p.propertyName(p.field(n, "property"))
		if p.kind(n) == "subscript_expression" {
			value, ok := p.literal(p.field(n, "index"), 0)
			if ok && value.Kind == "String" {
				name = value.Text
			}
		}
		return a.methods(owner, name, map[string]bool{})
	}
	return nil
}

func (a *jsAnalysis) implementations() {
	for _, item := range a.declarations {
		if !item.class || item.d.Usage.Abstract {
			continue
		}
		seen := map[string]bool{}
		var visit func(*jsDeclaration)
		visit = func(parent *jsDeclaration) {
			if seen[parent.d.ID] {
				return
			}
			seen[parent.d.ID] = true
			if parent.d.Usage.Abstract {
				if len(item.d.Usage.Unknown) > 0 {
					a.unknown(parent, nil, item.p, item.n, "候选实现仍有不能核对的继承关系："+item.d.Location())
				} else {
					addUse(&parent.d.Usage.Implementations, item.p.use(item.n, item.d.ID))
				}
			}
			for _, base := range parent.parents {
				visit(base)
			}
		}
		for _, parent := range item.parents {
			visit(parent)
		}
	}
}

func (a *jsAnalysis) objectImplementation(p *jsFile, n *ts.Node, typeName string) {
	if p.kind(n) != "object" {
		return
	}
	target := a.typeTarget(typeName, map[string]bool{})
	if target == nil || !target.d.Usage.Abstract {
		return
	}
	addUse(&target.d.Usage.Implementations, p.use(n, fmt.Sprintf("%s:object@%d", p.file.Path, n.StartByte())))
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if method := p.usage[p.key(child)]; method != nil && method.function != nil && slices.Contains(target.d.Usage.Methods, method.d.Name) {
			for j := range method.d.Usage.Parameters {
				method.d.Usage.Parameters[j].Interface = true
			}
		}
	}
}

func (p *jsFile) typeArgumentsAt(n *ts.Node, depth int) (*jsFile, *ts.Node) {
	if n == nil || depth > 40 {
		return p, nil
	}
	if arguments := p.field(n, "type_arguments"); arguments != nil {
		return p, arguments
	}
	if p.kind(n) == "identifier" {
		if b, ok := p.lookup(p.text(n), n); ok && b != nil {
			if b.value != nil {
				return b.file.typeArgumentsAt(b.value, depth+1)
			}
			if exported := p.engine.exports[b.qualified]; len(exported) == 1 && exported[0].value != nil {
				return exported[0].file.typeArgumentsAt(exported[0].value, depth+1)
			}
		}
	}
	return p, nil
}

func (a *jsAnalysis) genericUse(item *jsDeclaration, p *jsFile, use *ts.Node, argFile *jsFile, args *ts.Node) {
	if !item.d.Usage.Generic {
		return
	}
	count := 0
	if args != nil {
		for i := 0; i < args.ChildCount(); i++ {
			child := args.Child(i)
			if child.IsNamed() && argFile.kind(child) != "comment" {
				count++
			}
		}
	}
	if args == nil || count != len(item.d.Usage.TypeParameters) {
		item.d.Usage.Inferred = true
		a.unknown(item, nil, p, use, "泛型用法省略或只给出部分类型实参，不能完整计数")
		return
	}
	tokens, err := leafTokens(a.ctx, args, argFile.syntax.language, argFile.file.Data, nil)
	if err != nil {
		a.unknown(item, nil, p, use, err.Error())
		return
	}
	var text strings.Builder
	for _, token := range tokens {
		if token.Kind != "comment" {
			text.WriteString(token.Text)
		}
	}
	addUse(&item.d.Usage.TypeArguments, p.use(use, text.String()))
}

func (a *jsAnalysis) inspectTypes(p *jsFile, n *ts.Node) {
	switch p.kind(n) {
	case "generic_type":
		if item := a.byID[p.typeName(p.field(n, "name"), 0)]; item != nil {
			a.genericUse(item, p, n, p, p.field(n, "type_arguments"))
		}
	case "extends_clause":
		if item := a.byID[p.typeName(p.field(n, "value"), 0)]; item != nil {
			a.genericUse(item, p, n, p, p.field(n, "type_arguments"))
		}
	case "instantiation_expression":
		if n.NamedChildCount() > 0 {
			for _, item := range a.callable(p, n.NamedChild(0)) {
				a.genericUse(item, p, n, p, p.field(n, "type_arguments"))
			}
		}
	case "call_expression", "new_expression":
		callee := p.field(n, "function")
		if callee == nil {
			callee = p.field(n, "constructor")
		}
		if p.kind(callee) == "super" {
			return
		}
		argFile, args := p.typeArgumentsAt(n, 0)
		if args == nil {
			argFile, args = p.typeArgumentsAt(callee, 0)
		}
		for _, item := range a.callable(p, callee) {
			a.genericUse(item, p, n, argFile, args)
		}
	case "object":
		parent := p.parent(n)
		if parent != nil && slices.Contains([]string{"variable_declarator", "required_parameter", "optional_parameter"}, p.kind(parent)) {
			a.objectImplementation(p, n, p.typeName(p.field(parent, "type"), 0))
		}
	case "binary_expression":
		if p.text(p.field(n, "operator")) != "instanceof" {
			return
		}
		right := p.field(n, "right")
		target := a.typeTarget(p.reference(right, 0).qualified, map[string]bool{})
		if target != nil && target.d.Usage.Error {
			addUse(&target.d.Usage.ErrorChecks, p.use(n, target.d.ID))
		} else if p.reference(right, 0).qualified == "" {
			p.missing("usage", n, "instanceof 右侧的类型不能静态核对")
		}
	case "type_identifier", "identifier":
		item := a.byID[p.reference(n, 0).qualified]
		if item == nil || !item.d.Usage.Generic {
			return
		}
		if item.d.File == p.file.Path && item.d.NameSpan == span(n) {
			return
		}
		parent := p.parent(n)
		if parent == nil {
			return
		}
		if slices.Contains([]string{"generic_type", "extends_clause", "instantiation_expression", "type_parameter", "import_specifier", "export_specifier"}, p.kind(parent)) {
			return
		}
		if p.kind(parent) == "call_expression" || p.kind(parent) == "new_expression" {
			callee := p.field(parent, "function")
			if callee == nil {
				callee = p.field(parent, "constructor")
			}
			if callee != nil && span(callee) == span(n) {
				return
			}
		}
		if p.kind(parent) == "member_expression" || p.kind(parent) == "subscript_expression" {
			return
		}
		if p.stableAlias(n) {
			return
		}
		a.genericUse(item, p, n, p, nil)
	}
}
