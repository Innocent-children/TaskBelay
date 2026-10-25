package codescan

import (
	"fmt"
	"go/constant"
	"go/token"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

func (u *goUsage) zeroValue(g *goFile, n *ts.Node, seen map[string]bool) (Value, bool) {
	switch g.kind(n) {
	case "pointer_type", "slice_type", "map_type", "channel_type", "function_type", "interface_type":
		return Value{Kind: "nil", Text: "nil"}, true
	case "type_identifier":
		name := g.text(n)
		switch name {
		case "bool":
			return Value{Kind: "Bool", Text: "false"}, true
		case "string":
			return Value{Kind: "String", Text: `""`}, true
		case "byte", "rune", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
			return Value{Kind: "Int", Text: "0"}, true
		case "float32", "float64":
			return Value{Kind: "Float", Text: "0"}, true
		case "complex64", "complex128":
			return Value{Kind: "Complex", Text: "0"}, true
		case "any", "error":
			return Value{Kind: "nil", Text: "nil"}, true
		}
		key := g.packageKey() + ":" + name
		if seen[key] {
			return Value{}, false
		}
		seen[key] = true
		defer delete(seen, key)
		if targets := u.localTypes(g, "local:"+name); len(targets) == 1 {
			return u.zeroValue(targets[0].g, targets[0].g.field(targets[0].n, "type"), seen)
		}
	}
	return Value{}, false
}

/**
 * 仅求字面量及可解析常量表达式。无效运算属于源代码的未知值，不能使
 * hook 进程崩溃，也不能把普通变量的某次赋值冒充常量。
 */
func goConstant(g *goFile, n *ts.Node, depth int) (result constant.Value) {
	defer func() {
		if recover() != nil {
			result = nil
		}
	}()
	if n == nil || depth > 64 || g.ctx.Err() != nil {
		return nil
	}
	switch g.kind(n) {
	case "true":
		return constant.MakeBool(true)
	case "false":
		return constant.MakeBool(false)
	case "interpreted_string_literal", "raw_string_literal":
		return constant.MakeFromLiteral(g.text(n), token.STRING, 0)
	case "int_literal":
		return constant.MakeFromLiteral(g.text(n), token.INT, 0)
	case "float_literal":
		return constant.MakeFromLiteral(g.text(n), token.FLOAT, 0)
	case "imaginary_literal":
		return constant.MakeFromLiteral(g.text(n), token.IMAG, 0)
	case "rune_literal":
		return constant.MakeFromLiteral(g.text(n), token.CHAR, 0)
	case "identifier":
		if b, ok := g.lookup(g.text(n), n.StartByte()); ok && b != nil && b.expression != nil {
			return goConstant(b.file, b.expression, depth+1)
		}
	case "parenthesized_expression", "literal_element":
		if n.NamedChildCount() == 1 {
			return goConstant(g, n.NamedChild(0), depth+1)
		}
	case "unary_expression":
		value := goConstant(g, g.field(n, "operand"), depth+1)
		if value == nil {
			return nil
		}
		op := goOperator(g.text(g.field(n, "operator")))
		if op == token.ADD || op == token.SUB || op == token.NOT || op == token.XOR {
			return constant.UnaryOp(op, value, 0)
		}
	case "binary_expression":
		left, right := goConstant(g, g.field(n, "left"), depth+1), goConstant(g, g.field(n, "right"), depth+1)
		if left == nil || right == nil {
			return nil
		}
		op := goOperator(g.text(g.field(n, "operator")))
		switch op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return constant.MakeBool(constant.Compare(left, op, right))
		case token.SHL, token.SHR:
			count, ok := constant.Uint64Val(right)
			if !ok || count > 4096 {
				return nil
			}
			return constant.Shift(left, op, uint(count))
		case token.ILLEGAL:
			return nil
		default:
			return constant.BinaryOp(left, op, right)
		}
	}
	return nil
}

