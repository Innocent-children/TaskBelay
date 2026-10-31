package codescan

import (
	"context"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/lang/golang"
	ts "github.com/odvcencio/gotreesitter"
)

type goDeclaration struct {
	d        *Declaration
	g        *goFile
	n        *ts.Node
	embedded []string
}

type goUsage struct {
	ctx              context.Context
	declarations     []*goDeclaration
	byName           map[string][]*goDeclaration
	interfaceMethods map[string]bool
	byScope          map[*goFile]map[Span]*goDeclaration
	methodsDone      map[*goDeclaration]bool
}

func analyzeGoUsage(ctx context.Context, files []*goFile) {
	u := &goUsage{ctx: ctx, byName: map[string][]*goDeclaration{}, interfaceMethods: map[string]bool{}, byScope: map[*goFile]map[Span]*goDeclaration{}, methodsDone: map[*goDeclaration]bool{}}
	for _, g := range files {
		u.byScope[g] = map[Span]*goDeclaration{}
		for i := range g.file.Declarations {
			if ctx.Err() != nil {
				g.missing("usage", g.syntax.tree.RootNode(), ctx.Err().Error())
				break
			}
			d := &g.file.Declarations[i]
			d.Usage = &Usage{}
			n := syntaxNodeAt(g.syntax.tree.RootNode(), g.syntax.language, d.NodeType, d.NameSpan)
			if n == nil {
				g.missing("usage", g.syntax.tree.RootNode(), "声明的语法位置无法核对："+d.Location())
				continue
			}
			item := &goDeclaration{d: d, g: g, n: n}
			d.Visibility = Visibility{Scope: golang.VisibilityScope(g.kind(n), d.Owner, d.Name), Package: g.packageName}
			u.declarations = append(u.declarations, item)
			u.byName[d.Name] = append(u.byName[d.Name], item)
			u.byScope[g][span(n)] = item
		}
	}
	for _, item := range u.declarations {
		u.declare(item)
	}
	for _, item := range u.declarations {
		u.methods(item, map[string]bool{})
	}
	for _, item := range u.declarations {
		if item.d.Usage.Abstract {
			for _, name := range item.d.Usage.Methods {
				u.interfaceMethods[name] = true
			}
		}
	}
	for _, item := range u.declarations {
		u.link(item)
	}
	for _, g := range files {
		if err := walkSyntax(ctx, g.syntax.tree.RootNode(), func(n *ts.Node) {
			switch g.kind(n) {
			case "identifier", "selector_expression":
				u.readWrite(g, n)
				u.functionReference(g, n)
			case "composite_literal":
				u.construct(g, n)
			}
		}); err != nil {
			g.missing("usage", g.syntax.tree.RootNode(), err.Error())
		}
	}
	for _, g := range files {
		if err := walkSyntax(ctx, g.syntax.tree.RootNode(), func(n *ts.Node) { u.inspect(g, n) }); err != nil {
			g.missing("usage", g.syntax.tree.RootNode(), err.Error())
		}
	}
	for _, item := range u.declarations {
		usage := item.d.Usage
		sort.Strings(usage.Methods)
		usage.Methods = slices.Compact(usage.Methods)
		sort.Strings(usage.Implicit)
		usage.Implicit = slices.Compact(usage.Implicit)
	}
}

func (g *goFile) packageKey() string { return path.Dir(g.file.Path) + ":" + g.packageName }

func (g *goFile) use(n *ts.Node, value string) Use {
	return Use{File: g.file.Path, Declaration: g.owner(n), Value: value, Span: span(n), Test: g.file.Kind.Test, Candidate: true}
}

func (u *goUsage) unknown(item *goDeclaration, slot *Slot, g *goFile, n *ts.Node, reason string) {
	use := g.use(n, "")
	use.Reason = reason
	if slot != nil {
		addUse(&slot.Unknown, use)
	} else if item != nil {
		addUse(&item.d.Usage.Unknown, use)
	}
	g.missing("usage", n, reason)
	if item != nil && item.g != g {
		item.g.missing("usage", item.n, fmt.Sprintf("%s:%d：%s", g.file.Path, n.StartPoint().Row+1, reason))
	}
}

