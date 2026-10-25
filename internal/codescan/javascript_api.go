package codescan

import (
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	ts "github.com/odvcencio/gotreesitter"
)

func jsTarget(target string) string {
	if strings.HasPrefix(target, "commander.program.") {
		return "commander.Command." + strings.TrimPrefix(target, "commander.program.")
	}
	if target == "commander.option" || target == "commander.command" {
		return "commander.Command." + strings.TrimPrefix(target, "commander.")
	}
	if target == "yargs.option" || target == "yargs.options" || target == "yargs.command" {
		return "yargs.Argv." + strings.TrimPrefix(target, "yargs.")
	}
	return target
}

func (p *jsFile) modified(target string) bool {
	for name := range p.mutations {
		if target == name || strings.HasPrefix(target, name+".") {
			return true
		}
	}
	return false
}

func (p *jsFile) inspectAPI(n *ts.Node) {
	if p.kind(n) == "member_expression" || p.kind(n) == "subscript_expression" {
		base := p.reference(p.field(n, "object"), 0).qualified
		if base == "global.process.env" || base == "import.meta.env" {
			if p.modified(base) {
				p.missing("entries", n, "环境变量对象被重新赋值，入口不能核对")
				return
			}
			name := p.propertyName(p.field(n, "property"))
			if p.kind(n) == "subscript_expression" {
				value, ok := p.literal(p.field(n, "index"), 0)
				if ok && value.Kind == "String" {
					name = value.Text
				}
			}
			if name != "" {
				p.entry(n, "env", name, "")
			} else {
				p.missing("entries", n, "环境变量名不是可核对的字符串常量")
			}
			return
		}
		if base == "" && (strings.HasPrefix(p.text(n), "process.env.") || strings.HasPrefix(p.text(n), "process.env[")) {
			p.missing("entries", n, "process 的局部遮蔽或重绑定不能核对")
		}
	}
	if p.kind(n) != "call_expression" && p.kind(n) != "new_expression" {
		return
	}
	callee := p.field(n, "function")
	if callee == nil {
		callee = p.field(n, "constructor")
	}
	raw := p.reference(callee, 0).qualified
	target := jsTarget(raw)
	args := p.arguments(n)
	if p.modified(raw) {
		target = ""
	}
	if target == "global.eval" {
		p.missing("entries", n, "eval 的动态代码不能静态核对")
		p.missing("risks", n, "eval 的动态代码不能静态核对")
	}
	for _, suffix := range []string{".call", ".apply", ".bind"} {
		if strings.HasSuffix(target, suffix) {
			base := jsTarget(strings.TrimSuffix(target, suffix))
			for _, rule := range javascript.Rules.Entries {
				if rule.Target == base {
					p.missing("entries", n, "入口通过 call/apply/bind 间接调用，实参不能完整核对")
				}
			}
			if len(jsRiskKinds(base, true)) > 0 {
				p.missing("risks", n, "风险调用通过 call/apply/bind 传递，实际调用不能完整核对")
			}
		}
	}
	if p.kind(callee) == "subscript_expression" && target == "" {
		base := p.reference(p.field(callee, "object"), 0)
		receiver := base.qualified
		if base.typ != "" {
			receiver = base.typ
		}
		if slices.Contains([]string{"fs", "fs.promises", "fs/promises", "child_process", "worker_threads", "global.JSON", "global.Promise", "global.Date"}, receiver) {
			p.missing("risks", n, "已知模块的计算方法名不能核对")
		}
		if strings.HasPrefix(receiver, "commander") || strings.HasPrefix(receiver, "yargs") {
			p.missing("entries", n, "入口接收者的计算方法名不能核对")
		}
		if len(args) > 0 {
			if value, ok := p.literal(args[0], 0); ok && value.Kind == "String" && strings.HasPrefix(value.Text, "/") {
				p.missing("entries", n, "路由候选的计算方法名不能核对")
			}
		}
	}
	if target == "global.require" && len(args) == 1 {
		value, ok := p.literal(args[0], 0)
		if ok && value.Kind == "String" && slices.Contains(javascript.Rules.ConcurrentImports, p.module(value.Text)) {
			p.risk(n, "concurrency")
		}
		if !ok || value.Kind != "String" {
			p.missing("entries", n, "require 的动态模块名不能核对")
			p.missing("risks", n, "require 的动态模块名不能核对")
		}
	}
	for _, rule := range javascript.Rules.Entries {
		if target != rule.Target {
			continue
		}
		if len(args) == 0 {
			p.missing("entries", n, "入口调用缺少可核对的参数")
			continue
		}
		switch target {
		case "util.parseArgs":
			options := p.objectProperty(args[0], "options")
			if options == nil {
				p.missing("entries", n, "parseArgs 的 options 对象不能核对")
				continue
			}
			p.optionObject(n, options, true)
		case "yargs.Argv.options":
			p.optionObject(n, args[0], false)
		default:
			value, ok := p.literal(args[0], 0)
			if !ok || value.Kind != "String" {
				p.missing("entries", n, "命令或参数名字不是可核对的字符串常量")
				continue
			}
			if rule.Kind == "cmd" {
				name := strings.Fields(value.Text)
				if len(name) > 0 {
					p.entry(n, "cmd", name[0], "")
				}
				continue
			}
			if strings.HasPrefix(target, "commander.") {
				p.commanderFlags(n, value.Text)
			} else {
				p.flagName(n, value.Text, false)
				if len(args) > 1 {
					p.optionAliases(n, args[1], false)
				}
			}
		}
	}
	route := false
	if p.kind(callee) == "member_expression" || p.kind(callee) == "subscript_expression" {
		name := p.propertyName(p.field(callee, "property"))
		if p.kind(callee) == "subscript_expression" {
			value, ok := p.literal(p.field(callee, "index"), 0)
			if ok && value.Kind == "String" {
				name = value.Text
			}
		}
		if slices.Contains([]string{"get", "post", "put", "patch", "delete", "all", "route"}, name) && len(args) > 0 {
			value, ok := p.literal(args[0], 0)
			if ok && value.Kind == "String" {
				if strings.HasPrefix(value.Text, "/") {
					method := strings.ToUpper(name)
					if name == "all" || name == "route" {
						method = ""
					}
					p.entry(n, "route", value.Text, method)
					route = true
				}
			} else {
				p.missing("entries", n, "路由候选的第一个参数不能核对为路径常量")
			}
		}
	}
	for _, kind := range jsRiskKinds(target, p.kind(n) == "new_expression") {
		p.risk(n, kind)
	}
	if target == "" {
		name := p.text(callee)
		if p.kind(callee) == "member_expression" {
			name = p.propertyName(p.field(callee, "property"))
		}
		for _, rule := range javascript.Rules.Entries {
			if !route && strings.HasSuffix(rule.Target, "."+name) {
				p.missing("entries", n, "调用 "+name+" 的导入、遮蔽或接收者不能核对")
				break
			}
		}
		if slices.Contains(javascript.FileWrites, name) {
			p.missing("risks", n, "文件写入候选 "+name+" 的接收者或绑定不能核对")
		}
		for _, rule := range javascript.Rules.Risks {
			if strings.HasSuffix(rule.Target, "."+name) {
				p.missing("risks", n, "风险候选 "+name+" 的绑定或遮蔽不能核对")
				break
			}
		}
	}
}

