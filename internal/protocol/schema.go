package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func Decode(raw []byte, out any) error {
	if len(raw) > 1024*1024 {
		return domain.Invalid("arguments", "request exceeds 1 MiB")
	}
	if !utf8.Valid(raw) {
		return domain.Invalid("arguments", "request must be UTF-8")
	}
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return domain.Invalid("arguments", "request must be one JSON object")
	}
	tokens := json.NewDecoder(bytes.NewReader(raw))
	tokens.UseNumber()
	if e := walkJSON(tokens, "arguments", Schema(out)); e != nil {
		return e
	}
	if _, e := tokens.Token(); e != io.EOF {
		return domain.Invalid("arguments", "trailing JSON is not allowed")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		var mismatch *json.UnmarshalTypeError
		if errors.As(e, &mismatch) {
			return domain.Invalid(mismatch.Field, "requires "+mismatch.Type.String())
		}
		return domain.Invalid("arguments", "request contains an unknown member or invalid value")
	}
	return nil
}
func walkJSON(d *json.Decoder, p string, schema map[string]any) error {
	if choices, ok := schema["anyOf"].([]any); ok && len(choices) > 0 {
		schema, _ = choices[0].(map[string]any)
	}
	t, e := d.Token()
	if e != nil {
		return domain.Invalid(p, "malformed JSON")
	}
	delim, ok := t.(json.Delim)
	if t != nil {
		kind := ""
		switch t.(type) {
		case string:
			kind = "string"
		case bool:
			kind = "boolean"
		case json.Number:
			kind = "integer"
		}
		if ok {
			if delim == '{' {
				kind = "object"
			} else if delim == '[' {
				kind = "array"
			}
		}
		if required, has := schema["type"].(string); has && required != kind {
			return domain.Invalid(p, "requires "+required)
		}
	}
	if !ok {
		return nil
	}
	if delim == '{' {
		properties, _ := schema["properties"].(map[string]any)
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return domain.Invalid(p, "malformed object")
			}
			k := key.(string)
			if seen[k] {
				return domain.Invalid(p+"."+k, "duplicate JSON member")
			}
			seen[k] = true
			child, known := properties[k].(map[string]any)
			if !known && schema["additionalProperties"] == false {
				return domain.Invalid(p+"."+k, "member is not allowed by the current request")
			}
			if e = walkJSON(d, p+"."+k, child); e != nil {
				return e
			}
		}
	} else if delim == '[' {
		item, _ := schema["items"].(map[string]any)
		i := 0
		for d.More() {
			if e = walkJSON(d, fmt.Sprintf("%s[%d]", p, i), item); e != nil {
				return e
			}
			i++
		}
	}
	_, e = d.Token()
	if e != nil {
		return domain.Invalid(p, "malformed JSON")
	}
	return nil
}

/**
 * Schemas derive from the same DTOs as every transport, with closed object members.
 */
func Schema(v any) map[string]any { return typeSchema(reflect.TypeOf(v)) }
func typeSchema(t reflect.Type) map[string]any {
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeFor[domain.Digest]() {
		return map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
	}
	if t.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{typeSchema(t.Elem()), map[string]any{"type": "null"}}}
	}
	switch t.Kind() {
	case reflect.Struct:
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				x := typeSchema(f.Type)
				for k, v := range x["properties"].(map[string]any) {
					props[k] = v
				}
				required = append(required, x["required"].([]string)...)
				continue
			}
			tag := f.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "-" || f.PkgPath != "" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fieldSchema := typeSchema(f.Type)
			if description := f.Tag.Get("description"); description != "" {
				fieldSchema["description"] = description
			}
			if values := f.Tag.Get("enum"); values != "" {
				fieldSchema["enum"] = strings.Split(values, ",")
			}
			props[name] = fieldSchema
			if !strings.Contains(tag, "omitempty") {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
	case reflect.Slice:
		return map[string]any{"type": []string{"array", "null"}, "items": typeSchema(t.Elem())}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Uint64, reflect.Int32:
		return map[string]any{"type": "integer"}
	case reflect.Map:
		return map[string]any{"type": "object"}
	default:
		return map[string]any{}
	}
}
func OutputSchema(v any) map[string]any {
	base := map[string]any{"request_id": map[string]any{"type": "string"}, "tool": map[string]any{"type": "string"}}
	success := map[string]any{}
	failure := map[string]any{}
	for k, x := range base {
		success[k] = x
		failure[k] = x
	}
	success["ok"] = map[string]any{"const": true}
	success["result"] = Schema(v)
	failure["ok"] = map[string]any{"const": false}
	failure["error"] = Schema(domain.Error{})
	failure["recovery"] = Schema(Recovery{})
	return map[string]any{"type": "object", "oneOf": []any{map[string]any{"properties": success, "required": []string{"ok", "request_id", "tool", "result"}, "additionalProperties": false}, map[string]any{"properties": failure, "required": []string{"ok", "request_id", "tool", "error", "recovery"}, "additionalProperties": false}}}
}
