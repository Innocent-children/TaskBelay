package codescan

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

type genericAnalysis struct {
	ctx    context.Context
	files  []*genericFile
	items  []*genericDeclaration
	byName map[string][]*genericDeclaration
	writes []genericWrite
}
type genericFile struct {
	file        *File
	syntax      *syntaxFile
	a           *genericAnalysis
	items       map[*ts.Node]*genericDeclaration
	locals      map[*genericDeclaration][]genericLocal
	definitions map[Span]bool
}
type genericDeclaration struct {
	d                  *Declaration
	p                  *genericFile
	n                  *ts.Node
	parameters         []*ts.Node
	defaults           []*ts.Node
	indirectParameters []bool
	fieldNodes         []*ts.Node
	bases              []string
	typ                bool
}
type genericLocal struct {
	name        string
	node, scope *ts.Node
}
type genericSlot struct {
	item  *genericDeclaration
	index int
	field bool
}
type genericWrite struct {
	slot genericSlot
	p    *genericFile
	n    *ts.Node
}

func analyzeGenericUsage(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	a := &genericAnalysis{ctx: ctx, byName: map[string][]*genericDeclaration{}}
	for name, syntax := range parsed {
		file := snapshot.Files[name]
		if slices.Contains([]string{"go", "python", "javascript", "typescript", "tsx", "java", "rust"}, file.Kind.Language) {
			continue
		}
		p := &genericFile{file: file, syntax: syntax, a: a, items: map[*ts.Node]*genericDeclaration{}, locals: map[*genericDeclaration][]genericLocal{}, definitions: map[Span]bool{}}
		a.files = append(a.files, p)
	}
	sort.Slice(a.files, func(i, j int) bool { return a.files[i].file.Path < a.files[j].file.Path })
	for _, p := range a.files {
		for i := range p.file.Declarations {
			d := &p.file.Declarations[i]
			d.Usage = &Usage{}
			p.definitions[d.NameSpan] = true
			n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				p.missing(p.syntax.tree.RootNode(), "声明位置未能核对："+d.Location())
				continue
			}
			item := &genericDeclaration{d: d, p: p, n: n, typ: genericType(p.kind(n))}
			p.items[n] = item
			a.items = append(a.items, item)
			key := fileKey(p.file.Kind.Language, d.Name)
			a.byName[key] = append(a.byName[key], item)
		}
	}
	for _, item := range a.items {
		a.declareGeneric(item)
	}
	for _, p := range a.files {
		_ = walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { p.indexLocal(n); a.genericField(p, n) })
	}
	a.genericInterfaces()
	for _, p := range a.files {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.genericUsage(p, n); a.genericCall(p, n) }); err != nil {
			p.missing(p.syntax.tree.RootNode(), err.Error())
		}
	}
	a.genericValues()
	for _, item := range a.items {
		a.genericShape(item)
	}
	for _, p := range a.files {
		p.file.Operations["entries"] = Operation{Reason: "文档未定义本语言的专用入口规则"}
		p.file.Operations["risks"] = Operation{Reason: "文档未定义本语言的专用风险规则"}
		p.file.Operations["tests"] = Operation{Reason: "文档未定义本语言的 Runner"}
		p.file.Operations["usage"] = Operation{Complete: true}
		for _, missing := range p.file.Missing {
			if missing.Operation == "usage" || missing.Operation == "syntax" {
				p.file.Operations["usage"] = Operation{Reason: "通用用法事实已记录，存在尚未核对的范围"}
				break
			}
		}
	}
}

