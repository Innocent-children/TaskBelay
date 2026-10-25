package codescan

import (
	"go/constant"
	"go/token"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
)

func (p *jsFile) literal(n *ts.Node, depth int) (Value, bool) {
	if n == nil || depth > 40 || p.engine.ctx.Err() != nil {
		return Value{}, false
	}
	switch p.kind(n) {
	case "string", "template_string":
		if hasInterpolation(n, p.syntax.language) {
			return Value{}, false
		}
		value, ok := javaScriptString(p.text(n))
		return Value{Kind: "String", Text: value}, ok
	case "true", "false":
		return Value{Kind: "Bool", Text: p.kind(n)}, true
	case "undefined":
		if b, bound := p.lookup("undefined", n); bound {
			if b != nil {
				return b.file.literalBinding(b, depth+1)
			}
			return Value{}, false
		}
		return Value{Kind: "undefined", Text: "undefined"}, true
	case "null":
		return Value{Kind: p.kind(n), Text: p.kind(n)}, true
	case "number":
		text := strings.ReplaceAll(p.text(n), "_", "")
		bigint := strings.HasSuffix(text, "n")
		text = strings.TrimSuffix(text, "n")
		typ := token.INT
		if strings.ContainsAny(text, ".eE") && !strings.HasPrefix(strings.ToLower(text), "0x") {
			typ = token.FLOAT
		}
		number := constant.MakeFromLiteral(text, typ, 0)
		if number.Kind() != constant.Unknown {
			if bigint {
				return Value{Kind: "BigInt", Text: number.ExactString()}, number.Kind() == constant.Int
			}
			value, _ := constant.Float64Val(number)
			if !math.IsInf(value, 0) {
				return Value{Kind: "Number", Text: strconv.FormatFloat(value, 'g', -1, 64)}, true
			}
		}
	case "unary_expression":
		operator := p.text(p.field(n, "operator"))
		if operator != "+" && operator != "-" {
			break
		}
		value, ok := p.literal(p.field(n, "argument"), depth+1)
		if !ok || (value.Kind != "Number" && value.Kind != "BigInt") || (operator == "+" && value.Kind == "BigInt") {
			break
		}
		if operator == "-" && !(value.Kind == "BigInt" && value.Text == "0") {
			if strings.HasPrefix(value.Text, "-") {
				value.Text = strings.TrimPrefix(value.Text, "-")
			} else {
				value.Text = "-" + value.Text
			}
		}
		return value, true
	case "identifier":
		if b, bound := p.lookup(p.text(n), n); bound && b != nil {
			return b.file.literalBinding(b, depth+1)
		}
	case "member_expression":
		base := p.reference(p.field(n, "object"), depth+1).qualified
		key := base + "." + p.propertyName(p.field(n, "property"))
		if exported := p.engine.exports[key]; len(exported) == 1 {
			return exported[0].file.literalBinding(exported[0], depth+1)
		}
	case "binary_expression":
		if p.text(p.field(n, "operator")) != "+" {
			break
		}
		left, lok := p.literal(p.field(n, "left"), depth+1)
		right, rok := p.literal(p.field(n, "right"), depth+1)
		if lok && rok && left.Kind == "String" && right.Kind == "String" {
			return Value{Kind: "String", Text: left.Text + right.Text}, true
		}
	case "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression":
		if n.NamedChildCount() > 0 {
			return p.literal(n.NamedChild(0), depth+1)
		}
	}
	return Value{}, false
}

func (p *jsFile) literalBinding(b *jsBinding, depth int) (Value, bool) {
	if depth > 40 || p.engine.ctx.Err() != nil {
		return Value{}, false
	}
	if b.constant && b.scope.kind == "program" && b.member == "" {
		return b.file.literal(b.value, depth+1)
	}
	qualified := b.qualified
	if b.member != "" {
		qualified = b.file.reference(b.value, depth+1).qualified + "." + b.member
	}
	if exported := p.engine.exports[qualified]; len(exported) == 1 {
		return exported[0].file.literalBinding(exported[0], depth+1)
	}
	return Value{}, false
}

func javaScriptString(text string) (string, bool) {
	if len(text) < 2 || !strings.ContainsRune("\"'`", rune(text[0])) || text[len(text)-1] != text[0] {
		return "", false
	}
	body := text[1 : len(text)-1]
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
		escape := body[0]
		body = body[1:]
		switch escape {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'v':
			out.WriteByte('\v')
		case '\n':
		case '\r':
			body = strings.TrimPrefix(body, "\n")
		case '0':
			if len(body) > 0 && body[0] >= '0' && body[0] <= '9' {
				return "", false
			}
			out.WriteByte(0)
		case 'x', 'u':
			count := 2
			if escape == 'u' {
				count = 4
			}
			braced := escape == 'u' && strings.HasPrefix(body, "{")
			if braced {
				end := strings.IndexByte(body, '}')
				if end < 2 {
					return "", false
				}
				body = body[1:]
				count = end - 1
			}
			if len(body) < count {
				return "", false
			}
			number, err := strconv.ParseUint(body[:count], 16, 32)
			if err != nil || number > utf8.MaxRune {
				return "", false
			}
			body = body[count:]
			if braced {
				body = body[1:]
			}
			r := rune(number)
			if utf16.IsSurrogate(r) {
				if !strings.HasPrefix(body, "\\u") || len(body) < 6 {
					return "", false
				}
				second, err := strconv.ParseUint(body[2:6], 16, 16)
				if err != nil {
					return "", false
				}
				r = utf16.DecodeRune(r, rune(second))
				if r == utf8.RuneError {
					return "", false
				}
				body = body[6:]
			}
			out.WriteRune(r)
		default:
			if escape >= '1' && escape <= '9' {
				return "", false
			}
			out.WriteByte(escape)
		}
	}
	return out.String(), true
}
