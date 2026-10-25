package codescan

import (
	"context"
	"sort"
)

type Change struct {
	Before            *Declaration
	After             *Declaration
	Kind              string
	Detail            string
	AmbiguousLocation bool
}

type Relocation struct {
	From string
	To   string
}

/**
 * 显式核对过的编辑对应优先；剩余声明仅在起点和当前快照中结构都唯一时
 * 自动迁移。相同模板的多个声明保持新增、删除，交由 Agent 补充位置。
 */
func Compare(ctx context.Context, before, after Snapshot, edits []Relocation) ([]Change, error) {
	old, current := before.Declarations(), after.Declarations()
	oldLocations, newLocations := locationCounts(old), locationCounts(current)
	for id, item := range old {
		if file := after.Files[item.File]; file != nil && !file.CodeComplete {
			delete(old, id)
		}
	}
	for id, item := range current {
		if file := before.Files[item.File]; file != nil && !file.CodeComplete {
			delete(current, id)
		}
	}
	usedOld, usedNew := map[string]bool{}, map[string]bool{}
	var changes []Change
	appendPair := func(a, b Declaration) {
		usedOld[a.ID], usedNew[b.ID] = true, true
		kind := "modify"
		if a.File != b.File {
			kind = "move"
		} else if a.Name != b.Name || a.Owner != b.Owner {
			kind = "rename"
		}
		if a.Text == b.Text && kind == "modify" {
			return
		}
		detail := "behavior"
		switch {
		case a.Kind != b.Kind || a.NodeType != b.NodeType:
			detail = "behavior"
		case a.FullHash == b.FullHash:
			detail = "format"
		case a.CodeHash == b.CodeHash:
			detail = "comment"
		case a.ShapeHash == b.ShapeHash:
			detail = "names"
		case a.LiteralFreeHash == b.LiteralFreeHash:
			detail = "literal"
		}
		changes = append(changes, Change{Before: &a, After: &b, Kind: kind, Detail: detail, AmbiguousLocation: oldLocations[a.Location()] > 1 || newLocations[b.Location()] > 1})
	}
	for _, edit := range edits {
		a, okA := old[edit.From]
		b, okB := current[edit.To]
		if okA && okB && !usedOld[a.ID] && !usedNew[b.ID] {
			appendPair(a, b)
		}
	}
	for id, a := range old {
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		if b, ok := current[id]; ok && !usedOld[id] && !usedNew[id] && oldLocations[a.Location()] == 1 && newLocations[b.Location()] == 1 {
			appendPair(a, b)
		}
	}
	// 重载的内部编号随位置变化。先按同一位置下的完整代码唯一对应，
	// 避免插入一个重载后把旧重载的编号误配给另一份定义。
	oldCode, newCode := map[string][]Declaration{}, map[string][]Declaration{}
	for _, item := range old {
		key := item.Location() + "\x00" + item.CodeHash
		oldCode[key] = append(oldCode[key], item)
	}
	for _, item := range current {
		key := item.Location() + "\x00" + item.CodeHash
		newCode[key] = append(newCode[key], item)
	}
	for key, a := range oldCode {
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		b := newCode[key]
		if len(a) == 1 && len(b) == 1 && !usedOld[a[0].ID] && !usedNew[b[0].ID] {
			appendPair(a[0], b[0])
		}
	}
	oldShapes, newShapes := map[string][]Declaration{}, map[string][]Declaration{}
	shapeKey := func(snapshot Snapshot, item Declaration) string {
		return snapshot.Files[item.File].Kind.Language + "\x00" + item.Kind + "\x00" + item.ShapeHash
	}
	incomplete := map[string]bool{}
	for _, snapshot := range []Snapshot{before, after} {
		for _, file := range snapshot.Files {
			if file.Kind.Source && !file.Kind.Generated && !file.Kind.Dependency && !file.Kind.TestData && !file.CodeComplete {
				incomplete[file.Kind.Language] = true
			}
		}
	}
	for _, item := range old {
		key := shapeKey(before, item)
		oldShapes[key] = append(oldShapes[key], item)
	}
	for _, item := range current {
		key := shapeKey(after, item)
		newShapes[key] = append(newShapes[key], item)
	}
	for shape, a := range oldShapes {
		if err := ctx.Err(); err != nil {
			return changes, err
		}
		b := newShapes[shape]
		if len(a) == 1 && len(b) == 1 && !incomplete[before.Files[a[0].File].Kind.Language] && !usedOld[a[0].ID] && !usedNew[b[0].ID] {
			appendPair(a[0], b[0])
		}
	}
	for id, item := range old {
		if !usedOld[id] {
			copy := item
			changes = append(changes, Change{Before: &copy, Kind: "delete", Detail: "behavior"})
		}
	}
	for id, item := range current {
		if !usedNew[id] {
			copy := item
			changes = append(changes, Change{After: &copy, Kind: "add", Detail: "behavior"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Location() < changes[j].Location() })
	return changes, nil
}

func (c Change) Location() string {
	if c.After != nil {
		return c.After.Location()
	}
	return c.Before.Location()
}

func Migrations(changes []Change) map[string]string {
	result := map[string]string{}
	for _, change := range changes {
		if change.Before != nil && change.After != nil && !change.AmbiguousLocation && change.Before.Location() != change.After.Location() {
			result[change.Before.Location()] = change.After.Location()
		}
	}
	return result
}

func locationCounts(declarations map[string]Declaration) map[string]int {
	counts := map[string]int{}
	for _, d := range declarations {
		counts[d.Location()]++
	}
	return counts
}
