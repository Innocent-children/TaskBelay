package codescan

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/golang"
	ts "github.com/odvcencio/gotreesitter"
)

type goValue struct {
	typeName, text, prefix  string
	constant, prefixUnknown bool
}

type goBinding struct {
	name        string
	scope       Span
	available   uint32
	value       goValue
	expression  *ts.Node
	initializer *ts.Node
	file        *goFile
	resolving   bool
	evaluated   bool
}

type goFile struct {
	file           *File
	syntax         *syntaxFile
	packageName    string
	imports        map[string]string
	bindings       []*goBinding
	bindingsByName map[string][]*goBinding
	// 同名参数可分布在很多互不相交的函数；前缀最大终点用于跳过已结束的作用域。
	bindingEnds   map[string][]uint32
	globals       map[string][]*goBinding
	ctx           context.Context
	unknown       map[string]bool
	mockTypes     map[string]bool
	mockFactories map[string]string
}

func analyzeGoAPIs(ctx context.Context, snapshot *Snapshot, parsed map[string]*syntaxFile) {
	packages := map[string]map[string][]*goBinding{}
	var files []*goFile
	paths := make([]string, 0, len(parsed))
	for name := range parsed {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	for _, name := range paths {
		file := snapshot.Files[name]
		if file.Kind.Language != "go" {
			continue
		}
		g := &goFile{file: file, syntax: parsed[name], imports: map[string]string{}, ctx: ctx, unknown: map[string]bool{}}
		root := g.syntax.tree.RootNode()
		for i := 0; i < root.NamedChildCount(); i++ {
			n := root.NamedChild(i)
			if g.kind(n) == "package_clause" && n.NamedChildCount() > 0 {
				g.packageName = g.text(n.NamedChild(0))
			}
		}
		key := path.Dir(name) + ":" + g.packageName
		if packages[key] == nil {
			packages[key] = map[string][]*goBinding{}
		}
		g.globals = packages[key]
		_ = walkSyntax(ctx, root, func(n *ts.Node) {
			if g.kind(n) == "import_spec" {
				p := g.field(n, "path")
				if p == nil {
					for i := 0; i < n.NamedChildCount(); i++ {
						if strings.Contains(g.kind(n.NamedChild(i)), "string_literal") {
							p = n.NamedChild(i)
						}
					}
				}
				value, err := strconv.Unquote(g.text(p))
				if err != nil {
					return
				}
				alias := g.text(g.field(n, "name"))
				if alias == "" {
					alias = path.Base(value)
					if strings.HasPrefix(alias, "v") {
						if _, err := strconv.Atoi(alias[1:]); err == nil {
							alias = path.Base(path.Dir(value))
						}
					}
					if strings.HasPrefix(value, "gopkg.in/") {
						alias = strings.Split(alias, ".")[0]
					}
					if known := golang.Rules.ImportNames[value]; known != "" {
						alias = known
					}
				}
				if alias == "." {
					g.missing("entries", n, "点导入的调用尚不能唯一核对")
					g.missing("risks", n, "点导入的调用尚不能唯一核对")
				} else if alias != "_" {
					g.imports[alias] = value
				}
			}
		})
		_ = walkSyntax(ctx, root, func(n *ts.Node) {
			kind := g.kind(n)
			if kind != "var_spec" && kind != "const_spec" && kind != "function_declaration" && kind != "type_spec" && kind != "type_alias" {
				return
			}
			if g.enclosingCallable(n) != nil {
				return
			}
			if kind == "var_spec" || kind == "const_spec" {
				g.bindSpec(n, true)
				return
			}
			if name := g.field(n, "name"); name != nil {
				b := &goBinding{name: g.text(name), scope: span(root), file: g}
				g.globals[b.name] = append(g.globals[b.name], b)
			}
		})
		files = append(files, g)
	}
	for _, g := range files {
		// 局部定义按作用域和声明位置解析，遮蔽同名导入后不能继续套用库规则。
		err := walkSyntax(ctx, g.syntax.tree.RootNode(), func(n *ts.Node) {
			switch g.kind(n) {
			case "parameter_declaration", "variadic_parameter_declaration":
				g.bindParameters(n)
			case "var_spec", "const_spec":
				if g.enclosingCallable(n) != nil {
					g.bindSpec(n, false)
				}
			case "short_var_declaration", "assignment_statement", "range_clause":
				g.bindAssignment(n)
			}
			g.inspect(n)
		})
		if err != nil {
			g.missing("entries", g.syntax.tree.RootNode(), err.Error())
			g.missing("risks", g.syntax.tree.RootNode(), err.Error())
		}
	}
	analyzeGoUsage(ctx, files)
	indexGoTestMocks(ctx, snapshot, files)
	for _, g := range files {
		g.analyzeTests()
		for _, operation := range []string{"entries", "risks", "usage"} {
			complete := true
			for _, missing := range g.file.Missing {
				if missing.Operation == operation || missing.Operation == "syntax" {
					complete = false
				}
			}
			g.file.Operations[operation] = Operation{Complete: complete}
			if !complete {
				g.file.Operations[operation] = Operation{Reason: "存在未能核对的位置"}
			}
		}
		sort.Slice(g.file.Entries, func(i, j int) bool {
			a, b := g.file.Entries[i], g.file.Entries[j]
			if a.Span.Start != b.Span.Start {
				return a.Span.Start < b.Span.Start
			}
			return a.Location() < b.Location()
		})
	}
}

func (g *goFile) kind(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Type(g.syntax.language)
}
func (g *goFile) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Text(g.file.Data)
}
func (g *goFile) field(n *ts.Node, name string) *ts.Node {
	if n == nil {
		return nil
	}
	return n.ChildByFieldName(name, g.syntax.language)
}
func (g *goFile) enclosingCallable(n *ts.Node) *ts.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if callable(g.kind(p)) {
			return p
		}
	}
	return nil
}
func (g *goFile) scope(n *ts.Node) Span {
	for p := n.Parent(); p != nil; p = p.Parent() {
		switch g.kind(p) {
		case "block", "if_statement", "for_statement", "expression_switch_statement", "type_switch_statement", "func_literal":
			return span(p)
		}
	}
	return span(g.syntax.tree.RootNode())
}

