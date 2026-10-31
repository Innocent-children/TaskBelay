package codescan

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/python"
	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type pyScope struct {
	start, end uint32
	kind       string
}
type pyBinding struct {
	name, qualified, typ   string
	expression, annotation *ts.Node
	declaration            *pyDeclaration
	scope                  pyScope
	available              uint32
}
type pyReference struct{ qualified, typ string }
type pyParameter struct {
	index                       int
	name                        string
	defaultValue                *ts.Node
	keywordOnly, positionalOnly bool
	rest                        string
}
type pyDeclaration struct {
	d                          *Declaration
	p                          *pyFile
	n                          *ts.Node
	bases                      []string
	parents                    []*pyDeclaration
	protocol, external, config bool
	abstractMethods            []string
	parameters                 []pyParameter
	self                       string
	staticmethod, classmethod  bool
	linked                     bool
}
type pyFile struct {
	file        *File
	syntax      *syntaxFile
	engine      *pyAnalysis
	bindings    map[pyScope]map[string][]*pyBinding
	stars       map[pyScope]bool
	missingKeys map[string]bool
	byNode      map[pyScope]*pyDeclaration
}
type pyAnalysis struct {
	ctx            context.Context
	files          []*pyFile
	declarations   []*pyDeclaration
	byName         map[string][]*pyDeclaration
	public         bool
	packageUnknown string
	fieldLinks     []pyFieldLink
	fieldWrites    []pyFieldWrite
}

func analyzePython(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	a := &pyAnalysis{ctx: ctx, byName: map[string][]*pyDeclaration{}}
	for _, file := range snapshot.Files {
		if file.Kind.Language == "python" && parsed[file.Path] != nil {
			a.files = append(a.files, &pyFile{file: file, syntax: parsed[file.Path], engine: a, bindings: map[pyScope]map[string][]*pyBinding{}, stars: map[pyScope]bool{}, missingKeys: map[string]bool{}, byNode: map[pyScope]*pyDeclaration{}})
		}
	}
	if len(a.files) == 0 {
		return
	}
	sort.Slice(a.files, func(i, j int) bool { return a.files[i].file.Path < a.files[j].file.Path })
	a.packageStatus(snapshot)
	for _, p := range a.files {
		for i := range p.file.Declarations {
			d := &p.file.Declarations[i]
			d.Usage = &Usage{}
			n := syntaxNodeAt(p.syntax.tree.RootNode(), p.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				p.missing("usage", p.syntax.tree.RootNode(), "声明的语法位置无法核对："+d.Location())
				continue
			}
			item := &pyDeclaration{d: d, p: p, n: n}
			p.byNode[p.key(n)] = item
			a.declarations = append(a.declarations, item)
			for _, name := range []string{d.ID, pythonModule(d.File) + "." + strings.TrimPrefix(d.Location(), d.File+":"), d.Name} {
				a.byName[name] = append(a.byName[name], item)
			}
		}
	}
	for _, p := range a.files {
		p.bindTree()
	}
	for _, item := range a.declarations {
		a.declare(item)
	}
	for _, item := range a.declarations {
		a.link(item, map[string]bool{})
	}
	a.implementations()
	for _, item := range a.declarations {
		a.fields(item)
	}
	for _, p := range a.files {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.readWrite(p, n) }); err != nil {
			p.missing("usage", p.syntax.tree.RootNode(), err.Error())
		}
	}
	for _, item := range a.declarations {
		for _, parameter := range item.parameters {
			if parameter.defaultValue != nil {
				a.assign(item, &item.d.Usage.Parameters[parameter.index], item.p, parameter.defaultValue, true)
			}
		}
	}
	for _, p := range a.files {
		if err := walkSyntax(ctx, p.syntax.tree.RootNode(), func(n *ts.Node) { a.calls(p, n); p.inspectAPI(n) }); err != nil {
			for _, op := range []string{"usage", "entries", "risks"} {
				p.missing(op, p.syntax.tree.RootNode(), err.Error())
			}
		}
	}
	for _, write := range a.fieldWrites {
		for i := range write.item.d.Usage.Fields {
			slot := &write.item.d.Usage.Fields[i]
			if slot.Name == write.field {
				a.assign(write.item, slot, write.file, write.node, write.implicit)
				break
			}
		}
	}
	a.constructorValues()
	for _, p := range a.files {
		p.analyzeTests()
		for _, op := range []string{"usage", "entries", "risks"} {
			p.file.Operations[op] = Operation{Complete: true}
			for _, m := range p.file.Missing {
				if m.Operation == op || m.Operation == "syntax" {
					p.file.Operations[op] = Operation{Reason: "存在未能核对的位置"}
					break
				}
			}
		}
		sort.Slice(p.file.Entries, func(i, j int) bool {
			a, b := p.file.Entries[i], p.file.Entries[j]
			if a.Span.Start != b.Span.Start {
				return a.Span.Start < b.Span.Start
			}
			return a.Location() < b.Location()
		})
	}
}

