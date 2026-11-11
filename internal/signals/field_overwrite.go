package signals

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/units"
)

/**
 * 仅在新绑定、条件和第一条写入紧邻时核对字段。跨语句追踪可能把已经使用或泄露的对象算作未读。
 */
func fieldOverwrites(ctx context.Context, f units.File, lines map[int][]units.Token, changed []changes.Hunk, clusters map[string]changes.Cluster) []Candidate {
	var out []Candidate
	for _, u := range f.Units {
		if ctx.Err() != nil {
			break
		}
		record := u.Tokens
		if len(record) < 5 || record[0].Kind != "identifier" || record[1].Text != ":=" || record[2].Kind != "identifier" || record[3].Text != "{" {
			continue
		}
		last := record[len(record)-1]
		line := lines[last.Line]
		if last.Text != "}" || line[len(line)-1].End != last.End || lines[record[0].Line][0].Start != record[0].Start {
			continue
		}
		object := record[0].Text
		guard := lines[last.Line+1]
		write := lines[last.Line+2]
		if !independentGuard(guard, object) || len(write) < 5 || write[0].Text != object || write[1].Text != "." || write[2].Kind != "identifier" || write[3].Text != "=" {
			continue
		}
		lineNo := write[0].Line
		h := slices.IndexFunc(changed, func(h changes.Hunk) bool { return h.Span.AfterStart < lineNo && lineNo <= h.Span.AfterEnd })
		if h < 0 {
			continue
		}
		rhs := write[4:]
		end := rhs[len(rhs)-1]
		if end.Kind == "symbol" && end.Text != ")" && end.Text != "]" && end.Text != "}" {
			continue
		}
		if slices.ContainsFunc(rhs, func(t units.Token) bool {
			return t.Text == object || t.Text == ";" || strings.ContainsAny(t.Text, "\r\n")
		}) || recoveryCalls(rhs) == nil {
			continue
		}
		field := write[2].Text
		value := initialCall(record[4:], field)
		if len(value) == 0 {
			continue
		}
		cluster := clusters[changed[h].ID]
		out = append(out, Candidate{
			Kind: "field_overwrite", Object: units.Hash(f.Path, u.ID, field), At: fmt.Sprintf("%s:%d", f.Path, lineNo),
			Cluster: cluster.ID, AgentOnly: changes.ByAgent(cluster.Origin), ReviewOnly: true, Benefit: 1,
			Facts:  []string{fmt.Sprintf("第 %d 行为 %s.%s 计算初值「%s」。进入「%s」后，第一条赋值直接改写该字段，赋值右侧没有出现变量 %s", value[0].Line, object, field, previewExcerpt(value), previewExcerpt(guard[:len(guard)-1]), object)},
			Advice: "核对初值调用的副作用和求值顺序。若只有未进入分支的路径需要原值，可移到那条路径计算",
		})
	}
	return out
}

func independentGuard(header []units.Token, object string) bool {
	if len(header) < 3 || header[0].Text != "if" || header[len(header)-1].Text != "{" || len(header)%2 != 1 {
		return false
	}
	for i, t := range header[1 : len(header)-1] {
		if i%2 == 0 && (t.Kind != "identifier" || t.Text == object) || i%2 == 1 && t.Text != "." {
			return false
		}
	}
	return true
}

/**
 * 逗号仅在记录最外层分隔字段，嵌套调用、集合和内层记录里的同名键属于各自的表达式。
 */
func initialCall(fields []units.Token, name string) []units.Token {
	start, depth := 0, 0
	for i, t := range fields {
		switch t.Text {
		case "(", "[", "{":
			depth++
			continue
		case ")", "]", "}":
			if depth > 0 {
				depth--
				continue
			}
			if t.Text != "}" || i != len(fields)-1 {
				return nil
			}
		case ",":
			if depth != 0 {
				continue
			}
		default:
			continue
		}
		if i-start > 2 && fields[start].Text == name && fields[start+1].Text == ":" {
			value := fields[start+2 : i]
			// 集合初值的调用出现在元素中，如 []string{format(...)}。
			if len(value) >= 6 && value[0].Text == "[" && value[1].Text == "]" && value[2].Kind == "identifier" && value[3].Text == "{" && value[len(value)-1].Text == "}" {
				value = value[4 : len(value)-1]
			}
			if value[len(value)-1].Text == ")" && len(recoveryCalls(value)) > 0 {
				return value
			}
			return nil
		}
		start = i + 1
	}
	return nil
}
