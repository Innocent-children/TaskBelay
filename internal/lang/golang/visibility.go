package golang

import (
	"unicode"
	"unicode/utf8"
)

func VisibilityScope(node, owner, name string) string {
	first, _ := utf8.DecodeRuneInString(name)
	if owner == "" && !unicode.IsUpper(first) {
		switch node {
		case "function_declaration", "type_spec", "type_alias", "const_spec", "var_spec":
			return "package"
		}
	}
	return "language"
}
