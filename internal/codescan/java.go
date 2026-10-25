package codescan

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/java"
	ts "github.com/odvcencio/gotreesitter"
)

type javaBinding struct {
	name                           string
	node, annotation, value, scope *ts.Node
	file                           *javaFile
	constant                       bool
}
type javaType struct {
	name string
	node *ts.Node
	file *javaFile
}
type javaReference struct{ qualified, typ string }
type javaFile struct {
	file                   *File
	syntax                 *syntaxFile
	engine                 *javaAnalysis
	packageName            string
	imports, staticImports map[string][]string
	stars, staticStars     []string
	types                  map[*ts.Node]string
	localTypes             map[*ts.Node]map[string][]string
	bindings               map[*ts.Node]map[string][]*javaBinding
	declarations           map[*ts.Node][]*Declaration
	missingKeys            map[string]bool
	usage                  map[*ts.Node]*javaDeclaration
	module                 string
}
type javaAnalysis struct {
	ctx           context.Context
	files         []*javaFile
	types         map[string][]javaType
	members       map[string][]*javaBinding
	methods       map[string][]*javaBinding
	knownTypes    map[string]bool
	knownPackages map[string]bool
	declarations  []*javaDeclaration
	byName        map[string][]*javaDeclaration
	slots         map[*javaBinding]javaSlot
	fieldLinks    []javaFieldLink
	writes        []javaWrite
	overrides     map[string][]*javaDeclaration
}

func analyzeJavaAPIs(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	a := &javaAnalysis{ctx: ctx, types: map[string][]javaType{}, members: map[string][]*javaBinding{}, methods: map[string][]*javaBinding{}, knownTypes: map[string]bool{}, knownPackages: map[string]bool{}}
	for _, name := range java.Types {
		a.knownTypes[name] = true
	}
	for name := range java.Annotations {
		a.knownTypes[name] = true
	}
	for name := range a.knownTypes {
		if index := strings.LastIndex(name, "."); index >= 0 {
			a.knownPackages[name[:index]] = true
		}
	}
	for _, file := range snapshot.Files {
		if file.Kind.Language != "java" || parsed[file.Path] == nil {
			continue
		}
		p := &javaFile{file: file, syntax: parsed[file.Path], engine: a, imports: map[string][]string{}, staticImports: map[string][]string{}, types: map[*ts.Node]string{}, localTypes: map[*ts.Node]map[string][]string{}, bindings: map[*ts.Node]map[string][]*javaBinding{}, declarations: map[*ts.Node][]*Declaration{}, missingKeys: map[string]bool{}}
		a.files = append(a.files, p)
		for i := range file.Declarations {
			d := &file.Declarations[i]
			if n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan); n != nil {
				p.declarations[n] = append(p.declarations[n], d)
			}
		}
	}
	sort.Slice(a.files, func(i, j int) bool { return a.files[i].file.Path < a.files[j].file.Path })
	for _, p := range a.files {
		p.indexTypes()
		a.knownPackages[p.packageName] = true
	}
	for _, p := range a.files {
		p.bindTree()
	}
	a.analyzeUsage(snapshot)
	for _, p := range a.files {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), p.inspectAPI); err != nil {
			p.missing("entries", p.syntax.tree.RootNode(), err.Error())
			p.missing("risks", p.syntax.tree.RootNode(), err.Error())
		}
		for _, operation := range []string{"entries", "risks", "usage"} {
			p.file.Operations[operation] = Operation{Complete: true}
			for _, m := range p.file.Missing {
				if m.Operation == operation || m.Operation == "syntax" || (operation == "usage" && m.Operation == "declarations") {
					p.file.Operations[operation] = Operation{Reason: "存在未能核对的位置"}
					break
				}
			}
		}
		p.analyzeTests()
		sort.Slice(p.file.Entries, func(i, j int) bool {
			x, y := p.file.Entries[i], p.file.Entries[j]
			if x.Span.Start != y.Span.Start {
				return x.Span.Start < y.Span.Start
			}
			return x.Location() < y.Location()
		})
	}
}

