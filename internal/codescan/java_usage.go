package codescan

import (
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/java"
	ts "github.com/odvcencio/gotreesitter"
)

type javaDeclaration struct {
	d          *Declaration
	p          *javaFile
	n          *ts.Node
	parameters []*javaBinding
}

func javaCallable(kind string) bool {
	return kind == "method_declaration" || kind == "constructor_declaration" || kind == "compact_constructor_declaration"
}

func (a *javaAnalysis) analyzeUsage(snapshot *Snapshot) {
	a.byName = map[string][]*javaDeclaration{}
	a.slots = map[*javaBinding]javaSlot{}
	a.overrides = map[string][]*javaDeclaration{}
	modules, incomplete := map[string]bool{}, map[string]bool{}
	for _, file := range snapshot.Files {
		if file.Kind.Language == "java" && !file.CodeComplete {
			incomplete[javaModule(file.Path, snapshot)] = true
		}
	}
	for _, p := range a.files {
		p.usage = map[*ts.Node]*javaDeclaration{}
		p.module = javaModule(p.file.Path, snapshot)
		incomplete[p.module] = incomplete[p.module] || !p.file.CodeComplete
		for i := range p.file.Declarations {
			d := &p.file.Declarations[i]
			d.Usage = &Usage{}
			n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				p.missing("usage", p.syntax.tree.RootNode(), "声明语法位置不能核对："+d.Location())
				continue
			}
			item := &javaDeclaration{d: d, p: p, n: n}
			d.Visibility = Visibility{Scope: java.VisibilityScope(p.hasModifier(n, "private"))}
			a.declarations = append(a.declarations, item)
			a.byName[d.Name] = append(a.byName[d.Name], item)
			if len(p.declarations[n]) == 1 {
				p.usage[n] = item
			}
			if p.kind(n) == "method_declaration" && d.Name == "main" {
				modules[p.module] = true
			}
			for _, annotation := range p.annotations(n) {
				name := p.typeName(p.field(annotation, "name"), 0)
				if name == "org.springframework.boot.autoconfigure.SpringBootApplication" {
					modules[p.module] = true
				} else if name == "" && p.text(p.field(annotation, "name")) == "SpringBootApplication" {
					incomplete[p.module] = true
				}
			}
		}
	}
	for _, item := range a.declarations {
		a.declareJava(item, !modules[item.p.module], incomplete[item.p.module])
	}
	for _, item := range a.declarations {
		if javaTypeNode(item.p.kind(item.n)) {
			item.d.Usage.Methods = a.methodNames(item.p.types[item.n], map[string]bool{})
		}
		if item.p.kind(item.n) == "method_declaration" && !item.p.hasModifier(item.n, "static") && !item.p.hasModifier(item.n, "private") {
			seen := map[string]bool{}
			var visit func(string)
			visit = func(name string) {
				if name == "" || seen[name] || a.ctx.Err() != nil {
					return
				}
				seen[name] = true
				key := name + "." + item.d.Name
				a.overrides[key] = append(a.overrides[key], item)
				for _, parent := range a.parentTypes(name, 0) {
					visit(parent)
				}
			}
			for _, parent := range a.parentTypes(item.p.types[item.p.enclosingType(item.n)], 0) {
				visit(parent)
			}
		}
	}
	for _, item := range a.declarations {
		a.javaSlots(item)
		if javaTypeNode(item.p.kind(item.n)) && !item.d.Usage.Abstract {
			a.javaImplementation(item.p.types[item.n], item.p, item.n, item.d.ID, map[string]bool{})
		}
	}
	for _, p := range a.files {
		if err := walkSyntax(a.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
			a.javaReadWrite(p, n)
			a.javaTypes(p, n)
		}); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	a.javaPrepareValues()
	for _, p := range a.files {
		if err := walkSyntax(a.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.javaCalls(p, n) }); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	a.javaConstructorValues()
}

func javaModule(file string, snapshot *Snapshot) string {
	for dir := path.Dir(file); ; dir = path.Dir(dir) {
		for _, manifest := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
			if snapshot.Files[path.Join(dir, manifest)] != nil {
				return dir
			}
		}
		if dir == "." || dir == "/" {
			return "."
		}
	}
}

func (p *javaFile) use(n *ts.Node, value string) Use {
	return Use{File: p.file.Path, Declaration: p.owner(n), Value: value, Span: span(n), Test: p.file.Kind.Test, Candidate: true}
}

