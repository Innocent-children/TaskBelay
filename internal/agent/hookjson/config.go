package hookjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Field struct {
	Name  string
	Value json.RawMessage
}
type Object []Field

func ParseObject(data []byte) (Object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("配置必须为 JSON 对象")
	}
	var result Object
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name := token.(string)
		if seen[name] {
			return nil, fmt.Errorf("配置包含重复键 %q", name)
		}
		seen[name] = true
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		result = append(result, Field{name, value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("配置包含多余内容")
	}
	return result, nil
}

func (o Object) Get(name string) json.RawMessage {
	for _, field := range o {
		if field.Name == name {
			return field.Value
		}
	}
	return nil
}

func (o *Object) Set(name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	for i := range *o {
		if (*o)[i].Name == name {
			(*o)[i].Value = data
			return nil
		}
	}
	*o = append(*o, Field{name, data})
	return nil
}

func (o Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, field := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(field.Name)
		b.Write(key)
		b.WriteByte(':')
		b.Write(field.Value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type Group struct {
	Event, Matcher string
	Handler        any
}

/**
 * 只移除能确认属于 TaskBelay 的处理器。组中其他处理器、未知字段及键顺序
 * 原样保留；不把同一组或同一事件下的用户 hook 一并替换。
 */
func Merge(data []byte, additions []Group, owned func(Object) bool) ([]byte, error) {
	if len(data) == 0 {
		data = []byte("{}")
	}
	root, err := ParseObject(data)
	if err != nil {
		return nil, err
	}
	var hooks Object
	if raw := root.Get("hooks"); raw != nil {
		hooks, err = ParseObject(raw)
		if err != nil {
			return nil, err
		}
	}
	changed := false
	for i := range hooks {
		var groups []json.RawMessage
		if err := json.Unmarshal(hooks[i].Value, &groups); err != nil || groups == nil {
			return nil, fmt.Errorf("%s hook 配置必须是数组", hooks[i].Name)
		}
		var kept []json.RawMessage
		for _, raw := range groups {
			group, err := ParseObject(raw)
			if err != nil {
				return nil, err
			}
			var handlers []json.RawMessage
			if err := json.Unmarshal(group.Get("hooks"), &handlers); err != nil || handlers == nil {
				return nil, fmt.Errorf("hook 组缺少处理器数组")
			}
			remaining := make([]json.RawMessage, 0, len(handlers))
			removed := false
			for _, rawHandler := range handlers {
				handler, err := ParseObject(rawHandler)
				if err != nil {
					return nil, err
				}
				if owned(handler) {
					removed, changed = true, true
				} else {
					remaining = append(remaining, rawHandler)
				}
			}
			if !removed {
				kept = append(kept, raw)
				continue
			}
			unknown := false
			for _, field := range group {
				if field.Name != "hooks" && field.Name != "matcher" {
					unknown = true
				}
			}
			if len(remaining) == 0 && !unknown {
				continue
			}
			if err := group.Set("hooks", remaining); err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(group)
			if err != nil {
				return nil, err
			}
			kept = append(kept, encoded)
		}
		if kept == nil {
			kept = []json.RawMessage{}
		}
		hooks[i].Value, err = json.Marshal(kept)
		if err != nil {
			return nil, err
		}
	}
	for _, add := range additions {
		var groups []json.RawMessage
		if raw := hooks.Get(add.Event); raw != nil {
			if err := json.Unmarshal(raw, &groups); err != nil {
				return nil, err
			}
		}
		var group Object
		if add.Matcher != "" {
			if err := group.Set("matcher", add.Matcher); err != nil {
				return nil, err
			}
		}
		if err := group.Set("hooks", []any{add.Handler}); err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(group)
		if err != nil {
			return nil, err
		}
		if err := hooks.Set(add.Event, append(groups, encoded)); err != nil {
			return nil, err
		}
		changed = true
	}
	if !changed {
		return data, nil
	}
	if err := root.Set("hooks", hooks); err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

type File struct {
	Path   string
	Before []byte
	Mode   os.FileMode
}

func Load(path string) (File, error) {
	f := File{Path: path, Mode: 0o600}
	if _, err := os.Lstat(path); err == nil {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return f, err
		}
		f.Path = resolved
	} else if !os.IsNotExist(err) {
		return f, err
	}
	info, err := os.Stat(f.Path)
	if os.IsNotExist(err) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if !info.Mode().IsRegular() {
		return f, fmt.Errorf("配置不是普通文件")
	}
	f.Mode = info.Mode().Perm()
	f.Before, err = os.ReadFile(f.Path)
	return f, err
}

func (f File) Write(data []byte) error {
	if bytes.Equal(f.Before, data) {
		return nil
	}
	current, err := os.ReadFile(f.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(current, f.Before) {
		return fmt.Errorf("配置在读取后被修改，请重试")
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(f.Path), ".taskbelay-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(f.Mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}
