package gitstate

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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

// 普通 v2 索引可直接作为私有副本，由 Git 校验并重查工作区。特殊索引由 Git
// 导出条目重建；不解析扩展或修改二进制索引字段。
func plainIndex(data []byte, hashBytes int) bool {
	if len(data) < 12+hashBytes || string(data[:4]) != "DIRC" || binary.BigEndian.Uint32(data[4:8]) != 2 {
		return false
	}
	entries := binary.BigEndian.Uint32(data[8:12])
	data = data[12 : len(data)-hashBytes]
	const statBytes = 40
	flagsAt := statBytes + hashBytes
	for range entries {
		if len(data) < flagsAt+2 {
			return false
		}
		flags := int(binary.BigEndian.Uint16(data[flagsAt:]))
		// 长名称、特殊标记或早于 racy 检查起点的 mtime 交给 Git 处理。
		if flags >= 0xfff || binary.BigEndian.Uint32(data[8:12]) == 0 {
			return false
		}
		pathEnd := flagsAt + 2 + flags
		size := (pathEnd + 1 + 7) &^ 7
		if size > len(data) || data[pathEnd] != 0 {
			return false
		}
		data = data[size:]
	}
	return len(data) == 0 || len(data) >= 8 && string(data[:4]) == "TREE" && uint64(binary.BigEndian.Uint32(data[4:8])) == uint64(len(data)-8)
}

/**
 * CaptureTree 让 Git 在独立目录中读取原始字节；原索引只读，所有暂存都在临时索引中。
 * 目录清单和内容检查由 Git 完成，调用方随后只取两个 tree 的差异。
 */
func (w Workspace) CaptureTree(ctx context.Context) (snapshot Snapshot, err error) {
	snapshot.Context, err = w.Context(ctx)
	if err != nil {
		return
	}
	temp, err := os.MkdirTemp(w.GitDir, "taskbelay-capture-")
	if err != nil {
		return snapshot, err
	}
	defer os.RemoveAll(temp)
	objectID := snapshot.Context.HEAD
	if objectID == "" {
		empty, err := w.command(ctx, nil, nil, "hash-object", "-t", "tree", "-w", "--stdin")
		if err != nil {
			return snapshot, err
		}
		objectID = strings.TrimSpace(string(empty))
	}
	copyPath := filepath.Join(temp, "index")
	sourcePath := filepath.Join(temp, "source-index")
	env := []string{"GIT_INDEX_FILE=" + copyPath}
	data, err := os.ReadFile(filepath.Join(w.GitDir, "index"))
	plain := false
	if errors.Is(err, os.ErrNotExist) {
		_, err = w.command(ctx, nil, env, "read-tree", "--empty")
		plain = true
	} else if err == nil {
		plain = plainIndex(data, len(objectID)/2)
		destination := copyPath
		if !plain {
			destination = sourcePath
		}
		err = os.WriteFile(destination, data, 0600)
	}
	if err != nil {
		return snapshot, err
	}
	metadata := filepath.Join(temp, "repo")
	args := []string{"init", "--bare", "--template="}
	if len(objectID) == 64 {
		args = append(args, "--object-format=sha256")
	}
	args = append(args, metadata)
	if _, err = w.command(ctx, nil, nil, args...); err != nil {
		return snapshot, err
	}
	if err = os.MkdirAll(filepath.Join(metadata, "info"), 0700); err != nil {
		return snapshot, err
	}
	if err = os.WriteFile(filepath.Join(metadata, "info", "attributes"), []byte("* -text -eol -filter -ident -working-tree-encoding\n"), 0600); err != nil {
		return snapshot, err
	}
	env = append(env, "GIT_DIR="+metadata, "GIT_WORK_TREE="+w.Root, "GIT_OBJECT_DIRECTORY="+filepath.Join(w.CommonDir, "objects"))
	config := []string{"-c", "include.path=" + filepath.Join(w.CommonDir, "config")}
	if _, statErr := os.Stat(filepath.Join(w.GitDir, "config.worktree")); statErr == nil {
		config = append(config, "-c", "include.path="+filepath.Join(w.GitDir, "config.worktree"))
	}
	// 原始模式与目标文件核对一致，不沿用仓库对执行位或符号链接变化的忽略设置。
	config = append(config, "-c", "core.bare=false", "-c", "core.worktree="+w.Root, "-c", "core.sparseCheckout=false", "-c", "core.filemode=true", "-c", "core.symlinks=true")
	paths, err := os.Create(filepath.Join(temp, "paths"))
	if err != nil {
		return snapshot, err
	}
	defer paths.Close()
	if !plain {
		sourceEnv := []string{"GIT_INDEX_FILE=" + sourcePath}
		conflicts, err := w.command(ctx, nil, sourceEnv, "ls-files", "--unmerged", "-z")
		if err != nil {
			return snapshot, err
		}
		if len(conflicts) != 0 {
			return snapshot, fmt.Errorf("索引包含尚未解决的合并冲突")
		}
		if _, err = w.commandTo(ctx, nil, paths, sourceEnv, "ls-files", "--stage", "-z"); err != nil {
			return snapshot, err
		}
		if _, err = paths.Seek(0, io.SeekStart); err != nil {
			return snapshot, err
		}
		if _, err = w.command(ctx, paths, env, append(append([]string{}, config...), "update-index", "-z", "--index-info")...); err != nil {
			return snapshot, err
		}
	}
	// Git 以非零索引时间启用 racy-stat 核对。即使真实索引的 stat 与文件一致，
	// 也要区分原始字节与 clean filter 或行尾转换后的 blob。
	early := time.Unix(1, 0)
	if err = os.Chtimes(copyPath, early, early); err != nil {
		return snapshot, err
	}
	if _, err = w.command(ctx, nil, env, append(append([]string{}, config...), "add", "-A", "--ignore-removal", "--", ".")...); err != nil {
		return snapshot, err
	}
	if err = paths.Truncate(0); err != nil {
		return snapshot, err
	}
	if _, err = paths.Seek(0, io.SeekStart); err != nil {
		return snapshot, err
	}
	if _, err = w.commandTo(ctx, nil, paths, nil, "ls-files", "--deleted", "-z"); err != nil {
		return snapshot, err
	}
	deleted, err := paths.Stat()
	if err != nil {
		return snapshot, err
	}
	if deleted.Size() > 0 {
		if _, err = paths.Seek(0, io.SeekStart); err != nil {
			return snapshot, err
		}
		if _, err = w.command(ctx, paths, env, "update-index", "--force-remove", "-z", "--stdin"); err != nil {
			return snapshot, err
		}
	}
	out, err := w.command(ctx, nil, env, "write-tree")
	if err != nil {
		return snapshot, err
	}
	snapshot.Tree = strings.TrimSpace(string(out))
	return snapshot, nil
}

