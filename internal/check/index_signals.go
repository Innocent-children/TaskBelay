package check

import (
	"encoding/json"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"maps"
	"path"
	"slices"
	"strings"
)

func (s indexedSnapshot) queries() *signals.Queries {
	x, origin := s.Current, s.Origin
	q := &signals.Queries{Prior: origin.meta.Indirection, Mean: origin.meta.SimilarityMean, Variance: origin.meta.SimilarityVariance}
	callsByName := map[string][]signals.CallSite{}
	headsByWord := map[string][]signals.Head{}
	syntaxByExtension := map[string]units.Syntax{}
	q.Syntax = func(p string) units.Syntax {
		ext := path.Ext(p)
		if syntax, ok := syntaxByExtension[ext]; ok {
			return syntax
		}
		var syntax units.Syntax
		x.get("syntax", ext, &syntax)
		syntaxByExtension[ext] = syntax
		return syntax
	}
	q.Structure = func(key string) signals.UnitStructure { return x.unit(key).Structure }
	q.Calls = func(name string) []signals.CallSite {
		if calls, ok := callsByName[name]; ok {
			return calls
		}
		var out []signals.CallSite
		rows := x.read("calls", name)
		for _, p := range keys(rows) {
			var calls [][]units.Token
			if err := json.Unmarshal([]byte(rows[p]), &calls); err != nil {
				x.err = err
			}
			for _, args := range calls {
				out = append(out, signals.CallSite{Path: p, Args: args})
			}
		}
		callsByName[name] = out
		return out
	}
	q.Heads = func(word string) []signals.Head {
		if heads, ok := headsByWord[word]; ok {
			return heads
		}
		var out []signals.Head
		for _, id := range keys(x.read("head", word)) {
			u := x.unit(id)
			out = append(out, signals.Head{ID: id, Name: u.Unit.Name, Words: u.Head})
		}
		headsByWord[word] = out
		return out
	}
	q.Defined = func(name string) bool { return len(x.read("name", name)) > 0 }
	q.Uses = func(name string) int { return x.stats(name).Uses }
	q.Similar = func(sig []uint64) []signals.SimilarUnit {
		set := map[string]bool{}
		for _, bucket := range signals.SimilarityBuckets(sig) {
			for key := range origin.read("lsh", bucket) {
				set[key] = true
			}
		}
		var out []signals.SimilarUnit
		for _, key := range keys(set) {
			u := origin.unit(key)
			out = append(out, signals.SimilarUnit{Path: u.Path, Unit: u.Unit, Grams: u.Grams, Signature: u.Signature, Callable: u.Structure.Callable})
		}
		return out
	}
	return q
}

/**
 * 用例只补齐被改动声明及其被测对象的直接引用邻域；其余计算查询计数和首行汇总。
 */
func (s *indexedSnapshot) caseNeighbourhood() {
	paths := s.casePaths()
	s.Before = s.Origin.view(paths)
	s.After = s.Current.view(paths)
}

