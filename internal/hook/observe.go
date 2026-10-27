package hook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/plan"
)

type Observer struct {
	Store     *ledger.Store
	Workspace gitstate.Workspace
}

func (o Observer) observation(e event.Event, task, trigger string, view ledger.ObservationView) ledger.Observation {
	return ledger.Observation{WorkspaceID: o.Workspace.ID, TaskID: task, Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID, Trigger: trigger, At: time.Now().UTC(), Context: view.Context, Files: make(map[string]gitstate.File), Complete: true}
}

/**
 * 完整观察同时保存 Git tree 和原始文件内容标识，避免 clean filter 或 CRLF 转换
 * 导致完整观察与目标文件核对之间产生虚假的工作区更新。
 */
func (o Observer) Observe(ctx context.Context, e event.Event, task string) (ledger.Observation, error) {
	observation, err := o.observe(ctx, e, task)
	if errors.Is(err, ledger.ErrObservationConflict) && ctx.Err() == nil {
		return o.observe(ctx, e, task)
	}
	return observation, err
}

func (o Observer) observe(ctx context.Context, e event.Event, task string) (ledger.Observation, error) {
	view, err := o.Store.ObservationState(ctx, o.Workspace.ID, nil)
	if err != nil {
		return ledger.Observation{}, err
	}
	observation := o.observation(e, task, string(e.Kind), view)
	snapshot, captureErr := o.Workspace.Capture(ctx)
	observation.Full, observation.Complete = true, snapshot.Complete()
	observation.Tree, observation.Context = snapshot.Tree, snapshot.Context
	observation.Files, observation.Incomplete, observation.Boundaries = snapshot.Files, snapshot.Incomplete, snapshot.Boundaries
	if ctx.Err() != nil {
		return observation, ctx.Err()
	}
	if snapshot.Complete() {
		for path := range view.Files {
			if _, ok := observation.Files[path]; !ok {
				observation.Files[path] = gitstate.File{}
			}
		}
	}
	changes, err := o.workspaceChanges(ctx, view, observation.Files)
	if err != nil {
		return observation, err
	}
	if err := o.retain(ctx, &observation, view, nil, nil); err != nil {
		return observation, err
	}
	err = o.Store.SaveObservation(ctx, view, &observation, changes, nil)
	return observation, errors.Join(captureErr, err)
}

func (o Observer) workspaceChanges(ctx context.Context, view ledger.ObservationView, files map[string]gitstate.File) ([]ledger.FileChange, error) {
	var changes []ledger.FileChange
	for _, path := range sortedPaths(files) {
		before, known := view.Files[path]
		after := files[path]
		if before.File == after || !known && view.LatestID == 0 {
			continue
		}
		fragments, err := o.Workspace.Fragments(ctx, before.File, after)
		if err != nil {
			return nil, err
		}
		changes = append(changes, ledger.FileChange{Path: path, BeforeObservation: before.ObservationID, Kind: "workspace_update", Before: before.File, After: after, Fragments: fragments})
	}
	return changes, nil
}

func sortedPaths(files map[string]gitstate.File) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (o Observer) retain(ctx context.Context, observation *ledger.Observation, view ledger.ObservationView, prepared []ledger.PreparedFile, expected map[string][]byte) error {
	owner := observation.TaskID
	if owner == "" {
		owner = fmt.Sprintf("session-%x", sha256.Sum256([]byte(o.Workspace.ID+"\x00"+observation.Agent+"\x00"+observation.SessionID)))
	}
	id := "o" + rand.Text()
	if observation.Full && observation.Complete {
		retained, err := o.Workspace.Retain(ctx, owner, id, observation.Tree)
		if err != nil {
			return fmt.Errorf("保留完整快照引用: %w", err)
		}
		observation.SnapshotRef = retained.Ref
	}
	var files []gitstate.File
	for path, file := range observation.Files {
		files = append(files, file, view.Files[path].File)
	}
	var contents [][]byte
	var positions []int
	for i, file := range prepared {
		files = append(files, file.Before, file.Expected)
		if content, ok := expected[file.Path]; ok {
			contents = append(contents, content)
			positions = append(positions, i)
		}
	}
	tree, blobs, err := o.Workspace.WriteEvidenceTree(ctx, files, observation.Tree, contents)
	if err != nil {
		return fmt.Errorf("写入证据 tree: %w", err)
	}
	for i, blob := range blobs {
		prepared[positions[i]].Expected.Blob = blob
	}
	retained, err := o.Workspace.Retain(ctx, owner, id+"-evidence", tree)
	if err != nil {
		return fmt.Errorf("保留证据引用: %w", err)
	}
	observation.EvidenceRef = retained.Ref
	return nil
}

/**
 * 编辑前只读取目标文件。冲突始终返回当前内容；hint 只改变返回决定，
 * 不改变核对结果，也不把过期请求记为能够归责的 Agent 编辑。
 */
