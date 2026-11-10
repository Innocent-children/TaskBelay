package changes

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/units"
)

type Span struct{ BeforeStart, BeforeEnd, AfterStart, AfterEnd int }

func Diff(ctx context.Context, a, b []string) ([]Span, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil, nil
	}
	if n == 0 || m == 0 {
		return []Span{{BeforeEnd: n, AfterEnd: m}}, nil
	}
	v := map[int]int{1: 0}
	var trace []map[int]int
	cells := 0
	for d := 0; d <= n+m; d++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cells += len(v)
		if cells > DiffTraceBudget {
			return nil, fmt.Errorf("差分回溯超过计算预算")
		}
		copyV := map[int]int{}
		for k, x := range v {
			copyV[k] = x
		}
		trace = append(trace, copyV)
		for k := -d; k <= d; k += 2 {
			x := 0
			if k == -d || k != d && v[k-1] < v[k+1] {
				x = v[k+1]
			} else {
				x = v[k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
				if x%units.PollBytes == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
			}
			v[k] = x
			if x >= n && y >= m {
				return backtrack(trace, n, m), nil
			}
		}
	}
	return nil, fmt.Errorf("差分未能完成")
}

func backtrack(trace []map[int]int, x, y int) []Span {
	type edit struct {
		x, y int
		add  bool
	}
	var edits []edit
	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		k := x - y
		prev := k - 1
		if k == -d || k != d && v[k-1] < v[k+1] {
			prev = k + 1
		}
		px := v[prev]
		py := px - prev
		for x > px && y > py {
			x--
			y--
		}
		if d == 0 {
			break
		}
		if x == px {
			y--
			edits = append(edits, edit{x, y, true})
		} else {
			x--
			edits = append(edits, edit{x, y, false})
		}
	}
	slices.Reverse(edits)
	var spans []Span
	for _, e := range edits {
		s := Span{e.x, e.x, e.y, e.y}
		if e.add {
			s.AfterEnd++
		} else {
			s.BeforeEnd++
		}
		if len(spans) > 0 {
			p := &spans[len(spans)-1]
			if p.BeforeEnd == s.BeforeStart && p.AfterEnd == s.AfterStart {
				p.BeforeEnd = s.BeforeEnd
				p.AfterEnd = s.AfterEnd
				continue
			}
		}
		spans = append(spans, s)
	}
	return spans
}

func classify(before, after []units.Token) string {
	a, b := units.Sequence(before, false), units.Sequence(after, false)
	if slices.Equal(a, b) {
		return "cosmetic"
	}
	aT, bT := units.Significant(before), units.Significant(after)
	if len(aT) == len(bT) && len(aT) > 0 {
		forward, reverse := map[string]string{}, map[string]string{}
		valid := true
		for i, t := range aT {
			u := bT[i]
			if t.Kind == "identifier" && u.Kind == "identifier" {
				if f, ok := forward[t.Text]; ok && f != u.Text {
					valid = false
				}
				if r, ok := reverse[u.Text]; ok && r != t.Text {
					valid = false
				}
				forward[t.Text] = u.Text
				reverse[u.Text] = t.Text
			} else if t.Text != u.Text {
				valid = false
			}
		}
		if valid {
			return "rename"
		}
	}
	return "behavior"
}

func tokensIn(f units.File, start, end int) []units.Token {
	var out []units.Token
	for _, t := range f.Tokens {
		if t.Line >= start+1 && t.Line <= end {
			out = append(out, t)
		}
	}
	return out
}
func lineText(lines []string, start, end int) string { return strings.Join(lines[start:end], "") }
