package gitstate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/**
 * 只向新建的副本目录导出固定 Git 对象；不读取当前源码、不运行 checkout
 * 过滤器，也不沿符号链接写入。子模块及副本外链接无法提供固定源码证明。
 */
func (w Workspace) ExportTree(ctx context.Context, tree, destination string) error {
	files, err := w.TreeFiles(ctx, tree)
	if err != nil {
		return err
	}
	var paths, blobs []string
	for path, file := range files {
		if !filepath.IsLocal(filepath.FromSlash(path)) || strings.EqualFold(strings.Split(path, "/")[0], ".git") {
			return fmt.Errorf("快照路径不能导出到副本: %q", path)
		}
		if file.Mode != "100644" && file.Mode != "100755" && file.Mode != "120000" {
			return fmt.Errorf("快照 %s 的模式 %s 无法导出固定源码", path, file.Mode)
		}
		paths, blobs = append(paths, path), append(blobs, file.Blob)
	}
	contents, err := w.ReadBlobs(ctx, blobs)
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	sort.Strings(paths)
	var links []string
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		file := files[path]
		target := filepath.Join(destination, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if file.Mode == "120000" {
			link := string(contents[file.Blob])
			resolved := filepath.Join(filepath.Dir(filepath.FromSlash(path)), link)
			if filepath.IsAbs(link) || !filepath.IsLocal(resolved) {
				return fmt.Errorf("快照链接指向副本之外: %s", path)
			}
			links = append(links, path)
			continue
		}
		mode := os.FileMode(0600)
		if file.Mode == "100755" {
			mode = 0700
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			return err
		}
		_, writeErr := out.Write(contents[file.Blob])
		closeErr := out.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	for _, path := range links {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Symlink(string(contents[files[path].Blob]), filepath.Join(destination, filepath.FromSlash(path))); err != nil {
			return err
		}
	}
	return nil
}
