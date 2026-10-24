// Package hookjson 是两个 Agent 适配器共用的 JSON 读写：hook 配置文件的修改，
// 以及 hook 输出的编码。Claude Code 和 Codex 的 hook 配置结构相同，输出字段也相同。
package hookjson

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Object 是保持键顺序的 JSON 对象。修改用户的配置文件时用它，没改动的部分原样写回，
// 键的顺序也不变。
type Object []Member

// Member 是 Object 里的一个键和它的值。
type Member struct {
	Key   string
	Value json.RawMessage
}

// UnmarshalJSON 按原来的顺序读出对象的每个键。
func (o *Object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return errors.New("不是 JSON 对象")
	}
	members := Object{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		members = append(members, Member{Key: tok.(string), Value: value})
	}
	*o = members
	return nil
}

// MarshalJSON 按顺序写出每个键。
func (o Object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		key, err := json.Marshal(m.Key)
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(m.Value)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// Get 返回 key 的值。
func (o Object) Get(key string) (json.RawMessage, bool) {
	for _, m := range o {
		if m.Key == key {
			return m.Value, true
		}
	}
	return nil, false
}

// Set 设置 key 的值：已有这个键时原地替换，没有时加在最后。
func (o *Object) Set(key string, value json.RawMessage) {
	for i, m := range *o {
		if m.Key == key {
			(*o)[i].Value = value
			return
		}
	}
	*o = append(*o, Member{Key: key, Value: value})
}

// Delete 删除 key。
func (o *Object) Delete(key string) {
	kept := (*o)[:0]
	for _, m := range *o {
		if m.Key != key {
			kept = append(kept, m)
		}
	}
	*o = kept
}
