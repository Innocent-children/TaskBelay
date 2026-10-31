package codescan

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	ts "github.com/odvcencio/gotreesitter"
)

type jsScope struct {
	start, end uint32
	kind       string
}
type jsBinding struct {
	name, qualified, member string
	value, annotation       *ts.Node
	constant                bool
	file                    *jsFile
	scope                   jsScope
	available               uint32
	scopeNode               *ts.Node
}
type jsReference struct{ qualified, typ string }
type jsFile struct {
	file          *File
	syntax        *syntaxFile
	engine        *jsAnalysis
	bindings      map[jsScope]map[string][]*jsBinding
	bindingOrder  []*jsBinding
	declarations  map[jsScope]*Declaration
	missingKeys   map[string]bool
	mutations     map[string]bool
	usage         map[jsScope]*jsDeclaration
	exportedNames map[string]jsExport
	visibility    Visibility
}
type jsAnalysis struct {
	ctx             context.Context
	files           map[string]*jsFile
	ordered         []*jsFile
	exports         map[string][]*jsBinding
	declarations    []*jsDeclaration
	byID            map[string]*jsDeclaration
	byName          map[string][]*jsDeclaration
	public          bool
	packageUnknown  string
	methodsByOwner  map[string][]*jsDeclaration
	fieldWrites     []jsFieldWrite
	parameterWrites []jsParameterWrite
	fieldLinks      []jsFieldLink
	fieldsByName    map[string][]jsField
}

