package codescan

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	ts "github.com/odvcencio/gotreesitter"
)

type jsParameter struct {
	slots                 []int
	pattern, defaultValue *ts.Node
	variadic, optional    bool
}

type jsExport struct {
	defaultExport bool
	statement     *ts.Node
}

func (p *jsFile) markExport(name string, defaultExport bool, statement *ts.Node) {
	old := p.exportedNames[name]
	p.exportedNames[name] = jsExport{defaultExport || old.defaultExport, statement}
}

type jsDeclaration struct {
	d                       *Declaration
	p                       *jsFile
	n, function             *ts.Node
	class, exported, linked bool
	bases                   []string
	parents                 []*jsDeclaration
	parameters              []jsParameter
	fieldTypes              map[string]*ts.Node
}

func (a *jsAnalysis) initUsage(snapshot *Snapshot) {
	a.byID = map[string]*jsDeclaration{}
	a.byName = map[string][]*jsDeclaration{}
	a.methodsByOwner = map[string][]*jsDeclaration{}
	if file := snapshot.Files["package.json"]; file != nil {
		var manifest map[string]json.RawMessage
		if err := json.Unmarshal(file.Data, &manifest); err != nil {
			a.packageUnknown = "package.json 无法解析，公开声明范围不能核对"
		} else {
			private := strings.TrimSpace(string(manifest["private"]))
			if private != "" && private != "true" && private != "false" {
				a.packageUnknown = "package.json 的 private 不是布尔值，公开声明范围不能核对"
			} else {
				a.public = private != "true"
			}
		}
	}
	for _, p := range a.ordered {
		p.usage = map[jsScope]*jsDeclaration{}
		for i := range p.file.Declarations {
			d := &p.file.Declarations[i]
			d.Usage = &Usage{}
			n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				p.missing("usage", p.syntax.tree.RootNode(), "声明的语法位置无法核对："+d.Location())
				continue
			}
			item := &jsDeclaration{d: d, p: p, n: n}
			if jsCallable(p.kind(n)) {
				item.function = n
			}
			if p.kind(n) == "variable_declarator" {
				value := p.field(n, "value")
				if jsFunction(p.kind(value)) {
					item.function = value
				}
				if p.kind(value) == "class" {
					item.class = true
					p.usage[p.key(value)] = item
				}
			}
			item.class = item.class || slices.Contains([]string{"class_declaration", "abstract_class_declaration", "interface_declaration"}, p.kind(n))
			p.usage[p.key(n)] = item
			if item.function != nil {
				p.usage[p.key(item.function)] = item
			}
			a.declarations = append(a.declarations, item)
			a.byID[d.ID] = item
			a.byName[d.Name] = append(a.byName[d.Name], item)
		}
	}
	for _, item := range a.declarations {
		if item.function != nil {
			if owner := item.p.thisClass(item.function); owner != nil {
				a.methodsByOwner[owner.d.ID] = append(a.methodsByOwner[owner.d.ID], item)
			}
		}
	}
}

func jsCallable(kind string) bool {
	return jsFunction(kind) || slices.Contains([]string{"function_signature", "method_signature", "abstract_method_signature"}, kind)
}

func (p *jsFile) thisClass(n *ts.Node) *jsDeclaration {
	for node := n; node != nil; node = p.parent(node) {
		if jsFunction(p.kind(node)) && p.kind(node) != "method_definition" && p.kind(node) != "arrow_function" {
			return nil
		}
		if p.kind(node) == "method_definition" && p.kind(p.parent(node)) == "object" {
			return nil
		}
		if slices.Contains([]string{"class", "class_declaration", "abstract_class_declaration", "interface_declaration"}, p.kind(node)) {
			return p.usage[p.key(node)]
		}
	}
	return nil
}

