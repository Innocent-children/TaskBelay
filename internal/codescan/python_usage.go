package codescan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/python"
	ts "github.com/odvcencio/gotreesitter"
)

type pyFieldLink struct {
	item, method *pyDeclaration
	field        string
	parameter    int
	node         *ts.Node
}
type pyField struct {
	item  *pyDeclaration
	index int
}

type pyFieldWrite struct {
	item     *pyDeclaration
	field    string
	file     *pyFile
	node     *ts.Node
	implicit bool
}

func (a *pyAnalysis) declare(item *pyDeclaration) {
	p, d, n := item.p, item.d, item.n
	usage := d.Usage
	usage.Public = a.public && !strings.HasPrefix(d.Name, "_") && (d.Owner == "" || p.kind(n) == "function_definition")
	for _, owner := range strings.Split(d.Owner, ".") {
		if strings.HasPrefix(owner, "_") {
			usage.Public = false
		}
	}
	if a.packageUnknown != "" {
		a.unknown(item, nil, p, n, a.packageUnknown)
	}
	if p.kind(n) == "assignment" && d.Owner == "" && p.kind(p.field(n, "right")) == "identifier" {
		usage.Alias = p.text(p.field(n, "right"))
	}
	if d.Owner != "" && p.kind(n) == "function_definition" && strings.HasPrefix(d.Name, "__") && strings.HasSuffix(d.Name, "__") {
		usage.Implicit = append(usage.Implicit, "特殊方法："+d.Name)
	}
	if parent := n.Parent(); parent != nil && p.kind(parent) == "decorated_definition" {
		for i := 0; i < parent.NamedChildCount(); i++ {
			decorator := parent.NamedChild(i)
			if p.kind(decorator) != "decorator" || decorator.NamedChildCount() == 0 {
				continue
			}
			expression := decorator.NamedChild(0)
			callee := expression
			if p.kind(expression) == "call" {
				callee = p.field(expression, "function")
			}
			name := p.reference(callee, 0).qualified
			item.staticmethod = item.staticmethod || name == "builtins.staticmethod"
			item.classmethod = item.classmethod || name == "builtins.classmethod"
			ignored := false
			for _, rule := range python.Rules.IgnoredDecorators {
				if matchesAPI(rule, name) {
					ignored = true
					break
				}
			}
			if !ignored && p.kind(n) == "function_definition" {
				usage.Implicit = append(usage.Implicit, "装饰器："+p.text(callee))
			}
			if slices.Contains(python.Rules.AbstractDecorators, name) {
				usage.Abstract = true
			}
			if p.text(callee) == "deprecated" || strings.HasSuffix(name, ".deprecated") {
				usage.Deprecated = true
			}
			if name == "click.group" {
				for _, b := range p.bindings[p.key(p.scope(n))][d.Name] {
					if b.declaration == item {
						b.typ = "click.Group"
					}
				}
			}
		}
	}
	if p.kind(n) == "class_definition" {
		if bases := p.field(n, "superclasses"); bases != nil {
			for i := 0; i < bases.NamedChildCount(); i++ {
				base := bases.NamedChild(i)
				if p.kind(base) == "keyword_argument" || p.kind(base) == "comment" {
					continue
				}
				name := p.typeName(base, 0)
				if name == "" {
					a.unknown(item, nil, p, base, "基类名字或动态继承关系无法核对")
					continue
				}
				item.bases = append(item.bases, name)
				usage.Abstract = usage.Abstract || slices.Contains(python.Rules.AbstractBases, name)
				item.protocol = item.protocol || slices.Contains(python.Rules.StructuralBases, name)
				item.external = item.external || slices.Contains(python.Rules.ExternalFieldBases, name)
				item.config = item.config || slices.Contains(python.Rules.ConfigBases, name)
				usage.Error = usage.Error || (strings.HasPrefix(name, "builtins.") && slices.Contains(python.Rules.ErrorBases, strings.TrimPrefix(name, "builtins.")))
			}
		}
	}
	if p.kind(n) == "function_definition" {
		a.parameters(item)
	}
}