func (g *goFile) lookup(name string, at uint32) (*goBinding, bool) {
	var found *goBinding
	bindings := g.bindings
	if g.bindingsByName != nil {
		bindings = g.bindingsByName[name]
		upper := sort.Search(len(bindings), func(i int) bool { return bindings[i].scope.Start > at })
		lower := upper
		ends := g.bindingEnds[name]
		for lower > 0 && ends[lower-1] > at {
			lower--
		}
		bindings = bindings[lower:upper]
	}
	for _, b := range bindings {
		if b.name != name || b.available > at || b.scope.Start > at || b.scope.End <= at {
			continue
		}
		if found == nil || b.scope.End-b.scope.Start < found.scope.End-found.scope.Start || (b.scope == found.scope && b.available > found.available) {
			found = b
		}
	}
	if found != nil {
		return found, true
	}
	if bindings := g.globals[name]; len(bindings) == 1 {
		return bindings[0], true
	} else if len(bindings) > 1 {
		return nil, true
	}
	return nil, false
}

func (g *goFile) addBinding(b *goBinding) {
	if g.bindingsByName == nil {
		g.bindingsByName = map[string][]*goBinding{}
		g.bindingEnds = map[string][]uint32{}
	}
	g.bindings = append(g.bindings, b)
	bindings := g.bindingsByName[b.name]
	index := sort.Search(len(bindings), func(i int) bool { return bindings[i].scope.Start > b.scope.Start })
	bindings = append(bindings, nil)
	copy(bindings[index+1:], bindings[index:])
	bindings[index] = b
	ends := append(g.bindingEnds[b.name], 0)
	for i := index; i < len(bindings); i++ {
		ends[i] = bindings[i].scope.End
		if i > 0 {
			ends[i] = max(ends[i], ends[i-1])
		}
	}
	g.bindingsByName[b.name], g.bindingEnds[b.name] = bindings, ends
}

