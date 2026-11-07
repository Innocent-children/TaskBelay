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
		for _, u := range f.Units {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if u.Parent < 0 || u.End <= u.HeaderEnd || in.After.TestUnits[repostats.UnitKey(path, u)] || !slices.ContainsFunc(changed[path], func(h changes.Hunk) bool {
				return h.Span.AfterStart < u.EndLine && u.Line <= h.Span.AfterEnd
			}) {
				continue
			}
			prior := lines[u.Line-1]
			assign := slices.IndexFunc(prior, func(t units.Token) bool { return t.Text == "=" || t.Text == ":=" })
			if assign <= 0 || u.Line <= f.Units[u.Parent].Line {
				continue
			}
			ts := units.Significant(u.Tokens)
			cut := slices.IndexFunc(ts, func(t units.Token) bool { return t.Start >= u.HeaderEnd })
			if cut <= 0 || f.Layout == "brackets" && ts[cut-1].Text != "{" {
				continue
			}
			var value string
			for _, lhs := range prior[:assign] {
				if lhs.Kind == "identifier" && slices.ContainsFunc(ts[:cut], func(t units.Token) bool { return t.Text == lhs.Text }) {
					value = lhs.Text
					break
				}
			}
			if value == "" {
				continue
			}
			calls := units.Calls(prior[assign+1:])
			if len(calls) == 0 {
				continue
			}
			body := ts[cut:]
			for i, t := range body {
				if t.Kind != "identifier" || i+1 == len(body) || body[i+1].Text != "(" {
					continue
				}
				line := slices.IndexFunc(body[:i+1], func(v units.Token) bool { return v.Line == t.Line })
				if _, repeated := calls[t.Text]; !repeated && slices.Contains(in.After.SyntaxFor(path).Returns, body[line].Text) {
					continue
				}
				for _, h := range changed[path] {
					if t.Line <= h.Span.AfterStart || t.Line > h.Span.AfterEnd {
						continue
					}
					cluster := clusters[h.ID]
					at := fmt.Sprintf("%s:%d", path, t.Line)
					out = append(out, Candidate{Kind: "recovery", At: at, Object: units.Hash(path, u.ID, t.Text, fmt.Sprint(t.Start-u.Start)), Cluster: cluster.ID, AgentOnly: cluster.Origin == "agent", Defensive: true, ReviewOnly: true, Benefit: 1,
						Facts:  []string{fmt.Sprintf("%s:%d 的赋值中出现调用 %s；下一行的条件使用同名 %s，分支内的改动含调用 %s", path, u.Line-1, strings.Join(keys(calls), "、"), value, t.Text)},
						Advice: "核对触发条件、处理职责以及再次调用会改变什么结果；结合幂等约定、已有失败处理或更直接的做法说明用途"})
				}
			}
		}
	}
	return out, ctx.Err()
}
