package hook

import (
	"context"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
	"github.com/Innocent-children/taskbelay/internal/summary"
)

func (t Tasks) summaryInput(ctx context.Context, actor ledger.Actor, view ledger.TaskView) (summary.Input, error) {
	input := summary.Input{View: view, Sessionless: actor.SessionID == ""}
	state, err := t.Store.ObservationState(ctx, t.Workspace.ID, []string{})
	if err != nil {
		return input, err
	}
	observation, err := t.Store.Observation(ctx, state.LatestID)
	if err != nil {
		return input, err
	}
	if !observation.Complete {
		return input, fmt.Errorf("当前观察不完整，无法生成完整接续摘要")
	}
	input.Observation, input.ContentRevision = observation, state.Revision
	input.Partial = !observation.Full
	if view.Task != nil && observation.Full {
		changes, err := t.Workspace.Diff(ctx, view.Task.OriginTree, observation.Tree)
		if err != nil {
			return input, err
		}
		for _, change := range changes {
			input.ChangedFiles = append(input.ChangedFiles, change.Path)
		}
	} else if view.Task == nil {
		changes, err := t.Store.Changes(ctx, t.Workspace.ID, 0)
		if err != nil {
			return input, err
		}
		for _, change := range changes {
			if change.Kind == "workspace_update" {
				input.View.WorkspaceUpdates = append(input.View.WorkspaceUpdates, change)
			}
		}
	}
	if note := summary.NextStep(view); note != nil {
		input.NextStepChanged, err = t.Store.ChangedSince(ctx, t.Workspace.ID, note.ObservationID)
		if err != nil {
			return input, err
		}
	}
	sessions, err := t.Store.ActiveSessions(ctx, t.Workspace.ID, time.Now().UTC())
	if err != nil {
		return input, err
	}
	for _, session := range sessions {
		if session.Agent != actor.Agent || session.SessionID != actor.SessionID {
			input.Peers = append(input.Peers, session)
		}
	}
	if actor.SessionID == "" {
		// status 没有 Agent 身份，不把一个正在活动的 session 说成双方协作。
		if len(input.Peers) < 2 {
			input.Peers = nil
		}
	}
	return input, nil
}

type summaryDelivery struct {
	view  ledger.TaskView
	state ledger.SummaryState
}

func (t Tasks) prepareSummary(ctx context.Context, e event.Event) (string, *summaryDelivery, error) {
	actor := ledger.Actor{Agent: e.Agent, SessionID: e.SessionID, RoundID: e.RoundID}
	view, err := t.Store.TaskView(ctx, t.Workspace.ID, "", actor)
	if err != nil {
		return "", nil, err
	}
	input, err := t.summaryInput(ctx, actor, view)
	if err != nil {
		return "", nil, err
	}
	previous, err := t.Store.SummaryState(ctx, t.Workspace.ID, actor)
	if err != nil {
		return "", nil, err
	}
	state := ledger.SummaryState{InputVersion: summary.Version(input), SettingsRevision: view.Settings.Revision, GuidanceVersion: summary.GuidanceVersion(view.Settings), ObservationID: input.Observation.ID}
	fresh := e.Kind == event.SessionStart && (e.StartMode == event.NewSession || e.StartMode == event.Clear || e.StartMode == event.Compact)
	if !fresh && previous.InputVersion == state.InputVersion {
		return "", nil, nil
	}
	guidance := fresh || previous.GuidanceVersion != state.GuidanceVersion
	return summary.Build(input, guidance), &summaryDelivery{view, state}, nil
}

/**
 * MCP 的 task_start 返回目标任务的完整接续摘要。当前轮次的返回纳入
 * session 去重；旧调用回读其他任务或旧轮次时不覆盖当前接续记录。
 */
func (t Tasks) TaskSummary(ctx context.Context, actor ledger.Actor, id string) (string, error) {
	view, err := t.Store.TaskView(ctx, t.Workspace.ID, id, actor)
	if err != nil {
		return "", err
	}
	input, err := t.summaryInput(ctx, actor, view)
	if err != nil {
		return "", err
	}
	text := summary.Build(input, true)
	if id == view.CurrentTaskID && t.Store.ValidateSession(ctx, t.Workspace.ID, actor) == nil {
		state := ledger.SummaryState{InputVersion: summary.Version(input), SettingsRevision: view.Settings.Revision, GuidanceVersion: summary.GuidanceVersion(view.Settings), ObservationID: input.Observation.ID}
		if err := t.Store.SaveSummaryState(ctx, t.Workspace.ID, actor, view, state); err != nil {
			return "", err
		}
	}
	return text, nil
}

type Status struct {
	View      ledger.TaskView
	Summary   string
	HookError string
	Checks    []ledger.CheckRecord
}

func (t Tasks) Status(ctx context.Context) (Status, error) {
	view, _, err := t.observeEvent(ctx, event.Event{Kind: event.SessionStart})
	if err != nil {
		return Status{}, err
	}
	input, err := t.summaryInput(ctx, ledger.Actor{}, view)
	if err != nil {
		return Status{}, err
	}
	failure, err := t.Store.LatestHookError(ctx, t.Workspace.ID)
	if err != nil {
		return Status{}, err
	}
	checks := view.Checks
	if view.Task == nil && len(view.Candidates) > 0 {
		latest := view.Candidates[0]
		for _, candidate := range view.Candidates[1:] {
			if candidate.UpdatedAt.After(latest.UpdatedAt) {
				latest = candidate
			}
		}
		last, err := t.Store.TaskView(ctx, t.Workspace.ID, latest.ID, ledger.Actor{})
		if err != nil {
			return Status{}, err
		}
		checks = last.Checks
	}
	return Status{View: view, Summary: summary.Build(input, false), HookError: failure, Checks: checks}, nil
}
