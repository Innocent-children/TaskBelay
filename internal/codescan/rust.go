package codescan

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/rust"
	ts "github.com/odvcencio/gotreesitter"
)

type rustBinding struct {
	name, qualified                string
	node, value, annotation, scope *ts.Node
	file                           *rustFile
	constant, hoisted, changed     bool
}
type rustReference struct{ qualified, typ string }
type rustFile struct {
	file         *File
	syntax       *syntaxFile
	engine       *rustAnalysis
	root, module string
	modules      map[*ts.Node]string
	bindings     map[*ts.Node]map[string][]*rustBinding
	imports      map[*ts.Node]map[string][]string
	importNodes  map[*ts.Node][]string
	stars        map[*ts.Node][]string
	declarations map[*ts.Node][]*Declaration
	missingKeys  map[string]bool
	usage        map[*ts.Node]*rustDeclaration
}
type rustAnalysis struct {
	ctx          context.Context
	files        []*rustFile
	symbols      map[string][]*rustBinding
	known        map[string]bool
	declarations []*rustDeclaration
	byTarget     map[string][]*rustDeclaration
	slots        map[*rustBinding]rustSlot
	writes       []rustWrite
}

func analyzeRustAPIs(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	a := &rustAnalysis{ctx: ctx, symbols: map[string][]*rustBinding{}, known: map[string]bool{}}
	for _, name := range rust.KnownPaths {
		a.known[name] = true
	}
	for _, rule := range append(slices.Clone(rust.Rules.Entries), rust.Rules.Risks...) {
		if !strings.HasSuffix(rule.Target, "*") {
			a.known[rule.Target] = true
		}
	}
	for _, verb := range rust.HTTPMethods {
		a.known["actix_web::"+verb] = true
		a.known["axum::routing::"+verb] = true
	}
	for _, file := range snapshot.Files {
		if file.Kind.Language != "rust" || parsed[file.Path] == nil {
			continue
		}
		p := &rustFile{file: file, syntax: parsed[file.Path], engine: a, modules: map[*ts.Node]string{}, bindings: map[*ts.Node]map[string][]*rustBinding{}, imports: map[*ts.Node]map[string][]string{}, importNodes: map[*ts.Node][]string{}, stars: map[*ts.Node][]string{}, declarations: map[*ts.Node][]*Declaration{}, missingKeys: map[string]bool{}}
		p.root, p.module = rustModule(file.Path, snapshot)
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
		p.index()
	}
	a.indexMethods()
	for _, p := range a.files {
		p.mutations()
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
				if m.Operation == operation || m.Operation == "syntax" {
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

func rustModule(file string, snapshot *Snapshot) (string, string) {
	root := "."
	for dir := path.Dir(file); ; dir = path.Dir(dir) {
		if snapshot.Files[path.Join(dir, "Cargo.toml")] != nil {
			root = dir
			break
		}
		if dir == "." || dir == "/" {
			break
		}
	}
	relative := file
	if root != "." {
		relative = strings.TrimPrefix(file, root+"/")
	}
	relative = strings.TrimPrefix(relative, "src/")
	relative = strings.TrimSuffix(relative, ".rs")
	if relative == "lib" || relative == "main" {
		relative = ""
	} else {
		relative = strings.TrimSuffix(relative, "/mod")
	}
	prefix := "local:" + root
	if relative != "" {
		return prefix, prefix + "::" + strings.ReplaceAll(relative, "/", "::")
	}
	return prefix, prefix
}

func (p *rustFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(p.syntax.language)
}
func (p *rustFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(p.file.Data)
}
func (p *rustFile) field(n *ts.Node, key string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(key, p.syntax.language)
}
func (p *rustFile) parent(n *ts.Node) *ts.Node {
	if n == nil {
		return nil
	}
	return p.syntax.parents.parent(n)
}
func (p *rustFile) named(n *ts.Node) []*ts.Node {
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
func rustScope(kind string) bool {
	return slices.Contains([]string{"source_file", "declaration_list", "function_item", "function_signature_item", "block", "closure_expression", "match_arm", "for_expression", "if_expression", "while_expression"}, kind)
}
func (p *rustFile) scope(n *ts.Node) *ts.Node {
	for node := p.parent(n); node != nil; node = p.parent(node) {
		if rustScope(p.kind(node)) {
			return node
		}
	}
	return p.syntax.tree.RootNode()
}
func (p *rustFile) moduleAt(n *ts.Node) string {
	for node := n; node != nil; node = p.parent(node) {
		if m := p.modules[node]; m != "" {
			return m
		}
	}
	return p.module
}
func (p *rustFile) add(scope *ts.Node, b *rustBinding) {
	b.scope, b.file = scope, p
	if p.bindings[scope] == nil {
		p.bindings[scope] = map[string][]*rustBinding{}
	}
	p.bindings[scope][b.name] = append(p.bindings[scope][b.name], b)
	if b.qualified != "" {
		p.engine.symbols[b.qualified] = append(p.engine.symbols[b.qualified], b)
	}
}

func (p *rustFile) index() {
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		if p.kind(n) == "mod_item" {
			if body := p.field(n, "body"); body != nil {
				p.modules[body] = p.moduleAt(p.parent(n)) + "::" + p.text(p.field(n, "name"))
			}
		}
		switch p.kind(n) {
		case "type_parameter", "const_parameter":
			if name := p.field(n, "name"); name != nil {
				p.add(p.parent(p.parent(n)), &rustBinding{name: p.text(name), node: n})
			}
		case "use_declaration":
			p.importNodes[n] = p.useImports(p.scope(n), p.field(n, "argument"), "")
		case "const_item", "static_item", "function_item", "function_signature_item", "struct_item", "enum_item", "trait_item", "type_item", "mod_item":
			name := p.text(p.field(n, "name"))
			if name == "" {
				return
			}
			scope := p.scope(n)
			qualified := p.moduleAt(n) + "::" + name
			if p.kind(scope) != "source_file" && p.kind(scope) != "declaration_list" {
				qualified = fmt.Sprintf("%s@%d", qualified, n.StartByte())
			}
			for node := p.parent(n); node != nil; node = p.parent(node) {
				if p.kind(node) == "impl_item" {
					qualified = p.moduleAt(node) + "::" + p.text(p.field(node, "type")) + "::" + name
					break
				}
				if p.kind(node) == "function_item" {
					break
				}
			}
			p.add(scope, &rustBinding{name: name, qualified: qualified, node: n, value: p.field(n, "value"), annotation: p.field(n, "type"), constant: p.kind(n) == "const_item", hoisted: true})
		case "let_declaration", "parameter", "let_condition":
			pattern := p.field(n, "pattern")
			for _, name := range p.patternNames(pattern) {
				if p.text(name) != "_" {
					p.add(p.scope(n), &rustBinding{name: p.text(name), node: n, value: p.field(n, "value"), annotation: p.field(n, "type")})
				}
			}
		case "for_expression", "match_arm":
			for _, name := range p.patternNames(p.field(n, "pattern")) {
				p.add(n, &rustBinding{name: p.text(name), node: n})
			}
		case "closure_expression":
			for _, name := range p.named(p.field(n, "parameters")) {
				if p.kind(name) == "identifier" {
					p.add(n, &rustBinding{name: p.text(name), node: name})
				}
			}
		}
	})
}

func rustJoin(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "self" || name == "" {
		return prefix
	}
	return prefix + "::" + name
}
func (p *rustFile) useImports(scope, n *ts.Node, prefix string) []string {
	if n == nil {
		return nil
	}
	switch p.kind(n) {
	case "scoped_use_list":
		return p.useImports(scope, p.field(n, "list"), rustJoin(prefix, p.text(p.field(n, "path"))))
	case "use_list":
		var names []string
		for _, child := range p.named(n) {
			names = append(names, p.useImports(scope, child, prefix)...)
		}
		return names
	case "use_wildcard":
		name := strings.TrimSuffix(strings.TrimSpace(p.text(n)), "::*")
		if name == "*" {
			name = ""
		}
		name = strings.TrimSuffix(rustJoin(prefix, name), "::")
		p.stars[scope] = append(p.stars[scope], name)
		return []string{name}
	}
	name, alias := rustJoin(prefix, p.text(n)), ""
	if p.kind(n) == "use_as_clause" {
		name = rustJoin(prefix, p.text(p.field(n, "path")))
		alias = p.text(p.field(n, "alias"))
	}
	if alias == "" {
		parts := strings.Split(name, "::")
		alias = parts[len(parts)-1]
	}
	if alias == "_" {
		return []string{name}
	}
	if p.imports[scope] == nil {
		p.imports[scope] = map[string][]string{}
	}
	p.imports[scope][alias] = append(p.imports[scope][alias], name)
	return []string{name}
}

func (p *rustFile) lookup(name string, n *ts.Node) (*rustBinding, bool) {
	for node := n; node != nil; node = p.parent(node) {
		var found *rustBinding
		for _, b := range p.bindings[node][name] {
			if !b.hoisted && (b.node.StartByte() > n.StartByte() || (slices.Contains([]string{"let_declaration", "let_condition"}, p.kind(b.node)) && b.node.EndByte() > n.StartByte())) {
				continue
			}
			if p.kind(b.node) == "for_expression" {
				if body := p.field(b.node, "body"); body == nil || n.StartByte() < body.StartByte() {
					continue
				}
			}
			if p.kind(b.node) == "let_condition" && p.kind(b.scope) == "if_expression" {
				if body := p.field(b.scope, "consequence"); body != nil && n.StartByte() >= body.EndByte() {
					continue
				}
			}
			if found != nil && b.hoisted && found.hoisted {
				return nil, true
			}
			if found == nil || b.node.StartByte() > found.node.StartByte() {
				found = b
			}
		}
		if found != nil {
			return found, true
		}
	}
	return nil, false
}

func (p *rustFile) resolve(name string, n *ts.Node, depth int) string {
	if name == "" || depth > 40 || p.engine.ctx.Err() != nil {
		return ""
	}
	name = strings.Join(strings.Fields(name), "")
	absolute := strings.HasPrefix(name, "::")
	name = strings.TrimPrefix(name, "::")
	first, rest, _ := strings.Cut(name, "::")
	if !absolute {
		if first == "crate" {
			return rustJoin(p.root, rest)
		}
		if first == "self" {
			return rustJoin(p.moduleAt(n), rest)
		}
		if first == "super" {
			prefix := p.moduleAt(n)
			for first == "super" {
				if i := strings.LastIndex(prefix, "::"); i >= 0 {
					prefix = prefix[:i]
				} else {
					return ""
				}
				first, rest, _ = strings.Cut(rest, "::")
			}
			return rustJoin(prefix, rustJoin(first, rest))
		}
		if binding, found := p.lookup(first, n); found {
			if binding == nil {
				return ""
			}
			if binding.qualified != "" {
				if rest != "" {
					return binding.qualified + "::" + rest
				}
				return binding.qualified
			}
			return ""
		}
		for node := n; node != nil; node = p.parent(node) {
			if imports := p.imports[node][first]; len(imports) > 0 {
				if len(imports) != 1 {
					return ""
				}
				target := imports[0]
				if target == name {
					if p.engine.known[strings.Split(target, "::")[0]] {
						return target
					}
					return ""
				}
				resolved := p.resolveImport(target, node, depth+1)
				if resolved == "" {
					return ""
				}
				if rest != "" {
					return resolved + "::" + rest
				}
				return resolved
			}
		}
		if candidates := p.engine.symbols[p.moduleAt(n)+"::"+name]; len(candidates) > 0 {
			if len(candidates) == 1 {
				return candidates[0].qualified
			}
			return ""
		}
		var candidates []string
		for node := n; node != nil; node = p.parent(node) {
			for _, star := range p.stars[node] {
				prefix := p.resolveImport(star, node, depth+1)
				if prefix == "" {
					return ""
				}
				key := prefix + "::" + first
				if !p.engine.known[prefix] && !strings.HasPrefix(prefix, "local:") {
					return ""
				}
				if p.engine.known[key] || len(p.engine.symbols[key]) == 1 {
					if rest != "" {
						key += "::" + rest
					}
					if !slices.Contains(candidates, key) {
						candidates = append(candidates, key)
					}
				}
			}
		}
		if len(candidates) == 1 {
			return candidates[0]
		}
		if len(candidates) > 1 {
			return ""
		}
		if first == "String" {
			return rustJoin("std::string::String", rest)
		}
	}
	if p.engine.known[first] {
		return name
	}
	return ""
}

func (p *rustFile) resolveImport(name string, scope *ts.Node, depth int) string {
	first, _, _ := strings.Cut(strings.TrimPrefix(name, "::"), "::")
	if !strings.HasPrefix(name, "::") {
		if binding, bound := p.lookup(first, scope); bound && binding != nil && p.kind(binding.node) == "mod_item" {
			return binding.qualified + strings.TrimPrefix(name, first)
		}
	}
	if p.engine.known[first] {
		return strings.TrimPrefix(name, "::")
	}
	if first == "crate" || first == "self" || first == "super" {
		return p.resolve(name, scope, depth+1)
	}
	if target := p.moduleAt(scope) + "::" + name; len(p.engine.symbols[target]) == 1 {
		return target
	}
	return ""
}

func (p *rustFile) mutations() {
	_ = walkSyntax(p.engine.ctx, p.syntax.tree.RootNode(), func(n *ts.Node) {
		var target *ts.Node
		if p.kind(n) == "assignment_expression" || p.kind(n) == "compound_assignment_expr" {
			target = p.field(n, "left")
		} else if p.kind(n) == "reference_expression" {
			for _, child := range p.named(n) {
				if p.kind(child) == "mutable_specifier" {
					target = p.field(n, "value")
				}
			}
		}
		if p.kind(target) == "identifier" {
			if b, ok := p.lookup(p.text(target), target); ok && b != nil {
				b.changed = true
			}
		}
	})
}

func (p *rustFile) owner(n *ts.Node) string {
	for node := n; node != nil; node = p.parent(node) {
		if d := p.declarations[node]; len(d) == 1 {
			return d[0].ID
		}
	}
	return p.file.Path
}
func (p *rustFile) missing(operation string, n *ts.Node, reason string) {
	key := fmt.Sprintf("%s:%d:%s", operation, n.StartByte(), reason)
	if p.missingKeys[key] {
		return
	}
	p.missingKeys[key] = true
	p.file.Missing = append(p.file.Missing, Missing{p.file.Path, operation, fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason), "unsupported"})
}
func (p *rustFile) entry(n *ts.Node, kind, name, method string) {
	entry := Entry{Kind: kind, Name: name, Method: method, Declaration: p.owner(n), Evidence: p.text(n), Span: span(n)}
	if !slices.Contains(p.file.Entries, entry) {
		p.file.Entries = append(p.file.Entries, entry)
	}
}
func (p *rustFile) risk(n *ts.Node, kind string) {
	risk := Risk{Kind: kind, Declaration: p.owner(n), Evidence: p.text(n), Span: span(n)}
	if !slices.Contains(p.file.Risks, risk) {
		p.file.Risks = append(p.file.Risks, risk)
	}
}