func analyzeJavaScriptAPIs(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	a := &jsAnalysis{ctx: ctx, files: map[string]*jsFile{}, exports: map[string][]*jsBinding{}}
	for name, file := range snapshot.Files {
		if !slices.Contains([]string{"javascript", "typescript", "tsx"}, file.Kind.Language) || parsed[name] == nil {
			continue
		}
		p := &jsFile{file: file, syntax: parsed[name], engine: a, bindings: map[jsScope]map[string][]*jsBinding{}, declarations: map[jsScope]*Declaration{}, missingKeys: map[string]bool{}, mutations: map[string]bool{}}
		a.files[name] = p
		a.ordered = append(a.ordered, p)
		for i := range file.Declarations {
			d := &file.Declarations[i]
			if n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan); n != nil {
				p.declarations[p.key(n)] = d
			}
		}
	}
	sort.Slice(a.ordered, func(i, j int) bool { return a.ordered[i].file.Path < a.ordered[j].file.Path })
	a.initUsage(snapshot)
	for _, p := range a.ordered {
		p.bindTree()
	}
	for _, p := range a.ordered {
		p.bindExports()
		p.visibility = p.moduleVisibility(snapshot)
	}
	for _, item := range a.declarations {
		a.declare(item)
		item.d.Visibility = item.p.visibility
		if item.p.visibility.Scope != "unknown" && (item.exported || item.d.Owner != "") {
			item.d.Visibility = Visibility{Scope: "language"}
		}
	}
	for _, item := range a.declarations {
		if item.exported && a.public && item.p.kind(item.n) == "variable_declarator" {
			if target := a.byID[item.p.reference(item.p.field(item.n, "value"), 0).qualified]; target != nil {
				target.d.Usage.Public = true
			}
		}
	}
	for _, item := range a.declarations {
		a.link(item, map[string]bool{})
	}
	for _, item := range a.declarations {
		a.parameters(item)
	}
	a.fieldsByName = map[string][]jsField{}
	for _, item := range a.declarations {
		a.fields(item)
	}
	for _, item := range a.declarations {
		for i, field := range item.d.Usage.Fields {
			a.fieldsByName[field.Name] = append(a.fieldsByName[field.Name], jsField{item, i})
		}
	}
	for _, p := range a.ordered {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.readWrite(p, n) }); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	a.implementations()
	a.defaults()
	for _, p := range a.ordered {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.inspectTypes(p, n); a.calls(p, n) }); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
		for _, b := range p.bindingOrder {
			if len(p.bindings[b.scope][b.name]) < 2 {
				continue
			}
			ref := p.bindingReference(b, 0)
			at := b.value
			if at == nil {
				at = b.scopeNode
			}
			if target := a.byID[ref.qualified]; target != nil {
				a.callUnknown(target, p, at, "别名 "+b.name+" 存在重绑定，间接调用实参不能完整核对")
			}
			if ref.typ != "" {
				a.objectEscape(p, at, ref.typ, map[string]bool{}, "对象别名 "+b.name+" 存在重绑定，字段写入不能完整核对")
			}
		}
	}
	a.constructorValues()
	for _, p := range a.ordered {
		for _, b := range p.bindingOrder {
			if len(p.bindings[b.scope][b.name]) < 2 {
				continue
			}
			ref := p.bindingReference(b, 0)
			at := b.value
			if at == nil {
				at = b.scopeNode
			}
			if ref.qualified == "global.process.env" || ref.qualified == "import.meta.env" {
				p.missing("entries", at, "环境变量对象的别名 "+b.name+" 存在重绑定，读取范围不能完整核对")
			}
			if len(jsRiskKinds(ref.qualified, true)) > 0 {
				p.missing("risks", at, "风险函数的别名 "+b.name+" 存在重绑定，调用范围不能完整核对")
			}
			for _, rule := range javascript.Rules.Entries {
				if rule.Target == jsTarget(ref.qualified) {
					p.missing("entries", at, "入口函数的别名 "+b.name+" 存在重绑定，调用范围不能完整核对")
				}
			}
		}
		_ = walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
			if p.kind(n) != "assignment_expression" && p.kind(n) != "augmented_assignment_expression" {
				return
			}
			left := p.field(n, "left")
			if p.kind(left) != "member_expression" && p.kind(left) != "subscript_expression" {
				return
			}
			if target := p.reference(left, 0).qualified; target != "" {
				p.mutations[target] = true
			} else if base := p.reference(p.field(left, "object"), 0).qualified; base != "" {
				p.mutations[base] = true
				p.missing("entries", left, "对象的计算属性被写入，调用绑定不能完整核对")
				p.missing("risks", left, "对象的计算属性被写入，调用绑定不能完整核对")
			}
		})
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), p.inspectAPI); err != nil {
			p.missing("entries", p.syntax.tree.RootNode(), err.Error())
			p.missing("risks", p.syntax.tree.RootNode(), err.Error())
		}
		if strings.Contains("/"+p.file.Path, "/pages/api/") || (strings.Contains("/"+p.file.Path, "/app/") && (path.Base(p.file.Path) == "route.js" || path.Base(p.file.Path) == "route.ts")) {
			p.entry(p.syntax.tree.RootNode(), "route", p.file.Path, "")
		}
	}
	for _, p := range a.ordered {
		for _, op := range []string{"entries", "risks", "usage"} {
			p.file.Operations[op] = Operation{Complete: true}
			for _, m := range p.file.Missing {
				if m.Operation == op || m.Operation == "syntax" || (op == "usage" && m.Operation == "declarations") {
					p.file.Operations[op] = Operation{Reason: "存在未能核对的位置"}
					break
				}
			}
		}
		p.analyzeTests()
		sort.Slice(p.file.Entries, func(i, j int) bool {
			a, b := p.file.Entries[i], p.file.Entries[j]
			if a.Span.Start != b.Span.Start {
				return a.Span.Start < b.Span.Start
			}
			return a.Location() < b.Location()
		})
	}
}

func (p *jsFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.syntax.language)
}
func (p *jsFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.file.Data)
}
func (p *jsFile) field(n *ts.Node, name string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(name, p.syntax.language)
}
func (p *jsFile) parent(n *ts.Node) *ts.Node { return p.syntax.parents.parent(n) }
func (p *jsFile) key(n *ts.Node) jsScope     { return jsScope{n.StartByte(), n.EndByte(), p.kind(n)} }
func jsFunction(kind string) bool {
	return slices.Contains([]string{"function_declaration", "function_expression", "generator_function_declaration", "generator_function", "arrow_function", "method_definition"}, kind)
}
func jsScopeKind(kind string) bool {
	return jsCallable(kind) || slices.Contains([]string{"program", "statement_block", "for_statement", "for_in_statement", "catch_clause", "class", "class_declaration", "abstract_class_declaration", "interface_declaration", "type_alias_declaration"}, kind)
}
func (p *jsFile) scope(n *ts.Node, function bool) *ts.Node {
	for n = p.parent(n); n != nil; n = p.parent(n) {
		if p.kind(n) == "program" || jsFunction(p.kind(n)) || (!function && jsScopeKind(p.kind(n))) {
			return n
		}
	}
	return p.syntax.tree.RootNode()
}

