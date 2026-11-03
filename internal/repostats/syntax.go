package repostats

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"strings"
	"unicode"
)

type SyntaxCounts struct{ Quotes, Pairs, Comments, Prose map[string]int }

func CountSyntax(ctx context.Context, data []byte) (SyntaxCounts, error) {
	c := SyntaxCounts{map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}}
	for _, line := range strings.Split(string(data), "\n") {
		if err := ctx.Err(); err != nil {
			return c, err
		}
		counts := map[string]int{}
		for _, r := range line {
			if r == '\'' || r == '"' || r == '`' || unicode.Is(unicode.Pi, r) || unicode.Is(unicode.Pf, r) {
				counts[string(r)]++
			}
		}
		for q, n := range counts {
			c.Quotes[q]++
			if n%2 == 0 {
				c.Pairs[q]++
			}
		}
		trim := strings.TrimSpace(line)
		end := 0
		for _, r := range trim {
			if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
				break
			}
			end += len(string(r))
		}
		if end > 0 && end < len(trim) {
			prefix := trim[:end]
			c.Comments[prefix]++
			tail := trim[end:]
			symbols := 0
			for _, r := range tail {
				if unicode.IsPunct(r) || unicode.IsSymbol(r) {
					symbols++
				}
			}
			if len(strings.Fields(tail)) > symbols {
				c.Prose[prefix]++
			}
		}
	}
	return c, ctx.Err()
}

func LearnSyntax(ctx context.Context, sources []units.Source, cached map[string]SyntaxCounts) (map[string]units.Syntax, error) {
	groups := map[string]SyntaxCounts{}
	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, ok := cached[src.Blob]
		if !ok {
			var err error
			c, err = CountSyntax(ctx, src.Data)
			if err != nil {
				return nil, err
			}
		}
		ext := path.Ext(src.Path)
		g, ok := groups[ext]
		if !ok {
			g = SyntaxCounts{map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}}
		}
		for i, counts := range []map[string]int{c.Quotes, c.Pairs, c.Comments, c.Prose} {
			target := []map[string]int{g.Quotes, g.Pairs, g.Comments, g.Prose}[i]
			for word, n := range counts {
				target[word] += n
			}
		}
		groups[ext] = g
	}
	out := map[string]units.Syntax{}
	for _, ext := range sortedKeys(groups) {
		c := groups[ext]
		syn := units.Syntax{}
		for _, q := range sortedKeys(c.Quotes) {
			if c.Pairs[q]*2 > c.Quotes[q] {
				syn.Quotes = append(syn.Quotes, q)
			}
		}
		for _, p := range sortedKeys(c.Comments) {
			if c.Prose[p]*2 > c.Comments[p] && !strings.ContainsAny(p, "{}[]()\"'`") {
				syn.Comments = append(syn.Comments, p)
			}
		}
		out[ext] = syn
	}
	return out, ctx.Err()
}