func (g *goFile) bindingValue(b *goBinding, depth int) goValue {
	if b == nil || depth > 30 || g.ctx.Err() != nil {
		return goValue{}
	}
	value := b.value
	if b.expression != nil {
		if b.resolving {
			b.file.missing("entries", b.expression, "常量引用出现循环，不能核对入口名字")
			return goValue{}
		}
		if b.evaluated {
			return b.value
		}
		b.resolving = true
		resolved := b.file.value(b.expression, depth+1)
		b.resolving = false
		if value.typeName == "" {
			value.typeName = resolved.typeName
		}
		value.text, value.constant, value.prefix, value.prefixUnknown = resolved.text, resolved.constant, resolved.prefix, resolved.prefixUnknown
		b.value, b.evaluated = value, true
	}
	return value
}

func (g *goFile) typeName(n *ts.Node) string {
	if n == nil {
		return ""
	}
	switch g.kind(n) {
	case "pointer_type", "generic_type":
		if inner := g.field(n, "type"); inner != nil {
			return g.typeName(inner)
		}
		if n.NamedChildCount() > 0 {
			return g.typeName(n.NamedChild(0))
		}
	case "qualified_type":
		p := g.text(g.field(n, "package"))
		name := g.text(g.field(n, "name"))
		if p == "" && n.NamedChildCount() > 1 {
			p = g.text(n.NamedChild(0))
			name = g.text(n.NamedChild(1))
		}
		if imported := g.imports[p]; imported != "" {
			return imported + "." + name
		}
	case "type_identifier":
		return "local:" + g.text(n)
	}
	return ""
}

func (g *goFile) value(n *ts.Node, depth int) goValue {
	if n == nil || depth > 30 || g.ctx.Err() != nil {
		return goValue{}
	}
	switch g.kind(n) {
	case "interpreted_string_literal", "raw_string_literal":
		v, err := strconv.Unquote(g.text(n))
		return goValue{text: v, constant: err == nil}
	case "identifier":
		if b, ok := g.lookup(g.text(n), n.StartByte()); ok {
			return g.bindingValue(b, depth+1)
		}
	case "parenthesized_expression", "unary_expression", "literal_element":
		if n.NamedChildCount() == 1 {
			return g.value(n.NamedChild(0), depth+1)
		}
	case "binary_expression":
		if g.text(g.field(n, "operator")) == "+" {
			a, b := g.value(g.field(n, "left"), depth+1), g.value(g.field(n, "right"), depth+1)
			if a.constant && b.constant {
				return goValue{text: a.text + b.text, constant: true}
			}
		}
	case "composite_literal":
		return goValue{typeName: g.typeName(g.field(n, "type"))}
	case "call_expression":
		name, receiver := g.target(g.field(n, "function"), depth+1)
		if t := golang.Rules.Constructors[name]; t != "" {
			return goValue{typeName: t}
		}
		if t := golang.Rules.ReceiverMethods[name]; t != "" {
			value := goValue{typeName: t, prefix: receiver.prefix, prefixUnknown: receiver.prefixUnknown}
			if strings.HasSuffix(name, ".Group") {
				args := g.arguments(n)
				if len(args) > 0 {
					prefix := g.value(args[0], depth+1)
					value.prefixUnknown = value.prefixUnknown || !prefix.constant
					value.prefix = strings.TrimSuffix(value.prefix, "/") + prefix.text
				} else {
					value.prefixUnknown = true
				}
			}
			return value
		}
	}
	return goValue{}
}

func (g *goFile) target(n *ts.Node, depth int) (string, goValue) {
	if n == nil || depth > 30 || g.ctx.Err() != nil {
		return "", goValue{}
	}
	if g.kind(n) == "selector_expression" {
		operand := g.field(n, "operand")
		member := g.text(g.field(n, "field"))
		if g.kind(operand) == "identifier" {
			if _, bound := g.lookup(g.text(operand), operand.StartByte()); !bound {
				if imported := g.imports[g.text(operand)]; imported != "" {
					return imported + "." + member, goValue{}
				}
			}
		}
		value := g.value(operand, depth+1)
		if value.typeName != "" {
			return value.typeName + "." + member, value
		}
	}
	return "", goValue{}
}

