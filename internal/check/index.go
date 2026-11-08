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
	"path"
	"strconv"
	"strings"
)

type indexMeta struct {
	Tree                               string
	Units                              int
	Converged                          bool
	Iterations                         int
	LogPrior                           float64
	TestExtensions                     map[string]bool
	Matchers                           map[string][]string
	SimilarityMean, SimilarityVariance float64
	Indirection                        map[string]float64
	VariationCounts                    map[string][2]int
	Validation                         validation.Summary
}
type indexFile struct {
	Blob, Mode, Cache string
	Role              repostats.Role
	Units             []string
	Tests, Seeds      map[string]bool
	Rows              []ledger.IndexRow
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
	rows        map[string]map[string]string
	loaded      map[string]bool
	rowKinds    map[string]map[string]bool
	volumes     map[string]changes.VolumeNode
	volumeDirty map[string]bool
	fileCache   map[string]indexFile
	unitCache   map[string]indexUnit
	statCache   map[string]wordStats
	writes      map[string]ledger.IndexRow
	files       map[string]units.File
	err         error
}

func rowKey(kind, key string) string { return kind + "\x00" + key }
func encode(v any) string            { b, _ := json.Marshal(v); return string(b) }
func (x *repositoryIndex) read(kind, key string) map[string]string {
	k := rowKey(kind, key)
	if x.loaded[k] {
		return x.rows[k]
	}
	r := map[string]string{}
	if x.base != nil && x.err == nil {
		for member, raw := range x.base.read(kind, key) {
			r[member] = raw
		}
		x.err = x.base.err
	} else if x.id != "" && x.err == nil {
		rows, err := x.s.IndexRows(x.ctx, x.id, kind, key)
		if err != nil {
			x.err = err
		}
		for _, v := range rows {
			r[v.Member] = v.Data
		}
	}
	for member, raw := range x.rows[k] {
		if raw == "" {
			delete(r, member)
		} else {
			r[member] = raw
		}
	}
	x.rows[k] = r
	x.loaded[k] = true
	if x.rowKinds[kind] == nil {
		x.rowKinds[kind] = map[string]bool{}
	}
	x.rowKinds[kind][key] = true
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
	raw := ""
	if v != nil {
		raw = encode(v)
	}
	k := rowKey(kind, key)
	r := x.rows[k]
	if x.id == "" {
		x.loaded[k] = true
	}
	if r == nil {
		r = map[string]string{}
		x.rows[k] = r
	}
	if old, exists := r[member]; old == raw && (exists || x.loaded[k] || x.id == "") {
		return
	}
	if raw == "" && (x.loaded[k] || x.id == "") {
		delete(r, member)
	} else {
		r[member] = raw
	}
	if x.rowKinds[kind] == nil {
		x.rowKinds[kind] = map[string]bool{}
	}
	x.rowKinds[kind][key] = true
	x.writes[rowKey(kind, key)+"\x00"+member] = ledger.IndexRow{Kind: kind, Key: key, Member: member, Data: raw}
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
	if _, local := x.rows[rowKey("unit", key)]; !local && x.base != nil && x.err == nil {
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
	x.volumeDirty[key] = true
}
func (x *repositoryIndex) statDelta(word string, counts map[int]int, uses int) {
	old := x.stats(word)
	if len(counts) == 0 {
		old.Uses += uses
		x.put("stats", word, "", old)
		return
	}
	for c, n := range old.Counts {
		changes.AdjustVolume(old.DF, c, -n, x.volumeNode, x.setVolume)
	}
	for c, n := range counts {
		old.Counts[c] += n
		old.DF += n
		if old.Counts[c] == 0 {
			delete(old.Counts, c)
		}
	}
	old.Uses += uses
	for c, n := range old.Counts {
		changes.AdjustVolume(old.DF, c, n, x.volumeNode, x.setVolume)
	}
	x.put("stats", word, "", old)
}
func (x *repositoryIndex) load(p string) units.File {
	if f, ok := x.files[p]; ok {
		return f
	}
	info := x.fileInfo(p)
	if info.Blob == "" {
		return units.File{}
	}
	var f units.File
	found, err := x.s.AnalysisCache(x.ctx, "file", info.Cache, &f)
	if err != nil {
		x.err = err
	} else if !found {
		x.err = fmt.Errorf("索引文件分析缺失：%s", p)
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

func (x *repositoryIndex) matcher(kind, text string) ([]units.Match, error) {
	var out []units.Match
	for _, root := range x.meta.Matchers[kind] {
		pages := map[int][]units.MatcherState{}
		matches, err := units.FindStates(x.ctx, text, func(i int) (units.MatcherState, error) {
			page, offset := i/MatcherPageSize, i%MatcherPageSize
			states, ok := pages[page]
			if !ok {
				x.get("matcher", root+":"+strconv.Itoa(page), &states)
				pages[page] = states
			}
			if offset >= len(states) {
				return units.MatcherState{}, fmt.Errorf("自动机索引页缺失：%s/%d", root, page)
			}
			return states[offset], x.err
		})
		if err != nil {
			return nil, err
		}
		out = append(out, matches...)
	}
	return out, x.err
}

func (x *repositoryIndex) namesIn(text string) []string {
	set := map[string]bool{}
	matches, err := x.matcher("case", text)
	if err != nil {
		x.err = err
	}
	for _, m := range matches {
		if units.WholeWord(text, m) && len(x.read("case_name", m.Pattern)) > 0 {
			set[m.Pattern] = true
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
		ms, err := x.matcher("graph", text)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			for _, raw := range x.read("match", m.Pattern) {
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
				out[m.Pattern] = append(out[m.Pattern], node)
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
	owned := map[string]bool{}
	put := func(kind, key, member string, v any) {
		if sign < 0 {
			return
		}
		row := rowKey(kind, key) + "\x00" + member
		if owned[row] {
			return
		}
		owned[row] = true
		x.put(kind, key, member, v)
		info.Rows = append(info.Rows, ledger.IndexRow{Kind: kind, Key: key, Member: member})
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
	for _, u := range f.Units {
		key := repostats.UnitKey(p, u)
		info.Units = append(info.Units, key)
		info.Tests[key] = r.TestUnits[key]
		info.Seeds[key] = r.TestSeeds[key]
		v := indexUnit{Path: p, Unit: u, Words: map[string]int{}, Head: map[string]bool{}, Structure: signals.Structure(f, u)}
		v.Unit.Tokens = nil
		for _, t := range u.Tokens {
			if t.Kind == "identifier" || t.Kind == "key" {
				v.Words[units.Literal(t)]++
			}
			if t.End <= u.HeaderEnd {
				v.Head[t.Text] = true
			}
		}
		if u.Parent >= 0 {
			for _, t := range f.Units[u.Parent].Tokens {
				if t.End <= f.Units[u.Parent].HeaderEnd {
					v.Head[t.Text] = true
				}
			}
		}
		for word := range v.Head {
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
		for w, n := range v.Words {
			put("word", w, key, n)
			if words[w] == nil {
				words[w] = map[int]int{}
			}
			words[w][n] += sign
			wordID := units.Hash(w)
			if sign > 0 {
				x.put("lexeme", wordID, "", w)
			}
			for _, match := range append([]string{strings.ToLower(w)}, units.Words(w)...) {
				put("match", match, key+":"+wordID, "lexeme:"+wordID)
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
	for name, calls := range units.Calls(f.Tokens) {
		put("calls", name, p, calls)
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
		if err := x.s.SaveAnalysisCache(x.ctx, "file", info.Cache, f); err != nil {
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

func (x *repositoryIndex) publish(id, parent string) error {
	for key := range x.volumeDirty {
		x.put("volume", key, "", x.volumes[key])
	}
	if x.err != nil {
		return x.err
	}
	var batch []ledger.IndexRow
	bytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := x.s.SaveIndexRows(x.ctx, id, batch); err != nil {
			return fmt.Errorf("保存索引批次 %s/%s: %w", batch[0].Kind, batch[0].Key, err)
		}
		batch, bytes = nil, 0
		return nil
	}
	for _, key := range keys(x.writes) {
		row := x.writes[key]
		// 已读取的父层成员可直接继承。只在发布时省略，约定和匹配器仍使用完整的本次改动。
		if x.base != nil {
			k := rowKey(row.Kind, row.Key)
			if x.base.loaded[k] && x.base.rows[k][row.Member] == row.Data {
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
	if err := flush(); err != nil {
		return err
	}
	if err := x.s.PublishIndex(x.ctx, ledger.AnalysisIndex{ID: id, ProjectID: x.w.ProjectID, Tree: x.meta.Tree, Variant: x.variant, ParentID: parent, Data: json.RawMessage(encode(x.meta))}); err != nil {
		return err
	}
	saved, found, err := x.s.FindIndex(x.ctx, x.w.ProjectID, x.meta.Tree, x.variant)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("起点索引未发布：%s", x.meta.Tree)
	}
	if saved.ID != id {
		// 并行构建可能从不同父版本出发，只采用最先发布的完整版本。
		next := newIndex(x.ctx, x.s, x.w)
		next.id, next.variant = saved.ID, x.variant
		if err := json.Unmarshal(saved.Data, &next.meta); err != nil {
			return err
		}
		*x = *next
	} else {
		x.id = id
	}
	return nil
}

func newIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace) *repositoryIndex {
	return &repositoryIndex{ctx: ctx, s: s, w: w, variant: Algorithm, rows: map[string]map[string]string{}, loaded: map[string]bool{}, rowKinds: map[string]map[string]bool{}, volumes: map[string]changes.VolumeNode{}, volumeDirty: map[string]bool{}, fileCache: map[string]indexFile{}, unitCache: map[string]indexUnit{}, statCache: map[string]wordStats{}, writes: map[string]ledger.IndexRow{}, files: map[string]units.File{}, meta: indexMeta{Matchers: map[string][]string{}}}
}

func openIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, tree string) (*repositoryIndex, error) {
	x := newIndex(ctx, s, w)
	r, found, err := s.FindIndex(ctx, w.ProjectID, tree, Algorithm)
	if err != nil {
		return x, err
	}
	if found {
		x.id = r.ID
		err = json.Unmarshal(r.Data, &x.meta)
		return x, err
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
	for _, p := range keys(raw.Files) {
		x.changeFile(p, raw.Files[p], raw, 1)
		if x.err != nil {
			return x, x.err
		}
	}
	for _, source := range src {
		sketch, err := repostats.Sketch(ctx, raw.Files[source.Path], source.Data)
		if err != nil {
			return x, err
		}
		sketch.Classified = raw.Roles[source.Path]
		info := x.fileInfo(source.Path)
		sketch.Tests, sketch.Seeds = info.Tests, info.Seeds
		x.saveSketch(sketch)
	}
	mean, variance, err := signals.SimilarityStatistics(ctx, raw)
	if err != nil {
		return x, err
	}
	x.meta.SimilarityMean, x.meta.SimilarityVariance = mean, variance
	x.meta.VariationCounts = map[string][2]int{}
	if err := x.saveFacts(keys(raw.Files)); err != nil {
		return x, err
	}
	x.updatePrior()
	x.buildMatchers()
	if err := x.buildValidation(raw); err != nil {
		return x, err
	}
	id := units.Hash(Algorithm, w.ProjectID, tree)
	if err := x.publish(id, ""); err != nil {
		return x, err
	}
	x.writes = map[string]ledger.IndexRow{}
	return x, x.err
}

func (x *repositoryIndex) buildMatchers() {
	patterns := map[string][]string{}
	for _, r := range x.writes {
		if r.Data == "" {
			continue
		}
		switch r.Kind {
		case "match":
			patterns["graph"] = append(patterns["graph"], r.Key)
		case "case_name":
			patterns["case"] = append(patterns["case"], r.Key)
		case "doc_pattern":
			patterns["doc"] = append(patterns["doc"], r.Key)
		}
	}
	for _, kind := range keys(patterns) {
		x.addMatcher(kind, patterns[kind])
	}
}
