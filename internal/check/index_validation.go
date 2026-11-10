package check

import (
	"encoding/json"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/units"
	"github.com/Innocent-children/taskbelay/internal/validation"
	"path"
	"strings"
	"unicode/utf8"
)

type globReference struct{ Path, Pattern string }

func (x *repositoryIndex) prefix(kind, prefix string) map[string]map[string]string {
	out := map[string]map[string]string{}
	if x.id != "" && x.err == nil {
		rows, err := x.s.IndexPrefixRows(x.ctx, x.id, kind, prefix, IndexBatch, IndexBatchBytes)
		if err != nil {
			x.err = err
		}
		for _, r := range rows {
			if out[r.Key] == nil {
				out[r.Key] = map[string]string{}
			}
			out[r.Key][r.Member] = r.Data
		}
	}
	for key, members := range x.rows[kind] {
		if strings.HasPrefix(key, prefix) {
			k := rowKey(kind, key)
			if x.loaded[k] || x.id == "" {
				out[key] = members
				continue
			}
			if out[key] == nil {
				out[key] = map[string]string{}
			}
			for member, raw := range members {
				if raw == "" {
					delete(out[key], member)
				} else {
					out[key][member] = raw
				}
			}
		}
	}
	return out
}
func (x *repositoryIndex) refs(f units.File) []string {
	set := map[string]bool{}
	matches, err := x.matchPatterns("doc", strings.Join(f.Lines, ""), false)
	if err != nil {
		x.err = err
	}
	for m := range matches {
		for p := range x.read("doc_pattern", m) {
			set[p] = true
		}
	}
	for _, glob := range validation.Globs(f) {
		for _, rows := range x.prefix("doc_pattern", validation.GlobPrefix(glob)) {
			for p := range rows {
				if validation.MatchesGlob(glob, p) {
					set[p] = true
				}
			}
		}
	}
	return keys(set)
}
func (x *repositoryIndex) refCount(p string) int { var n int; x.get("ref_count", p, &n); return n }
func (x *repositoryIndex) adjustRefs(f indexFile, sign int) {
	for _, p := range f.References {
		x.put("ref_count", p, "", x.refCount(p)+sign)
	}
}
func (x *repositoryIndex) summaryFile(p string, sign int) {
	f := x.fileInfo(p)
	if f.Blob != "" {
		if err := x.meta.Validation.Adjust(f.Content, f.Semantic, !f.Role.Document || x.refCount(p) > 0, sign); err != nil {
			x.err = err
		}
	}
}

/**
 * 新增文档按字面引用和 glob 反查现有代码；新代码按文档自动机查询。
 * 三字以下的引用使用短串倒排，避免全文索引退化为逐文件扫描。
 */
func (x *repositoryIndex) saveText(build ledger.IndexBuild, f units.File) error {
	text := strings.Join(f.Lines, "")
	short := map[string]bool{}
	previous := -1
	for i := 0; i < len(text); {
		_, width := utf8.DecodeRuneInString(text[i:])
		end := i + width
		short[text[i:end]] = true
		if previous >= 0 {
			short[text[previous:end]] = true
		}
		previous = i
		i = end
	}
	info := x.fileInfo(f.Path)
	for _, key := range keys(short) {
		x.put("text_short", key, f.Path, true)
		info.Rows = append(info.Rows, indexMember{"text_short", key, f.Path})
	}
	x.put("file", f.Path, "", info)
	if err := x.s.SaveIndexText(x.ctx, build, f.Path, text); err != nil {
		return fmt.Errorf("保存引用索引 %s: %w", f.Path, err)
	}
	return nil
}
func (x *repositoryIndex) codeReferences(p string) []string {
	set := map[string]bool{}
	for _, pattern := range validation.DocumentPatterns(p) {
		if utf8.RuneCountInString(pattern) < 3 {
			for code := range x.read("text_short", pattern) {
				set[code] = true
			}
		} else if x.id != "" {
			paths, err := x.s.IndexTextMatches(x.ctx, x.id, pattern)
			if err != nil {
				x.err = err
			}
			for _, code := range paths {
				set[code] = true
			}
		}
	}
	for _, text := range []string{p, path.Base(p)} {
		for i := 0; i <= len(text); i++ {
			for _, raw := range x.read("glob", text[:i]) {
				var v globReference
				if err := json.Unmarshal([]byte(raw), &v); err != nil {
					x.err = err
				}
				if validation.MatchesGlob(v.Pattern, p) {
					set[v.Path] = true
				}
			}
		}
	}
	return keys(set)
}

func (x *repositoryIndex) updateValidation(before *repositoryIndex, paths []string) error {
	codes := map[string]bool{}
	affected := map[string]bool{}
	for _, p := range paths {
		affected[p] = true
		codes[p] = true
		if x.fileInfo(p).Role.Document || before.fileInfo(p).Role.Document {
			for _, code := range x.codeReferences(p) {
				codes[code] = true
			}
		}
	}
	for _, p := range keys(codes) {
		old := before.fileInfo(p)
		next := x.fileInfo(p)
		for _, d := range old.References {
			affected[d] = true
		}
		x.adjustRefs(old, -1)
		if next.Blob != "" && !next.Role.Document {
			next.References = x.refs(x.load(p))
			for _, d := range next.References {
				affected[d] = true
			}
			x.adjustRefs(next, 1)
		} else {
			next.References = nil
		}
		if next.Blob != "" {
			x.put("file", p, "", next)
		}
	}
	for _, p := range keys(affected) {
		old := before.fileInfo(p)
		if old.Blob != "" {
			x.meta.Validation.Adjust(old.Content, old.Semantic, !old.Role.Document || before.refCount(p) > 0, -1)
		}
		x.summaryFile(p, 1)
	}
	return x.err
}