func (a *javaAnalysis) unknown(item *javaDeclaration, slot *Slot, p *javaFile, n *ts.Node, reason string) {
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

func (a *javaAnalysis) typeItem(name string) *javaDeclaration {
	if types := a.types[name]; len(types) == 1 {
		return types[0].file.usage[types[0].node]
	}
	return nil
}

func (a *javaAnalysis) declareJava(item *javaDeclaration, library, incomplete bool) {
	p, d, n := item.p, item.d, item.n
	u := d.Usage
	u.Abstract = p.kind(n) == "interface_declaration" || p.kind(n) == "annotation_type_declaration" || (javaTypeNode(p.kind(n)) && p.hasModifier(n, "abstract"))
	public := p.hasModifier(n, "public")
	for parent := p.parent(n); parent != nil; parent = p.parent(parent) {
		if !javaTypeNode(p.kind(parent)) {
			continue
		}
		if p.kind(parent) == "interface_declaration" {
			public = public || p.kind(n) == "method_declaration" || p.kind(n) == "constant_declaration"
		}
		if !p.hasModifier(parent, "public") {
			public = false
		}
	}
	u.Public = public && library
	if public && incomplete {
		a.unknown(item, nil, p, n, "模块源码未完整核对，公开 API 范围不能确定")
	}
	u.Deprecated = strings.Contains(d.Documentation, "/**") && javaDeprecatedDoc(d.Documentation)
	for _, annotation := range p.annotations(n) {
		if p.typeName(p.field(annotation, "name"), 0) == "java.lang.Deprecated" {
			u.Deprecated = true
		}
	}
	if (javaTypeNode(p.kind(n)) || p.kind(n) == "method_declaration") && len(p.annotations(n)) > 0 {
		u.Implicit = append(u.Implicit, "带注解的类或方法")
	}
	if p.kind(n) == "method_declaration" && slices.Contains(java.Rules.ImplicitMethods, d.Name) {
		u.Implicit = append(u.Implicit, "特殊方法："+d.Name)
	}
	for _, parameter := range p.named(p.field(n, "type_parameters")) {
		for _, child := range p.named(parameter) {
			if p.kind(child) == "type_identifier" {
				u.TypeParameters = append(u.TypeParameters, p.text(child))
				break
			}
		}
	}
	u.Generic = len(u.TypeParameters) > 0
	if javaTypeNode(p.kind(n)) {
		u.Error = a.javaError(p.types[n], map[string]bool{})
		for _, base := range a.parentTypes(p.types[n], 0) {
			if base == "" || (!a.knownTypes[base] && len(a.types[base]) != 1) {
				a.unknown(item, nil, p, n, "继承类型的导入、遮蔽或类型实参不能核对")
			}
		}
	}
}

func javaDeprecatedDoc(text string) bool {
	index := strings.Index(text, "@deprecated")
	if index < 0 {
		return false
	}
	rest := text[index+len("@deprecated"):]
	return len(rest) == 0 || strings.ContainsRune(" \r\n\t*", rune(rest[0]))
}

func (a *javaAnalysis) javaError(name string, seen map[string]bool) bool {
	if slices.Contains([]string{"java.lang.Exception", "java.lang.RuntimeException", "java.lang.Throwable"}, name) {
		return true
	}
	if name == "" || seen[name] || a.ctx.Err() != nil {
		return false
	}
	seen[name] = true
	for _, parent := range a.parentTypes(name, 0) {
		if a.javaError(parent, seen) {
			return true
		}
	}
	return false
}

func (a *javaAnalysis) methodNames(name string, seen map[string]bool) []string {
	if name == "" || seen[name] || a.ctx.Err() != nil {
		return nil
	}
	seen[name] = true
	item := a.typeItem(name)
	if item == nil {
		return nil
	}
	var names []string
	for _, candidate := range a.declarations {
		if candidate.p == item.p && candidate.p.enclosingType(candidate.n) == item.n && candidate.p.kind(candidate.n) == "method_declaration" && !candidate.p.hasModifier(candidate.n, "static") {
			names = append(names, candidate.d.Name)
		}
	}
	for _, parent := range a.parentTypes(name, 0) {
		names = append(names, a.methodNames(parent, seen)...)
	}
	sort.Strings(names)
	return slices.Compact(names)
}

func (a *javaAnalysis) javaImplementation(name string, p *javaFile, n *ts.Node, value string, seen map[string]bool) {
	if name == "" || seen[name] || a.ctx.Err() != nil {
		return
	}
	seen[name] = true
	if item := a.typeItem(name); item != nil && item.d.Usage.Abstract {
		addUse(&item.d.Usage.Implementations, p.use(n, value))
	}
	for _, parent := range a.parentTypes(name, 0) {
		a.javaImplementation(parent, p, n, value, seen)
	}
}

func (a *javaAnalysis) javaContract(item *javaDeclaration) bool {
	p, n := item.p, item.n
	for _, annotation := range p.annotations(n) {
		if p.typeName(p.field(annotation, "name"), 0) == "java.lang.Override" {
			return true
		}
	}
	if p.hasModifier(n, "abstract") || p.kind(p.enclosingType(n)) == "interface_declaration" {
		return true
	}
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(name string) bool {
		if name == "" || seen[name] {
			return false
		}
		seen[name] = true
		if base := a.typeItem(name); base != nil && base.d.Usage.Abstract && slices.Contains(base.d.Usage.Methods, item.d.Name) {
			return true
		}
		for _, parent := range a.parentTypes(name, 0) {
			if visit(parent) {
				return true
			}
		}
		return false
	}
	return visit(p.types[p.enclosingType(n)])
}

func (a *javaAnalysis) javaGeneric(item *javaDeclaration, p *javaFile, n, args *ts.Node) {
	if item == nil || !item.d.Usage.Generic {
		return
	}
	values := p.named(args)
	var types []string
	complete := len(values) == len(item.d.Usage.TypeParameters)
	for _, value := range values {
		name := p.canonicalType(value)
		if name == "" {
			complete = false
		}
		types = append(types, name)
	}
	if !complete {
		item.d.Usage.Inferred = true
		a.unknown(item, nil, p, n, "泛型省略类型实参、使用菱形语法或依赖类型变量，不能完整计数")
		return
	}
	addUse(&item.d.Usage.TypeArguments, p.use(n, "<"+strings.Join(types, ",")+">"))
}

func (p *javaFile) canonicalType(n *ts.Node) string {
	if n == nil {
		return ""
	}
	if p.kind(n) == "generic_type" {
		name := p.typeName(n, 0)
		if name == "" {
			return ""
		}
		var arguments []string
		for _, child := range p.named(n) {
			if p.kind(child) == "type_arguments" {
				for _, arg := range p.named(child) {
					value := p.canonicalType(arg)
					if value == "" {
						return ""
					}
					arguments = append(arguments, value)
				}
			}
		}
		if len(arguments) == 0 {
			return ""
		}
		return name + "<" + strings.Join(arguments, ",") + ">"
	}
	if p.kind(n) == "array_type" {
		name := p.canonicalType(p.field(n, "element"))
		if name == "" {
			return ""
		}
		return name + strings.Join(strings.Fields(p.text(p.field(n, "dimensions"))), "")
	}
	if slices.Contains([]string{"integral_type", "floating_point_type", "boolean_type", "void_type"}, p.kind(n)) {
		return p.text(n)
	}
	return p.typeName(n, 0)
}

func (a *javaAnalysis) javaTypes(p *javaFile, n *ts.Node) {
	switch p.kind(n) {
	case "class_declaration", "record_declaration", "enum_declaration":
		if p.usage[n] == nil && !p.hasModifier(n, "abstract") {
			a.javaImplementation(p.types[n], p, n, fmt.Sprintf("%s:local@%d", p.file.Path, n.StartByte()), map[string]bool{})
		}
	case "generic_type":
		var args *ts.Node
		for _, child := range p.named(n) {
			if p.kind(child) == "type_arguments" {
				args = child
			}
		}
		a.javaGeneric(a.typeItem(p.typeName(n, 0)), p, n, args)
	case "type_identifier", "scoped_type_identifier":
		parent := p.parent(n)
		if slices.Contains([]string{"generic_type", "scoped_type_identifier", "type_parameter", "annotation", "marker_annotation"}, p.kind(parent)) {
			return
		}
		if name := p.field(parent, "name"); name != nil && span(name) == span(n) {
			return
		}
		a.javaGeneric(a.typeItem(p.typeName(n, 0)), p, n, nil)
	case "object_creation_expression":
		for _, child := range p.named(n) {
			if p.kind(child) == "class_body" {
				a.javaImplementation(p.typeName(p.field(n, "type"), 0), p, n, fmt.Sprintf("%s:anonymous@%d", p.file.Path, n.StartByte()), map[string]bool{})
				break
			}
		}
	case "catch_type":
		for _, typ := range p.named(n) {
			a.javaErrorCheck(p, typ, n)
		}
	case "instanceof_expression":
		a.javaErrorCheck(p, p.field(n, "right"), n)
	}
}

func (a *javaAnalysis) javaErrorCheck(p *javaFile, typ, n *ts.Node) {
	if item := a.typeItem(p.typeName(typ, 0)); item != nil && item.d.Usage.Error {
		addUse(&item.d.Usage.ErrorChecks, p.use(n, p.text(typ)))
	}
}
