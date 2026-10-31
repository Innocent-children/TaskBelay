package codescan

import (
	"path"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	ts "github.com/odvcencio/gotreesitter"
)

func javascriptLanguage(language string) bool {
	return slices.Contains([]string{"javascript", "typescript", "tsx"}, language)
}

func (p *jsFile) moduleVisibility(snapshot *Snapshot) Visibility {
	moduleSyntax, jsx := false, false
	root := p.syntax.tree.RootNode()
	for i := 0; i < root.NamedChildCount(); i++ {
		n := root.NamedChild(i)
		moduleSyntax = moduleSyntax || p.kind(n) == "import_statement" || p.kind(n) == "export_statement"
	}
	// 全局扩展和注册不因文件本身是模块而变成私有声明。
	if strings.Contains(string(p.file.Data), "declare global") {
		return Visibility{Scope: "unknown", Reason: "文件包含全局扩展，不能按模块内私有声明缩小范围", Kind: "unsupported"}
	}
	if err := walkSyntax(p.engine.ctx, root, func(n *ts.Node) {
		jsx = jsx || strings.HasPrefix(p.kind(n), "jsx_")
	}); err != nil {
		return Visibility{Scope: "unknown", Reason: err.Error(), Kind: "incomplete"}
	}
	nearest := func(names ...string) []byte {
		for dir := path.Dir(p.file.Path); ; dir = path.Dir(dir) {
			for _, name := range names {
				if file := snapshot.Files[path.Join(dir, name)]; file != nil {
					return file.Data
				}
			}
			if dir == "." || dir == "/" {
				return nil
			}
		}
	}
	scope, reason, kind := javascript.ModuleScope(p.file.Path, moduleSyntax, jsx, nearest("package.json"), nearest("tsconfig.json", "jsconfig.json"))
	return Visibility{Scope: scope, Reason: reason, Kind: kind}
}

/**
 * 名字和配置仍在整个快照中搜索；这里只限定无用结论所需的完整性范围。
 * 同目录但 package 未解析成功的 Go 文件仍须核对，不能绕过其语法错误。
 */
func (s Snapshot) Visible(d Declaration, file *File) bool {
	origin := s.Files[d.File]
	if origin == nil {
		return true
	}
	switch d.Visibility.Scope {
	case "file":
		return file.Path == d.File
	case "package":
		return file.Kind.Language == origin.Kind.Language && path.Dir(file.Path) == path.Dir(d.File) && (file.Package == "" || file.Package == d.Visibility.Package)
	case "language":
		return file.Kind.Language == origin.Kind.Language || javascriptLanguage(file.Kind.Language) && javascriptLanguage(origin.Kind.Language)
	default:
		return true
	}
}
