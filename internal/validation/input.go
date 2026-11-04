package validation

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/units"
	"path"
	"sort"
	"strings"
)

func sorted[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

type Command struct {
	Program                             string
	Args                                []string
	Directory, Shell, Environment, Tool string
	Mixed, Writes, Unknown              bool
}
type Summary struct {
	Content, Semantic  [32]byte
	Included, Excluded int
}
type InputData struct {
	Summary Summary
	Set     string
	Command Command
}
type Identity struct {
	Hash, Content, Semantic, Command, Environment, Tool, Set string
	Included, Excluded                                       int
	Complete                                                 bool
}

func CommandKey(c Command) string {
	return units.Hash(append([]string{c.Program, c.Directory, c.Shell}, c.Args...)...)
}

/**
 * 每个文件的摘要包含路径和模式，按 256 位加法组合；覆盖层只减去旧项、加入新项。
 */
func FileDigests(f units.File) (string, string) {
	blob := f.Blob
	if blob == "" {
		blob = units.Hash(strings.Join(f.Lines, ""))
	}
	semantic := strings.Join(units.Sequence(f.Tokens, false), "\x00")
	if f.Binary || f.Mode == "160000" || f.Mode == "120000" {
		semantic = blob
	}
	return units.Hash(f.Path, f.Mode, blob), units.Hash(f.Path, f.Mode, semantic)
}
func (s *Summary) Adjust(content, semantic string, included bool, sign int) error {
	if !included {
		s.Excluded += sign
		return nil
	}
	a, err := hex.DecodeString(content)
	if err != nil || len(a) != len(s.Content) {
		return fmt.Errorf("文件内容摘要无效")
	}
	b, err := hex.DecodeString(semantic)
	if err != nil || len(b) != len(s.Semantic) {
		return fmt.Errorf("文件语义摘要无效")
	}
	s.Included += sign
	for _, item := range []struct {
		target *[32]byte
		bytes  []byte
	}{{&s.Content, a}, {&s.Semantic, b}} {
		carry := 0
		for i := len(item.target) - 1; i >= 0; i-- {
			v := int(item.target[i]) + sign*int(item.bytes[i]) + carry
			item.target[i] = byte(v)
			carry = v >> 8
		}
	}
	return nil
}
func Input(ctx context.Context, in InputData) (Identity, error) {
	c := in.Command
	r := Identity{Command: CommandKey(c), Environment: c.Environment, Tool: c.Tool, Set: in.Set, Included: in.Summary.Included, Excluded: in.Summary.Excluded, Complete: c.Tool != "" && c.Environment != "" && !c.Unknown}
	r.Content = hex.EncodeToString(in.Summary.Content[:])
	r.Semantic = hex.EncodeToString(in.Summary.Semantic[:])
	r.Hash = units.Hash(r.Content, r.Command, r.Environment, r.Tool)
	return r, ctx.Err()
}
func DocumentPatterns(p string) []string {
	keys := []string{p, path.Base(p)}
	dir := path.Dir(p)
	for dir != "." && dir != "/" {
		keys = append(keys, dir, path.Base(dir))
		dir = path.Dir(dir)
	}
	return keys
}
func Globs(f units.File) []string {
	var out []string
	for _, t := range f.Tokens {
		for _, p := range strings.FieldsFunc(units.Literal(t), func(r rune) bool {
			return r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == '"' || r == '\'' || r == '`'
		}) {
			if strings.ContainsAny(p, "*?[") {
				out = append(out, p)
			}
		}
	}
	return out
}
func GlobPrefix(p string) string {
	if i := strings.IndexAny(p, "*?["); i >= 0 {
		return p[:i]
	}
	return p
}
func MatchesGlob(pattern, target string) bool {
	matched, _ := path.Match(pattern, target)
	base, _ := path.Match(pattern, path.Base(target))
	return matched || base || globstar(pattern, target)
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
