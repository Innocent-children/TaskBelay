package domain

// ID is a caller-provided stable identity.
type ID string

// IsValid reports whether id is a non-empty canonical identifier within Core limits.
func (id ID) IsValid() bool { return validateID(id) == nil }

// Digest is a lowercase hexadecimal SHA-256 digest.
type Digest string

// IsValid reports whether digest is a canonical lowercase SHA-256 hexadecimal value.
func (digest Digest) IsValid() bool { return validateDigest(digest) == nil }

type Host string

const (
	HostCodex    Host = "codex"
	HostDeepSeek Host = "deepseek"
	HostClaude   Host = "claude"
	HostZCode    Host = "zcode"
)

func (h Host) IsValid() bool {
	return h == HostCodex || h == HostDeepSeek || h == HostClaude || h == HostZCode
}

type ActionKind string

const (
	ActionResolveBlocker              ActionKind = "RESOLVE_BLOCKER"
	ActionCompleteRequirements        ActionKind = "COMPLETE_REQUIREMENTS"
	ActionCompleteDesign              ActionKind = "COMPLETE_DESIGN"
	ActionCompleteTasks               ActionKind = "COMPLETE_TASKS"
	ActionCompleteImplementation      ActionKind = "COMPLETE_IMPLEMENTATION"
	ActionCompleteTest                ActionKind = "COMPLETE_TEST"
	ActionCompleteComprehensionReview ActionKind = "COMPLETE_COMPREHENSION_REVIEW"
	ActionCompleteRefactor            ActionKind = "COMPLETE_REFACTOR"
	ActionCompleteDelivery            ActionKind = "COMPLETE_DELIVERY"
)

func (k ActionKind) IsValid() bool {
	switch k {
	case ActionCompleteRequirements, ActionCompleteDesign, ActionCompleteTasks, ActionCompleteImplementation,
		ActionCompleteTest, ActionCompleteComprehensionReview, ActionCompleteRefactor,
		ActionCompleteDelivery, ActionResolveBlocker:
		return true
	default:
		return false
	}
}

type OperationKind string

const (
	OperationOpenTask                OperationKind = "open_task"
	OperationApplyAction             OperationKind = "apply_action"
	OperationPrepareFileChange       OperationKind = "prepare_file_change"
	OperationCancelTask              OperationKind = "cancel_task"
	OperationPrepareTaskRelocation   OperationKind = "prepare_task_relocation"
	OperationPrepareTaskBranchRename OperationKind = "prepare_task_branch_rename"
	OperationObserveWorkspace        OperationKind = "observe_workspace"
	OperationAbandonTask             OperationKind = "abandon_task"
)

func (k OperationKind) IsValid() bool {
	return k == OperationOpenTask || k == OperationApplyAction || k == OperationPrepareFileChange || k == OperationCancelTask || k == OperationPrepareTaskRelocation || k == OperationPrepareTaskBranchRename || k == OperationObserveWorkspace || k == OperationAbandonTask
}

type EvidenceSource string

const (
	EvidenceSourceAutomated    EvidenceSource = "automated"
	EvidenceSourceUser         EvidenceSource = "user"
	EvidenceSourceStatic       EvidenceSource = "static"
	EvidenceSourceHostObserved EvidenceSource = "host_observed"
)

func (s EvidenceSource) IsValid() bool {
	return s == EvidenceSourceAutomated || s == EvidenceSourceUser ||
		s == EvidenceSourceStatic || s == EvidenceSourceHostObserved
}

type VerificationLevel string

const (
	VerificationMinimal  VerificationLevel = "minimal"
	VerificationTargeted VerificationLevel = "targeted"
	VerificationFull     VerificationLevel = "full"
)

func (l VerificationLevel) IsValid() bool {
	return l == VerificationMinimal || l == VerificationTargeted || l == VerificationFull
}

type TerminalStatus string

const (
	TerminalCompleted TerminalStatus = "completed"
	TerminalCancelled TerminalStatus = "cancelled"
)

func (s TerminalStatus) IsValid() bool { return s == TerminalCompleted || s == TerminalCancelled }

type RecoveryClassification string

const (
	RecoveryNotStarted             RecoveryClassification = "not_started"
	RecoveryCompletedAndRecorded   RecoveryClassification = "completed_and_recorded"
	RecoveryCompletedButUnrecorded RecoveryClassification = "completed_but_unrecorded"
	RecoveryPartiallyCompleted     RecoveryClassification = "partially_completed"
	RecoveryConflicting            RecoveryClassification = "conflicting"
)

func (c RecoveryClassification) IsValid() bool {
	switch c {
	case RecoveryNotStarted, RecoveryCompletedAndRecorded, RecoveryCompletedButUnrecorded,
		RecoveryPartiallyCompleted, RecoveryConflicting:
		return true
	default:
		return false
	}
}

type EvidenceStatus string

const (
	EvidencePassed   EvidenceStatus = "passed"
	EvidenceFailed   EvidenceStatus = "failed"
	EvidenceSkipped  EvidenceStatus = "skipped"
	EvidenceNotRun   EvidenceStatus = "not_run"
	EvidenceObserved EvidenceStatus = "observed"
)

func (s EvidenceStatus) IsValid() bool {
	switch s {
	case EvidencePassed, EvidenceFailed, EvidenceSkipped, EvidenceNotRun, EvidenceObserved:
		return true
	default:
		return false
	}
}

type AllowedEffect string

const (
	EffectReadRepository          AllowedEffect = "read_repository"
	EffectRunVerificationCommands AllowedEffect = "run_verification_commands"
	EffectPrepareDeliverySummary  AllowedEffect = "prepare_delivery_summary"
	EffectResolveBlocker          AllowedEffect = "resolve_blocker"
	EffectEditProcessArtifacts    AllowedEffect = "edit_process_artifacts"
	EffectEditProductFiles        AllowedEffect = "edit_product_files"
	EffectRequestUserDecision     AllowedEffect = "request_user_decision"
)

func (e AllowedEffect) IsValid() bool {
	switch e {
	case EffectReadRepository, EffectEditProcessArtifacts, EffectEditProductFiles,
		EffectRunVerificationCommands, EffectRequestUserDecision,
		EffectPrepareDeliverySummary, EffectResolveBlocker:
		return true
	default:
		return false
	}
}

type EvidenceRequirementKind string

const (
	RequirementRepositoryObservation EvidenceRequirementKind = "repository_observation"
	RequirementImplementationSummary EvidenceRequirementKind = "implementation_summary"
	RequirementDeliverySummary       EvidenceRequirementKind = "delivery_summary"
	RequirementBlockerResolution     EvidenceRequirementKind = "blocker_resolution"
)

type OutcomeCriterionStatus string

const (
	CriterionSatisfied  OutcomeCriterionStatus = "satisfied"
	CriterionUnverified OutcomeCriterionStatus = "unverified"
)

func (s OutcomeCriterionStatus) IsValid() bool {
	return s == CriterionSatisfied || s == CriterionUnverified
}