func (g *goFile) arguments(n *ts.Node) []*ts.Node {
	args := g.field(n, "arguments")
	var result []*ts.Node
	if args != nil {
		for i := 0; i < args.NamedChildCount(); i++ {
			child := args.NamedChild(i)
			if !strings.Contains(g.kind(child), "comment") {
				result = append(result, child)
			}
		}
	}
	return result
}

func (g *goFile) bindSpec(n *ts.Node, global bool) {
	var names []*ts.Node
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if g.kind(child) == "identifier" {
			names = append(names, child)
		}
	}
	values := g.field(n, "value")
	kind := g.typeName(g.field(n, "type"))
	for i, name := range names {
		b := &goBinding{name: g.text(name), scope: g.scope(n), available: n.EndByte(), value: goValue{typeName: kind}, file: g}
		if values != nil && values.NamedChildCount() == len(names) {
			b.expression = values.NamedChild(i)
			b.initializer = b.expression
		}
		if g.kind(n) != "const_spec" {
			if b.expression != nil {
				v := g.value(b.expression, 0)
				if b.value.typeName == "" {
					b.value.typeName = v.typeName
				}
				b.value.prefix, b.value.prefixUnknown = v.prefix, v.prefixUnknown
			}
			b.expression = nil
		}
		if global {
			b.available = 0
			g.globals[b.name] = append(g.globals[b.name], b)
		} else {
			g.addBinding(b)
		}
	}
}

func (g *goFile) bindParameters(n *ts.Node) {
	function := g.enclosingCallable(n)
	if function == nil {
		return
	}
	// 函数类型中的参数属于签名，不能遮蔽外层函数里同名的导入。
	for p := n.Parent(); p != nil && p != function; p = p.Parent() {
		if g.kind(p) == "function_type" {
			return
		}
	}
	kind := g.typeName(g.field(n, "type"))
	value := goValue{typeName: kind}
	if strings.HasSuffix(kind, ".Router") || strings.HasSuffix(kind, ".RouterGroup") || strings.HasSuffix(kind, ".Group") || strings.HasSuffix(kind, ".IRouter") || strings.HasSuffix(kind, ".IRoutes") {
		value.prefixUnknown = true
		if parent := function.Parent(); parent != nil && g.kind(parent) == "argument_list" && parent.Parent() != nil {
			call := parent.Parent()
			target, receiver := g.target(g.field(call, "function"), 0)
			if strings.HasPrefix(target, "github.com/go-chi/chi") && (strings.HasSuffix(target, ".Route") || strings.HasSuffix(target, ".Group")) {
				value.prefix, value.prefixUnknown = receiver.prefix, receiver.prefixUnknown
				if strings.HasSuffix(target, ".Route") {
					args := g.arguments(call)
					if len(args) > 0 {
						prefix := g.value(args[0], 0)
						value.prefixUnknown = value.prefixUnknown || !prefix.constant
						value.prefix = strings.TrimSuffix(value.prefix, "/") + prefix.text
					}
				}
			}
		}
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		name := n.NamedChild(i)
		if g.kind(name) == "identifier" {
			g.addBinding(&goBinding{name: g.text(name), scope: span(function), available: function.StartByte(), value: value, file: g})
		}
	}
}

