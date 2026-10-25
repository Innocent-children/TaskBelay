package check

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type lineSource struct {
	kind, evidence string
}

type fileSources struct {
	file  gitstate.File
	lines []lineSource
}

type provenance map[string]fileSources

func lineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	count := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		count++
	}
	return count
}

func sourceLines(data []byte, kind string) []lineSource {
	lines := make([]lineSource, lineCount(data))
	for i := range lines {
		lines[i].kind = kind
	}
	return lines
}

func buildProvenance(ctx context.Context, before, after codescan.Snapshot, edits []ledger.FileChange, contents map[string][]byte) (provenance, error) {
	result := provenance{}
	for path, file := range before.Files {
		result[path] = fileSources{gitstate.File{Blob: file.Blob, Mode: file.Mode}, sourceLines(file.Data, "baseline")}
	}
	for _, edit := range edits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := result[edit.Path]
		if state.file != edit.Before {
			state = fileSources{edit.Before, sourceLines(contents[edit.Before.Blob], "unknown")}
		}
		kind := "workspace"
		if edit.Kind == "agent_edit" && edit.ToolCallID != "" {
			kind = "agent"
		}
		fragments := append([]gitstate.Fragment(nil), edit.Fragments...)
		sort.SliceStable(fragments, func(i, j int) bool { return fragments[i].BeforeStart > fragments[j].BeforeStart })
		valid := len(fragments) > 0 || edit.Before == edit.After
		for _, fragment := range fragments {
			start := fragment.BeforeStart - 1
			if fragment.BeforeLines == 0 {
				start = fragment.BeforeStart
			}
			if start < 0 || start > len(state.lines) || fragment.BeforeLines < 0 || start+fragment.BeforeLines > len(state.lines) || fragment.AfterLines < 0 {
				valid = false
				break
			}
			added := make([]lineSource, fragment.AfterLines)
			for i := range added {
				added[i] = lineSource{kind, fmt.Sprintf("%s:%d:%s", edit.Kind, edit.AfterObservation, edit.ToolCallID)}
			}
			next := append([]lineSource(nil), state.lines[:start]...)
			next = append(next, added...)
			next = append(next, state.lines[start+fragment.BeforeLines:]...)
			// 删除本身也可能改变声明，不能仅因剩余行原属 Agent 就建议整段删除。
			if kind != "agent" && fragment.AfterLines == 0 && len(next) > 0 {
				index := min(start, len(next)-1)
				next[index] = lineSource{kind, fmt.Sprintf("workspace_update:%d:delete", edit.AfterObservation)}
			}
			state.lines = next
		}
		if !valid || len(state.lines) != lineCount(contents[edit.After.Blob]) {
			state.lines = sourceLines(contents[edit.After.Blob], "unknown")
		}
		state.file = edit.After
		result[edit.Path] = state
	}
	for path, file := range after.Files {
		state := result[path]
		if state.file.Blob != file.Blob || state.file.Mode != file.Mode || len(state.lines) != lineCount(file.Data) {
			state = fileSources{gitstate.File{Blob: file.Blob, Mode: file.Mode}, sourceLines(file.Data, "unknown")}
		}
		result[path] = state
	}
	return result, nil
}

func (p provenance) at(file string, spans []codescan.Span) (agent, mixed bool, evidence []string) {
	agent = len(spans) > 0
	lines := p[file].lines
	seen := map[string]bool{}
	for _, span := range spans {
		if span.StartLine <= 0 || span.EndLine < span.StartLine || span.EndLine > len(lines) {
			agent, mixed = false, true
			continue
		}
		for _, line := range lines[span.StartLine-1 : span.EndLine] {
			if line.kind != "agent" {
				agent = false
			}
			if line.kind == "workspace" || line.kind == "unknown" {
				mixed = true
			}
			if line.evidence != "" && !seen[line.evidence] {
				evidence = append(evidence, line.evidence)
				seen[line.evidence] = true
			}
		}
	}
	sort.Strings(evidence)
	return
}
