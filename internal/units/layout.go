package units

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

func layout(ctx context.Context, f File) ([]Unit, string, float64, error) {
	tokens := Significant(f.Tokens)
	var nodes []Unit
	var stack []int
	var openings []string
	var starts []int
	lineStart := 0
	matched, failed, aligned, total := 0, 0, 0, 0
	for i, t := range tokens {
		if err := ctx.Err(); err != nil {
			return nil, "", 0, err
		}
		if i == 0 || tokens[i-1].Line != t.Line {
			lineStart = i
		}
		if strings.Contains("({[", t.Text) && len(t.Text) == 1 {
			parent := -1
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			start := lineStart
			u := Unit{Start: tokens[start].Start, HeaderEnd: t.End, End: t.End, Line: tokens[start].Line, EndLine: t.Line, Parent: parent}
			nodes = append(nodes, u)
			openings = append(openings, t.Text)
			starts = append(starts, start)
			stack = append(stack, len(nodes)-1)
			total++
		} else if strings.Contains(")}]", t.Text) && len(t.Text) == 1 {
			if len(stack) == 0 {
				failed++
				continue
			}
			idx := stack[len(stack)-1]
			open := openings[idx]
			if open+t.Text != "()" && open+t.Text != "{}" && open+t.Text != "[]" {
				failed++
				continue
			}
			stack = stack[:len(stack)-1]
			nodes[idx].End = t.End
			nodes[idx].EndLine = t.Line
			nodes[idx].Tokens = tokens[starts[idx] : i+1]
			matched++
			if indent(f.Lines[nodes[idx].Line-1]) == indent(f.Lines[t.Line-1]) || nodes[idx].Line == t.Line {
				aligned++
			}
		}
	}
	failed += len(stack)
	consistency := float64(matched+aligned) / float64(max(1, total+matched+failed))
	if failed == 0 && matched > 0 && aligned == matched {
		return nodes, "brackets", consistency, nil
	}
	return indentLayout(ctx, f, tokens, failed)
}

// IndentedUnits 供需要核对缩进分支的分析使用。调用括号可以配对，但未必界定分支正文。
func IndentedUnits(ctx context.Context, f File, syntax Syntax) ([]Unit, error) {
	var err error
	f.Units, _, _, err = indentLayout(ctx, f, Significant(f.Tokens), 0)
	if err != nil {
		return nil, err
	}
	err = assignNames(ctx, &f, syntax)
	return f.Units, err
}

func indentLayout(ctx context.Context, f File, tokens []Token, failed int) ([]Unit, string, float64, error) {
	var nodes []Unit
	var stack []int
	offset := 0
	for line, text := range f.Lines {
		if err := ctx.Err(); err != nil {
			return nil, "", 0, err
		}
		trim := strings.TrimSpace(text)
		if trim == "" {
			offset += len(text)
			continue
		}
		level := indent(text)
		for len(stack) > 0 && indent(f.Lines[nodes[stack[len(stack)-1]].Line-1]) >= level {
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			nodes[idx].End = offset
			nodes[idx].EndLine = line
		}
		parent := -1
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		nodes = append(nodes, Unit{Start: offset, HeaderEnd: offset + len(strings.TrimRight(text, "\r\n")), Line: line + 1, Parent: parent})
		stack = append(stack, len(nodes)-1)
		offset += len(text)
	}
	for _, idx := range stack {
		nodes[idx].End = offset
		nodes[idx].EndLine = len(f.Lines)
	}
	for i := range nodes {
		start := sort.Search(len(tokens), func(j int) bool { return tokens[j].Start >= nodes[i].Start })
		end := sort.Search(len(tokens), func(j int) bool { return tokens[j].Start >= nodes[i].End })
		// 跨行词只能归入完整包含它的单元，不能把半个字符串当作单元内容。
		if end > start && tokens[end-1].End > nodes[i].End {
			end--
		}
		nodes[i].Tokens = tokens[start:end]
	}
	return nodes, "indent", float64(len(nodes)) / float64(max(1, len(nodes)+failed)), ctx.Err()
}

func indent(line string) int {
	n := 0
	for _, r := range line {
		if r != ' ' && r != '\t' {
			break
		}
		n++
	}
	return n
}

func assignNames(ctx context.Context, f *File, syntax Syntax) error {
	freq := map[string]int{}
	keywords := map[string]bool{}
	for _, k := range syntax.Keywords {
		keywords[k] = true
	}
	for _, u := range f.Units {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, t := range u.Tokens {
			if t.End <= u.HeaderEnd && t.Kind == "identifier" {
				seen[t.Text] = true
			}
		}
		for w := range seen {
			freq[w]++
		}
	}
	contexts := make([]string, len(f.Units))
	occurrences := map[string]int{}
	for i := range f.Units {
		if err := ctx.Err(); err != nil {
			return err
		}
		u := &f.Units[i]
		u.Name = ""
		var names []string
		for _, t := range u.Tokens {
			if t.End <= u.HeaderEnd && (t.Kind == "identifier" || t.Kind == "key") && !keywords[Literal(t)] {
				names = append(names, Literal(t))
			}
		}
		sort.SliceStable(names, func(i, j int) bool { return freq[names[i]] < freq[names[j]] })
		if len(names) > 0 {
			u.Name = names[0]
		}
		for j, t := range u.Tokens {
			if t.End > u.HeaderEnd {
				break
			}
			if t.Kind == "identifier" && j+1 < len(u.Tokens) && u.Tokens[j+1].Text == "(" {
				u.Name = t.Text
				break
			}
		}
		var header []Token
		for _, token := range u.Tokens {
			if token.End <= u.HeaderEnd {
				header = append(header, token)
			}
		}
		parent := ""
		if u.Parent >= 0 {
			parent = contexts[u.Parent]
		}
		// 父级只取首行，避免兄弟单元改动重置身份；同层重复首行按出现次序区分。
		key := Hash(parent, strings.Join(Sequence(header, false), "\x00"))
		occurrences[key]++
		contexts[i] = Hash(key, strconv.Itoa(occurrences[key]))
		u.ID = Hash(contexts[i], strings.Join(Sequence(u.Tokens, false), "\x00"))
	}
	return ctx.Err()
}

func Calls(tokens []Token) map[string][][]Token {
	result := map[string][][]Token{}
	ts := Significant(tokens)
	for i, t := range ts {
		if t.Kind != "identifier" || i+1 >= len(ts) || !strings.Contains("([<", ts[i+1].Text) || len(ts[i+1].Text) != 1 {
			continue
		}
		depth := 0
		start := i + 2
		var args []Token
		for j := i + 1; j < len(ts); j++ {
			v := ts[j].Text
			if len(v) == 1 && strings.Contains("([{<", v) {
				depth++
			}
			if len(v) == 1 && strings.Contains(")]}>", v) {
				depth--
			}
			if depth == 0 {
				if j > start {
					args = append(args, Token{Kind: "argument", Text: strings.Join(Sequence(ts[start:j], false), " "), Start: ts[start].Start, End: ts[j-1].End, Line: ts[start].Line})
				}
				result[t.Text] = append(result[t.Text], args)
				break
			}
			if depth == 1 && v == "," {
				if j > start {
					args = append(args, Token{Kind: "argument", Text: strings.Join(Sequence(ts[start:j], false), " "), Start: ts[start].Start, End: ts[j-1].End, Line: ts[start].Line})
				}
				start = j + 1
			}
		}
	}
	return result
}
