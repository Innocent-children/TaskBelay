// Package rustinputs describes Rust test ranges for isolated evaluation.
package rustinputs

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type Input struct {
	Path string `json:"path"`
	Side string `json:"side"`
	Data []byte `json:"data"`
}

type Span struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Line  int    `json:"line"`
	Last  int    `json:"last_line"`
	Text  string `json:"text"`
}

type Item struct {
	ID      string   `json:"id"`
	Parent  string   `json:"parent"`
	Start   int      `json:"start"`
	End     int      `json:"end"`
	Insert  int      `json:"insert"`
	Test    bool     `json:"test"`
	Docs    []Span   `json:"docs"`
	Control []string `json:"test_control"`
}

type Reference struct {
	Choices []string `json:"choices"`
	Test    bool     `json:"test"`
	Doc     bool     `json:"doc"`
}

type Output struct {
	Path       string      `json:"path"`
	Side       string      `json:"side"`
	Items      []Item      `json:"items"`
	References []Reference `json:"references"`
	Error      string      `json:"error,omitempty"`
}

func compact(value string) string { return strings.Join(strings.Fields(value), "") }

func arguments(value string) []string {
	var result []string
	start, depth, quoted, escaped := 0, 0, false, false
	for index, ch := range value {
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	if value[start:] != "" {
		result = append(result, value[start:])
	}
	return result
}

func onlyTest(expression string) bool {
	if expression == "test" || expression == "doctest" {
		return true
	}
	if strings.HasPrefix(expression, "all(") && strings.HasSuffix(expression, ")") {
		for _, part := range arguments(expression[4 : len(expression)-1]) {
			if onlyTest(part) {
				return true
			}
		}
	}
	if strings.HasPrefix(expression, "any(") && strings.HasSuffix(expression, ")") {
		parts := arguments(expression[4 : len(expression)-1])
		if len(parts) == 0 {
			return false
		}
		for _, part := range parts {
			if !onlyTest(part) {
				return false
			}
		}
		return true
	}
	return false
}

func testAttribute(value string) bool {
	value = compact(value)
	if strings.HasPrefix(value, "#[cfg(") && strings.HasSuffix(value, ")]") {
		return onlyTest(value[6 : len(value)-2])
	}
	for _, name := range []string{"test", "tokio::test", "rstest", "rstest::rstest"} {
		if value == "#["+name+"]" || strings.HasPrefix(value, "#["+name+"(") {
			return true
		}
	}
	return false
}

func docAttribute(value string) bool {
	value = compact(value)
	return strings.HasPrefix(value, "#[doc=") || strings.HasPrefix(value, "#[doc(") || strings.HasPrefix(value, "#![doc")
}

func docComment(value string) bool {
	return strings.HasPrefix(value, "///") && !strings.HasPrefix(value, "////") || strings.HasPrefix(value, "//!") || strings.HasPrefix(value, "/**") && !strings.HasPrefix(value, "/***") || strings.HasPrefix(value, "/*!")
}

func containsTestAttribute(value string) bool {
	value = compact(value)
	for _, name := range []string{"test", "tokio::test", "rstest", "rstest::rstest"} {
		if strings.Contains(value, "#["+name+"]") || strings.Contains(value, "#["+name+"(") {
			return true
		}
	}
	return false
}

func literal(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "r") {
		quote := strings.IndexByte(value, '"')
		if quote < 1 {
			return "", fmt.Errorf("unsupported Rust path literal")
		}
		hashes := value[1:quote]
		if strings.Trim(hashes, "#") != "" || !strings.HasSuffix(value, "\""+hashes) {
			return "", fmt.Errorf("unsupported Rust path literal")
		}
		return value[quote+1 : len(value)-len(hashes)-1], nil
	}
	return strconv.Unquote(value)
}

