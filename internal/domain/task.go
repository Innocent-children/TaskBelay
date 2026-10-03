package domain

import (
	"strings"
	"time"
)

type TaskIntent struct {
	Request                 string        `json:"request"`
	InitialScope            []string      `json:"initial_scope"`
	InitialOutOfScope       []string      `json:"initial_out_of_scope"`
	KnownAcceptanceCriteria []string      `json:"known_acceptance_criteria"`
	MethodProfile           MethodProfile `json:"method_profile"`
}

func (i TaskIntent) Validate() error {
	if requireNormalizedText(i.Request, MaxGoalBytes, true) != nil || !i.MethodProfile.IsValid() {
		return ErrInvalidArgument
	}
	for _, list := range [][]string{i.InitialScope, i.InitialOutOfScope, i.KnownAcceptanceCriteria} {
		if validateNormalizedList(list) != nil {
			return ErrInvalidArgument
		}
	}
	return nil
}

type TransitionProjection struct {
	TransitionID       TransitionID      `json:"transition_id"`
	Destination        NodeID            `json:"destination_node"`
	Guard              TransitionGuardID `json:"guard_id"`
	Description        string            `json:"description"`
	SelectionCondition string            `json:"selection_condition"`
	ReasonRequired     bool              `json:"reason_required"`
}
type NodeContractProjection struct {
	Purpose              string   `json:"purpose"`
	EntryConditions      []string `json:"entry_conditions"`
	CompletionConditions []string `json:"completion_conditions"`
}
type ProcessAction struct {
	ActionID                ID                     `json:"action_id"`
	Kind                    ActionKind             `json:"kind"`
	TaskID                  ID                     `json:"task_id"`
	Revision                uint64                 `json:"revision"`
	Process                 ProcessReference       `json:"process"`
	NodeID                  NodeID                 `json:"current_node"`
	RepositoryBindingDigest Digest                 `json:"repository_binding_digest"`
	IssuanceIdentityDigest  Digest                 `json:"issuance_identity_digest"`
	IssuanceHistoryDigest   Digest                 `json:"issuance_history_digest"`
	IssuanceContentDigest   Digest                 `json:"issuance_content_digest"`
	AllowedEffects          []AllowedEffect        `json:"allowed_effects"`
	RequiredEvidence        []EvidenceRequirement  `json:"required_evidence"`
	PayloadContract         string                 `json:"payload_contract"`
	NodeContract            NodeContractProjection `json:"node_contract"`
	AvailableTransitions    []TransitionProjection `json:"available_transitions"`
	MethodProfile           MethodProfile          `json:"method_profile"`
	SemanticMethodSteps     []SemanticMethodStep   `json:"method_steps"`
	Guidance                string                 `json:"guidance"`
	IssuedAt                time.Time              `json:"issued_at"`
}

func (a ProcessAction) Validate() error {
	if validateID(a.ActionID) != nil || validateID(a.TaskID) != nil || a.Revision == 0 || a.Process.Validate() != nil || !a.NodeID.IsValid() || !a.Kind.IsValid() || !a.RepositoryBindingDigest.IsValid() || !a.IssuanceIdentityDigest.IsValid() || !a.IssuanceHistoryDigest.IsValid() || !a.IssuanceContentDigest.IsValid() || !a.MethodProfile.IsValid() || validateUTC(a.IssuedAt) != nil || requireNormalizedText(a.PayloadContract, MaxIdentifierBytes, true) != nil || requireNormalizedText(a.Guidance, MaxGuidanceBytes, true) != nil || requireNormalizedText(a.NodeContract.Purpose, MaxGuidanceBytes, true) != nil || len(a.NodeContract.EntryConditions) == 0 || validateNormalizedList(a.NodeContract.EntryConditions) != nil || len(a.NodeContract.CompletionConditions) == 0 || validateNormalizedList(a.NodeContract.CompletionConditions) != nil || len(a.AllowedEffects) == 0 || len(a.RequiredEvidence) == 0 || len(a.SemanticMethodSteps) == 0 || len(a.AvailableTransitions) > MaxStandardProcessTransitions {
		return ErrInvalidArgument
	}
	seenEffects := map[AllowedEffect]bool{}
	for _, effect := range a.AllowedEffects {
		if !effect.IsValid() || seenEffects[effect] {
			return ErrInvalidArgument
		}
		seenEffects[effect] = true
	}
	seenEvidence := map[EvidenceRequirementKind]bool{}
	for _, requirement := range a.RequiredEvidence {
		if requirement.Validate() != nil || seenEvidence[requirement.Kind] {
			return ErrInvalidArgument
		}
		seenEvidence[requirement.Kind] = true
	}
	seenTransitions := map[TransitionID]bool{}
	for _, transition := range a.AvailableTransitions {
		if !transition.TransitionID.IsValid() || !transition.Destination.IsValid() || !transition.Guard.IsValid() || requireNormalizedText(transition.Description, MaxGuidanceBytes, true) != nil || requireNormalizedText(transition.SelectionCondition, MaxGuidanceBytes, true) != nil || seenTransitions[transition.TransitionID] {
			return ErrInvalidArgument
		}
		seenTransitions[transition.TransitionID] = true
	}
	seenSteps := map[MethodStepID]bool{}
	for _, step := range a.SemanticMethodSteps {
		if step.Validate() != nil || seenSteps[step.StepID] {
			return ErrInvalidArgument
		}
		seenSteps[step.StepID] = true
	}
	return nil
}