func (u *goUsage) declare(item *goDeclaration) {
	d, g, n := item.d, item.g, item.n
	usage := d.Usage
	first, _ := utf8.DecodeRuneInString(d.Name)
	usage.Public = g.packageName != "main" && !slices.Contains(strings.Split(d.File, "/"), "internal") && unicode.IsUpper(first)
	for _, marker := range golang.Rules.DeprecatedMarkers {
		if strings.Contains(d.Documentation, marker) {
			usage.Deprecated = true
		}
	}
	for _, directive := range golang.Rules.ImplicitDirectives {
		if strings.Contains(d.Documentation, directive) {
			usage.Implicit = append(usage.Implicit, directive)
		}
	}
	if d.Owner == "" && g.kind(n) == "function_declaration" && slices.Contains(golang.Rules.ImplicitNames, d.Name) {
		usage.Implicit = append(usage.Implicit, d.Name)
	}
	usage.Generic = g.field(n, "type_parameters") != nil
	if parameters := g.field(n, "type_parameters"); parameters != nil {
		for i := 0; i < parameters.NamedChildCount(); i++ {
			parameter := parameters.NamedChild(i)
			for j := 0; j < parameter.NamedChildCount(); j++ {
				name := parameter.NamedChild(j)
				if g.kind(name) == "identifier" {
					usage.TypeParameters = append(usage.TypeParameters, g.text(name))
				}
			}
		}
	}
	if g.kind(n) == "type_alias" {
		usage.Alias = g.text(g.field(n, "type"))
	}
	if g.kind(n) == "var_spec" {
		values := g.field(n, "value")
		if values != nil {
			var names []*ts.Node
			for i := 0; i < n.NamedChildCount(); i++ {
				if child := n.NamedChild(i); g.kind(child) == "identifier" {
					names = append(names, child)
				}
			}
			for i, name := range names {
				if g.text(name) == d.Name && len(names) == values.NamedChildCount() {
					call := values.NamedChild(i)
					if g.kind(call) == "call_expression" {
						target, _ := g.target(g.field(call, "function"), 0)
						usage.Sentinel = slices.Contains(golang.Rules.ErrorConstructors, target)
						usage.Error = usage.Sentinel
					}
				}
			}
		}
	}
	underlying := g.field(n, "type")
	if g.kind(underlying) == "interface_type" {
		usage.Abstract = true
		for i := 0; i < underlying.NamedChildCount(); i++ {
			child := underlying.NamedChild(i)
			if g.kind(child) == "method_elem" {
				usage.Methods = append(usage.Methods, g.text(g.field(child, "name")))
			} else if g.kind(child) == "type_elem" {
				if child.NamedChildCount() == 1 && (g.kind(child.NamedChild(0)) == "type_identifier" || g.kind(child.NamedChild(0)) == "qualified_type") {
					item.embedded = append(item.embedded, g.typeName(child.NamedChild(0)))
				} else {
					u.unknown(item, nil, g, child, "接口的类型约束不能仅按方法名核对实现")
				}
			}
		}
	}
	if g.kind(underlying) == "struct_type" {
		u.declareFields(item, underlying)
	}
	if callable(g.kind(n)) || g.kind(n) == "method_elem" {
		u.declareParameters(item)
	}
}

func (u *goUsage) localTypes(g *goFile, typeName string) []*goDeclaration {
	name := strings.TrimPrefix(typeName, "local:")
	if name == typeName {
		return nil
	}
	var result []*goDeclaration
	for _, item := range u.byName[name] {
		if item.g.packageKey() == g.packageKey() && (item.d.Kind == "type" || item.d.Kind == "interface" || item.d.Kind == "class" || item.d.Kind == "struct") {
			result = append(result, item)
		}
	}
	return result
}

func (u *goUsage) methods(item *goDeclaration, visiting map[string]bool) {
	if u.ctx.Err() != nil || u.methodsDone[item] {
		return
	}
	if item.d.Kind != "type" && item.d.Kind != "interface" && item.d.Kind != "class" && item.d.Kind != "struct" {
		return
	}
	if visiting[item.d.ID] {
		u.unknown(item, nil, item.g, item.n, "嵌入类型形成循环，方法集合无法核对")
		return
	}
	visiting[item.d.ID] = true
	defer delete(visiting, item.d.ID)
	defer func() { u.methodsDone[item] = true }()
	if !item.d.Usage.Abstract {
		for _, method := range u.declarations {
			if method.g.packageKey() == item.g.packageKey() && method.d.Owner == item.d.Name && method.d.Kind == "method" {
				item.d.Usage.Methods = append(item.d.Usage.Methods, method.d.Name)
			}
		}
	}
	for _, embedded := range item.embedded {
		if embedded == "local:error" {
			item.d.Usage.Methods = append(item.d.Usage.Methods, "Error")
			continue
		}
		targets := u.localTypes(item.g, embedded)
		if len(targets) != 1 {
			u.unknown(item, nil, item.g, item.n, "嵌入类型的方法集合无法唯一核对："+embedded)
			continue
		}
		u.methods(targets[0], visiting)
		item.d.Usage.Methods = append(item.d.Usage.Methods, targets[0].d.Usage.Methods...)
		if len(targets[0].d.Usage.Unknown) > 0 {
			u.unknown(item, nil, item.g, item.n, "嵌入类型的方法集合尚不完整："+embedded)
		}
	}
	sort.Strings(item.d.Usage.Methods)
	item.d.Usage.Methods = slices.Compact(item.d.Usage.Methods)
}

