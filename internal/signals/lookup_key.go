package signals

import (
	"context"
	"fmt"
	"slices"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/units"
)

/**
 * 只记录完整的单行查表赋值。父节点给出局部变量可见的范围，避免跨兄弟块比较同名调用。
 */
func repeatedLookupKeys(ctx context.Context, f units.File, lines map[int][]units.Token, changed []changes.Hunk, clusters map[string]changes.Cluster) []Candidate {
	type lookup struct {
		call []units.Token
		end  int
	}
	starts := map[int]lookup{}
	for _, u := range f.Units {
		if ctx.Err() != nil {
			return nil
		}
		if u.Parent < 0 || u.Line != u.EndLine || len(u.Tokens) < 9 {
			continue
		}
		ts := u.Tokens
		if len(ts) != len(lines[u.Line]) ||
			ts[0].Kind != "identifier" || ts[1].Text != ":=" || ts[2].Kind != "identifier" || ts[3].Text != "[" ||
			ts[4].Kind != "identifier" || ts[5].Text != "(" || ts[len(ts)-2].Text != ")" || ts[len(ts)-1].Text != "]" {
			continue
		}
		args := ts[6 : len(ts)-2]
		if slices.ContainsFunc(args, func(t units.Token) bool {
			return t.Kind == "symbol" && !slices.Contains([]string{".", ",", "[", "]", ":"}, t.Text)
		}) {
			continue
		}
		starts[u.Line] = lookup{ts[4 : len(ts)-1], f.Units[u.Parent].End}
	}
	if len(starts) == 0 {
		return nil
	}
	var out []Candidate
	for _, h := range changed {
		prior := map[string]lookup{}
		for n := h.Span.AfterStart + 1; n <= h.Span.AfterEnd; n++ {
			if ctx.Err() != nil {
				return nil
			}
			ts := lines[n]
			for i, t := range ts {
				if i == 0 || ts[i-1].Text != "," && ts[i-1].Text != "(" {
					continue
				}
				key, ok := prior[t.Text]
				end := i + len(key.call)
				if !ok || end >= len(ts) || ts[end].Start >= key.end || ts[end].Text != "," && ts[end].Text != ")" ||
					!slices.EqualFunc(key.call, ts[i:end], func(a, b units.Token) bool { return a.Text == b.Text }) {
					continue
				}
				first := key.call[0].Line
				cluster := clusters[h.ID]
				out = append(out, Candidate{
					Kind: "lookup_key", At: fmt.Sprintf("%s:%d", f.Path, n), Object: units.Hash(f.Path, h.ID, fmt.Sprint(first), t.Text),
					Cluster: cluster.ID, AgentOnly: changes.ByAgent(cluster.Origin), ReviewOnly: true, Benefit: 1,
					Facts:  []string{fmt.Sprintf("第 %d 行用「%s」的返回值作下标，第 %d 行再次出现相同写法的调用", first, previewExcerpt(key.call), n)},
					Advice: "核对两处变量的绑定、期间的状态变化及调用的副作用。若可以共用结果，可在查表前算一次键，两处直接使用",
				})
				delete(prior, t.Text)
			}
			if key, ok := starts[n]; ok {
				prior[key.call[0].Text] = key
			}
		}
	}
	return out
}
