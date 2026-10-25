package codescan

import (
	"context"
	"path"
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type goMockIndex struct {
	fields    map[string][][]string
	expects   map[string][]string
	factories map[string][]string
}

func indexGoTestMocks(ctx context.Context, snapshot *Snapshot, files []*goFile) {
	needed := map[string]bool{}
	for _, file := range files {
		if file.file.Kind.Test && strings.Contains(string(file.file.Data), "EXPECT") {
			needed[path.Dir(file.file.Path)] = true
		}
	}
	if len(needed) == 0 {
		return
	}
	indexes := map[string]*goMockIndex{}
	inspect := func(file *goFile) {
		key := path.Dir(file.file.Path) + "\x00" + file.packageName
		index := indexes[key]
		if index == nil {
			index = &goMockIndex{fields: map[string][][]string{}, expects: map[string][]string{}, factories: map[string][]string{}}
			indexes[key] = index
		}
		_ = walkSyntax(ctx, file.syntax.tree.RootNode(), func(n *ts.Node) {
			switch file.kind(n) {
			case "type_spec":
				if typ := file.field(n, "type"); file.kind(typ) == "struct_type" {
					var fields []string
					_ = walkSyntax(ctx, typ, func(field *ts.Node) {
						if file.kind(field) == "field_declaration" {
							fields = append(fields, file.typeName(file.field(field, "type")))
						}
					})
					name := "local:" + file.text(file.field(n, "name"))
					index.fields[name] = append(index.fields[name], fields)
				}
			case "method_declaration":
				if file.text(file.field(n, "name")) != "EXPECT" {
					return
				}
				receiver := file.field(n, "receiver")
				if receiver == nil || receiver.NamedChildCount() != 1 {
					return
				}
				owner := file.typeName(file.field(receiver.NamedChild(0), "type"))
				index.expects[owner] = append(index.expects[owner], file.typeName(file.field(n, "result")))
			case "function_declaration":
				result := file.typeName(file.field(n, "result"))
				parameters := file.field(n, "parameters")
				if parameters == nil || result == "" {
					return
				}
				controller := false
				for i := 0; i < parameters.NamedChildCount(); i++ {
					controller = controller || gomockController(file.typeName(file.field(parameters.NamedChild(i), "type")))
				}
				if controller {
					name := file.text(file.field(n, "name"))
					index.factories[name] = append(index.factories[name], result)
				}
			}
		})
	}
	for _, file := range files {
		if needed[path.Dir(file.file.Path)] {
			inspect(file)
		}
	}
	for _, source := range snapshot.Files {
		if !source.Kind.Generated || source.Kind.Language != "go" || !needed[path.Dir(source.Path)] {
			continue
		}
		entry := grammars.DetectLanguageByName("go")
		tree, err := parse(ctx, entry, source.Data)
		if tree == nil {
			continue
		}
		if err == nil {
			file := &goFile{file: source, syntax: &syntaxFile{tree: tree, language: entry.Language()}, ctx: ctx, imports: map[string]string{}}
			_ = walkSyntax(ctx, tree.RootNode(), func(n *ts.Node) {
				if file.kind(n) == "package_clause" && n.NamedChildCount() > 0 {
					file.packageName = file.text(n.NamedChild(0))
				}
				if file.kind(n) == "import_spec" {
					module, e := strconv.Unquote(file.text(file.field(n, "path")))
					if e != nil {
						return
					}
					alias := file.text(file.field(n, "name"))
					if alias == "" {
						alias = path.Base(module)
					}
					file.imports[alias] = module
				}
			})
			inspect(file)
		}
		tree.Release()
	}
	for _, file := range files {
		index := indexes[path.Dir(file.file.Path)+"\x00"+file.packageName]
		if index == nil {
			continue
		}
		file.mockTypes = map[string]bool{}
		file.mockFactories = map[string]string{}
		for typ, results := range index.expects {
			if len(results) != 1 || len(index.fields[typ]) != 1 || len(index.fields[results[0]]) != 1 {
				continue
			}
			controller, backReference := false, false
			for _, field := range index.fields[typ][0] {
				controller = controller || gomockController(field)
			}
			for _, field := range index.fields[results[0]][0] {
				backReference = backReference || field == typ
			}
			if controller && backReference {
				file.mockTypes[typ] = true
			}
		}
		for factory, results := range index.factories {
			if len(results) == 1 && file.mockTypes[results[0]] {
				file.mockFactories[factory] = results[0]
			}
		}
	}
}

func gomockController(typ string) bool {
	return typ == "go.uber.org/mock/gomock.Controller" || typ == "github.com/golang/mock/gomock.Controller"
}

func (g *goFile) mockReceiver(n *ts.Node, depth int) bool {
	if n == nil || depth > 20 {
		return false
	}
	if g.mockTypes[g.value(n, 0).typeName] {
		return true
	}
	if g.kind(n) == "identifier" {
		if binding, ok := g.lookup(g.text(n), n.StartByte()); ok && binding != nil && binding.initializer != nil {
			return binding.file.mockReceiver(binding.initializer, depth+1)
		}
	}
	if g.kind(n) == "call_expression" {
		fn := g.field(n, "function")
		if g.kind(fn) != "identifier" {
			return false
		}
		name := g.text(fn)
		if g.mockFactories[name] == "" || g.imports[name] != "" {
			return false
		}
		if binding, bound := g.lookup(name, fn.StartByte()); bound && (binding == nil || binding.scope != span(binding.file.syntax.tree.RootNode())) {
			return false
		}
		return true
	}
	return false
}