func fileKey(language, name string) string { return language + ":" + strings.TrimPrefix(name, "$") }
func (p *genericFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.syntax.language)
}
func (p *genericFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.file.Data)
}
func (p *genericFile) field(n *ts.Node, name string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(name, p.syntax.language)
}
func (p *genericFile) parent(n *ts.Node) *ts.Node {
	if n == nil {
		return nil
	}
	return p.syntax.parents.parent(n)
}
func (p *genericFile) named(n *ts.Node) []*ts.Node {
	var result []*ts.Node
	if n != nil {
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if child.IsNamed() && !strings.Contains(p.kind(child), "comment") {
				result = append(result, child)
			}
		}
	}
	return result
}
func (p *genericFile) enclosing(n *ts.Node, typ bool) *genericDeclaration {
	for ; n != nil; n = p.parent(n) {
		if item := p.items[n]; item != nil && item.typ == typ {
			return item
		}
	}
	return nil
}
func (p *genericFile) use(n *ts.Node, value string) Use {
	owner := p.file.Path
	test := p.file.Kind.Test
	for node := n; node != nil; node = p.parent(node) {
		if item := p.items[node]; item != nil {
			owner = item.d.ID
			test = item.d.Test
			break
		}
	}
	return Use{File: p.file.Path, Declaration: owner, Value: value, Span: span(n), Test: test, Candidate: true}
}
func (p *genericFile) missing(n *ts.Node, reason string) {
	entry := Missing{p.file.Path, "usage", fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason)}
	if !slices.Contains(p.file.Missing, entry) {
		p.file.Missing = append(p.file.Missing, entry)
	}
}
func (a *genericAnalysis) unknown(item *genericDeclaration, slot *Slot, p *genericFile, n *ts.Node, reason string) {
	use := p.use(n, "")
	use.Reason = reason
	if slot != nil {
		use.Reason = item.d.Location() + "." + slot.Name + "：" + reason
		addUse(&slot.Unknown, use)
	} else if item != nil {
		addUse(&item.d.Usage.Unknown, use)
	}
	p.missing(n, use.Reason)
	if item != nil && item.p != p {
		item.p.missing(item.n, p.file.Path+"："+use.Reason)
	}
}
func (s genericSlot) slot() *Slot {
	if s.item == nil {
		return nil
	}
	if s.field {
		return &s.item.d.Usage.Fields[s.index]
	}
	return &s.item.d.Usage.Parameters[s.index]
}
func genericType(kind string) bool {
	return slices.Contains([]string{"class", "class_definition", "class_declaration", "class_specifier", "struct_specifier", "struct_declaration", "record_declaration", "interface_declaration", "interface_definition", "trait_declaration", "module"}, kind)
}
func (p *genericFile) name(n *ts.Node) *ts.Node {
	if slices.Contains([]string{"identifier", "field_identifier", "variable_name", "instance_variable", "name", "constant"}, p.kind(n)) {
		return n
	}
	for _, key := range []string{"name", "declarator", "pattern", "left"} {
		if child := p.field(n, key); child != nil {
			if key == "declarator" {
				return declaratorName(child, p.syntax.language)
			}
			return child
		}
	}
	return nil
}
func (p *genericFile) defaultValue(n *ts.Node) *ts.Node {
	for _, key := range []string{"default_value", "value", "default"} {
		if value := p.field(n, key); value != nil {
			return value
		}
	}
	if p.kind(n) == "parameter" || p.kind(n) == "variable_declarator" {
		seen := false
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if p.text(child) == "=" {
				seen = true
				continue
			}
			if seen && child.IsNamed() {
				return child
			}
		}
	}
	return nil
}
func (a *genericAnalysis) declareGeneric(item *genericDeclaration) {
	p, n, u := item.p, item.n, item.d.Usage
	a.unknown(item, nil, p, n, "文档未定义本语言的公开范围及隐式入口约定，不能确认声明仅供内部调用")
	u.Abstract = strings.Contains(p.kind(n), "interface") || p.kind(n) == "trait_declaration"
	for _, child := range p.named(n) {
		if (p.kind(child) == "modifier" || p.kind(child) == "abstract_modifier") && p.text(child) == "abstract" {
			u.Abstract = true
		}
		if slices.Contains([]string{"base_list", "base_class_clause", "class_interface_clause", "base_clause", "superclass"}, p.kind(child)) {
			for _, base := range p.named(child) {
				if slices.Contains([]string{"identifier", "type_identifier", "name", "constant", "qualified_name", "qualified_identifier"}, p.kind(base)) {
					item.bases = append(item.bases, p.text(base))
				}
			}
		}
	}
	if item.typ {
		return
	}
	if !callable(p.kind(n)) {
		return
	}
	params := p.field(n, "parameters")
	for declarator := p.field(n, "declarator"); params == nil && declarator != nil; declarator = p.field(declarator, "declarator") {
		params = p.field(declarator, "parameters")
	}
	for _, parameter := range p.named(params) {
		name := p.name(parameter)
		if name == nil {
			if len(p.named(params)) == 1 && p.text(parameter) == "void" {
				continue
			}
			a.unknown(item, nil, p, parameter, "参数绑定没有可核对的名字")
			continue
		}
		slot := Slot{Name: p.text(name), Type: p.text(p.field(parameter, "type")), Span: span(name)}
		indirect := false
		_ = walkSyntax(a.ctx, parameter, func(child *ts.Node) {
			if slices.Contains([]string{"pointer_declarator", "reference_declarator", "array_declarator", "array_type", "reference_modifier"}, p.kind(child)) {
				indirect = true
			}
			if slices.Contains([]string{"function_declarator", "function_pointer_type", "lambda_type"}, p.kind(child)) {
				slot.Function = true
			}
		})
		for i := 0; i < parameter.ChildCount(); i++ {
			if slices.Contains([]string{"ref", "out", "&"}, p.text(parameter.Child(i))) {
				indirect = true
			}
		}
		if owner := p.enclosing(p.parent(n), true); owner != nil {
			slot.Interface = owner.d.Usage.Abstract
		}
		u.Parameters = append(u.Parameters, slot)
		item.parameters = append(item.parameters, name)
		p.definitions[span(name)] = true
		item.defaults = append(item.defaults, p.defaultValue(parameter))
		item.indirectParameters = append(item.indirectParameters, indirect && !slot.Function)
		if indirect && !slot.Function {
			a.unknown(item, &u.Parameters[len(u.Parameters)-1], p, parameter, "指针、数组或引用参数的间接写入范围未确定")
		}
		if value := p.defaultValue(parameter); value != nil {
			a.writes = append(a.writes, genericWrite{genericSlot{item, len(u.Parameters) - 1, false}, p, value})
		}
	}
}

