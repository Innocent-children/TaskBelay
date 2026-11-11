package signals

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type filterLoop struct {
	path, function    string
	line, slot, arity int
	cluster           changes.Cluster
}

// 对已分词的首行取出判断结果、集合实参和回调形参。只接受直接读变量或成员。
const filterName = `[\p{L}_][\p{L}\p{N}_]*`
const filterSelector = filterName + `(?: \. ` + filterName + `)*`

var filterCallback = regexp.MustCompile(`^(` + filterName + `) := ` + filterSelector + ` \( (` + filterSelector + `) , func \( (` + filterName + `) ` + filterSelector + ` \) bool \{$`)

/**
 * 仅归一化元素变量名。字段顺序、取反与逻辑运算符不同都不能作为同一筛选的依据。
 */
func fieldCondition(ts []units.Token, element string) string {
	var key strings.Builder
	for len(ts) > 0 {
		if ts[0].Text == "!" {
			key.WriteByte('!')
			ts = ts[1:]
		}
		if len(ts) < 3 || ts[0].Text != element || ts[1].Text != "." || ts[2].Kind != "identifier" {
			return ""
		}
		key.WriteString(ts[2].Text)
		ts = ts[3:]
		if len(ts) == 0 {
			return key.String()
		}
		if ts[0].Text != "&&" && ts[0].Text != "||" {
			return ""
		}
		key.WriteString(ts[0].Text)
		ts = ts[1:]
	}
	return ""
}

/**
 * 只取声明首行中的具名参数，沿用调用实参的分隔规则。
 */
func filterParams(u units.Unit) []units.Token {
	end := len(u.Tokens) - len(u.BodyTokens())
	sites := units.Calls(u.Tokens[:end])[u.Name]
	if len(sites) != 1 {
		return nil
	}
	return sites[0]
}

func collectFilterLoops(ctx context.Context, f units.File, lines map[int][]units.Token, changed []changes.Hunk, clusters map[string]changes.Cluster, loops map[string][]filterLoop) {
	for _, h := range changed {
		for n := h.Span.AfterStart + 1; n <= h.Span.AfterEnd; n++ {
			if ctx.Err() != nil {
				return
			}
			loop, guard := lines[n-1], lines[n]
			if len(loop) != 8 || loop[0].Text != "for" || loop[1].Text != "_" || loop[2].Text != "," || loop[4].Text != ":=" || loop[5].Text != "range" || loop[7].Text != "{" {
				continue
			}
			if len(guard) < 5 || guard[0].Text != "if" || guard[len(guard)-1].Text != "{" {
				continue
			}
			key := fieldCondition(guard[1:len(guard)-1], loop[3].Text)
			if key == "" {
				continue
			}
			u := EnclosingDeclaration(f, loop[0].Start, guard[len(guard)-1].End)
			params := filterParams(u)
			slot := slices.IndexFunc(params, func(param units.Token) bool {
				return strings.HasPrefix(param.Text, loop[6].Text+" [ ] ")
			})
			if slot >= 0 {
				loops[key] = append(loops[key], filterLoop{f.Path, u.Name, n, slot, len(params), clusters[h.ID]})
			}
		}
	}
}

func repeatedFilters(ctx context.Context, in Input, files map[string]map[int][]units.Token, loops map[string][]filterLoop) []Candidate {
	var out []Candidate
	for _, p := range keys(files) {
		f, lines := in.After.Files[p], files[p]
		for n := 1; n <= len(f.Lines); n++ {
			if ctx.Err() != nil {
				return nil
			}
			ts := lines[n]
			if len(ts) < 4 || ts[0].Text != "return" {
				continue
			}
			elem := ts[1]
			if elem.Text == "!" {
				elem = ts[2]
			}
			condition := fieldCondition(ts[1:], elem.Text)
			matched := loops[condition]
			if len(matched) == 0 || !slices.Equal(units.Sequence(lines[n+1], false), []string{"}", ")"}) {
				continue
			}
			callback := filterCallback.FindStringSubmatch(strings.Join(units.Sequence(lines[n-1], false), " "))
			if callback == nil || callback[3] != elem.Text {
				continue
			}
			caller := EnclosingDeclaration(f, lines[n-1][0].Start, lines[n+1][1].End)
			for _, block := range f.Units {
				if block.Line <= n+1 || block.End > caller.End || block.EndLine <= block.Line ||
					!slices.Equal(units.Sequence(lines[block.Line], false), []string{"if", callback[1], "{"}) {
					continue
				}
				calls := units.Calls(block.BodyTokens())
				for _, loop := range matched {
					if path.Dir(p) != path.Dir(loop.path) {
						continue
					}
					route := filterRoute(ctx, in, files, block, calls, callback[2], loop)
					if route == "" {
						continue
					}
					at := fmt.Sprintf("%s:%d", loop.path, loop.line)
					out = append(out, Candidate{
						Kind: "repeated_filter", Object: units.Hash(p, caller.Name, loop.path, loop.function, route, condition), At: at,
						Cluster: loop.cluster.ID, AgentOnly: changes.ByAgent(loop.cluster.Origin), ReviewOnly: true, Benefit: 1,
						Facts:  []string{fmt.Sprintf("%s:%d 的回调返回「%s」。按分支中的实参和同名声明的位置，%s 经 %s 对应到循环参数；循环检查相同字段条件", p, n, previewExcerpt(ts[1:]), strings.ReplaceAll(callback[2], " ", ""), route)},
						Advice: "核对实际调用对象和途中数据变化。若同一批元素最终需要全部筛出，可一次收集并用结果是否为空判断；需要短路或重新取值时保留两步",
					})
				}
			}
		}
	}
	return out
}

/**
 * 只核对同批声明中的直接传参和一层原样转交。调用名和实参位置不能证明运行时接收者相同。
 */
func filterRoute(ctx context.Context, in Input, files map[string]map[int][]units.Token, branch units.Unit, calls map[string][][]units.Token, source string, loop filterLoop) string {
	passes := func(owner units.Unit, sites [][]units.Token, arity, slot int, arg string) bool {
		return slices.ContainsFunc(sites, func(site []units.Token) bool {
			if len(site) != arity || site[slot].Text != arg {
				return false
			}
			// Calls 也收录下标及类型实参。只在名字、位置和实参都命中时核对左括号。
			first := sort.Search(len(owner.Tokens), func(i int) bool { return owner.Tokens[i].Start >= site[0].Start })
			return first > 0 && owner.Tokens[first-1].Text == "("
		})
	}
	if passes(branch, calls[loop.function], loop.arity, loop.slot, source) {
		return loop.function
	}
	for _, p := range keys(files) {
		if path.Dir(p) != path.Dir(loop.path) {
			continue
		}
		f := in.After.Files[p]
		for _, u := range f.Units {
			if ctx.Err() != nil {
				return ""
			}
			sites := calls[u.Name]
			if len(sites) == 0 || !declaration(f, u) {
				continue
			}
			params := filterParams(u)
			for i, param := range params {
				variable, _, slice := strings.Cut(param.Text, " [ ] ")
				if !slice || !passes(branch, sites, len(params), i, source) {
					continue
				}
				forwarded := units.Calls(u.BodyTokens())[loop.function]
				if passes(u, forwarded, loop.arity, loop.slot, variable) {
					return u.Name + " → " + loop.function
				}
			}
		}
	}
	return ""
}
