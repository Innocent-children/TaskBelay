package codescan

import (
	"context"
	"sort"
	"strings"
)

type codeLine struct {
	line int
	code string
}

func declarationLines(d Declaration) []codeLine {
	byLine := map[int]string{}
	for _, token := range d.Tokens {
		if strings.Contains(token.Kind, "comment") {
			continue
		}
		byLine[token.Span.StartLine] += token.Kind + "\x00" + token.Text + "\x00"
	}
	var lines []codeLine
	for line, code := range byLine {
		lines = append(lines, codeLine{line, code})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].line < lines[j].line })
	return lines
}

/**
 * 对规范化后的源码行求差异，忽略缩进与注释。先裁去公共两端，矩阵受限，
 * 极大声明或取消返回未知，不能把整段未改风险当成这次行为变化。
 */
func BehaviorLines(ctx context.Context, before, after Declaration) (map[int]bool, map[int]bool, bool) {
	a, b := declarationLines(before), declarationLines(after)
	for len(a) > 0 && len(b) > 0 && a[0].code == b[0].code {
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1].code == b[len(b)-1].code {
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	old, current := map[int]bool{}, map[int]bool{}
	if len(a) > 0 && len(b) > 1_000_000/len(a) {
		return old, current, false
	}
	width := len(b) + 1
	table := make([]uint32, (len(a)+1)*width)
	for i := len(a) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return old, current, false
		}
		for j := len(b) - 1; j >= 0; j-- {
			if a[i].code == b[j].code {
				table[i*width+j] = 1 + table[(i+1)*width+j+1]
			} else {
				table[i*width+j] = max(table[(i+1)*width+j], table[i*width+j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i].code == b[j].code:
			i++
			j++
		case j == len(b) || (i < len(a) && table[(i+1)*width+j] >= table[i*width+j+1]):
			old[a[i].line] = true
			i++
		default:
			current[b[j].line] = true
			j++
		}
	}
	return old, current, ctx.Err() == nil
}