func (p *jsFile) add(scope *ts.Node, b *jsBinding) {
	b.file = p
	b.scope = p.key(scope)
	b.scopeNode = scope
	if p.bindings[b.scope] == nil {
		p.bindings[b.scope] = map[string][]*jsBinding{}
	}
	p.bindings[b.scope][b.name] = append(p.bindings[b.scope][b.name], b)
	p.bindingOrder = append(p.bindingOrder, b)
}

func (p *jsFile) bindTree() {
	err := walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		switch p.kind(n) {
		case "import_statement":
			p.bindImport(n)
		case "function_declaration", "function_signature", "generator_function_declaration", "class_declaration", "abstract_class_declaration", "interface_declaration", "type_alias_declaration":
			name := p.field(n, "name")
			if name != nil {
				q := p.file.Path + ":" + p.text(name)
				if d := p.declarations[p.key(n)]; d != nil {
					q = d.ID
				}
				p.add(p.scope(n, false), &jsBinding{name: p.text(name), qualified: q, available: n.StartByte()})
			}
		case "variable_declarator":
			scope := p.scope(n, p.kind(p.parent(n)) == "variable_declaration")
			for _, name := range bindingNames(p.field(n, "name"), p.syntax.language) {
				member := ""
				if p.kind(p.parent(name)) == "pair_pattern" {
					member = p.propertyName(p.field(p.parent(name), "key"))
				} else if p.kind(name) == "shorthand_property_identifier_pattern" {
					member = p.text(name)
				}
				value := p.field(n, "value")
				if !slices.Contains([]string{"identifier", "undefined"}, p.kind(p.field(n, "name"))) && member == "" {
					value = nil
				}
				p.add(scope, &jsBinding{name: p.text(name), value: value, member: member, annotation: p.field(n, "type"), constant: strings.HasPrefix(strings.TrimSpace(p.text(p.parent(n))), "const "), available: n.EndByte()})
			}
		case "assignment_expression", "augmented_assignment_expression":
			if left := p.field(n, "left"); p.kind(left) == "identifier" || p.kind(left) == "undefined" {
				scope := p.scope(n, false)
				if b, ok := p.lookup(p.text(left), left); ok && b != nil {
					scope = b.scopeNode
				}
				p.add(scope, &jsBinding{name: p.text(left), available: n.StartByte()})
			}
		case "update_expression":
			if arg := p.field(n, "argument"); p.kind(arg) == "identifier" || p.kind(arg) == "undefined" {
				scope := p.scope(n, false)
				if b, ok := p.lookup(p.text(arg), arg); ok && b != nil {
					scope = b.scopeNode
				}
				p.add(scope, &jsBinding{name: p.text(arg), available: n.StartByte()})
			}
		case "catch_clause":
			for _, name := range bindingNames(p.field(n, "parameter"), p.syntax.language) {
				p.add(n, &jsBinding{name: p.text(name), available: n.StartByte()})
			}
		case "for_in_statement":
			left := p.field(n, "left")
			for _, name := range bindingNames(left, p.syntax.language) {
				p.add(n, &jsBinding{name: p.text(name), available: n.StartByte()})
			}
		case "with_statement":
			p.missing("entries", n, "with 动态作用域中的名字不能完整核对")
			p.missing("risks", n, "with 动态作用域中的名字不能完整核对")
		}
		if parameters := p.field(n, "type_parameters"); parameters != nil {
			for i := 0; i < parameters.ChildCount(); i++ {
				child := parameters.Child(i)
				if p.kind(child) == "type_parameter" {
					if name := p.field(child, "name"); name != nil {
						p.add(n, &jsBinding{name: p.text(name), available: n.StartByte()})
					}
				}
			}
		}
		if jsCallable(p.kind(n)) {
			parameters := p.field(n, "parameters")
			if parameters == nil {
				parameters = p.field(n, "parameter")
			}
			if parameters == nil {
				return
			}
			if p.kind(parameters) == "identifier" || p.kind(parameters) == "undefined" {
				p.add(n, &jsBinding{name: p.text(parameters), available: n.StartByte()})
				return
			}
			for i := 0; i < parameters.ChildCount(); i++ {
				child := parameters.Child(i)
				if !child.IsNamed() {
					continue
				}
				name := child
				if p.kind(child) == "required_parameter" || p.kind(child) == "optional_parameter" {
					name = p.field(child, "pattern")
				}
				for _, binding := range bindingNames(name, p.syntax.language) {
					p.add(n, &jsBinding{name: p.text(binding), annotation: p.field(child, "type"), available: n.StartByte()})
				}
			}
		}
	})
	if err != nil {
		for _, op := range []string{"entries", "risks"} {
			p.missing(op, p.syntax.tree.RootNode(), err.Error())
		}
	}
}

