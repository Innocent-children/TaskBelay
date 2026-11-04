package gitstate

import (
	"context"
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
	empty, err := w.command(ctx, nil, nil, "hash-object", "-t", "tree", "-w", "--stdin")
	if err != nil {
		return snapshot, err
	}
	copyPath := filepath.Join(temp, "index")
	if _, err = w.command(ctx, nil, nil, "read-tree", "--index-output="+copyPath, "--prefix=", strings.TrimSpace(string(empty))); err != nil {
		return snapshot, err
	}
	env := []string{"GIT_INDEX_FILE=" + copyPath}
	if _, err = w.command(ctx, nil, env, "update-index", "--no-split-index"); err != nil {
		return snapshot, err
	}
	objects, err := w.command(ctx, nil, nil, "rev-parse", "--git-path", "objects")
	if err != nil {
		return snapshot, err
	}
	objectPath := strings.TrimSpace(string(objects))
	if !filepath.IsAbs(objectPath) {
		objectPath = filepath.Join(w.Root, objectPath)
	}
	metadata := filepath.Join(temp, "repo")
	args := []string{"init", "--bare", "--template="}
	if len(strings.TrimSpace(string(empty))) == 64 {
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
	env = append(env, "GIT_DIR="+metadata, "GIT_WORK_TREE="+w.Root, "GIT_OBJECT_DIRECTORY="+objectPath)
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
	if _, err = w.commandTo(ctx, nil, paths, env, append(append([]string{}, config...), "ls-files", "-z")...); err != nil {
		return snapshot, err
	}
	if _, err = paths.Seek(0, io.SeekStart); err != nil {
		return snapshot, err
	}
	if _, err = w.command(ctx, paths, env, append(append([]string{}, config...), "update-index", "--no-assume-unchanged", "-z", "--stdin")...); err != nil {
		return snapshot, err
	}
	// 所有标记只在临时索引中清除。缺失的稀疏条目随后保留；普通删除由原索引给出。
	if _, err = paths.Seek(0, io.SeekStart); err != nil {
		return snapshot, err
	}
	if _, err = w.command(ctx, paths, env, append(append([]string{}, config...), "update-index", "--no-skip-worktree", "-z", "--stdin")...); err != nil {
		return snapshot, err
	}
	// 零时间让 Git 把每个 stat 命中视为可能的快速编辑，按原始字节重新核对。
	epoch := time.Unix(0, 0)
	if err = os.Chtimes(copyPath, epoch, epoch); err != nil {
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
	if _, err = paths.Seek(0, io.SeekStart); err != nil {
		return snapshot, err
	}
	if _, err = w.command(ctx, paths, env, "update-index", "--force-remove", "-z", "--stdin"); err != nil {
		return snapshot, err
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
