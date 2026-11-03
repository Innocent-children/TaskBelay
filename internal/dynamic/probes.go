package dynamic

import (
	"context"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"path/filepath"
	"strings"
	"unicode"
)

func interfaceWords(text string) []string {
	var out []string
	for _, word := range strings.Fields(text) {
		word = strings.Trim(word, "\"'`,;()[]{}")
		if strings.HasPrefix(word, "--") || strings.HasPrefix(word, "/") {
			out = append(out, word)
		}
	}
	// 同一列的短键加说明也是接口清单；普通句子不会形成重复的两列版式。
	rows := map[string][]string{}
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		key := fields[0]
		valid := true
		for _, r := range key {
			valid = valid && (unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-", r))
		}
		if !valid {
			continue
		}
		rest := trimmed[len(key):]
		gap := len(rest) - len(strings.TrimLeft(rest, " \t"))
		if gap < ProbeColumnGap && !strings.HasPrefix(rest, "\t") {
			continue
		}
		column := line[:len(line)-len(trimmed)]
		rows[column] = append(rows[column], key)
	}
	for _, column := range sorted(rows) {
		keys := unique(rows[column])
		if repostats.BinomialTail(len(keys), max(len(lines), len(keys)), 1/float64(len(lines)+1)) < Significance {
			out = append(out, keys...)
		}
	}
	return unique(out)
}

func (r *runner) probes(ctx context.Context, current []RunResult) error {
	origin := ""
	count := 0
	for _, command := range r.input.Commands {
		if count >= ProbeLimit {
			break
		}
		tokens := strings.Fields(command.Output)
		shapes := interfaceWords(command.Output)
		if len(shapes) == 0 || repostats.BinomialTail(len(shapes), max(len(tokens), len(shapes)), 1/float64(len(tokens)+1)) >= Significance {
			continue
		}
		if origin == "" {
			origin = filepath.Join(r.root, "origin")
			if err := r.input.Workspace.ExportTree(ctx, r.input.Before.Tree, origin); err != nil {
				return err
			}
		}
		count++
		runs, err := r.commands(ctx, "probe-origin", origin, nil, []Command{command})
		if err != nil {
			return err
		}
		probe := Probe{Command: command.ID, State: "unknown", Runs: runs}
		var latest RunResult
		for _, run := range current {
			if run.Command == command.ID {
				latest = run
				probe.Runs = append(probe.Runs, run)
			}
		}
		if len(runs) > 0 && runs[0].State == "passed" && latest.State == "passed" {
			old := map[string]bool{}
			for _, word := range interfaceWords(runs[0].Output) {
				old[word] = true
			}
			for _, word := range interfaceWords(latest.Output) {
				if !old[word] {
					probe.Added = append(probe.Added, word)
				}
			}
			probe.State = "confirmed"
			probe.Added = unique(probe.Added)
		}
		r.result.Probes = append(r.result.Probes, probe)
	}
	return r.emit()
}
