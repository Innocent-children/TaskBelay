package hook

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

type textLine struct{ text, ending string }

/**
 * 按上下文定位的行补丁保留原有行结束符；插入行沿用第一个行结束符。
 * 核对只接受唯一的精确匹配，不模仿编辑工具的空白容错来猜测旧原文。
 */
func applyTextHunks(current string, hunks []event.TextHunk) (*string, error) {
	var lines []textLine
	preferred := ""
	start := 0
	for i := 0; i < len(current); i++ {
		if current[i] != '\r' && current[i] != '\n' {
			continue
		}
		end := i + 1
		if current[i] == '\r' && end < len(current) && current[end] == '\n' {
			end++
		}
		ending := current[i:end]
		if preferred == "" {
			preferred = ending
		}
		lines = append(lines, textLine{current[start:i], ending})
		start = end
		i = end - 1
	}
	if start < len(current) {
		lines = append(lines, textLine{text: current[start:]})
	}
	if preferred == "" {
		preferred = "\n"
	}
	type replacement struct {
		start, end int
		lines      []textLine
	}
	var replacements []replacement
	offset := 0
	find := func(before []string, offset int, atEnd bool) (int, int) {
		match := -1
		for i := offset; i+len(before) <= len(lines); i++ {
			if atEnd && i+len(before) != len(lines) {
				continue
			}
			ok := true
			for j, text := range before {
				if lines[i+j].text != text {
					ok = false
					break
				}
			}
			if ok {
				if match != -1 {
					return match, i
				}
				match = i
			}
		}
		return match, -1
	}
	var ambiguity error
	var alternativeEnd int
	for _, hunk := range hunks {
		if hunk.Anchor != nil {
			at, second := find([]string{*hunk.Anchor}, offset, false)
			if second >= 0 {
				return nil, fmt.Errorf("补丁锚点有多个匹配位置")
			}
			if at < 0 {
				return nil, ambiguity
			}
			offset = at + 1
		}
		var before []string
		for _, line := range hunk.Lines {
			if line == "" || !strings.ContainsRune(" +-", rune(line[0])) {
				return nil, fmt.Errorf("无效的行补丁")
			}
			if line[0] != '+' {
				before = append(before, line[1:])
			}
		}
		at, second := len(lines), -1
		if len(before) > 0 {
			at, second = find(before, offset, hunk.AtEnd)
			if at < 0 {
				return nil, ambiguity
			}
		}
		if ambiguity != nil {
			// 只有本段原文位置唯一，且前段的第二个匹配放不下时，才能采用前段的第一个匹配。
			if len(before) == 0 || second >= 0 || alternativeEnd <= at {
				return nil, ambiguity
			}
			ambiguity = nil
		}
		if second >= 0 {
			ambiguity = fmt.Errorf("补丁上下文有多个匹配位置")
			alternativeEnd = second + len(before)
		}
		r := replacement{start: at, end: at + len(before)}
		old := at
		for _, line := range hunk.Lines {
			switch line[0] {
			case ' ':
				r.lines = append(r.lines, lines[old])
				old++
			case '-':
				old++
			case '+':
				r.lines = append(r.lines, textLine{line[1:], preferred})
			}
		}
		replacements = append(replacements, r)
		if len(before) > 0 {
			offset = r.end
		}
	}
	if ambiguity != nil {
		return nil, ambiguity
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].start < replacements[j].start })
	var result []textLine
	offset = 0
	for _, r := range replacements {
		if r.start < offset {
			return nil, fmt.Errorf("补丁片段重叠")
		}
		result = append(result, lines[offset:r.start]...)
		result = append(result, r.lines...)
		offset = r.end
	}
	result = append(result, lines[offset:]...)
	var output strings.Builder
	for _, line := range result {
		output.WriteString(line.text)
		if line.ending == "" {
			line.ending = preferred
		}
		output.WriteString(line.ending)
	}
	text := output.String()
	return &text, nil
}