func (u *goUsage) link(item *goDeclaration) {
	d, g, usage := item.d, item.g, item.d.Usage
	if d.Owner != "" && d.Kind == "method" {
		if u.interfaceMethods[d.Name] {
			usage.Implicit = append(usage.Implicit, "仓库接口方法："+d.Name)
		}
		if slices.Contains(golang.Rules.ImplicitMethods, d.Name) {
			usage.Implicit = append(usage.Implicit, "标准库接口方法："+d.Name)
		}
		for i := range usage.Parameters {
			usage.Parameters[i].Interface = u.interfaceMethods[d.Name] || slices.Contains(golang.Rules.ImplicitMethods, d.Name)
		}
		if d.Name == "Error" && len(usage.Parameters) == 0 && goStringResult(g, g.field(item.n, "result")) {
			for _, owner := range u.localTypes(g, "local:"+d.Owner) {
				owner.d.Usage.Error = true
			}
		}
	}
	if usage.Abstract {
		for _, candidate := range u.declarations {
			if (candidate.d.Kind != "type" && candidate.d.Kind != "struct") || candidate.d.Usage.Abstract || candidate.g.kind(candidate.n) == "type_alias" {
				continue
			}
			matches := true
			for _, method := range usage.Methods {
				if !slices.Contains(candidate.d.Usage.Methods, method) {
					matches = false
					break
				}
			}
			if matches {
				addUse(&usage.Implementations, candidate.g.use(candidate.n, candidate.d.ID))
			}
		}
	}
}

func goStringResult(g *goFile, n *ts.Node) bool {
	if g.text(n) == "string" {
		return true
	}
	if g.kind(n) == "parameter_list" && n.NamedChildCount() == 1 {
		parameter := n.NamedChild(0)
		count := 0
		for i := 0; i < parameter.NamedChildCount(); i++ {
			if g.kind(parameter.NamedChild(i)) == "identifier" {
				count++
			}
		}
		return count <= 1 && g.text(g.field(parameter, "type")) == "string"
	}
	return false
}

func (u *goUsage) callableTargets(g *goFile, expression *ts.Node) []*goDeclaration {
	name, owner := g.text(expression), ""
	local := true
	if g.kind(expression) == "identifier" || g.kind(expression) == "type_identifier" {
		if b, ok := g.lookup(name, expression.StartByte()); ok && b != nil && b.available != 0 {
			return nil
		}
	} else if g.kind(expression) == "selector_expression" {
		name = g.text(g.field(expression, "field"))
		value := g.value(g.field(expression, "operand"), 0)
		if strings.HasPrefix(value.typeName, "local:") {
			owner = strings.TrimPrefix(value.typeName, "local:")
		} else {
			local = false
		}
	} else {
		return nil
	}
	var result []*goDeclaration
	for _, item := range u.byName[name] {
		if !(callable(item.g.kind(item.n)) || item.g.kind(item.n) == "method_elem") {
			continue
		}
		if local && (item.g.packageKey() != g.packageKey() || item.d.Owner != owner) {
			continue
		}
		result = append(result, item)
	}
	return result
}