func (a *pyAnalysis) ownMethods(item *pyDeclaration) []*pyDeclaration {
	var methods []*pyDeclaration
	qualified := strings.TrimPrefix(item.d.Location(), item.d.File+":")
	for _, candidate := range a.declarations {
		if candidate.p == item.p && candidate.d.Owner == qualified && candidate.p.kind(candidate.n) == "function_definition" {
			methods = append(methods, candidate)
		}
	}
	return methods
}

func (a *pyAnalysis) link(item *pyDeclaration, visiting map[string]bool) {
	if item.linked || item.p.kind(item.n) != "class_definition" || a.ctx.Err() != nil {
		return
	}
	if visiting[item.d.ID] {
		a.unknown(item, nil, item.p, item.n, "继承关系形成循环")
		return
	}
	visiting[item.d.ID] = true
	defer delete(visiting, item.d.ID)
	abstract := map[string]bool{}
	for _, name := range item.bases {
		parents := a.byName[name]
		if len(parents) == 1 && parents[0].p.kind(parents[0].n) == "class_definition" {
			parent := parents[0]
			a.link(parent, visiting)
			item.parents = append(item.parents, parent)
			item.d.Usage.Methods = append(item.d.Usage.Methods, parent.d.Usage.Methods...)
			for _, method := range parent.abstractMethods {
				abstract[method] = true
			}
			item.d.Usage.Error = item.d.Usage.Error || parent.d.Usage.Error
			item.external = item.external || parent.external
			item.config = item.config || parent.config
			if len(parent.d.Usage.Unknown) > 0 {
				a.unknown(item, nil, item.p, item.n, "父类的继承关系尚未完整核对："+parent.d.Location())
			}
		} else if name != "builtins.object" && !slices.Contains(python.Rules.AbstractBases, name) && !slices.Contains(python.Rules.ExternalFieldBases, name) && !(strings.HasPrefix(name, "builtins.") && slices.Contains(python.Rules.ErrorBases, strings.TrimPrefix(name, "builtins."))) {
			a.unknown(item, nil, item.p, item.n, "外部或同名基类的完整关系无法核对："+name)
		}
	}
	for _, method := range a.ownMethods(item) {
		item.d.Usage.Methods = append(item.d.Usage.Methods, method.d.Name)
		if method.d.Usage.Abstract {
			abstract[method.d.Name] = true
		} else {
			delete(abstract, method.d.Name)
		}
	}
	for name := range abstract {
		item.abstractMethods = append(item.abstractMethods, name)
	}
	if len(abstract) > 0 {
		item.d.Usage.Abstract = true
	}
	sort.Strings(item.d.Usage.Methods)
	item.d.Usage.Methods = slices.Compact(item.d.Usage.Methods)
	item.linked = true
}

func pyInherits(item, target *pyDeclaration, seen map[string]bool) bool {
	if seen[item.d.ID] {
		return false
	}
	seen[item.d.ID] = true
	for _, parent := range item.parents {
		if parent == target || pyInherits(parent, target, seen) {
			return true
		}
	}
	return false
}

func (a *pyAnalysis) implementations() {
	for _, target := range a.declarations {
		if target.p.kind(target.n) != "class_definition" || !target.d.Usage.Abstract {
			continue
		}
		for _, candidate := range a.declarations {
			if candidate.p.kind(candidate.n) != "class_definition" || candidate.d.Usage.Abstract {
				continue
			}
			matches := pyInherits(candidate, target, map[string]bool{})
			if target.protocol {
				matches = true
				for _, method := range target.d.Usage.Methods {
					if !slices.Contains(candidate.d.Usage.Methods, method) {
						matches = false
						break
					}
				}
			}
			if !matches {
				continue
			}
			if len(candidate.d.Usage.Unknown) > 0 {
				a.unknown(target, nil, candidate.p, candidate.n, "候选实现的继承关系尚未完整核对："+candidate.d.Location())
				continue
			}
			addUse(&target.d.Usage.Implementations, candidate.p.use(candidate.n, candidate.d.ID))
			for _, method := range a.ownMethods(candidate) {
				if slices.Contains(target.d.Usage.Methods, method.d.Name) {
					for i := range method.d.Usage.Parameters {
						method.d.Usage.Parameters[i].Interface = true
					}
				}
			}
		}
	}
}

