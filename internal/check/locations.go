package check

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/codescan"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

/**
 * 分析固定输入后才发布派生位置；写事务复用观察点与需求修订的核对。
 * 需求原文、引用与历史修订保持原样，解析未完成或映射有歧义时保留旧位置。
 */
func ResolveLocations(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, view ledger.TaskView, observation ledger.Observation, before, after codescan.Snapshot) (ledger.TaskView, codescan.Association, error) {
	resolved, association, err := resolveLocations(ctx, store, workspace, view, observation, before, after)
	if err == nil {
		err = store.SaveLocations(ctx, workspace.ID, observation.ID, resolved.Requirements)
	}
	return resolved, association, err
}

func resolveLocations(ctx context.Context, store *ledger.Store, workspace gitstate.Workspace, view ledger.TaskView, observation ledger.Observation, before, after codescan.Snapshot) (ledger.TaskView, codescan.Association, error) {
	if view.Task == nil || view.Task.WorkspaceID != workspace.ID || observation.WorkspaceID != workspace.ID || !observation.Full || !observation.Complete || before.Tree != view.Task.OriginTree || after.Tree == "" || after.Tree != observation.Tree {
		return view, codescan.Association{}, fmt.Errorf("定位需要同一任务起点和当前完整观察的固定分析输入")
	}
	edits, err := store.Changes(ctx, workspace.ID, view.Task.OriginObservationID)
	if err != nil {
		return view, codescan.Association{}, err
	}
	load := snapshotLoader(ctx, workspace)
	relocations, err := verifiedRelocations(ctx, load, edits, observation, after)
	if err != nil {
		return view, codescan.Association{}, err
	}
	changes, err := codescan.Compare(ctx, before, after, relocations)
	if err != nil {
		return view, codescan.Association{}, err
	}
	view.Requirements = append([]ledger.Requirement(nil), view.Requirements...)
	var positions []codescan.Position
	for i := range view.Requirements {
		requirement := &view.Requirements[i]
		requirement.ImplementedBy = append([]ledger.Location(nil), requirement.ImplementedBy...)
		for j := range requirement.ImplementedBy {
			location := &requirement.ImplementedBy[j]
			if location.BeforeAt == "" {
				location.BeforeAt = location.At
			}
			kind, file, suffix := splitPosition(location.At)
			if file != "" {
				canonical, err := workspace.TargetPath(file)
				if err == nil {
					file = canonical
					location.At = file + suffix
					if kind == "test" {
						location.At = "test:" + location.At
					}
				}
			}
			location.Kind, location.File = kind, file
			positions = append(positions, codescan.Position{Requirement: requirement.ID, At: location.At, Purpose: location.Purpose, Change: location.Change})
		}
	}
	deleted, err := deletedLocations(ctx, load, edits, observation, after, positions)
	if err != nil {
		return view, codescan.Association{}, err
	}
	for id, declaration := range deleted.declarations {
		found := false
		for i := range changes {
			if changes[i].After == nil && changes[i].Before != nil && changes[i].Before.ID == id {
				changes[i].Before = &declaration
				found = true
			}
		}
		if !found {
			changes = append(changes, codescan.Change{Before: &declaration, Kind: "delete", Detail: "behavior"})
		}
	}
	evidenceBefore := before
	evidenceBefore.Files = maps.Clone(before.Files)
	for name, file := range deleted.files {
		evidenceBefore.Files[name] = file
	}
	association, err := codescan.Associate(ctx, evidenceBefore, after, changes, positions)
	if err != nil {
		return view, association, err
	}
	pending := map[string]bool{}
	for _, position := range association.Pending {
		pending[position.Requirement+"\x00"+position.At] = true
	}
	beforeDeclarations, afterDeclarations := before.Declarations(), after.Declarations()
	view.PendingLocations = nil
	index := 0
	for i := range view.Requirements {
		requirement := &view.Requirements[i]
		for j := range requirement.ImplementedBy {
			location := &requirement.ImplementedBy[j]
			position := association.Positions[index]
			index++
			location.At = position.At
			location.Pending = ""
			location.AnalysisTree = ""
			location.ObservationID = observation.ID
			if pending[requirement.ID+"\x00"+location.At] {
				location.Pending = "位置不存在、分析尚未完成或对应不唯一；原位置及用途保留，需补充核对"
			}
			kind, file, _ := splitPosition(location.At)
			location.Kind, location.File = kind, file
			id := strings.TrimPrefix(location.At, "test:")
			if d, ok := afterDeclarations[id]; ok {
				file = d.File
				location.File = file
			}
			if blob := deleted.blobs[location.At]; blob != "" {
				location.BeforeBlob = blob
			}
			if location.BeforeBlob == "" {
				_, oldFile, _ := splitPosition(location.BeforeAt)
				if d, ok := beforeDeclarations[strings.TrimPrefix(location.BeforeAt, "test:")]; ok {
					oldFile = d.File
				}
				if old := before.Files[oldFile]; old != nil && location.BeforeBlob == "" {
					location.BeforeBlob = old.Blob
				}
			}
			if current := after.Files[file]; current != nil && location.Change != "delete" {
				location.Blob = current.Blob
			} else {
				location.Blob = ""
			}
			if location.Pending == "" {
				location.AnalysisTree = after.Tree
			} else {
				view.PendingLocations = append(view.PendingLocations, *location)
			}
		}
	}
	return view, association, nil
}

func splitPosition(at string) (kind, file, suffix string) {
	for _, kind := range []string{"flag", "cmd", "env", "route", "config", "dep"} {
		if strings.HasPrefix(at, kind+":") {
			return kind, "", ""
		}
	}
	kind = "file"
	if strings.HasPrefix(at, "test:") {
		kind = "test"
		at = strings.TrimPrefix(at, "test:")
		if index := strings.Index(at, ":"); index >= 0 {
			return kind, at[:index], at[index:]
		}
	}
	if index := strings.LastIndex(at, ":"); index >= 0 {
		if kind == "file" {
			kind = "code"
		}
		return kind, at[:index], at[index:]
	}
	return kind, at, ""
}