func (s indexedSnapshot) casePaths() []string {
	paths := map[string]bool{}
	names := map[string]bool{}
	caseNames := map[string]bool{}
	for _, p := range s.Paths {
		paths[p] = true
		for _, u := range s.After.Files[p].Units {
			if !s.After.Roles[p].Test && u.Name != "" {
				names[u.Name] = true
			}
			if s.After.TestUnits[repostats.UnitKey(p, u)] {
				collectIdentifiers(caseNames, u.Tokens)
			}
		}
	}
	requestedNames := keys(caseNames)
	s.Current.prefetchReferenceFiles("name", requestedNames)
	if s.Current.err != nil {
		return nil
	}
	for _, name := range requestedNames {
		found := map[string]bool{}
		if !s.Current.collectReferencePaths("name", name, false, found) {
			return nil
		}
		if len(found) > 0 {
			names[name] = true
			maps.Copy(paths, found)
		}
	}
	referencedNames := keys(names)
	s.Current.prefetchReferenceFiles("word", referencedNames)
	if s.Current.err != nil {
		return nil
	}
	for _, name := range referencedNames {
		if !s.Current.collectReferencePaths("word", name, true, paths) {
			return nil
		}
	}
	// 只从当前邻域的既有测试收集名字；后面补入的声明不再扩展邻域。
	otherNames := map[string]bool{}
	for _, p := range keys(paths) {
		if err := s.Current.ctx.Err(); err != nil {
			s.Current.err = err
			return nil
		}
		if s.Current.fileInfo(p).Role.Test {
			collectIdentifiers(otherNames, s.Current.load(p).Tokens)
		}
		if s.Current.err != nil {
			return nil
		}
	}
	for name := range caseNames {
		delete(otherNames, name)
	}
	remainingNames := keys(otherNames)
	s.Current.prefetchReferenceFiles("name", remainingNames)
	if s.Current.err != nil {
		return nil
	}
	for _, name := range remainingNames {
		if !s.Current.collectReferencePaths("name", name, false, paths) {
			return nil
		}
	}
	return keys(paths)
}

/**
 * 预取的是原始行。缓存命中后仍须逐个检查取消，避免超时后继续解码其余文件的元数据。
 */
func (x *repositoryIndex) collectReferencePaths(kind, key string, wantTest bool, paths map[string]bool) bool {
	members := x.read(kind, key)
	if x.err != nil {
		return false
	}
	for member := range members {
		if err := x.ctx.Err(); err != nil {
			x.err = err
			return false
		}
		p := indexUnitPath(member)
		info := x.fileInfo(p)
		if x.err != nil {
			return false
		}
		if info.Role.Test == wantTest {
			paths[p] = true
		}
	}
	return true
}

func collectIdentifiers(names map[string]bool, tokens []units.Token) {
	for _, token := range tokens {
		if token.Kind == "identifier" {
			names[token.Text] = true
		}
	}
}

/**
 * UnitKey 由路径、冒号和单元哈希组成；从末尾拆分，保留路径中的冒号。
 */
func indexUnitPath(key string) string { return key[:strings.LastIndexByte(key, ':')] }

/**
 * 倒排成员的文件角色随后会被逐一查询，只预取这些文件尚未缓存的元数据。
 */
func (x *repositoryIndex) prefetchReferenceFiles(kind string, requested []string) {
	if !usesPublishedOnly(x.ctx) || x.base != nil || x.id == "" || x.err != nil {
		return
	}
	x.prefetchMembers(kind, requested)
	if x.err != nil {
		return
	}
	paths := map[string]bool{}
	for _, key := range requested {
		for member := range x.read(kind, key) {
			if err := x.ctx.Err(); err != nil {
				x.err = err
				return
			}
			p := indexUnitPath(member)
			if _, cached := x.fileCache[p]; !cached {
				paths[p] = true
			}
		}
	}
	x.prefetchMembers("file", keys(paths))
}

/**
 * 编辑前只分批预取本来会逐键查询的倒排；静态叠加索引仍沿用原读取方式。
 */
func (x *repositoryIndex) prefetchMembers(kind string, requested []string) {
	if !usesPublishedOnly(x.ctx) || x.base != nil || x.id == "" || x.err != nil {
		return
	}
	pending := map[string]bool{}
	for _, key := range requested {
		if !x.loaded[rowKey(kind, key)] {
			pending[key] = true
		}
	}
	for batch := range slices.Chunk(keys(pending), IndexBatch) {
		rows, err := x.s.IndexRowsForKeys(x.ctx, x.id, kind, batch)
		if err != nil {
			x.err = err
			return
		}
		for _, key := range batch {
			x.setMembers(kind, key, map[string]string{})
			x.loaded[rowKey(kind, key)] = true
		}
		for _, row := range rows {
			x.rows[kind][row.Key][row.Member] = row.Data
		}
	}
}