func pythonModule(file string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(file, "src/"), ".py")
	name = strings.TrimSuffix(name, "/__init__")
	return strings.ReplaceAll(name, "/", ".")
}

func (a *pyAnalysis) packageStatus(snapshot *Snapshot) {
	if file := snapshot.Files["setup.py"]; file != nil && !file.Kind.Dependency && (file.Mode == "" || file.Mode == "100644" || file.Mode == "100755") {
		a.public = true
		return
	}
	file := snapshot.Files["pyproject.toml"]
	if file == nil {
		return
	}
	entry := grammars.DetectLanguageByName("toml")
	tree, err := parse(a.ctx, entry, file.Data)
	if tree != nil {
		defer tree.Release()
	}
	if err != nil {
		a.packageUnknown = "pyproject.toml 无法解析，公开声明范围尚不能核对"
		return
	}
	_ = walkSyntax(a.ctx, tree.RootNode(), func(n *ts.Node) {
		if n.Type(entry.Language()) == "table" && n.NamedChildCount() > 0 && slices.Equal(tomlKey(n.NamedChild(0), entry.Language(), file.Data), []string{"project"}) {
			a.public = true
		}
	})
}

func (p *pyFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.syntax.language)
}
func (p *pyFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.file.Data)
}
func (p *pyFile) field(n *ts.Node, name string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(name, p.syntax.language)
}
func (p *pyFile) key(n *ts.Node) pyScope { return pyScope{n.StartByte(), n.EndByte(), p.kind(n)} }
func pyScopeKind(kind string) bool {
	switch kind {
	case "module", "function_definition", "lambda", "class_definition", "list_comprehension", "dictionary_comprehension", "set_comprehension", "generator_expression":
		return true
	}
	return false
}
func (p *pyFile) scope(n *ts.Node) *ts.Node {
	for n = n.Parent(); n != nil; n = n.Parent() {
		if pyScopeKind(p.kind(n)) {
			return n
		}
	}
	return p.syntax.tree.RootNode()
}

func (p *pyFile) add(scope *ts.Node, b *pyBinding) {
	b.scope = p.key(scope)
	if p.bindings[b.scope] == nil {
		p.bindings[b.scope] = map[string][]*pyBinding{}
	}
	p.bindings[b.scope][b.name] = append(p.bindings[b.scope][b.name], b)
}

func (p *pyFile) bindTree() {
	err := walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		scope := p.scope(n)
		switch p.kind(n) {
		case "import_statement", "import_from_statement":
			p.bindImport(n, scope)
		case "class_definition", "function_definition":
			name := p.field(n, "name")
			if name == nil {
				return
			}
			item := p.byNode[p.key(n)]
			b := &pyBinding{name: p.text(name), declaration: item, available: n.EndByte()}
			if item != nil {
				b.qualified = item.d.ID
			}
			p.add(scope, b)
			if p.kind(n) == "function_definition" {
				p.bindParameters(n)
			}
		case "lambda":
			p.bindParameters(n)
		case "assignment", "augmented_assignment", "named_expression":
			left, right := p.field(n, "left"), p.field(n, "right")
			if p.kind(n) == "named_expression" {
				left, right = p.field(n, "name"), p.field(n, "value")
				for strings.HasSuffix(p.kind(scope), "comprehension") || p.kind(scope) == "generator_expression" {
					scope = p.scope(scope)
				}
			}
			names := bindingNames(left, p.syntax.language)
			for i, name := range names {
				expression := right
				if len(names) > 1 {
					expression = nil
					if (p.kind(right) == "tuple" || p.kind(right) == "list") && right.NamedChildCount() == len(names) {
						expression = right.NamedChild(i)
					}
				}
				if p.kind(n) == "augmented_assignment" {
					expression = nil
				}
				p.add(scope, &pyBinding{name: p.text(name), expression: expression, annotation: p.field(n, "type"), available: n.EndByte()})
			}
		case "for_statement", "for_in_clause":
			for _, name := range bindingNames(p.field(n, "left"), p.syntax.language) {
				p.add(scope, &pyBinding{name: p.text(name), available: n.StartByte()})
			}
		case "as_pattern_target":
			for i := 0; i < n.NamedChildCount(); i++ {
				for _, name := range bindingNames(n.NamedChild(i), p.syntax.language) {
					p.add(scope, &pyBinding{name: p.text(name), available: n.StartByte()})
				}
			}
		case "delete_statement":
			for i := 0; i < n.NamedChildCount(); i++ {
				for _, name := range bindingNames(n.NamedChild(i), p.syntax.language) {
					p.add(scope, &pyBinding{name: p.text(name), available: n.StartByte()})
				}
			}
		case "global_statement", "nonlocal_statement":
			for i := 0; i < n.NamedChildCount(); i++ {
				name := n.NamedChild(i)
				if p.kind(name) == "identifier" {
					p.add(scope, &pyBinding{name: p.text(name), available: n.StartByte()})
				}
			}
			for _, op := range []string{"usage", "entries", "risks"} {
				p.missing(op, n, "global/nonlocal 的间接绑定需进一步核对")
			}
		}
	})
	if err != nil {
		for _, op := range []string{"usage", "entries", "risks"} {
			p.missing(op, p.syntax.tree.RootNode(), err.Error())
		}
	}
}