func goOperator(text string) token.Token {
	switch text {
	case "+":
		return token.ADD
	case "-":
		return token.SUB
	case "*":
		return token.MUL
	case "/":
		return token.QUO
	case "%":
		return token.REM
	case "&":
		return token.AND
	case "|":
		return token.OR
	case "^":
		return token.XOR
	case "&^":
		return token.AND_NOT
	case "<<":
		return token.SHL
	case ">>":
		return token.SHR
	case "&&":
		return token.LAND
	case "||":
		return token.LOR
	case "!":
		return token.NOT
	case "==":
		return token.EQL
	case "!=":
		return token.NEQ
	case "<":
		return token.LSS
	case "<=":
		return token.LEQ
	case ">":
		return token.GTR
	case ">=":
		return token.GEQ
	}
	return token.ILLEGAL
}

func (u *goUsage) assignValue(item *goDeclaration, slot *Slot, g *goFile, n *ts.Node) {
	if n == nil {
		u.slotUnknown(item, slot, g, item.n, "赋值没有可核对的右值")
		return
	}
	for g.kind(n) == "literal_element" && n.NamedChildCount() == 1 {
		n = n.NamedChild(0)
	}
	if slot.Function {
		if g.kind(n) == "func_literal" {
			addUse(&slot.Functions, g.use(n, fmt.Sprintf("%s:lambda@%d", g.file.Path, n.StartByte())))
			return
		}
		targets := u.callableTargets(g, n)
		if len(targets) == 1 {
			addUse(&slot.Functions, g.use(n, targets[0].d.ID))
			return
		}
		if g.kind(n) != "nil" {
			u.slotUnknown(item, slot, g, n, "函数值不能唯一对应到具名函数或匿名函数")
			return
		}
	}
	value := Value{At: g.use(n, slot.Name)}
	if g.kind(n) == "nil" {
		value.Kind, value.Text = "nil", "nil"
	} else if v := goConstant(g, n, 0); v != nil && v.Kind() != constant.Unknown {
		value.Kind, value.Text = v.Kind().String(), v.ExactString()
	} else {
		u.slotUnknown(item, slot, g, n, "值不能解析为字面量或顶层常量")
		return
	}
	for _, old := range slot.Values {
		if old.Kind == value.Kind && old.Text == value.Text && old.At.File == value.At.File && old.At.Span == value.At.Span {
			return
		}
	}
	slot.Values = append(slot.Values, value)
}

func (u *goUsage) callArguments(target *goDeclaration, g *goFile, call *ts.Node) {
	callee, args, _ := goCallParts(g, call)
	parameters := target.d.Usage.Parameters
	if len(u.callableTargets(g, callee)) != 1 || (g.kind(callee) == "selector_expression" && !strings.HasPrefix(g.value(g.field(callee, "operand"), 0).typeName, "local:")) {
		for i := range parameters {
			u.slotUnknown(target, &target.d.Usage.Parameters[i], g, call, "调用仅有名字候选，接收者或目标声明无法唯一核对")
		}
		return
	}
	for i := range parameters {
		slot := &target.d.Usage.Parameters[i]
		if slot.Variadic {
			if i >= len(args) {
				slot.Values = append(slot.Values, Value{Kind: "nil", Text: "nil", Implicit: true, At: g.use(call, slot.Name)})
				return
			}
			var sequence []string
			complete := true
			for j := i; j < len(args); j++ {
				if slot.Function {
					u.assignValue(target, slot, g, args[j])
					continue
				}
				if value := goConstant(g, args[j], 0); value != nil && value.Kind() != constant.Unknown {
					sequence = append(sequence, value.Kind().String()+":"+value.ExactString())
				} else {
					u.slotUnknown(target, slot, g, args[j], "可变参数的展开或元素值无法完整核对")
					complete = false
				}
			}
			if complete && !slot.Function {
				slot.Values = append(slot.Values, Value{Kind: "sequence", Text: "[" + strings.Join(sequence, ",") + "]", At: g.use(call, slot.Name)})
			}
			return
		}
		if i >= len(args) {
			u.slotUnknown(target, slot, g, call, "调用参数数量或多返回值展开无法核对")
			continue
		}
		u.assignValue(target, slot, g, args[i])
	}
	if len(args) > len(parameters) && len(parameters) > 0 {
		u.slotUnknown(target, &target.d.Usage.Parameters[len(parameters)-1], g, call, "调用参数数量与声明不符")
	}
}

