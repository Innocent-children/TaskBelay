package javascript

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

/**
 * 只用快照中的语法、扩展名和配置确认模块隔离；未解析的继承配置不当作默认值。
 * 没有模块证据的脚本可能共享全局作用域，调用方必须保留这个边界的未知状态。
 */
func ModuleScope(file string, moduleSyntax, jsx bool, manifest, config []byte) (scope, reason, kind string) {
	ext := path.Ext(file)
	if moduleSyntax || ext == ".mjs" || ext == ".mts" || ext == ".cjs" || ext == ".cts" {
		return "file", "", ""
	}
	var pkg struct {
		Type string `json:"type"`
	}
	if manifest != nil {
		if err := json.Unmarshal(manifest, &pkg); err != nil {
			return "unknown", "package.json 的模块类型无法读取：" + err.Error(), "error"
		}
	}
	if (ext == ".js" || ext == ".jsx") && pkg.Type == "module" {
		return "file", "", ""
	}
	var cfg struct {
		Extends  json.RawMessage `json:"extends"`
		Compiler struct {
			Detection string `json:"moduleDetection"`
			Module    string `json:"module"`
			JSX       string `json:"jsx"`
		} `json:"compilerOptions"`
	}
	if config != nil {
		data, err := configurationJSON(config)
		if err == nil {
			err = json.Unmarshal(data, &cfg)
		}
		if err != nil {
			return "unknown", "编译配置无法读取：" + err.Error(), "error"
		}
	}
	if cfg.Compiler.Detection == "force" {
		return "file", "", ""
	}
	if len(cfg.Extends) > 0 {
		return "unknown", "继承的编译配置尚未完整解析，不能确认模块边界", "unsupported"
	}
	if cfg.Compiler.Detection != "legacy" {
		module := strings.ToLower(cfg.Compiler.Module)
		if (module == "nodenext" || module == "node16" || module == "node18" || module == "node20") && pkg.Type == "module" || jsx && (cfg.Compiler.JSX == "react-jsx" || cfg.Compiler.JSX == "react-jsxdev") {
			return "file", "", ""
		}
	}
	return "unknown", "脚本可能共享全局作用域，尚未确认所有加载入口", "unsupported"
}

func configurationJSON(input []byte) ([]byte, error) {
	data := append([]byte(nil), input...)
	quoted, escaped := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c != '/' || i+1 >= len(data) {
			continue
		}
		if data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				data[i] = ' '
				i++
			}
		} else if data[i+1] == '*' {
			data[i], data[i+1] = ' ', ' '
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				data[i] = ' '
				i++
			}
			if i+1 >= len(data) {
				return nil, fmt.Errorf("注释未结束")
			}
			data[i], data[i+1] = ' ', ' '
			i++
		}
	}
	quoted, escaped = false, false
	for i, c := range data {
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(data) && strings.ContainsRune(" \r\n\t", rune(data[j])) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				data[i] = ' '
			}
		}
	}
	return data, nil
}