func jsRiskKinds(target string, construct bool) []string {
	var kinds []string
	for _, rule := range javascript.Rules.Risks {
		if matchesAPI(rule.Target, target) && (target != "global.URL" || construct) {
			kinds = append(kinds, rule.Kind)
		}
	}
	for _, module := range []string{"fs.", "fs/promises.", "fs.promises."} {
		if strings.HasPrefix(target, module) && slices.Contains(javascript.FileWrites, strings.TrimPrefix(target, module)) {
			kinds = append(kinds, "write")
		}
	}
	return kinds
}

func (p *jsFile) commanderFlags(n *ts.Node, text string) {
	for _, part := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '|' }) {
		if strings.HasPrefix(part, "-") {
			p.entry(n, "flag", strings.SplitN(part, "=", 2)[0], "")
		}
	}
}

func (p *jsFile) flagName(n *ts.Node, name string, short bool) {
	if name == "" {
		p.missing("entries", n, "参数名字为空")
		return
	}
	prefix := "--"
	if short || len([]rune(name)) == 1 {
		prefix = "-"
	}
	if strings.HasPrefix(name, "-") {
		prefix = ""
	}
	p.entry(n, "flag", prefix+name, "")
}

func (p *jsFile) objectValue(n *ts.Node, depth int) *ts.Node {
	if n == nil || depth > 40 {
		return nil
	}
	if p.kind(n) == "object" {
		return n
	}
	if p.kind(n) == "identifier" {
		if b, ok := p.lookup(p.text(n), n); ok && b != nil && b.constant && b.scope.kind == "program" && b.file == p {
			return p.objectValue(b.value, depth+1)
		}
	}
	if p.kind(n) == "as_expression" || p.kind(n) == "parenthesized_expression" || p.kind(n) == "satisfies_expression" {
		if n.NamedChildCount() > 0 {
			return p.objectValue(n.NamedChild(0), depth+1)
		}
	}
	return nil
}