func Describe(source Input) Output {
	result := Output{Path: source.Path, Side: source.Side, Items: []Item{}, References: []Reference{}}
	entry := grammars.DetectLanguageByName("rust")
	language := entry.Language()
	parser := ts.NewParser(language)
	parser.SetTimeoutMicros(5000000)
	var tree *ts.Tree
	var err error
	if entry.TokenSourceFactory != nil {
		tree, err = parser.ParseWithTokenSourceStrict(source.Data, entry.TokenSourceFactory(source.Data, language))
	} else {
		tree, err = parser.ParseStrict(source.Data)
	}
	if tree != nil {
		defer tree.Release()
	}
	if err != nil || tree == nil || tree.RootNode() == nil || tree.RootNode().HasErrorOrMissing() {
		result.Error = fmt.Sprintf("incomplete Rust syntax: %v", err)
		return result
	}
	text := func(node *ts.Node) string {
		if node == nil {
			return ""
		}
		return node.Text(source.Data)
	}
	field := func(node *ts.Node, name string) string { return text(node.ChildByFieldName(name, language)) }
	rangeOf := func(node *ts.Node) Span {
		return Span{int(node.StartByte()), int(node.EndByte()), int(node.StartPoint().Row) + 1, int(node.EndPoint().Row) + 1, text(node)}
	}
	docReference := func(value string, inTest bool) {
		if !docAttribute(value) || !strings.Contains(value, "include_str!") {
			return
		}
		left := strings.Index(value, "include_str!(") + len("include_str!(")
		right := strings.LastIndex(value, ")")
		if left < len("include_str!(") || right < left {
			result.Error = "unverified included documentation"
			return
		}
		filename, e := literal(value[left:right])
		if e != nil {
			result.Error = e.Error()
			return
		}
		result.References = append(result.References, Reference{Choices: []string{path.Join(path.Dir(source.Path), filename)}, Test: inTest, Doc: true})
	}
	result.Items = append(result.Items, Item{ID: "$", Start: 0, End: len(source.Data), Insert: len(source.Data), Docs: []Span{}, Control: []string{}})
	var walk func(*ts.Node, string, []string, bool)
	walk = func(parent *ts.Node, owner string, modules []string, inheritedTest bool) {
		var attributes []*ts.Node
		docs := []Span{}
		for n := 0; n < parent.ChildCount(); n++ {
			node := parent.Child(n)
			kind := node.Type(language)
			value := text(node)
			if kind == "attribute_item" {
				attributes = append(attributes, node)
				if docAttribute(value) {
					docs = append(docs, rangeOf(node))
				}
				continue
			}
			if kind == "inner_attribute_item" || strings.HasPrefix(value, "//!") || strings.HasPrefix(value, "/*!") {
				if testAttribute(strings.Replace(value, "#![", "#[", 1)) {
					inheritedTest = true
					for i := range result.Items {
						if result.Items[i].ID == owner {
							result.Items[i].Test = true
						}
					}
				}
				docReference(value, inheritedTest)
				if docComment(value) || docAttribute(value) {
					for i := range result.Items {
						if result.Items[i].ID == owner {
							result.Items[i].Docs = append(result.Items[i].Docs, rangeOf(node))
						}
					}
				}
				if strings.Contains(value, "test") && !docAttribute(value) {
					for i := range result.Items {
						if result.Items[i].ID == owner {
							result.Items[i].Control = append(result.Items[i].Control, compact(value))
						}
					}
				}
				continue
			}
			if strings.Contains(kind, "comment") {
				if docComment(value) {
					docs = append(docs, rangeOf(node))
				}
				continue
			}
			if !node.IsNamed() {
				continue
			}
			name := field(node, "name")
			identified := strings.HasSuffix(kind, "_item") || kind == "field_declaration" || kind == "enum_variant" || kind == "macro_definition" || kind == "associated_type" || kind == "use_declaration"
			if kind == "use_declaration" {
				name = compact(value)
			}
			if kind == "impl_item" {
				name = compact(field(node, "trait")) + " for " + compact(field(node, "type"))
			}
			if identified && name == "" {
				identified = false
			}
			id, inTest, nextModules := owner, inheritedTest, modules
			if identified {
				id = owner + "/" + kind + ":" + name
				for _, attr := range attributes {
					value := compact(text(attr))
					if strings.HasPrefix(value, "#[cfg(") && !testAttribute(value) {
						id += "@" + value
					}
				}
				start := int(node.StartByte())
				if len(attributes) > 0 {
					start = int(attributes[0].StartByte())
				}
				if len(docs) > 0 && docs[0].Start < start {
					start = docs[0].Start
				}
				row := Item{ID: id, Parent: owner, Start: start, End: int(node.EndByte()), Insert: -1, Docs: docs, Control: []string{}}
				if kind == "macro_definition" && containsTestAttribute(value) {
					row.Control = append(row.Control, value)
				}
				if body := node.ChildByFieldName("body", language); body != nil {
					row.Insert = int(body.EndByte()) - 1
				}
				for _, attr := range attributes {
					a := text(attr)
					if testAttribute(a) {
						row.Test = true
						inTest = true
					}
					if !row.Test && strings.HasPrefix(compact(a), "#[cfg") && strings.Contains(a, "test") {
						row.Control = append(row.Control, compact(a))
					}
				}
				result.Items = append(result.Items, row)
				if kind == "mod_item" {
					nextModules = append(append([]string{}, modules...), name)
				}
				if kind == "mod_item" && node.ChildByFieldName("body", language) == nil {
					base := path.Dir(source.Path)
					stem := strings.TrimSuffix(path.Base(source.Path), ".rs")
					if stem != "lib" && stem != "main" && stem != "mod" {
						base = path.Join(base, stem)
					}
					for _, module := range modules {
						base = path.Join(base, module)
					}
					choices := []string{path.Join(base, name+".rs"), path.Join(base, name, "mod.rs")}
					for _, attr := range attributes {
						a := strings.TrimSpace(text(attr))
						if strings.HasPrefix(compact(a), "#[path=") {
							if len(modules) > 0 {
								result.Error = "inline module with explicit external path is unverified"
								continue
							}
							part := a[strings.IndexByte(a, '=')+1 : len(a)-1]
							filename, e := literal(part)
							if e != nil {
								result.Error = e.Error()
							} else {
								choices = []string{path.Join(path.Dir(source.Path), filename)}
							}
						}
					}
					result.References = append(result.References, Reference{Choices: choices, Test: inTest})
				}
			} else if len(docs) > 0 {
				result.Error = fmt.Sprintf("documentation belongs to %s at line %d: %.100s", kind, node.StartPoint().Row+1, value)
			}
			if kind == "macro_invocation" {
				if containsTestAttribute(value) {
					for i := range result.Items {
						if result.Items[i].ID == owner {
							result.Items[i].Control = append(result.Items[i].Control, value)
						}
					}
				}
				macro := field(node, "macro")
				if macro == "include" || macro == "include_str" || macro == "include_bytes" {
					for i := 0; i < node.ChildCount(); i++ {
						arguments := node.Child(i)
						if arguments.Type(language) != "token_tree" {
							continue
						}
						value := text(arguments)
						filename, e := literal(value[1 : len(value)-1])
						if e != nil {
							if inTest {
								result.Error = "dynamic included test input"
							}
							result.References = append(result.References, Reference{Choices: []string{}, Test: inTest})
							continue
						}
						result.References = append(result.References, Reference{Choices: []string{path.Join(path.Dir(source.Path), filename)}, Test: inTest})
					}
				}
			}
			for _, attr := range attributes {
				docReference(text(attr), inTest)
			}
			attributes = nil
			docs = []Span{}
			walk(node, id, nextModules, inTest)
		}
		if len(docs) > 0 {
			result.Error = "unattached documentation"
		}
	}
	walk(tree.RootNode(), "$", nil, false)
	return result
}
