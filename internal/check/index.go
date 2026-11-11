package check

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/changes"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/signals"
	"github.com/Innocent-children/taskbelay/internal/units"
	"github.com/Innocent-children/taskbelay/internal/validation"
	"iter"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
)

type indexMeta struct {
	Generation                         int64
	Tree                               string
	Units                              int
	Converged                          bool
	Iterations                         int
	LogPrior                           float64
	TestExtensions                     map[string]bool
	TestFilesIndexed                   bool `json:",omitempty"`
	TestWordPathsIndexed               bool `json:",omitempty"`
	Matchers                           map[string][]string
	SimilarityMean, SimilarityVariance float64
	Indirection                        map[string]float64
	VariationCounts                    map[string][2]int
	Validation                         validation.Summary
}

// 文件只引用它拥有的行，不需要在归属清单中重复保存 Data。
type indexMember struct{ Kind, Key, Member string }

type indexFile struct {
	Blob, Mode, Cache string
	Role              repostats.Role
	Units             []string
	Tests, Seeds      map[string]bool
	Rows              []indexMember
	References        []string
	Content, Semantic string
}
type indexUnit struct {
	Path             string
	Unit             units.Unit
	Words            map[string]int
	Head             map[string]bool
	Grams, Signature []uint64
	Structure        signals.UnitStructure
}
type wordStats struct {
	DF, Uses int
	Counts   map[int]int
}
type repositoryIndex struct {
	base        *repositoryIndex
	ctx         context.Context
	s           *ledger.Store
	w           gitstate.Workspace
	id          string
	variant     string
	meta        indexMeta
	rows        map[string]map[string]map[string]string // kind → key → member
	loaded      map[string]bool
	volumes     map[string]changes.VolumeNode
	volumeDirty map[string]bool
	fileCache   map[string]indexFile
	unitCache   map[string]indexUnit
	statCache   map[string]wordStats
	writes      map[string]ledger.IndexRow
	files       map[string]units.File
	pages       map[string][]units.MatcherState
	preloaded   map[string]bool
	err         error
}

// 叠加当前改动时，这几类条目会被逐键读取数万次；整类一次读入，避免逐条查询账本。
var preloadKinds = map[string]bool{"volume": true, "test_model": true, "stats": true}

func (x *repositoryIndex) preload(kind, prefix string) {
	rows, err := x.s.IndexPrefixRows(x.ctx, x.id, kind, prefix, IndexBatch, IndexBatchBytes)
	if err != nil {
		x.err = err
		return
	}
	for len(rows) > 0 {
		key := rows[0].Key
		end := 1
		for end < len(rows) && rows[end].Key == key {
			end++
		}
		group := rows[:end]
		rows = rows[end:]
		k := rowKey(kind, key)
		if x.loaded[k] {
			continue
		}
		members := make(map[string]string, len(group))
		for _, row := range group {
			members[row.Member] = row.Data
		}
		x.setMembers(kind, key, members)
		x.loaded[k] = true
	}
}

