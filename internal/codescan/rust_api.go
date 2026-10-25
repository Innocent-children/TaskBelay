package codescan

import (
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/rust"
	ts "github.com/odvcencio/gotreesitter"
)

func (p *rustFile) inspectAPI(n *ts.Node) {
	switch p.kind(n) {
	case "struct_item", "enum_item", "function_item", "function_signature_item", "field_declaration", "enum_variant", "mod_item", "const_item", "static_item", "type_item", "impl_item":
		p.inspectAttributes(n)
	case "call_expression":
		target := p.callTarget(n, 0)
		args := p.named(p.field(n, "arguments"))
		for _, rule := range rust.Rules.Entries {
			if target != rule.Target {
				continue
			}
			if len(args) == 0 {
				p.missing("entries", n, "入口调用缺少可核对的显式名字")
				continue
			}
			name, ok := p.literal(args[0], 0)
			if !ok {
				p.missing("entries", n, "入口名字不能解析为字符串常量")
				continue
			}
			if target == "clap::Arg::long" {
				if name == "" {
					p.missing("entries", n, "长参数名字为空")
					continue
				}
				name = "--" + strings.TrimPrefix(name, "--")
			}
			if rule.Kind == "route" {
				if len(args) != 2 {
					p.missing("entries", n, "路由处理器参数不能完整核对")
					continue
				}
				methods, complete := p.routeMethods(args[1], 0)
				if !complete {
					p.missing("entries", n, "路由处理器的 HTTP 方法不能完整核对")
				}
				if len(methods) == 0 {
					p.entry(n, "route", name, "")
				} else {
					for _, method := range methods {
						p.entry(n, "route", name, method)
					}
				}
			} else {
				p.entry(n, rule.Kind, name, "")
			}
		}
		if target == "clap::Command::subcommand" || target == "clap::Command::subcommands" {
			commands := args
			if len(args) == 1 && target == "clap::Command::subcommands" {
				if p.kind(args[0]) == "array_expression" {
					commands = p.named(args[0])
				} else {
					p.missing("entries", n, "子命令列表不是可逐项核对的数组")
					commands = nil
				}
			}
			for _, command := range commands {
				if name, ok := p.commandName(command, 0); ok {
					p.entry(n, "cmd", name, "")
				} else {
					p.missing("entries", command, "加入的子命令没有可核对的显式名字")
				}
			}
		}
		if target == "axum::Router::nest" || target == "axum::Router::merge" {
			p.missing("entries", n, "路由经过嵌套或合并，最终路径需结合组合位置核对")
		}
		for _, kind := range rustRiskKinds(target) {
			p.risk(n, kind)
		}
		if target == "" {
			p.unresolvedCall(n)
		}
	case "use_declaration":
		p.inspectAttributes(n)
		for _, name := range p.importNodes[n] {
			target := p.resolveImport(name, p.scope(n), 0)
			for _, kind := range rustRiskKinds(target) {
				if kind == "concurrency" {
					p.risk(n, kind)
				}
			}
		}
	case "type_identifier", "scoped_type_identifier":
		for _, kind := range rustRiskKinds(p.typeName(n, 0)) {
			p.risk(n, kind)
		}
	case "macro_invocation":
		name := p.text(p.field(n, "macro"))
		unknown := !slices.Contains([]string{"assert", "assert_eq", "assert_ne", "debug_assert", "debug_assert_eq", "debug_assert_ne", "matches", "vec", "format", "format_args", "println", "print", "eprintln", "eprint", "panic", "todo", "unreachable", "unimplemented", "write", "writeln"}, name)
		_ = walkSyntax(p.engine.ctx, n, func(child *ts.Node) {
			if p.kind(child) == "token_tree" && p.kind(p.parent(child)) == "token_tree" && strings.HasPrefix(p.text(child), "(") {
				unknown = true
			}
		})
		if unknown {
			p.missing("entries", n, "宏调用未展开，生成的入口不能完整核对")
			p.missing("risks", n, "宏调用未展开，内部调用不能完整核对")
		}
	case "identifier", "scoped_identifier":
		parent := p.parent(n)
		if slices.Contains([]string{"use_declaration", "use_as_clause", "use_list", "scoped_identifier", "scoped_use_list", "attribute", "token_tree"}, p.kind(parent)) {
			return
		}
		if p.kind(parent) == "call_expression" && p.field(parent, "function") == n {
			return
		}
		if p.kind(parent) == "generic_function" && p.field(parent, "function") == n {
			return
		}
		if p.kind(parent) == "field_expression" && p.field(parent, "field") == n {
			return
		}
		if field := p.field(parent, "name"); field == n {
			return
		}
		target := p.resolve(p.text(n), n, 0)
		for _, rule := range rust.Rules.Entries {
			if target == rule.Target {
				p.missing("entries", n, "入口函数作为值传递，后续实参不能完整核对")
			}
		}
		for _, kind := range rustRiskKinds(target) {
			p.risk(n, kind)
		}
	}
}