type ProcessOutcome struct {
	Status                TerminalStatus     `json:"status"`
	Summary               string             `json:"summary"`
	RequirementsRevision  uint32             `json:"requirements_revision"`
	Acceptance            []OutcomeCriterion `json:"acceptance"`
	TestRecordID          ID                 `json:"test_record_id"`
	ComprehensionRecordID ID                 `json:"comprehension_record_id"`
	AutomatedEvidenceIDs  []ID               `json:"automated_evidence_ids"`
	ManualEvidenceIDs     []ID               `json:"manual_evidence_ids"`
	FinalRepositoryDigest Digest             `json:"final_repository_digest"`
	Risks                 []string           `json:"risks"`
	CompletedAt           time.Time          `json:"completed_at"`
}

func (o ProcessOutcome) Validate() error {
	if !o.Status.IsValid() || requireNormalizedText(o.Summary, MaxOutcomeSummaryBytes, true) != nil || !o.FinalRepositoryDigest.IsValid() || validateUTC(o.CompletedAt) != nil || validateNormalizedList(o.Risks) != nil || len(o.AutomatedEvidenceIDs)+len(o.ManualEvidenceIDs) > MaxRetainedEvidenceItems {
		return ErrInvalidArgument
	}
	if o.Status == TerminalCompleted && (o.RequirementsRevision == 0 || len(o.Acceptance) == 0 || validateID(o.TestRecordID) != nil || validateID(o.ComprehensionRecordID) != nil) {
		return ErrInvalidArgument
	}
	for _, criterion := range o.Acceptance {
		if criterion.Validate() != nil {
			return ErrInvalidArgument
		}
	}
	seen := map[ID]bool{}
	for _, ids := range [][]ID{o.AutomatedEvidenceIDs, o.ManualEvidenceIDs} {
		for _, id := range ids {
			if validateID(id) != nil || seen[id] {
				return ErrInvalidArgument
			}
			seen[id] = true
		}
	}
	return nil
}