func (p *jsFile) lookup(name string, at *ts.Node) (*jsBinding, bool) {
	insideFunction := false
	for node := at; node != nil; node = p.parent(node) {
		if p.kind(node) == "with_statement" {
			return nil, true
		}
		if !jsScopeKind(p.kind(node)) {
			continue
		}
		insideFunction = insideFunction || jsFunction(p.kind(node))
		if bindings := p.bindings[p.key(node)][name]; len(bindings) > 0 {
			if len(bindings) != 1 {
				return nil, true
			}
			b := bindings[0]
			if !insideFunction && b.available > at.StartByte() {
				return nil, true
			}
			return b, true
		}
	}
	return nil, false
}

func (p *jsFile) module(name string) string {
	name = strings.TrimPrefix(name, "node:")
	if !strings.HasPrefix(name, ".") {
		return name
	}
	base := path.Clean(path.Join(path.Dir(p.file.Path), name))
	var matches []string
	for _, suffix := range []string{"", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts", "/index.js", "/index.ts", "/index.tsx"} {
		if p.engine.files[base+suffix] != nil {
			matches = append(matches, base+suffix)
		}
	}
	if len(matches) == 1 {
		return "module:" + matches[0]
	}
	return ""
}

func (p *jsFile) bindImport(n *ts.Node) {
	source, ok := javaScriptString(p.text(p.field(n, "source")))
	if !ok {
		p.missing("entries", n, "导入源不是可核对的字符串")
		p.missing("risks", n, "导入源不是可核对的字符串")
		return
	}
	module := p.module(source)
	if module == "" {
		p.missing("entries", n, "相对模块不能唯一对应到固定快照中的源码")
		p.missing("risks", n, "相对模块不能唯一对应到固定快照中的源码")
	}
	if slices.Contains(javascript.Rules.ConcurrentImports, module) {
		p.risk(n, "concurrency")
	}
	_ = walkSyntax(p.engine.ctx, n, func(child *ts.Node) {
		var name, q string
		switch p.kind(child) {
		case "import_specifier":
			name = p.text(p.field(child, "alias"))
			original := p.text(p.field(child, "name"))
			if name == "" {
				name = original
			}
			if module != "" {
				q = module + "." + original
			}
		case "namespace_import":
			if child.NamedChildCount() > 0 {
				name = p.text(child.NamedChild(0))
				q = module
			}
		case "identifier":
			if p.kind(p.parent(child)) == "import_clause" {
				name = p.text(child)
				q = module
				if strings.HasPrefix(module, "module:") {
					q += ".default"
				}
			}
		}
		if name != "" {
			p.add(p.syntax.tree.RootNode(), &jsBinding{name: name, qualified: q, available: n.StartByte()})
		}
	})
}

func (p *jsFile) bindExports() {
	p.exportedNames = map[string]jsExport{}
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		if p.kind(n) != "export_statement" {
			return
		}
		root := p.bindings[p.key(p.syntax.tree.RootNode())]
		source, hasSource := javaScriptString(p.text(p.field(n, "source")))
		module := ""
		if hasSource {
			module = p.module(source)
		}
		for i := 0; i < n.ChildCount(); i++ {
			child := n.Child(i)
			if !child.IsNamed() {
				continue
			}
			if p.kind(child) == "export_clause" {
				for j := 0; j < child.NamedChildCount(); j++ {
					entry := child.NamedChild(j)
					name := p.text(p.field(entry, "name"))
					alias := p.text(p.field(entry, "alias"))
					if alias == "" {
						alias = name
					}
					if hasSource {
						if module != "" {
							p.engine.exports["module:"+p.file.Path+"."+alias] = []*jsBinding{{name: alias, qualified: module + "." + name, file: p, scopeNode: n}}
						}
					} else {
						p.engine.exports["module:"+p.file.Path+"."+alias] = root[name]
						p.markExport(name, alias == "default", n)
					}
				}
			} else {
				var names []*ts.Node
				if p.kind(child) == "identifier" && strings.HasPrefix(strings.TrimSpace(p.text(n)), "export default ") {
					p.engine.exports["module:"+p.file.Path+".default"] = root[p.text(child)]
					p.markExport(p.text(child), true, n)
				}
				if name := p.field(child, "name"); name != nil {
					names = append(names, name)
				}
				if p.kind(child) == "lexical_declaration" || p.kind(child) == "variable_declaration" {
					for j := 0; j < child.NamedChildCount(); j++ {
						names = append(names, bindingNames(p.field(child.NamedChild(j), "name"), p.syntax.language)...)
					}
				}
				for _, name := range names {
					exported := p.text(name)
					if strings.HasPrefix(strings.TrimSpace(p.text(n)), "export default ") {
						exported = "default"
					}
					p.engine.exports["module:"+p.file.Path+"."+exported] = root[p.text(name)]
					p.markExport(p.text(name), exported == "default", n)
				}
			}
		}
	})
}

