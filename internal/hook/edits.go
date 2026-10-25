package hook

import (
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/event"
)

/**
 * 每次都核对具体原文。此前的冲突提示不能成为覆盖过期内容的授权；
 * 无法解析的补丁保留为未核对，由适配层按自身故障处理。
 */
func expectedContent(edit event.FileEdit, current string, exists, updated bool) (content string, verifiable, conflict bool, reason string) {
	if edit.Unverifiable != "" {
		return "", false, false, edit.Unverifiable
	}
	if len(edit.Hunks) > 0 {
		if !exists {
			return "", false, true, "补丁目标已不存在"
		}
		content, err := applyTextHunks(current, edit.Hunks)
		if err != nil {
			return "", false, false, "补丁格式无法核对: " + err.Error()
		}
		if content == nil {
			return "", false, true, "补丁上下文与当前文件不一致或无法唯一定位"
		}
		return *content, true, false, ""
	}
	if edit.Delete {
		if !exists {
			return "", true, false, ""
		}
		if edit.Before == nil || *edit.Before != current {
			return "", false, true, "删除请求缺少与当前文件一致的前置内容"
		}
		return "", true, false, ""
	}
	if edit.Patch != "" {
		content, err := applyPatch(current, edit.Patch)
		if err != nil {
			return "", false, false, "补丁格式无法核对: " + err.Error()
		}
		if content == nil {
			return "", false, true, "补丁上下文与当前文件不一致或无法唯一定位"
		}
		return *content, true, false, ""
	}
	if edit.After == nil {
		return "", false, false, "请求未提供可核对的目标内容"
	}
	if edit.WholeFile || edit.Before == nil {
		if edit.Before != nil && (!exists || *edit.Before != current) {
			return "", false, true, "整文件编辑的前置内容已经过期"
		}
		if edit.Before == nil && exists && updated {
			return "", false, true, "整文件写入会覆盖工作区更新；请读取当前文件并改用带当前上下文的编辑"
		}
		return *edit.After, true, false, ""
	}
	count := strings.Count(current, *edit.Before)
	if !exists || *edit.Before == "" || count == 0 || !edit.ReplaceAll && count != 1 {
		return "", false, true, "请求中的原文与当前内容不一致或无法唯一定位"
	}
	if edit.ReplaceAll {
		return strings.ReplaceAll(current, *edit.Before, *edit.After), true, false, ""
	}
	return strings.Replace(current, *edit.Before, *edit.After, 1), true, false, ""
}

/**
 * 这里只处理标准化的 unified hunk。Agent 外层格式由 G04 的适配器转换。
 * 上下文须精确匹配，保留空白和文件末尾换行，不按旧行号模糊套用补丁。
 */
func applyPatch(current, patch string) (*string, error) {
	type replacement struct{ before, after string }
	var hunks []replacement
	var last byte
	for _, line := range strings.SplitAfter(patch, "\n") {
		if strings.HasPrefix(line, "@@") {
			hunks = append(hunks, replacement{})
			last = 0
			continue
		}
		if len(hunks) == 0 {
			if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") {
				continue
			}
			if line == "" {
				continue
			}
			return nil, fmt.Errorf("缺少 hunk 上下文")
		}
		if line == "" {
			continue
		}
		hunk := &hunks[len(hunks)-1]
		switch line[0] {
		case ' ':
			hunk.before += line[1:]
			hunk.after += line[1:]
			last = ' '
		case '-':
			hunk.before += line[1:]
			last = '-'
		case '+':
			hunk.after += line[1:]
			last = '+'
		case '\\':
			if !strings.HasPrefix(line, "\\ No newline at end of file") {
				return nil, fmt.Errorf("无法识别补丁行")
			}
			if last == '-' || last == ' ' {
				hunk.before = strings.TrimSuffix(hunk.before, "\n")
			}
			if last == '+' || last == ' ' {
				hunk.after = strings.TrimSuffix(hunk.after, "\n")
			}
		default:
			return nil, fmt.Errorf("无法识别补丁行")
		}
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("补丁没有 hunk")
	}
	offset := 0
	for _, hunk := range hunks {
		if hunk.before == "" {
			if current != "" || len(hunks) != 1 {
				return nil, nil
			}
			current = hunk.after
			continue
		}
		if strings.Count(current[offset:], hunk.before) != 1 {
			return nil, nil
		}
		start := offset + strings.Index(current[offset:], hunk.before)
		current = current[:start] + hunk.after + current[start+len(hunk.before):]
		offset = start + len(hunk.after)
	}
	return &current, nil
}