func (u *goUsage) construct(g *goFile, n *ts.Node) {
	targets := u.localTypes(g, g.typeName(g.field(n, "type")))
	if len(targets) != 1 {
		return
	}
	item := targets[0]
	body := g.field(n, "body")
	if body == nil {
		return
	}
	position := 0
	for i := 0; i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		if strings.Contains(g.kind(child), "comment") {
			continue
		}
		var slot *Slot
		value := child
		if g.kind(child) == "keyed_element" {
			key := g.field(child, "key")
			value = g.field(child, "value")
			for j := range item.d.Usage.Fields {
				if item.d.Usage.Fields[j].Name == g.text(key) {
					slot = &item.d.Usage.Fields[j]
					break
				}
			}
		} else if position < len(item.d.Usage.Fields) {
			slot = &item.d.Usage.Fields[position]
			position++
		}
		if slot != nil {
			slot.Written = true
			u.assignValue(item, slot, g, value)
		} else {
			u.unknown(item, nil, g, child, "结构体构造字段无法对应到声明")
		}
	}
}

func goAccess(g *goFile, n *ts.Node) (read, write bool, value *ts.Node, unknown string) {
	for current := n; current.Parent() != nil; current = current.Parent() {
		parent := current.Parent()
		switch g.kind(parent) {
		case "inc_statement", "dec_statement":
			return true, true, nil, "自增减的结果依赖先前值"
		case "unary_expression":
			if g.text(g.field(parent, "operator")) == "&" {
				return true, false, nil, "取址后的间接写入无法完整核对"
			}
		case "assignment_statement", "short_var_declaration", "range_clause":
			left, right := g.field(parent, "left"), g.field(parent, "right")
			if left != nil && n.StartByte() >= left.StartByte() && n.EndByte() <= left.EndByte() {
				if g.kind(parent) == "range_clause" {
					return false, true, nil, "循环赋值依赖迭代值"
				}
				for i := 0; i < left.NamedChildCount(); i++ {
					lhs := left.NamedChild(i)
					if n.StartByte() < lhs.StartByte() || n.EndByte() > lhs.EndByte() {
						continue
					}
					if lhs.StartByte() != n.StartByte() || lhs.EndByte() != n.EndByte() {
						return true, true, nil, "经索引、解引用或别名写入，结果无法核对"
					}
					if op := g.text(g.field(parent, "operator")); op != "" && op != "=" && op != ":=" {
						return true, true, nil, "复合赋值的结果依赖先前值"
					}
					if right != nil && right.NamedChildCount() == left.NamedChildCount() {
						return false, true, right.NamedChild(i), ""
					}
					return false, true, nil, "多返回值赋值无法逐项对应"
				}
			}
			return true, false, nil, ""
		case "expression_list", "parenthesized_expression", "index_expression", "selector_expression":
			continue
		default:
			return true, false, nil, ""
		}
	}
	return true, false, nil, ""
}

func (u *goUsage) readWrite(g *goFile, n *ts.Node) {
	if g.kind(n) == "selector_expression" {
		name := g.text(g.field(n, "field"))
		owner := g.value(g.field(n, "operand"), 0).typeName
		fields := u.fields(g, owner, name, map[string]bool{})
		if len(fields) == 1 {
			u.accessSlot(fields[0].item, fields[0].slot, g, n)
		} else {
			if len(fields) == 0 {
				for _, item := range u.declarations {
					for i := range item.d.Usage.Fields {
						slot := &item.d.Usage.Fields[i]
						if slot.Name == name {
							fields = append(fields, goField{item, slot})
						}
					}
				}
			}
			for _, field := range fields {
				u.slotUnknown(field.item, field.slot, g, n, "同名字段的接收者或嵌入关系无法唯一核对")
			}
		}
		return
	}
	name := g.text(n)
	if parent := n.Parent(); parent != nil && g.kind(parent) == "literal_element" && parent.Parent() != nil && g.kind(parent.Parent()) == "keyed_element" {
		if key := g.field(parent.Parent(), "key"); key != nil && key.StartByte() == parent.StartByte() {
			return
		}
	}
	at := n.StartByte()
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		if g.kind(parent) == "var_spec" || g.kind(parent) == "const_spec" {
			if values := g.field(parent, "value"); values == nil || n.EndByte() <= values.StartByte() {
				return
			}
			break
		}
		if g.kind(parent) == "short_var_declaration" || g.kind(parent) == "range_clause" {
			if left := g.field(parent, "left"); left != nil && n.StartByte() >= left.StartByte() && n.EndByte() <= left.EndByte() {
				at = parent.EndByte()
			}
			break
		}
		if g.kind(parent) != "expression_list" && g.kind(parent) != "literal_element" {
			break
		}
	}
	b, ok := g.lookup(name, at)
	if !ok || b == nil {
		return
	}
	if item := u.byScope[g][b.scope]; item != nil {
		for i := range item.d.Usage.Parameters {
			slot := &item.d.Usage.Parameters[i]
			if slot.Name != name || n.StartByte() == slot.Span.Start {
				continue
			}
			u.accessSlot(item, slot, g, n)
		}
	}
}

