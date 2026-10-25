package codescan

import (
	"go/constant"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *pyFile) literal(n *ts.Node, depth int) (Value, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return Value{}, false
	}
	switch p.kind(n) {
	case "string":
		if hasInterpolation(n, p.syntax.language) {
			return Value{}, false
		}
		text, ok := pythonString(p.text(n))
		return Value{Kind: "String", Text: text}, ok
	case "concatenated_string":
		var value strings.Builder
		for i := 0; i < n.NamedChildCount(); i++ {
			part, ok := p.literal(n.NamedChild(i), depth+1)
			if !ok || part.Kind != "String" {
				return Value{}, false
			}
			value.WriteString(part.Text)
		}
		return Value{Kind: "String", Text: value.String()}, true
	case "integer", "float":
		kind := token.INT
		if p.kind(n) == "float" {
			kind = token.FLOAT
		}
		value := constant.MakeFromLiteral(strings.ReplaceAll(p.text(n), "_", ""), kind, 0)
		if value.Kind() == constant.Unknown {
			return Value{}, false
		}
		return Value{Kind: value.Kind().String(), Text: value.ExactString()}, true
	case "true":
		return Value{Kind: "Bool", Text: "true"}, true
	case "false":
		return Value{Kind: "Bool", Text: "false"}, true
	case "none":
		return Value{Kind: "nil", Text: "None"}, true
	case "identifier":
		if b, ok := p.lookup(p.text(n), n); ok && b != nil && b.scope.kind == "module" && b.expression != nil {
			return p.literal(b.expression, depth+1)
		}
		fallthrough
	case "attribute":
		qualified := p.reference(n, 0).qualified
		if declarations := p.engine.byName[qualified]; len(declarations) == 1 {
			d := declarations[0]
			if d.d.Owner == "" && d.p.kind(d.n) == "assignment" && len(d.d.Segments) == 1 {
				return d.p.literal(d.p.field(d.n, "right"), depth+1)
			}
		}
	case "parenthesized_expression":
		if n.NamedChildCount() == 1 {
			return p.literal(n.NamedChild(0), depth+1)
		}
	case "unary_operator":
		value, ok := p.literal(p.field(n, "argument"), depth+1)
		op := p.text(p.field(n, "operator"))
		if ok && (value.Kind == "Int" || value.Kind == "Float") && (op == "-" || op == "+") {
			if op == "-" {
				if strings.HasPrefix(value.Text, "-") {
					value.Text = strings.TrimPrefix(value.Text, "-")
				} else {
					value.Text = "-" + value.Text
				}
			}
			return value, true
		}
	case "binary_operator":
		left, okA := p.literal(p.field(n, "left"), depth+1)
		right, okB := p.literal(p.field(n, "right"), depth+1)
		if okA && okB && left.Kind == "String" && right.Kind == "String" && p.text(p.field(n, "operator")) == "+" {
			return Value{Kind: "String", Text: left.Text + right.Text}, true
		}
	}
	return Value{}, false
}

func pythonString(text string) (string, bool) {
	quote := strings.IndexAny(text, "\"'")
	if quote < 0 {
		return "", false
	}
	prefix := strings.ToLower(text[:quote])
	if strings.ContainsAny(prefix, "bf") {
		return "", false
	}
	delimiter := text[quote : quote+1]
	if strings.HasPrefix(text[quote:], strings.Repeat(delimiter, 3)) {
		delimiter = strings.Repeat(delimiter, 3)
	}
	if len(text) < quote+len(delimiter)*2 || !strings.HasSuffix(text, delimiter) {
		return "", false
	}
	body := text[quote+len(delimiter) : len(text)-len(delimiter)]
	if strings.Contains(prefix, "r") {
		return body, true
	}
	var out strings.Builder
	for len(body) > 0 {
		if body[0] != '\\' {
			r, size := utf8.DecodeRuneInString(body)
			out.WriteRune(r)
			body = body[size:]
			continue
		}
		body = body[1:]
		if body == "" {
			return "", false
		}
		escaped := body[0]
		body = body[1:]
		switch escaped {
		case '\\', '\'', '"':
			out.WriteByte(escaped)
		case 'a':
			out.WriteByte('\a')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'v':
			out.WriteByte('\v')
		case '\n':
		case '\r':
			body = strings.TrimPrefix(body, "\n")
		case 'x', 'u', 'U':
			count := 2
			if escaped == 'u' {
				count = 4
			} else if escaped == 'U' {
				count = 8
			}
			if len(body) < count {
				return "", false
			}
			value, err := strconv.ParseUint(body[:count], 16, 32)
			if err != nil || value > utf8.MaxRune || (value >= 0xD800 && value <= 0xDFFF) {
				return "", false
			}
			out.WriteRune(rune(value))
			body = body[count:]
		case 'N':
			return "", false
		default:
			if escaped >= '0' && escaped <= '7' {
				digits := string(escaped)
				for len(digits) < 3 && len(body) > 0 && body[0] >= '0' && body[0] <= '7' {
					digits += body[:1]
					body = body[1:]
				}
				value, _ := strconv.ParseUint(digits, 8, 32)
				out.WriteRune(rune(value))
			} else {
				out.WriteByte('\\')
				out.WriteByte(escaped)
			}
		}
	}
	return out.String(), true
}
