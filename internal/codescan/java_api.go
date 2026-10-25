package codescan

import (
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/java"
	ts "github.com/odvcencio/gotreesitter"
)

func javaRiskKinds(target string) []string {
	var result []string
	for _, rule := range java.Rules.Risks {
		if matchesAPI(rule.Target, target) && !slices.Contains(result, rule.Kind) {
			result = append(result, rule.Kind)
		}
	}
	return result
}

func (p *javaFile) inspectAPI(n *ts.Node) {
	switch p.kind(n) {
	case "synchronized_statement":
		p.risk(n, "concurrency")
	case "method_declaration":
		if p.hasModifier(n, "synchronized") {
			p.risk(n, "concurrency")
		}
	case "annotation", "marker_annotation":
		p.inspectAnnotation(n)
	case "type_identifier", "scoped_type_identifier":
		target := p.typeName(n, 0)
		if target == "java.lang.Thread" || strings.HasPrefix(target, "java.util.concurrent.") {
			p.risk(n, "concurrency")
		}
	case "object_creation_expression":
		target := p.typeName(p.field(n, "type"), 0)
		for _, kind := range javaRiskKinds(target + ".new") {
			p.risk(n, kind)
		}
		if target == "" {
			name := p.text(p.field(n, "type"))
			for _, typ := range []string{"Thread", "FileOutputStream", "FileWriter", "ProcessBuilder"} {
				if name == typ {
					p.missing("risks", n, "构造类型 "+name+" 的导入或遮蔽不能核对")
				}
			}
		}
	case "method_invocation":
		target := p.methodTarget(n, 0)
		args := p.named(p.field(n, "arguments"))
		name := p.text(p.field(n, "name"))
		for _, rule := range java.Rules.Entries {
			if target != rule.Target {
				continue
			}
			if len(args) != 1 {
				p.missing("entries", n, "环境变量调用没有唯一的显式名字")
				continue
			}
			value, ok := p.literal(args[0], 0)
			if ok && value.Kind == "String" {
				p.entry(n, rule.Kind, value.Text, "")
			} else {
				p.missing("entries", n, "环境变量名不能解析为字符串常量")
			}
		}
		for _, kind := range javaRiskKinds(target) {
			p.risk(n, kind)
		}
		if target == "java.lang.Class.forName" || target == "java.lang.reflect.Method.invoke" {
			p.missing("entries", n, "反射调用的入口及参数不能完整核对")
			p.missing("risks", n, "反射调用的实际目标不能完整核对")
		}
		if methods := p.engine.methods[target]; len(methods) > 0 {
			for _, method := range methods {
				owner := method.file.enclosingType(method.node)
				for _, base := range p.engine.parentTypes(method.file.types[owner], 0) {
					inherited := p.engine.apiMethod(base, name, 0)
					if len(javaRiskKinds(inherited)) > 0 {
						p.missing("risks", n, "同名方法与已知 API 存在继承或重载，调用签名尚不能唯一核对")
					}
					for _, rule := range java.Rules.Entries {
						if inherited == rule.Target {
							p.missing("entries", n, "入口方法存在继承或重载，调用签名尚不能唯一核对")
						}
					}
				}
			}
		}
		if target == "" || strings.HasPrefix(target, "java.lang.Object.") {
			if name == "getenv" {
				p.missing("entries", n, "getenv 的导入、遮蔽或接收者不能核对")
			}
			for _, rule := range java.Rules.Risks {
				if strings.HasSuffix(rule.Target, "."+name) {
					p.missing("risks", n, "调用 "+name+" 的导入、遮蔽或接收者不能核对")
					break
				}
			}
		}
	case "method_reference":
		children := p.named(n)
		if len(children) < 2 {
			return
		}
		target := p.reference(children[0], 0)
		owner := target.qualified
		if target.typ != "" {
			owner = target.typ
		}
		name := p.text(children[len(children)-1])
		qualified := owner + "." + name
		for _, kind := range javaRiskKinds(qualified) {
			p.risk(n, kind)
		}
		if owner == "" {
			for _, rule := range java.Rules.Risks {
				if strings.HasSuffix(rule.Target, "."+name) {
					p.missing("risks", n, "方法引用 "+name+" 的所属类型不能核对")
					break
				}
			}
			if name == "getenv" {
				p.missing("entries", n, "getenv 方法引用的所属类型及实参不能核对")
			}
		}
		if qualified == "java.lang.System.getenv" {
			p.missing("entries", n, "getenv 作为方法引用传递，环境变量实参不能完整核对")
		}
	}
}

