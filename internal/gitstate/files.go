package gitstate

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

type File struct {
	Blob string
	Mode string
}

type Incomplete struct {
	Path   string
	Reason string
}

func isExit(err error, code int) bool {
	var target *gitExitError
	return errors.As(err, &target) && target.Code == code
}

/**
 * 新文件从最近的已存在父目录定位，已有别名解析到实际路径。
 * 路径拼写取自文件系统；无法查询大小写规则时，不折叠新文件名。
 */
func (w Workspace) TargetPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.Root, path)
	}
	path = filepath.Clean(path)
	var tail []string
	for {
		_, err := os.Lstat(path)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		path = parent
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	path, err = actualSpelling(path, w.Root)
	if err != nil {
		return "", err
	}
	for i := len(tail) - 1; i >= 0; i-- {
		path = filepath.Join(path, tail[i])
	}
	rel, err := filepath.Rel(w.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == "." {
		return "", fmt.Errorf("目标不在当前工作区内: %s", path)
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".git" {
			return "", fmt.Errorf("目标是 git 元数据: %s", path)
		}
	}
	// 子仓库的编辑须使用它自己的观察历史。
	for parent := filepath.Dir(path); parent != w.Root; parent = filepath.Dir(parent) {
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return "", fmt.Errorf("目标属于独立工作区: %s", parent)
		}
	}
	return filepath.ToSlash(rel), nil
}

func actualSpelling(path, root string) (string, error) {
	// Git 已给出规范根目录；正常目标不必重新枚举工作区之外的父目录。
	if path == root {
		return path, nil
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	parent, err := actualSpelling(parent, root)
	if err != nil {
		return "", err
	}
	name := filepath.Base(path)
	path = filepath.Join(parent, name)
	rule := platform.DirectoryCaseRule(parent)
	if rule == platform.CaseSensitive {
		return path, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.Name() == name {
			return path, nil
		}
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), name) {
			other, err := entry.Info()
			if err == nil && os.SameFile(info, other) {
				return filepath.Join(parent, entry.Name()), nil
			}
		}
	}
	return path, nil
}

/**
 * 比较键只折叠实际不区分大小写目录下的新名字，保留用于访问文件的原路径。
 * 父目录用文件身份隔开，避免把大小写敏感父目录下的两个不同子目录合并。
 */
func (w Workspace) TargetKey(path string) (string, error) {
	relative, err := w.TargetPath(path)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Join(w.Root, filepath.FromSlash(relative)))
	suffix := filepath.Base(relative)
	for {
		info, err := os.Stat(parent)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("目标父路径不是目录")
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = filepath.Dir(parent)
	}
	identity, err := platform.FileIdentity(parent)
	if err != nil {
		return "", err
	}
	if platform.DirectoryCaseRule(parent) == platform.CaseInsensitive {
		suffix = strings.ToLower(suffix)
	}
	return identity + ":" + filepath.ToSlash(suffix), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (w Workspace) WriteBlob(ctx context.Context, content []byte) (string, error) {
	out, err := w.command(ctx, bytes.NewReader(content), nil, "hash-object", "-w", "--stdin", "--no-filters")
	return strings.TrimSpace(string(out)), err
}

func (w Workspace) ReadBlob(ctx context.Context, blob string) ([]byte, error) {
	if blob == "" {
		return nil, nil
	}
	if !validObjectID(blob) {
		return nil, fmt.Errorf("无效的 blob 标识")
	}
	return w.command(ctx, nil, nil, "cat-file", "blob", blob)
}

func validObjectID(id string) bool {
	if len(id) != 40 && len(id) != 64 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

/**
 * 编辑证据保存原始字节，避免过滤器和换行转换改变待核对内容。
 * 特殊文件记为未完成，不尝试阻塞读取 FIFO 或设备。
 */
func (w Workspace) ReadFile(ctx context.Context, path string) (File, []byte, error) {
	return w.ReadObservedFile(ctx, path, File{})
}

/**
 * 始终读取当前字节和模式，只复用内容完全相同的已保留 blob。
 * 文件大小、时间和已有观察不能证明内容未变；不一致时仍由 Git 保存新证据。
 */
func (w Workspace) ReadObservedFile(ctx context.Context, path string, observed File) (File, []byte, error) {
	if err := ctx.Err(); err != nil {
		return File{}, nil, err
	}
	if filepath.IsAbs(path) || path == "" || path == "." || filepath.Clean(path) == ".." || strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator)) {
		return File{}, nil, fmt.Errorf("无效的工作区文件路径: %s", path)
	}
	full := filepath.Join(w.Root, filepath.FromSlash(path))
	parent := filepath.Dir(full)
	if parent != w.Root {
		// 只解析父目录；Git 保存末尾符号链接本身的内容。
		resolved, err := w.TargetPath(parent)
		if err != nil {
			return File{}, nil, err
		}
		parent = filepath.Join(w.Root, filepath.FromSlash(resolved))
		if _, err := os.Lstat(filepath.Join(parent, ".git")); err == nil {
			return File{}, nil, fmt.Errorf("目标属于独立工作区: %s", parent)
		}
		full = filepath.Join(parent, filepath.Base(full))
	}
	info, err := os.Lstat(full)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil, nil
	}
	if err != nil {
		return File{}, nil, err
	}
	var content []byte
	mode := "100644"
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(full)
		if err != nil {
			return File{}, nil, err
		}
		content, mode = []byte(target), "120000"
	} else {
		if !info.Mode().IsRegular() {
			return File{}, nil, fmt.Errorf("不是普通文件: %s", path)
		}
		file, err := os.Open(full)
		if err != nil {
			return File{}, nil, err
		}
		content, err = io.ReadAll(contextReader{ctx, file})
		file.Close()
		if err != nil {
			return File{}, nil, err
		}
		after, err := os.Lstat(full)
		if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || info.ModTime() != after.ModTime() || info.Mode() != after.Mode() {
			return File{}, nil, fmt.Errorf("采集时文件发生变化: %s", path)
		}
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
	}
	if matches, err := matchesBlob(ctx, content, observed.Blob); err != nil {
		return File{}, nil, err
	} else if matches {
		return File{Blob: observed.Blob, Mode: mode}, content, nil
	}
	blob, err := w.WriteBlob(ctx, content)
	return File{Blob: blob, Mode: mode}, content, err
}

func matchesBlob(ctx context.Context, content []byte, blob string) (bool, error) {
	if !validObjectID(blob) {
		return false, nil
	}
	var digest hash.Hash
	if len(blob) == 40 {
		digest = sha1.New()
	} else {
		digest = sha256.New()
	}
	fmt.Fprintf(digest, "blob %d\x00", len(content))
	if _, err := io.Copy(digest, contextReader{ctx, bytes.NewReader(content)}); err != nil {
		return false, err
	}
	return hex.EncodeToString(digest.Sum(nil)) == blob, ctx.Err()
}
