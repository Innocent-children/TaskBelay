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
 * 从本次直接改写字段的行找对应的新绑定。只检查紧邻分支的第一条赋值，或同段改动中首次使用该记录的赋值。
 */
func fieldOverwrites(ctx context.Context, f units.File, lines map[int][]units.Token, changed []changes.Hunk, clusters map[string]changes.Cluster) []Candidate {
	writes := map[string][]int{}
	for _, h := range changed {
		for n := h.Span.AfterStart + 1; n <= h.Span.AfterEnd; n++ {
			if ctx.Err() != nil {
				return nil
			}
			ts := lines[n]
			if len(ts) >= 5 && ts[0].Kind == "identifier" && ts[1].Text == "." && ts[2].Kind == "identifier" && ts[3].Text == "=" {
				writes[ts[0].Text] = append(writes[ts[0].Text], n)
			}
		}
	}
	if len(writes) == 0 {
		return nil
	}
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
		positions := writes[object]
		i, _ := slices.BinarySearch(positions, last.Line+1)
		if i == len(positions) {
			continue
		}
		lineNo := positions[i]
		write := lines[lineNo]
		h := slices.IndexFunc(changed, func(h changes.Hunk) bool { return h.Span.AfterStart < lineNo && lineNo <= h.Span.AfterEnd })
		guard := lines[last.Line+1]
		inBranch := lineNo == last.Line+2 && independentGuard(guard, object)
		rhs := write[4:]
		end := rhs[len(rhs)-1]
		if end.Kind == "symbol" && end.Text != ")" && end.Text != "]" && end.Text != "}" {
			continue
		}
		if slices.ContainsFunc(rhs, func(t units.Token) bool {
			return t.Text == object || !inBranch && literalMentionsRecord(t, object) || t.Text == ";" || strings.ContainsAny(t.Text, "\r\n")
		}) || recoveryCalls(rhs) == nil {
			continue
		}
		field := write[2].Text
		value := initialCall(record[4:], field)
		if len(value) == 0 {
			continue
		}
		if !inBranch && (f.Layout != "brackets" || u.Line <= changed[h].Span.AfterStart || !unusedBeforeWrite(ctx, lines, last.Line+1, lineNo, object)) {
			continue
		}
		fact := fmt.Sprintf("第 %d 行为 %s.%s 计算初值「%s」。", value[0].Line, object, field, previewExcerpt(value))
		var advice string
		if inBranch {
			fact += fmt.Sprintf("进入「%s」后，第一条赋值直接改写该字段，赋值右侧没有出现变量 %s", previewExcerpt(guard[:len(guard)-1]), object)
			advice = "核对初值调用的副作用和求值顺序。若只有未进入分支的路径需要原值，可移到那条路径计算"
		} else {
			fact += fmt.Sprintf("从记录创建到第 %d 行，首次引用 %s 就是同层赋值「%s」，右侧没有读取该记录", lineNo, object, previewExcerpt(write))
			advice = "核对是否有路径绕过赋值并读取原值。若没有，而且初值调用没有必要的副作用，可省去这次计算"
		}
		cluster := clusters[changed[h].ID]
		out = append(out, Candidate{
			Kind: "field_overwrite", Object: units.Hash(f.Path, u.ID, field), At: fmt.Sprintf("%s:%d", f.Path, lineNo),
			Cluster: cluster.ID, AgentOnly: changes.ByAgent(cluster.Origin), ReviewOnly: true, Benefit: 1,
			Facts: []string{fact}, Advice: advice,
		})
	}
	return out
}

/**
 * 文件的括号已配对。中间可以嵌套准备新值的代码，但不能离开原块，也不能在另一个块内改写。
 */
func unusedBeforeWrite(ctx context.Context, lines map[int][]units.Token, start, end int, object string) bool {
	depth := 0
	var last units.Token
	for n := start; n < end; n++ {
		if ctx.Err() != nil {
			return false
		}
		for _, t := range lines[n] {
			if t.Text == object || literalMentionsRecord(t, object) {
				return false
			}
			last = t
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
				if depth < 0 {
					return false
				}
			case ":", "#", "goto":
				return false
			}
		}
	}
	// 上一行若以点号等运算符结尾，赋值左侧可能是跨行的成员表达式。
	return depth == 0 && (last.Kind != "symbol" || last.Text == ")" || last.Text == "]" || last.Text == "}" || last.Text == ";")
}

func literalMentionsRecord(t units.Token, object string) bool {
	return (t.Kind == "string" || t.Kind == "key") && strings.Contains(t.Text, object)
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