func (p *javaFile) annotationArguments(n *ts.Node) map[string]*ts.Node {
	result := map[string]*ts.Node{}
	for _, child := range p.named(p.field(n, "arguments")) {
		if p.kind(child) == "element_value_pair" {
			result[p.text(p.field(child, "key"))] = p.field(child, "value")
		} else {
			result["value"] = child
		}
	}
	return result
}
func (p *javaFile) annotations(n *ts.Node) []*ts.Node {
	var result []*ts.Node
	for _, child := range p.named(n) {
		if p.kind(child) == "modifiers" {
			for _, item := range p.named(child) {
				if p.kind(item) == "annotation" || p.kind(item) == "marker_annotation" {
					result = append(result, item)
				}
			}
		}
	}
	return result
}
func (p *javaFile) annotationOwner(n *ts.Node) *ts.Node {
	if parent := p.parent(n); p.kind(parent) == "modifiers" {
		return p.parent(parent)
	}
	return nil
}

func (p *javaFile) inspectAnnotation(n *ts.Node) {
	name := p.typeName(p.field(n, "name"), 0)
	rule, known := java.Annotations[name]
	if !known {
		if name == "" {
			simple := p.text(p.field(n, "name"))
			for key := range java.Annotations {
				if strings.HasSuffix(key, "."+simple) {
					p.missing("entries", n, "注解 "+simple+" 的导入或同名类型不能核对")
					break
				}
			}
		}
		return
	}
	args := p.annotationArguments(n)
	switch rule.Kind {
	case "flag", "cmd":
		values, ok := p.strings(args[rule.Property])
		if !ok || len(values) == 0 {
			p.missing("entries", n, "注解入口没有可完整核对的显式名字")
		}
		for _, value := range values {
			if value != "" {
				p.entry(n, rule.Kind, value, "")
			}
		}
	case "route":
		p.routeAnnotation(n, name, rule, args)
	case "cfg":
		values, ok := p.strings(args["value"])
		if !ok {
			p.missing("entries", n, "Value 配置表达式不能解析为字符串常量")
		}
		for _, value := range values {
			names, complete := javaConfigNames(value)
			for _, key := range names {
				p.entry(n, "cfg", key, "")
			}
			if !complete {
				p.missing("entries", n, "配置占位符包含嵌套或不完整名字")
			}
		}
	case "cfg_fields":
		owner := p.annotationOwner(n)
		if !javaTypeNode(p.kind(owner)) {
			p.missing("entries", n, "ConfigurationProperties 不在可核对的类声明上")
			return
		}
		prefix := ""
		value := args["prefix"]
		if value == nil {
			value = args["value"]
		}
		if value != nil {
			values, ok := p.strings(value)
			if !ok || len(values) != 1 {
				p.missing("entries", n, "ConfigurationProperties 前缀不能解析为字符串常量")
				return
			}
			prefix = values[0]
		}
		for _, field := range p.named(p.field(owner, "body")) {
			if p.kind(field) != "field_declaration" || p.hasModifier(field, "static") {
				continue
			}
			for _, child := range p.named(field) {
				if p.kind(child) == "variable_declarator" {
					key := p.text(p.field(child, "name"))
					if prefix != "" {
						key = prefix + "." + key
					}
					p.entry(child, "cfg", key, "")
				}
			}
		}
		if p.kind(owner) == "record_declaration" {
			for _, parameter := range p.named(p.field(owner, "parameters")) {
				name := p.field(parameter, "name")
				if name == nil {
					p.missing("entries", parameter, "配置记录类的字段名不能核对")
					continue
				}
				key := p.text(name)
				if prefix != "" {
					key = prefix + "." + key
				}
				p.entry(parameter, "cfg", key, "")
			}
		}
	}
}

