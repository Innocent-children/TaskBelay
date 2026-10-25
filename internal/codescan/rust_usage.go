package codescan

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

type rustDeclaration struct {
	d      *Declaration
	p      *rustFile
	n      *ts.Node
	target string
}

func (a *rustAnalysis) analyzeUsage(snapshot *Snapshot) {
	a.byTarget = map[string][]*rustDeclaration{}
	a.slots = map[*rustBinding]rustSlot{}
	for _, p := range a.files {
		p.usage = map[*ts.Node]*rustDeclaration{}
		for i := range p.file.Declarations {
			d := &p.file.Declarations[i]
			d.Usage = &Usage{}
			n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				p.missing("usage", p.syntax.tree.RootNode(), "声明语法位置不能核对："+d.Location())
				continue
			}
			item := &rustDeclaration{d: d, p: p, n: n}
			a.declarations = append(a.declarations, item)
			p.usage[n] = item
			for _, b := range p.bindings[p.scope(n)][d.Name] {
				if b.node == n {
					item.target = b.qualified
					break
				}
			}
			if item.target != "" {
				a.byTarget[item.target] = append(a.byTarget[item.target], item)
			}
		}
	}
	for _, item := range a.declarations {
		a.declareRust(item, snapshot)
		a.rustSlots(item)
	}
	for _, p := range a.files {
		_ = walkSyntax(a.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
			if p.kind(n) == "impl_item" || p.kind(n) == "trait_item" {
				a.rustRelations(p, n)
			}
		})
	}
	for _, p := range a.files {
		if err := walkSyntax(a.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
			if p.kind(n) != "impl_item" && p.kind(n) != "trait_item" {
				a.rustRelations(p, n)
			}
			a.rustReadWrite(p, n)
		}); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	for _, p := range a.files {
		if err := walkSyntax(a.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.rustCalls(p, n) }); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	a.rustValues()
}

func (p *rustFile) use(n *ts.Node, value string) Use {
	return Use{File: p.file.Path, Declaration: p.owner(n), Value: value, Span: span(n), Test: p.file.Kind.Test || rustTestRange(n, p.syntax.language, p.file.Data), Candidate: true}
}

func (a *rustAnalysis) unknown(item *rustDeclaration, slot *Slot, p *rustFile, n *ts.Node, reason string) {
	use := p.use(n, "")
	use.Reason = reason
	if slot != nil {
		use.Reason = item.d.Location() + "." + slot.Name + "：" + reason
		addUse(&slot.Unknown, use)
	} else if item != nil {
		addUse(&item.d.Usage.Unknown, use)
	}
	p.missing("usage", n, use.Reason)
	if item != nil && item.p != p {
		item.p.missing("usage", item.n, fmt.Sprintf("%s:%d：%s", p.file.Path, n.StartPoint().Row+1, use.Reason))
	}
}

func (a *rustAnalysis) item(target string) *rustDeclaration {
	if list := a.byTarget[target]; len(list) == 1 {
		return list[0]
	}
	return nil
}

func (p *rustFile) enclosingUsage(n *ts.Node) *rustDeclaration {
	for ; n != nil; n = p.parent(n) {
		if item := p.usage[n]; item != nil {
			return item
		}
	}
	return nil
}

func (p *rustFile) derived(n *ts.Node, name string) bool {
	for _, attr := range p.attributes(n) {
		key, groups := p.attributeParts(attr)
		if key != "derive" {
			continue
		}
		for _, group := range groups {
			text := p.groupText(group)
			if text == name || strings.HasSuffix(text, "::"+name) {
				return true
			}
		}
	}
	return false
}

func (a *rustAnalysis) declareRust(item *rustDeclaration, snapshot *Snapshot) {
	p, d, n := item.p, item.d, item.n
	u := d.Usage
	library := snapshot.Files[path.Join(strings.TrimPrefix(p.root, "local:"), "src/lib.rs")] != nil
	u.Public = library && p.public(n)
	u.Abstract = p.kind(n) == "trait_item"
	u.Error = (p.kind(n) == "struct_item" || p.kind(n) == "enum_item") && p.derived(n, "Error")
	if owner := p.ancestor(n, "trait_item"); owner != nil && callable(p.kind(n)) {
		u.Public = library && p.public(owner)
	}
	if callable(p.kind(n)) {
		if d.Name == "main" && d.Owner == "" {
			u.Implicit = append(u.Implicit, "main")
		}
		if owner := p.ancestor(n, "impl_item"); owner != nil && p.field(owner, "trait") != nil {
			u.Implicit = append(u.Implicit, "trait 实现方法")
		}
	}
	for _, attr := range p.attributes(n) {
		name, _ := p.attributeParts(attr)
		if name == "deprecated" {
			u.Deprecated = true
		}
		if callable(p.kind(n)) && (name == "no_mangle" || name == "export_name") {
			u.Implicit = append(u.Implicit, name)
		}
	}
	for _, parameter := range p.named(p.field(n, "type_parameters")) {
		if name := p.field(parameter, "name"); name != nil && p.kind(parameter) != "lifetime_parameter" {
			u.TypeParameters = append(u.TypeParameters, p.text(name))
		}
	}
	u.Generic = len(u.TypeParameters) > 0
	if p.kind(n) == "type_item" {
		u.Alias = p.text(p.field(n, "type"))
	}
	if p.kind(n) == "use_as_clause" {
		u.Alias = p.text(p.field(n, "path"))
		u.Public = library && rustPublicAlias(n, p.syntax.language, p.file.Data, p.syntax.parents)
	}
	if !p.file.CodeComplete {
		a.unknown(item, nil, p, n, "源码未完整解析，声明用法不能完整核对")
	}
	if item.target != "" && len(a.byTarget[item.target]) > 1 {
		a.unknown(item, nil, p, n, "同一模块或类型存在同名声明，分派不能唯一核对")
	}
}

