package hook

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Innocent-children/taskbelay/internal/check"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

type Tasks struct {
	Observer
	ReportError func(error)
	Background  *check.Background
}

func (t Tasks) RecordUserMessage(ctx context.Context, e event.Event) (ledger.Message, error) {
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID}
	if e.Kind != event.UserPrompt || e.MessageID == "" {
		return ledger.Message{}, fmt.Errorf("用户消息缺少事件编号")
	}
	if err := actor.Validate(); err != nil {
		return ledger.Message{}, err
	}
	old, err := t.Store.MessageByEvent(ctx, t.Workspace.ID, actor, e.MessageID)
	if err == nil {
		if old.Text != e.Message || old.Actor.RoundID != e.RoundID {
			return ledger.Message{}, fmt.Errorf("消息事件编号与既有原文冲突")
		}
		return old, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ledger.Message{}, err
	}
	view, err := t.Store.TaskView(ctx, t.Workspace.ID, "", actor)
	if err != nil {
		return ledger.Message{}, err
	}
	before, err := t.Store.ObservationState(ctx, t.Workspace.ID, []string{})
	if err != nil {
		return ledger.Message{}, err
	}
	observation, observeErr := t.Observe(ctx, e, view.CurrentTaskID)
	if observation.ID == 0 {
		return ledger.Message{}, observeErr
	}
	message, err := t.Store.RecordMessage(ctx, ledger.Message{WorkspaceID: t.Workspace.ID, Actor: actor, EventID: e.MessageID, Text: e.Message, ObservationID: observation.ID})
	if err != nil {
		return message, err
	}
	if observeErr != nil {
		return message, observeErr
	}
	return message, t.reconcile(ctx, view, before.Context.Branch, observation)
}

func (t Tasks) resumeChanges(ctx context.Context, task ledger.TaskRecord, observation ledger.Observation) ([]ledger.FileChange, error) {
	if task.LastObservationID == 0 || task.LastObservationID == observation.ID {
		return nil, nil
	}
	previous, err := t.Store.ObservationMetadata(ctx, task.LastObservationID)
	if err != nil {
		return nil, err
	}
	diff, err := t.Workspace.Diff(ctx, previous.Tree, observation.Tree)
	if err != nil {
		return nil, err
	}
	files := make(map[string]gitstate.File, len(diff))
	view := ledger.ObservationView{LatestID: previous.ID, Files: map[string]ledger.ObservedFile{}}
	for _, change := range diff {
		view.Files[change.BeforePath] = ledger.ObservedFile{File: change.Before, ObservationID: previous.ID}
		files[change.BeforePath] = gitstate.File{}
		files[change.Path] = change.After
	}
	changes, err := t.workspaceChanges(ctx, view, files)
	if err != nil {
		return nil, err
	}
	for i := range changes {
		changes[i].AfterObservation, changes[i].At = observation.ID, observation.At
	}
	return changes, nil
}

func (t Tasks) reconcile(ctx context.Context, view ledger.TaskView, previousBranch string, observation ledger.Observation) error {
	branch := previousBranch
	if view.Task != nil {
		branch = view.Task.Context.Branch
	}
	if branch == observation.Context.Branch {
		return nil
	}
	updates := map[string][]ledger.FileChange{}
	for _, candidate := range view.Candidates {
		if candidate.State != ledger.TaskPaused || candidate.Context.Branch != observation.Context.Branch {
			continue
		}
		changes, err := t.resumeChanges(ctx, candidate, observation)
		if err != nil {
			return err
		}
		updates[candidate.ID] = changes
	}
	return t.Store.ReconcileBranch(ctx, t.Workspace.ID, view.CurrentTaskID, previousBranch, observation.ID, updates)
}

func (t Tasks) observeTask(ctx context.Context, actor ledger.Actor) (ledger.TaskView, ledger.Observation, error) {
	return t.observeEvent(ctx, event.Event{Kind: event.SessionStart, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID})
}

func (t Tasks) observeEvent(ctx context.Context, e event.Event) (ledger.TaskView, ledger.Observation, error) {
	observation, err := t.observeWorkspace(ctx, e, nil)
	if err != nil {
		return ledger.TaskView{}, observation, err
	}
	view, err := t.Store.TaskView(ctx, t.Workspace.ID, "", ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID})
	return view, observation, err
}

/**
 * 命令前后只需任务和观察。仅在切换分支、需要选择恢复任务时加载候选任务。
 */
func (t Tasks) observeWorkspace(ctx context.Context, e event.Event, command *ledger.VerificationRecord) (ledger.Observation, error) {
	task, _, err := t.Store.CommandContext(ctx, t.Workspace.ID)
	if err != nil {
		return ledger.Observation{}, err
	}
	branch := task.Context.Branch
	if task.ID == "" {
		before, err := t.Store.ObservationState(ctx, t.Workspace.ID, []string{})
		if err != nil {
			return ledger.Observation{}, err
		}
		branch = before.Context.Branch
	}
	observation, err := t.observeChanges(ctx, e, task.ID, command)
	if err != nil || branch == observation.Context.Branch {
		return observation, err
	}
	view, err := t.Store.TaskView(ctx, t.Workspace.ID, "", ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID})
	if err != nil {
		return observation, err
	}
	return observation, t.reconcile(ctx, view, branch, observation)
}

func (t Tasks) Get(ctx context.Context, actor ledger.Actor, id string) (ledger.TaskView, error) {
	observation, err := t.observeWorkspace(ctx, event.Event{Kind: event.SessionStart, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID}, nil)
	if err != nil {
		return ledger.TaskView{}, err
	}
	view, err := t.Store.TaskOverview(ctx, t.Workspace.ID, id, actor)
	if err != nil {
		return view, err
	}
	return t.resolveView(ctx, view, observation)
}

