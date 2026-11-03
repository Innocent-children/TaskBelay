package check

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"math"
	"sort"
	"strings"
)

func fit(ctx context.Context, labels []ledger.CalibrationLabel) (map[string]float64, error) {
	w := map[string]float64{}
	for k, v := range initialWeights {
		w[k] = v
	}
	for _, label := range labels {
		for k := range label.Signals {
			if _, ok := w[k]; !ok {
				w[k] = 0
			}
		}
	}
	names := keys(w)
	for i := 0; i < FitIterations && len(labels) > 0; i++ {
		if err := ctx.Err(); err != nil {
			return w, err
		}
		g := map[string]float64{}
		for _, k := range names {
			g[k] = Regularization * (w[k] - initialWeights[k])
		}
		for _, label := range labels {
			error := probability(w, label.Signals)
			if label.Accepted {
				error--
			}
			g["bias"] += error
			for _, k := range names {
				if k != "bias" {
					g[k] += error * label.Signals[k]
				}
			}
		}
		norm := 0.0
		for _, k := range names {
			step := FitRate * g[k] / float64(len(labels)+1)
			w[k] -= step
			norm += step * step
		}
		if norm < FitPrecision*FitPrecision {
			break
		}
	}
	return w, nil
}

func probability(w, x map[string]float64) float64 {
	z := w["bias"]
	for _, k := range keys(x) {
		z += w[k] * x[k]
	}
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}

type selection struct {
	value float64
	ids   []int
}

// 分类内先做有数量约束的背包，再按分类卷积；同收益保留稳定编号靠前的集合。
func selectFeedback(ctx context.Context, candidates []signals.Candidate, w map[string]float64) ([]signals.Candidate, error) {
	capacity := FeedbackBudget / FeedbackUnit
	groups := map[string][]int{}
	for i, c := range candidates {
		if c.BlockFact == "" {
			groups[c.Kind] = append(groups[c.Kind], i)
		}
	}
	best := map[int]selection{0: {}}
	for _, kind := range keys(groups) {
		local := make([]map[int]selection, CategoryLimit+1)
		for i := range local {
			local[i] = map[int]selection{}
		}
		local[0][0] = selection{}
		for _, id := range groups[kind] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			c := candidates[id]
			cost := max(1, (len([]rune(c.Advice+strings.Join(c.Facts, "；")+c.At))+FeedbackUnit-1)/FeedbackUnit)
			value := max(1, c.Benefit) * probability(w, c.Features)
			for count := CategoryLimit; count > 0; count-- {
				for _, used := range intKeys(local[count-1]) {
					prior := local[count-1][used]
					if used+cost > capacity {
						continue
					}
					v := selection{prior.value + value, append(append([]int(nil), prior.ids...), id)}
					if old, ok := local[count][used+cost]; !ok || v.value > old.value {
						local[count][used+cost] = v
					}
				}
			}
		}
		options := map[int]selection{}
		for _, row := range local {
			for _, cost := range intKeys(row) {
				v := row[cost]
				if old, ok := options[cost]; !ok || v.value > old.value {
					options[cost] = v
				}
			}
		}
		next := map[int]selection{}
		for _, a := range intKeys(best) {
			for _, b := range intKeys(options) {
				if a+b > capacity {
					continue
				}
				x, y := best[a], options[b]
				v := selection{x.value + y.value, append(append([]int(nil), x.ids...), y.ids...)}
				if old, ok := next[a+b]; !ok || v.value > old.value {
					next[a+b] = v
				}
			}
		}
		best = next
	}
	chosen := selection{}
	for _, cost := range intKeys(best) {
		if best[cost].value > chosen.value {
			chosen = best[cost]
		}
	}
	var out []signals.Candidate
	for _, c := range candidates {
		if c.BlockFact != "" {
			out = append(out, c)
		}
	}
	for _, id := range chosen.ids {
		out = append(out, candidates[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].BlockFact != "") != (out[j].BlockFact != "") {
			return out[i].BlockFact != ""
		}
		a, b := probability(w, out[i].Features), probability(w, out[j].Features)
		if a != b {
			return a > b
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
func intKeys[V any](m map[int]V) []int {
	v := make([]int, 0, len(m))
	for k := range m {
		v = append(v, k)
	}
	sort.Ints(v)
	return v
}
