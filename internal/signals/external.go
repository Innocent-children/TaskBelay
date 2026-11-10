package signals

import (
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/units"
)

/**
 * referenced 判断新文件是否已接入本次改动的其他代码：其他文件用到了它声明的名字，
 * 或者按文件名引用了它。新文件只可能被本次改动过的文件使用，所以只看改动过的文件。
 */
func referenced(in Input, p string) bool {
	f := in.After.Files[p]
	names := map[string]bool{}
	for _, u := range f.Units {
		if u.Name != "" && declaration(f, u) {
			names[u.Name] = true
		}
	}
	base := path.Base(p)
	for _, other := range keys(in.After.Files) {
		if other == p || in.After.Roles[other].Document || in.After.Roles[other].Test {
			continue
		}
		g := in.After.Files[other]
		for _, t := range g.Tokens {
			if names[units.Literal(t)] {
				return true
			}
		}
		for _, line := range g.Lines {
			if strings.Contains(line, base) {
				return true
			}
		}
	}
	return false
}

func external(in Input) []External {
	var out []External
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		old, exists := in.Before.Files[p]
		cluster, agent := clusterFor(in, p, 1, len(f.Lines))
		role := in.After.Roles[p]
		// 被本次改动的代码引用的新文件是实现的一部分；没有代码引用的文档、脚本、
		// 示例和配置才是独立新增的内容。新文件只可能被本次改动过的文件引用。
		if !exists && !f.Binary && !referenced(in, p) {
			out = append(out, External{Kind: "file", Method: "new_file", Name: p, At: p, Cluster: cluster, Reliable: true, AgentOnly: agent})
		}
		// 文档里提到的名字是说明文字，测试里的字面量是测试数据，都不是新增的入口。
		// 入口形状的字面量只在含函数声明的代码文件里识别；配置和清单另按键和依赖识别。
		code := false
		for _, u := range f.Units {
			code = code || declaration(f, u)
		}
		if role.Document || role.Test || !code && !role.Config && !role.Manifest {
			continue
		}
		oldTokens := map[string]bool{}
		for _, t := range old.Tokens {
			oldTokens[t.Kind+":"+units.Literal(t)] = true
		}
		for i, t := range f.Tokens {
			if oldTokens[t.Kind+":"+units.Literal(t)] {
				continue
			}
			name := units.Literal(t)
			cl, ag := clusterFor(in, p, t.Line, t.Line)
			item := External{Name: name, At: p, Cluster: cl, AgentOnly: ag}
			if !code && !role.Config {
				continue
			}
			switch {
			case role.Config && !code && (t.Kind == "key" || t.Kind == "identifier"):
				j := i + 1
				for j < len(f.Tokens) && f.Tokens[j].Kind == "space" {
					j++
				}
				if j >= len(f.Tokens) || (f.Tokens[j].Text != ":" && f.Tokens[j].Text != "=") {
					continue
				}
				item.Kind, item.Method = "config", "config_key"
				if j+1 < len(f.Tokens) {
					for _, next := range f.Tokens[j+1:] {
						if next.Kind != "space" {
							item.Value = units.Literal(next)
							break
						}
					}
				}
			case (t.Kind == "key" || t.Kind == "string") && strings.HasPrefix(name, "--") && name != "--":
				item.Kind, item.Method = "flag", "literal"
			case (t.Kind == "key" || t.Kind == "string") && strings.HasPrefix(name, "/"):
				item.Kind, item.Method = "route", "literal"
			case (t.Kind == "key" || t.Kind == "string") && upperSnake(name):
				item.Kind, item.Method = "env", "literal"
			default:
				confirmed := false
				for _, value := range in.Confirmed {
					confirmed = confirmed || name == value
				}
				if !confirmed {
					continue
				}
				item.Kind, item.Method = "cmd", "probe"
			}
			for _, value := range in.Confirmed {
				if name == value {
					item.Reliable, item.Method = true, "probe"
				}
			}
			out = append(out, item)
		}
		for _, u := range f.Units {
			cl, ag := clusterFor(in, p, u.Line, u.EndLine)
			if cl == "" {
				continue
			}
			if role.Manifest {
				var name, version string
				for _, t := range u.Tokens {
					if t.Kind == "identifier" || t.Kind == "key" {
						if name == "" {
							name = units.Literal(t)
						}
					}
					if strings.Count(units.Literal(t), ".") > 0 {
						version = units.Literal(t)
					}
				}
				if name != "" && version != "" && !strings.Contains(strings.Join(old.Lines, ""), name) {
					out = append(out, External{Kind: "dep", Method: "manifest", Name: name, Value: version, At: p, Cluster: cl, Reliable: true, AgentOnly: ag})
				}
			}
		}
		if in.After.SyntaxFor(p).CallRatio > CallPrecision {
			literals := map[int]units.Token{}
			for _, t := range f.Tokens {
				if t.Kind == "key" || t.Kind == "string" || t.Kind == "number" {
					literals[t.Start] = t
				}
			}
			calls := units.Calls(f.Tokens)
			for _, name := range keys(calls) {
				for _, args := range calls[name] {
					var key, value string
					line := 0
					hasSentence := false
					for _, arg := range args {
						t, ok := literals[arg.Start]
						if !ok || t.End != arg.End {
							continue
						}
						literal := units.Literal(t)
						hasSentence = hasSentence || t.Kind == "string" && strings.ContainsFunc(literal, unicode.IsSpace)
						if key == "" && t.Kind == "key" && strings.ContainsFunc(literal, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) && !oldTokens[t.Kind+":"+literal] {
							key, line = literal, t.Line
						} else if key != "" && value == "" {
							value = literal
						}
					}
					if hasSentence && key != "" {
						cl, ag := clusterFor(in, p, line, line)
						if cl != "" {
							out = append(out, External{Kind: "entry", Method: "registration", Name: key, Value: value, At: p, Cluster: cl, AgentOnly: ag})
						}
					}
				}
			}
		}
	}
	seen := map[string]bool{}
	var result []External
	for _, item := range out {
		key := item.Kind + ":" + item.At + ":" + item.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		item.Anchor, item.Source, item.Location = anchor(item, in)
		result = append(result, item)
	}
	return result
}

func (item External) Object() string {
	if item.Kind == "file" {
		return item.Kind + ":" + item.Name
	}
	return item.At + ":" + item.Kind + ":" + item.Name
}

func anchor(item External, in Input) (string, string, string) {
	for _, r := range in.Requirements {
		source := r.Quote + " " + r.Plan
		words := map[string]bool{}
		for _, w := range units.Words(source) {
			words[w] = true
		}
		name := units.Words(item.Name)
		all := len(name) > 0
		for _, w := range name {
			all = all && words[w]
		}
		value := strings.TrimSpace(item.Value)
		if all || value != "" && strings.Contains(strings.ToLower(source), strings.ToLower(value)) {
			return "quote", r.ID, ""
		}
		for _, at := range r.Locations {
			if at == item.Kind+":"+item.Name || item.Kind == "file" && at == item.Name || item.Kind == "entry" && strings.HasSuffix(at, ":"+item.Name) {
				return "agent", r.ID, at
			}
		}
	}
	return "none", "", ""
}
func upperSnake(s string) bool {
	if !strings.Contains(s, "_") {
		return false
	}
	letter := false
	for _, r := range s {
		if unicode.IsLetter(r) {
			letter = true
			if !unicode.IsUpper(r) {
				return false
			}
		} else if !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return letter
}
func location(p string, u units.Unit) string {
	if u.Name != "" {
		return p + ":" + u.Name
	}
	return p + ":" + strconv.Itoa(u.Line)
}
