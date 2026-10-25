package gitstate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

/**
 * 固定 tree 的目录清单只含 Git 对象，不跟随工作区符号链接或展开子模块。
 * 后续分析全部通过 blob 读取，避免检查期间读取混合版本的工作区内容。
 */
func (w Workspace) TreeFiles(ctx context.Context, tree string) (map[string]File, error) {
	if !validObjectID(tree) {
		return nil, fmt.Errorf("文件分析需要有效的固定 tree")
	}
	out, err := w.command(ctx, nil, nil, "ls-tree", "-r", "-z", "--full-tree", tree)
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
	reader := bufio.NewReader(bytes.NewReader(out))
	for _, id := range unique {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		header, err := reader.ReadString('\n')
		if err != nil {
			return result, err
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != id || fields[1] != "blob" {
			return result, fmt.Errorf("固定 tree 对象不是可读取的 blob: %s", id)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(out) {
			return result, fmt.Errorf("固定 blob 长度无效")
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(reader, data); err != nil {
			return result, err
		}
		if end, err := reader.ReadByte(); err != nil || end != '\n' {
			return result, fmt.Errorf("固定 blob 结束标记无效")
		}
		result[id] = data
	}
	return result, nil
}
