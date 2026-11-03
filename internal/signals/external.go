package signals

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/Innocent-children/taskbelay/internal/units"
)

func external(in Input) []External {
	var out []External
	for _, p := range keys(in.After.Files) {
		f := in.After.Files[p]
		old, exists := in.Before.Files[p]
		cluster, agent := clusterFor(in, p, 1, len(f.Lines))
		if !exists {
			out = append(out, External{Kind: "file", Method: "new_file", Name: p, At: p, Cluster: cluster, Reliable: true, AgentOnly: agent})
		}
		oldTokens := map[string]bool{}
		for _, t := range old.Tokens {
			oldTokens[t.Kind+":"+units.Literal(t)] = true
		}
		role := in.After.Roles[p]
		for i, t := range f.Tokens {
			if oldTokens[t.Kind+":"+units.Literal(t)] {
				continue
			}
			name := units.Literal(t)
			cl, ag := clusterFor(in, p, t.Line, t.Line)
			item := External{Name: name, At: p, Cluster: cl, AgentOnly: ag}
			switch {
			case role.Config && (t.Kind == "key" || t.Kind == "identifier"):
				j := i + 1
				for j < len(f.Tokens) && f.Tokens[j].Kind == "space" {
					j++
				}
				if j >= len(f.Tokens) || (f.Tokens[j].Text != ":" && f.Tokens[j].Text != "=") {
					continue
				}
				item.Kind, item.Method, item.Reliable = "config", "config_key", true
				if j+1 < len(f.Tokens) {
					for _, next := range f.Tokens[j+1:] {
						if next.Kind != "space" {
							item.Value = units.Literal(next)
							break
						}
					}
				}
			case (t.Kind == "key" || t.Kind == "string") && strings.HasPrefix(name, "--"):
				item.Kind, item.Method = "flag", "literal"
			case (t.Kind == "key" || t.Kind == "string") && strings.HasPrefix(name, "/"):
				item.Kind, item.Method = "route", "literal"
			case (t.Kind == "key" || t.Kind == "string") && upperSnake(name):
				item.Kind, item.Method = "env", "literal"
			default:
				continue
			}
			out = append(out, item)
		}
		for _, u := range f.Units {
			cl, ag := clusterFor(in, p, u.Line, u.EndLine)
			if cl == "" {
				continue
			}
			text := strings.Join(units.Sequence(u.Tokens, false), " ")
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
			if in.After.SyntaxFor(p).CallRatio <= CallPrecision {
				continue
			}
			hasSentence := false
			var key, value string
			for _, t := range u.Tokens {
				if t.Kind == "string" && strings.Contains(units.Literal(t), " ") {
					hasSentence = true
				}
				if t.Kind == "key" && !oldTokens[t.Kind+":"+units.Literal(t)] && key == "" {
					key = units.Literal(t)
				} else if key != "" && value == "" && (t.Kind == "key" || t.Kind == "number" || t.Kind == "string") {
					value = units.Literal(t)
				}
			}
			if hasSentence && key != "" && strings.Contains(text, "(") {
				out = append(out, External{Kind: "entry", Method: "registration", Name: key, Value: value, At: p, Cluster: cl, AgentOnly: ag})
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
		item.Anchor, item.Source = anchor(item, in)
		result = append(result, item)
	}
	return result
}

func anchor(item External, in Input) (string, string) {
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
			return "quote", r.ID
		}
		for _, at := range r.Locations {
			if at == item.Kind+":"+item.Name || item.Kind == "file" && at == item.Name || item.Kind == "entry" && strings.HasSuffix(at, ":"+item.Name) {
				return "agent", r.ID
			}
		}
	}
	return "none", ""
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