type ProcessTask struct {
	TaskID                        ID                             `json:"task_id"`
	OriginHost                    Host                           `json:"origin_host"`
	Intent                        TaskIntent                     `json:"intent"`
	Process                       ProcessReference               `json:"process"`
	CurrentNode                   NodeID                         `json:"current_node"`
	ResumeNode                    *NodeID                        `json:"resume_node"`
	CurrentAction                 *ProcessAction                 `json:"current_action"`
	Blocker                       *ProcessBlocker                `json:"blocker"`
	LastOperation                 *LastOperation                 `json:"last_operation"`
	PrimaryRepositoryKey          RepositoryKey                  `json:"primary_repository_key"`
	WorkspaceOrigin               WorkspaceOrigin                `json:"workspace_origin"`
	Repository                    RepositoryBinding              `json:"repository"`
	AdditionalRepositories        []RepositoryScopeEntry         `json:"additional_repositories"`
	Requirements                  *RequirementsBaseline          `json:"requirements"`
	Design                        *DesignBaseline                `json:"design"`
	TaskPlan                      *TaskPlanBaseline              `json:"task_plan"`
	Implementation                *ImplementationRecord          `json:"implementation"`
	Test                          *TestRecord                    `json:"test"`
	Comprehension                 *ComprehensionAssessment       `json:"comprehension"`
	BaselineHistory               []BaselineReference            `json:"baseline_history"`
	Evidence                      []EvidenceSummary              `json:"evidence"`
	VerificationAttempts          []VerificationAttempt          `json:"verification_attempts"`
	VerificationBudgetAdjustments []VerificationBudgetAdjustment `json:"verification_budget_adjustments"`
	FileScopeRecords              []FileScopeRecord              `json:"file_scope_records"`
	CurrentChangedPaths           []string                       `json:"current_changed_paths"`
	Relocation                    *TaskRelocation                `json:"relocation,omitempty"`
	BranchRename                  *TaskBranchRename              `json:"branch_rename,omitempty"`
	Outcome                       *ProcessOutcome                `json:"outcome"`
	Revision                      uint64                         `json:"revision"`
	CreatedAt                     time.Time                      `json:"created_at"`
	UpdatedAt                     time.Time                      `json:"updated_at"`
	CompletedAt                   *time.Time                     `json:"completed_at"`
}

