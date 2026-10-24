package hookjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Entry 是要加进配置文件的一个 hook 条目。
type Entry struct {
	Event   string // 配置里的事件名，如 PreToolUse
	Matcher string // 匹配哪些工具，空表示不限
	Handler any    // hook 处理器，编码成 JSON 后放进这个条目的 hooks 列表
}

// group 是配置文件里一个事件下的一个条目。
type group struct {
	Matcher string `json:"matcher,omitempty"`
	Hooks   []any  `json:"hooks"`
}

// EditHooks 修改 path 处的 JSON 配置文件里的 hooks 一项：先删掉 ours 认出的处理器，
// 再加上 entries。只剩 TaskBelay 处理器的条目整条删掉，删空的事件和 hooks 一项也删掉。
// 文件里的其他内容和键的顺序保持原样；文件不存在时新建。path 是符号链接时，修改它指向的文件。
func EditHooks(path string, ours func(handler json.RawMessage) bool, entries []Entry) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	root := Object{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if len(entries) == 0 {
			return nil // 只删不加，文件又不存在，没有要改的
		}
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("读取 %s：%w", path, err)
		}
	}

	hooks := Object{}
	if raw, ok := root.Get("hooks"); ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("读取 %s 的 hooks：%w", path, err)
		}
	}
	kept := Object{}
	for _, m := range hooks {
		groups, err := removeOurs(m.Value, ours)
		if err != nil {
			return fmt.Errorf("读取 %s 的 hooks.%s：%w", path, m.Key, err)
		}
		if groups != nil {
			kept = append(kept, Member{Key: m.Key, Value: groups})
		}
	}
	hooks = kept

	for _, e := range entries {
		var groups []json.RawMessage
		if raw, ok := hooks.Get(e.Event); ok {
			if err := json.Unmarshal(raw, &groups); err != nil {
				return err
			}
		}
		g, err := marshal(group{Matcher: e.Matcher, Hooks: []any{e.Handler}})
		if err != nil {
			return err
		}
		raw, err := marshal(append(groups, g))
		if err != nil {
			return err
		}
		hooks.Set(e.Event, raw)
	}

	if len(hooks) == 0 {
		root.Delete("hooks")
	} else {
		raw, err := marshal(hooks)
		if err != nil {
			return err
		}
		root.Set("hooks", raw)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		return err
	}
	return writeFile(path, out.Bytes())
}

// marshal 编码 v，不把 &、<、> 转义成 \u 形式。Windows 上 Codex 的命令以 & 开头，
// 原样写出来，用户打开配置文件时读得懂。
func marshal(v any) (json.RawMessage, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// removeOurs 从一个事件的条目列表里删掉 TaskBelay 的处理器。没有删到时原样返回；
// 删完一个条目都不剩时返回 nil。
func removeOurs(raw json.RawMessage, ours func(json.RawMessage) bool) (json.RawMessage, error) {
	var groups []json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return nil, err
	}
	changed := false
	var keptGroups []json.RawMessage
	for _, rawGroup := range groups {
		var g Object
		if err := json.Unmarshal(rawGroup, &g); err != nil {
			return nil, err
		}
		var handlers []json.RawMessage
		if h, ok := g.Get("hooks"); ok {
			if err := json.Unmarshal(h, &handlers); err != nil {
				return nil, err
			}
		}
		var keptHandlers []json.RawMessage
		for _, h := range handlers {
			if !ours(h) {
				keptHandlers = append(keptHandlers, h)
			}
		}
		switch {
		case len(keptHandlers) == len(handlers):
			keptGroups = append(keptGroups, rawGroup)
		case len(keptHandlers) > 0:
			h, err := marshal(keptHandlers)
			if err != nil {
				return nil, err
			}
			g.Set("hooks", h)
			rawGroup, err := marshal(g)
			if err != nil {
				return nil, err
			}
			keptGroups = append(keptGroups, rawGroup)
			changed = true
		default:
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	if len(keptGroups) == 0 {
		return nil, nil
	}
	return marshal(keptGroups)
}

// writeFile 先写到同一目录下的临时文件，再改名替换原文件，写到一半出错时原文件不受影响。
// 原文件的权限保持不变。
func writeFile(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".taskbelay-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
