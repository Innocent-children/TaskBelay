package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

// ReadGit is restricted to the engineering observer's two read-only operations.
func ReadGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	if len(args) == 0 || (args[0] != "show" && args[0] != "ls-files" && args[0] != "diff") {
		return nil, ErrGitObservation
	}
	c := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "-C", root}, args...)...)
	configureGitCommand(c)
	c.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	out := &boundedRead{limit: domain.MaxGitCommandOutputBytes}
	c.Stdout = out
	c.Stderr = io.Discard
	if e := c.Run(); e != nil {
		return nil, ErrGitObservation
	}
	return out.data, nil
}

type boundedRead struct {
	data  []byte
	limit int
}

func (w *boundedRead) Write(b []byte) (int, error) {
	if len(w.data)+len(b) > w.limit {
		return 0, ErrGitOutputLimit
	}
	w.data = append(w.data, b...)
	return len(b), nil
}
func SafePath(root, relative string) (string, error) {
	if domain.ValidatePath(relative) != nil {
		return "", domain.Invalid("path", "requires a safe repository-relative path")
	}
	p := filepath.Join(root, filepath.FromSlash(relative))
	current := p
	for {
		info, e := os.Lstat(current)
		if e == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", domain.Fail("UNSUPPORTED_INPUT", "path", "symlink inputs or working directories require an explicit external check")
		}
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
		if current == root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", ErrInvalidRepositoryPath
		}
		current = parent
	}
	return p, nil
}
func FileDigest(root, relative string) (domain.Digest, error) {
	p, e := SafePath(root, relative)
	if e != nil {
		return "", e
	}
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return domain.HashBytes([]byte("taskbelay:missing")), nil
	}
	if e != nil {
		return "", e
	}
	if len(b) > 16*1024*1024 {
		return "", domain.Fail("RESOURCE_LIMIT", "path", "scope analysis file exceeds 16 MiB")
	}
	return domain.HashBytes(b), nil
}