func (a *rustAnalysis) rustRelations(p *rustFile, n *ts.Node) {
	switch p.kind(n) {
	case "impl_item":
		typ := p.typeName(p.field(n, "type"), 0)
		trait := p.typeName(p.field(n, "trait"), 0)
		concrete := a.item(typ)
		for i := 0; i < n.ChildCount(); i++ {
			if p.text(n.Child(i)) == "!" {
				if item := a.item(trait); item != nil {
					a.unknown(item, nil, p, n, "负向 trait 实现不能计为具体实现")
				}
				return
			}
		}
		if trait == "std::error::Error" && concrete != nil {
			concrete.d.Usage.Error = true
		}
		if item := a.item(trait); item != nil {
			value, complete := p.typeArgument(p.field(n, "type"), 0)
			if value == "" {
				value = p.text(p.field(n, "type"))
			}
			addUse(&item.d.Usage.Implementations, p.use(n, value))
			if !complete {
				a.unknown(item, nil, p, n, "trait 实现类型依赖推断或泛型参数，不能确定具体类型集合")
			}
		}
		for _, method := range p.named(p.field(n, "body")) {
			if !callable(p.kind(method)) {
				continue
			}
			if concrete != nil {
				concrete.d.Usage.Methods = append(concrete.d.Usage.Methods, p.text(p.field(method, "name")))
			}
		}
	case "trait_item":
		if item := p.usage[n]; item != nil {
			for _, method := range p.named(p.field(n, "body")) {
				if callable(p.kind(method)) {
					item.d.Usage.Methods = append(item.d.Usage.Methods, p.text(p.field(method, "name")))
				}
			}
		}
	case "generic_type", "generic_type_with_turbofish":
		if p.kind(p.parent(n)) == "impl_item" {
			return
		}
		if item := a.item(p.typeName(n, 0)); item != nil {
			a.rustGeneric(item, p, n, p.field(n, "type_arguments"))
		}
	case "generic_function":
		if parent := p.parent(n); p.kind(parent) == "call_expression" && p.field(parent, "function") == n {
			return
		}
		if item := a.item(p.reference(p.field(n, "function"), 0).qualified); item != nil {
			a.rustGeneric(item, p, n, p.field(n, "type_arguments"))
			for i := range item.d.Usage.Parameters {
				a.unknown(item, &item.d.Usage.Parameters[i], p, n, "函数作为值传递后，后续调用实参不能完整核对")
			}
		}
	case "type_identifier", "scoped_type_identifier":
		parent := p.parent(n)
		if p.text(n) == "Self" {
			return
		}
		if slices.Contains([]string{"generic_type", "generic_type_with_turbofish", "type_parameter", "impl_item", "scoped_type_identifier"}, p.kind(parent)) || p.field(parent, "name") == n {
			return
		}
		if item := a.item(p.typeName(n, 0)); item != nil {
			a.rustGeneric(item, p, n, nil)
		}
	case "match_arm", "let_condition":
		a.rustErrorPattern(p, p.field(n, "pattern"), n)
	case "macro_invocation":
		if p.text(p.field(n, "macro")) == "matches" {
			a.rustErrorTokens(p, n)
		}
	case "attribute_item":
		name, _ := p.attributeParts(n)
		if name == "cfg_attr" || name == "path" || (name == "cfg" && strings.Join(strings.Fields(p.text(n)), "") != "#[cfg(test)]") {
			a.unknown(p.enclosingUsage(n), nil, p, n, "条件属性或特殊模块路径依赖构建配置")
		}
	}
}