type loadSnapshot func(string, gitstate.File) (codescan.Snapshot, error)

func snapshotLoader(ctx context.Context, workspace gitstate.Workspace) loadSnapshot {
	cache := map[string]codescan.Snapshot{}
	return func(path string, file gitstate.File) (codescan.Snapshot, error) {
		key := path + "\x00" + file.Blob
		if cached, ok := cache[key]; ok {
			return cached, nil
		}
		data, err := workspace.ReadBlob(ctx, file.Blob)
		if err != nil {
			return codescan.Snapshot{}, err
		}
		result := codescan.Analyze(ctx, file.Blob, []codescan.Source{{Path: path, Blob: file.Blob, Mode: file.Mode, Data: data}})
		cache[key] = result
		return result, ctx.Err()
	}
}

func verifiedRelocations(ctx context.Context, load loadSnapshot, edits []ledger.FileChange, current ledger.Observation, after codescan.Snapshot) ([]codescan.Relocation, error) {
	type identity struct{ target, path, blob string }
	mapping := map[string]identity{}
	for _, edit := range edits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if edit.Kind != "agent_edit" || edit.ToolCallID == "" || edit.AfterObservation > current.ID || edit.Before.Blob == "" || edit.After.Blob == "" {
			continue
		}
		old, err := load(edit.Path, edit.Before)
		if err != nil {
			return nil, err
		}
		next, err := load(edit.Path, edit.After)
		if err != nil {
			return nil, err
		}
		if !old.Files[edit.Path].CodeComplete || !next.Files[edit.Path].CodeComplete {
			continue
		}
		for _, fragment := range edit.Fragments {
			var beforeNames, afterNames []codescan.Declaration
			for _, d := range old.Files[edit.Path].Declarations {
				for _, name := range d.NameRanges() {
					if name.StartLine >= fragment.BeforeStart && name.EndLine < fragment.BeforeStart+fragment.BeforeLines {
						beforeNames = append(beforeNames, d)
						break
					}
				}
			}
			for _, d := range next.Files[edit.Path].Declarations {
				for _, name := range d.NameRanges() {
					if name.StartLine >= fragment.AfterStart && name.EndLine < fragment.AfterStart+fragment.AfterLines {
						afterNames = append(afterNames, d)
						break
					}
				}
			}
			if len(beforeNames) != 1 || len(afterNames) != 1 {
				continue
			}
			a, b := beforeNames[0], afterNames[0]
			if a.ID == b.ID || a.Kind != b.Kind || a.ShapeHash != b.ShapeHash {
				continue
			}
			value := identity{target: b.ID, path: edit.Path, blob: edit.After.Blob}
			for key, previous := range mapping {
				if previous.target == a.ID {
					mapping[key] = value
				}
			}
			if _, exists := mapping[a.ID]; !exists {
				mapping[a.ID] = value
			}
		}
	}
	var result []codescan.Relocation
	for from, to := range mapping {
		if file := after.Files[to.path]; file != nil && file.Blob == to.blob {
			result = append(result, codescan.Relocation{From: from, To: to.target})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].From < result[j].From })
	return result, nil
}

/**
 * 累计净改动可能已经没有中途新增又删除的声明，需求仍可引用有 blob 证据
 * 的删除前位置。只补当前确实缺失的声明，不把解析失败当作删除。
 */
func deletedLocations(ctx context.Context, load loadSnapshot, edits []ledger.FileChange, current ledger.Observation, after codescan.Snapshot, positions []codescan.Position) (deletedEvidence, error) {
	result := deletedEvidence{blobs: map[string]string{}, declarations: map[string]codescan.Declaration{}, files: map[string]*codescan.File{}}
	present := after.Declarations()
	for _, position := range positions {
		kind, file, _ := splitPosition(position.At)
		if position.Change != "delete" || file == "" {
			continue
		}
		currentFile := after.Files[file]
		if (kind == "file" && currentFile != nil) || (kind != "file" && currentFile != nil && !currentFile.CodeComplete) {
			continue
		}
		if _, exists := present[strings.TrimPrefix(position.At, "test:")]; kind != "file" && exists {
			continue
		}
		if kind == "test" && currentFile != nil {
			present := false
			for _, test := range currentFile.Tests {
				present = present || test.Location() == position.At
			}
			if present || !currentFile.Operations["tests"].Complete {
				continue
			}
		}
		for i := len(edits) - 1; i >= 0; i-- {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			edit := edits[i]
			if edit.Path != file || edit.Before.Blob == "" || edit.AfterObservation > current.ID {
				continue
			}
			previous, err := load(file, edit.Before)
			if err != nil {
				return result, err
			}
			if kind != "file" {
				found := false
				if previous.Files[file].CodeComplete {
					if kind == "test" {
						for _, test := range previous.Files[file].Tests {
							found = found || test.Location() == position.At && len(test.Unknown) == 0
						}
					}
					for _, d := range previous.Files[file].Declarations {
						if d.Location() == strings.TrimPrefix(position.At, "test:") && (kind != "test" || d.Test) {
							result.declarations[d.ID] = d
							found = true
						}
					}
				}
				if !found {
					continue
				}
			}
			result.blobs[position.At] = edit.Before.Blob
			result.files[file] = previous.Files[file]
			break
		}
	}
	return result, nil
}

type deletedEvidence struct {
	blobs        map[string]string
	declarations map[string]codescan.Declaration
	files        map[string]*codescan.File
}
