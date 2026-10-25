package codex

import (
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

/**
 * apply_patch 转为独立于 Agent 的行补丁，保留定位锚点和文件结束约束。
 * 没有前置内容的删除、移动不凭当前文件补造请求证据。
 */
func parsePatch(input string) ([]event.FileEdit, error) {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(input, "\r\n", "\n")), "\n")
	if len(lines) < 3 || lines[0] != "*** Begin Patch" || lines[len(lines)-1] != "*** End Patch" {
		return nil, fmt.Errorf("apply_patch 缺少完整的补丁边界")
	}
	var edits []event.FileEdit
	for i := 1; i < len(lines)-1; {
		header := lines[i]
		i++
		var edit event.FileEdit
		switch {
		case strings.HasPrefix(header, "*** Add File: "):
			edit.Path, edit.WholeFile = strings.TrimPrefix(header, "*** Add File: "), true
			var content strings.Builder
			for i < len(lines)-1 && !strings.HasPrefix(lines[i], "*** ") {
				if !strings.HasPrefix(lines[i], "+") {
					return nil, fmt.Errorf("apply_patch 新文件含无效内容行")
				}
				content.WriteString(lines[i][1:])
				content.WriteByte('\n')
				i++
			}
			value := content.String()
			edit.After = &value
		case strings.HasPrefix(header, "*** Delete File: "):
			edit.Path, edit.Delete = strings.TrimPrefix(header, "*** Delete File: "), true
			edit.Unverifiable = "删除请求没有前置内容；实际变化保留为工作区更新"
		case strings.HasPrefix(header, "*** Update File: "):
			edit.Path = strings.TrimPrefix(header, "*** Update File: ")
			var move string
			if i < len(lines)-1 && strings.HasPrefix(lines[i], "*** Move to: ") {
				move = strings.TrimPrefix(lines[i], "*** Move to: ")
				i++
			}
			var hunk *event.TextHunk
			for i < len(lines)-1 {
				line := lines[i]
				if line == "*** End of File" {
					if hunk == nil {
						return nil, fmt.Errorf("文件结束标记缺少补丁内容")
					}
					hunk.AtEnd = true
					i++
					continue
				}
				if strings.HasPrefix(line, "*** ") {
					break
				}
				if line == "@@" || strings.HasPrefix(line, "@@ ") {
					edit.Hunks = append(edit.Hunks, event.TextHunk{})
					hunk = &edit.Hunks[len(edit.Hunks)-1]
					if line != "@@" {
						anchor := strings.TrimPrefix(line, "@@ ")
						hunk.Anchor = &anchor
					}
				} else {
					if hunk != nil && hunk.AtEnd && line == "" {
						i++
						continue
					}
					if hunk == nil {
						edit.Hunks = append(edit.Hunks, event.TextHunk{})
						hunk = &edit.Hunks[0]
					}
					if line == "" {
						line = " "
					}
					if !strings.ContainsRune(" +-", rune(line[0])) {
						return nil, fmt.Errorf("apply_patch 包含无效补丁行")
					}
					hunk.Lines = append(hunk.Lines, line)
				}
				i++
			}
			if len(edit.Hunks) == 0 {
				return nil, fmt.Errorf("apply_patch 更新缺少上下文")
			}
			for _, hunk := range edit.Hunks {
				if len(hunk.Lines) == 0 {
					return nil, fmt.Errorf("apply_patch 包含空补丁段")
				}
			}
			if move != "" {
				edit.Unverifiable = "移动补丁的源文件与目标文件需结合实际结果核对；本次保留为工作区更新"
				edits = append(edits, event.FileEdit{Path: move, Unverifiable: edit.Unverifiable})
			}
		default:
			return nil, fmt.Errorf("无法识别 apply_patch 文件标记")
		}
		if edit.Path == "" {
			return nil, fmt.Errorf("apply_patch 文件路径为空")
		}
		edits = append(edits, edit)
	}
	if len(edits) == 0 {
		return nil, fmt.Errorf("apply_patch 没有文件编辑")
	}
	return edits, nil
}
