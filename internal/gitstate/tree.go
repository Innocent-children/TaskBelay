package gitstate

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
)

/**
 * 固定 tree 的目录清单只含 Git 对象，不跟随工作区符号链接或展开子模块。
 * 后续分析全部通过 blob 读取，避免检查期间读取混合版本的工作区内容。
 */
func (w Workspace) TreeFiles(ctx context.Context, tree string, paths ...string) (map[string]File, error) {
	if !validObjectID(tree) {
		return nil, fmt.Errorf("文件分析需要有效的固定 tree")
	}
	args := []string{"--literal-pathspecs", "ls-tree", "-r", "-z", "--full-tree", tree}
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	out, err := w.command(ctx, nil, nil, args...)
	if err != nil {
		return nil, err
	}
	files := make(map[string]File)
	for _, row := range bytes.Split(out, []byte{0}) {
		if len(row) == 0 {
			continue
		}
		metadata, path, ok := bytes.Cut(row, []byte{'\t'})
		fields := strings.Fields(string(metadata))
		if !ok || len(fields) != 3 || !validObjectID(fields[2]) {
			return nil, fmt.Errorf("无法读取固定 tree 的文件条目")
		}
		files[string(path)] = File{Blob: fields[2], Mode: fields[0]}
	}
	return files, nil
}

/**
 * TreeContains 返回 patterns 中哪些文本原样出现在 tree 的文件里。
 * 所有文本交给一次 git grep 做固定字符串匹配，不逐个文件读取。
 */
func (w Workspace) TreeContains(ctx context.Context, tree string, patterns []string) (map[string]bool, error) {
	found := map[string]bool{}
	var input strings.Builder
	for _, pattern := range patterns {
		if pattern != "" && !strings.ContainsAny(pattern, "\n\r\x00") {
			input.WriteString(pattern + "\n")
		}
	}
	if input.Len() == 0 {
		return found, nil
	}
	out, err := w.command(ctx, strings.NewReader(input.String()), nil, "grep", "-I", "-h", "-F", "-f", "-", tree)
	if err != nil && !isExit(err, 1) {
		return nil, err
	}
	// 同一行可能同时包含多段文本，逐行核对，不依赖 grep 只报告第一处匹配。
	for _, line := range strings.Split(string(out), "\n") {
		for _, pattern := range patterns {
			if !found[pattern] && pattern != "" && strings.Contains(line, pattern) {
				found[pattern] = true
			}
		}
	}
	return found, nil
}

func (w Workspace) ReadBlobs(ctx context.Context, ids []string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	var request strings.Builder
	var unique []string
	seen := make(map[string]bool)
	for _, id := range ids {
		if !validObjectID(id) {
			return nil, fmt.Errorf("无效的 blob 标识")
		}
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
			fmt.Fprintln(&request, id)
		}
	}
	if len(unique) == 0 {
		return result, nil
	}
	out, err := w.command(ctx, strings.NewReader(request.String()), nil, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	for _, id := range unique {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		header, data, ok := bytes.Cut(out, []byte{'\n'})
		if !ok {
			return result, fmt.Errorf("固定 blob 缺少对象头: %s", id)
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 || fields[0] != id || fields[1] != "blob" {
			return result, fmt.Errorf("固定 tree 对象不是可读取的 blob: %s", id)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(data) {
			return result, fmt.Errorf("固定 blob 长度无效")
		}
		if size == len(data) || data[size] != '\n' {
			return result, fmt.Errorf("固定 blob 结束标记无效")
		}
		// 共享这一批的输出，限制容量以免追加内容时覆盖下一个 blob。
		result[id] = data[:size:size]
		out = data[size+1:]
	}
	return result, nil
}