func (p *rustFile) typeArgument(n *ts.Node, depth int) (string, bool) {
	if n == nil || depth > 40 {
		return "", false
	}
	if p.kind(n) == "integer_literal" || p.kind(n) == "boolean_literal" {
		return p.text(n), true
	}
	switch p.kind(n) {
	case "unit_type":
		return "()", true
	case "tuple_type":
		parts, complete := []string{}, true
		for _, child := range p.named(n) {
			value, ok := p.typeArgument(child, depth+1)
			parts = append(parts, value)
			complete = complete && ok
		}
		return "(" + strings.Join(parts, ",") + ")", complete
	case "array_type":
		element, ok := p.typeArgument(p.field(n, "element"), depth+1)
		if length := p.field(n, "length"); length != nil {
			size, known := p.typeArgument(length, depth+1)
			return "[" + element + ";" + size + "]", ok && known
		}
		return "[" + element + "]", ok
	case "function_type":
		parts, complete := []string{}, true
		for _, child := range p.named(p.field(n, "parameters")) {
			value, ok := p.typeArgument(child, depth+1)
			parts = append(parts, value)
			complete = complete && ok
		}
		result := "()"
		if typ := p.field(n, "return_type"); typ != nil {
			value, ok := p.typeArgument(typ, depth+1)
			result = value
			complete = complete && ok
		}
		return "fn(" + strings.Join(parts, ",") + ")->" + result, complete
	}
	if p.kind(n) == "generic_type" || p.kind(n) == "generic_type_with_turbofish" {
		base := p.typeName(n, 0)
		var parts []string
		ok := base != ""
		for _, child := range p.named(p.field(n, "type_arguments")) {
			value, complete := p.typeArgument(child, depth+1)
			parts = append(parts, value)
			ok = ok && complete
		}
		return base + "<" + strings.Join(parts, ",") + ">", ok
	}
	if p.kind(n) == "reference_type" || p.kind(n) == "pointer_type" {
		value, ok := p.typeArgument(p.field(n, "type"), depth+1)
		prefix := strings.TrimSuffix(p.text(n), p.text(p.field(n, "type")))
		return strings.Join(strings.Fields(prefix), "") + value, ok
	}
	name := p.typeName(n, 0)
	if name == "" || p.text(n) == "_" {
		return p.text(n), false
	}
	return name, true
}

func (a *rustAnalysis) rustGeneric(item *rustDeclaration, p *rustFile, n, args *ts.Node) {
	if !item.d.Usage.Generic {
		return
	}
	complete := args != nil
	var values []string
	for _, arg := range p.named(args) {
		if p.kind(arg) == "lifetime" {
			continue
		}
		value, ok := p.typeArgument(arg, 0)
		values = append(values, value)
		complete = complete && ok
	}
	complete = complete && len(values) == len(item.d.Usage.TypeParameters)
	if !complete {
		item.d.Usage.Inferred = true
		a.unknown(item, nil, p, n, "泛型用法依赖推断，不能完整统计类型实参")
		return
	}
	addUse(&item.d.Usage.TypeArguments, p.use(n, "<"+strings.Join(values, ",")+">"))
}

func (a *rustAnalysis) rustErrorPattern(p *rustFile, pattern, evidence *ts.Node) {
	if pattern == nil {
		return
	}
	_ = walkSyntax(a.ctx, pattern, func(n *ts.Node) {
		if !slices.Contains([]string{"identifier", "type_identifier", "scoped_identifier", "scoped_type_identifier"}, p.kind(n)) {
			return
		}
		a.rustErrorName(p, p.resolve(p.text(n), n, 0), evidence)
	})
}

func (a *rustAnalysis) rustErrorName(p *rustFile, name string, n *ts.Node) {
	for name != "" {
		if item := a.item(name); item != nil && item.d.Usage.Error {
			addUse(&item.d.Usage.ErrorChecks, p.use(n, name))
			return
		}
		index := strings.LastIndex(name, "::")
		if index < 0 {
			return
		}
		name = name[:index]
	}
}

func (a *rustAnalysis) rustErrorTokens(p *rustFile, n *ts.Node) {
	var tree *ts.Node
	for _, child := range p.named(n) {
		if p.kind(child) == "token_tree" {
			tree = child
		}
	}
	if tree == nil {
		return
	}
	pattern := false
	for i := 0; i < tree.ChildCount(); i++ {
		child := tree.Child(i)
		if p.text(child) == "," {
			pattern = true
			continue
		}
		if !pattern || p.kind(child) != "identifier" {
			continue
		}
		name := p.text(child)
		for i+2 < tree.ChildCount() && p.text(tree.Child(i+1)) == "::" {
			name += "::" + p.text(tree.Child(i+2))
			i += 2
		}
		a.rustErrorName(p, p.resolve(name, child, 0), n)
	}
}

func sortRustMethods(items []*rustDeclaration) {
	for _, item := range items {
		sort.Strings(item.d.Usage.Methods)
		item.d.Usage.Methods = slices.Compact(item.d.Usage.Methods)
	}
}