func rowKey(kind, key string) string { return kind + "\x00" + key }
func encode(v any) string {
	// 倒排行里大部分值是标记和计数，直接得到同样的 JSON 标量。
	switch v := v.(type) {
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func (x *repositoryIndex) setMembers(kind, key string, members map[string]string) {
	if x.rows[kind] == nil {
		x.rows[kind] = make(map[string]map[string]string)
	}
	x.rows[kind][key] = members
}
func (x *repositoryIndex) read(kind, key string) map[string]string {
	if x.id == "" {
		return x.rows[kind][key]
	}
	k := rowKey(kind, key)
	if x.loaded[k] {
		return x.rows[kind][key]
	}
	r := map[string]string{}
	if x.base != nil && x.err == nil {
		for member, raw := range x.base.read(kind, key) {
			r[member] = raw
		}
		x.err = x.base.err
	} else if x.err == nil && preloadKinds[kind] {
		if !x.preloaded[kind] {
			x.preload(kind, "")
			x.preloaded[kind] = true
		}
		if x.loaded[k] {
			return x.rows[kind][key]
		}
	} else if x.err == nil {
		rows, err := x.s.IndexRows(x.ctx, x.id, kind, key)
		if err != nil {
			x.err = err
		}
		for _, v := range rows {
			r[v.Member] = v.Data
		}
	}
	for member, raw := range x.rows[kind][key] {
		if raw == "" {
			delete(r, member)
		} else {
			r[member] = raw
		}
	}
	x.setMembers(kind, key, r)
	x.loaded[k] = true
	return r
}
func (x *repositoryIndex) get(kind, key string, out any) bool {
	raw, ok := x.read(kind, key)[""]
	if !ok {
		return false
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		x.err = err
		return false
	}
	return true
}
func (x *repositoryIndex) put(kind, key, member string, v any) {
	// 初建时文件归属清单还要补入引用和短串，只在发布最终版本时编码一次。
	if kind == "file" && x.id == "" {
		f, _ := v.(indexFile)
		x.fileCache[key] = f
		return
	}
	raw := ""
	if v != nil {
		raw = encode(v)
	}
	var k string
	complete := x.id == ""
	if !complete {
		k = rowKey(kind, key)
		complete = x.loaded[k]
	}
	r := x.rows[kind][key]
	if r == nil {
		r = map[string]string{}
		x.setMembers(kind, key, r)
	}
	if old, exists := r[member]; old == raw && (exists || complete) {
		return
	}
	if raw == "" && complete {
		delete(r, member)
	} else {
		r[member] = raw
	}
	if x.id != "" {
		x.writes[k+"\x00"+member] = ledger.IndexRow{Kind: kind, Key: key, Member: member, Data: raw}
	}
	switch kind {
	case "file":
		f, _ := v.(indexFile)
		x.fileCache[key] = f
	case "unit":
		u, _ := v.(indexUnit)
		x.unitCache[key] = u
	case "stats":
		s, _ := v.(wordStats)
		x.statCache[key] = s
	}
}
func (x *repositoryIndex) fileInfo(p string) indexFile {
	if f, ok := x.fileCache[p]; ok {
		return f
	}
	var f indexFile
	x.get("file", p, &f)
	x.fileCache[p] = f
	return f
}
func (x *repositoryIndex) unit(key string) indexUnit {
	if u, ok := x.unitCache[key]; ok {
		return u
	}
	if _, local := x.rows["unit"][key]; !local && x.base != nil && x.err == nil {
		u := x.base.unit(key)
		x.err = x.base.err
		x.unitCache[key] = u
		return u
	}
	var u indexUnit
	x.get("unit", key, &u)
	x.unitCache[key] = u
	return u
}
func (x *repositoryIndex) stats(word string) wordStats {
	if v, ok := x.statCache[word]; ok {
		return v
	}
	var v wordStats
	x.get("stats", word, &v)
	if v.Counts == nil {
		v.Counts = map[int]int{}
	}
	x.statCache[word] = v
	return v
}
func (x *repositoryIndex) volumeNode(key string) changes.VolumeNode {
	if v, ok := x.volumes[key]; ok {
		return v
	}
	var v changes.VolumeNode
	x.get("volume", key, &v)
	x.volumes[key] = v
	return v
}
func (x *repositoryIndex) setVolume(key string, v changes.VolumeNode) {
	x.volumes[key] = v
	if x.id != "" {
		x.volumeDirty[key] = true
	}
}
func (x *repositoryIndex) statDelta(word string, counts map[int]int, uses int) {
	old := x.stats(word)
	if x.id != "" && len(counts) != 0 {
		for c, n := range old.Counts {
			changes.AdjustVolume(old.DF, c, -n, x.volumeNode, x.setVolume)
		}
	}
	for c, n := range counts {
		old.Counts[c] += n
		old.DF += n
		if old.Counts[c] == 0 {
			delete(old.Counts, c)
		}
	}
	old.Uses += uses
	if x.id == "" {
		x.statCache[word] = old
		return
	}
	if len(counts) != 0 {
		for c, n := range old.Counts {
			changes.AdjustVolume(old.DF, c, n, x.volumeNode, x.setVolume)
		}
	}
	x.put("stats", word, "", old)
}

// 初建时只需最终词频。具有相同文档频率和单元词频的词对体积贡献相同，
// 先按整数累计出现数，再一次性建立前缀统计，避免留下中间词频的空节点。
func (x *repositoryIndex) buildVolume() {
	groups := map[int]map[int]int{}
	for _, stats := range x.statCache {
		if len(stats.Counts) == 0 {
			continue
		}
		group := groups[stats.DF]
		if group == nil {
			group = map[int]int{}
			groups[stats.DF] = group
		}
		for count, n := range stats.Counts {
			group[count] += n
		}
	}
	for _, df := range slices.Sorted(maps.Keys(groups)) {
		for _, c := range slices.Sorted(maps.Keys(groups[df])) {
			changes.AdjustVolume(df, c, groups[df][c], x.volumeNode, x.setVolume)
		}
	}
}
func (x *repositoryIndex) load(p string) units.File {
	if f, ok := x.files[p]; ok {
		return f
	}
	info := x.fileInfo(p)
	if info.Blob == "" {
		return units.File{}
	}
	f, found, err := cachedFile(x.ctx, x.s, info.Cache)
	if err != nil {
		x.err = err
	} else if !found {
		// 文件分析是可以重算的缓存：被清理后按索引记录的内容重新分析并保存。
		var syn units.Syntax
		x.get("syntax", path.Ext(p), &syn)
		data, readErr := x.w.ReadBlob(x.ctx, info.Blob)
		if readErr == nil {
			f, readErr = units.Analyze(x.ctx, units.Source{Path: p, Blob: info.Blob, Mode: info.Mode, Data: data}, syn)
		}
		if readErr == nil {
			readErr = saveCachedFile(x.ctx, x.s, info.Cache, f)
		}
		if readErr != nil {
			x.err = fmt.Errorf("索引文件分析缺失：%s: %w", p, readErr)
		}
	}
	f.Path, f.Blob, f.Mode = p, info.Blob, info.Mode
	var syn units.Syntax
	x.get("syntax", path.Ext(p), &syn)
	f, err = units.Reclassify(x.ctx, f, syn)
	if err != nil {
		x.err = err
	}
	x.files[p] = f
	return f
}
func (x *repositoryIndex) view(paths []string) repostats.Repository {
	r := repostats.Repository{Tree: x.meta.Tree, UnitCount: x.meta.Units, Converged: x.meta.Converged, TestIterations: x.meta.Iterations, Files: map[string]units.File{}, Roles: map[string]repostats.Role{}, Syntax: map[string]units.Syntax{}, TestUnits: map[string]bool{}, TestSeeds: map[string]bool{}, Tests: repostats.TestModel{LogPrior: x.meta.LogPrior, Weights: map[string]float64{}, Extensions: x.meta.TestExtensions}}
	r.FrequencyLookup = func(w string) int { return x.stats(w).DF }
	r.NamesLookup = func(text string) ([]string, error) { names := x.namesIn(text); return names, x.err }
	r.TestWordLookup = func(w string, fold bool) float64 {
		var v float64
		k := "test_word"
		if fold {
			k = "test_fold"
			w = strings.ToLower(w)
		}
		x.get(k, w, &v)
		return v
	}
	r.EntryLookup = func(p string, u units.Unit) bool {
		for _, k := range entryKeys(p, u) {
			if len(x.read("entry", k)) > 0 {
				return true
			}
		}
		return false
	}
	for _, p := range paths {
		f := x.load(p)
		if f.Blob == "" {
			continue
		}
		info := x.fileInfo(p)
		r.Files[p], r.Roles[p] = f, info.Role
		var syn units.Syntax
		x.get("syntax", path.Ext(p), &syn)
		r.Syntax[path.Ext(p)] = syn
		for k, v := range info.Tests {
			r.TestUnits[k] = v
		}
		for k, v := range info.Seeds {
			r.TestSeeds[k] = v
		}
	}
	return r
}
func entryKeys(p string, u units.Unit) []string {
	var keys []string
	runes := []rune(u.Name)
	for i := 1; i <= len(runes); i++ {
		keys = append(keys, "prefix\x00"+string(runes[:i]), "suffix\x00"+string(runes[len(runes)-i:]))
	}
	// 已学到的路径模式可能是路径中间一段；只枚举当前目标的候选片段。
	for end := range p {
		if p[end] != '/' {
			continue
		}
		for start := 0; start <= end; start++ {
			keys = append(keys, "path\x00"+p[start:end+1])
		}
	}
	for _, t := range u.Tokens {
		if t.End <= u.HeaderEnd {
			keys = append(keys, "header\x00"+t.Text)
		}
	}
	return keys
}

func (x *repositoryIndex) matchPatterns(kind, text string, whole bool) (map[string]bool, error) {
	out := map[string]bool{}
	for _, root := range x.meta.Matchers[kind] {
		lastPage := -1
		var states []units.MatcherState
		err := units.ScanStates(x.ctx, text, func(i int) (units.MatcherState, error) {
			page, offset := i/MatcherPageSize, i%MatcherPageSize
			if page != lastPage {
				key := root + ":" + strconv.Itoa(page)
				var ok bool
				states, ok = x.pages[key]
				if !ok {
					x.get("matcher", key, &states)
					// 读取失败的页不缓存，也不当作缺页。
					if x.err != nil {
						return units.MatcherState{}, x.err
					}
					x.pages[key] = states
				}
				lastPage = page
			}
			if offset >= len(states) {
				return units.MatcherState{}, fmt.Errorf("自动机索引页缺失：%s/%d", root, page)
			}
			return states[offset], x.err
		}, func(m units.Match) {
			if !out[m.Pattern] && (!whole || units.WholeWord(text, m)) {
				out[m.Pattern] = true
			}
		})
		if err != nil {
			return nil, err
		}
	}
	return out, x.err
}

func (x *repositoryIndex) namesIn(text string) []string {
	set, err := x.matchPatterns("case", text, true)
	if err != nil {
		x.err = err
	}
	for p := range set {
		if len(x.read("case_name", p)) == 0 {
			delete(set, p)
		}
	}
	return keys(set)
}
func (x *repositoryIndex) addMatcher(kind string, patterns []string) {
	if len(patterns) == 0 {
		return
	}
	set := map[string]bool{}
	for _, p := range patterns {
		set[p] = true
	}
	patterns = keys(set)
	m, err := units.NewMatcher(x.ctx, patterns)
	if err != nil {
		x.err = err
		return
	}
	root := units.Hash(kind, strings.Join(patterns, "\x00"))
	x.meta.Matchers[kind] = append(append([]string(nil), x.meta.Matchers[kind]...), root)
	states := m.States()
	for i := 0; i < len(states); i += MatcherPageSize {
		x.put("matcher", root+":"+strconv.Itoa(i/MatcherPageSize), "", states[i:min(i+MatcherPageSize, len(states))])
	}
}

func (x *repositoryIndex) graph() changes.Graph {
	g := changes.Graph{Volume: changes.GraphVolume(x.meta.Units, x.volumeNode)}
	g.NodeDegree = func(ctx context.Context, node string) (float64, error) {
		d := 0.0
		switch {
		case strings.HasPrefix(node, "u:"):
			u := x.unit(strings.TrimPrefix(node, "u:"))
			if u.Path != "" {
				d = changes.FileWeight
			}
			for w, n := range u.Words {
				d += changes.WordWeight(x.meta.Units, x.stats(w).DF, n)
			}
		case strings.HasPrefix(node, "f:"):
			d = float64(len(x.fileInfo(strings.TrimPrefix(node, "f:")).Units)) * changes.FileWeight
		case strings.HasPrefix(node, "w:"):
			s := x.stats(strings.TrimPrefix(node, "w:"))
			for n, count := range s.Counts {
				d += float64(count) * changes.WordWeight(x.meta.Units, s.DF, n)
			}
		}
		return d, x.err
	}
	g.Neighbours = func(ctx context.Context, node string) (map[string]float64, error) {
		e := map[string]float64{}
		switch {
		case strings.HasPrefix(node, "u:"):
			u := x.unit(strings.TrimPrefix(node, "u:"))
			if u.Path != "" {
				e["f:"+u.Path] = changes.FileWeight
			}
			for w, n := range u.Words {
				weight := changes.WordWeight(x.meta.Units, x.stats(w).DF, n)
				if weight > 0 {
					e["w:"+w] = weight
				}
			}
		case strings.HasPrefix(node, "f:"):
			for _, u := range x.fileInfo(strings.TrimPrefix(node, "f:")).Units {
				e["u:"+u] = changes.FileWeight
			}
		case strings.HasPrefix(node, "w:"):
			w := strings.TrimPrefix(node, "w:")
			s := x.stats(w)
			for u, raw := range x.read("word", w) {
				var n int
				if err := json.Unmarshal([]byte(raw), &n); err != nil {
					return nil, err
				}
				weight := changes.WordWeight(x.meta.Units, s.DF, n)
				if weight > 0 {
					e["u:"+u] = weight
				}
			}
		}
		return e, x.err
	}
	g.Match = func(ctx context.Context, text string) (map[string][]string, error) {
		out := map[string][]string{}
		patterns, err := x.matchPatterns("graph", text, true)
		if err != nil {
			return nil, err
		}
		for pattern := range patterns {
			seen := map[string]bool{}
			for _, raw := range x.read("match", pattern) {
				if seen[raw] {
					continue
				}
				seen[raw] = true
				var node string
				if err := json.Unmarshal([]byte(raw), &node); err != nil {
					return nil, err
				}
				if strings.HasPrefix(node, "lexeme:") {
					var word string
					x.get("lexeme", strings.TrimPrefix(node, "lexeme:"), &word)
					node = "w:" + word
				}
				if strings.HasPrefix(node, "w:") {
					s := x.stats(node[2:])
					active := false
					for n := range s.Counts {
						active = active || changes.WordWeight(x.meta.Units, s.DF, n) > 0
					}
					if !active {
						continue
					}
				}
				out[pattern] = append(out[pattern], node)
			}
		}
		return out, x.err
	}
	return g
}

func (x *repositoryIndex) changeFile(p string, f units.File, r repostats.Repository, sign int) {
	old := x.fileInfo(p)
	if sign < 0 {
		for _, row := range old.Rows {
			x.put(row.Kind, row.Key, row.Member, nil)
		}
		x.put("file", p, "", nil)
	}
	info := indexFile{Blob: f.Blob, Mode: f.Mode, Cache: units.Hash(Algorithm, f.Blob, r.SyntaxFor(p).Key()), Role: r.Roles[p], Tests: map[string]bool{}, Seeds: map[string]bool{}}
	owned := map[indexMember]bool{}
	put := func(kind, key, member string, v any) {
		if sign < 0 {
			return
		}
		row := indexMember{kind, key, member}
		if x.id == "" && kind != "doc_pattern" {
			// 初建时成员带有所属文件或单元，行目录本身即可判重。
			if _, ok := x.rows[kind][key][member]; ok {
				return
			}
		} else {
			if owned[row] {
				return
			}
			owned[row] = true
		}
		x.put(kind, key, member, v)
		info.Rows = append(info.Rows, row)
	}
	x.meta.Units += sign * len(f.Units)
	words := map[string]map[int]int{}
	uses := map[string]int{}
	for _, t := range f.Tokens {
		if !r.Roles[p].Test {
			uses[units.Literal(t)] += sign
		}
	}
	put("match", strings.ToLower(p), "f:"+p, "f:"+p)
	if r.Roles[p].Document {
		for _, pattern := range validation.DocumentPatterns(p) {
			put("doc_pattern", pattern, p, true)
		}
	} else {
		for _, pattern := range validation.Globs(f) {
			put("glob", validation.GlobPrefix(pattern), p+"\x00"+pattern, globReference{p, pattern})
		}
	}
	type lexeme struct {
		id, node string
		matches  []string
	}
	lexemes := make(map[string]lexeme)
	var shapes []signals.UnitStructure
	if sign > 0 {
		shapes = signals.FileStructures(f)
	}
	for i, u := range f.Units {
		counts := map[string]int{}
		for _, t := range u.Tokens {
			if t.Kind == "identifier" || t.Kind == "key" {
				counts[units.Literal(t)]++
			}
		}
		for word, n := range counts {
			if words[word] == nil {
				words[word] = map[int]int{}
			}
			words[word][n] += sign
		}
		// 旧索引的归属行已经逐条撤回，只需扣除词频和体积。
		if sign < 0 {
			continue
		}
		key := repostats.UnitKey(p, u)
		info.Units = append(info.Units, key)
		info.Tests[key] = r.TestUnits[key]
		info.Seeds[key] = r.TestSeeds[key]
		v := indexUnit{Path: p, Unit: u, Words: counts, Head: map[string]bool{}, Structure: shapes[i]}
		v.Unit.Tokens = nil
		for _, t := range u.Tokens {
			if t.End > u.HeaderEnd {
				break
			}
			v.Head[t.Text] = true
		}
		if u.Parent >= 0 {
			for _, t := range f.Units[u.Parent].Tokens {
				if t.End > f.Units[u.Parent].HeaderEnd {
					break
				}
				v.Head[t.Text] = true
			}
		}
		// 归属清单按内容共享，固定 map 的遍历顺序使相同文件重建后仍命中同一份正文。
		for _, word := range keys(v.Head) {
			put("head", word, key, true)
		}
		if u.Name != "" {
			put("name", u.Name, key, true)
			put("fold_name", strings.ToLower(u.Name), key, u.Name)
			if r.TestUnits[key] {
				put("case_name", u.Name, key, true)
			}
		}
		put("match", strings.ToLower(fmt.Sprintf("%s:%d", p, u.Line)), key, "u:"+key)
		if u.Name != "" {
			put("match", strings.ToLower(p+":"+u.Name), key, "u:"+key)
		}
		for _, w := range keys(v.Words) {
			put("word", w, key, v.Words[w])
			entry, ok := lexemes[w]
			if !ok {
				id := units.Hash(w)
				entry = lexeme{id, "lexeme:" + id, append([]string{strings.ToLower(w)}, units.Words(w)...)}
				lexemes[w] = entry
				x.put("lexeme", id, "", w)
			}
			member := key + ":" + entry.id
			for _, match := range entry.matches {
				put("match", match, member, entry.node)
			}
		}
		if v.Structure.Callable {
			var err error
			v.Grams, err = units.Grams(x.ctx, units.Sequence(u.Tokens, true))
			if err != nil {
				x.err = err
				return
			}
			v.Signature, err = units.MinHash(x.ctx, v.Grams)
			if err != nil {
				x.err = err
				return
			}
			for _, bucket := range signals.SimilarityBuckets(v.Signature) {
				put("lsh", bucket, key, true)
			}
		}
		put("unit", key, "", v)
	}
	if sign > 0 {
		calls := units.Calls(f.Tokens)
		for _, name := range keys(calls) {
			put("calls", name, p, calls[name])
		}
	}
	all := map[string]bool{}
	for w := range words {
		all[w] = true
	}
	for w := range uses {
		all[w] = true
	}
	for _, w := range keys(all) {
		x.statDelta(w, words[w], uses[w])
	}
	if sign > 0 {
		if err := saveCachedFile(x.ctx, x.s, info.Cache, f); err != nil {
			x.err = fmt.Errorf("保存文件分析 %s: %w", p, err)
			return
		}
		x.files[p] = f
		info.Content, info.Semantic = validation.FileDigests(f)
		x.put("file", p, "", info)
	} else {
		delete(x.files, p)
	}
}

// 初建时所有行都是新行，直接遍历分类目录；增量版本只发布写集（含删除标记）。
func (x *repositoryIndex) changedRows(yield func(ledger.IndexRow) bool) {
	if x.id != "" {
		for _, key := range keys(x.writes) {
			if !yield(x.writes[key]) {
				return
			}
		}
		return
	}
	for _, word := range keys(x.statCache) {
		if !yield(ledger.IndexRow{Kind: "stats", Key: word, Data: encode(x.statCache[word])}) {
			return
		}
	}
	for _, key := range keys(x.volumes) {
		if !yield(ledger.IndexRow{Kind: "volume", Key: key, Data: encode(x.volumes[key])}) {
			return
		}
	}
	for _, kind := range keys(x.rows) {
		for _, key := range keys(x.rows[kind]) {
			members := x.rows[kind][key]
			for _, member := range keys(members) {
				if !yield(ledger.IndexRow{Kind: kind, Key: key, Member: member, Data: members[member]}) {
					return
				}
			}
		}
	}
}

func (x *repositoryIndex) writeRows(build ledger.IndexBuild, rows iter.Seq[ledger.IndexRow]) error {
	var batch []ledger.IndexRow
	bytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := x.s.SaveIndexRows(x.ctx, build, batch); err != nil {
			return fmt.Errorf("保存索引批次 %s/%s: %w", batch[0].Kind, batch[0].Key, err)
		}
		clear(batch)
		batch, bytes = batch[:0], 0
		return nil
	}
	for row := range rows {
		// 已读取的父层成员可直接继承。只在发布时省略，约定和匹配器仍使用完整的本次改动。
		if x.base != nil {
			k := rowKey(row.Kind, row.Key)
			if x.base.loaded[k] && x.base.rows[row.Kind][row.Key][row.Member] == row.Data {
				continue
			}
		}
		size := len(row.Kind) + len(row.Key) + len(row.Member) + len(row.Data)
		if len(batch) == IndexBatch || bytes+size > IndexBatchBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch = append(batch, row)
		bytes += size
	}
	if x.err != nil {
		return x.err
	}
	return flush()
}

func testWordPathKind(indexID string) string { return "test_word_paths:" + indexID }

func (x *repositoryIndex) indexTestWords(build ledger.IndexBuild, sketch repostats.FileSketch) {
	kind := testWordPathKind(build.ID)
	for _, unit := range sketch.Units {
		if err := x.ctx.Err(); err != nil {
			x.err = err
			return
		}
		for word := range unit.Words {
			x.put(kind, word, sketch.Path, true)
		}
	}
}

func (x *repositoryIndex) publish(build ledger.IndexBuild, roles map[string]repostats.Role, sketches []repostats.FileSketch) error {
	// 初建时每个摘要已随源文件处理。派生索引按这次学习后的角色重新选取测试文件。
	for _, sketch := range sketches {
		if roles[sketch.Path].Test {
			x.indexTestWords(build, sketch)
		}
	}
	for p, role := range roles {
		if role.Test {
			x.put("test_files", build.ID, p, true)
		}
	}
	for key := range x.volumeDirty {
		x.put("volume", key, "", x.volumes[key])
	}
	if x.err != nil {
		return x.err
	}
	if err := x.writeRows(build, x.changedRows); err != nil {
		return err
	}
	x.meta.Generation = build.Generation
	// 随根记录发布完整性标记，即使同一构建有旧版进程参与，也不会读到只写了一批的路径。
	x.meta.TestFilesIndexed = true
	x.meta.TestWordPathsIndexed = true
	parent := ledger.IndexBuild{}
	if x.base != nil {
		parent = x.base.publishedBuild()
	}
	if err := x.s.PublishIndex(x.ctx, ledger.AnalysisIndex{ID: build.ID, ProjectID: x.w.ProjectID, Tree: x.meta.Tree, Variant: x.variant, ParentID: parent.ID, Data: json.RawMessage(encode(x.meta))}, build.Generation, parent.Generation); err != nil {
		return err
	}
	saved, found, err := x.s.FindIndex(x.ctx, x.w.ProjectID, x.meta.Tree, x.variant)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("起点索引未发布：%s", x.meta.Tree)
	}
	// 只使用实际发布的版本，同时释放构建时的词流、写集和父层缓存。
	next := newIndex(x.ctx, x.s, x.w)
	next.id, next.variant = saved.ID, saved.Variant
	if err := json.Unmarshal(saved.Data, &next.meta); err != nil {
		return err
	}
	*x = *next
	return nil
}

