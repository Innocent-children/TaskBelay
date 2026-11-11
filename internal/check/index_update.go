package check

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/platform"
	"github.com/Innocent-children/taskbelay/internal/repostats"
	"github.com/Innocent-children/taskbelay/internal/units"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
)

func (x *repositoryIndex) saveSketch(sketch repostats.FileSketch) {
	sketch.Units = append([]repostats.UnitSketch(nil), sketch.Units...)
	for i, u := range sketch.Units {
		sketch.Units[i].Callable = x.unit(repostats.UnitKey(sketch.Path, u.Unit)).Structure.Callable
	}
	x.put("learning", sketch.Path, "", sketch)
}

func (x *repositoryIndex) sketches() []repostats.FileSketch {
	rows := x.prefix("learning", "")
	var out []repostats.FileSketch
	for _, p := range keys(rows) {
		raw, ok := rows[p][""]
		if !ok {
			continue
		}
		var s repostats.FileSketch
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			x.err = err
		}
		out = append(out, s)
	}
	return out
}

func (x *repositoryIndex) replaceValues(kind string, values map[string]string) {
	old := x.prefix(kind, "")
	for _, k := range keys(old) {
		if _, ok := values[k]; !ok {
			x.put(kind, k, "", nil)
		}
	}
	for _, k := range keys(values) {
		var v any
		if err := json.Unmarshal([]byte(values[k]), &v); err != nil {
			x.err = err
			return
		}
		x.put(kind, k, "", v)
	}
}

func (x *repositoryIndex) conventions(sketches []repostats.FileSketch, r repostats.Repository) {
	defer func() { x.files = map[string]units.File{} }()
	x.meta.Converged, x.meta.Iterations, x.meta.LogPrior = r.Converged, r.TestIterations, r.Tests.LogPrior
	x.meta.TestExtensions = r.Tests.Extensions
	syntax := map[string]string{}
	for k, v := range r.Syntax {
		syntax[k] = encode(v)
	}
	x.replaceValues("syntax", syntax)
	model, words, fold, entries := map[string]string{}, map[string]string{}, map[string]string{}, map[string]string{}
	for k, v := range r.Tests.Weights {
		model[k] = encode(v)
	}
	lower := map[string]float64{}
	for k, v := range r.TestWords {
		words[k] = encode(v)
		lower[strings.ToLower(k)] = max(lower[strings.ToLower(k)], v)
	}
	for k, v := range lower {
		fold[k] = encode(v)
	}
	for _, e := range r.Entries {
		entries[e.Kind+"\x00"+e.Value] = encode(e)
	}
	x.replaceValues("test_model", model)
	x.replaceValues("test_word", words)
	x.replaceValues("test_fold", fold)
	x.replaceValues("entry", entries)
	for _, sketch := range sketches {
		p := sketch.Path
		next := r.Files[p]
		tests, seeds := map[string]bool{}, map[string]bool{}
		namesChanged := false
		for i, u := range next.Units {
			key := repostats.UnitKey(p, u)
			tests[key], seeds[key] = r.TestUnits[key], r.TestSeeds[key]
			namesChanged = namesChanged || u.Name != sketch.Units[i].Unit.Name
		}
		if !namesChanged && sketch.Classified == r.Roles[p] && reflect.DeepEqual(sketch.Tests, tests) && reflect.DeepEqual(sketch.Seeds, seeds) {
			continue
		}
		info := x.fileInfo(p)
		rows := map[string]indexMember{}
		for _, row := range info.Rows {
			rows[rowKey(row.Kind, row.Key)+"\x00"+row.Member] = row
		}
		update := func(kind, key, member string, v any) {
			x.put(kind, key, member, v)
			k := rowKey(kind, key) + "\x00" + member
			if v == nil {
				delete(rows, k)
			} else {
				rows[k] = indexMember{kind, key, member}
			}
		}
		for i, u := range next.Units {
			key := repostats.UnitKey(p, u)
			old := sketch.Units[i].Unit.Name
			v := x.unit(key)
			members := []string(nil)
			for j, child := range next.Units {
				if child.Parent >= 0 && next.Units[child.Parent].ID == u.ID && j != u.Parent && child.Name != "" {
					members = append(members, child.Name)
				}
			}
			if v.Unit.Name != u.Name || !reflect.DeepEqual(v.Structure.Members, members) {
				v.Unit.Name, v.Structure.Members = u.Name, members
				x.put("unit", key, "", v)
			}
			if old != u.Name {
				if old != "" {
					update("name", old, key, nil)
					update("fold_name", strings.ToLower(old), key, nil)
					update("match", strings.ToLower(p+":"+old), key, nil)
				}
				if u.Name != "" {
					update("name", u.Name, key, true)
					update("fold_name", strings.ToLower(u.Name), key, u.Name)
					update("match", strings.ToLower(p+":"+u.Name), key, "u:"+key)
				}
			}
			if old != "" && sketch.Tests[key] && (old != u.Name || !tests[key]) {
				update("case_name", old, key, nil)
			}
			if u.Name != "" && tests[key] {
				update("case_name", u.Name, key, true)
			}
			sketch.Units[i].Unit.Name = u.Name
		}
		if info.Role.Test != r.Roles[p].Test {
			sign := 1
			if r.Roles[p].Test {
				sign = -1
			}
			for w, n := range sketch.Uses {
				x.statDelta(w, nil, sign*n)
			}
		}
		info.Role, info.Tests, info.Seeds = r.Roles[p], tests, seeds
		info.Rows = nil
		for _, k := range keys(rows) {
			info.Rows = append(info.Rows, rows[k])
		}
		x.put("file", p, "", info)
		sketch.Classified, sketch.Tests, sketch.Seeds = info.Role, tests, seeds
		x.saveSketch(sketch)
		delete(x.files, p)
	}
}