func (p *rustFile) unresolvedCall(n *ts.Node) {
	fn := p.field(n, "function")
	if p.kind(fn) == "generic_function" {
		fn = p.field(fn, "function")
	}
	name := p.text(fn)
	if p.kind(fn) == "field_expression" {
		name = p.text(p.field(fn, "field"))
	} else if p.kind(fn) == "scoped_identifier" {
		name = p.text(p.field(fn, "name"))
	}
	if slices.Contains([]string{"var", "var_os", "route", "subcommand", "subcommands", "long", "env"}, name) {
		p.missing("entries", n, "调用 "+name+" 的导入、遮蔽或接收者不能核对")
	}
	if name == "new" && (strings.Contains(p.text(fn), "Arg::") || strings.Contains(p.text(fn), "Command::")) {
		p.missing("entries", n, "构造入口类型的来源不能核对")
	}
	for _, rule := range rust.Rules.Risks {
		if strings.HasSuffix(rule.Target, "::"+name) {
			p.missing("risks", n, "调用 "+name+" 的导入、遮蔽或接收者不能核对")
			break
		}
	}
}

func (p *rustFile) routeMethods(n *ts.Node, depth int) ([]string, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return nil, false
	}
	if p.kind(n) == "identifier" {
		if b, ok := p.lookup(p.text(n), n); ok && b != nil && !b.changed {
			return b.file.routeMethods(b.value, depth+1)
		}
	}
	if p.kind(n) != "call_expression" {
		return nil, false
	}
	target := p.callTarget(n, 0)
	if method := strings.TrimPrefix(target, "axum::routing::"); target != method && slices.Contains(rust.HTTPMethods, method) {
		return []string{strings.ToUpper(method)}, true
	}
	fn := p.field(n, "function")
	if p.kind(fn) == "field_expression" {
		methods, ok := p.routeMethods(p.field(fn, "value"), depth+1)
		name := p.text(p.field(fn, "field"))
		if ok && slices.Contains(rust.HTTPMethods, name) {
			value := strings.ToUpper(name)
			if !slices.Contains(methods, value) {
				methods = append(methods, value)
			}
			return methods, true
		}
	}
	return nil, false
}

func (p *rustFile) commandName(n *ts.Node, depth int) (string, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return "", false
	}
	if p.kind(n) == "identifier" {
		if b, ok := p.lookup(p.text(n), n); ok && b != nil && !b.changed {
			return b.file.commandName(b.value, depth+1)
		}
	}
	if p.kind(n) != "call_expression" {
		return "", false
	}
	if target := p.callTarget(n, 0); target == "clap::Command::new" || target == "clap::Command::name" {
		args := p.named(p.field(n, "arguments"))
		if len(args) == 1 {
			return p.literal(args[0], depth+1)
		}
		return "", false
	}
	if fn := p.field(n, "function"); p.kind(fn) == "field_expression" && p.reference(n, 0).typ == "clap::Command" {
		return p.commandName(p.field(fn, "value"), depth+1)
	}
	return "", false
}

func (p *rustFile) attributes(n *ts.Node) []*ts.Node {
	parent := p.parent(n)
	if parent == nil {
		return nil
	}
	var result []*ts.Node
	for i := 0; i < parent.ChildCount(); i++ {
		if parent.Child(i) != n {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			child := parent.Child(j)
			if !child.IsNamed() || strings.Contains(p.kind(child), "comment") {
				continue
			}
			if p.kind(child) != "attribute_item" {
				break
			}
			result = append(result, child)
		}
		break
	}
	slices.Reverse(result)
	return result
}
func (p *rustFile) attributeParts(n *ts.Node) (string, [][]*ts.Node) {
	children := p.named(n)
	if len(children) != 1 {
		return "", nil
	}
	attribute := children[0]
	children = p.named(attribute)
	if len(children) == 0 {
		return "", nil
	}
	name := p.text(children[0])
	args := p.field(attribute, "arguments")
	var groups [][]*ts.Node
	var group []*ts.Node
	if args != nil {
		for i := 1; i < args.ChildCount()-1; i++ {
			child := args.Child(i)
			if p.text(child) == "," {
				if len(group) > 0 {
					groups = append(groups, group)
					group = nil
				}
				continue
			}
			if !strings.Contains(p.kind(child), "comment") {
				group = append(group, child)
			}
		}
	}
	if len(group) > 0 {
		groups = append(groups, group)
	}
	return name, groups
}
func (p *rustFile) groupText(nodes []*ts.Node) string {
	var text strings.Builder
	for _, n := range nodes {
		text.WriteString(p.text(n))
	}
	return text.String()
}

