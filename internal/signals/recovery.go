package signals

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type recoveryBlock struct {
	unit         units.Unit
	header, body []units.Token
}

func recoveryBlocks(ctx context.Context, repo repostats.Repository, f units.File) ([]recoveryBlock, error) {
	var blocks []recoveryBlock
	collect := func(nodes []units.Unit, ending string) {
		for _, u := range nodes {
			if u.Parent < 0 || u.Line <= nodes[u.Parent].Line || u.End <= u.HeaderEnd || repo.TestUnits[repostats.UnitKey(f.Path, u)] {
				continue
			}
			ts := u.Tokens
			cut := slices.IndexFunc(ts, func(t units.Token) bool { return t.Start >= u.HeaderEnd })
			if cut > 0 && (ending == "" || ts[cut-1].Text == ending) {
				blocks = append(blocks, recoveryBlock{u, ts[:cut], ts[cut:]})
			}
		}
	}
	if f.Layout != "brackets" {
		collect(f.Units, "")
		return blocks, nil
	}
	collect(f.Units, "{")
	if !repo.Roles[f.Path].Test && slices.ContainsFunc(f.Lines, func(s string) bool { return strings.HasSuffix(strings.TrimSpace(s), ":") }) {
		nodes, err := units.IndentedUnits(ctx, f, repo.SyntaxFor(f.Path))
		if err != nil {
			return nil, err
		}
		collect(nodes, ":")
	}
	return blocks, nil
}

/**
 * 词法样本稀少时，按完整首行识别直接判断左值的条件。冒号引出的缩进分支允许一个前置运算词。
 */
func bareResult(header []units.Token, value string) bool {
	if len(header) == 0 || header[len(header)-1].Text != ":" && header[len(header)-1].Text != "{" {
		return false
	}
	words := header[:len(header)-1]
	return (len(words) == 2 || len(words) == 3 && header[len(header)-1].Text == ":") && words[len(words)-1].Text == value &&
		!slices.ContainsFunc(words, func(t units.Token) bool { return t.Kind != "identifier" })
}

/**
 * 恢复候选需要实际的圆括号调用证据，不能沿用宽松搜索中的下标和类型参数形状。
 */
func recoveryCalls(tokens []units.Token) map[string]bool {
	calls := map[string]bool{}
	var stack []int
	for i, t := range tokens {
		switch t.Text {
		case "(", "[", "{":
			stack = append(stack, i)
		case ")", "]", "}":
			if len(stack) == 0 {
				return nil
			}
			open := stack[len(stack)-1]
			if tokens[open].Text+t.Text != "()" && tokens[open].Text+t.Text != "[]" && tokens[open].Text+t.Text != "{}" {
				return nil
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 && tokens[open].Text == "(" && open > 0 && tokens[open-1].Kind == "identifier" {
				calls[tokens[open-1].Text] = true
			}
		}
	}
	if len(stack) > 0 {
		return nil
	}
	return calls
}

// 只描述相邻赋值、条件和调用的关系。名字和括号不能证明这是错误路径。
func recoveries(ctx context.Context, in Input) ([]Candidate, error) {
	clusters := map[string]changes.Cluster{}
	for _, cluster := range in.Changes.Clusters {
		for _, id := range cluster.Hunks {
			clusters[id] = cluster
		}
	}
	changed := map[string][]changes.Hunk{}
	for _, h := range in.Changes.Hunks {
		if h.Kind != "cosmetic" && h.Span.AfterEnd > h.Span.AfterStart {
			changed[h.Path] = append(changed[h.Path], h)
		}
	}
	var out []Candidate
	for _, path := range keys(changed) {
		f := in.After.Files[path]
		lines := map[int][]units.Token{}
		for _, t := range units.Significant(f.Tokens) {
			lines[t.Line] = append(lines[t.Line], t)
		}
		blocks, err := recoveryBlocks(ctx, in.After, f)
		if err != nil {
			return nil, err
		}
		for _, block := range blocks {
			u := block.unit
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !slices.ContainsFunc(changed[path], func(h changes.Hunk) bool {
				return h.Span.AfterStart < u.EndLine && u.Line <= h.Span.AfterEnd
			}) {
				continue
			}
			prior := lines[u.Line-1]
			assign := slices.IndexFunc(prior, func(t units.Token) bool { return t.Text == "=" || t.Text == ":=" })
			if assign <= 0 {
				continue
			}
			// 比较、取反或直接判断调用返回的值。引入迭代变量的循环不属于这种条件。
			var symbols strings.Builder
			onlyAssigned := true
			var value string
			for i, t := range block.header {
				if t.Kind == "symbol" {
					symbols.WriteString(t.Text)
				}
				if t.Kind == "identifier" {
					member := i > 0 && block.header[i-1].Text == "." || i+1 < len(block.header) && (block.header[i+1].Text == "." || block.header[i+1].Text == "[")
					if !member && slices.ContainsFunc(prior[:assign], func(lhs units.Token) bool { return lhs.Kind == "identifier" && lhs.Text == t.Text }) {
						value = t.Text
					} else if !slices.Contains(in.After.SyntaxFor(path).Keywords, t.Text) {
						onlyAssigned = false
					}
				}
			}
			header := symbols.String()
			compares := strings.Contains(header, "==") || strings.ContainsAny(header, "!<>")
			if value == "" || strings.Contains(header, ":=") || !(compares || onlyAssigned || bareResult(block.header, value)) {
				continue
			}
			calls := recoveryCalls(prior[assign+1:])
			if len(calls) == 0 {
				continue
			}
			body := block.body
			for i, t := range body {
				if t.Kind != "identifier" || i+1 == len(body) || body[i+1].Text != "(" {
					continue
				}
				line := slices.IndexFunc(body[:i+1], func(v units.Token) bool { return v.Line == t.Line })
				if _, repeated := calls[t.Text]; !repeated && line < i && slices.Contains(in.After.SyntaxFor(path).Returns, body[line].Text) {
					continue
				}
				for _, h := range changed[path] {
					if t.Line <= h.Span.AfterStart || t.Line > h.Span.AfterEnd {
						continue
					}
					cluster := clusters[h.ID]
					at := fmt.Sprintf("%s:%d", path, t.Line)
					out = append(out, Candidate{Kind: "recovery", At: at, Object: units.Hash(path, u.ID, t.Text, fmt.Sprint(t.Start-u.Start)), Cluster: cluster.ID, AgentOnly: changes.ByAgent(cluster.Origin), Defensive: true, ReviewOnly: true, Benefit: 1,
						Facts:  []string{fmt.Sprintf("%s:%d 的赋值中出现调用 %s；下一行的条件使用同名 %s，分支内的改动含调用 %s", path, u.Line-1, strings.Join(keys(calls), "、"), value, t.Text)},
						Advice: "核对触发条件、处理职责以及再次调用会改变什么结果；结合幂等约定、已有失败处理或更直接的做法说明用途"})
				}
			}
		}
	}
	return out, ctx.Err()
}