// ErrIndexPending 表示所需索引尚未发布，而当前入口不负责建立它。
var ErrIndexPending = errors.New("分析索引尚未就绪")

type publishedOnlyKey struct{}

type backgroundIndexKey struct{}

// 仅后台预热合并同一账本、项目、起点和变体的重建。前台检查保留自己的截止时间；
// 锁不可用时仍可依靠账本的代次核对和原子发布。
func findBuild(ctx context.Context, s *ledger.Store, w gitstate.Workspace, tree, variant string) (ledger.AnalysisIndex, bool, func(), error) {
	saved, found, err := s.FindIndex(ctx, w.ProjectID, tree, variant)
	if found || err != nil || ctx.Value(backgroundIndexKey{}) != true {
		return saved, found, nil, err
	}
	dir := filepath.Join(s.DataDir(), "build-locks")
	if os.MkdirAll(dir, 0700) != nil {
		return saved, false, nil, nil
	}
	name := filepath.Join(dir, units.Hash(w.ProjectID, tree, variant))
	for {
		release, err := platform.TryBuildLock(name)
		if err != nil {
			return saved, false, nil, ctx.Err()
		}
		if release != nil {
			// 等待期间，持有者可能已发布，也可能中途退出。重新查账本决定是否接手。
			saved, found, err = s.FindIndex(ctx, w.ProjectID, tree, variant)
			return saved, found, release, err
		}
		timer := time.NewTimer(BackgroundPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return saved, false, nil, ctx.Err()
		case <-timer.C:
			saved, found, err = s.FindIndex(ctx, w.ProjectID, tree, variant)
			if found || err != nil {
				return saved, found, nil, err
			}
		}
	}
}

// publishedOnly 用于 hook：只读取已发布的索引。建立起点索引、扫描新的运行输出和
// 重新学习测试内容的耗时超过 hook 预算，由 MCP 后台和交付检查完成。
func publishedOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, publishedOnlyKey{}, true)
}

func usesPublishedOnly(ctx context.Context) bool {
	only, _ := ctx.Value(publishedOnlyKey{}).(bool)
	return only
}

/**
 * publishedIndex 返回某个起点上最近发布的索引，不读取运行记录，也不建立新版本。
 * 起点索引还没有任何带测试识别结果的版本时，使用起点索引本身。
 */
func publishedIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, origin string) (*repositoryIndex, error) {
	base, err := openIndex(publishedOnly(ctx), s, w, origin)
	if err != nil {
		return nil, err
	}
	saved, found, err := s.LatestDerivedIndex(base.ctx, w.ProjectID, base.id)
	if err != nil || !found {
		return base, err
	}
	x := newIndex(base.ctx, s, w)
	x.id, x.variant = saved.ID, saved.Variant
	return x, json.Unmarshal(saved.Data, &x.meta)
}

func taskIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, task ledger.TaskRecord) (*repositoryIndex, error) {
	if usesPublishedOnly(ctx) {
		return publishedIndex(ctx, s, w, task.OriginTree)
	}
	base, err := openIndex(ctx, s, w, task.OriginTree)
	if err != nil {
		return nil, err
	}
	records, err := s.IndexVerifications(ctx, w.ID, task.ID, Algorithm)
	if err != nil {
		return nil, err
	}
	return observedIndex(base, runSamples(records))
}