func (a *pyAnalysis) parameters(item *pyDeclaration) {
	p := item.p
	parameters := p.field(item.n, "parameters")
	if parameters == nil {
		return
	}
	keywordOnly := false
	for i := 0; i < parameters.NamedChildCount(); i++ {
		n := parameters.NamedChild(i)
		if p.kind(n) == "keyword_separator" {
			keywordOnly = true
			continue
		}
		if p.kind(n) == "positional_separator" {
			for j := range item.parameters {
				item.parameters[j].positionalOnly = true
			}
			continue
		}
		name := pyParameterName(p, n)
		if name == nil {
			continue
		}
		if i == 0 && item.d.Owner != "" && !item.staticmethod && (p.text(name) == "self" || p.text(name) == "cls") {
			item.self = p.text(name)
			for _, b := range p.bindings[p.key(item.n)][item.self] {
				b.typ = item.d.File + ":" + item.d.Owner
			}
			continue
		}
		slot := Slot{Name: p.text(name), Type: p.text(p.field(n, "type")), Span: span(name), Public: item.d.Usage.Public, Interface: item.d.Usage.Abstract}
		typ := p.typeName(p.field(n, "type"), 0)
		slot.Function = typ == "typing.Callable" || typ == "collections.abc.Callable"
		rest := ""
		if p.kind(n) == "list_splat_pattern" || strings.HasPrefix(p.text(n), "*") {
			rest = "positional"
			slot.Variadic = true
			keywordOnly = true
		}
		if p.kind(n) == "dictionary_splat_pattern" || strings.HasPrefix(p.text(n), "**") {
			rest = "keyword"
			slot.Variadic = true
			keywordOnly = true
		}
		parameter := pyParameter{index: len(item.d.Usage.Parameters), name: slot.Name, defaultValue: p.field(n, "value"), keywordOnly: keywordOnly, rest: rest}
		item.d.Usage.Parameters = append(item.d.Usage.Parameters, slot)
		item.parameters = append(item.parameters, parameter)
	}
}

func (a *pyAnalysis) fields(item *pyDeclaration) {
	p := item.p
	if p.kind(item.n) != "class_definition" {
		return
	}
	_ = walkSyntax(a.ctx, item.n, func(n *ts.Node) {
		if p.kind(n) != "assignment" && p.kind(n) != "augmented_assignment" {
			return
		}
		for parent := n.Parent(); parent != nil && parent != item.n; parent = parent.Parent() {
			if p.kind(parent) == "class_definition" {
				return
			}
		}
		left, right := p.field(n, "left"), p.field(n, "right")
		var names []*ts.Node
		if p.scope(n) == item.n {
			names = bindingNames(left, p.syntax.language)
		} else if p.kind(left) == "attribute" && p.text(p.field(left, "object")) == "self" {
			if p.reference(p.field(left, "object"), 0).typ == item.d.ID {
				names = []*ts.Node{p.field(left, "attribute")}
			} else {
				a.unknown(item, nil, p, left, "self 的绑定已变化，字段归属不能核对")
			}
		}
		for _, name := range names {
			if name == nil {
				continue
			}
			index := -1
			for i := range item.d.Usage.Fields {
				if item.d.Usage.Fields[i].Name == p.text(name) {
					index = i
					break
				}
			}
			if index < 0 {
				typ := p.typeName(p.field(n, "type"), 0)
				item.d.Usage.Fields = append(item.d.Usage.Fields, Slot{Name: p.text(name), Type: p.text(p.field(n, "type")), Span: span(name), External: item.external, Public: item.d.Usage.Public, Function: typ == "typing.Callable" || typ == "collections.abc.Callable"})
				index = len(item.d.Usage.Fields) - 1
				if item.config {
					p.entry(n, "config", p.text(name), "")
				}
			}
			slot := &item.d.Usage.Fields[index]
			if right == nil {
				continue
			}
			slot.Written = true
			if p.kind(n) == "augmented_assignment" {
				slot.Read = true
				a.unknown(item, slot, p, n, "复合赋值依赖先前值")
				continue
			}
			if len(names) > 1 {
				a.unknown(item, slot, p, n, "字段的解构赋值尚不能逐项核对")
				continue
			}
			a.fieldValue(item, slot, p, right, p.scope(n) == item.n)
		}
	})
}

