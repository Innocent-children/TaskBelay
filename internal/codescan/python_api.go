package codescan

import (
	"path"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/python"
	ts "github.com/odvcencio/gotreesitter"
)

type pyArgument struct {
	name   string
	value  *ts.Node
	spread bool
}

func (p *pyFile) arguments(call *ts.Node) []pyArgument {
	n := p.field(call, "arguments")
	if n == nil {
		return nil
	}
	var args []pyArgument
	for i := 0; i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		if strings.Contains(p.kind(child), "comment") {
			continue
		}
		arg := pyArgument{value: child}
		if p.kind(child) == "keyword_argument" {
			arg.name = p.text(p.field(child, "name"))
			arg.value = p.field(child, "value")
		}
		arg.spread = p.kind(child) == "list_splat" || p.kind(child) == "dictionary_splat"
		args = append(args, arg)
	}
	return args
}

func pyArgumentAt(args []pyArgument, index int, names ...string) *ts.Node {
	position := 0
	for _, arg := range args {
		if arg.spread {
			return nil
		}
		if arg.name != "" {
			if slices.Contains(names, arg.name) {
				return arg.value
			}
			continue
		}
		if position == index {
			return arg.value
		}
		position++
	}
	return nil
}

func (p *pyFile) entry(n *ts.Node, kind, name, method string) {
	use := p.use(n, "")
	for _, entry := range p.file.Entries {
		if entry.Span == use.Span && entry.Kind == kind && entry.Name == name && entry.Method == method {
			return
		}
	}
	p.file.Entries = append(p.file.Entries, Entry{Kind: kind, Name: name, Method: method, Declaration: use.Declaration, Evidence: p.text(n), Span: use.Span})
}

func (p *pyFile) risk(n *ts.Node, kind string) {
	use := p.use(n, "")
	for _, risk := range p.file.Risks {
		if risk.Span == use.Span && risk.Kind == kind {
			return
		}
	}
	p.file.Risks = append(p.file.Risks, Risk{Kind: kind, Declaration: use.Declaration, Evidence: p.text(n), Span: use.Span})
}

func (p *pyFile) inspectAPI(n *ts.Node) {
	if p.kind(n) == "decorator" && n.NamedChildCount() == 1 {
		expression := n.NamedChild(0)
		if p.kind(expression) != "call" {
			target := p.reference(expression, 0).qualified
			if target == "click.command" || target == "click.Group.command" {
				p.missing("entries", n, "命令装饰器未给出显式名字，默认命名需结合项目 click 版本核对")
			}
		}
	}
	if p.kind(n) == "subscript" && p.reference(p.field(n, "value"), 0).qualified == "os.environ" {
		value, ok := p.literal(p.field(n, "subscript"), 0)
		if ok && value.Kind == "String" {
			p.entry(n, "env", value.Text, "")
		} else {
			p.missing("entries", n, "环境变量名不能解析为字符串常量")
		}
		return
	}
	if p.kind(n) != "call" {
		return
	}
	callee := p.field(n, "function")
	target := p.reference(callee, 0).qualified
	args := p.arguments(n)
	decorator := p.kind(n.Parent()) == "decorator"
	for _, rule := range python.Rules.Entries {
		if target != rule.Target {
			continue
		}
		if strings.HasPrefix(target, "click.") && !decorator {
			continue
		}
		if rule.Kind == "route" && path.Base(p.file.Path) != "urls.py" {
			continue
		}
		if rule.Kind == "flag" {
			for _, arg := range args {
				if arg.name != "" {
					continue
				}
				value, ok := p.literal(arg.value, 0)
				if !ok || value.Kind != "String" {
					p.missing("entries", n, "命令行参数名不能解析为字符串常量")
					continue
				}
				if strings.HasPrefix(value.Text, "-") {
					p.entry(n, "flag", value.Text, "")
				}
			}
			continue
		}
		valueNode := pyArgumentAt(args, rule.Arg, "name", "key", "route", "pattern")
		value, ok := p.literal(valueNode, 0)
		if !ok || value.Kind != "String" {
			p.missing("entries", n, "入口名字缺少可核对的字符串常量")
			continue
		}
		p.entry(n, rule.Kind, value.Text, "")
	}
	routeDecorator := false
	if decorator && p.kind(callee) == "attribute" {
		method := p.text(p.field(callee, "attribute"))
		if slices.Contains([]string{"route", "get", "post", "put", "patch", "delete"}, method) {
			routeDecorator = true
			value, ok := p.literal(pyArgumentAt(args, 0, "path", "rule"), 0)
			if ok && value.Kind == "String" && strings.HasPrefix(value.Text, "/") {
				if method != "route" {
					p.entry(n, "route", value.Text, strings.ToUpper(method))
				} else {
					p.routeMethods(n, args, value.Text)
				}
			} else {
				p.missing("entries", n, "路由装饰器的路径不能核对为以 / 开头的字符串常量")
			}
		}
	}
	for _, rule := range python.Rules.Risks {
		if matchesAPI(rule.Target, target) {
			p.risk(n, rule.Kind)
		}
	}
	if target == "builtins.open" {
		for _, arg := range args {
			if arg.spread {
				p.missing("risks", n, "open 的展开参数可能指定文件模式")
			}
		}
		modeNode := pyArgumentAt(args, 1, "mode")
		if modeNode != nil {
			mode, ok := p.literal(modeNode, 0)
			if !ok || mode.Kind != "String" {
				p.missing("risks", n, "open 的文件模式不能静态核对")
			} else if strings.ContainsAny(mode.Text, "wax+") {
				p.risk(n, "write")
			}
		}
	}
	if target == "warnings.warn" {
		category := pyArgumentAt(args, 1, "category")
		if name := p.reference(category, 0).qualified; name == "builtins.DeprecationWarning" || name == "builtins.PendingDeprecationWarning" {
			if owner := p.owner(n); owner != nil {
				owner.d.Usage.Deprecated = true
			}
		}
	}
	if target == "" {
		name := p.text(callee)
		if p.kind(callee) == "attribute" {
			name = p.text(p.field(callee, "attribute"))
		}
		for _, rule := range python.Rules.Entries {
			if !routeDecorator && strings.HasSuffix(rule.Target, "."+name) {
				p.missing("entries", n, "调用 "+name+" 的导入、遮蔽、重绑定或接收者尚不能核对")
				break
			}
		}
		for _, rule := range python.Rules.Risks {
			if strings.HasSuffix(rule.Target, "."+name) {
				p.missing("risks", n, "调用 "+name+" 的导入、遮蔽、重绑定或接收者尚不能核对")
				break
			}
		}
		if name == "execute" || name == "executemany" {
			p.missing("risks", n, "execute/executemany 的数据库游标接收者尚不能核对")
		}
		if name == "open" {
			p.missing("risks", n, "open 的绑定或遮蔽尚不能核对")
		}
	}
}

func (p *pyFile) routeMethods(n *ts.Node, args []pyArgument, name string) {
	var methods *ts.Node
	for _, arg := range args {
		if arg.name == "methods" {
			methods = arg.value
		}
	}
	if methods == nil {
		p.entry(n, "route", name, "")
		return
	}
	if p.kind(methods) != "list" && p.kind(methods) != "tuple" {
		p.missing("entries", n, "路由方法列表不能静态核对")
		return
	}
	for i := 0; i < methods.NamedChildCount(); i++ {
		value, ok := p.literal(methods.NamedChild(i), 0)
		if ok && value.Kind == "String" {
			p.entry(n, "route", name, strings.ToUpper(value.Text))
		} else {
			p.missing("entries", n, "路由方法不是字符串常量")
		}
	}
}