func (t ProcessTask) Validate() error {
	digests, err := t.EffectiveWorkspaceDigests()
	effectiveRepositoryDigest := digests.Binding
	if err != nil || t.validateRepositoryPaths() != nil {
		return ErrInvalidArgument
	}
	derivedChangedPaths := RepositoryScopeTaskSurfacePaths(
		t.EffectivePrimaryRepositoryKey(),
		t.Repository,
		t.AdditionalRepositories,
	)
	if !sameStrings(t.CurrentChangedPaths, derivedChangedPaths) {
		return ErrInvalidArgument
	}
	if validateID(t.TaskID) != nil || !t.OriginHost.IsValid() || t.Intent.Validate() != nil || t.Process.Validate() != nil || !t.CurrentNode.IsValid() || t.Revision == 0 || validateUTC(t.CreatedAt) != nil || validateUTC(t.UpdatedAt) != nil || t.UpdatedAt.Before(t.CreatedAt) || len(t.Evidence) > MaxRetainedEvidenceItems || len(t.VerificationAttempts) > MaxRetainedVerificationAttempts || len(t.VerificationBudgetAdjustments) > MaxVerificationBudgetAdjustments || len(t.FileScopeRecords) > MaxFileScopeRecords || len(t.CurrentChangedPaths) > MaxFingerprintPaths {
		return ErrInvalidArgument
	}
	if t.Requirements != nil && t.Requirements.Validate() != nil {
		return ErrInvalidArgument
	}
	if t.Design != nil && (t.Design.Validate() != nil || t.Requirements == nil || t.Design.RequirementsRevision != t.Requirements.Revision) {
		return ErrInvalidArgument
	}
	if t.TaskPlan != nil && (t.TaskPlan.Validate() != nil || t.Design == nil || t.TaskPlan.DesignRevision != t.Design.Revision) {
		return ErrInvalidArgument
	}
	if !verificationBudgetAdjustmentsValid(t) {
		return ErrInvalidArgument
	}
	if t.TaskPlan != nil && t.TaskPlan.Confirmation != nil && !t.TaskPlan.Confirmation.Matches(t.Requirements, t.Design, t.TaskPlan) {
		return ErrInvalidArgument
	}
	if t.Implementation != nil && (t.Implementation.Validate() != nil || t.TaskPlan == nil || t.Implementation.TaskPlanRevision != t.TaskPlan.Revision) {
		return ErrInvalidArgument
	}
	if !implementationContentAuthorityValid(t, digests.Content) {
		return ErrInvalidArgument
	}
	if t.CurrentNode.Terminal() {
		if t.CurrentAction != nil || t.Blocker != nil || t.ResumeNode != nil || t.Relocation != nil || t.Outcome == nil || t.CompletedAt == nil {
			return ErrInvalidArgument
		}
		if t.Outcome.Validate() != nil || !t.CompletedAt.Equal(t.Outcome.CompletedAt) || (t.CurrentNode == NodeDone && t.Outcome.Status != TerminalCompleted) || (t.CurrentNode == NodeCancelled && t.Outcome.Status != TerminalCancelled) {
			return ErrInvalidArgument
		}
	} else if t.CurrentNode == NodeBlocked {
		if t.Blocker == nil || t.Blocker.Validate() != nil || t.ResumeNode == nil || !t.ResumeNode.Normal() ||
			t.Blocker.ResumeNode != *t.ResumeNode || t.Blocker.Condition.ExpectedBindingDigest != effectiveRepositoryDigest ||
			t.CurrentAction == nil || t.CurrentAction.Kind != ActionResolveBlocker {
			return ErrInvalidArgument
		}
	} else if !t.CurrentNode.Normal() || t.CurrentAction == nil || t.Blocker != nil || t.ResumeNode != nil || t.Outcome != nil || t.CompletedAt != nil {
		return ErrInvalidArgument
	}
	if t.CurrentAction != nil && (t.CurrentAction.Validate() != nil || t.CurrentAction.TaskID != t.TaskID || t.CurrentAction.Revision != t.Revision || t.CurrentAction.Process != t.Process || t.CurrentAction.NodeID != t.CurrentNode || t.CurrentAction.RepositoryBindingDigest != effectiveRepositoryDigest || t.CurrentAction.IssuanceIdentityDigest != digests.Identity || t.CurrentAction.IssuanceHistoryDigest != digests.History || t.CurrentAction.IssuanceContentDigest != digests.Content || t.CurrentAction.MethodProfile != t.Intent.MethodProfile) {
		return ErrInvalidArgument
	}
	if !t.BranchRenameValid() {
		return ErrInvalidArgument
	}
	if t.Relocation != nil {
		if t.Relocation.Validate() != nil || t.CurrentNode != NodeBlocked || t.Blocker == nil || t.ResumeNode == nil ||
			t.Blocker.Cause != BlockerCauseTaskRelocationPending ||
			t.Relocation.RelocationID != t.Blocker.Condition.RelocationID ||
			t.Relocation.ResumeNode != *t.ResumeNode ||
			t.Relocation.SourceBindingDigest != t.Blocker.ObservedBindingDigest ||
			!t.Relocation.PreparedAt.Equal(t.Blocker.CreatedAt) {
			return ErrInvalidArgument
		}
		for _, path := range t.Relocation.SourceTaskSurface {
			if t.ValidateRepositoryPath(path) != nil {
				return ErrInvalidArgument
			}
		}
	}
	if t.CurrentNode == NodeBlocked && t.Blocker != nil && t.Blocker.Cause == BlockerCauseTaskRelocationPending && t.Relocation == nil {
		return ErrInvalidArgument
	}
	if t.Blocker != nil && t.Blocker.Cause == BlockerCauseWorkspaceHistoryConflict && !workspaceHistorySnapshotMatchesTask(t) {
		return ErrInvalidArgument
	}
	if !fileScopeStateValid(t) {
		return ErrInvalidArgument
	}
	if !authorityMatchesCurrentNode(t) {
		return ErrInvalidArgument
	}
	if t.LastOperation != nil && (t.LastOperation.Validate() != nil || t.LastOperation.ToRevision != t.Revision) {
		return ErrInvalidArgument
	}
	evidenceIDs := map[ID]bool{}
	evidenceByID := map[ID]EvidenceSummary{}
	for _, item := range t.Evidence {
		if item.Validate() != nil || evidenceIDs[item.EvidenceID] {
			return ErrInvalidArgument
		}
		evidenceIDs[item.EvidenceID] = true
		evidenceByID[item.EvidenceID] = item
	}
	var previousAttemptRevision uint64
	for _, attempt := range t.VerificationAttempts {
		if attempt.Validate() != nil || attempt.TaskRevision <= previousAttemptRevision || attempt.TaskRevision > t.Revision {
			return ErrInvalidArgument
		}
		for _, id := range attempt.EvidenceIDs {
			item, ok := evidenceByID[id]
			if !ok || item.TaskPlanRevision != attempt.TaskPlanRevision || !item.RecordedAt.Equal(attempt.RecordedAt) {
				return ErrInvalidArgument
			}
		}
		previousAttemptRevision = attempt.TaskRevision
	}
	if t.Test != nil && (t.Test.Validate() != nil || t.Requirements == nil || t.Design == nil || t.TaskPlan == nil || t.Test.RequirementsRevision != t.Requirements.Revision || t.Test.DesignRevision != t.Design.Revision || t.Test.TaskPlanRevision != t.TaskPlan.Revision || t.CurrentNode != NodeCancelled && t.Test.ContentDigest != digests.Content) {
		return ErrInvalidArgument
	}
	if t.Test != nil {
		for _, id := range t.Test.EvidenceIDs {
			item, ok := evidenceByID[id]
			if !ok || item.TaskPlanRevision != t.Test.TaskPlanRevision || !TestEvidenceEligible(t.Test, item) {
				return ErrInvalidArgument
			}
		}
	}
	if t.Test != nil && t.Test.KnownFailureAcceptance != nil {
		var checks []EvidenceSummary
		for _, id := range t.Test.EvidenceIDs {
			checks = append(checks, evidenceByID[id])
		}
		if !KnownFailuresAccepted(t.Test.KnownFailureAcceptance, checks, t.Test.TaskPlanRevision, t.Test.ContentDigest) {
			return ErrInvalidArgument
		}
	}
	if t.TaskPlan != nil && !taskPlanAcceptanceIndexesValid(*t.TaskPlan, t.Requirements) {
		return ErrInvalidArgument
	}
	if t.Implementation != nil && !implementationWorkItemsValid(*t.Implementation, t.TaskPlan) {
		return ErrInvalidArgument
	}
	node := t.CurrentNode
	if node == NodeBlocked && t.ResumeNode != nil {
		node = *t.ResumeNode
	}
	if t.Test != nil || node == NodeTest || node == NodeComprehensionReview || node == NodeDelivery || node == NodeDone {
		if t.Implementation == nil || !CompletedWorkItemsCoverPlan(t.TaskPlan, t.Implementation.CompletedWorkItemIDs) {
			return ErrInvalidArgument
		}
	}
	if t.Comprehension != nil && (t.Comprehension.Validate() != nil || t.Test == nil || t.Comprehension.TestRecordID != t.Test.RecordID || t.Comprehension.RequirementsRevision != t.Test.RequirementsRevision || t.Comprehension.DesignRevision != t.Test.DesignRevision || t.Comprehension.TaskPlanRevision != t.Test.TaskPlanRevision || t.Comprehension.ContentDigest != t.Test.ContentDigest) {
		return ErrInvalidArgument
	}
	if t.Comprehension != nil {
		item, ok := evidenceByID[t.Comprehension.UserEvidenceID]
		if !ok || item.TaskPlanRevision != t.Comprehension.TaskPlanRevision || item.Source != EvidenceSourceUser || item.Status != EvidencePassed || !item.RecordedAt.Equal(t.Comprehension.ConfirmedAt) {
			return ErrInvalidArgument
		}
	}
	if t.Outcome != nil && t.Outcome.Status == TerminalCompleted && !completedOutcomeMatchesTask(t, evidenceByID) {
		return ErrInvalidArgument
	}
	history := map[BaselineKind]map[uint32]bool{}
	for _, ref := range t.BaselineHistory {
		if history[ref.Kind] == nil {
			history[ref.Kind] = map[uint32]bool{}
		}
		if history[ref.Kind][ref.Revision] || !baselineReferencePrecedesCurrent(ref, t) {
			return ErrInvalidArgument
		}
		history[ref.Kind][ref.Revision] = true
	}
	for _, ref := range t.BaselineHistory {
		if ref.Validate() != nil {
			return ErrInvalidArgument
		}
	}
	if !baselineRevisionChainsValid(t, history) {
		return ErrInvalidArgument
	}
	// The aggregate budget protects non-history state. Complete history has no
	// configured retention or aggregate byte limit. Each reference remains validated.
	withoutHistory := t
	withoutHistory.BaselineHistory = nil
	if size, err := compactJSONSize(withoutHistory); err != nil || size > MaxPersistedTaskSnapshotBytes {
		return ErrInvalidArgument
	}
	for _, record := range t.FileScopeRecords {
		for _, path := range record.Paths {
			if t.ValidateRepositoryPath(path) != nil {
				return ErrInvalidArgument
			}
		}
	}
	for _, path := range t.CurrentChangedPaths {
		if t.ValidateRepositoryPath(path) != nil {
			return ErrInvalidArgument
		}
	}
	return nil
}