func (a *pyAnalysis) fieldValue(item *pyDeclaration, slot *Slot, p *pyFile, n *ts.Node, implicit bool) {
	if p.kind(n) == "identifier" {
		if method, index := a.parameterAt(p, n); method != nil {
			for _, link := range a.fieldLinks {
				if link.item == item && link.field == slot.Name && link.method == method && link.parameter == index && span(link.node) == span(n) {
					return
				}
			}
			a.fieldLinks = append(a.fieldLinks, pyFieldLink{item, method, slot.Name, index, n})
			return
		}
	}
	a.fieldWrites = append(a.fieldWrites, pyFieldWrite{item, slot.Name, p, n, implicit})
}

func (a *pyAnalysis) assign(item *pyDeclaration, slot *Slot, p *pyFile, n *ts.Node, implicit bool) {
	if n == nil {
		return
	}
	if slot.Function {
		if p.kind(n) == "lambda" {
			addUse(&slot.Functions, p.use(n, fmt.Sprintf("%s:lambda@%d", p.file.Path, n.StartByte())))
			return
		}
		if targets := a.byName[p.reference(n, 0).qualified]; len(targets) == 1 && targets[0].p.kind(targets[0].n) == "function_definition" {
			addUse(&slot.Functions, p.use(n, targets[0].d.ID))
			return
		}
		if p.kind(n) != "none" {
			a.unknown(item, slot, p, n, "函数值无法唯一对应到具名函数或匿名函数")
			return
		}
	}
	value, ok := p.literal(n, 0)
	if !ok {
		a.unknown(item, slot, p, n, "值不能解析为字面量或顶层常量")
		return
	}
	value.At = p.use(n, slot.Name)
	value.Implicit = implicit
	for _, old := range slot.Values {
		if old.Kind == value.Kind && old.Text == value.Text && old.At.File == value.At.File && old.At.Span == value.At.Span && old.Implicit == value.Implicit {
			return
		}
	}
	slot.Values = append(slot.Values, value)
}

func (a *pyAnalysis) parameterAt(p *pyFile, n *ts.Node) (*pyDeclaration, int) {
	for node := n; node.Parent() != nil; node = node.Parent() {
		if p.kind(node.Parent()) == "parameters" || p.kind(node.Parent()) == "lambda_parameters" {
			if name := pyParameterName(p, node); name != nil && span(name) == span(n) {
				return nil, 0
			}
			break
		}
		if pyScopeKind(p.kind(node.Parent())) {
			break
		}
	}
	for node := n.Parent(); node != nil; node = node.Parent() {
		if strings.HasSuffix(p.kind(node), "comprehension") || p.kind(node) == "generator_expression" {
			if p.outerComprehensionIterable(node, n) {
				continue
			}
			if len(p.bindings[p.key(node)][p.text(n)]) > 0 {
				return nil, 0
			}
		}
		if p.kind(node) != "function_definition" && p.kind(node) != "lambda" {
			continue
		}
		if body := p.field(node, "body"); body != nil && n.StartByte() < body.StartByte() {
			continue
		}
		if len(p.bindings[p.key(node)][p.text(n)]) == 0 {
			continue
		}
		if item := p.byNode[p.key(node)]; item != nil {
			for i, slot := range item.d.Usage.Parameters {
				if slot.Name == p.text(n) {
					return item, i
				}
			}
		}
		return nil, 0
	}
	return nil, 0
}

func (a *pyAnalysis) matchingFields(owner, name string, seen map[string]bool) []pyField {
	if seen[owner] || a.ctx.Err() != nil {
		return nil
	}
	seen[owner] = true
	defer delete(seen, owner)
	var result []pyField
	for _, item := range a.byName[owner] {
		found := false
		for i, slot := range item.d.Usage.Fields {
			if slot.Name == name {
				result = append(result, pyField{item, i})
				found = true
			}
		}
		if !found {
			for _, parent := range item.parents {
				result = append(result, a.matchingFields(parent.d.ID, name, seen)...)
			}
		}
	}
	return result
}