func (x *repositoryIndex) publishedBuild() ledger.IndexBuild {
	return ledger.IndexBuild{ID: x.id, Generation: x.meta.Generation}
}

func (x *repositoryIndex) readResult() error {
	if x.err != nil {
		return x.err
	}
	return x.s.CheckPublishedIndex(x.ctx, x.publishedBuild())
}

func newIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace) *repositoryIndex {
	return &repositoryIndex{ctx: ctx, s: s, w: w, variant: Algorithm, rows: map[string]map[string]map[string]string{}, loaded: map[string]bool{}, volumes: map[string]changes.VolumeNode{}, volumeDirty: map[string]bool{}, fileCache: map[string]indexFile{}, unitCache: map[string]indexUnit{}, statCache: map[string]wordStats{}, writes: map[string]ledger.IndexRow{}, files: map[string]units.File{}, pages: map[string][]units.MatcherState{}, preloaded: map[string]bool{}, meta: indexMeta{Matchers: map[string][]string{}}}
}

func openIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, tree string) (*repositoryIndex, error) {
	x := newIndex(ctx, s, w)
	r, found, release, err := findBuild(ctx, s, w, tree, Algorithm)
	if release != nil {
		defer release()
	}
	if err != nil {
		return x, err
	}
	if found {
		x.id = r.ID
		err = json.Unmarshal(r.Data, &x.meta)
		return x, err
	}
	// 建立起点索引的耗时超过 hook 的预算，由 MCP 后台和交付检查完成。
	if usesPublishedOnly(ctx) {
		return x, ErrIndexPending
	}
	previous, hasPrevious, err := s.FindIndex(ctx, w.ProjectID, "", Algorithm)
	if err != nil {
		return x, err
	}
	if hasPrevious {
		return buildNextIndex(ctx, s, w, previous, tree)
	}
	src, err := sources(ctx, w, tree)
	if err != nil {
		return x, err
	}
	raw, err := repostats.Learn(ctx, repostats.Input{Tree: tree, Sources: src})
	if err != nil {
		return x, err
	}
	x.meta.Tree = tree
	x.meta.Converged, x.meta.Iterations, x.meta.LogPrior = raw.Converged, raw.TestIterations, raw.Tests.LogPrior
	x.meta.TestExtensions = raw.Tests.Extensions
	for ext, v := range raw.Syntax {
		x.put("syntax", ext, "", v)
	}
	for w, v := range raw.Tests.Weights {
		x.put("test_model", w, "", v)
	}
	fold := map[string]float64{}
	for w, v := range raw.TestWords {
		x.put("test_word", w, "", v)
		fold[strings.ToLower(w)] = max(fold[strings.ToLower(w)], v)
	}
	for w, v := range fold {
		x.put("test_fold", w, "", v)
	}
	for _, e := range raw.Entries {
		x.put("entry", e.Kind+"\x00"+e.Value, "", e)
	}
	mean, variance, err := signals.SimilarityStatistics(ctx, raw)
	if err != nil {
		return x, err
	}
	x.meta.SimilarityMean, x.meta.SimilarityVariance = mean, variance
	build, err := s.BeginIndexBuild(ctx, units.Hash(Algorithm, w.ProjectID, tree))
	if err != nil {
		return x, err
	}
	paths := keys(raw.Files)
	for _, p := range paths {
		if raw.Roles[p].Document {
			for _, pattern := range validation.DocumentPatterns(p) {
				x.put("doc_pattern", pattern, p, true)
			}
		}
	}
	x.addMatcher("doc", keys(x.rows["doc_pattern"]))
	if err := x.writeRows(build, func(yield func(ledger.IndexRow) bool) {
		for _, source := range src {
			p := source.Path
			f := raw.Files[p]
			x.changeFile(p, f, raw, 1)
			if x.err != nil {
				return
			}
			sketch, err := repostats.Sketch(ctx, f, source.Data)
			if err != nil {
				x.err = err
				return
			}
			info := x.fileInfo(p)
			if info.Role.Test {
				x.indexTestWords(build, sketch)
			}
			sketch.Classified = info.Role
			sketch.Tests, sketch.Seeds = info.Tests, info.Seeds
			x.saveSketch(sketch)
			if !info.Role.Document {
				info.References = x.refs(f)
				x.put("file", p, "", info)
				x.adjustRefs(info, 1)
				if err := x.saveText(build, f); err != nil {
					x.err = err
					return
				}
				info = x.fileInfo(p)
			}
			// 文件归属和单元正文在此已经确定。批次写入未发布的代次；
			// 后续跨文件统计只需要单元首行、结构和文件摘要。
			if !yield(ledger.IndexRow{Kind: "file", Key: p, Data: encode(info)}) ||
				!yield(ledger.IndexRow{Kind: "learning", Key: p, Data: x.rows["learning"][p][""]}) {
				return
			}
			delete(x.rows["learning"], p)
			info.Rows = nil
			x.fileCache[p] = info
			unitKeys := slices.Clone(info.Units)
			slices.Sort(unitKeys)
			for _, key := range unitKeys {
				if !yield(ledger.IndexRow{Kind: "unit", Key: key, Data: x.rows["unit"][key][""]}) {
					return
				}
				delete(x.rows["unit"], key)
				u := x.unitCache[key]
				u.Words, u.Grams, u.Signature = nil, nil, nil
				x.unitCache[key] = u
			}
			delete(raw.Files, p)
			delete(x.files, p)
		}
	}); err != nil {
		return x, err
	}
	members := make(map[string][]string, len(paths))
	for _, p := range paths {
		x.summaryFile(p, 1)
		members[p] = x.fileCache[p].Units
	}
	x.meta.VariationCounts = map[string][2]int{}
	x.buildVolume()
	if err := x.saveFacts(members); err != nil {
		return x, err
	}
	x.updatePrior()
	x.addMatcher("case", keys(x.rows["case_name"]))
	x.addMatcher("graph", keys(x.rows["match"]))
	if err := x.publish(build, raw.Roles, nil); err != nil {
		return x, err
	}
	return x, x.err
}

func (x *repositoryIndex) buildMatchers() {
	patterns := map[string][]string{}
	kinds := map[string]string{"match": "graph", "case_name": "case", "doc_pattern": "doc"}
	for _, r := range x.writes {
		if r.Data == "" {
			continue
		}
		if matcher, ok := kinds[r.Kind]; ok {
			patterns[matcher] = append(patterns[matcher], r.Key)
		}
	}
	for _, kind := range keys(patterns) {
		x.addMatcher(kind, patterns[kind])
	}
}