func (p *javaFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.syntax.language)
}
func (p *javaFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.file.Data)
}
func (p *javaFile) field(n *ts.Node, key string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(key, p.syntax.language)
}
func (p *javaFile) parent(n *ts.Node) *ts.Node { return p.syntax.parents.parent(n) }
func javaTypeNode(kind string) bool {
	return slices.Contains([]string{"class_declaration", "interface_declaration", "enum_declaration", "record_declaration", "annotation_type_declaration"}, kind)
}
func javaScope(kind string) bool {
	return javaTypeNode(kind) || javaCallable(kind) || slices.Contains([]string{"program", "lambda_expression", "block", "for_statement", "enhanced_for_statement", "catch_clause", "try_with_resources_statement"}, kind)
}
func (p *javaFile) enclosingType(n *ts.Node) *ts.Node {
	for node := n; node != nil; node = p.parent(node) {
		if javaTypeNode(p.kind(node)) {
			return node
		}
		if p.kind(node) == "class_body" && p.kind(p.parent(node)) == "object_creation_expression" {
			return nil
		}
	}
	return nil
}
func (p *javaFile) scope(n *ts.Node) *ts.Node {
	for n = p.parent(n); n != nil; n = p.parent(n) {
		if javaScope(p.kind(n)) {
			return n
		}
	}
	return p.syntax.tree.RootNode()
}
func (p *javaFile) named(n *ts.Node) []*ts.Node {
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

func (p *javaFile) indexTypes() {
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		switch p.kind(n) {
		case "package_declaration":
			for _, child := range p.named(n) {
				if p.kind(child) == "identifier" || p.kind(child) == "scoped_identifier" {
					p.packageName = p.text(child)
				}
			}
		case "import_declaration":
			name := ""
			star, static := false, false
			for i := 0; i < n.ChildCount(); i++ {
				child := n.Child(i)
				switch p.kind(child) {
				case "identifier", "scoped_identifier":
					name = strings.ReplaceAll(p.text(child), " ", "")
				case "asterisk":
					star = true
				case "static":
					static = true
				}
			}
			if name == "" {
				return
			}
			if name == "java.util.concurrent" || strings.HasPrefix(name, "java.util.concurrent.") {
				p.risk(n, "concurrency")
			}
			if star {
				if static {
					p.staticStars = append(p.staticStars, name)
				} else {
					p.stars = append(p.stars, name)
				}
				return
			}
			key := name[strings.LastIndex(name, ".")+1:]
			if static {
				p.staticImports[key] = append(p.staticImports[key], name)
			} else {
				p.imports[key] = append(p.imports[key], name)
			}
		}
		if javaTypeNode(p.kind(n)) {
			name := p.text(p.field(n, "name"))
			prefix := p.packageName
			if owner := p.enclosingType(p.parent(n)); owner != nil {
				prefix = p.types[owner]
			}
			if prefix != "" {
				name = prefix + "." + name
			}
			for parent := p.parent(n); parent != nil; parent = p.parent(parent) {
				if !javaCallable(p.kind(parent)) {
					continue
				}
				name = fmt.Sprintf("%s@%d", name, n.StartByte())
				scope := p.scope(n)
				if p.localTypes[scope] == nil {
					p.localTypes[scope] = map[string][]string{}
				}
				key := p.text(p.field(n, "name"))
				p.localTypes[scope][key] = append(p.localTypes[scope][key], name)
				break
			}
			p.types[n] = name
			p.engine.types[name] = append(p.engine.types[name], javaType{name, n, p})
		}
	})
}