func (p *genericFile) indexLocal(n *ts.Node) {
	if !slices.Contains([]string{"declaration", "variable_declarator", "init_declarator", "parameter", "parameter_declaration", "simple_parameter", "optional_parameter", "keyword_parameter", "block_parameter"}, p.kind(n)) {
		return
	}
	item := p.enclosing(n, false)
	if item == nil || !callable(p.kind(item.n)) {
		return
	}
	var names []*ts.Node
	if p.kind(n) == "declaration" {
		for i := 0; i < n.ChildCount(); i++ {
			if n.FieldNameForChild(i, p.syntax.language) == "declarator" {
				if name := declaratorName(n.Child(i), p.syntax.language); name != nil {
					names = append(names, name)
				}
			}
		}
	} else if name := p.name(n); name != nil {
		names = append(names, name)
	}
	scope := p.parent(n)
	for scope != nil && !slices.Contains([]string{"block", "compound_statement", "body_statement", "statement_block", "for_statement", "lambda_expression", "anonymous_function", "block_parameters", "function_definition", "method_declaration"}, p.kind(scope)) {
		scope = p.parent(scope)
	}
	if scope == nil {
		scope = item.n
	}
	for _, name := range names {
		if p.definitions[span(name)] {
			continue
		}
		p.definitions[span(name)] = true
		p.locals[item] = append(p.locals[item], genericLocal{p.text(name), name, scope})
	}
}

