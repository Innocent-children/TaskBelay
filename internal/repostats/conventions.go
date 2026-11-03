package repostats

import (
	"context"
	"math"
	"path"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/units"
)

func learnTests(ctx context.Context, r *Repository, runs []Run) error {
	seeds := map[string]bool{}
	for _, p := range sortedKeys(r.Files) {
		seed := false
		for _, w := range units.Words(strings.TrimSuffix(p, path.Ext(p))) {
			if w == "test" || w == "tests" || w == "spec" || w == "specs" {
				seed = true
			}
		}
		for _, u := range r.Files[p].Units {
			key := UnitKey(p, u)
			match := false
			for _, run := range runs {
				if run.Started && run.ExitCode != nil && units.HasWord(run.Output, u.Name) {
					match = true
				}
			}
			if match || len(runs) == 0 && seed {
				seeds[key] = true
				r.TestUnits[key] = true
			}
		}
	}
	for iteration := 0; iteration < IterationBudget; iteration++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a, b := map[string]int{}, map[string]int{}
		na, nb := 0, 0
		for _, p := range sortedKeys(r.Files) {
			for _, u := range r.Files[p].Units {
				for _, t := range u.Tokens {
					if t.Kind != "identifier" {
						continue
					}
					if r.TestUnits[UnitKey(p, u)] {
						a[t.Text]++
						na++
					} else {
						b[t.Text]++
						nb++
					}
				}
			}
		}
		vocab := map[string]bool{}
		for w := range a {
			vocab[w] = true
		}
		for w := range b {
			vocab[w] = true
		}
		alpha := Prior * float64(max(1, len(vocab)))
		for _, w := range sortedKeys(vocab) {
			x, y := float64(a[w])+Prior, float64(b[w])+Prior
			delta := math.Log(x/(float64(na)+alpha-x+Prior)) - math.Log(y/(float64(nb)+alpha-y+Prior))
			r.TestWords[w] = delta / math.Sqrt(1/x+1/y)
		}
		next := map[string]bool{}
		changed := false
		for _, p := range sortedKeys(r.Files) {
			for _, u := range r.Files[p].Units {
				key := UnitKey(p, u)
				score := 0.0
				for _, t := range u.Tokens {
					if t.Kind == "identifier" {
						score += r.TestWords[t.Text]
					}
				}
				if seeds[key] || na > 0 && nb > 0 && score > 0 {
					next[key] = true
				}
				if next[key] != r.TestUnits[key] {
					changed = true
				}
			}
		}
		r.TestUnits = next
		if !changed {
			r.Converged = true
			return nil
		}
	}
	return nil
}

func learnEntries(r *Repository) {
	type counts struct{ all, once int }
	patterns := map[string]counts{}
	total, once := 0, 0
	for _, p := range sortedKeys(r.Files) {
		for _, u := range r.Files[p].Units {
			if u.Name == "" {
				continue
			}
			total++
			single := r.Frequency[u.Name] == 1
			if single {
				once++
			}
			words := units.Words(u.Name)
			var keys []string
			if len(words) > 0 {
				keys = append(keys, "prefix\x00"+firstPart(u.Name, words[0]), "suffix\x00"+lastPart(u.Name, words[len(words)-1]))
			}
			keys = append(keys, "path\x00"+path.Dir(p)+"/")
			for _, t := range u.Tokens {
				if t.End <= u.HeaderEnd && t.Kind == "identifier" && t.Text != u.Name {
					keys = append(keys, "header\x00"+t.Text)
				}
			}
			for _, key := range unique(keys) {
				c := patterns[key]
				c.all++
				if single {
					c.once++
				}
				patterns[key] = c
			}
		}
	}
	prob := float64(once+1) / float64(total+2)
	for _, key := range sortedKeys(patterns) {
		c := patterns[key]
		p := BinomialTail(c.once, c.all, prob)
		if p < Significance/float64(max(1, len(patterns))) {
			parts := strings.SplitN(key, "\x00", 2)
			r.Entries = append(r.Entries, Pattern{parts[0], parts[1], p})
		}
	}
}

func firstPart(s, w string) string {
	r := []rune(s)
	n := min(len(r), len([]rune(w)))
	return string(r[:n])
}
func lastPart(s, w string) string {
	r := []rune(s)
	n := min(len(r), len([]rune(w)))
	return string(r[len(r)-n:])
}
func BinomialTail(k, n int, p float64) float64 {
	if k <= 0 {
		return 1
	}
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	a, _ := math.Lgamma(float64(n + 1))
	sum := 0.0
	for j := k; j <= n; j++ {
		b, _ := math.Lgamma(float64(j + 1))
		c, _ := math.Lgamma(float64(n - j + 1))
		sum += math.Exp(a - b - c + float64(j)*math.Log(p) + float64(n-j)*math.Log1p(-p))
	}
	return math.Min(1, sum)
}
func CaseNames(r Repository) []string {
	var names []string
	for _, p := range sortedKeys(r.Files) {
		for _, u := range CaseUnits(r, p) {
			if u.Name != "" {
				names = append(names, u.Name)
			}
		}
	}
	return unique(names)
}

func CaseUnits(r Repository, p string) []units.Unit {
	var out []units.Unit
	for _, u := range r.Files[p].Units {
		if !r.TestUnits[UnitKey(p, u)] {
			continue
		}
		contained := false
		for _, parent := range r.Files[p].Units {
			if r.TestUnits[UnitKey(p, parent)] && parent.Start <= u.Start && parent.End > u.End {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, u)
		}
	}
	return out
}
