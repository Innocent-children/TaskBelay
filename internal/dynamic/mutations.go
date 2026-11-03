package dynamic

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"math/rand/v2"
	"path"
	"sort"
	"strconv"
	"strings"
)

func generate(ctx context.Context, in Input, clusters []string) ([]Mutation, error) {
	hunks := map[string]string{}
	for _, c := range in.Changes.Clusters {
		if contains(clusters, c.ID) {
			for _, h := range c.Hunks {
				hunks[h] = c.ID
			}
		}
	}
	seen := map[string]bool{}
	var out []Mutation
	add := func(cluster, p, op string, start, end int, before, after string) {
		if before == after {
			return
		}
		id := units.Hash(cluster, p, op, strconv.Itoa(start), before, after)
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, Mutation{ID: id, Cluster: cluster, Path: p, Operator: op, Extension: path.Ext(p), Start: start, End: end, Before: before, After: after})
	}
	for _, h := range in.Changes.Hunks {
		if in.After.Roles[h.Path].Test {
			continue
		}
		cluster := hunks[h.ID]
		if cluster == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return out, err
		}
		f := in.After.Files[h.Path]
		tokens := units.Significant(f.Tokens)
		for i, t := range tokens {
			if t.Line <= h.Span.AfterStart || t.Line > h.Span.AfterEnd {
				continue
			}
			value := t.Text
			end := t.End
			if i+1 < len(tokens) && tokens[i+1].Start == end {
				pair := value + tokens[i+1].Text
				switch pair {
				case "<=", ">=", "==", "!=", "&&", "||":
					value = pair
					end = tokens[i+1].End
				}
			}
			op, replacement := "", ""
			switch value {
			case "<":
				op, replacement = "relation", ">="
			case "<=":
				op, replacement = "relation", ">"
			case ">":
				op, replacement = "relation", "<="
			case ">=":
				op, replacement = "relation", "<"
			case "==":
				op, replacement = "relation", "!="
			case "!=":
				op, replacement = "relation", "=="
			case "+":
				op, replacement = "arithmetic", "-"
			case "-":
				op, replacement = "arithmetic", "+"
			case "*":
				op, replacement = "arithmetic", "/"
			case "/":
				op, replacement = "arithmetic", "*"
			case "&&":
				op, replacement = "boolean", "||"
			case "||":
				op, replacement = "boolean", "&&"
			case "true":
				op, replacement = "literal", "false"
			case "false":
				op, replacement = "literal", "true"
			}
			if op != "" {
				add(cluster, h.Path, op, t.Start, end, value, replacement)
			}
			if t.Kind == "number" {
				if n, err := strconv.ParseInt(t.Text, 0, 64); err == nil {
					if n < int64(^uint64(0)>>1) {
						add(cluster, h.Path, "integer", t.Start, t.End, t.Text, strconv.FormatInt(n+1, 10))
					}
					if n > -int64(^uint64(0)>>1)-1 {
						add(cluster, h.Path, "integer", t.Start, t.End, t.Text, strconv.FormatInt(n-1, 10))
					}
				}
			}
		}
		offset := 0
		for i, line := range f.Lines {
			if i >= h.Span.AfterStart && i < h.Span.AfterEnd && strings.TrimSpace(line) != "" {
				add(cluster, h.Path, "line", offset, offset+len(line), line, "")
			}
			offset += len(line)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *runner) mutations() error {
	clusters := append([]string(nil), r.result.Required...)
	if len(clusters) == 0 {
		var scores []float64
		for _, c := range r.input.Changes.Clusters {
			scores = append(scores, r.input.Confidence[c.ID])
		}
		sort.Float64s(scores)
		if len(scores) > 0 {
			middle := scores[len(scores)/2]
			for _, c := range r.input.Changes.Clusters {
				if !contains(r.result.Rollback, c.ID) && r.input.Confidence[c.ID] <= middle {
					clusters = append(clusters, c.ID)
				}
			}
		}
	}
	supported := r.supported()
	var selected []string
	for _, id := range clusters {
		if supported[id] {
			selected = append(selected, id)
		}
	}
	mutants, err := generate(r.search, r.input, selected)
	if err != nil {
		return err
	}
	layers := map[string][]Mutation{}
	for _, m := range mutants {
		layers[m.Cluster+"\x00"+m.Operator] = append(layers[m.Cluster+"\x00"+m.Operator], m)
	}
	seed := sha256.Sum256([]byte(r.input.Tree))
	random := rand.New(rand.NewPCG(binary.LittleEndian.Uint64(seed[:8]), binary.LittleEndian.Uint64(seed[8:16])))
	classes := map[string]int{}
	valid := 0
	for sampled := 0; sampled < MutationLimit; sampled++ {
		if err := r.search.Err(); err != nil {
			r.result.Gaps = append(r.result.Gaps, "变异抽样预算到期，保留已完成发现矩阵")
			r.result.Incomplete = append(r.result.Incomplete, "变异抽样预算到期，保留已完成发现矩阵")
			break
		}
		total := 0
		for _, key := range sorted(layers) {
			total += len(layers[key])
		}
		if total == 0 {
			break
		}
		pick := random.IntN(total)
		var mutant Mutation
		for _, key := range sorted(layers) {
			if pick >= len(layers[key]) {
				pick -= len(layers[key])
				continue
			}
			mutant = layers[key][pick]
			layers[key] = append(layers[key][:pick], layers[key][pick+1:]...)
			break
		}
		operatorKey := mutant.Extension + ":" + mutant.Operator
		stat := r.result.Operators[operatorKey]
		if stat.Disabled {
			continue
		}
		f := r.input.After.Files[mutant.Path]
		text := strings.Join(f.Lines, "")
		if mutant.Start < 0 || mutant.End > len(text) || text[mutant.Start:mutant.End] != mutant.Before {
			return fmt.Errorf("变异与固定内容不对应")
		}
		patch := Patch{Path: mutant.Path, Before: text, After: text[:mutant.Start] + mutant.After + text[mutant.End:], Mode: f.Mode}
		runs, runErr := r.commands(r.search, "mutation:"+mutant.ID, r.base, []Patch{patch}, r.input.Commands)
		row := Discovery{Mutation: mutant, Runs: runs, State: "unknown"}
		if runErr != nil {
			r.result.Gaps = append(r.result.Gaps, runErr.Error())
			r.result.Incomplete = append(r.result.Incomplete, runErr.Error())
			break
		}
		named := false
		invalid := false
		uncertain := false
		for _, run := range runs {
			invalid = invalid || run.State == "invalid"
			uncertain = uncertain || run.State != "passed" && run.State != "failed" && run.State != "invalid"
			failureFound := false
			for _, c := range run.Cases {
				if c.Name == run.Command {
					continue
				}
				if c.State == "passed" || c.State == "failed" {
					named = true
				} else {
					uncertain = true
				}
				if c.State == "failed" {
					failureFound = true
					matches := []string{}
					for _, candidate := range r.input.Signals.Cases {
						if candidate.Name == c.Name {
							matches = append(matches, candidate.ID)
						}
					}
					if len(matches) == 1 {
						row.Cases = append(row.Cases, matches[0])
					} else {
						uncertain = true
					}
				}
			}
			uncertain = uncertain || run.State == "failed" && !failureFound
		}
		if invalid {
			row.State = "invalid"
			stat.Invalid++
		} else if !uncertain && (named || allPassed(runs)) {
			row.State = "valid"
			stat.Valid++
			valid++
			sort.Strings(row.Cases)
			row.Cases = unique(row.Cases)
			classes[strings.Join(row.Cases, "\x00")]++
			if len(row.Cases) == 0 {
				r.result.TestGaps = append(r.result.TestGaps, mutant.Path+":"+strconv.Itoa(mutant.Start)+" "+mutant.Before+" → "+mutant.After)
			}
		} else {
			stat.Unknown++
			r.result.Gaps = append(r.result.Gaps, "无法区分变异的构建失败与用例失败："+mutant.ID)
		}
		stat.Disabled = stat.Invalid > 0 && repostats.BinomialTail(stat.Invalid, stat.Valid+stat.Invalid, InvalidProbability) < Significance
		r.result.Operators[operatorKey] = stat
		r.result.Matrix = append(r.result.Matrix, row)
		singletons := 0
		for _, n := range classes {
			if n == 1 {
				singletons++
			}
		}
		if valid > 0 {
			r.result.UnseenProbability = float64(singletons) / float64(valid)
		}
		if err = r.emit(); err != nil {
			return err
		}
		if valid > 0 && r.result.UnseenProbability < Epsilon {
			break
		}
	}
	return r.reduce()
}

func allPassed(runs []RunResult) bool {
	if len(runs) == 0 {
		return false
	}
	for _, run := range runs {
		if run.State != "passed" {
			return false
		}
	}
	return true
}

func unique(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	sort.Strings(items)
	out := items[:1]
	for _, s := range items[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