func (g *goFile) bindAssignment(n *ts.Node) {
	left, right := g.field(n, "left"), g.field(n, "right")
	if left == nil {
		return
	}
	for i := 0; i < left.NamedChildCount(); i++ {
		name := left.NamedChild(i)
		if g.kind(name) != "identifier" || g.text(name) == "_" {
			continue
		}
		v := goValue{}
		var initializer *ts.Node
		if right != nil {
			if right.NamedChildCount() == left.NamedChildCount() {
				v = g.value(right.NamedChild(i), 0)
				initializer = right.NamedChild(i)
			} else if i == 0 && right.NamedChildCount() == 1 {
				v = g.value(right.NamedChild(0), 0)
			}
		}
		previous, exists := g.lookup(g.text(name), n.StartByte())
		if exists && previous != nil && v.typeName == "" {
			v.typeName = previous.value.typeName
		}
		scope := g.scope(n)
		if g.kind(n) == "short_var_declaration" && previous != nil {
			if function := g.enclosingCallable(n); function != nil {
				body := g.field(function, "body")
				if body != nil && scope == span(body) && previous.scope == span(function) {
					scope = previous.scope
				}
			}
		}
		if (g.kind(n) == "assignment_statement" || (g.kind(n) == "range_clause" && goAssignmentOperator(g, n) == "=")) && previous != nil {
			scope = previous.scope
			v.prefixUnknown = true
		}
		g.addBinding(&goBinding{name: g.text(name), scope: scope, available: n.EndByte(), value: goValue{typeName: v.typeName, prefix: v.prefix, prefixUnknown: v.prefixUnknown}, initializer: initializer, file: g})
	}
}

func goAssignmentOperator(g *goFile, n *ts.Node) string {
	if operator := g.field(n, "operator"); operator != nil {
		return g.text(operator)
	}
	for i := 0; i < n.ChildCount(); i++ {
		if text := g.text(n.Child(i)); text == ":=" || text == "=" {
			return text
		}
	}
	return ""
}

func (g *goFile) owner(n *ts.Node) string {
	var found *Declaration
	for i := range g.file.Declarations {
		d := &g.file.Declarations[i]
		if d.Span.Start <= n.StartByte() && d.Span.End >= n.EndByte() && (found == nil || d.Span.End-d.Span.Start < found.Span.End-found.Span.Start) {
			found = d
		}
	}
	if found != nil {
		return found.ID
	}
	return g.file.Path
}

func (g *goFile) missing(operation string, n *ts.Node, reason string) {
	key := fmt.Sprintf("%s:%d:%s", operation, n.StartByte(), reason)
	if g.unknown[key] {
		return
	}
	g.unknown[key] = true
	g.file.Missing = append(g.file.Missing, Missing{Path: g.file.Path, Operation: operation, Reason: fmt.Sprintf("第 %d 行：%s", n.StartPoint().Row+1, reason)})
}

func (g *goFile) entry(n *ts.Node, kind, name string) {
	if name == "" {
		g.missing("entries", n, "入口名字为空，尚不能定位")
		return
	}
	for _, e := range g.file.Entries {
		if e.Span.Start == n.StartByte() && e.Kind == kind && e.Name == name {
			return
		}
	}
	g.file.Entries = append(g.file.Entries, Entry{Kind: kind, Name: name, Declaration: g.owner(n), Span: span(n), Evidence: g.text(n)})
}