func (t Tasks) Start(ctx context.Context, actor ledger.Actor, input ledger.StartTaskInput) (ledger.TaskRecord, error) {
	call, err := ledger.NewCall(t.Workspace.ID, actor, "task_start", input)
	if err != nil {
		return ledger.TaskRecord{}, err
	}
	var result ledger.TaskRecord
	if found, err := t.Store.Replay(ctx, call, &result); found || err != nil {
		if found && err == nil {
			ledger.DebugBindTask(ctx, result.ID)
		}
		return result, err
	}
	if err := t.Store.ValidateSession(ctx, t.Workspace.ID, actor); err != nil {
		return result, err
	}
	view, observation, err := t.observeTask(ctx, actor)
	if err != nil {
		return result, err
	}
	var changes []ledger.FileChange
	if input.TaskID != "" && input.TaskID != view.CurrentTaskID {
		target, err := t.Store.TaskView(ctx, t.Workspace.ID, input.TaskID, actor)
		if err != nil {
			return result, err
		}
		changes, err = t.resumeChanges(ctx, *target.Task, observation)
		if err != nil {
			return result, err
		}
	}
	result, err = t.Store.StartTask(ctx, call, view.CurrentTaskID, observation.ID, input, changes)
	if err == nil {
		ledger.DebugBindTask(ctx, result.ID)
		if !call.Replayed() {
			action := "任务已开始"
			if input.TaskID != "" {
				action = "任务已恢复"
			}
			t.recordSession(actor, feedbackRecord(result.ID, action, result.Title, "已保存任务状态："+string(result.State)+"。", "task:"+result.ID))
		}
	}
	return result, err
}

func (t Tasks) SaveRequirement(ctx context.Context, actor ledger.Actor, input ledger.RequirementInput) (ledger.Requirement, error) {
	call, err := ledger.NewCall(t.Workspace.ID, actor, "requirement_save", input)
	if err != nil {
		return ledger.Requirement{}, err
	}
	var result ledger.Requirement
	if found, err := t.Store.Replay(ctx, call, &result); found || err != nil {
		if found && err == nil {
			ledger.DebugBindTask(ctx, result.TaskID)
		}
		return result, err
	}
	if err := t.Store.ValidateSession(ctx, t.Workspace.ID, actor); err != nil {
		return result, err
	}
	view, observation, err := t.observeTask(ctx, actor)
	if err != nil {
		return result, err
	}
	if input.ImplementedBy != nil {
		locations := make([]ledger.Location, len(*input.ImplementedBy))
		for i, location := range *input.ImplementedBy {
			// 对外请求只采用文档规定的字段，文件位置和内容标识由本机重新核对。
			clean := ledger.Location{At: location.At, Quote: location.Quote, SourceID: location.SourceID, Purpose: location.Purpose, Change: location.Change}
			locations[i], err = t.resolveLocation(ctx, clean, view.Task, observation)
			if err != nil {
				return result, err
			}
		}
		input.ImplementedBy = &locations
	}
	result, err = t.Store.SaveRequirement(ctx, call, view.CurrentTaskID, observation.ID, input)
	if err == nil {
		ledger.DebugBindTask(ctx, result.TaskID)
		if !call.Replayed() {
			t.recordSession(actor, feedbackRecord(result.TaskID, "需求已保存", result.ID, fmt.Sprintf("保存第 %d 版需求：%s", result.Revision, result.Content), fmt.Sprintf("requirement:%s@%d", result.ID, result.Revision)))
		}
	}
	return result, err
}

func (t Tasks) AddNote(ctx context.Context, actor ledger.Actor, input ledger.NoteInput) (ledger.Note, error) {
	call, err := ledger.NewCall(t.Workspace.ID, actor, "note_add", input)
	if err != nil {
		return ledger.Note{}, err
	}
	var result ledger.Note
	if found, err := t.Store.Replay(ctx, call, &result); found || err != nil {
		return result, err
	}
	if err := t.Store.ValidateSession(ctx, t.Workspace.ID, actor); err != nil {
		return result, err
	}
	view, observation, err := t.observeTask(ctx, actor)
	if err != nil {
		return result, err
	}
	result, err = t.Store.AddNote(ctx, call, view.CurrentTaskID, observation.ID, input)
	if err == nil && !call.Replayed() {
		t.recordSession(actor, feedbackRecord(result.TaskID, "笔记已保存", result.Kind+" / "+result.ID, result.Text, "note:"+result.ID))
	}
	return result, err
}

/**
 * 约定来源只从已经保存的观察点回读，文件后来变化不会改写这份引用。
 * 调用方选择要引用的项目约定，不递归扫描或自动扩大需求范围。
 */
func (t Tasks) RecordProjectSource(ctx context.Context, path string, observation ledger.Observation) (ledger.Source, error) {
	path, err := t.Workspace.TargetPath(path)
	if err != nil {
		return ledger.Source{}, err
	}
	file, err := t.Store.ObservationFile(ctx, t.Workspace.ID, observation.ID, path)
	if err != nil {
		return ledger.Source{}, err
	}
	if file.Blob == "" || file.Mode == "160000" {
		return ledger.Source{}, fmt.Errorf("约定文件不在观察点内")
	}
	content, err := t.Workspace.ReadBlob(ctx, file.Blob)
	if err != nil {
		return ledger.Source{}, err
	}
	source := ledger.Source{ID: fmt.Sprintf("convention-%x", sha256.Sum256([]byte(t.Workspace.ID+"\x00"+path+"\x00"+file.Blob))), WorkspaceID: t.Workspace.ID, Kind: "project_convention", Text: string(content), Path: path, Blob: file.Blob, ObservationID: observation.ID, At: observation.At}
	return source, t.Store.RecordSource(ctx, source)
}