func (o Observer) BeforeEdit(ctx context.Context, e event.Event, task, enforcement string) (event.Decision, error) {
	decision, err := o.beforeEdit(ctx, e, task, enforcement)
	if errors.Is(err, ledger.ErrObservationConflict) && ctx.Err() == nil {
		return o.beforeEdit(ctx, e, task, enforcement)
	}
	return decision, err
}

func (o Observer) beforeEdit(ctx context.Context, e event.Event, task, enforcement string) (event.Decision, error) {
	if e.Agent == "" || e.SessionID == "" || e.ToolCallID == "" || len(e.Edits) == 0 {
		return event.Decision{}, fmt.Errorf("编辑事件缺少来源、工具调用编号或目标")
	}
	if enforcement != "block" && enforcement != "hint" {
		return event.Decision{}, fmt.Errorf("未知的拦截强度")
	}
	var paths []string
	seen := make(map[string]bool)
	for _, edit := range e.Edits {
		requestPath := edit.Path
		if !filepath.IsAbs(requestPath) && e.WorkingDir != "" {
			dir := e.WorkingDir
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(o.Workspace.Root, dir)
			}
			requestPath = filepath.Join(dir, requestPath)
		}
		path, err := o.Workspace.TargetPath(requestPath)
		if err != nil {
			return event.Decision{}, fmt.Errorf("核对编辑路径: %w", err)
		}
		key, err := o.Workspace.TargetKey(path)
		if err != nil {
			return event.Decision{}, fmt.Errorf("核对编辑路径身份: %w", err)
		}
		if seen[key] {
			return event.Decision{}, fmt.Errorf("同一文件的多段编辑应合并为带上下文的补丁: %s", path)
		}
		seen[key] = true
		paths = append(paths, path)
	}
	view, err := o.Store.ObservationState(ctx, o.Workspace.ID, paths)
	if err != nil {
		return event.Decision{}, fmt.Errorf("读取编辑前观察: %w", err)
	}
	observation := o.observation(e, task, "edit_before", view)
	record := ledger.EditRecord{WorkspaceID: o.Workspace.ID, Agent: e.Agent, SessionID: e.SessionID, ToolCallID: e.ToolCallID, Request: e.Edits}
	contents := make(map[string]string)
	planContents := make(map[string]string)
	expectedContents := make(map[string][]byte)
	var messages []string
	for i, path := range paths {
		before, known := view.Files[path]
		current, content, err := o.Workspace.ReadObservedFile(ctx, path, before.File)
		if err != nil {
			return event.Decision{}, fmt.Errorf("读取编辑目标: %w", err)
		}
		contents[path] = string(content)
		observation.Files[path] = current
		updated := before.WorkspaceUpdate || known && before.File != current || !known && view.LatestID != 0 && current.Blob != ""
		expected, verifiable, conflict, reason := expectedContent(e.Edits[i], string(content), current.Blob != "", updated)
		planContents[path] = string(content)
		if verifiable && !e.Edits[i].Delete {
			planContents[path] = expected
		}
		prepared := ledger.PreparedFile{Path: path, Before: current, Verifiable: verifiable}
		if verifiable && !e.Edits[i].Delete {
			expectedContents[path] = []byte(expected)
			prepared.Expected = gitstate.File{Mode: current.Mode}
			if prepared.Expected.Mode == "" {
				prepared.Expected.Mode = "100644"
			}
		}
		record.Files = append(record.Files, prepared)
		if conflict {
			messages = append(messages, fmt.Sprintf("%s: %s\n当前内容：\n%s", path, reason, content))
			if enforcement == "block" {
				record.Decision.DenyTool = "编辑前置内容已过期，请依据返回的当前内容重新组织编辑。"
			}
		} else if reason != "" {
			messages = append(messages, path+": "+reason)
		}
	}
	existing, err := o.Store.Edit(ctx, o.Workspace.ID, e.Agent, e.SessionID, e.ToolCallID)
	if err == nil {
		if !reflect.DeepEqual(existing.Request, e.Edits) {
			return event.Decision{}, ledger.ErrEditExists
		}
		for _, file := range existing.Files {
			if observation.Files[file.Path] != file.Before {
				decision := event.Decision{AdditionalContext: fmt.Sprintf("%s 在此前核对后发生变化，请按当前内容重新提交编辑：\n%s", file.Path, contents[file.Path])}
				if enforcement == "block" {
					decision.DenyTool = "原工具调用的前置内容已过期，请重新核对。"
				}
				return decision, nil
			}
		}
		if existing.PlanCheck == nil || !existing.PlanCheck.Blocked {
			existing.Decision.DenyTool = record.Decision.DenyTool
		}
		return existing.Decision, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return event.Decision{}, fmt.Errorf("读取原编辑记录: %w", err)
	}
	changes, err := o.workspaceChanges(ctx, view, observation.Files)
	if err != nil {
		return event.Decision{}, fmt.Errorf("核对工作区更新: %w", err)
	}
	for _, change := range changes {
		if len(change.Fragments) == 0 {
			messages = append(messages, change.Path+": 观察到工作区更新")
		} else {
			for _, fragment := range change.Fragments {
				messages = append(messages, fmt.Sprintf("%s:%d 工作区更新：\n%s", change.Path, fragment.AfterStart, fragment.AfterText))
			}
		}
	}
	record.Decision.AdditionalContext = strings.Join(messages, "\n\n")
	planContext, err := o.Store.PlanEditContext(ctx, o.Workspace.ID, ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID})
	if err != nil {
		return event.Decision{}, fmt.Errorf("读取方案范围: %w", err)
	}
	if planContext.TaskID != task {
		return event.Decision{}, &ledger.TaskConflict{CurrentTaskID: planContext.TaskID}
	}
	planCheck, err := plan.BeforeEdit(o.Workspace, planContext, paths, planContents)
	if err != nil {
		return event.Decision{}, fmt.Errorf("核对编辑授权: %w", err)
	}
	record.PlanCheck = &planCheck
	if err := o.retain(ctx, &observation, view, record.Files, expectedContents); err != nil {
		return event.Decision{}, fmt.Errorf("保存编辑证据: %w", err)
	}
	if err := o.Store.SaveObservation(ctx, view, &observation, changes, &record); err != nil {
		return event.Decision{}, fmt.Errorf("登记编辑观察: %w", err)
	}
	return record.Decision, nil
}