func fileScopeStateValid(t ProcessTask) bool {
	requestIDs := make(map[ID]bool, len(t.FileScopeRecords))
	for index, path := range t.CurrentChangedPaths {
		if ValidateRepositoryContractPath(path) != nil || index > 0 && t.CurrentChangedPaths[index-1] >= path {
			return false
		}
	}
	highestPlanRevision := uint32(0)
	if t.TaskPlan != nil {
		highestPlanRevision = t.TaskPlan.Revision
	}
	for _, reference := range t.BaselineHistory {
		if reference.Kind == BaselineTaskPlan && reference.Revision > highestPlanRevision {
			highestPlanRevision = reference.Revision
		}
	}
	pending := ID("")
	for _, record := range t.FileScopeRecords {
		if record.Validate() != nil || requestIDs[record.RequestID] || record.TaskPlanRevision > highestPlanRevision {
			return false
		}
		requestIDs[record.RequestID] = true
		if record.Decision == FileScopePending {
			if pending != "" {
				return false
			}
			pending = record.RequestID
		}
	}
	fileScopeBlocked := t.CurrentNode == NodeBlocked && t.Blocker != nil && t.Blocker.Cause == BlockerCauseFileScopeDecision
	if fileScopeBlocked {
		return pending != "" && t.Blocker.Condition.ScopeRequestID == pending
	}
	return pending == ""
}

