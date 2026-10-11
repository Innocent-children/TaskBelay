// Package execution owns process execution and observed applicability conditions.
package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
)

type Fingerprint struct {
	Inputs      domain.Digest `json:"inputs"`
	Environment domain.Digest `json:"environment"`
	Executable  string        `json:"executable"`
	Files       int           `json:"files"`
}

func Observe(ctx context.Context, t domain.Task, c domain.Check) (Fingerprint, error) {
	r, ok := t.Repository(c.RepositoryKey)
	if !ok {
		return Fingerprint{}, domain.Invalid("check.repository_key", "repository is absent")
	}
	root := r.Origin.CanonicalWorktreeRoot
	raw, e := repository.ReadGit(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e != nil {
		return Fingerprint{}, e
	}
	paths := map[string]bool{}
	for _, p := range strings.Split(string(raw), "\x00") {
		if p == "" {
			continue
		}
		for _, scope := range c.Inputs {
			if domain.MatchPath(scope, p) {
				paths[p] = true
			}
		}
	}
	for _, scope := range c.Inputs {
		if scope == "." {
			continue
		}
		p, e := repository.SafePath(root, scope)
		if e != nil {
			return Fingerprint{}, e
		}
		info, e := os.Lstat(p)
		if os.IsNotExist(e) {
			paths[scope] = true
			continue
		}
		if e != nil {
			return Fingerprint{}, e
		}
		if info.IsDir() {
			e = filepath.WalkDir(p, func(local string, d os.DirEntry, er error) error {
				if er != nil {
					return er
				}
				if d.Name() == ".git" && d.IsDir() {
					return filepath.SkipDir
				}
				if d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(root, local)
				paths[filepath.ToSlash(rel)] = true
				if len(paths) > domain.MaxInputFiles {
					return domain.Fail("RESOURCE_LIMIT", "check.inputs", "input file count exceeds 16384")
				}
				return nil
			})
			if e != nil {
				return Fingerprint{}, e
			}
		} else {
			paths[scope] = true
		}
	}
	if len(paths) > domain.MaxInputFiles {
		return Fingerprint{}, domain.Fail("RESOURCE_LIMIT", "check.inputs", "input file count exceeds 16384")
	}
	ordered := []string{}
	for p := range paths {
		ordered = append(ordered, p)
	}
	sort.Strings(ordered)
	entries := []string{}
	for _, p := range ordered {
		if e = ctx.Err(); e != nil {
			return Fingerprint{}, e
		}
		local, e := repository.SafePath(root, p)
		if e != nil {
			return Fingerprint{}, e
		}
		info, e := os.Lstat(local)
		if os.IsNotExist(e) {
			entries = append(entries, p+":missing")
			continue
		}
		if e != nil {
			return Fingerprint{}, e
		}
		if !info.Mode().IsRegular() {
			return Fingerprint{}, domain.Fail("UNSUPPORTED_INPUT", "check.inputs", "non-regular inputs need an explicit external verification boundary")
		}
		d, e := hashFile(local)
		if e != nil {
			return Fingerprint{}, e
		}
		entries = append(entries, fmt.Sprintf("%s:%s:%d", p, d, info.Mode()))
	}
	cwd, e := repository.SafePath(root, c.Cwd)
	if e != nil {
		return Fingerprint{}, e
	}
	env := []string{runtime.GOOS, runtime.GOARCH, cwd, c.Conditions}
	names := append([]string{"PATH"}, c.Environment...)
	sort.Strings(names)
	for _, name := range names {
		v, exists := os.LookupEnv(name)
		env = append(env, name+":"+string(domain.Hash([]any{exists, v})))
	}
	executable := ""
	if c.Kind == "command" {
		executable = c.Argv[0]
		if strings.ContainsAny(executable, "/\\") {
			if !filepath.IsAbs(executable) {
				executable = filepath.Join(cwd, executable)
			}
		} else {
			executable, e = exec.LookPath(executable)
			if e != nil {
				return Fingerprint{}, domain.Fail("EXECUTABLE_UNAVAILABLE", "check.argv[0]", "approved executable was not found")
			}
		}
		executable, e = filepath.Abs(executable)
		if e != nil {
			return Fingerprint{}, e
		}
		d, e := hashFile(executable)
		if e != nil {
			return Fingerprint{}, e
		}
		env = append(env, executable+":"+string(d))
	}
	return Fingerprint{Inputs: domain.Hash(entries), Environment: domain.Hash(env), Executable: executable, Files: len(ordered)}, nil
}
func hashFile(p string) (domain.Digest, error) {
	f, e := os.Open(p)
	if e != nil {
		return "", e
	}
	defer f.Close()
	a, e := f.Stat()
	if e != nil {
		return "", e
	}
	if a.Size() > 512*1024*1024 {
		return "", domain.Fail("RESOURCE_LIMIT", "check.inputs", "a fingerprint input exceeds 512 MiB")
	}
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	b, e := f.Stat()
	if e != nil || a.Size() != b.Size() || a.ModTime() != b.ModTime() {
		return "", repository.ErrInconsistentWorktree
	}
	return domain.Digest(hex.EncodeToString(h.Sum(nil))), nil
}