func (p *javaFile) add(scope *ts.Node, b *javaBinding) {
	b.scope = scope
	b.file = p
	if p.bindings[scope] == nil {
		p.bindings[scope] = map[string][]*javaBinding{}
	}
	p.bindings[scope][b.name] = append(p.bindings[scope][b.name], b)
}
func (p *javaFile) hasModifier(n *ts.Node, name string) bool {
	for _, child := range p.named(n) {
		if p.kind(child) == "modifiers" {
			for i := 0; i < child.ChildCount(); i++ {
				if p.text(child.Child(i)) == name {
					return true
				}
			}
		}
	}
	return false
}
func (p *javaFile) bindTree() {
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		switch p.kind(n) {
		case "variable_declarator":
			parent := p.parent(n)
			if p.kind(parent) == "spread_parameter" {
				return
			}
			scope := p.scope(n)
			b := &javaBinding{name: p.text(p.field(n, "name")), node: n, annotation: p.field(parent, "type"), value: p.field(n, "value")}
			if p.kind(parent) == "field_declaration" || p.kind(parent) == "constant_declaration" {
				scope = p.enclosingType(n)
				if scope == nil {
					return
				}
				b.constant = (p.hasModifier(parent, "final") && p.hasModifier(parent, "static")) || p.kind(scope) == "interface_declaration"
				p.engine.members[p.types[scope]+"."+b.name] = append(p.engine.members[p.types[scope]+"."+b.name], b)
			}
			p.add(scope, b)
		case "formal_parameter", "spread_parameter", "catch_formal_parameter":
			name := p.field(n, "name")
			if name == nil {
				for _, child := range p.named(n) {
					if p.kind(child) == "variable_declarator" {
						name = p.field(child, "name")
					}
				}
			}
			if name != nil {
				typ := p.field(n, "type")
				if typ == nil {
					for _, child := range p.named(n) {
						if p.kind(child) == "catch_type" || p.kind(child) == "integral_type" || p.kind(child) == "floating_point_type" || p.kind(child) == "type_identifier" || p.kind(child) == "scoped_type_identifier" || p.kind(child) == "generic_type" || p.kind(child) == "array_type" || p.kind(child) == "boolean_type" {
							typ = child
							break
						}
					}
				}
				p.add(p.scope(n), &javaBinding{name: p.text(name), node: n, annotation: typ})
			}
		case "enhanced_for_statement":
			if name := p.field(n, "name"); name != nil {
				p.add(n, &javaBinding{name: p.text(name), node: n, annotation: p.field(n, "type")})
			}
		case "enum_constant":
			if scope := p.enclosingType(n); scope != nil {
				b := &javaBinding{name: p.text(p.field(n, "name")), node: n}
				p.add(scope, b)
				p.engine.members[p.types[scope]+"."+b.name] = append(p.engine.members[p.types[scope]+"."+b.name], b)
			}
		case "lambda_expression":
			parameters := p.field(n, "parameters")
			if p.kind(parameters) == "identifier" {
				p.add(n, &javaBinding{name: p.text(parameters), node: parameters})
			}
			if p.kind(parameters) == "inferred_parameters" {
				for _, child := range p.named(parameters) {
					p.add(n, &javaBinding{name: p.text(child), node: child})
				}
			}
		case "method_declaration", "constructor_declaration", "compact_constructor_declaration":
			if owner := p.enclosingType(n); owner != nil {
				key := p.types[owner] + "." + p.text(p.field(n, "name"))
				p.engine.methods[key] = append(p.engine.methods[key], &javaBinding{name: key, node: n, annotation: p.field(n, "type"), file: p})
				if p.kind(n) == "compact_constructor_declaration" {
					for _, param := range p.named(p.field(owner, "parameters")) {
						p.add(n, &javaBinding{name: p.text(p.field(param, "name")), node: param, annotation: p.field(param, "type")})
					}
				}
			}
		}
	})
}

