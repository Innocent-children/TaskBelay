package codescan

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func rustString(text string) (string, bool) {
	if strings.HasPrefix(text, "r") {
		start := strings.IndexByte(text, '"')
		if start < 1 {
			return "", false
		}
		hashes := text[1:start]
		if strings.Trim(hashes, "#") != "" {
			return "", false
		}
		end := "\"" + hashes
		if !strings.HasSuffix(text, end) || len(text) < start+1+len(end) {
			return "", false
		}
		return text[start+1 : len(text)-len(end)], true
	}
	if len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"' {
		return "", false
	}
	text = text[1 : len(text)-1]
	var out strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			out.WriteByte(text[i])
			continue
		}
		i++
		if i >= len(text) {
			return "", false
		}
		switch text[i] {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case '0':
			out.WriteByte(0)
		case '\\', '\'', '"':
			out.WriteByte(text[i])
		case '\n', '\r':
			for i+1 < len(text) && strings.ContainsRune(" \t\r\n", rune(text[i+1])) {
				i++
			}
		case 'x':
			if i+2 >= len(text) {
				return "", false
			}
			value, err := strconv.ParseUint(text[i+1:i+3], 16, 8)
			if err != nil || value > 127 {
				return "", false
			}
			out.WriteByte(byte(value))
			i += 2
		case 'u':
			if i+1 >= len(text) || text[i+1] != '{' {
				return "", false
			}
			end := strings.IndexByte(text[i+2:], '}')
			if end < 0 {
				return "", false
			}
			end += i + 2
			digits := strings.ReplaceAll(text[i+2:end], "_", "")
			if len(digits) == 0 || len(digits) > 6 {
				return "", false
			}
			value, err := strconv.ParseUint(digits, 16, 32)
			if err != nil || !utf8.ValidRune(rune(value)) {
				return "", false
			}
			out.WriteRune(rune(value))
			i = end
		default:
			return "", false
		}
	}
	return out.String(), utf8.ValidString(out.String())
}
