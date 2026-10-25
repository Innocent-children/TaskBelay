package codescan

import (
	"strconv"
	"strings"
)

func javaNumber(text string) (Value, bool) {
	text = strings.ReplaceAll(text, "_", "")
	if text == "" {
		return Value{}, false
	}
	lower := strings.ToLower(text)
	hex := strings.HasPrefix(strings.TrimLeft(lower, "+-"), "0x")
	floating := strings.Contains(lower, ".") || strings.Contains(lower, "p") || (!hex && strings.Contains(lower, "e")) || (!hex && (strings.HasSuffix(lower, "f") || strings.HasSuffix(lower, "d")))
	if floating {
		kind, bits := "double", 64
		if strings.HasSuffix(lower, "f") {
			kind, bits = "float", 32
			text = text[:len(text)-1]
		} else if strings.HasSuffix(lower, "d") {
			text = text[:len(text)-1]
		}
		value, err := strconv.ParseFloat(text, bits)
		return Value{Kind: kind, Text: strconv.FormatFloat(value, 'g', -1, bits)}, err == nil
	}
	kind, bits := "int", 32
	if strings.HasSuffix(lower, "l") {
		kind, bits = "long", 64
		text = text[:len(text)-1]
	}
	unsigned := strings.TrimLeft(text, "+-")
	base := 10
	if len(unsigned) > 1 && unsigned[0] == '0' {
		base = 0
	}
	if base == 10 {
		value, err := strconv.ParseInt(text, 10, bits)
		return Value{Kind: kind, Text: strconv.FormatInt(value, 10)}, err == nil
	}
	value, err := strconv.ParseUint(unsigned, base, bits)
	number := int64(value)
	if bits == 32 {
		number = int64(int32(value))
	}
	if strings.HasPrefix(text, "-") {
		number = -number
		if bits == 32 {
			number = int64(int32(number))
		}
	}
	return Value{Kind: kind, Text: strconv.FormatInt(number, 10)}, err == nil
}

func javaConvertValue(value Value, typ string) Value {
	if value.Kind == "int" || value.Kind == "long" || value.Kind == "byte" || value.Kind == "short" || value.Kind == "float" || value.Kind == "double" {
		switch typ {
		case "int", "long", "byte", "short":
			if value.Kind != "float" && value.Kind != "double" {
				value.Kind = typ
			}
		case "float", "double":
			bits := 64
			if typ == "float" {
				bits = 32
			}
			if number, err := strconv.ParseFloat(value.Text, bits); err == nil {
				value.Kind = typ
				value.Text = strconv.FormatFloat(number, 'g', -1, bits)
			}
		}
	}
	return value
}
