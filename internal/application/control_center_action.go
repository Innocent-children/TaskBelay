package application

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type SubmitControlCenterActionRequest struct {
	RequestID        domain.ID
	TaskID           domain.ID
	ExpectedRevision uint64
	ActionID         domain.ID
	Payload          json.RawMessage
}

type AssessControlCenterRecoveryRequest struct {
	TaskID   domain.ID
	ActionID domain.ID
}
type ApplyControlCenterRecoveryRequest struct {
	TaskID   domain.ID
	ActionID domain.ID
}
type ControlCenterActionResult struct {
	Task       domain.ProcessTask
	Assessment *recovery.RecoveryAssessment
	Committed  bool
}

// controlCenterSubmission translates the semantic HTTP payload into Core requests.
type controlCenterSubmission struct {
	TransitionID domain.TransitionID `json:"transition_id"`
	Summary      string              `json:"summary"`
	Reason       string              `json:"reason"`
	Artifacts    struct {
		Current      []ArtifactSubmission `json:"current"`
		OtherProcess []ArtifactSubmission `json:"other_process"`
	} `json:"artifacts"`
	MethodResults          map[domain.MethodStepID]MethodResultSubmission `json:"method_results"`
	NodeResult             json.RawMessage                                `json:"node_result"`
	Choice                 domain.FileScopeDecision                       `json:"choice"`
	RenameID               domain.ID                                      `json:"rename_id"`
	RenameChoice           string                                         `json:"rename_choice"`
	RelocationID           domain.ID                                      `json:"relocation_id"`
	RelocationDestinations []domain.RelocationDestination                 `json:"relocation_destinations"`
	HistoryResolution      *domain.WorkspaceHistoryResolutionInput        `json:"history_resolution"`
}

func (c *ControlCenter) SubmitCurrentAction(ctx context.Context, request SubmitControlCenterActionRequest) (ControlCenterActionResult, error) {
	if !c.valid() || ctx == nil || !request.RequestID.IsValid() || !request.TaskID.IsValid() || !request.ActionID.IsValid() || request.ExpectedRevision == 0 {
		return ControlCenterActionResult{}, domain.ErrInvalidArgument
	}
	stored, err := c.tasks.LoadTask(ctx, request.TaskID)
	if err != nil {
		return ControlCenterActionResult{}, mapStoreError(err)
	}
	if stored.Revision != request.ExpectedRevision {
		return ControlCenterActionResult{}, domain.ErrRevisionConflict
	}
	if stored.CurrentAction == nil || stored.CurrentAction.ActionID != request.ActionID {
		return ControlCenterActionResult{}, domain.ErrActionStale
	}
	if err := workflow.ValidateCurrentSubmission(*stored.CurrentAction, stored.Blocker, request.Payload); err != nil {
		return ControlCenterActionResult{}, err
	}
	var payload controlCenterSubmission
	decoder := json.NewDecoder(bytes.NewReader(request.Payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil {
		return ControlCenterActionResult{}, domain.ErrInvalidArgument
	}
	var applied ApplyActionResult
	if stored.CurrentAction.Kind == domain.ActionResolveBlocker {
		resolve := RecoverActionRequest{Host: stored.OriginHost, TaskID: request.TaskID, ActionID: request.ActionID, ExpectedRevision: request.ExpectedRevision, RenameID: payload.RenameID, RenameChoice: payload.RenameChoice, RelocationID: payload.RelocationID, RelocationDestinations: payload.RelocationDestinations, HistoryResolution: payload.HistoryResolution}
		if payload.Choice != "" {
			resolve.FileScopeDecision = &domain.FileScopeDecisionInput{Choice: payload.Choice, Reason: payload.Reason}
		}
		applied, err = c.core.ResolveBlockerAction(ctx, resolve, request.RequestID)
	} else {
		applied, err = c.core.SubmitAction(ctx, SubmitActionRequest{
			RequestID: request.RequestID, Host: stored.OriginHost, TaskID: request.TaskID, ActionID: request.ActionID,
			ExpectedRevision: request.ExpectedRevision, ExpectedActionKind: stored.CurrentAction.Kind,
			TransitionID: payload.TransitionID, Summary: payload.Summary, Reason: payload.Reason,
			CurrentArtifacts: payload.Artifacts.Current, OtherProcessArtifacts: payload.Artifacts.OtherProcess,
			MethodResults: payload.MethodResults, NodeResult: payload.NodeResult,
		})
	}
	if err != nil {
		return ControlCenterActionResult{}, err
	}
	return ControlCenterActionResult{Task: applied.Task, Committed: true}, nil
}

func (c *ControlCenter) AssessTaskOperation(ctx context.Context, request AssessControlCenterRecoveryRequest) (ControlCenterActionResult, error) {
	if !c.valid() || ctx == nil || !request.ActionID.IsValid() {
		return ControlCenterActionResult{}, domain.ErrInvalidArgument
	}
	host, err := c.controlCenterTaskHost(ctx, request.TaskID)
	if err != nil {
		return ControlCenterActionResult{}, err
	}
	result, err := c.core.GetTask(ctx, GetTaskRequest{Host: host, TaskID: request.TaskID})
	if err != nil {
		return ControlCenterActionResult{}, err
	}
	assessment := result.RecoveryAssessment
	if assessment != nil && assessment.Operation.ActionID != request.ActionID {
		assessment = nil
	}
	return ControlCenterActionResult{Task: result.Task, Assessment: assessment, Committed: assessment != nil && assessment.Classification == domain.RecoveryCompletedAndRecorded}, nil
}

func (c *ControlCenter) ApplyTaskRecovery(ctx context.Context, request ApplyControlCenterRecoveryRequest) (ControlCenterActionResult, error) {
	if !c.valid() || ctx == nil || !request.ActionID.IsValid() {
		return ControlCenterActionResult{}, domain.ErrInvalidArgument
	}
	host, err := c.controlCenterTaskHost(ctx, request.TaskID)
	if err != nil {
		return ControlCenterActionResult{}, err
	}
	result, err := c.core.RecoverAction(ctx, RecoverActionRequest{Host: host, TaskID: request.TaskID, ActionID: request.ActionID})
	if err != nil {
		return ControlCenterActionResult{}, err
	}
	recorded := result.Task.LastOperation != nil && result.Task.LastOperation.ActionID != nil && *result.Task.LastOperation.ActionID == request.ActionID
	return ControlCenterActionResult{Task: result.Task, Committed: recorded}, nil
}

func (c *ControlCenter) controlCenterTaskHost(ctx context.Context, taskID domain.ID) (domain.Host, error) {
	if !taskID.IsValid() {
		return "", domain.ErrInvalidArgument
	}
	task, err := c.tasks.LoadTask(ctx, taskID)
	if err != nil {
		return "", mapStoreError(err)
	}
	if !task.OriginHost.IsValid() {
		return "", domain.ErrInternal
	}
	return task.OriginHost, nil
}
