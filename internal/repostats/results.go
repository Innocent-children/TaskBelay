package repostats

import (
	"context"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/units"
)

type Run struct {
	ID, Output string
	ExitCode   *int
	Started    bool
}
type ResultModel struct{ Cases, Pass, Fail []string }
type CaseResult struct{ Name, State string }

func LearnResults(runs []Run, names []string) ResultModel {
	m := ResultModel{Cases: append([]string(nil), names...)}
	pass, fail := map[string]bool{}, map[string]bool{}
	for _, run := range runs {
		if !run.Started || run.ExitCode == nil {
			continue
		}
		for _, line := range strings.Split(run.Output, "\n") {
			for _, name := range names {
				at := strings.Index(line, name)
				if at < 0 {
					continue
				}
				words := units.Words(line[:at] + " " + line[at+len(name):])
				for _, w := range words[max(0, len(words)-OutputRadius):] {
					if *run.ExitCode == 0 {
						pass[w] = true
					} else {
						fail[w] = true
					}
				}
			}
		}
	}
	for _, w := range sortedKeys(pass) {
		if !fail[w] {
			m.Pass = append(m.Pass, w)
		}
	}
	for _, w := range sortedKeys(fail) {
		if !pass[w] {
			m.Fail = append(m.Fail, w)
		}
	}
	return m
}

func ParseResults(ctx context.Context, m ResultModel, run Run) ([]CaseResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	states := map[string]string{}
	pass, fail := map[string]bool{}, map[string]bool{}
	for _, w := range m.Pass {
		pass[w] = true
	}
	for _, w := range m.Fail {
		fail[w] = true
	}
	matcher, err := units.NewMatcher(ctx, m.Cases)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(run.Output, "\n") {
		matches, err := matcher.Find(ctx, line)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			state := "unknown"
			for _, w := range units.Words(line[:match.Start] + " " + line[match.End:]) {
				if fail[w] {
					state = "failed"
					break
				}
				if pass[w] {
					state = "passed"
				}
			}
			if states[match.Pattern] == "failed" {
				continue
			}
			states[match.Pattern] = state
		}
	}
	var result []CaseResult
	for _, name := range sortedKeys(states) {
		result = append(result, CaseResult{name, states[name]})
	}
	if len(result) == 0 {
		state := "unknown"
		if run.Started && run.ExitCode != nil {
			if *run.ExitCode == 0 {
				state = "passed"
			} else {
				state = "failed"
			}
		}
		result = append(result, CaseResult{run.ID, state})
	}
	return result, nil
}