func (w Workspace) Capture(ctx context.Context, previous ...string) (snapshot Snapshot, err error) {
	snapshot, err = w.CaptureTree(ctx)
	if err != nil {
		return snapshot, err
	}
	snapshot.Files = map[string]File{}
	if len(previous) > 0 && previous[0] != "" {
		if previous[0] == snapshot.Tree {
			return snapshot, ctx.Err()
		}
		diff, err := w.Diff(ctx, previous[0], snapshot.Tree)
		if err != nil {
			return snapshot, err
		}
		for _, d := range diff {
			snapshot.Files[d.BeforePath] = File{}
			snapshot.Files[d.Path] = d.After
		}
	} else {
		snapshot.Files, err = w.TreeFiles(ctx, snapshot.Tree)
		if err != nil {
			return snapshot, err
		}
	}
	for path, file := range snapshot.Files {
		if file.Mode == "160000" {
			boundary := Boundary{Path: path, Version: file.Blob}
			child, childErr := Discover(ctx, filepath.Join(w.Root, filepath.FromSlash(path)))
			if childErr == nil && child.ID != w.ID {
				boundary.WorkspaceID = child.ID
			}
			snapshot.Boundaries = append(snapshot.Boundaries, boundary)
		}
	}
	return snapshot, nil
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
 * 单独保存编辑前后的原始内容和请求内容，避免 clean filter 改写快照后无法核对编辑。
 * tree 条目名只给这些内容编号；原文件路径和片段范围另存于观察记录。
 */
func (w Workspace) EvidenceTree(ctx context.Context, files []File, tree string) (string, error) {
	input, err := evidenceInput(files, tree)
	if err != nil {
		return "", err
	}
	out, err := w.command(ctx, strings.NewReader(input), nil, "mktree", "-z")
	if err != nil {
		err = fmt.Errorf("创建保存原始内容的 Git tree: %w", err)
	}
	return strings.TrimSpace(string(out)), err
}

/**
 * 先启动 git mktree 等待输入，同时写入新的 blob。
 * 只有内容全部保存后才发送条目；失败和取消同样关闭管道并等待子进程退出。
 */
func (w Workspace) WriteEvidenceTree(ctx context.Context, files []File, tree string, contents [][]byte) (string, []string, error) {
	if len(contents) == 0 {
		id, err := w.EvidenceTree(ctx, files, tree)
		return id, nil, err
	}
	input, writer, err := os.Pipe()
	if err != nil {
		return "", nil, fmt.Errorf("创建 git mktree 输入管道: %w", err)
	}
	type outcome struct {
		output []byte
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := w.command(ctx, input, nil, "mktree", "-z")
		if err != nil {
			err = fmt.Errorf("创建保存原始内容的 Git tree: %w", err)
		}
		done <- outcome{out, errors.Join(err, input.Close())}
	}()
	all := append([]File(nil), files...)
	var blobs []string
	for _, content := range contents {
		var blob string
		blob, err = w.WriteBlob(ctx, content)
		if err != nil {
			err = fmt.Errorf("将原始内容写入 Git blob: %w", err)
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
				err = fmt.Errorf("向 git mktree 发送条目: %w", err)
			}
		}
	}
	closeErr := writer.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("关闭 git mktree 输入管道: %w", closeErr)
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
			return "", fmt.Errorf("原始内容的 Git blob 标识无效")
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
