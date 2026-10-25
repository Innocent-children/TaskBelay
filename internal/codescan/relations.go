package codescan

import (
	"context"
	"sort"
	"strings"
)

type References struct {
	Forward  map[string][]string
	Backward map[string][]string
}

func IndexReferences(ctx context.Context, declarations map[string]Declaration) (References, error) {
	index := References{Forward: map[string][]string{}, Backward: map[string][]string{}}
	byName := map[string][]string{}
	for id, declaration := range declarations {
		byName[declaration.Name] = append(byName[declaration.Name], id)
	}
	for id, declaration := range declarations {
		for _, name := range declaration.References {
			for _, target := range byName[name] {
				if err := ctx.Err(); err != nil {
					return index, err
				}
				if target != id {
					index.Forward[id] = append(index.Forward[id], target)
					index.Backward[target] = append(index.Backward[target], id)
				}
			}
		}
	}
	for _, edges := range []map[string][]string{index.Forward, index.Backward} {
		for id := range edges {
			sort.Strings(edges[id])
		}
	}
	return index, ctx.Err()
}

type Position struct {
	Requirement string
	At          string
	Purpose     string
	Change      string
}

type Association struct {
	Requirements map[string][]string
	Positions    []Position
	Pending      []Position
	Candidate    bool
}

/**
 * 分别求位置的出向、入向可达集合，不能混成无向连通分量：两个函数共用
 * 一个帮助函数，并不代表它们彼此可达。边始终是名字候选，不是类型解析。
 */
func Associate(ctx context.Context, before, after Snapshot, changes []Change, positions []Position) (Association, error) {
	declarations := after.Declarations()
	present := map[string]bool{}
	for _, d := range declarations {
		present[d.Location()] = true
	}
	for _, change := range changes {
		if change.After == nil && change.Before != nil {
			declarations[change.Before.ID] = *change.Before
		}
	}
	// 子测试和参数组拥有各自引用；不能借用外层函数把相邻用例的用途合并。
	for _, snapshot := range []Snapshot{after, before} {
		for _, file := range snapshot.Files {
			for _, test := range file.Tests {
				id := test.Location()
				if _, exists := declarations[id]; !exists {
					declarations[id] = Declaration{ID: id, File: test.File, Name: test.Name, Test: true, References: test.References}
				}
			}
		}
	}
	byLocation := map[string][]string{}
	for id, d := range declarations {
		byLocation[d.Location()] = append(byLocation[d.Location()], id)
	}
	index, err := IndexReferences(ctx, declarations)
	result := Association{Requirements: map[string][]string{}, Candidate: true}
	if err != nil {
		return result, err
	}
	migrations := Migrations(changes)
	for from, to := range TestMigrations(before, after, changes) {
		migrations[from] = to
	}
	add := func(location, requirement string) {
		for _, existing := range result.Requirements[location] {
			if existing == requirement {
				return
			}
		}
		result.Requirements[location] = append(result.Requirements[location], requirement)
	}
	connect := func(root, requirement string) error {
		for _, edges := range []map[string][]string{index.Forward, index.Backward} {
			queue := []string{root}
			seen := map[string]bool{}
			for len(queue) > 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
				id := queue[len(queue)-1]
				queue = queue[:len(queue)-1]
				if seen[id] {
					continue
				}
				seen[id] = true
				add(id, requirement)
				queue = append(queue, edges[id]...)
			}
		}
		return nil
	}
	for _, position := range positions {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if migrated := migrations[position.At]; migrated != "" && position.Change != "delete" {
			position.At = migrated
		}
		result.Positions = append(result.Positions, position)
		if roots := byLocation[position.At]; len(roots) > 0 {
			if (!present[position.At] && position.Change != "delete") || (present[position.At] && position.Change == "delete") {
				result.Pending = append(result.Pending, position)
			}
			for _, root := range roots {
				if err := connect(root, position.Requirement); err != nil {
					return result, err
				}
			}
		} else if after.Files[position.At] != nil || before.Files[position.At] != nil {
			if (after.Files[position.At] == nil && position.Change != "delete") || (after.Files[position.At] != nil && position.Change == "delete") {
				result.Pending = append(result.Pending, position)
			}
			add(position.At, position.Requirement)
			for id, declaration := range declarations {
				if declaration.File == position.At {
					add(id, position.Requirement)
				}
			}
		} else if isStructuredPosition(position.At) {
			roots := structuredRoots(after, position.At)
			if strings.HasPrefix(position.At, "test:") {
				file, _, _ := strings.Cut(strings.TrimPrefix(position.At, "test:"), ":")
				if current := after.Files[file]; current != nil && !current.Operations["tests"].Complete {
					result.Pending = append(result.Pending, position)
				}
			}
			if position.Change == "delete" {
				if len(roots) == 0 {
					roots = structuredRoots(before, position.At)
				} else {
					result.Pending = append(result.Pending, position)
				}
			}
			if len(roots) == 0 {
				result.Pending = append(result.Pending, position)
				continue
			}
			add(position.At, position.Requirement)
			for _, root := range roots {
				if _, exists := declarations[root]; exists {
					if err := connect(root, position.Requirement); err != nil {
						return result, err
					}
				} else {
					add(root, position.Requirement)
				}
			}
		} else {
			result.Pending = append(result.Pending, position)
		}
	}
	for id := range result.Requirements {
		sort.Strings(result.Requirements[id])
	}
	return result, nil
}

func structuredRoots(snapshot Snapshot, at string) []string {
	roots := map[string]bool{}
	for _, file := range snapshot.Files {
		for _, entry := range file.Entries {
			if entry.Location() == at {
				roots[entry.Declaration] = true
			}
		}
		if strings.HasPrefix(at, "dep:") {
			for _, dependency := range file.Dependencies {
				if "dep:"+dependency.Name == at {
					roots[file.Path] = true
					for _, lock := range dependency.Locks {
						if snapshot.Files[lock] != nil {
							roots[lock] = true
						}
					}
				}
			}
		}
		if strings.HasPrefix(at, "test:") {
			for _, test := range file.Tests {
				if test.Location() == at {
					roots[test.Location()] = true
				}
			}
			id := strings.TrimPrefix(at, "test:")
			for _, declaration := range file.Declarations {
				if declaration.Test && declaration.Location() == id {
					roots[declaration.ID] = true
				}
			}
		}
	}
	result := make([]string, 0, len(roots))
	for root := range roots {
		result = append(result, root)
	}
	sort.Strings(result)
	return result
}

func isStructuredPosition(at string) bool {
	for _, prefix := range []string{"flag:", "cmd:", "env:", "route:", "config:", "dep:", "test:"} {
		if strings.HasPrefix(at, prefix) {
			return true
		}
	}
	return false
}