// ScopeFacts observes surfaces within files too. Semantic meaning remains a Host judgment.
func ScopeFacts(ctx context.Context, t domain.Task) ([]domain.Fact, error) {
	facts := []domain.Fact{}
	add := func(kind, p, detail string, blocking bool) {
		facts = append(facts, domain.Fact{ID: string(domain.Hash([]string{kind, p, detail}))[:20], Kind: kind, Path: p, Detail: detail, Blocking: blocking})
	}
	for _, r := range t.Repositories {
		root := r.Origin.CanonicalWorktreeRoot
		for _, e := range r.Binding.TaskSurface {
			p := t.QualifiedPath(r.Key, e.Path)
			if !t.Authorization.AllowsPath(p) {
				add("unauthorized_path", p, "Change is outside the approved allowed paths or touches a forbidden path.", true)
			}
			expected := false
			for _, v := range t.Plan.ExpectedPaths {
				expected = expected || domain.MatchPath(v, p)
			}
			if len(t.Plan.ExpectedPaths) > 0 && !expected {
				add("unexpected_path", p, "Path was not predicted; explain its necessity for an authorized goal.", false)
			}
			if e.Gitlink {
				add("gitlink", p, "Submodule object changed; ordinary-file checks do not establish its contents.", false)
			}
			if strings.HasSuffix(e.Path, ".go") || strings.HasSuffix(e.Path, "package.json") || filepath.Base(e.Path) == "go.mod" || strings.HasSuffix(e.Path, ".sql") || strings.HasSuffix(e.Path, ".proto") || strings.HasSuffix(e.Path, ".schema.json") {
				var before []byte
				if e.BaseMode != "000000" {
					var err error
					before, err = ReadGit(ctx, root, "show", r.Origin.BaseCommit+":"+e.Path)
					if err != nil {
						return nil, err
					}
				}
				local, er := SafePath(root, e.Path)
				if er != nil {
					return nil, er
				}
				after, er := os.ReadFile(local)
				if er != nil && !os.IsNotExist(er) {
					return nil, er
				}
				if len(after) > 16*1024*1024 {
					return nil, domain.Fail("RESOURCE_LIMIT", "scope", "scope analysis file exceeds 16 MiB")
				}
				if strings.HasSuffix(e.Path, ".go") {
					a, ae := exported(before)
					b, be := exported(after)
					if ae != nil || be != nil {
						add("api_unknown", p, "Go source could not be parsed for public-interface comparison.", false)
					} else if a != b {
						add("interface_change", p, "Exported Go declarations changed (signature/type/value comparison).", false)
					}
				} else if strings.HasSuffix(e.Path, "package.json") || filepath.Base(e.Path) == "go.mod" {
					a, ae := dependencies(e.Path, before)
					b, be := dependencies(e.Path, after)
					if ae != nil || be != nil {
						add("dependency_unknown", p, "Dependency manifest could not be parsed; correct it or explicitly revise the contract before proceeding.", true)
					} else {
						names := []string{}
						for name, v := range a {
							if b[name] != v {
								names = append(names, name)
							}
						}
						for name, v := range b {
							if a[name] != v && a[name] == "" {
								names = append(names, name)
							}
						}
						sort.Strings(names)
						for _, name := range names {
							// npm facts retain the declaration section; manifest policy grants name the package.
							policyName := name
							if strings.HasSuffix(e.Path, "package.json") {
								_, policyName, _ = strings.Cut(name, ":")
							}
							allowed, restricted := false, false
							for _, policy := range t.Authorization.Dependencies {
								if policy.Path == p {
									restricted = true
									for _, v := range policy.Allowed {
										allowed = allowed || policyName == v
									}
								}
							}
							reason := "review necessity and compatibility against the requested outcome"
							if restricted && !allowed {
								reason = "the explicit manifest restriction does not allow this package"
							} else if restricted {
								reason = "the explicit manifest restriction allows this package"
							}
							add("dependency_change", p, fmt.Sprintf("Dependency %s changed from %q to %q at %s: %s. File write scope still applies.", name, a[name], b[name], p, reason), restricted && !allowed)
						}
					}
				} else {
					add("data_contract_change", p, "Persisted data or wire-schema file changed; assess its contract implications.", false)
				}
			}
		}
		for _, pin := range t.Authorization.Pins {
			relative := pin.Path
			if len(t.Repositories) > 1 {
				prefix := string(r.Key) + "::"
				if !strings.HasPrefix(relative, prefix) {
					continue
				}
				relative = strings.TrimPrefix(relative, prefix)
			}
			digest, e := FileDigest(root, relative)
			if e != nil {
				return nil, e
			}
			if digest != pin.Digest {
				add("pinned_content", pin.Path, "Content no longer matches the authorized SHA-256 pin.", true)
			}
		}
		num, e := ReadGit(ctx, root, "diff", "--no-ext-diff", "--numstat", r.Origin.BaseCommit, "--")
		if e != nil {
			return nil, e
		}
		lines := 0
		for _, line := range strings.Split(string(num), "\n") {
			var a, b int
			if _, e := fmt.Sscanf(line, "%d\t%d\t", &a, &b); e == nil {
				lines += a + b
			}
		}
		for _, e := range r.Binding.TaskSurface {
			if e.ChangeType == domain.RepositoryChangeAdded && e.BaseMode == "000000" {
				p, er := SafePath(root, e.Path)
				if er != nil {
					return nil, er
				}
				b, _ := os.ReadFile(p)
				lines += strings.Count(string(b), "\n")
			}
		}
		threshold := t.Plan.ReviewAfterLines
		if threshold == 0 {
			threshold = 500
		}
		if lines > threshold {
			add("change_magnitude", string(r.Key), fmt.Sprintf("Observed change is approximately %d lines, above the %d-line review threshold; explain why this amount is necessary.", lines, threshold), false)
		}
	}
	return facts, nil
}
func exported(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	fset := token.NewFileSet()
	f, e := parser.ParseFile(fset, "observed.go", b, 0)
	if e != nil {
		return "", e
	}
	out := []string{}
	for _, d := range f.Decls {
		switch v := d.(type) {
		case *ast.FuncDecl:
			if !v.Name.IsExported() {
				continue
			}
			v.Body = nil
			var s strings.Builder
			printer.Fprint(&s, fset, v)
			out = append(out, s.String())
		case *ast.GenDecl:
			for _, spec := range v.Specs {
				keep := false
				switch x := spec.(type) {
				case *ast.TypeSpec:
					keep = x.Name.IsExported()
				case *ast.ValueSpec:
					for _, n := range x.Names {
						keep = keep || n.IsExported()
					}
				}
				if keep {
					var s strings.Builder
					printer.Fprint(&s, fset, spec)
					out = append(out, v.Tok.String()+" "+s.String())
				}
			}
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n"), nil
}
func dependencies(p string, b []byte) (map[string]string, error) {
	m := map[string]string{}
	if len(b) == 0 {
		return m, nil
	}
	if strings.HasSuffix(p, "package.json") {
		var v map[string]json.RawMessage
		if e := json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		for _, kind := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
			if v[kind] == nil {
				continue
			}
			var ds map[string]string
			if e := json.Unmarshal(v[kind], &ds); e != nil {
				return nil, e
			}
			for k, x := range ds {
				m[kind+":"+k] = x
			}
		}
		return m, nil
	}
	block := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, "(") {
			if block != "" {
				return nil, fmt.Errorf("nested module directive")
			}
			block = strings.TrimSpace(strings.TrimSuffix(line, "("))
			continue
		}
		if line == ")" {
			block = ""
			continue
		}
		xs := strings.Fields(line)
		kind := block
		if kind == "" {
			kind = xs[0]
			xs = xs[1:]
		}
		switch kind {
		case "require":
			if len(xs) != 2 {
				return nil, fmt.Errorf("unrecognized require")
			}
			m[xs[0]] = xs[1]
		case "replace":
			index := -1
			for i, v := range xs {
				if v == "=>" {
					index = i
				}
			}
			if index < 1 || index == len(xs)-1 {
				return nil, fmt.Errorf("unrecognized replace")
			}
			m["replace:"+xs[0]] = strings.Join(xs[1:], " ")
		case "exclude":
			if len(xs) != 2 {
				return nil, fmt.Errorf("unrecognized exclude")
			}
			m["exclude:"+xs[0]] = xs[1]
		case "go", "toolchain", "module":
			if len(xs) != 1 {
				return nil, fmt.Errorf("unrecognized module property")
			}
			m[kind] = xs[0]
		case "retract", "godebug", "tool", "ignore":
			m[kind] = m[kind] + strings.Join(xs, " ") + "\n"
		default:
			return nil, fmt.Errorf("unsupported module directive")
		}
	}
	if block != "" {
		return nil, fmt.Errorf("unterminated module block")
	}
	return m, nil
}