func (p *jsFile) typeName(n *ts.Node, depth int) string {
	if n == nil || depth > 40 {
		return ""
	}
	if p.kind(n) == "type_annotation" && n.NamedChildCount() == 1 {
		return p.typeName(n.NamedChild(0), depth+1)
	}
	if p.kind(n) == "generic_type" {
		return p.typeName(p.field(n, "name"), depth+1)
	}
	return p.reference(n, depth+1).qualified
}

func (p *jsFile) use(n *ts.Node, value string) Use {
	return Use{File: p.file.Path, Declaration: p.owner(n), Value: value, Span: span(n), Test: p.file.Kind.Test, Candidate: true}
}

func (a *jsAnalysis) unknown(item *jsDeclaration, slot *Slot, p *jsFile, n *ts.Node, reason string) {
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

func (p *jsFile) exportContext(item *jsDeclaration) (exported, defaultExport bool, statement *ts.Node) {
	for node := p.parent(item.n); node != nil; node = p.parent(node) {
		if p.kind(node) == "export_statement" {
			exported = true
			defaultExport = strings.HasPrefix(strings.TrimSpace(p.text(node)), "export default ")
			statement = node
			break
		}
		if jsCallable(p.kind(node)) || slices.Contains([]string{"class_body", "interface_body", "statement_block"}, p.kind(node)) {
			break
		}
	}
	if item.d.Owner == "" {
		if value, ok := p.exportedNames[item.d.Name]; ok {
			exported = true
			defaultExport = defaultExport || value.defaultExport
			statement = value.statement
		}
	}
	return
}

func jsHasDecorator(p *jsFile, n *ts.Node) bool {
	if n == nil {
		return false
	}
	for i := 0; i < n.ChildCount(); i++ {
		if p.kind(n.Child(i)) == "decorator" {
			return true
		}
	}
	return false
}

func (a *jsAnalysis) declare(item *jsDeclaration) {
	p, d, n := item.p, item.d, item.n
	u := d.Usage
	exported, defaultExport, statement := p.exportContext(item)
	item.exported = exported
	u.Public = exported && a.public
	if a.packageUnknown != "" && exported {
		a.unknown(item, nil, p, n, a.packageUnknown)
	}
	u.Abstract = p.kind(n) == "interface_declaration" || p.kind(n) == "abstract_class_declaration"
	if defaultExport {
		u.Implicit = append(u.Implicit, "默认导出")
	}
	if exported {
		for _, component := range strings.Split(d.File, "/") {
			if slices.Contains([]string{"pages", "app", "routes"}, component) {
				u.Implicit = append(u.Implicit, "文件路由目录中的导出")
				break
			}
		}
	}
	if d.Owner != "" && slices.Contains(javascript.Rules.ImplicitMethods, d.Name) {
		u.Implicit = append(u.Implicit, "特殊方法："+d.Name)
	}
	if jsHasDecorator(p, n) || jsHasDecorator(p, statement) {
		u.Implicit = append(u.Implicit, "带装饰器的声明")
	}
	documentation := d.Documentation
	if statement != nil {
		for _, token := range leadingComments(statement, p.syntax.language, p.file.Data, p.syntax.parents) {
			documentation += "\n" + token.Text
		}
	}
	if strings.Contains(documentation, "/**") {
		if index := strings.Index(documentation, "@deprecated"); index >= 0 {
			rest := documentation[index+len("@deprecated"):]
			u.Deprecated = len(rest) == 0 || strings.ContainsRune(" \t\r\n*", rune(rest[0]))
		}
	}
	if exported {
		switch p.kind(n) {
		case "variable_declarator":
			if strings.HasPrefix(strings.TrimSpace(p.text(p.parent(n))), "const ") && p.kind(p.field(n, "value")) == "identifier" {
				u.Alias = p.text(p.field(n, "value"))
			}
		case "type_alias_declaration":
			value := p.field(n, "value")
			if p.kind(value) == "type_identifier" || p.kind(value) == "generic_type" {
				u.Alias = p.text(value)
			}
		case "export_specifier":
			if original := p.text(p.field(n, "name")); original != d.Name {
				u.Alias = original
			}
		}
	}
	definition := n
	if item.function != nil {
		definition = item.function
	} else if item.class && p.kind(n) == "variable_declarator" {
		definition = p.field(n, "value")
	}
	if parameters := p.field(definition, "type_parameters"); parameters != nil {
		for i := 0; i < parameters.ChildCount(); i++ {
			child := parameters.Child(i)
			if p.kind(child) == "type_parameter" {
				if name := p.field(child, "name"); name != nil {
					u.TypeParameters = append(u.TypeParameters, p.text(name))
				}
			}
		}
		u.Generic = len(u.TypeParameters) > 0
	}
	if !item.class {
		return
	}
	_ = walkSyntax(a.ctx, definition, func(child *ts.Node) {
		for parent := p.parent(child); parent != nil && parent != definition; parent = p.parent(parent) {
			if p.kind(parent) == "class_body" || p.kind(parent) == "interface_body" {
				return
			}
		}
		switch p.kind(child) {
		case "class_heritage":
			if p.file.Kind.Language == "javascript" {
				for i := 0; i < child.ChildCount(); i++ {
					base := child.Child(i)
					if base.IsNamed() && p.kind(base) != "comment" {
						item.bases = append(item.bases, p.typeName(base, 0))
					}
				}
			}
		case "extends_clause":
			if base := p.field(child, "value"); base != nil {
				item.bases = append(item.bases, p.typeName(base, 0))
			}
		case "implements_clause", "extends_type_clause":
			for i := 0; i < child.ChildCount(); i++ {
				base := child.Child(i)
				if base.IsNamed() && p.kind(base) != "comment" {
					item.bases = append(item.bases, p.typeName(base, 0))
				}
			}
		}
	})
}

func (a *jsAnalysis) typeTarget(name string, seen map[string]bool) *jsDeclaration {
	if name == "" || seen[name] {
		return nil
	}
	seen[name] = true
	item := a.byID[name]
	if item == nil {
		return nil
	}
	if item.p.kind(item.n) == "type_alias_declaration" {
		return a.typeTarget(item.p.typeName(item.p.field(item.n, "value"), 0), seen)
	}
	return item
}

func (a *jsAnalysis) ownMethods(item *jsDeclaration) []*jsDeclaration {
	return a.methodsByOwner[item.d.ID]
}

func (a *jsAnalysis) link(item *jsDeclaration, seen map[string]bool) {
	if item.linked || !item.class || a.ctx.Err() != nil {
		return
	}
	if seen[item.d.ID] {
		a.unknown(item, nil, item.p, item.n, "继承关系形成循环")
		return
	}
	seen[item.d.ID] = true
	defer delete(seen, item.d.ID)
	for _, base := range item.bases {
		if strings.HasPrefix(base, "global.") && slices.Contains(javascript.Rules.ErrorBases, strings.TrimPrefix(base, "global.")) {
			item.d.Usage.Error = true
			continue
		}
		parent := a.typeTarget(base, map[string]bool{})
		if parent == nil || !parent.class {
			a.unknown(item, nil, item.p, item.n, "基类或接口的完整关系不能核对："+base)
			continue
		}
		a.link(parent, seen)
		item.parents = append(item.parents, parent)
		item.d.Usage.Error = item.d.Usage.Error || parent.d.Usage.Error
		item.d.Usage.Methods = append(item.d.Usage.Methods, parent.d.Usage.Methods...)
		if len(parent.d.Usage.Unknown) > 0 {
			a.unknown(item, nil, item.p, item.n, "父类型仍有不能核对的继承关系："+parent.d.Location())
		}
	}
	for _, method := range a.ownMethods(item) {
		item.d.Usage.Methods = append(item.d.Usage.Methods, method.d.Name)
	}
	sort.Strings(item.d.Usage.Methods)
	item.d.Usage.Methods = slices.Compact(item.d.Usage.Methods)
	item.linked = true
}