func (p *jsFile) objectProperty(n *ts.Node, name string) *ts.Node {
	n = p.objectValue(n, 0)
	if n == nil {
		return nil
	}
	var result *ts.Node
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if !child.IsNamed() || p.kind(child) == "comment" {
			continue
		}
		if p.kind(child) == "spread_element" {
			return nil
		}
		if p.kind(child) == "pair" && p.propertyName(p.field(child, "key")) == name {
			result = p.field(child, "value")
		}
	}
	return result
}

func (p *jsFile) optionObject(call, n *ts.Node, util bool) {
	n = p.objectValue(n, 0)
	if n == nil {
		p.missing("entries", call, "参数定义不是可核对的对象字面量或顶层常量")
		return
	}
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if !child.IsNamed() || p.kind(child) == "comment" {
			continue
		}
		name := p.propertyName(p.field(child, "key"))
		if name == "" {
			p.missing("entries", child, "参数对象存在计算名字、展开或不能核对的属性")
			continue
		}
		if util {
			p.entry(call, "flag", "--"+name, "")
		} else {
			p.flagName(call, name, false)
		}
		p.optionAliases(call, p.field(child, "value"), util)
	}
}

func (p *jsFile) optionAliases(call, n *ts.Node, util bool) {
	object := p.objectValue(n, 0)
	if object == nil {
		p.missing("entries", call, "参数配置不能核对，短名或别名未知")
		return
	}
	property := "alias"
	if util {
		property = "short"
	}
	for i := 0; i < object.ChildCount(); i++ {
		if p.kind(object.Child(i)) == "spread_element" {
			p.missing("entries", object, "参数配置的展开属性不能完整核对")
		}
	}
	alias := p.objectProperty(object, property)
	if alias == nil {
		return
	}
	values := []*ts.Node{alias}
	if p.kind(alias) == "array" {
		values = nil
		for i := 0; i < alias.ChildCount(); i++ {
			if child := alias.Child(i); child.IsNamed() && p.kind(child) != "comment" {
				values = append(values, child)
			}
		}
	}
	for _, n := range values {
		value, ok := p.literal(n, 0)
		if ok && value.Kind == "String" {
			p.flagName(call, value.Text, util)
		} else {
			p.missing("entries", call, "参数的短名或别名不是可核对的字符串常量")
		}
	}
}
