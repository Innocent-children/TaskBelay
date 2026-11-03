package validation

import (
	"context"
	"path"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
)

type Command struct {
	Program                             string
	Args                                []string
	Directory, Shell, Environment, Tool string
	Mixed, Writes, Unknown              bool
}
type InputData struct {
	Files   map[string]units.File
	Roles   map[string]repostats.Role
	Command Command
}
type Identity struct {
	Hash, Content, Semantic, Command, Environment, Tool string
	Included, Excluded                                  []string
	Complete                                            bool
}

func CommandKey(c Command) string {
	parts := []string{c.Program, c.Directory, c.Shell}
	parts = append(parts, c.Args...)
	return units.Hash(parts...)
}

func Input(ctx context.Context, in InputData) (Identity, error) {
	r := Identity{Command: CommandKey(in.Command), Environment: in.Command.Environment, Tool: in.Command.Tool, Complete: in.Command.Tool != "" && in.Command.Environment != "" && !in.Command.Unknown}
	patterns := map[string][]string{}
	for _, p := range sorted(in.Files) {
		if !in.Roles[p].Document {
			continue
		}
		candidates := []string{p, path.Base(p)}
		dir := path.Dir(p)
		for dir != "." && dir != "/" {
			candidates = append(candidates, dir, path.Base(dir))
			dir = path.Dir(dir)
		}
		for _, pattern := range candidates {
			patterns[pattern] = append(patterns[pattern], p)
		}
	}
	matcher, err := units.NewMatcher(ctx, sorted(patterns))
	if err != nil {
		return r, err
	}
	referenced := map[string]bool{}
	for _, p := range sorted(in.Files) {
		if in.Roles[p].Document {
			continue
		}
		f := in.Files[p]
		text := strings.Join(f.Lines, "")
		matches, err := matcher.Find(ctx, text)
		if err != nil {
			return r, err
		}
		for _, m := range matches {
			for _, target := range patterns[m.Pattern] {
				referenced[target] = true
			}
		}
		for _, t := range f.Tokens {
			for _, pattern := range strings.FieldsFunc(units.Literal(t), func(r rune) bool {
				return r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == '"' || r == '\'' || r == '`'
			}) {
				if !strings.ContainsAny(pattern, "*?[") {
					continue
				}
				for _, target := range sorted(in.Files) {
					if !in.Roles[target].Document {
						continue
					}
					matched, _ := path.Match(pattern, target)
					base, _ := path.Match(pattern, path.Base(target))
					if matched || base || globstar(pattern, target) {
						referenced[target] = true
					}
				}
			}
		}
	}
	var content, semantic []string
	for _, p := range sorted(in.Files) {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		f := in.Files[p]
		if in.Roles[p].Document && !referenced[p] {
			r.Excluded = append(r.Excluded, p)
			continue
		}
		r.Included = append(r.Included, p)
		blob := f.Blob
		if blob == "" {
			blob = units.Hash(strings.Join(f.Lines, ""))
		}
		content = append(content, p, f.Mode, blob)
		semantic = append(semantic, p, f.Mode, strings.Join(units.Sequence(f.Tokens, false), "\x00"))
		if f.Binary {
			semantic = append(semantic, blob)
		}
	}
	r.Content = units.Hash(content...)
	r.Semantic = units.Hash(semantic...)
	r.Hash = units.Hash(r.Content, r.Command, r.Environment, r.Tool)
	return r, nil
}

func globstar(pattern, target string) bool {
	parts := strings.Split(pattern, "/**/")
	if len(parts) != 2 {
		return false
	}
	if !strings.HasPrefix(target, strings.TrimSuffix(parts[0], "/")+"/") {
		return false
	}
	rest := strings.TrimPrefix(target, strings.TrimSuffix(parts[0], "/")+"/")
	for {
		if ok, _ := path.Match(parts[1], rest); ok {
			return true
		}
		i := strings.IndexByte(rest, '/')
		if i < 0 {
			return false
		}
		rest = rest[i+1:]
	}
}
func sorted[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
