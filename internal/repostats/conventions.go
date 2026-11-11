package repostats

import (
	"context"
	"math"
	"path"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/units"
)

type TestModel struct {
	LogPrior   float64
	Weights    map[string]float64
	Extensions map[string]bool
}

/**
 * TestFeatures 只取文件名和首行，不让函数体中的断言式写法决定测试身份。
 */
func TestFeatures(p string, u units.Unit) map[string]int {
	counts := map[string]int{}
	for _, w := range units.Words(strings.TrimSuffix(path.Base(p), path.Ext(p))) {
		counts["file:"+w] = 1
	}
	for _, t := range u.Tokens {
		if t.End <= u.HeaderEnd && t.Kind == "identifier" {
			for _, w := range units.Words(t.Text) {
				counts["head:"+w] = 1
			}
		}
	}
	return counts
}

// pathSeed 判断路径里是否含 test、spec 一类词；这样的文件是测试内容的初始样本。
func pathSeed(p string) bool {
	for _, w := range units.Words(strings.TrimSuffix(p, path.Ext(p))) {
		if w == "test" || w == "tests" || w == "spec" || w == "specs" {
			return true
		}
	}
	return false
}

func (m TestModel) TestCounts(counts map[string]int, previous bool) bool {
	score := m.LogPrior
	for _, w := range sortedKeys(counts) {
		if counts[w] > 0 {
			score += m.Weights[w]
		}
	}
	return (score > 0 || score == 0 && previous) && m.fileEvidence(counts)
}

/**
 * fileEvidence 要求文件名里至少有一个词在测试样本中更常见。
 * 首行里的 import、function 一类词在测试和产品代码中都大量出现，只凭它们扩展样本，
 * 划分会逐轮扩大到整个仓库；文件名层面的证据把扩展限制在与样本同类的文件内。
 */
func (m TestModel) fileEvidence(counts map[string]int) bool {
	for w, n := range counts {
		if n > 0 && strings.HasPrefix(w, "file:") && m.Weights[w] > 0 {
			return true
		}
	}
	return false
}

func testCounts(r *Repository, p string, u units.Unit) map[string]int {
	if r.TestCounts != nil {
		return r.TestCounts[UnitKey(p, u)]
	}
	counts := map[string]int{}
	for _, t := range u.Tokens {
		if t.Kind == "identifier" {
			counts[t.Text]++
		}
	}
	return counts
}