func (t ProcessTask) EffectivePrimaryRepositoryKey() RepositoryKey {
	if t.PrimaryRepositoryKey == "" {
		return DefaultPrimaryRepositoryKey
	}
	return t.PrimaryRepositoryKey
}

func (t ProcessTask) EffectiveRepositoryBindingDigest() (Digest, error) {
	digests, err := t.EffectiveWorkspaceDigests()
	return digests.Binding, err
}

func (t ProcessTask) EffectiveWorkspaceDigests() (WorkspaceDigests, error) {
	return effectiveRepositoryDigests(t.EffectivePrimaryRepositoryKey(), t.WorkspaceOrigin, t.Repository, t.AdditionalRepositories)
}

func RepositoryScopeMembershipEqual(left, right ProcessTask) bool {
	if _, err := left.EffectiveRepositoryBindingDigest(); err != nil {
		return false
	}
	if _, err := right.EffectiveRepositoryBindingDigest(); err != nil {
		return false
	}
	if left.EffectivePrimaryRepositoryKey() != right.EffectivePrimaryRepositoryKey() ||
		left.Repository.WorktreeInstanceDigest != right.Repository.WorktreeInstanceDigest ||
		len(left.AdditionalRepositories) != len(right.AdditionalRepositories) {
		return false
	}
	for i := range left.AdditionalRepositories {
		leftEntry, rightEntry := left.AdditionalRepositories[i], right.AdditionalRepositories[i]
		if leftEntry.Key != rightEntry.Key || leftEntry.Binding.WorktreeInstanceDigest != rightEntry.Binding.WorktreeInstanceDigest {
			return false
		}
	}
	return true
}

func (t ProcessTask) ValidateRepositoryPath(value string) error {
	if len(t.AdditionalRepositories) == 0 {
		if strings.Contains(value, repositoryPathSeparator) {
			return ErrInvalidArgument
		}
		return validateRepositoryRelativePath(value)
	}
	key, relative, ok := strings.Cut(value, repositoryPathSeparator)
	if !ok || !RepositoryKey(key).IsValid() || validateRepositoryRelativePath(relative) != nil {
		return ErrInvalidArgument
	}
	if RepositoryKey(key) == t.EffectivePrimaryRepositoryKey() {
		return nil
	}
	for _, entry := range t.AdditionalRepositories {
		if entry.Key == RepositoryKey(key) {
			return nil
		}
	}
	return ErrInvalidArgument
}

func (t ProcessTask) validateRepositoryPaths() error {
	for _, artifacts := range [][]ArtifactReference{
		artifactReferences(t.Requirements), designArtifactReferences(t.Design), taskPlanArtifactReferences(t.TaskPlan),
	} {
		for _, artifact := range artifacts {
			if t.ValidateRepositoryPath(artifact.Path) != nil {
				return ErrInvalidArgument
			}
		}
	}
	if t.TaskPlan != nil {
		for _, item := range t.TaskPlan.WorkItems {
			for _, path := range item.ExpectedPaths {
				if t.ValidateRepositoryPath(path) != nil {
					return ErrInvalidArgument
				}
			}
		}
	}
	if t.Implementation != nil {
		for _, path := range t.Implementation.ActionChangedPaths {
			if t.ValidateRepositoryPath(path) != nil {
				return ErrInvalidArgument
			}
		}
	}
	for _, attempt := range t.VerificationAttempts {
		for _, path := range attempt.ImplementationPaths {
			if t.ValidateRepositoryPath(path) != nil {
				return ErrInvalidArgument
			}
		}
	}
	return nil
}