func (p *javaFile) lookup(name string, n *ts.Node) (*javaBinding, bool) {
	for node := n; node != nil; node = p.parent(node) {
		if list := p.bindings[node][name]; len(list) > 0 {
			if len(list) != 1 {
				return nil, true
			}
			b := list[0]
			if !javaTypeNode(p.kind(node)) && b.node.StartByte() > n.StartByte() {
				continue
			}
			return b, true
		}
	}
	if owner := p.enclosingType(n); owner != nil {
		if members := p.engine.memberBindings(p.types[owner], name, 0); len(members) > 0 {
			if len(members) == 1 {
				return members[0], true
			}
			return nil, true
		}
	}
	return nil, false
}
func (p *javaFile) typeName(n *ts.Node, depth int) string {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return ""
	}
	if p.kind(n) == "generic_type" {
		for _, child := range p.named(n) {
			if p.kind(child) != "type_arguments" {
				return p.typeName(child, depth+1)
			}
		}
	}
	if p.kind(n) == "array_type" {
		return ""
	}
	return p.resolveType(strings.Join(strings.Fields(p.text(n)), ""), n, depth+1)
}
func (p *javaFile) resolveType(name string, at *ts.Node, depth int) string {
	if name == "" || depth > 40 || p.engine.ctx.Err() != nil {
		return ""
	}
	first, rest, _ := strings.Cut(name, ".")
	for node := at; node != nil; node = p.parent(node) {
		if local := p.localTypes[node][first]; len(local) > 0 {
			if len(local) != 1 {
				return ""
			}
			if rest != "" {
				return local[0] + "." + rest
			}
			return local[0]
		}
		if parameters := p.field(node, "type_parameters"); parameters != nil {
			for _, parameter := range p.named(parameters) {
				for _, child := range p.named(parameter) {
					if p.kind(child) == "type_identifier" && p.text(child) == first {
						return ""
					}
				}
			}
		}
		if owner := p.types[node]; owner != "" {
			if p.text(p.field(node, "name")) == first {
				if rest != "" {
					return owner + "." + rest
				}
				return owner
			}
			if candidates := p.engine.types[owner+"."+name]; len(candidates) > 0 {
				if len(candidates) == 1 {
					return owner + "." + name
				}
				return ""
			}
		}
	}
	if list := p.imports[first]; len(list) > 0 {
		if len(list) != 1 {
			return ""
		}
		if rest != "" {
			return list[0] + "." + rest
		}
		return list[0]
	}
	if list := p.staticImports[first]; len(list) == 1 && p.engine.knownTypes[list[0]] {
		if rest != "" {
			return list[0] + "." + rest
		}
		return list[0]
	}
	if p.packageName != "" {
		if candidates := p.engine.types[p.packageName+"."+name]; len(candidates) > 0 {
			if len(candidates) == 1 {
				return p.packageName + "." + name
			}
			return ""
		}
	}
	if p.packageName == "" {
		if candidates := p.engine.types[name]; len(candidates) > 0 {
			if len(candidates) == 1 {
				return name
			}
			return ""
		}
	}
	if rest != "" {
		if p.engine.knownTypes[name] || len(p.engine.types[name]) == 1 {
			return name
		}
		if first != "" && first[0] >= 'a' && first[0] <= 'z' {
			return name
		}
		return ""
	}
	var matches []string
	for _, prefix := range append(append([]string(nil), p.stars...), "java.lang") {
		if !p.engine.knownPackages[prefix] {
			return ""
		}
		target := prefix + "." + name
		if p.engine.knownTypes[target] || len(p.engine.types[target]) == 1 {
			matches = append(matches, target)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func (p *javaFile) bindingReference(b *javaBinding, depth int) javaReference {
	if b == nil || depth > 40 {
		return javaReference{}
	}
	if b.file.kind(b.node) == "enum_constant" {
		return javaReference{typ: b.file.types[b.scope]}
	}
	if b.annotation != nil && b.file.text(b.annotation) != "var" {
		return javaReference{typ: b.file.typeName(b.annotation, depth+1)}
	}
	return b.file.reference(b.value, depth+1)
}
func (p *javaFile) reference(n *ts.Node, depth int) javaReference {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return javaReference{}
	}
	switch p.kind(n) {
	case "this":
		return javaReference{typ: p.types[p.enclosingType(n)]}
	case "super":
		if owner := p.enclosingType(n); owner != nil {
			if parents := p.engine.parentTypes(p.types[owner], depth+1); len(parents) > 0 {
				return javaReference{typ: parents[0]}
			}
		}
	case "identifier", "type_identifier", "scoped_type_identifier":
		if b, bound := p.lookup(p.text(n), n); bound {
			return p.bindingReference(b, depth+1)
		}
		if imported := p.staticImports[p.text(n)]; len(imported) == 1 {
			return javaReference{qualified: imported[0]}
		}
		return javaReference{qualified: p.typeName(n, depth+1)}
	case "object_creation_expression":
		return javaReference{typ: p.typeName(p.field(n, "type"), depth+1)}
	case "class_literal":
		return javaReference{typ: "java.lang.Class"}
	case "field_access":
		if p.text(p.field(n, "field")) == "this" {
			return javaReference{typ: p.resolveType(p.text(p.field(n, "object")), n, depth+1)}
		}
		if q := strings.Join(strings.Fields(p.text(n)), ""); p.engine.knownTypes[q] || len(p.engine.types[q]) == 1 {
			first := strings.Split(q, ".")[0]
			if _, bound := p.lookup(first, n); !bound {
				return javaReference{qualified: p.resolveType(q, n, depth+1)}
			}
		}
		base := p.reference(p.field(n, "object"), depth+1)
		owner := base.qualified
		if base.typ != "" {
			owner = base.typ
		}
		if owner == "" {
			return javaReference{}
		}
		key := owner + "." + p.text(p.field(n, "field"))
		if bindings := p.engine.memberBindings(owner, p.text(p.field(n, "field")), depth+1); len(bindings) == 1 {
			return p.bindingReference(bindings[0], depth+1)
		}
		return javaReference{qualified: key}
	case "method_invocation":
		target := p.methodTarget(n, depth+1)
		if typ := java.Rules.ReceiverMethods[target]; typ != "" {
			return javaReference{typ: typ}
		}
		if methods := p.engine.methods[target]; len(methods) == 1 {
			return javaReference{typ: methods[0].file.typeName(methods[0].annotation, depth+1)}
		}
	case "cast_expression":
		return javaReference{typ: p.typeName(p.field(n, "type"), depth+1)}
	case "parenthesized_expression":
		if children := p.named(n); len(children) == 1 {
			return p.reference(children[0], depth+1)
		}
	}
	return javaReference{}
}
func (p *javaFile) methodTarget(n *ts.Node, depth int) string {
	if depth > 40 {
		return ""
	}
	name := p.text(p.field(n, "name"))
	object := p.field(n, "object")
	if object != nil {
		ref := p.reference(object, depth+1)
		owner := ref.qualified
		if ref.typ != "" {
			owner = ref.typ
		}
		if owner != "" {
			return p.engine.apiMethod(owner, name, depth+1)
		}
		return ""
	}
	if owner := p.enclosingType(n); owner != nil {
		target := p.engine.apiMethod(p.types[owner], name, depth+1)
		if len(p.engine.methods[target]) > 0 || javaKnownMethod(target) {
			return target
		}
	}
	if values := p.staticImports[name]; len(values) == 1 {
		return values[0]
	} else if len(values) > 1 {
		return ""
	}
	var matches []string
	for _, prefix := range p.staticStars {
		if !p.engine.knownTypes[prefix] && len(p.engine.types[prefix]) != 1 {
			return ""
		}
		target := prefix + "." + name
		known := len(p.engine.methods[target]) > 0 || javaKnownMethod(target)
		for _, rule := range java.Rules.Entries {
			known = known || rule.Target == target
		}
		if known {
			matches = append(matches, target)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return ""
}

func (p *javaFile) owner(n *ts.Node) string {
	for node := n; node != nil; node = p.parent(node) {
		if ds := p.declarations[node]; len(ds) == 1 {
			return ds[0].ID
		}
		if ds := p.declarations[node]; len(ds) > 1 {
			for _, d := range ds {
				if d.NameSpan.Start <= n.StartByte() && d.NameSpan.End >= n.EndByte() {
					return d.ID
				}
			}
		}
	}
	return p.file.Path
}
func (p *javaFile) missing(operation string, n *ts.Node, reason string) {
	if n == nil {
		n = p.syntax.tree.RootNode()
	}
	key := fmt.Sprintf("%s:%d:%s", operation, n.StartByte(), reason)
	if !p.missingKeys[key] {
		p.missingKeys[key] = true
		p.file.Missing = append(p.file.Missing, Missing{p.file.Path, operation, fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason)})
	}
}
func (p *javaFile) entry(n *ts.Node, kind, name, method string) {
	for _, entry := range p.file.Entries {
		if entry.Span == span(n) && entry.Kind == kind && entry.Name == name && entry.Method == method {
			return
		}
	}
	p.file.Entries = append(p.file.Entries, Entry{Kind: kind, Name: name, Method: method, Declaration: p.owner(n), Span: span(n), Evidence: p.text(n)})
}
func (p *javaFile) risk(n *ts.Node, kind string) {
	for _, risk := range p.file.Risks {
		if risk.Span == span(n) && risk.Kind == kind {
			return
		}
	}
	p.file.Risks = append(p.file.Risks, Risk{Kind: kind, Declaration: p.owner(n), Span: span(n), Evidence: p.text(n)})
}