func (p *rustFile) clapOwner(n *ts.Node) (bool, bool) {
	for node := n; node != nil; node = p.parent(node) {
		if p.kind(node) != "struct_item" && p.kind(node) != "enum_item" {
			continue
		}
		known, uncertain := false, false
		for _, attribute := range p.attributes(node) {
			name, groups := p.attributeParts(attribute)
			if name != "derive" {
				continue
			}
			for _, group := range groups {
				derived := p.groupText(group)
				qualified := p.resolve(derived, attribute, 0)
				if slices.Contains([]string{"clap::Parser", "clap::Args", "clap::Subcommand"}, qualified) {
					known = true
				}
				if qualified == "" && slices.Contains([]string{"Parser", "Args", "Subcommand"}, derived) {
					uncertain = true
				}
			}
		}
		return known, uncertain
	}
	return false, true
}

func (p *rustFile) attributeEntry(owner, evidence *ts.Node, kind, name, method string) {
	e := Entry{Kind: kind, Name: name, Method: method, Declaration: p.owner(owner), Span: span(evidence), Evidence: p.text(evidence)}
	if !slices.Contains(p.file.Entries, e) {
		p.file.Entries = append(p.file.Entries, e)
	}
}

func (p *rustFile) inspectAttributes(n *ts.Node) {
	attributes := p.attributes(n)
	for _, attr := range attributes {
		name, groups := p.attributeParts(attr)
		if (name == "cfg" && strings.Join(strings.Fields(p.text(attr)), "") != "#[cfg(test)]") || name == "cfg_attr" || name == "path" {
			p.missing("entries", attr, "条件属性或特殊模块路径需要构建配置才能完整核对")
			p.missing("risks", attr, "条件属性或特殊模块路径需要构建配置才能完整核对")
		}
		if p.kind(n) == "field_declaration" && (name == "arg" || name == "clap") {
			known, uncertain := p.clapOwner(n)
			if !known {
				if uncertain {
					p.missing("entries", attr, "clap 字段属性的派生来源不能核对")
				}
				continue
			}
			for _, group := range groups {
				key := p.text(group[0])
				kind := ""
				if key == "long" {
					kind = "flag"
				}
				if key == "env" {
					kind = "env"
				}
				if kind == "" {
					continue
				}
				if len(group) == 3 && p.text(group[1]) == "=" {
					if value, ok := p.literal(group[2], 0); ok {
						if kind == "flag" {
							if value == "" {
								p.missing("entries", attr, "长参数名字为空")
								continue
							}
							value = "--" + strings.TrimPrefix(value, "--")
						}
						p.attributeEntry(n, attr, kind, value, "")
						continue
					}
				}
				p.missing("entries", attr, "clap 属性没有可核对的显式名字，默认命名或表达式需结合项目版本核对")
			}
		}
		if p.kind(n) == "function_item" {
			target := p.resolve(name, attr, 0)
			method := strings.TrimPrefix(target, "actix_web::")
			if target != method && slices.Contains(rust.HTTPMethods, method) {
				if len(groups) == 1 && len(groups[0]) == 1 {
					if value, ok := p.literal(groups[0][0], 0); ok {
						p.attributeEntry(n, attr, "route", value, strings.ToUpper(method))
						continue
					}
				}
				p.missing("entries", attr, "HTTP 属性的路径不能解析为字符串常量")
			} else if target == "" && slices.Contains(rust.HTTPMethods, name) {
				p.missing("entries", attr, "HTTP 属性的导入或遮蔽不能核对")
			}
		}
	}
	if p.kind(n) == "enum_item" {
		subcommand, unknown := false, false
		for _, attr := range attributes {
			name, groups := p.attributeParts(attr)
			if name != "derive" {
				continue
			}
			for _, group := range groups {
				text := p.groupText(group)
				target := p.resolve(text, attr, 0)
				subcommand = subcommand || target == "clap::Subcommand"
				unknown = unknown || (text == "Subcommand" && target == "")
			}
		}
		if unknown {
			p.missing("entries", n, "Subcommand 派生的导入或遮蔽不能核对")
		}
		if subcommand {
			for _, variant := range p.named(p.field(n, "body")) {
				if p.kind(variant) != "enum_variant" {
					continue
				}
				found := false
				for _, attr := range p.attributes(variant) {
					name, groups := p.attributeParts(attr)
					if name != "command" && name != "clap" {
						continue
					}
					for _, group := range groups {
						if len(group) == 3 && p.text(group[0]) == "name" && p.text(group[1]) == "=" {
							if value, ok := p.literal(group[2], 0); ok {
								p.attributeEntry(n, attr, "cmd", value, "")
								found = true
							}
						}
					}
				}
				if !found {
					p.missing("entries", variant, "子命令变体省略显式名字，默认命名需结合项目版本核对")
				}
			}
		}
	}
}