func (p *pyFile) bindParameters(function *ts.Node) {
	parameters := p.field(function, "parameters")
	if parameters == nil {
		return
	}
	for i := 0; i < parameters.NamedChildCount(); i++ {
		n := parameters.NamedChild(i)
		name := pyParameterName(p, n)
		if name != nil {
			p.add(function, &pyBinding{name: p.text(name), annotation: p.field(n, "type"), available: function.StartByte()})
		}
	}
}

func pyParameterName(p *pyFile, n *ts.Node) *ts.Node {
	if p.kind(n) == "identifier" {
		return n
	}
	if name := p.field(n, "name"); name != nil {
		return name
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if p.kind(child) == "identifier" {
			return child
		}
		if p.kind(child) == "list_splat_pattern" || p.kind(child) == "dictionary_splat_pattern" {
			return pyParameterName(p, child)
		}
	}
	return nil
}

func (p *pyFile) bindImport(n, scope *ts.Node) {
	module := ""
	if p.kind(n) == "import_from_statement" {
		module = p.text(p.field(n, "module_name"))
		module = p.importModule(module)
	}
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if !child.IsNamed() || n.FieldNameForChild(i, p.syntax.language) == "module_name" {
			continue
		}
		if p.kind(child) == "wildcard_import" {
			p.stars[p.key(scope)] = true
			for _, op := range []string{"entries", "risks", "usage"} {
				p.missing(op, n, "通配导入的名字与遮蔽范围不能完整核对")
			}
			continue
		}
		name := p.text(child)
		alias := ""
		if p.kind(child) == "aliased_import" {
			name = p.text(p.field(child, "name"))
			alias = p.text(p.field(child, "alias"))
		}
		if p.kind(child) != "dotted_name" && p.kind(child) != "aliased_import" {
			continue
		}
		qualified := name
		if module != "" {
			qualified = module + "." + name
		}
		imported := qualified
		if alias == "" {
			alias = name
			if module == "" {
				alias = strings.Split(name, ".")[0]
				qualified = alias
			}
		}
		p.add(scope, &pyBinding{name: alias, qualified: qualified, available: n.EndByte()})
		for _, prefix := range python.Rules.ConcurrentImports {
			if imported == prefix || strings.HasPrefix(imported, prefix+".") {
				p.risk(n, "concurrency")
			}
		}
	}
}

func (p *pyFile) importModule(name string) string {
	if !strings.HasPrefix(name, ".") {
		return name
	}
	dots := len(name) - len(strings.TrimLeft(name, "."))
	directory := path.Dir(strings.TrimPrefix(p.file.Path, "src/"))
	for i := 1; i < dots; i++ {
		directory = path.Dir(directory)
	}
	if directory == "." {
		directory = ""
	}
	return strings.Trim(strings.ReplaceAll(directory, "/", ".")+"."+strings.TrimLeft(name, "."), ".")
}

func (p *pyFile) lookup(name string, at *ts.Node) (*pyBinding, bool) {
	insideFunction := false
	for node := at; node != nil; node = node.Parent() {
		kind := p.kind(node)
		if !pyScopeKind(kind) {
			continue
		}
		if p.outerComprehensionIterable(node, at) {
			continue
		}
		if kind == "function_definition" || kind == "lambda" {
			body := p.field(node, "body")
			if body != nil && at.StartByte() < body.StartByte() {
				continue
			}
			insideFunction = true
		}
		if kind == "class_definition" {
			body := p.field(node, "body")
			if insideFunction || (body != nil && at.StartByte() < body.StartByte()) {
				continue
			}
		}
		key := p.key(node)
		if bindings := p.bindings[key][name]; len(bindings) > 0 {
			if len(bindings) != 1 {
				return nil, true
			}
			b := bindings[0]
			if !insideFunction && b.available > at.StartByte() {
				return nil, true
			}
			return b, true
		}
		if p.stars[key] {
			return nil, true
		}
	}
	return nil, false
}