type goField struct {
	item *goDeclaration
	slot *Slot
}

func (u *goUsage) fields(g *goFile, owner, name string, seen map[string]bool) []goField {
	key := g.packageKey() + ":" + owner
	if seen[key] || u.ctx.Err() != nil {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)
	var result []goField
	for _, item := range u.localTypes(g, owner) {
		var direct []goField
		for i := range item.d.Usage.Fields {
			slot := &item.d.Usage.Fields[i]
			if slot.Name == name {
				direct = append(direct, goField{item, slot})
			}
		}
		if len(direct) > 0 {
			result = append(result, direct...)
			continue
		}
		for _, embedded := range item.embedded {
			result = append(result, u.fields(item.g, embedded, name, seen)...)
		}
	}
	return result
}

func (u *goUsage) aggregateWrite(g *goFile, n *ts.Node) {
	var left []*ts.Node
	right := g.field(n, "right")
	if g.kind(n) == "var_spec" {
		right = g.field(n, "value")
		for i := 0; i < n.NamedChildCount(); i++ {
			if child := n.NamedChild(i); g.kind(child) == "identifier" {
				left = append(left, child)
			}
		}
	} else if values := g.field(n, "left"); values != nil {
		for i := 0; i < values.NamedChildCount(); i++ {
			left = append(left, values.NamedChild(i))
		}
	}
	if right == nil {
		return
	}
	for i, lhs := range left {
		if g.kind(lhs) != "identifier" {
			continue
		}
		owner := g.value(lhs, 0).typeName
		if owner == "" && g.kind(n) == "var_spec" {
			owner = g.typeName(g.field(n, "type"))
		}
		if i < right.NamedChildCount() && len(left) == right.NamedChildCount() && g.value(right.NamedChild(i), 0).typeName == owner {
			continue
		}
		for _, item := range u.localTypes(g, owner) {
			for j := range item.d.Usage.Fields {
				u.slotUnknown(item, &item.d.Usage.Fields[j], g, n, "整个对象被不能追溯的值替换")
			}
		}
	}
}

func (u *goUsage) accessSlot(item *goDeclaration, slot *Slot, g *goFile, n *ts.Node) {
	read, write, value, unknown := goAccess(g, n)
	slot.Read = slot.Read || read
	slot.Written = slot.Written || write
	if parent := n.Parent(); parent != nil && g.kind(parent) == "call_expression" && g.field(parent, "function") == n {
		slot.Function = true
	}
	if unknown != "" {
		u.slotUnknown(item, slot, g, n, unknown)
	} else if write {
		u.assignValue(item, slot, g, value)
	}
}

func (u *goUsage) indirectWrites(g *goFile, n *ts.Node) {
	if len(u.callableTargets(g, g.field(n, "function"))) == 1 {
		return
	}
	target, _ := g.target(g.field(n, "function"), 0)
	for _, arg := range g.arguments(n) {
		for _, item := range u.localTypes(g, g.value(arg, 0).typeName) {
			for i := range item.d.Usage.Fields {
				u.slotUnknown(item, &item.d.Usage.Fields[i], g, arg, "传给 "+target+" 的对象可能经外部调用、解码或反射写入")
			}
		}
	}
}