func pyAccess(p *pyFile, n *ts.Node) (read, write bool, value *ts.Node, unknown string) {
	for node := n; node.Parent() != nil; node = node.Parent() {
		parent := node.Parent()
		switch p.kind(parent) {
		case "delete_statement":
			return false, true, nil, "字段或参数被删除"
		case "assignment", "augmented_assignment", "named_expression", "for_statement", "for_in_clause":
			left := p.field(parent, "left")
			if p.kind(parent) == "named_expression" {
				left = p.field(parent, "name")
			}
			if left != nil && n.StartByte() >= left.StartByte() && n.EndByte() <= left.EndByte() {
				if p.kind(parent) == "augmented_assignment" {
					return true, true, nil, "复合赋值依赖先前值"
				}
				if p.kind(parent) == "for_statement" || p.kind(parent) == "for_in_clause" {
					return false, true, nil, "循环赋值依赖迭代值"
				}
				if n.StartByte() != left.StartByte() || n.EndByte() != left.EndByte() {
					return true, true, nil, "解构、索引或间接写入无法逐项核对"
				}
				value := p.field(parent, "right")
				if p.kind(parent) == "named_expression" {
					value = p.field(parent, "value")
				}
				return false, true, value, ""
			}
			return true, false, nil, ""
		case "attribute", "subscript", "pattern_list", "tuple_pattern", "list_pattern", "parenthesized_expression":
			continue
		default:
			return true, false, nil, ""
		}
	}
	return true, false, nil, ""
}

func (a *pyAnalysis) readWrite(p *pyFile, n *ts.Node) {
	if p.kind(n) == "identifier" {
		if parent := n.Parent(); parent != nil {
			if name := p.field(parent, "name"); name != nil && p.kind(parent) == "keyword_argument" && name.StartByte() == n.StartByte() {
				return
			}
			if name := p.field(parent, "attribute"); name != nil && name.StartByte() == n.StartByte() {
				return
			}
		}
		if item, index := a.parameterAt(p, n); item != nil {
			slot := &item.d.Usage.Parameters[index]
			if slot.Span.Start == n.StartByte() {
				return
			}
			if parent := n.Parent(); parent != nil && (p.kind(parent) == "function_definition" || p.kind(parent) == "class_definition") {
				if name := p.field(parent, "name"); name != nil && span(name) == span(n) {
					slot.Written = true
					a.unknown(item, slot, p, n, "参数被局部定义重新绑定")
					return
				}
			}
			read, write, value, unknown := pyAccess(p, n)
			slot.Read = slot.Read || read
			slot.Written = slot.Written || write
			if parent := n.Parent(); parent != nil && p.kind(parent) == "call" {
				if fn := p.field(parent, "function"); fn != nil && fn.StartByte() == n.StartByte() {
					slot.Function = true
				}
			}
			if unknown != "" {
				a.unknown(item, slot, p, n, unknown)
			} else if write {
				a.assign(item, slot, p, value, false)
			}
		}
		return
	}
	if p.kind(n) != "attribute" {
		return
	}
	object := p.reference(p.field(n, "object"), 0)
	owner := object.typ
	if owner == "" {
		owner = object.qualified
	}
	name := p.text(p.field(n, "attribute"))
	if name == "__dict__" || name == "__setattr__" {
		a.objectEscape(p, n, owner, map[string]bool{})
	}
	fields := a.matchingFields(owner, name, map[string]bool{})
	if len(fields) == 1 {
		field := fields[0]
		slot := &field.item.d.Usage.Fields[field.index]
		if slot.Span.Start == p.field(n, "attribute").StartByte() && p.file.Path == field.item.d.File {
			return
		}
		read, write, value, unknown := pyAccess(p, n)
		slot.Read = slot.Read || read
		slot.Written = slot.Written || write
		if parent := n.Parent(); parent != nil && p.kind(parent) == "call" {
			if fn := p.field(parent, "function"); fn != nil && fn.StartByte() == n.StartByte() {
				slot.Function = true
			}
		}
		if unknown != "" {
			a.unknown(field.item, slot, p, n, unknown)
		} else if write {
			a.fieldValue(field.item, slot, p, value, false)
		}
	} else if owner == "" {
		for _, item := range a.declarations {
			for i := range item.d.Usage.Fields {
				slot := &item.d.Usage.Fields[i]
				if slot.Name == name {
					a.unknown(item, slot, p, n, "同名字段的接收者或别名无法核对")
				}
			}
		}
	}
}