func (p *jsFile) bindingReference(b *jsBinding, depth int) jsReference {
	if b == nil || depth > 40 {
		return jsReference{}
	}
	if b.qualified != "" {
		if exported := p.engine.exports[b.qualified]; len(exported) == 1 {
			return exported[0].file.bindingReference(exported[0], depth+1)
		}
		if strings.HasPrefix(b.qualified, "module:") && p.engine.files[strings.TrimPrefix(b.qualified, "module:")] == nil {
			p.missing("entries", b.scopeNode, "导出不能唯一核对："+b.qualified)
			p.missing("risks", b.scopeNode, "导出不能唯一核对："+b.qualified)
			return jsReference{}
		}
		return jsReference{qualified: b.qualified}
	}
	ref := p.reference(b.value, depth+1)
	if b.member != "" {
		base := ref.qualified
		if ref.typ != "" {
			base = ref.typ
		}
		if base != "" {
			qualified := base + "." + b.member
			if exported := p.engine.exports[qualified]; len(exported) == 1 {
				return exported[0].file.bindingReference(exported[0], depth+1)
			}
			if strings.HasPrefix(qualified, "module:") {
				p.missing("entries", b.scopeNode, "解构导出的名字不能唯一核对："+qualified)
				p.missing("risks", b.scopeNode, "解构导出的名字不能唯一核对："+qualified)
				return jsReference{}
			}
			return jsReference{qualified: qualified}
		}
		return jsReference{}
	}
	if ref.qualified == "" && ref.typ == "" && b.annotation != nil {
		typ := b.annotation
		for p.kind(typ) == "type_annotation" && typ.NamedChildCount() == 1 {
			typ = typ.NamedChild(0)
		}
		ref.typ = p.typeName(typ, depth+1)
	}
	return ref
}

