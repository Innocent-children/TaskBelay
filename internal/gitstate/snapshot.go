package gitstate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Boundary struct {
	Path        string
	Version     string
	WorkspaceID string
}

type Snapshot struct {
	Tree       string
	Context    Context
	Files      map[string]File
	Boundaries []Boundary
	Incomplete []Incomplete
}

func (s Snapshot) Complete() bool { return s.Tree != "" && len(s.Incomplete) == 0 }

func (w Workspace) Capture(ctx context.Context) (snapshot Snapshot, err error) {
	snapshot.Files = make(map[string]File)
	unfinished := "."
	defer func() {
		if err != nil {
			snapshot.Incomplete = append(snapshot.Incomplete, Incomplete{unfinished, err.Error()})
		}
	}()
	snapshot.Context, err = w.Context(ctx)
	if err != nil {
		return
	}
	out, err := w.command(ctx, nil, nil, "rev-parse", "--git-path", "index")
	if err != nil {
		return snapshot, err
	}
	index := strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "\r")
	if !filepath.IsAbs(index) {
		index = filepath.Join(w.Root, index)
	}
	temp, err := os.MkdirTemp("", "taskbelay-index-")
	if err != nil {
		return snapshot, err
	}
	defer os.RemoveAll(temp)
	copyPath := filepath.Join(temp, "index")
	env := []string{"GIT_INDEX_FILE=" + copyPath}
	source, err := os.Open(index)
	if errors.Is(err, os.ErrNotExist) {
		_, err = w.command(ctx, nil, env, "read-tree", "--empty")
	} else if err == nil {
		err = copyIndex(ctx, source, copyPath)
		source.Close()
	}
	if err != nil {
		return snapshot, err
	}
	// 仅在副本关闭分离索引，后续暂存不会写入真实索引。
	if _, err = w.command(ctx, nil, env, "update-index", "--no-split-index"); err != nil {
		return snapshot, err
	}
	out, err = w.command(ctx, nil, env, "ls-files", "-z")
	if err != nil {
		return snapshot, err
	}
	var present strings.Builder
	for _, path := range strings.Split(string(out), "\x00") {
		if path == "" {
			continue
		}
		if _, statErr := os.Lstat(filepath.Join(w.Root, filepath.FromSlash(path))); statErr == nil {
			present.WriteString(path)
			present.WriteByte(0)
		}
	}
	if present.Len() > 0 {
		// 两种标志须分开清除；合在一次 update-index 中会留下其中一种标志。
		for _, flag := range []string{"--no-assume-unchanged", "--no-skip-worktree"} {
			_, err = w.command(ctx, strings.NewReader(present.String()), env, "update-index", flag, "-z", "--stdin")
			if err != nil {
				return snapshot, err
			}
		}
	}
	if _, err = w.command(ctx, nil, env, "add", "-A", "--", "."); err != nil {
		return snapshot, err
	}
	out, err = w.command(ctx, nil, env, "write-tree")
	if err != nil {
		return snapshot, err
	}
	snapshot.Tree = strings.TrimSpace(string(out))
	out, err = w.command(ctx, nil, nil, "ls-tree", "-r", "-z", snapshot.Tree)
	if err != nil {
		return snapshot, err
	}
	entries := strings.Split(string(out), "\x00")
	for i, entry := range entries {
		if entry == "" {
			continue
		}
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return snapshot, fmt.Errorf("无法解析 tree 条目")
		}
		unfinished = path
		if fields[0] == "160000" {
			boundary := Boundary{Path: path, Version: fields[2]}
			child, childErr := Discover(ctx, filepath.Join(w.Root, filepath.FromSlash(path)))
			if childErr == nil && child.ID != w.ID {
				boundary.WorkspaceID = child.ID
			}
			snapshot.Boundaries = append(snapshot.Boundaries, boundary)
			snapshot.Files[path] = File{Blob: fields[2], Mode: fields[0]}
			if ctx.Err() != nil {
				return snapshot, ctx.Err()
			}
			continue
		}
		// 复用 tree 已保存的对象；过滤器改变内容时仍保存当前原始字节。
		file, content, readErr := w.ReadObservedFile(ctx, path, File{Blob: fields[2], Mode: fields[0]})
		if readErr == nil && file.Blob == "" {
			readErr = fmt.Errorf("tree 中的文件当前不可读，可能已移动或属于稀疏检出")
		}
		if readErr == nil && file.Blob != fields[2] {
			normalized, normalizeErr := w.command(ctx, bytes.NewReader(content), nil, "hash-object", "--path="+path, "--stdin")
			if normalizeErr != nil {
				readErr = normalizeErr
			} else if strings.TrimSpace(string(normalized)) != fields[2] {
				readErr = fmt.Errorf("tree 与当前文件不一致，采集期间发生了变化")
			}
		}
		if readErr != nil {
			snapshot.Incomplete = append(snapshot.Incomplete, Incomplete{path, readErr.Error()})
			if ctx.Err() != nil {
				for _, remaining := range entries[i+1:] {
					if _, pending, ok := strings.Cut(remaining, "\t"); ok {
						snapshot.Incomplete = append(snapshot.Incomplete, Incomplete{pending, ctx.Err().Error()})
					}
				}
				return snapshot, nil
			}
			continue
		}
		snapshot.Files[path] = file
	}
	return snapshot, nil
}