func (o Observer) AfterEdit(ctx context.Context, e event.Event, task string) (ledger.Observation, error) {
	observation, err := o.afterEdit(ctx, e, task)
	if errors.Is(err, ledger.ErrObservationConflict) && ctx.Err() == nil {
		return o.afterEdit(ctx, e, task)
	}
	return observation, err
}

func (o Observer) afterEdit(ctx context.Context, e event.Event, task string) (ledger.Observation, error) {
	record, err := o.Store.Edit(ctx, o.Workspace.ID, e.Agent, e.SessionID, e.ToolCallID)
	if err != nil {
		return ledger.Observation{}, err
	}
	if record.AfterObservation != 0 {
		return o.Store.Observation(ctx, record.AfterObservation)
	}
	paths := make([]string, 0, len(record.Files))
	for _, file := range record.Files {
		paths = append(paths, file.Path)
	}
	view, err := o.Store.ObservationState(ctx, o.Workspace.ID, paths)
	if err != nil {
		return ledger.Observation{}, err
	}
	observation := o.observation(e, task, "edit_after", view)
	var changes []ledger.FileChange
	for i := range record.Files {
		prepared := record.Files[i]
		record.Files[i].Verification = "unverified"
		actual, _, err := o.Workspace.ReadFile(ctx, prepared.Path)
		if err != nil {
			return observation, err
		}
		observation.Files[prepared.Path] = actual
		baseline := view.Files[prepared.Path]
		if baseline.File == actual {
			record.Files[i].Verification = "not_applied"
			continue
		}
		fragments, err := o.Workspace.Fragments(ctx, baseline.File, actual)
		if err != nil {
			return observation, err
		}
		change := ledger.FileChange{Path: prepared.Path, BeforeObservation: baseline.ObservationID, Before: baseline.File, After: actual, ToolCallID: e.ToolCallID}
		var verified, unknown []gitstate.Fragment
		compatible := prepared.Verifiable && baseline.File == prepared.Before && record.Decision.DenyTool == ""
		if compatible && actual == prepared.Expected {
			record.Files[i].Verification = "matched"
			change.Kind = "agent_edit"
			change.Fragments = fragments
			changes = append(changes, change)
			continue
		}
		var expected []gitstate.Fragment
		if compatible {
			expected, err = o.Workspace.Fragments(ctx, prepared.Before, prepared.Expected)
			if err != nil {
				return observation, err
			}
		}
		for _, fragment := range fragments {
			matches := 0
			for _, want := range expected {
				if fragment.BeforeStart == want.BeforeStart && fragment.BeforeText == want.BeforeText && fragment.AfterText == want.AfterText && fragment.BeforeLines == want.BeforeLines && fragment.AfterLines == want.AfterLines {
					matches++
				}
			}
			if matches == 1 {
				verified = append(verified, fragment)
			} else {
				unknown = append(unknown, fragment)
			}
		}
		if len(verified) > 0 {
			record.Files[i].Verification = "partial"
			change.Kind = "agent_edit"
			change.Fragments = verified
			changes = append(changes, change)
		}
		if len(unknown) > 0 || len(fragments) == 0 || actual.Mode != prepared.Expected.Mode {
			change.Kind = "workspace_update"
			change.Fragments = unknown
			changes = append(changes, change)
		}
	}
	if err := o.retain(ctx, &observation, view, record.Files, nil); err != nil {
		return observation, err
	}
	err = o.Store.SaveObservation(ctx, view, &observation, changes, &record)
	return observation, err
}