func (p *javaFile) routePaths(n *ts.Node, rule java.AnnotationRule, args map[string]*ts.Node) ([]string, bool) {
	value := args[rule.Property]
	if rule.Property == "" {
		value = args["path"]
		if value == nil {
			value = args["value"]
		}
	}
	if value == nil {
		p.missing("entries", n, "路由注解省略路径，默认路径需结合项目约定核对")
		return nil, false
	}
	values, ok := p.strings(value)
	if !ok || len(values) == 0 {
		p.missing("entries", n, "路由路径不能完整解析为字符串常量")
	}
	return values, ok && len(values) > 0
}
func (p *javaFile) routeAnnotation(n *ts.Node, name string, rule java.AnnotationRule, args map[string]*ts.Node) {
	paths, _ := p.routePaths(n, rule, args)
	if len(paths) == 0 {
		return
	}
	owner := p.annotationOwner(n)
	if !javaTypeNode(p.kind(owner)) {
		if class := p.enclosingType(owner); class != nil {
			matches := 0
			for _, annotation := range p.annotations(class) {
				typ := p.typeName(p.field(annotation, "name"), 0)
				if candidate, ok := java.Annotations[typ]; ok && candidate.Kind == "route" && strings.HasPrefix(name, "org.springframework.") == strings.HasPrefix(typ, "org.springframework.") {
					matches++
				}
			}
			if matches > 1 {
				p.missing("entries", n, "所属类有多个同类路由注解，路径组合不能唯一核对")
				return
			}
			if matches == 0 && p.inheritedRoute(p.types[class], name, map[string]bool{}) {
				p.missing("entries", n, "父类型带有路由注解，继承路径需结合框架约定核对")
				return
			}
			for _, annotation := range p.annotations(class) {
				typ := p.typeName(p.field(annotation, "name"), 0)
				parentRule, ok := java.Annotations[typ]
				if !ok || parentRule.Kind != "route" || strings.HasPrefix(name, "org.springframework.") != strings.HasPrefix(typ, "org.springframework.") {
					continue
				}
				prefixes, complete := p.routePaths(annotation, parentRule, p.annotationArguments(annotation))
				if !complete {
					p.missing("entries", n, "类路径前缀未能完整核对")
					return
				}
				var combined []string
				for _, prefix := range prefixes {
					for _, value := range paths {
						if value == "" {
							combined = append(combined, prefix)
						} else if prefix == "" {
							combined = append(combined, value)
						} else {
							combined = append(combined, strings.TrimSuffix(prefix, "/")+"/"+strings.TrimPrefix(value, "/"))
						}
					}
				}
				paths = combined
			}
		}
	}
	methods := []string{rule.Method}
	if value := args["method"]; value != nil && rule.Method == "" {
		methods = nil
		nodes := []*ts.Node{value}
		if p.kind(value) == "element_value_array_initializer" {
			nodes = p.named(value)
		}
		for _, node := range nodes {
			target := p.reference(node, 0).qualified
			const prefix = "org.springframework.web.bind.annotation.RequestMethod."
			if strings.HasPrefix(target, prefix) {
				methods = append(methods, strings.TrimPrefix(target, prefix))
			} else {
				p.missing("entries", n, "RequestMapping 的 HTTP 方法不能完整核对")
			}
		}
		if len(methods) == 0 {
			methods = []string{""}
		}
	}
	for _, value := range paths {
		if value == "" {
			p.missing("entries", n, "路由路径为空，默认路径需结合项目约定核对")
			continue
		}
		for _, method := range methods {
			p.entry(n, "route", value, method)
		}
	}
}

func (p *javaFile) inheritedRoute(owner, family string, seen map[string]bool) bool {
	if seen[owner] || p.engine.ctx.Err() != nil {
		return false
	}
	seen[owner] = true
	for _, base := range p.engine.parentTypes(owner, 0) {
		if types := p.engine.types[base]; len(types) == 1 {
			item := types[0]
			for _, annotation := range item.file.annotations(item.node) {
				name := item.file.typeName(item.file.field(annotation, "name"), 0)
				if rule, ok := java.Annotations[name]; ok && rule.Kind == "route" && strings.HasPrefix(family, "org.springframework.") == strings.HasPrefix(name, "org.springframework.") {
					return true
				}
			}
		}
		if p.inheritedRoute(base, family, seen) {
			return true
		}
	}
	return false
}