func (a *genericAnalysis) genericField(p *genericFile, n *ts.Node) {
	var nodes []*ts.Node
	switch p.kind(n) {
	case "field_declaration":
		if declarator := p.field(n, "declarator"); declarator != nil {
			if p.kind(declarator) == "function_declarator" && p.kind(p.field(declarator, "declarator")) == "field_identifier" {
				if owner := p.enclosing(p.parent(n), true); owner != nil {
					owner.d.Usage.Methods = append(owner.d.Usage.Methods, p.text(p.field(declarator, "declarator")))
					if p.text(p.field(n, "default_value")) == "0" {
						owner.d.Usage.Abstract = true
					}
				}
				return
			}
			for i := 0; i < n.ChildCount(); i++ {
				if n.FieldNameForChild(i, p.syntax.language) == "declarator" {
					nodes = append(nodes, n.Child(i))
				}
			}
		} else {
			_ = walkSyntax(a.ctx, n, func(child *ts.Node) {
				if p.kind(child) == "variable_declarator" {
					nodes = append(nodes, child)
				}
			})
		}
	case "property_element":
		nodes = append(nodes, n)
	case "instance_variable":
		nodes = append(nodes, n)
	default:
		return
	}
	owner := p.enclosing(p.parent(n), true)
	if owner == nil {
		return
	}
	for _, node := range nodes {
		name := p.name(node)
		if name == nil {
			name = declaratorName(node, p.syntax.language)
		}
		if name == nil {
			continue
		}
		if a.field(owner, p.text(name)).item != nil {
			continue
		}
		typ := p.field(n, "type")
		if typ == nil {
			typ = p.field(p.parent(node), "type")
		}
		if typ == nil {
			typ = p.field(p.parent(n), "type")
		}
		slot := Slot{Name: p.text(name), Type: p.text(typ), Span: span(name)}
		_ = walkSyntax(a.ctx, node, func(child *ts.Node) {
			if p.kind(child) == "function_declarator" {
				slot.Function = true
			}
		})
		owner.d.Usage.Fields = append(owner.d.Usage.Fields, slot)
		owner.fieldNodes = append(owner.fieldNodes, name)
		if p.kind(node) != "instance_variable" {
			p.definitions[span(name)] = true
		}
		ref := genericSlot{owner, len(owner.d.Usage.Fields) - 1, true}
		a.unknown(owner, ref.slot(), p, node, "通用语法未确定字段的隐式初始化及外部写入范围")
		value := p.defaultValue(node)
		if value == nil {
			value = p.defaultValue(n)
		}
		if value != nil {
			ref.slot().Written = true
			a.writes = append(a.writes, genericWrite{ref, p, value})
		}
	}
}

func (a *genericAnalysis) field(item *genericDeclaration, name string) genericSlot {
	if item != nil {
		for i, field := range item.d.Usage.Fields {
			if strings.TrimLeft(field.Name, "$@") == strings.TrimLeft(name, "$@") {
				return genericSlot{item, i, true}
			}
		}
	}
	return genericSlot{}
}

func (a *genericAnalysis) genericInterfaces() {
	for _, item := range a.items {
		if !callable(item.p.kind(item.n)) || item.d.Owner == "" {
			continue
		}
		if owner := item.p.enclosing(item.p.parent(item.n), true); owner != nil {
			owner.d.Usage.Methods = append(owner.d.Usage.Methods, item.d.Name)
		}
	}
	for _, item := range a.items {
		if a.ctx.Err() != nil {
			return
		}
		sort.Strings(item.d.Usage.Methods)
		item.d.Usage.Methods = slices.Compact(item.d.Usage.Methods)
		if !item.typ || item.d.Usage.Abstract {
			continue
		}
		for _, base := range a.items {
			if !base.d.Usage.Abstract || base.p.file.Kind.Language != item.p.file.Kind.Language {
				continue
			}
			explicit := a.inherits(item, base, map[*genericDeclaration]bool{})
			matches := len(item.bases) == 0 && len(base.d.Usage.Methods) > 0
			for _, method := range base.d.Usage.Methods {
				matches = matches && slices.Contains(item.d.Usage.Methods, method)
			}
			if explicit || matches {
				use := item.p.use(item.n, item.d.ID)
				if !explicit {
					use.Reason = "同名方法集合候选"
				}
				addUse(&base.d.Usage.Implementations, use)
				for _, method := range a.items {
					if method.d.Owner == item.d.Name && method.p == item.p && slices.Contains(base.d.Usage.Methods, method.d.Name) {
						for i := range method.d.Usage.Parameters {
							method.d.Usage.Parameters[i].Interface = true
						}
					}
				}
			}
		}
	}
}

func (a *genericAnalysis) inherits(item, target *genericDeclaration, seen map[*genericDeclaration]bool) bool {
	if seen[item] || a.ctx.Err() != nil {
		return false
	}
	seen[item] = true
	for _, base := range item.bases {
		if base == target.d.Name || strings.NewReplacer("::", ".", "\\", ".").Replace(base) == target.d.Owner+"."+target.d.Name {
			return true
		}
		for _, parent := range a.byName[fileKey(item.p.file.Kind.Language, base)] {
			if parent.typ && a.inherits(parent, target, seen) {
				return true
			}
		}
	}
	return false
}
