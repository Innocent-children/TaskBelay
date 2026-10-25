package hook

import (
	"context"
	"fmt"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func locationParts(at string) (kind, path, suffix string) {
	for _, entry := range []string{"flag", "cmd", "env", "route", "config", "dep"} {
		if strings.HasPrefix(at, entry+":") {
			return entry, "", strings.TrimPrefix(at, entry+":")
		}
	}
	kind = "file"
	if strings.HasPrefix(at, "test:") {
		kind = "test"
		at = strings.TrimPrefix(at, "test:")
	}
	if index := strings.LastIndex(at, ":"); index >= 0 {
		path, suffix = at[:index], at[index:]
		if kind == "file" {
			kind = "code"
		}
	} else {
		path = at
	}
	return kind, path, suffix
}

func locationAt(kind, path, suffix string) string {
	if kind == "test" {
		return "test:" + path + suffix
	}
	return path + suffix
}

func (t Tasks) resolveLocation(ctx context.Context, location ledger.Location, task *ledger.TaskRecord, current ledger.Observation) (ledger.Location, error) {
	if location.AnalysisTree != "" && location.AnalysisTree == current.Tree && current.Full && current.Complete && location.Pending == "" {
		location.ObservationID = current.ID
		return location, nil
	}
	location.AnalysisTree = ""
	kind, path, suffix := locationParts(location.At)
	location.Kind = kind
	location.Pending = ""
	if path == "" {
		if strings.TrimSpace(suffix) == "" {
			location.Pending = "位置缺少名称"
		} else {
			location.Pending = "入口或依赖尚待代码分析定位"
		}
		return location, nil
	}
	canonical, err := t.Workspace.TargetPath(path)
	if err != nil {
		location.Pending = "路径无法对应当前工作区: " + err.Error()
		return location, nil
	}
	path = canonical
	if (kind == "test" || kind == "code") && len(suffix) <= 1 {
		location.Pending = "声明或用例名称为空"
	}
	if location.BeforeAt == "" {
		location.BeforeAt = location.At
	}
	file := current.Files[path]
	if file.Blob == "" && location.ObservationID != 0 && location.Change != "delete" {
		previous, err := t.Store.Observation(ctx, location.ObservationID)
		if err != nil {
			return location, err
		}
		if previous.Tree != "" && current.Tree != "" {
			changes, err := t.Workspace.Diff(ctx, previous.Tree, current.Tree)
			if err != nil {
				return location, err
			}
			for _, change := range changes {
				if change.BeforePath == path && change.UniqueRename {
					path = change.Path
					file = current.Files[path]
					location.At = locationAt(kind, path, suffix)
					break
				}
			}
		}
	}
	location.File = path
	if file.Blob != "" && location.Change != "delete" {
		if location.Blob != "" && location.BeforeBlob == "" {
			location.BeforeBlob = location.Blob
		}
		location.Blob, location.ObservationID = file.Blob, current.ID
		if kind == "code" || kind == "test" {
			location.Pending = "文件已定位，声明或用例尚待代码分析核对"
		}
		return location, nil
	}
	if location.Change == "delete" {
		old := location.BeforeBlob
		if old == "" {
			old = location.Blob
		}
		if old == "" && task != nil {
			origin, err := t.Store.Observation(ctx, task.OriginObservationID)
			if err != nil {
				return location, err
			}
			old = origin.Files[path].Blob
		}
		if old == "" {
			changes, err := t.Store.Changes(ctx, t.Workspace.ID, 0)
			if err != nil {
				return location, err
			}
			for _, change := range changes {
				if change.Path == path && change.Before.Blob != "" && change.AfterObservation <= current.ID {
					old = change.Before.Blob
				}
			}
		}
		location.BeforeBlob = old
		location.Blob = ""
		if old != "" {
			if _, err := t.Workspace.ReadBlob(ctx, old); err != nil {
				return location, fmt.Errorf("回读已删除位置: %w", err)
			}
			if file.Blob != "" {
				location.Pending = "删除位置当前仍存在，需核对变更类型"
			} else if kind != "file" {
				location.Pending = "旧文件已保留，删除的声明或用例尚待代码分析核对"
			}
			location.ObservationID = current.ID
			return location, nil
		}
	}
	location.Pending = "位置已不存在或无法唯一对应，需补充当前位置；原记录保留"
	return location, nil
}

func (t Tasks) resolveView(ctx context.Context, view ledger.TaskView, observation ledger.Observation) (ledger.TaskView, error) {
	view.PendingLocations = []ledger.Location{}
	for i := range view.Requirements {
		requirement := &view.Requirements[i]
		// 历史版本与当前定位分开，派生位置更新不改写用户的需求修订。
		requirement.ImplementedBy = append([]ledger.Location{}, requirement.ImplementedBy...)
		for j, location := range requirement.ImplementedBy {
			resolved, err := t.resolveLocation(ctx, location, view.Task, observation)
			if err != nil {
				return view, err
			}
			requirement.ImplementedBy[j] = resolved
			if resolved.Pending != "" {
				view.PendingLocations = append(view.PendingLocations, resolved)
			}
		}
	}
	return view, t.Store.SaveLocations(ctx, t.Workspace.ID, observation.ID, view.Requirements)
}