func (u *goUsage) inspect(g *goFile, n *ts.Node) {
	switch g.kind(n) {
	case "call_expression", "type_conversion_expression":
		callee, _, typeArguments := goCallParts(g, n)
		targets := u.callableTargets(g, callee)
		for _, target := range targets {
			if len(targets) == 1 && target.g.packageKey() == g.packageKey() {
				use := g.use(n, target.d.ID)
				use.Candidate = false
				addUse(&target.d.Usage.References, use)
			}
			if target.d.Usage.Generic {
				if args := typeArguments; args != nil {
					count := 0
					for i := 0; i < args.NamedChildCount(); i++ {
						if g.kind(args.NamedChild(i)) == "type_elem" {
							count++
						}
					}
					if count == len(target.d.Usage.TypeParameters) {
						addUse(&target.d.Usage.TypeArguments, g.use(n, strings.Join(strings.Fields(g.text(args)), "")))
					} else {
						target.d.Usage.Inferred = true
						u.unknown(target, nil, g, n, "泛型调用只写出部分类型实参，剩余参数依赖推断："+target.d.Location())
					}
				} else {
					target.d.Usage.Inferred = true
					u.unknown(target, nil, g, n, "泛型调用省略类型实参，无法完整计数："+target.d.Location())
				}
			}
			u.callArguments(target, g, n)
		}
		target, _ := g.target(callee, 0)
		if slices.Contains(golang.Rules.ErrorChecks, target) {
			args := g.arguments(n)
			if len(args) > 1 {
				u.errorUse(g, n, args[1])
			}
		}
		u.indirectWrites(g, n)
	case "generic_type":
		if parent := g.enclosingCallable(n); parent != nil {
			if receiver := g.field(parent, "receiver"); receiver != nil && n.StartByte() >= receiver.StartByte() && n.EndByte() <= receiver.EndByte() {
				return
			}
		}
		for _, target := range u.localTypes(g, g.typeName(g.field(n, "type"))) {
			if target.d.Usage.Generic {
				addUse(&target.d.Usage.TypeArguments, g.use(n, strings.Join(strings.Fields(g.text(g.field(n, "type_arguments"))), "")))
			}
		}
	case "index_expression":
		for _, target := range u.callableTargets(g, g.field(n, "operand")) {
			if target.d.Usage.Generic {
				addUse(&target.d.Usage.TypeArguments, g.use(n, "["+strings.Join(strings.Fields(g.text(g.field(n, "index"))), "")+"]"))
				for i := range target.d.Usage.Parameters {
					u.slotUnknown(target, &target.d.Usage.Parameters[i], g, n, "实例化的泛型函数作为值传递，调用实参无法完整核对")
				}
			}
		}
	case "assignment_statement", "var_spec":
		u.aggregateWrite(g, n)
	case "type_assertion_expression":
		u.errorUse(g, n, g.field(n, "type"))
	case "type_case":
		for i := 0; i < n.NamedChildCount(); i++ {
			child := n.NamedChild(i)
			if g.kind(child) == "type_identifier" || g.kind(child) == "pointer_type" || g.kind(child) == "type_list" {
				u.errorUse(g, n, child)
			}
		}
	case "binary_expression":
		if operator := g.text(g.field(n, "operator")); operator == "==" || operator == "!=" {
			u.errorUse(g, n, g.field(n, "left"))
			u.errorUse(g, n, g.field(n, "right"))
		}
	}
}

func goCallParts(g *goFile, n *ts.Node) (callee *ts.Node, args []*ts.Node, typeArguments *ts.Node) {
	if g.kind(n) == "type_conversion_expression" {
		callee = g.field(n, "type")
		if g.kind(callee) == "generic_type" {
			typeArguments = g.field(callee, "type_arguments")
			callee = g.field(callee, "type")
		}
		if operand := g.field(n, "operand"); operand != nil {
			args = []*ts.Node{operand}
		}
		return
	}
	return g.field(n, "function"), g.arguments(n), g.field(n, "type_arguments")
}

func (u *goUsage) errorUse(g *goFile, location, expression *ts.Node) {
	if expression == nil {
		return
	}
	_ = walkSyntax(u.ctx, expression, func(n *ts.Node) {
		if g.kind(n) != "identifier" && g.kind(n) != "type_identifier" {
			return
		}
		name := g.text(n)
		value := g.value(n, 0)
		if binding, ok := g.lookup(name, n.StartByte()); g.kind(n) == "identifier" && ok && binding != nil && binding.available > 0 && !strings.HasPrefix(value.typeName, "local:") {
			return
		}
		if strings.HasPrefix(value.typeName, "local:") {
			name = strings.TrimPrefix(value.typeName, "local:")
		}
		for _, target := range u.byName[name] {
			if target.d.Usage.Error && (target.g.packageKey() == g.packageKey() || g.kind(expression) == "selector_expression") {
				addUse(&target.d.Usage.ErrorChecks, g.use(location, target.d.ID))
			}
		}
	})
}

