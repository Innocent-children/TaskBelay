package application

import (
	"encoding/json"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/recovery"
	"github.com/Innocent-children/taskbelay/internal/store"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

type TaskListFilter struct {
	Text        string
	Host        domain.Host
	Repository  string
	Node        domain.NodeID
	Lifecycle   string
	UpdatedFrom *time.Time
	UpdatedTo   *time.Time
	Page        int
}

type ListControlCenterTasksRequest struct {
	Filter TaskListFilter
}

type GetControlCenterTaskRequest struct {
	TaskID domain.ID
}

type ControlCenterTaskSummary struct {
	TaskID            domain.ID
	RequestSummary    string
	OriginHost        domain.Host
	ExecutionHost     domain.Host
	CurrentNode       domain.NodeID
	Lifecycle         string
	Revision          uint64
	UpdatedAt         time.Time
	Archived          bool
	RepositoryKeys    []domain.RepositoryKey
	RepositoryGroupID domain.Digest
	WorktreePath      string
	Blocker           *string
	Outcome           *string
}

type ControlCenterTaskList struct {
	Page    int
	HasNext bool
	Items   []ControlCenterTaskSummary
}

type ControlCenterDashboard struct {
	Counts map[string]int
	Recent []ControlCenterTaskSummary
}

type ControlCenterTaskDetail struct {
	Task            domain.ProcessTask
	Archived        bool
	Events          []store.TaskEvent
	Graph           workflow.ControlCenterGraph
	ReadOnly        bool
	PendingActionID *domain.ID
}

type SetTaskArchiveRequest struct {
	RequestID        domain.ID
	TaskID           domain.ID
	ExpectedRevision uint64
	Archived         bool
}

type PurgeControlCenterTaskRequest struct {
	RequestID        domain.ID
	TaskID           domain.ID
	ExpectedRevision uint64
	TypedTaskID      domain.ID
	Reason           string
	Irreversible     bool
}

type CancelControlCenterTaskRequest struct {
	RequestID        domain.ID
	TaskID           domain.ID
	ExpectedRevision uint64
	Reason           string
	Confirmed        bool
}

type ControlCenterMutationResult struct {
	Task     *domain.ProcessTask
	Archived *bool
	Purged   bool
}

type NewTaskInput struct {
	Request                 string
	InitialScope            []string
	InitialOutOfScope       []string
	KnownAcceptanceCriteria []string
	MethodProfile           domain.MethodProfile
}
type AdditionalRepositoryInput struct {
	Key             domain.RepositoryKey `json:"key"`
	RepositoryPath  string               `json:"repository_path"`
	WorkspaceOrigin WorkspaceOriginInput `json:"workspace_origin"`
}
type WorkspaceOriginInput struct {
	Mode                  domain.WorkspaceMode `json:"mode"`
	SourceType            string               `json:"source_type"`
	CarryChanges          bool                 `json:"carry_changes"`
	RemoteName            string               `json:"remote_name"`
	BaseBranch            string               `json:"base_branch"`
	BaseCommit            string               `json:"base_commit"`
	TaskBranch            string               `json:"task_branch"`
	ProvisioningReceiptID domain.ID            `json:"provisioning_receipt_id"`
}
type OpenTaskRequest struct {
	RequestID              domain.ID
	Host                   domain.Host
	RepositoryPath         string
	WorkspaceOrigin        *WorkspaceOriginInput
	PrimaryRepositoryKey   domain.RepositoryKey
	AdditionalRepositories []AdditionalRepositoryInput
	NewTask                *NewTaskInput
}
type OpenTaskResult struct {
	Created            bool
	Task               domain.ProcessTask
	RecoveryAssessment *recovery.RecoveryAssessment
}
type OperationProbe struct {
	OperationID             domain.ID
	ProcessID               domain.ProcessID
	ProcessDefinitionDigest domain.Digest
	SourceCursor            domain.NodeID
	ExpectedRevision        uint64
	ActionID                domain.ID
	ActionKind              domain.ActionKind
	RepositoryBindingDigest domain.Digest
	IssuanceIdentityDigest  domain.Digest
	IssuanceHistoryDigest   domain.Digest
	IssuanceContentDigest   domain.Digest
	Payload                 json.RawMessage
}

func (p OperationProbe) Reference() domain.OperationReference {
	return domain.OperationReference{OperationID: p.OperationID, Process: domain.ProcessReference{ID: p.ProcessID, DefinitionDigest: p.ProcessDefinitionDigest}, SourceCursor: p.SourceCursor, ExpectedRevision: p.ExpectedRevision, ActionID: p.ActionID, ActionKind: p.ActionKind, RepositoryBindingDigest: p.RepositoryBindingDigest, IssuanceIdentityDigest: p.IssuanceIdentityDigest, IssuanceHistoryDigest: p.IssuanceHistoryDigest, IssuanceContentDigest: p.IssuanceContentDigest}
}

type RecoveryApplyInput struct {
	OperationID  domain.ID
	SourceCursor domain.NodeID
}
type GetTaskRequest struct {
	Host            domain.Host
	TaskID          domain.ID
	OperationProbe  *OperationProbe
	BaselineHistory *domain.BaselineHistoryQuery
}
type GetTaskResult struct {
	Task               domain.ProcessTask
	RecoveryAssessment *recovery.RecoveryAssessment
	BaselineHistory    *domain.BaselineHistoryPage
}
type GetNextActionRequest struct {
	Host           domain.Host
	TaskID         domain.ID
	OperationProbe *OperationProbe
}
type NextActionResult struct {
	TaskID             domain.ID
	Process            domain.ProcessReference
	CurrentNode        domain.NodeID
	Revision           uint64
	MethodProfile      domain.MethodProfile
	Action             *domain.ProcessAction
	Outcome            *domain.ProcessOutcome
	Blocker            *domain.ProcessBlocker
	RecoveryAssessment *recovery.RecoveryAssessment
}
type ApplyActionRequest struct {
	RequestID               domain.ID
	Host                    domain.Host
	TaskID                  domain.ID
	ExpectedRevision        uint64
	ActionID                domain.ID
	ActionKind              domain.ActionKind
	ProcessID               domain.ProcessID
	ProcessDefinitionDigest domain.Digest
	SourceCursor            domain.NodeID
	RepositoryBindingDigest domain.Digest
	IssuanceIdentityDigest  domain.Digest
	IssuanceHistoryDigest   domain.Digest
	IssuanceContentDigest   domain.Digest
	Payload                 json.RawMessage
	RecoveryApply           *RecoveryApplyInput
}
type ApplyActionResult struct{ Task domain.ProcessTask }
type ArtifactSubmission struct {
	Path    string        `json:"path"`
	Digest  domain.Digest `json:"digest"`
	Summary string        `json:"summary"`
}
type MethodResultSubmission struct {
	Capability string `json:"capability"`
	Summary    string `json:"summary"`
}
type SubmitActionRequest struct {
	ExpectedRevision      uint64
	RequestID             domain.ID
	Host                  domain.Host
	TaskID                domain.ID
	ActionID              domain.ID
	ExpectedActionKind    domain.ActionKind
	TransitionID          domain.TransitionID
	Summary               string
	Reason                string
	CurrentArtifacts      []ArtifactSubmission
	OtherProcessArtifacts []ArtifactSubmission
	MethodResults         map[domain.MethodStepID]MethodResultSubmission
	NodeResult            json.RawMessage
}
type RecoverActionRequest struct {
	ExpectedRevision       uint64
	Host                   domain.Host
	TaskID                 domain.ID
	ActionID               domain.ID
	FileScopeDecision      *domain.FileScopeDecisionInput
	RelocationID           domain.ID
	RelocationDestinations []domain.RelocationDestination
	HistoryResolution      *domain.WorkspaceHistoryResolutionInput
	RenameID               domain.ID
	RenameChoice           string
}

type PrepareFileChangeRequest struct {
	Host              domain.Host
	RepositoryPath    string
	ToolName          string
	Paths             []string
	IntentDigest      domain.Digest
	PathParseComplete bool
}

type FileChangeDecision string

const (
	FileChangeAllow FileChangeDecision = "allow"
	FileChangeDeny  FileChangeDecision = "deny"
)

type PrepareFileChangeResult struct {
	Decision       FileChangeDecision
	Reason         string
	TaskID         domain.ID
	TaskRevision   uint64
	ScopeRequestID domain.ID
	Paths          []string
}

type FileScopeStatus struct {
	ExpectedPaths       []string
	CurrentChangedPaths []string
	UnexplainedPaths    []string
	Records             []domain.FileScopeRecord
	CoveredHostTools    []string
	FinalCheckEnabled   bool
}
type CancelTaskRequest struct {
	RequestID        domain.ID
	Host             domain.Host
	TaskID           domain.ID
	ExpectedRevision uint64
	Reason           string
}
type CancelTaskResult struct{ Task domain.ProcessTask }

type PrepareTaskRelocationRequest struct {
	RequestID        domain.ID
	Host             domain.Host
	TaskID           domain.ID
	ExpectedRevision uint64
}
type PrepareTaskRelocationResult struct {
	Task         domain.ProcessTask
	RelocationID domain.ID
}

type PrepareTaskBranchRenameRequest struct {
	RequestID        domain.ID
	Host             domain.Host
	TaskID           domain.ID
	ExpectedRevision uint64
	RepositoryKey    domain.RepositoryKey
	TargetBranch     string
	Reason           string
}
type PrepareTaskBranchRenameResult struct {
	Task     domain.ProcessTask
	RenameID domain.ID
}

type AbandonTaskRequest struct {
	RequestID        domain.ID
	Host             domain.Host
	TaskID           domain.ID
	ExpectedRevision uint64
	Reason           string
}
type AbandonTaskResult struct{ Task domain.ProcessTask }