func artifactReferences(value *RequirementsBaseline) []ArtifactReference {
	if value == nil {
		return nil
	}
	return value.ArtifactRefs
}

func taskPlanArtifactReferences(value *TaskPlanBaseline) []ArtifactReference {
	if value == nil {
		return nil
	}
	return value.ArtifactRefs
}

func designArtifactReferences(value *DesignBaseline) []ArtifactReference {
	if value == nil {
		return nil
	}
	return value.ArtifactRefs
}

func authorityMatchesCurrentNode(t ProcessTask) bool {
	node := t.CurrentNode
	if node == NodeBlocked {
		if t.ResumeNode == nil {
			return false
		}
		node = *t.ResumeNode
	}
	if node != NodeRequirements && node != NodeDesign && node != NodeTasks && node != NodeCancelled && (t.TaskPlan == nil || t.TaskPlan.Confirmation == nil || !t.TaskPlan.Confirmation.Matches(t.Requirements, t.Design, t.TaskPlan)) {
		return false
	}
	switch node {
	case NodeRequirements:
		return t.Design == nil && t.TaskPlan == nil && t.Implementation == nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeDesign:
		return t.Requirements != nil && t.TaskPlan == nil && t.Implementation == nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeTasks:
		return t.Requirements != nil && t.Design != nil && (t.TaskPlan == nil || t.TaskPlan.Confirmation == nil) && t.Implementation == nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeImplement:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeTest:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Implementation != nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeComprehensionReview:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Implementation != nil && t.Test != nil && t.Comprehension == nil && t.Outcome == nil
	case NodeRefactor:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Implementation != nil && t.Test == nil && t.Comprehension == nil && t.Outcome == nil
	case NodeDelivery:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Implementation != nil && t.Test != nil && t.Comprehension != nil && t.Outcome == nil
	case NodeDone:
		return t.Requirements != nil && t.Design != nil && t.TaskPlan != nil && t.Implementation != nil && t.Test != nil && t.Comprehension != nil && t.Outcome != nil && t.Outcome.Status == TerminalCompleted && t.CompletedAt != nil
	case NodeCancelled:
		return t.Outcome != nil && t.Outcome.Status == TerminalCancelled && t.CompletedAt != nil
	default:
		return false
	}
}

func implementationContentAuthorityValid(task ProcessTask, current Digest) bool {
	if task.Implementation == nil || task.CurrentNode == NodeCancelled {
		return true
	}
	node := task.CurrentNode
	if node == NodeBlocked {
		if task.ResumeNode == nil {
			return false
		}
		node = *task.ResumeNode
	}
	if node == NodeImplement || node == NodeRefactor {
		return true
	}
	return task.Implementation.ContentDigest == current
}

func taskPlanAcceptanceIndexesValid(plan TaskPlanBaseline, requirements *RequirementsBaseline) bool {
	if requirements == nil {
		return false
	}
	for _, item := range plan.WorkItems {
		for _, index := range item.AcceptanceIndexes {
			if int(index) >= len(requirements.AcceptanceCriteria) {
				return false
			}
		}
	}
	return true
}

func implementationWorkItemsValid(implementation ImplementationRecord, plan *TaskPlanBaseline) bool {
	if plan == nil {
		return false
	}
	known := make(map[ID]bool, len(plan.WorkItems))
	for _, item := range plan.WorkItems {
		known[item.WorkItemID] = true
	}
	for _, id := range implementation.CompletedWorkItemIDs {
		if !known[id] {
			return false
		}
	}
	return true
}