func (p *jsFile) reference(n *ts.Node, depth int) jsReference {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return jsReference{}
	}
	switch p.kind(n) {
	case "this":
		if item := p.thisClass(n); item != nil {
			return jsReference{typ: item.d.ID}
		}
	case "super":
		if item := p.thisClass(n); item != nil {
			for _, parent := range item.parents {
				if parent.p.kind(parent.n) != "interface_declaration" {
					return jsReference{qualified: parent.d.ID, typ: parent.d.ID}
				}
			}
		}
	case "arrow_function", "function_expression", "generator_function", "class":
		if item := p.usage[p.key(n)]; item != nil {
			return jsReference{qualified: item.d.ID}
		}
	case "generic_type":
		return p.reference(p.field(n, "name"), depth+1)
	case "instantiation_expression":
		if n.NamedChildCount() > 0 {
			return p.reference(n.NamedChild(0), depth+1)
		}
	case "identifier", "undefined", "type_identifier", "shorthand_property_identifier":
		name := p.text(n)
		if b, bound := p.lookup(name, n); bound {
			if b == nil {
				return jsReference{}
			}
			return b.file.bindingReference(b, depth+1)
		}
		if slices.Contains([]string{"process", "require", "eval", "JSON", "parseInt", "parseFloat", "Number", "URL", "Date", "Worker", "Promise", "String", "Boolean", "Array", "Function", "undefined"}, name) || slices.Contains(javascript.Rules.ErrorBases, name) {
			return jsReference{qualified: "global." + name}
		}
	case "member_expression", "subscript_expression":
		base := p.reference(p.field(n, "object"), depth+1)
		name := p.propertyName(p.field(n, "property"))
		if p.kind(n) == "subscript_expression" {
			value, ok := p.literal(p.field(n, "index"), depth+1)
			if ok && value.Kind == "String" {
				name = value.Text
			}
		}
		if name == "" {
			return jsReference{}
		}
		q := base.qualified
		if base.typ != "" {
			q = base.typ
		}
		if q != "" {
			owner := q
			q += "." + name
			if exported := p.engine.exports[q]; len(exported) == 1 {
				return exported[0].file.bindingReference(exported[0], depth+1)
			}
			if strings.HasPrefix(q, "module:") {
				p.missing("entries", n, "模块成员的导出不能唯一核对："+q)
				p.missing("risks", n, "模块成员的导出不能唯一核对："+q)
				return jsReference{}
			}
			ref := jsReference{qualified: q}
			if fields := p.engine.matchingFields(owner, name, map[string]bool{}); len(fields) == 1 {
				field := fields[0]
				ref.typ = field.item.p.typeName(field.item.fieldTypes[name], depth+1)
			}
			return ref
		}
	case "meta_property":
		if p.text(n) == "import.meta" {
			return jsReference{qualified: "import.meta"}
		}
	case "call_expression", "new_expression":
		callee := p.field(n, "function")
		if callee == nil {
			callee = p.field(n, "constructor")
		}
		target := jsTarget(p.reference(callee, depth+1).qualified)
		if target == "global.require" {
			args := p.arguments(n)
			if len(args) == 1 {
				value, ok := p.literal(args[0], depth+1)
				if ok && value.Kind == "String" {
					return jsReference{qualified: p.module(value.Text)}
				}
			}
			return jsReference{}
		}
		if target == "commander.program" {
			return jsReference{typ: "commander.Command"}
		}
		if typ := javascript.Rules.Constructors[target]; typ != "" {
			return jsReference{typ: typ}
		}
		if target == "commander.Command.option" || target == "commander.Command.command" {
			return jsReference{typ: "commander.Command"}
		}
		if target == "yargs.Argv.option" || target == "yargs.Argv.options" || target == "yargs.Argv.command" {
			return jsReference{typ: "yargs.Argv"}
		}
		if item := p.engine.byID[target]; item != nil && item.class {
			return jsReference{typ: item.d.ID}
		}
	case "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression":
		if n.NamedChildCount() > 0 {
			return p.reference(n.NamedChild(0), depth+1)
		}
	}
	return jsReference{}
}

func (p *jsFile) owner(n *ts.Node) string {
	for node := n; node != nil; node = p.parent(node) {
		if d := p.declarations[p.key(node)]; d != nil {
			return d.ID
		}
	}
	return p.file.Path
}
func (p *jsFile) arguments(n *ts.Node) []*ts.Node {
	args := p.field(n, "arguments")
	if args == nil {
		return nil
	}
	var result []*ts.Node
	for i := 0; i < args.ChildCount(); i++ {
		child := args.Child(i)
		if child.IsNamed() && p.kind(child) != "comment" {
			result = append(result, child)
		}
	}
	return result
}
func (p *jsFile) propertyName(n *ts.Node) string {
	if slices.Contains([]string{"property_identifier", "private_property_identifier", "identifier", "shorthand_property_identifier", "shorthand_property_identifier_pattern"}, p.kind(n)) {
		return p.text(n)
	}
	if p.kind(n) == "string" {
		value, ok := javaScriptString(p.text(n))
		if ok {
			return value
		}
	}
	if p.kind(n) == "computed_property_name" && n.NamedChildCount() == 1 {
		if value, ok := p.literal(n.NamedChild(0), 0); ok && value.Kind == "String" {
			return value.Text
		}
	}
	return ""
}
func (p *jsFile) missing(op string, n *ts.Node, reason string) {
	key := fmt.Sprintf("%s:%d:%s", op, n.StartByte(), reason)
	if p.missingKeys[key] {
		return
	}
	p.missingKeys[key] = true
	p.file.Missing = append(p.file.Missing, Missing{p.file.Path, op, fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason), "unsupported"})
}
func (p *jsFile) entry(n *ts.Node, kind, name, method string) {
	for _, old := range p.file.Entries {
		if old.Span == span(n) && old.Kind == kind && old.Name == name && old.Method == method {
			return
		}
	}
	p.file.Entries = append(p.file.Entries, Entry{Kind: kind, Name: name, Method: method, Declaration: p.owner(n), Span: span(n), Evidence: p.text(n)})
}
func (p *jsFile) risk(n *ts.Node, kind string) {
	for _, old := range p.file.Risks {
		if old.Span == span(n) && old.Kind == kind {
			return
		}
	}
	p.file.Risks = append(p.file.Risks, Risk{Kind: kind, Declaration: p.owner(n), Span: span(n), Evidence: p.text(n)})
}