func (u *goUsage) declareParameters(item *goDeclaration) {
	g, n := item.g, item.n
	parameters := g.field(n, "parameters")
	if parameters == nil {
		return
	}
	for i := 0; i < parameters.NamedChildCount(); i++ {
		parameter := parameters.NamedChild(i)
		if g.kind(parameter) != "parameter_declaration" && g.kind(parameter) != "variadic_parameter_declaration" {
			continue
		}
		typeNode := g.field(parameter, "type")
		var names []*ts.Node
		for j := 0; j < parameter.NamedChildCount(); j++ {
			if child := parameter.NamedChild(j); g.kind(child) == "identifier" {
				names = append(names, child)
			}
		}
		if len(names) == 0 {
			names = []*ts.Node{nil}
		}
		for _, name := range names {
			region := span(parameter)
			if name != nil {
				region = span(name)
			}
			item.d.Usage.Parameters = append(item.d.Usage.Parameters, Slot{Name: g.text(name), Type: g.text(typeNode), Span: region, Function: u.functionType(g, typeNode, map[string]bool{}), Variadic: g.kind(parameter) == "variadic_parameter_declaration", Public: item.d.Usage.Public, Interface: g.kind(n) == "method_elem"})
		}
	}
}

func (u *goUsage) declareFields(item *goDeclaration, structure *ts.Node) {
	g := item.g
	_ = walkSyntax(u.ctx, structure, func(n *ts.Node) {
		if g.kind(n) != "field_declaration" {
			return
		}
		for parent := n.Parent(); parent != nil && parent != structure; parent = parent.Parent() {
			if g.kind(parent) == "struct_type" {
				return
			}
		}
		typeNode := g.field(n, "type")
		var names []*ts.Node
		for i := 0; i < n.NamedChildCount(); i++ {
			if child := n.NamedChild(i); g.kind(child) == "field_identifier" {
				names = append(names, child)
			}
		}
		if len(names) == 0 {
			item.embedded = append(item.embedded, g.typeName(typeNode))
			if typeNode == nil {
				return
			}
			names = []*ts.Node{typeNode}
		}
		for _, name := range names {
			fieldName := g.text(name)
			if name == typeNode {
				fieldName = strings.TrimPrefix(g.typeName(typeNode), "local:")
				if dot := strings.LastIndex(fieldName, "."); dot >= 0 {
					fieldName = fieldName[dot+1:]
				}
			}
			slot := Slot{Name: fieldName, Type: g.text(typeNode), Span: span(name), Function: u.functionType(g, typeNode, map[string]bool{}), Public: item.d.Usage.Public, External: g.field(n, "tag") != nil}
			value, ok := u.zeroValue(g, typeNode, map[string]bool{})
			if name == typeNode && strings.HasPrefix(strings.TrimSpace(g.text(n)), "*") {
				slot.Type = "*" + slot.Type
				value, ok = Value{Kind: "nil", Text: "nil"}, true
			}
			if ok {
				value.Implicit = true
				value.At = g.use(n, slot.Name)
				slot.Values = append(slot.Values, value)
			} else {
				u.unknown(item, &slot, g, n, "字段的隐式零值类型无法核对："+slot.Name)
			}
			item.d.Usage.Fields = append(item.d.Usage.Fields, slot)
		}
	})
}

func (u *goUsage) functionType(g *goFile, n *ts.Node, seen map[string]bool) bool {
	if g.kind(n) == "function_type" {
		return true
	}
	if g.kind(n) != "type_identifier" {
		return false
	}
	key := g.packageKey() + ":" + g.text(n)
	if seen[key] {
		return false
	}
	seen[key] = true
	if targets := u.localTypes(g, g.typeName(n)); len(targets) == 1 {
		return u.functionType(targets[0].g, targets[0].g.field(targets[0].n, "type"), seen)
	}
	return false
}

func (u *goUsage) slotUnknown(item *goDeclaration, slot *Slot, g *goFile, n *ts.Node, reason string) {
	u.unknown(item, slot, g, n, fmt.Sprintf("%s.%s：%s", item.d.Location(), slot.Name, reason))
}

func (u *goUsage) functionReference(g *goFile, n *ts.Node) {
	if parent := n.Parent(); parent != nil {
		if function := g.field(parent, "function"); function != nil && function.StartByte() == n.StartByte() && function.EndByte() == n.EndByte() {
			return
		}
	}
	for _, target := range u.callableTargets(g, n) {
		if target.g == g && target.d.NameSpan.Start == n.StartByte() {
			continue
		}
		for i := range target.d.Usage.Parameters {
			u.slotUnknown(target, &target.d.Usage.Parameters[i], g, n, "函数作为值传递，其全部调用实参无法核对")
		}
	}
}