func baselineRevisionChainsValid(t ProcessTask, history map[BaselineKind]map[uint32]bool) bool {
	current := map[BaselineKind]uint32{}
	if t.Requirements != nil {
		current[BaselineRequirements] = t.Requirements.Revision
	}
	if t.Design != nil {
		current[BaselineDesign] = t.Design.Revision
	}
	if t.TaskPlan != nil {
		current[BaselineTaskPlan] = t.TaskPlan.Revision
	}
	for _, kind := range []BaselineKind{BaselineRequirements, BaselineDesign, BaselineTaskPlan} {
		revisions := history[kind]
		highest := uint64(len(revisions))
		if highest >= uint64(^uint32(0)) {
			return false
		}
		for offset := uint32(1); offset <= uint32(len(revisions)); offset++ {
			revision := offset
			if !revisions[revision] {
				return false
			}
		}
		if revision := current[kind]; revision != 0 && uint64(revision) != highest+1 {
			return false
		}
	}
	return true
}

func completedOutcomeMatchesTask(t ProcessTask, evidence map[ID]EvidenceSummary) bool {
	if t.Requirements == nil || t.Design == nil || t.TaskPlan == nil || t.Implementation == nil || t.Test == nil || t.Comprehension == nil ||
		t.Outcome.RequirementsRevision != t.Requirements.Revision || t.Outcome.TestRecordID != t.Test.RecordID ||
		t.Outcome.ComprehensionRecordID != t.Comprehension.RecordID || !outcomeRepositoryDigestMatches(t) ||
		len(t.Test.UnverifiedItems) != 0 || len(t.Test.ManualHandoffItems) != 0 || len(t.Outcome.Acceptance) != len(t.Requirements.AcceptanceCriteria) {
		return false
	}
	if !AcceptanceLinksCurrent(t, t.Outcome.Acceptance) {
		return false
	}
	expectedAutomated := []ID{}
	expectedManual := []ID{}
	for _, id := range t.Test.EvidenceIDs {
		item, ok := evidence[id]
		if !ok || !TestEvidenceEligible(t.Test, item) {
			return false
		}
		if item.Status != EvidencePassed {
			continue
		}
		switch item.Source {
		case EvidenceSourceAutomated:
			expectedAutomated = append(expectedAutomated, id)
		case EvidenceSourceUser:
			expectedManual = append(expectedManual, id)
		case EvidenceSourceStatic, EvidenceSourceHostObserved:
		default:
			return false
		}
	}
	confirmation, ok := evidence[t.Comprehension.UserEvidenceID]
	if !ok || confirmation.Source != EvidenceSourceUser || confirmation.Status != EvidencePassed {
		return false
	}
	expectedManual = append(expectedManual, t.Comprehension.UserEvidenceID)
	return sameIDs(t.Outcome.AutomatedEvidenceIDs, expectedAutomated) && sameIDs(t.Outcome.ManualEvidenceIDs, expectedManual)
}

func outcomeRepositoryDigestMatches(t ProcessTask) bool {
	digest, err := t.EffectiveRepositoryBindingDigest()
	return err == nil && t.Outcome.FinalRepositoryDigest == digest
}

func sameIDs(actual, expected []ID) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func sameStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}

func baselineReferencePrecedesCurrent(ref BaselineReference, t ProcessTask) bool {
	switch ref.Kind {
	case BaselineRequirements:
		return t.Requirements == nil || ref.Revision < t.Requirements.Revision
	case BaselineDesign:
		return t.Design == nil || ref.Revision < t.Design.Revision
	case BaselineTaskPlan:
		return t.TaskPlan == nil || ref.Revision < t.TaskPlan.Revision
	default:
		return false
	}
}

func (t *ProcessTask) InvalidateForDestination(destination NodeID) {
	switch destination {
	case NodeRequirements:
		t.Design = nil
		t.TaskPlan = nil
		t.Implementation = nil
		t.Test = nil
		t.Comprehension = nil
	case NodeDesign:
		t.TaskPlan = nil
		t.Implementation = nil
		t.Test = nil
		t.Comprehension = nil
	case NodeTasks:
		if t.TaskPlan != nil {
			t.TaskPlan.Confirmation, t.TaskPlan.ConfirmedAt = nil, nil
		}
		t.Implementation = nil
		t.Test = nil
		t.Comprehension = nil
	case NodeImplement, NodeTest, NodeRefactor:
		t.Test = nil
		t.Comprehension = nil
	case NodeComprehensionReview:
		t.Comprehension = nil
	}
}