func learnTests(ctx context.Context, r *Repository, runs []Run) error {
	r.Converged, r.TestIterations = false, 0
	r.TestUnits, r.TestWords = map[string]bool{}, map[string]float64{}
	r.Tests = TestModel{}
	defer func() {
		if !r.Converged {
			r.TestUnits, r.TestWords, r.TestSeeds = nil, nil, nil
			r.Tests = TestModel{}
		}
	}()
	type sample struct {
		key    string
		ext    string
		counts map[string]int
		body   map[string]int
		words  []string
	}
	var samples []sample
	seeds := map[string]bool{}
	extensions := map[string]bool{}
	for _, p := range sortedKeys(r.Files) {
		seed := pathSeed(p)
		f := r.Files[p]
		matched := map[string]bool{}
		for _, u := range f.Units {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, run := range runs {
				if err := ctx.Err(); err != nil {
					return err
				}
				if run.Started && run.ExitCode != nil && u.Name != "" && units.HasWord(run.Output, u.Name) {
					if root, ok := reportedTestRoot(f, u); ok {
						matched[UnitKey(p, root)] = true
					}
					break
				}
			}
		}
		for _, u := range f.Units {
			if u.Parent >= 0 {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			key := UnitKey(p, u)
			counts := TestFeatures(p, u)
			words := sortedKeys(counts)
			samples = append(samples, sample{key, path.Ext(p), counts, testCounts(r, p, u), words})
			if matched[key] || seed {
				seeds[key] = true
				r.TestUnits[key] = true
				extensions[path.Ext(p)] = true
			}
		}
	}
	r.TestSeeds = seeds
	eligible := samples[:0]
	vocab := map[string]bool{}
	for _, s := range samples {
		if !extensions[s.ext] {
			continue
		}
		eligible = append(eligible, s)
		for _, word := range s.words {
			vocab[word] = true
		}
	}
	samples = eligible
	vocabulary := sortedKeys(vocab)
	for iteration := 0; iteration < IterationBudget; iteration++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a, b := map[string]int{}, map[string]int{}
		ca, cb := 0, 0
		for _, s := range samples {
			if err := ctx.Err(); err != nil {
				return err
			}
			if r.TestUnits[s.key] {
				ca++
				for word := range s.counts {
					a[word]++
				}
			} else {
				cb++
				for word := range s.counts {
					b[word]++
				}
			}
		}
		model := TestModel{LogPrior: math.Log((float64(ca) + Prior) / (float64(cb) + Prior)), Weights: map[string]float64{}, Extensions: extensions}
		for _, w := range vocabulary {
			x, y := float64(a[w])+Prior, float64(b[w])+Prior
			xAbsent, yAbsent := float64(ca-a[w])+Prior, float64(cb-b[w])+Prior
			model.LogPrior += math.Log(xAbsent/(x+xAbsent)) - math.Log(yAbsent/(y+yAbsent))
			delta := math.Log(x/xAbsent) - math.Log(y/yAbsent)
			// 两类样本数量悬殊时，平滑项会让从未在测试样本中出现的词也得到正权重。
			// 文件名词只有确实在测试样本中出现得更频繁，才作为正向证据。
			if strings.HasPrefix(w, "file:") && delta > 0 && !(a[w] > 0 && a[w]*cb > b[w]*ca) {
				delta = 0
			}
			model.Weights[w] = delta
		}
		next := map[string]bool{}
		changed := false
		for _, s := range samples {
			if err := ctx.Err(); err != nil {
				return err
			}
			score := model.LogPrior
			for _, w := range s.words {
				score += model.Weights[w]
			}
			if seeds[s.key] || (score > 0 || score == 0 && r.TestUnits[s.key]) && model.fileEvidence(s.counts) {
				next[s.key] = true
			}
			changed = changed || next[s.key] != r.TestUnits[s.key]
		}
		r.TestUnits = next
		r.TestIterations = iteration + 1
		if !changed {
			r.Tests = model
			a, b = map[string]int{}, map[string]int{}
			bodyWords := map[string]bool{}
			for _, s := range samples {
				if err := ctx.Err(); err != nil {
					return err
				}
				for word := range s.body {
					bodyWords[word] = true
					if r.TestUnits[s.key] {
						a[word]++
					} else {
						b[word]++
					}
				}
			}
			for word := range bodyWords {
				if err := ctx.Err(); err != nil {
					return err
				}
				x, y := float64(a[word])+Prior, float64(b[word])+Prior
				xn, yn := float64(ca-a[word])+Prior, float64(cb-b[word])+Prior
				r.TestWords[word] = (math.Log(x/xn) - math.Log(y/yn)) / math.Sqrt(1/x+1/xn+1/y+1/yn)
			}
			for p, f := range r.Files {
				for _, u := range f.Units {
					if err := ctx.Err(); err != nil {
						return err
					}
					if r.TestUnits[UnitKey(p, testRoot(f, u))] {
						r.TestUnits[UnitKey(p, u)] = true
					}
				}
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			r.Converged = true
			return nil
		}
	}
	return nil
}

func testRoot(f units.File, u units.Unit) units.Unit {
	for u.Parent >= 0 {
		u = f.Units[u.Parent]
	}
	return u
}

/**
 * reportedTestRoot 把容器中报告的名字归到顶层。函数内部的同名局部节点不能证明外层函数是用例。
 */
func reportedTestRoot(f units.File, u units.Unit) (units.Unit, bool) {
	for u.Parent >= 0 {
		u = f.Units[u.Parent]
		if units.CallableHeader(u) {
			return units.Unit{}, false
		}
	}
	return u, true
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
	if !r.Converged {
		return out
	}
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