/**
 * Git 用索引 mtime 识别 stat 信息可能相同的快速编辑。
 * 副本若改用复制时间，会把这些条目误当成无需重新读取的旧文件。
 */
func copyIndex(ctx context.Context, source *os.File, path string) error {
	info, err := source.Stat()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, contextReader{ctx, source})
	closeErr := file.Close()
	if err := errors.Join(err, closeErr); err != nil {
		return err
	}
	return os.Chtimes(path, info.ModTime(), info.ModTime())
}

type Retained struct {
	Tree string
	SnapshotRef
}

/**
 * 只创建预登记的直接 tree 引用。重试只能复用相同对象，不能移动已有引用。
 */
func (w Workspace) Retain(ctx context.Context, owner, id, tree string) (Retained, error) {
	result := Retained{Tree: tree, SnapshotRef: SnapshotRef{Ref: "refs/taskbelay/" + owner + "/" + id, Object: tree, Type: "tree", WorkspaceID: w.ID}}
	if err := result.Validate(); err != nil {
		return result, err
	}
	kind, err := w.command(ctx, nil, nil, "cat-file", "-t", tree)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(string(kind)) != "tree" {
		return result, fmt.Errorf("快照对象不是 tree")
	}
	_, err = w.command(ctx, strings.NewReader("create "+result.Ref+" "+tree+"\n"), nil, "update-ref", "--no-deref", "--stdin")
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	fields, readErr := w.snapshotFields(ctx, result.Ref)
	if readErr == nil && len(fields) == 8 && fields[0] == result.Ref && fields[1] == tree && fields[2] == "tree" && fields[7] == "" {
		return result, nil
	}
	return result, fmt.Errorf("创建快照引用失败或已有其他对象: %s: %w", result.Ref, err)
}

func safeComponent(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

/**
 * 原始前后内容和请求内容单独保留，避免 clean filter 改写快照后丢失核对证据。
 * 条目名只是证据槽位，原始文件路径与片段范围由观察记录保存。
 */
func (w Workspace) EvidenceTree(ctx context.Context, files []File, tree string) (string, error) {
	input, err := evidenceInput(files, tree)
	if err != nil {
		return "", err
	}
	out, err := w.command(ctx, strings.NewReader(input), nil, "mktree", "-z")
	if err != nil {
		err = fmt.Errorf("构造证据 tree: %w", err)
	}
	return strings.TrimSpace(string(out)), err
}

/**
 * 证据 tree 进程先等待输入，与新 blob 的写入重叠启动。
 * 只有内容全部保存后才发送条目；失败和取消同样关闭管道并等待子进程退出。
 */
func (w Workspace) WriteEvidenceTree(ctx context.Context, files []File, tree string, contents [][]byte) (string, []string, error) {
	if len(contents) == 0 {
		id, err := w.EvidenceTree(ctx, files, tree)
		return id, nil, err
	}
	input, writer, err := os.Pipe()
	if err != nil {
		return "", nil, fmt.Errorf("创建证据 tree 管道: %w", err)
	}
	type outcome struct {
		output []byte
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := w.command(ctx, input, nil, "mktree", "-z")
		if err != nil {
			err = fmt.Errorf("构造证据 tree: %w", err)
		}
		done <- outcome{out, errors.Join(err, input.Close())}
	}()
	all := append([]File(nil), files...)
	var blobs []string
	for _, content := range contents {
		var blob string
		blob, err = w.WriteBlob(ctx, content)
		if err != nil {
			err = fmt.Errorf("写入证据 blob: %w", err)
			break
		}
		blobs = append(blobs, blob)
		all = append(all, File{Blob: blob, Mode: "100644"})
	}
	if err == nil {
		var entries string
		entries, err = evidenceInput(all, tree)
		if err == nil {
			_, err = io.WriteString(writer, entries)
			if err != nil {
				err = fmt.Errorf("发送证据 tree 条目: %w", err)
			}
		}
	}
	closeErr := writer.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("关闭证据 tree 管道: %w", closeErr)
	}
	result := <-done
	if err = errors.Join(err, closeErr, result.err); err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(string(result.output)), blobs, nil
}

func evidenceInput(files []File, tree string) (string, error) {
	var entries []string
	seen := make(map[string]bool)
	for _, file := range files {
		if file.Blob == "" || file.Mode == "160000" || seen[file.Blob] {
			continue
		}
		if !validObjectID(file.Blob) {
			return "", fmt.Errorf("无效的证据对象")
		}
		seen[file.Blob] = true
		entries = append(entries, "100644 blob "+file.Blob+"\t"+strconv.Itoa(len(entries)))
	}
	if tree != "" {
		if !validObjectID(tree) {
			return "", fmt.Errorf("无效的 tree 标识")
		}
		entries = append(entries, "040000 tree "+tree+"\tworkspace")
	}
	sort.Strings(entries)
	input := strings.Join(entries, "\x00")
	if len(entries) > 0 {
		input += "\x00"
	}
	return input, nil
}
