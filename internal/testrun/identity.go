package testrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const DynamicVersion = "T1-T2-runners-v2"

/**
 * 标识只保存摘要，不保存环境变量原文。工具链源码和编译程序一起核对，
 * 避免工具版本字符串相同、实际文件已变化时复用旧动态结果。
 */
func ExecutionIdentity(ctx context.Context, dir string) (string, error) {
	base, err := os.MkdirTemp("", "taskbelay-tools-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(base)
	executor := &Executor{Root: dir, Output: base, Env: os.Environ()}
	result := executor.Execute(ctx, "identity", dir, []string{"go", "env", "-json", "GOROOT", "GOVERSION", "GOFLAGS", "GOTOOLCHAIN"}, nil)
	if !result.Passed() || result.Truncated {
		return "", fmt.Errorf("Go 工具身份无法核对: %s %s", result.Reason, result.ErrorOutput)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(result.Output), &values); err != nil {
		return "", err
	}
	root := values["GOROOT"]
	if root == "" {
		return "", fmt.Errorf("Go 未返回 GOROOT")
	}
	digest := sha256.New()
	digest.Write([]byte(DynamicVersion))
	environment, _ := json.Marshal(mergeEnvironment(os.Environ(), nil))
	digest.Write(environment)
	digest.Write([]byte(result.Output))
	for _, name := range []string{"src", "pkg/tool", "pkg/include", "bin"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("工具链包含不能核对的链接: %s", path)
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			fmt.Fprintln(digest, path)
			_, readErr := io.Copy(digest, cancelReader{ctx, file})
			closeErr := file.Close()
			if readErr != nil {
				return readErr
			}
			return closeErr
		}); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func (g *GoRunner) Reusable() bool {
	if g.config.Command != "" || g.config.Prepare != "" || os.Getenv("GOFLAGS") != "" || os.Getenv("GOWORK") != "" && os.Getenv("GOWORK") != "off" {
		return false
	}
	allowed := map[string]bool{"testing": true, "fmt": true, "strings": true, "strconv": true, "bytes": true, "errors": true, "math": true, "math/bits": true, "sort": true, "slices": true, "cmp": true, "unicode": true, "unicode/utf8": true, "encoding/json": true, "encoding/hex": true, "encoding/base64": true}
	for _, pkg := range g.packages {
		if len(pkg.CgoFiles)+len(pkg.SFiles)+len(pkg.CFiles)+len(pkg.CXXFiles)+len(pkg.SwigFiles) > 0 {
			return false
		}
		imports := append(append(append([]string(nil), pkg.Imports...), pkg.TestImports...), pkg.XTestImports...)
		for _, imported := range imports {
			if _, local := g.packages[imported]; !local && !allowed[imported] {
				return false
			}
		}
	}
	for _, file := range g.request.Snapshot.Files {
		if file.Kind.Language != "go" {
			continue
		}
		if !file.CodeComplete || file.Kind.Generated {
			return false
		}
		if strings.Contains(string(file.Data), "//go:linkname") {
			return false
		}
		for _, risk := range file.Risks {
			if risk.Kind == "concurrency" || risk.Kind == "write" || risk.Kind == "command" {
				return false
			}
		}
		for _, declaration := range file.Declarations {
			for _, reference := range declaration.References {
				switch reference {
				case "Scan", "Scanf", "Scanln":
					return false
				}
			}
		}
		for _, reference := range file.References {
			if strings.Contains(reference, "linkname") {
				return false
			}
		}
		for _, test := range file.Tests {
			for _, reference := range test.References {
				switch reference {
				case "Setenv", "Chdir", "TempDir", "Deadline", "Parallel", "Benchmark", "AllocsPerRun":
					return false
				}
			}
		}
	}
	return true
}