func (g *goFile) route(n *ts.Node, target string, args []*ts.Node, name string) {
	method := target[strings.LastIndex(target, ".")+1:]
	var methods []string
	switch strings.ToUpper(method) {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT", "TRACE":
		methods = []string{strings.ToUpper(method)}
	case "METHOD", "METHODFUNC", "ADD", "MATCH", "HANDLE":
		if strings.HasPrefix(target, "net/http.") || strings.HasSuffix(target, ".Router.Handle") {
			methods = []string{""}
			break
		}
		if len(args) > 0 {
			if method == "Match" {
				values, ok := g.stringList(args[0])
				if ok {
					methods = values
				}
			} else {
				value := g.value(args[0], 0)
				if value.constant {
					methods = []string{value.text}
				}
			}
		}
	default:
		methods = []string{""}
	}
	if len(methods) == 0 {
		g.missing("entries", n, "路由方法不能静态核对")
		return
	}
	if strings.HasPrefix(target, "net/http.") {
		if first, rest, ok := strings.Cut(name, " "); ok {
			methods = []string{first}
			name = strings.TrimSpace(rest)
		}
	}
	for _, method := range methods {
		duplicate := false
		for _, entry := range g.file.Entries {
			if entry.Span.Start == n.StartByte() && entry.Kind == "route" && entry.Name == name && entry.Method == method {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		g.file.Entries = append(g.file.Entries, Entry{Kind: "route", Name: name, Method: method, Declaration: g.owner(n), Span: span(n), Evidence: g.text(n)})
	}
}

func (g *goFile) stringList(n *ts.Node) ([]string, bool) {
	if n == nil {
		return nil, false
	}
	if g.kind(n) == "literal_element" && n.NamedChildCount() == 1 {
		return g.stringList(n.NamedChild(0))
	}
	if g.kind(n) == "composite_literal" {
		return g.stringList(g.field(n, "body"))
	}
	if g.kind(n) != "literal_value" {
		return nil, false
	}
	var values []string
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if strings.Contains(g.kind(child), "comment") {
			continue
		}
		value := g.value(child, 0)
		if !value.constant {
			return nil, false
		}
		values = append(values, value.text)
	}
	return values, true
}

func (g *goFile) flag(n *ts.Node, name string, short bool) {
	if name == "" {
		g.missing("entries", n, "参数名字为空，尚不能定位")
		return
	}
	if !strings.HasPrefix(name, "-") {
		if short {
			name = "-" + name
		} else {
			name = "--" + name
		}
	}
	g.entry(n, "flag", name)
}

func (g *goFile) risk(n *ts.Node, kind string) {
	for _, r := range g.file.Risks {
		if r.Span.Start == n.StartByte() && r.Kind == kind {
			return
		}
	}
	g.file.Risks = append(g.file.Risks, Risk{Kind: kind, Declaration: g.owner(n), Span: span(n), Evidence: g.text(n)})
}

func matchesAPI(rule, target string) bool {
	if strings.HasSuffix(rule, "*") {
		return strings.HasPrefix(target, strings.TrimSuffix(rule, "*"))
	}
	return rule == target
}

func (g *goFile) inspect(n *ts.Node) {
	for _, kind := range golang.Rules.ConcurrentNodes {
		if g.kind(n) == kind {
			g.risk(n, "concurrency")
		}
	}
	if g.kind(n) == "unary_expression" && g.text(g.field(n, "operator")) == "<-" {
		g.risk(n, "concurrency")
	}
	if g.kind(n) == "selector_expression" || g.kind(n) == "qualified_type" {
		name, _ := g.target(n, 0)
		if name == "" && g.kind(n) == "qualified_type" {
			name = g.typeName(n)
		}
		for _, prefix := range golang.Rules.ConcurrentImports {
			if strings.HasPrefix(name, prefix+".") {
				g.risk(n, "concurrency")
			}
		}
		if strings.HasPrefix(name, "os/exec.") {
			g.risk(n, "command")
		}
		if g.kind(n) == "selector_expression" {
			parent := n.Parent()
			direct := parent != nil && g.kind(parent) == "call_expression" && g.field(parent, "function") == n
			if !direct {
				for _, rule := range golang.Rules.Entries {
					if rule.Target == name {
						g.missing("entries", n, "入口定义函数被间接传递，实际调用尚待核对")
						break
					}
				}
				for _, rule := range golang.Rules.Risks {
					if matchesAPI(rule.Target, name) {
						g.missing("risks", n, "风险函数被间接传递，实际调用尚待核对")
						break
					}
				}
			}
		}
	}
	switch g.kind(n) {
	case "call_expression":
		function := g.field(n, "function")
		target, receiver := g.target(function, 0)
		args := g.arguments(n)
		if g.kind(function) == "identifier" {
			if _, shadowed := g.lookup(g.text(function), n.StartByte()); !shadowed {
				if g.text(function) == "close" || (g.text(function) == "make" && len(args) > 0 && g.kind(args[0]) == "channel_type") {
					g.risk(n, "concurrency")
				}
			}
		}
		for _, rule := range golang.Rules.Entries {
			if target != rule.Target {
				continue
			}
			if rule.Arg >= len(args) {
				g.missing("entries", n, "入口定义缺少名字参数")
				continue
			}
			value := g.value(args[rule.Arg], 0)
			if !value.constant {
				g.missing("entries", n, "入口名字不能静态解析为字符串常量")
				continue
			}
			if rule.Kind == "route" {
				if receiver.prefixUnknown {
					g.missing("entries", n, "路由组前缀不能静态核对")
					continue
				}
				value.text = strings.TrimSuffix(receiver.prefix, "/") + value.text
			}
			switch rule.Kind {
			case "route":
				g.route(n, target, args, value.text)
			case "flag":
				g.flag(n, value.text, false)
				if strings.HasPrefix(target, "github.com/spf13/pflag.") && strings.HasSuffix(target, "P") && rule.Arg+1 < len(args) {
					short := g.value(args[rule.Arg+1], 0)
					if !short.constant {
						g.missing("entries", n, "短参数名不能静态核对")
					} else if short.text != "" {
						g.flag(n, short.text, true)
					}
				}
			default:
				g.entry(n, rule.Kind, value.text)
			}
		}
		for _, rule := range golang.Rules.Risks {
			if matchesAPI(rule.Target, target) {
				g.risk(n, rule.Kind)
			}
		}
		if target == "" && g.kind(function) == "selector_expression" {
			method := g.text(g.field(function, "field"))
			for _, rule := range golang.Rules.Entries {
				if strings.HasSuffix(rule.Target, "."+method) {
					g.missing("entries", n, "调用 "+method+" 的接收者类型不能核对")
					break
				}
			}
			for _, rule := range golang.Rules.Risks {
				if strings.HasSuffix(rule.Target, "."+method) {
					g.missing("risks", n, "调用 "+method+" 的接收者类型不能核对")
					break
				}
			}
		}
	case "composite_literal":
		g.literalEntries(n)
	case "field_declaration":
		tag := g.field(n, "tag")
		value, err := strconv.Unquote(g.text(tag))
		if err != nil {
			return
		}
		for _, key := range golang.Rules.ConfigTags {
			field, ok := reflect.StructTag(value).Lookup(key)
			if !ok {
				continue
			}
			name := strings.Split(field, ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				g.missing("entries", n, "配置标签没有显式名字，默认名或内联映射尚待核对")
			} else {
				g.entry(n, "config", name)
			}
		}
	}
}

func (g *goFile) literalEntries(n *ts.Node) {
	typeName := g.typeName(g.field(n, "type"))
	kind, key := "", ""
	for _, name := range golang.Rules.FlagTypes {
		if typeName == name {
			kind, key = "flag", "Name"
		}
	}
	for _, name := range golang.Rules.CommandTypes {
		if typeName == name {
			kind, key = "cmd", "Name"
			if name == "github.com/spf13/cobra.Command" {
				key = "Use"
			}
		}
	}
	if kind == "" {
		return
	}
	body := g.field(n, "body")
	if body == nil {
		for i := 0; i < n.NamedChildCount(); i++ {
			if g.kind(n.NamedChild(i)) == "literal_value" {
				body = n.NamedChild(i)
			}
		}
	}
	if body == nil {
		return
	}
	for i := 0; i < body.NamedChildCount(); i++ {
		item := body.NamedChild(i)
		if g.kind(item) != "keyed_element" || item.NamedChildCount() < 2 {
			continue
		}
		name := g.text(item.NamedChild(0))
		if name != key && name != "Aliases" {
			continue
		}
		if name == "Aliases" {
			values, ok := g.stringList(item.NamedChild(1))
			if !ok {
				g.missing("entries", item, "入口别名不能静态核对")
				continue
			}
			for _, value := range values {
				if kind == "flag" {
					g.flag(item, value, len(value) == 1)
				} else {
					g.entry(item, kind, value)
				}
			}
			continue
		}
		value := g.value(item.NamedChild(1), 0)
		if !value.constant {
			g.missing("entries", item, "入口字面量的名字不能静态核对")
			continue
		}
		if key == "Use" {
			parts := strings.Fields(value.text)
			if len(parts) > 0 {
				value.text = parts[0]
			}
		}
		for _, alias := range strings.Split(value.text, ",") {
			if kind == "flag" {
				value := strings.TrimSpace(alias)
				g.flag(item, value, len(value) == 1)
			} else {
				g.entry(item, kind, strings.TrimSpace(alias))
			}
		}
	}
}
