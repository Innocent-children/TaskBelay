package codescan

import (
	"context"
	"fmt"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/lang"
	"github.com/Innocent-children/taskbelay/internal/lang/golang"
	"github.com/Innocent-children/taskbelay/internal/lang/java"
	"github.com/Innocent-children/taskbelay/internal/lang/javascript"
	"github.com/Innocent-children/taskbelay/internal/lang/python"
	"github.com/Innocent-children/taskbelay/internal/lang/rust"
	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

/**
 * 调用方提供同一固定 tree 的 blob。分析结果只引用这些字节，不访问实时文件；
 * 预算中断和单文件失败保留已取得的事实，不能把遗漏范围标为代码检查完成。
 */
func Analyze(ctx context.Context, tree string, sources []Source) Snapshot {
	result := Snapshot{Tree: tree, Files: make(map[string]*File)}
	parsed := map[string]*syntaxFile{}
	defer func() {
		for _, syntax := range parsed {
			syntax.tree.Release()
		}
	}()
	sources = append([]Source(nil), sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	for _, source := range sources {
		file := &File{Source: source, Kind: lang.Classify(source.Path, source.Data), Operations: map[string]Operation{
			"files":        {Complete: true},
			"declarations": {Reason: "文件级处理，尚未取得具名声明"},
			"references":   {Reason: "文件级处理，尚未取得引用候选"},
			"dependencies": {Reason: "不是文档规定的依赖清单"},
			"entries":      {Reason: "本语言的入口分析尚未接通"},
			"risks":        {Reason: "本语言的风险分析尚未接通"},
			"usage":        {Reason: "调用、参数及字段值分析尚未接通"},
			"tests":        {Reason: "用例枚举与 Runner 尚未接通"},
		}}
		for name, operation := range file.Operations {
			if !operation.Complete {
				operation.Kind = "unsupported"
				file.Operations[name] = operation
			}
		}
		result.Files[source.Path] = file
		if source.Mode != "" && source.Mode != "100644" && source.Mode != "100755" {
			continue
		}
		if file.Kind.Dependency || file.Kind.Lock || file.Kind.Generated || file.Kind.TestData {
			continue
		}
		if err := ctx.Err(); err != nil {
			file.Missing = append(file.Missing, Missing{source.Path, "analysis", err.Error(), "incomplete"})
		} else {
			if lang.Manifest(source.Path) {
				var err error
				file.Dependencies, err = Dependencies(ctx, source.Path, source.Data)
				if err != nil {
					file.Missing = append(file.Missing, Missing{source.Path, "dependencies", err.Error(), failureKind(err)})
					file.Operations["dependencies"] = Operation{Reason: err.Error()}
				} else {
					file.Operations["dependencies"] = Operation{Complete: true}
				}
			}
			if file.Kind.Source {
				if syntax := analyzeCode(ctx, file); syntax != nil {
					parsed[file.Path] = syntax
				}
			}
		}
		result.Missing = append(result.Missing, file.Missing...)
	}
	analyzeGoAPIs(ctx, &result, parsed)
	analyzePython(ctx, &result, parsed)
	analyzeJavaScriptAPIs(ctx, &result, parsed)
	analyzeJavaAPIs(ctx, &result, parsed)
	analyzeRustAPIs(ctx, &result, parsed)
	analyzeGenericUsage(ctx, &result, parsed)
	result.Missing = nil
	for _, source := range sources {
		finishOperations(result.Files[source.Path])
		file := result.Files[source.Path]
		for i := range file.Declarations {
			d := &file.Declarations[i]
			if d.Visibility.Scope == "" {
				if d.Usage != nil {
					d.Visibility.Scope = "language"
				} else {
					d.Visibility = Visibility{Scope: "unknown", Kind: "unsupported", Reason: "本语言尚未核对声明可见范围"}
				}
			}
		}
		result.Missing = append(result.Missing, result.Files[source.Path].Missing...)
	}
	return result
}

func tagsQuery(entry *grammars.LangEntry) string {
	query := grammars.ResolveTagsQuery(*entry)
	switch entry.Name {
	case "go":
		query += golang.Tags
	case "python":
		query += python.Tags
	case "javascript":
		query += javascript.Tags
	case "typescript", "tsx":
		query += javascript.Tags + javascript.TypeScriptTags
	case "java":
		query += java.Tags
	case "rust":
		query += rust.Tags
	default:
		query += lang.GenericTags(entry.Name)
	}
	return query
}

type syntaxFile struct {
	tree     *ts.Tree
	language *ts.Language
	parents  syntaxParents
}

func analyzeCode(ctx context.Context, file *File) *syntaxFile {
	missing := func(operation string, err error) {
		file.Missing = append(file.Missing, Missing{file.Path, operation, err.Error(), failureKind(err)})
		file.Operations[operation] = Operation{Reason: err.Error()}
	}
	entry := grammars.DetectLanguageByName(file.Kind.Language)
	if entry == nil || entry.Language == nil {
		missing("syntax", fmt.Errorf("没有内置语法"))
		return nil
	}
	queryText := tagsQuery(entry)
	if queryText == "" {
		missing("declarations", fmt.Errorf("没有内置 tags 查询"))
		return nil
	}
	language := entry.Language()
	query, err := ts.NewQuery(queryText, language)
	if err != nil {
		missing("declarations", err)
		return nil
	}
	tree, err := parse(ctx, entry, file.Data)
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		missing("syntax", err)
		return nil
	}
	parents, err := indexSyntaxParents(ctx, tree, file.Kind.Language)
	if err != nil {
		tree.Release()
		missing("syntax", err)
		return nil
	}
	file.Declarations, err = declarations(ctx, file, tree, query, language, parents)
	if err != nil {
		missing("declarations", err)
	}
	if err := auditDefinitions(ctx, file, tree, language, parents); err != nil {
		missing("declarations", err)
	}
	var excluded []Span
	for _, declaration := range file.Declarations {
		excluded = append(excluded, declaration.Ranges()...)
	}
	tokens, err := leafTokens(ctx, tree.RootNode(), language, file.Data, excluded)
	if err != nil {
		missing("references", err)
	} else {
		names := map[string]bool{}
		for _, token := range tokens {
			if token.Kind == "identifier" {
				names[token.Text] = true
			}
			if token.Kind == "literal" {
				file.Literals = append(file.Literals, token.Text)
			}
		}
		for name := range names {
			file.References = append(file.References, name)
		}
		sort.Strings(file.References)
	}
	file.CodeComplete = len(file.Missing) == 0
	if file.CodeComplete {
		file.Operations["declarations"] = Operation{Complete: true}
		file.Operations["references"] = Operation{Complete: true}
	} else {
		file.Operations["declarations"] = Operation{Reason: "解析或具名绑定尚有未完成范围"}
		file.Operations["references"] = Operation{Reason: "声明不完整，引用候选不能证明完整用法"}
	}
	return &syntaxFile{tree: tree, language: language, parents: parents}
}