/**
 * observedNames 返回运行输出中出现过的单元名。每条输出只扫描一次，结果按起点索引
 * 和输出内容保存；整段历史的并集按运行编号保存，新增一条运行时只需处理这一条。
 * hook 中只查询整段历史的已有结果，known 为 false 表示还有运行记录没有并入。
 */
func observedNames(base *repositoryIndex, runs []repostats.Run) (names []string, known bool, err error) {
	var eligible []repostats.Run
	for _, run := range runs {
		if run.Started && run.ExitCode != nil {
			eligible = append(eligible, run)
		}
	}
	if len(eligible) == 0 {
		return []string{}, true, nil
	}
	history := func(n int) string {
		parts := []string{Algorithm, base.id}
		for _, run := range eligible[:n] {
			parts = append(parts, run.ID, units.Hash(run.Output))
		}
		return units.Hash(parts...)
	}
	found, err := base.s.AnalysisCache(base.ctx, "observed", history(len(eligible)), &names)
	if err != nil || found {
		return names, found, err
	}
	done := 0
	if len(eligible) > 1 {
		found, err = base.s.AnalysisCache(base.ctx, "observed", history(len(eligible)-1), &names)
		if err != nil {
			return nil, false, err
		}
		if found {
			done = len(eligible) - 1
		}
	}
	seeds := map[string]bool{}
	for _, name := range names {
		seeds[name] = true
	}
	for _, run := range eligible[done:] {
		var matched []string
		key := units.Hash(Algorithm, base.id, units.Hash(run.Output))
		found, err := base.s.AnalysisCache(base.ctx, "run-names", key, &matched)
		if err != nil {
			return nil, false, err
		}
		if !found {
			patterns, err := base.matchPatterns("graph", strings.ToLower(run.Output), false)
			if err != nil {
				return nil, false, err
			}
			// 同一个名字在一条输出里会命中很多次，整词核对只做一次。
			own := map[string]bool{}
			for pattern := range patterns {
				for _, raw := range base.read("fold_name", pattern) {
					var name string
					if err := json.Unmarshal([]byte(raw), &name); err != nil {
						return nil, false, err
					}
					if _, checked := own[name]; !checked {
						own[name] = units.HasWord(run.Output, name)
					}
				}
			}
			if base.err != nil {
				return nil, false, base.err
			}
			var present []string
			for _, name := range keys(own) {
				if own[name] {
					present = append(present, name)
				}
			}
			if matched, err = executed(base, run.Output, present); err != nil {
				return nil, false, err
			}
			if err := base.readResult(); err != nil {
				return nil, false, err
			}
			if err := base.s.SaveAnalysisCache(base.ctx, "run-names", key, matched); err != nil {
				return nil, false, err
			}
		}
		for _, name := range matched {
			seeds[name] = true
		}
	}
	names = keys(seeds)
	return names, true, base.s.SaveAnalysisCache(base.ctx, "observed", history(len(eligible)), names)
}

// EvidenceContext 是判断输出片段是否照抄源码时，名字两侧各取的字符数。
const EvidenceContext = 16

// 带源码位置的诊断正文可能逐项打印任意名字，不能据此确认这些用例运行过。
var diagnosticLocation = regexp.MustCompile(`^[[:space:]]*[^[:space:]]+:[0-9]+:([[:space:]]|$)`)

/**
 * executed 从输出里出现的名字中，留下能说明代码确实被执行过的那些。
 * 查看文件、搜索文本的命令也会输出名字，但名字连同旁边的文字是从源码照抄的；
 * 测试运行器输出的是名字加上自己的状态文字。名字每次出现时，取同一行内它左侧和右侧
 * 的相邻文字各组成一段；任何一段能在起点内容中原样找到，这次出现就不算运行证据。
 * 这样不依赖命令写法，经容器或脚本转发的读文件命令也不会被当成测试运行。
 */