func (p *pyFile) outerComprehensionIterable(scope, at *ts.Node) bool {
	if !strings.HasSuffix(p.kind(scope), "comprehension") && p.kind(scope) != "generator_expression" {
		return false
	}
	for i := 0; i < scope.ChildCount(); i++ {
		child := scope.Child(i)
		if p.kind(child) == "for_in_clause" {
			right := p.field(child, "right")
			return right != nil && at.StartByte() >= right.StartByte() && at.EndByte() <= right.EndByte()
		}
	}
	return false
}

func (p *pyFile) reference(n *ts.Node, depth int) pyReference {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return pyReference{}
	}
	switch p.kind(n) {
	case "identifier":
		name := p.text(n)
		if b, bound := p.lookup(name, n); bound {
			if b == nil {
				return pyReference{}
			}
			if b.qualified != "" || b.typ != "" {
				return pyReference{b.qualified, b.typ}
			}
			if b.annotation != nil {
				return pyReference{typ: p.typeName(b.annotation, depth+1)}
			}
			if b.expression != nil {
				return p.reference(b.expression, depth+1)
			}
			return pyReference{}
		}
		if slices.Contains([]string{"int", "float", "str", "bool", "list", "dict", "tuple", "set", "object", "open", "isinstance", "staticmethod", "classmethod", "property", "Exception", "DeprecationWarning", "PendingDeprecationWarning"}, name) || slices.Contains(python.Rules.ErrorBases, name) {
			return pyReference{qualified: "builtins." + name}
		}
	case "attribute":
		base := p.reference(p.field(n, "object"), depth+1)
		member := p.text(p.field(n, "attribute"))
		if base.typ != "" {
			return pyReference{qualified: base.typ + "." + member}
		}
		if base.qualified != "" {
			return pyReference{qualified: base.qualified + "." + member}
		}
	case "call":
		callee := p.reference(p.field(n, "function"), depth+1).qualified
		if typ := python.Rules.Constructors[callee]; typ != "" {
			return pyReference{typ: typ}
		}
		if typ := python.Rules.ReceiverMethods[callee]; typ != "" {
			return pyReference{typ: typ}
		}
		if targets := p.engine.byName[callee]; len(targets) == 1 && p.kind(targets[0].n) == "class_definition" {
			return pyReference{typ: targets[0].d.ID}
		}
	case "parenthesized_expression":
		if n.NamedChildCount() == 1 {
			return p.reference(n.NamedChild(0), depth+1)
		}
	}
	return pyReference{}
}

func (p *pyFile) typeName(n *ts.Node, depth int) string {
	if n == nil || depth > 40 {
		return ""
	}
	if p.kind(n) == "type" && n.NamedChildCount() == 1 {
		return p.typeName(n.NamedChild(0), depth+1)
	}
	if p.kind(n) == "subscript" {
		return p.typeName(p.field(n, "value"), depth+1)
	}
	if p.kind(n) == "generic_type" && n.NamedChildCount() > 0 {
		return p.typeName(n.NamedChild(0), depth+1)
	}
	return p.reference(n, depth+1).qualified
}

func (p *pyFile) owner(n *ts.Node) *pyDeclaration {
	for node := n; node != nil; node = node.Parent() {
		if item := p.byNode[p.key(node)]; item != nil {
			return item
		}
		if p.kind(node) == "decorated_definition" {
			if definition := p.field(node, "definition"); definition != nil {
				if item := p.byNode[p.key(definition)]; item != nil {
					return item
				}
			}
		}
	}
	return nil
}

func (p *pyFile) use(n *ts.Node, value string) Use {
	declaration := p.file.Path
	if owner := p.owner(n); owner != nil {
		declaration = owner.d.ID
	}
	return Use{File: p.file.Path, Declaration: declaration, Value: value, Span: span(n), Test: p.file.Kind.Test, Candidate: true}
}

func (p *pyFile) missing(operation string, n *ts.Node, reason string) {
	key := fmt.Sprintf("%s:%d:%s", operation, n.StartByte(), reason)
	if p.missingKeys[key] {
		return
	}
	p.missingKeys[key] = true
	p.file.Missing = append(p.file.Missing, Missing{p.file.Path, operation, fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason), "unsupported"})
}

func (a *pyAnalysis) unknown(item *pyDeclaration, slot *Slot, p *pyFile, n *ts.Node, reason string) {
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
