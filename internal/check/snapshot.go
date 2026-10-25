package check

import (
	"context"
	"sort"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/lang"
)

func AnalyzeTree(ctx context.Context, workspace gitstate.Workspace, tree string) (codescan.Snapshot, error) {
	files, err := workspace.TreeFiles(ctx, tree)
	if err != nil {
		return codescan.Snapshot{}, err
	}
	var paths, blobs []string
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file := files[path]
		kind := lang.Classify(path, nil)
		if file.Mode != "100644" && file.Mode != "100755" || kind.Dependency || kind.Lock || kind.TestData {
			continue
		}
		// 字符串和配置中的按名字调用也是 D1 的核对输入。
		blobs = append(blobs, file.Blob)
	}
	contents, err := workspace.ReadBlobs(ctx, blobs)
	if err != nil {
		return codescan.Snapshot{}, err
	}
	var sources []codescan.Source
	for _, path := range paths {
		file := files[path]
		sources = append(sources, codescan.Source{Path: path, Blob: file.Blob, Mode: file.Mode, Data: contents[file.Blob]})
	}
	return codescan.Analyze(ctx, tree, sources), nil
}