func executed(base *repositoryIndex, output string, names []string) ([]string, error) {
	type occurrence struct {
		contexts []string
		shape    string
	}
	byName := map[string][]occurrence{}
	var patterns []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		if diagnosticLocation.MatchString(line) {
			continue
		}
		for _, name := range names {
			if len(byName[name]) >= 3 || !strings.Contains(line, name) {
				continue
			}
			at := reportNameOffset(line, name)
			if at < 0 {
				continue
			}
			left, right := []rune(line[:at]), []rune(line[at+len(name):])
			if len(left) > EvidenceContext {
				left = left[len(left)-EvidenceContext:]
			}
			if len(right) > EvidenceContext {
				right = right[:EvidenceContext]
			}
			o := occurrence{shape: reportShape(line, at, len(name))}
			for _, context := range []string{string(left) + name, name + string(right)} {
				// 相邻文字太短时无法区分照抄和运行输出，这一段不参与判断。
				if len([]rune(strings.TrimSpace(strings.Replace(context, name, "", 1)))) >= 3 {
					o.contexts = append(o.contexts, context)
					patterns = append(patterns, context)
				}
			}
			byName[name] = append(byName[name], o)
		}
	}
	shown, err := base.w.TreeContains(base.ctx, base.meta.Tree, patterns)
	if err != nil {
		return nil, err
	}
	// 运行器用同一种格式逐条报告用例；日志或报错里偶然提到的函数名没有这种成组的格式。
	shapes := map[string]map[string]bool{}
	for _, name := range names {
		for _, o := range byName[name] {
			copied := len(o.contexts) == 0
			for _, context := range o.contexts {
				copied = copied || shown[context]
			}
			if !copied {
				if shapes[o.shape] == nil {
					shapes[o.shape] = map[string]bool{}
				}
				shapes[o.shape][name] = true
			}
		}
	}
	reported := map[string]bool{}
	for _, group := range shapes {
		if len(group) > 1 {
			for name := range group {
				reported[name] = true
			}
		}
	}
	return keys(reported), nil
}

/**
 * reportNameOffset 要求名字之外另有报告文字，选项拼写、单列文件名和纯计数不足以提供运行证据。
 */
func reportNameOffset(line, name string) int {
	at, offset := -1, 0
	hasDetail := false
	for field := range strings.FieldsSeq(line) {
		start := offset + strings.Index(line[offset:], field)
		offset = start + len(field)
		if strings.HasPrefix(field, "-") {
			continue
		}
		if i := units.WordIndex(field, name); i >= 0 {
			if at < 0 {
				at = start + i
			}
		} else if strings.ContainsFunc(field, func(r rune) bool { return unicode.IsLetter(r) || unicode.Is(unicode.So, r) }) {
			hasDetail = true
		}
		if at >= 0 && hasDetail {
			return at
		}
	}
	return -1
}

// reportShape 把一行输出里的名字换成占位符、数字换成 0，得到这一行的报告格式。
func reportShape(line string, at, size int) string {
	var b strings.Builder
	for i, r := range line {
		switch {
		case i == at:
			b.WriteByte(0)
		case i > at && i < at+size:
		case r >= '0' && r <= '9':
			b.WriteByte('0')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func observedIndex(base *repositoryIndex, runs []repostats.Run) (*repositoryIndex, error) {
	if len(runs) == 0 {
		return base, base.err
	}
	names, known, err := observedNames(base, runs)
	if err != nil {
		return base, err
	}
	if base.err != nil {
		return base, base.err
	}
	open := func(saved ledger.AnalysisIndex) (*repositoryIndex, error) {
		x := newIndex(base.ctx, base.s, base.w)
		x.id, x.variant = saved.ID, saved.Variant
		return x, json.Unmarshal(saved.Data, &x.meta)
	}
	variant := Algorithm + ":seeds:" + units.Hash(encode(names))
	if known {
		saved, found, release, err := findBuild(base.ctx, base.s, base.w, base.meta.Tree, variant)
		if release != nil {
			defer release()
		}
		if err != nil {
			return base, err
		}
		if found {
			return open(saved)
		}
	}
	x := base.overlay(base.meta.Tree)
	x.variant = variant
	sketches := x.sketches()
	if x.err != nil {
		return x, x.err
	}
	zero := 0
	r, err := repostats.Relearn(x.ctx, x.meta.Tree, sketches, []repostats.Run{{Output: strings.Join(names, "\n"), Started: true, ExitCode: &zero}})
	if err != nil {
		return x, err
	}
	x.conventions(sketches, r)
	x.buildMatchers()
	build, err := x.s.BeginIndexBuild(x.ctx, units.Hash(variant, x.w.ProjectID, x.meta.Tree))
	if err != nil {
		return x, err
	}
	if err = x.publish(build); err != nil {
		return x, err
	}
	return x, x.err
}

func buildNextIndex(ctx context.Context, s *ledger.Store, w gitstate.Workspace, parent ledger.AnalysisIndex, tree string) (*repositoryIndex, error) {
	base := newIndex(ctx, s, w)
	base.id = parent.ID
	if err := json.Unmarshal(parent.Data, &base.meta); err != nil {
		return base, err
	}
	x := base.overlay(tree)
	diff, err := w.Diff(ctx, parent.Tree, tree)
	if err != nil {
		return x, err
	}
	changed := map[string]bool{}
	entries := map[string]gitstate.File{}
	for _, d := range diff {
		changed[d.BeforePath] = true
		changed[d.Path] = true
		entries[d.BeforePath] = gitstate.File{}
		entries[d.Path] = d.After
	}
	var ids []string
	for _, e := range entries {
		if e.Blob != "" && e.Mode != "160000" {
			ids = append(ids, e.Blob)
		}
	}
	blobs, err := w.ReadBlobs(ctx, ids)
	if err != nil {
		return x, err
	}
	for _, p := range keys(changed) {
		if old := base.load(p); old.Blob != "" {
			x.changeFile(p, old, base.view([]string{p}), -1)
		}
		x.put("learning", p, "", nil)
		e := entries[p]
		if e.Blob == "" {
			continue
		}
		var syntax units.Syntax
		base.get("syntax", path.Ext(p), &syntax)
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: e.Blob, Mode: e.Mode, Data: blobs[e.Blob]}, syntax)
		if err != nil {
			return x, err
		}
		r, err := base.apply([]string{p}, map[string]units.File{p: f}, tree)
		if err != nil {
			return x, err
		}
		x.changeFile(p, f, r, 1)
		sketch, err := repostats.Sketch(ctx, f, blobs[e.Blob])
		if err != nil {
			return x, err
		}
		sketch.Classified = r.Roles[p]
		info := x.fileInfo(p)
		sketch.Tests, sketch.Seeds = info.Tests, info.Seeds
		x.saveSketch(sketch)
	}
	sketches := x.sketches()
	if x.err != nil {
		return x, x.err
	}
	learned, err := repostats.Relearn(ctx, tree, sketches, nil)
	if err != nil {
		return x, err
	}
	invalid := map[string]bool{}
	for ext, syntax := range learned.Syntax {
		var old units.Syntax
		base.get("syntax", ext, &old)
		if !reflect.DeepEqual(old.Quotes, syntax.Quotes) || !reflect.DeepEqual(old.Comments, syntax.Comments) {
			invalid[ext] = true
		}
	}
	for _, sketch := range sketches {
		p := sketch.Path
		if !invalid[path.Ext(p)] {
			continue
		}
		old := x.load(p)
		view := x.view([]string{p})
		data, err := w.ReadBlob(ctx, sketch.Blob)
		if err != nil {
			return x, err
		}
		f, err := units.Analyze(ctx, units.Source{Path: p, Blob: sketch.Blob, Mode: sketch.Mode, Data: data}, learned.SyntaxFor(p))
		if err != nil {
			return x, err
		}
		r, err := repostats.Apply(ctx, learned, tree, map[string]units.File{p: f})
		if err != nil {
			return x, err
		}
		x.changeFile(p, old, view, -1)
		x.changeFile(p, f, r, 1)
		next, err := repostats.Sketch(ctx, f, data)
		if err != nil {
			return x, err
		}
		next.Classified = r.Roles[p]
		info := x.fileInfo(p)
		next.Tests, next.Seeds = info.Tests, info.Seeds
		x.saveSketch(next)
		changed[p] = true
	}
	if len(invalid) > 0 {
		sketches = x.sketches()
		learned, err = repostats.Relearn(ctx, tree, sketches, nil)
		if err != nil {
			return x, err
		}
	}
	x.conventions(sketches, learned)
	x.buildMatchers()
	if err = x.updateValidation(base, keys(changed)); err != nil {
		return x, err
	}
	build, err := s.BeginIndexBuild(ctx, units.Hash(Algorithm, w.ProjectID, tree, base.id))
	if err != nil {
		return x, err
	}
	for _, p := range keys(changed) {
		if info := x.fileInfo(p); info.Blob != "" && !info.Role.Document {
			if err = x.saveText(build, x.load(p)); err != nil {
				return x, err
			}
		}
	}
	if err = x.refreshStatistics(base, changed, sketches); err != nil {
		return x, err
	}
	if err = x.publish(build); err != nil {
		return x, err
	}
	return x, x.err
}
