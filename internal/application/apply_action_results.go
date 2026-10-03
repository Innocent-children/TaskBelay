package application

import (
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
)

func applyRequirementsResult(task *domain.ProcessTask, envelope workflow.StandardPayload, result *workflow.RequirementsResult, now time.Time) error {
	if envelope.TransitionID != "requirements_ready" || result.Baseline == nil {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "REQUIREMENTS accepts requirements_ready with a non-null baseline.")
	}
	revision := nextBaselineRevision(task, domain.BaselineRequirements)
	digest, err := requirementsDigest(*result.Baseline, envelope.Artifacts)
	if err != nil {
		return err
	}
	baseline := domain.RequirementsBaseline{Revision: revision, Digest: digest, Goal: result.Baseline.Goal, Scope: result.Baseline.Scope, OutOfScope: result.Baseline.OutOfScope, AcceptanceCriteria: result.Baseline.AcceptanceCriteria, Constraints: result.Baseline.Constraints, Assumptions: result.Baseline.Assumptions, ArtifactRefs: envelope.Artifacts, CreatedAt: now}
	if err := baseline.Validate(); err != nil {
		return domain.AtField("payload.node_result.baseline", err)
	}
	if task.Requirements != nil {
		if err := appendBaselineHistory(task, domain.BaselineReference{Kind: domain.BaselineRequirements, Revision: task.Requirements.Revision, Digest: task.Requirements.Digest, Summary: task.Requirements.Goal, CreatedAt: task.Requirements.CreatedAt}); err != nil {
			return err
		}
	}
	if err := invalidateForDestination(task, domain.NodeRequirements); err != nil {
		return err
	}
	task.Requirements = &baseline
	return nil
}

func applyDesignResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.DesignResult, now time.Time) error {
	if transition.TransitionID != "design_ready" {
		if len(result.Findings) == 0 {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A DESIGN problem transition requires non-empty findings.")
		}
		return invalidateForDestination(task, transition.Destination)
	}
	if task.Requirements == nil || result.Baseline == nil || result.Baseline.RequirementsRevision != task.Requirements.Revision {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The design baseline must reference the current saved requirements revision.")
	}
	revision := nextBaselineRevision(task, domain.BaselineDesign)
	digest, err := designDigest(*result.Baseline, envelope.Artifacts)
	if err != nil {
		return err
	}
	baseline := domain.DesignBaseline{Revision: revision, Digest: digest, RequirementsRevision: result.Baseline.RequirementsRevision, Approach: result.Baseline.Approach, Components: result.Baseline.Components, Decisions: result.Baseline.Decisions, RejectedAlternatives: result.Baseline.RejectedAlternatives, ComplexityJustification: result.Baseline.ComplexityJustification, Risks: result.Baseline.Risks, ArtifactRefs: envelope.Artifacts, CreatedAt: now}
	if err := baseline.Validate(); err != nil {
		return domain.AtField("payload.node_result.baseline", err)
	}
	if task.Design != nil {
		if err := appendBaselineHistory(task, domain.BaselineReference{Kind: domain.BaselineDesign, Revision: task.Design.Revision, Digest: task.Design.Digest, Summary: task.Design.Approach, CreatedAt: task.Design.CreatedAt}); err != nil {
			return err
		}
	}
	if err := invalidateForDestination(task, domain.NodeDesign); err != nil {
		return err
	}
	task.Design = &baseline
	return nil
}

func applyTaskPlanResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.TasksResult, now time.Time) error {
	if transition.TransitionID == "tasks_ready" {
		if value := result.UserConfirmation; value == nil || !value.Matches(task.Requirements, task.Design, task.TaskPlan) {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "tasks_ready requires explicit user confirmation of the current requirements, design and saved Task Plan.")
		}
		confirmation := *result.UserConfirmation
		task.TaskPlan.Confirmation, task.TaskPlan.ConfirmedAt = &confirmation, &now
		return nil
	}
	if transition.TransitionID != "tasks_plan_saved" {
		if len(result.Findings) == 0 {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A TASKS problem transition requires non-empty findings.")
		}
		return invalidateForDestination(task, transition.Destination)
	}
	if task.Requirements == nil || task.Design == nil || result.Baseline == nil || result.Baseline.DesignRevision != task.Design.Revision {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task Plan requires current requirements and design records, and must reference the current design revision.")
	}
	revision := nextBaselineRevision(task, domain.BaselineTaskPlan)
	digest, err := taskPlanDigest(*result.Baseline, envelope.Artifacts)
	if err != nil {
		return err
	}
	baseline := domain.TaskPlanBaseline{Revision: revision, Digest: digest, DesignRevision: result.Baseline.DesignRevision, WorkItems: result.Baseline.WorkItems, VerificationPlan: result.Baseline.VerificationPlan, ArtifactRefs: envelope.Artifacts, CreatedAt: now}
	if err := baseline.Validate(); err != nil {
		return domain.AtField("payload.node_result.baseline", err)
	}
	if !acceptanceCovered(baseline.WorkItems, len(task.Requirements.AcceptanceCriteria)) {
		return domain.InvalidArgumentViolations(domain.Violation("payload.node_result.baseline.work_items", domain.RuleAcceptanceCoverageRequired))
	}
	for _, item := range baseline.WorkItems {
		if len(item.VerificationSteps) == 0 {
			return domain.WithExplanation(domain.ErrInvalidArgument, "Every work item must list at least one verification step.")
		}
		for _, index := range item.AcceptanceIndexes {
			if int(index) >= len(task.Requirements.AcceptanceCriteria) {
				return domain.WithExplanation(domain.ErrInvalidArgument, "Every acceptance index must refer to a current requirements acceptance criterion.")
			}
		}
	}
	if task.TaskPlan != nil {
		if err := appendBaselineHistory(task, taskPlanReference(*task.TaskPlan)); err != nil {
			return err
		}
	}
	task.Implementation, task.Test, task.Comprehension = nil, nil, nil
	task.TaskPlan = &baseline
	return nil
}

func applyImplementationResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.ImplementationResult, actionPaths []string, now time.Time) error {
	if task.TaskPlan == nil || result.TaskPlanRevision != task.TaskPlan.Revision {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The implementation result must reference the current Task Plan revision.")
	}
	known := make(map[domain.ID]bool, len(task.TaskPlan.WorkItems))
	for _, item := range task.TaskPlan.WorkItems {
		known[item.WorkItemID] = true
	}
	for _, id := range result.CompletedWorkItemIDs {
		if !known[id] {
			return domain.WithExplanation(domain.ErrInvalidArgument, "A completed work-item identifier is absent from the current Task Plan.")
		}
	}
	if transition.TransitionID == "implementation_ready_for_test" {
		if len(result.Findings) != 0 || !domain.CompletedWorkItemsCoverPlan(task.TaskPlan, result.CompletedWorkItemIDs) {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Entering TEST requires every planned work item to be completed and findings to be empty.")
		}
	} else if len(result.Findings) == 0 {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "An implementation problem transition requires non-empty findings.")
	}
	revision := uint32(1)
	if task.Implementation != nil {
		revision = task.Implementation.Revision + 1
	}
	workspace, err := task.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	record := domain.ImplementationRecord{Revision: revision, TaskPlanRevision: result.TaskPlanRevision, ContentDigest: workspace.Content, CompletedWorkItemIDs: result.CompletedWorkItemIDs, ActionChangedPaths: append([]string(nil), actionPaths...), Deviations: result.Deviations, Summary: envelope.Summary, CreatedAt: now}
	if record.Validate() != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The implementation record contains invalid completion identifiers, changed paths, deviations or record metadata.")
	}
	task.Implementation = &record
	return invalidateForDestination(task, transition.Destination)
}

func (s *Service) applyTestResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.TestResult, now time.Time) error {
	workspace, err := task.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	if task.Requirements == nil || task.Design == nil || task.TaskPlan == nil || task.Implementation == nil || task.Implementation.TaskPlanRevision != task.TaskPlan.Revision {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "TEST requires current requirements, design, plan and an implementation record bound to that plan.")
	}
	if transition.TransitionID == "verification_budget_increased" {
		return applyVerificationBudgetAdjustment(task, envelope.Reason, result.BudgetAdjustment, now)
	}
	if result.BudgetAdjustment != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "budget_adjustment is only accepted by verification_budget_increased.")
	}
	budget, ok := task.CurrentVerificationBudget()
	if !ok {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "No current verification budget is available for this Task Plan.")
	}
	if err := workflow.EvaluateVerificationBudget(budget, task.TaskPlan.Revision, task.Evidence, result.Checks, result.ManualHandoffItems); err != nil {
		return err
	}
	if a := result.KnownFailureAcceptance; a != nil && (a.ContentDigest != workspace.Content || a.TaskPlanRevision != task.TaskPlan.Revision) {
		return domain.TransitionGuardFailure(transition.Guard, domain.GuardViolation("payload.node_result.known_failure_acceptance", domain.GuardUserConfirmationRequired))
	}
	completed := transition.Destination == domain.NodeComprehensionReview
	if completed {
		if len(result.Checks) == 0 || (result.KnownFailureAcceptance == nil && len(result.FailedItems) != 0) || len(result.Findings) != 0 {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Completing TEST requires checks, empty findings and either no failed items or an accepted known-failure record.")
		}
		for _, check := range result.Checks {
			if check.Status != domain.EvidencePassed && result.KnownFailureAcceptance == nil {
				return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A non-passing check cannot complete TEST without a valid known-failure acceptance.")
			}
		}
	} else if !testFailureFactsPresent(result) {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A TEST failure transition requires a failed check, failed item or finding.")
	}
	evidence, err := s.buildEvidence(task.TaskPlan.Revision, result.Checks, now)
	if err != nil {
		return err
	}
	task.Evidence = append(task.Evidence, evidence...)
	if err := recordVerificationAttempt(task, transition, result, evidence, now); err != nil {
		return err
	}
	if !completed {
		task.Test = nil
		return invalidateForDestination(task, transition.Destination)
	}
	recordID, err := s.id("test")
	if err != nil {
		return err
	}
	ids := make([]domain.ID, len(evidence))
	for i := range evidence {
		ids[i] = evidence[i].EvidenceID
	}
	task.Test = &domain.TestRecord{RecordID: recordID, RequirementsRevision: task.Requirements.Revision, DesignRevision: task.Design.Revision, TaskPlanRevision: task.TaskPlan.Revision, ContentDigest: workspace.Content, EvidenceIDs: ids, UnverifiedItems: result.UnverifiedItems, ManualHandoffItems: result.ManualHandoffItems, CompletedAt: now, KnownFailureAcceptance: result.KnownFailureAcceptance}
	task.Comprehension = nil
	return nil
}

func applyVerificationBudgetAdjustment(task *domain.ProcessTask, reason string, input *workflow.VerificationBudgetAdjustmentInput, now time.Time) error {
	if task.TaskPlan == nil || input == nil || input.Validate() != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "A budget adjustment requires a saved Task Plan and a valid adjustment with explained checks.")
	}
	previous, ok := task.CurrentVerificationBudget()
	if !ok || previous.MaxAutomaticCommands > domain.MaxTotalAutomaticVerificationCommands-input.AdditionalAutomaticCommands {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The current budget is missing or the requested increase exceeds the total automatic command limit.")
	}
	// The adjustment records why existing or new checks need more capacity.
	// Input validation owns name uniqueness within this adjustment.
	current := previous
	current.MaxAutomaticCommands += input.AdditionalAutomaticCommands
	if input.AllowFullSuite {
		current.AllowFullSuite = true
	}
	if input.AllowManualHandoff {
		current.AllowManualHandoff = true
	}
	if current == previous {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The budget adjustment does not increase capacity or enable a new permission.")
	}
	record := domain.VerificationBudgetAdjustment{
		Revision: uint32(len(task.VerificationBudgetAdjustments) + 1), TaskPlanRevision: task.TaskPlan.Revision,
		Basis: input.Basis, Reason: reason, AdditionalChecks: append([]domain.VerificationPlanCheck(nil), input.AdditionalChecks...),
		AdditionalAutomaticCommands: input.AdditionalAutomaticCommands, AllowFullSuite: input.AllowFullSuite,
		AllowManualHandoff: input.AllowManualHandoff, PreviousBudget: previous, CurrentBudget: current, CreatedAt: now,
	}
	if record.Validate() != nil || len(task.VerificationBudgetAdjustments) >= domain.MaxVerificationBudgetAdjustments {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The budget adjustment record is invalid or the Task has reached its retained adjustment limit.")
	}
	task.VerificationBudgetAdjustments = append(task.VerificationBudgetAdjustments, record)
	return nil
}

func testFailureFactsPresent(result *workflow.TestResult) bool {
	if len(result.FailedItems) != 0 || len(result.Findings) != 0 {
		return true
	}
	for _, check := range result.Checks {
		if check.Status == domain.EvidenceFailed {
			return true
		}
	}
	return false
}

func (s *Service) applyComprehensionResult(task *domain.ProcessTask, transition domain.TransitionDefinition, result *workflow.ComprehensionResult, now time.Time) error {
	if task.Requirements == nil || task.Design == nil || task.TaskPlan == nil || task.Implementation == nil || task.Test == nil {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Comprehension review requires saved requirements, design, plan, implementation and Test records.")
	}
	if transition.TransitionID != "comprehension_passed" {
		if result.UserConfirmation != nil || !comprehensionFailureFactsPresent(transition.TransitionID, result) {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A comprehension problem transition requires matching failure facts and must omit user_confirmation.")
		}
		task.Comprehension = nil
		return invalidateForDestination(task, transition.Destination)
	}
	confirmation := result.UserConfirmation
	if confirmation == nil || confirmation.Source != domain.EvidenceSourceUser || confirmation.Status != domain.EvidencePassed || len(result.ExplainedComponents) == 0 || len(result.UnresolvedQuestions) != 0 || len(result.UnnecessaryAbstractions) != 0 || len(result.Findings) != 0 {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "comprehension_passed requires passed user confirmation, explained components and no unresolved questions, unnecessary abstractions or findings.")
	}
	input := workflow.EvidenceInput{Source: confirmation.Source, Name: "comprehension_confirmation", Status: confirmation.Status, Summary: confirmation.Summary}
	if err := workflow.ValidateComprehensionConfirmation(task.Evidence, input); err != nil {
		return err
	}
	evidence, err := s.buildEvidence(task.TaskPlan.Revision, []workflow.EvidenceInput{input}, now)
	if err != nil {
		return err
	}
	recordID, err := s.id("comprehension")
	if err != nil {
		return err
	}
	workspace, err := task.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	task.Evidence = append(task.Evidence, evidence[0])
	task.Comprehension = &domain.ComprehensionAssessment{RecordID: recordID, TestRecordID: task.Test.RecordID, RequirementsRevision: task.Requirements.Revision, DesignRevision: task.Design.Revision, TaskPlanRevision: task.TaskPlan.Revision, ContentDigest: workspace.Content, ExplainedComponents: result.ExplainedComponents, MaintenanceRisks: result.MaintenanceRisks, UserEvidenceID: evidence[0].EvidenceID, ConfirmedAt: now}
	return nil
}

func comprehensionFailureFactsPresent(transition domain.TransitionID, result *workflow.ComprehensionResult) bool {
	switch transition {
	case "code_too_complex", "design_too_complex":
		return len(result.UnnecessaryAbstractions) != 0 || len(result.Findings) != 0 || len(result.MaintenanceRisks) != 0
	case "evidence_insufficient", "requirement_unclear":
		return len(result.UnresolvedQuestions) != 0 || len(result.Findings) != 0
	case "implementation_defect":
		return len(result.Findings) != 0
	default:
		return false
	}
}

func applyRefactorResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.RefactorResult, actionPaths []string, now time.Time) error {
	if task.Requirements == nil || task.Design == nil || task.TaskPlan == nil || task.Implementation == nil {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Refactoring requires saved requirements, design, plan and implementation records.")
	}
	if transition.TransitionID != "refactor_ready_for_test" {
		if len(result.Findings) == 0 {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A refactor problem transition requires non-empty findings.")
		}
		return invalidateForDestination(task, transition.Destination)
	}
	if len(result.Simplifications) == 0 || result.BehaviorChangeIntended || len(result.Findings) != 0 || !domain.CompletedWorkItemsCoverPlan(task.TaskPlan, task.Implementation.CompletedWorkItemIDs) {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Returning to TEST requires simplifications, complete work items, unchanged intended behavior and empty findings.")
	}
	previous := task.Implementation
	workspace, err := task.EffectiveWorkspaceDigests()
	if err != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task repository scope is invalid and cannot produce workspace digests.")
	}
	record := domain.ImplementationRecord{Revision: previous.Revision + 1, TaskPlanRevision: task.TaskPlan.Revision, ContentDigest: workspace.Content, CompletedWorkItemIDs: previous.CompletedWorkItemIDs, ActionChangedPaths: append([]string(nil), actionPaths...), Deviations: previous.Deviations, Summary: envelope.Summary, CreatedAt: now}
	if record.Validate() != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The refactored implementation record has invalid completion identifiers, paths, deviations or record metadata.")
	}
	task.Implementation = &record
	return invalidateForDestination(task, domain.NodeTest)
}

func applyDeliveryResult(task *domain.ProcessTask, transition domain.TransitionDefinition, envelope workflow.StandardPayload, result *workflow.DeliveryResult, now time.Time) error {
	if transition.TransitionID != "delivery_complete" {
		if len(result.Findings) == 0 || len(result.Acceptance) != 0 || len(result.AutomatedEvidenceIDs) != 0 || len(result.ManualEvidenceIDs) != 0 || result.TestRecordID != "" || result.ComprehensionRecordID != "" {
			return domain.WithExplanation(domain.ErrTransitionNotAllowed, "A delivery problem transition requires findings and must omit acceptance, evidence and completed-record references.")
		}
		return invalidateForDestination(task, transition.Destination)
	}
	if task.Requirements == nil || task.Design == nil || task.TaskPlan == nil || task.Implementation == nil || task.Test == nil || task.Comprehension == nil || len(task.Test.UnverifiedItems) != 0 || len(task.Test.ManualHandoffItems) != 0 || len(result.UnverifiedItems) != 0 || len(result.Findings) != 0 || result.TestRecordID != task.Test.RecordID || result.ComprehensionRecordID != task.Comprehension.RecordID || len(result.Acceptance) != len(task.Requirements.AcceptanceCriteria) {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Delivery requires all current records, matching Test and comprehension identities, complete acceptance coverage and no pending checks or findings.")
	}
	if !domain.AcceptanceLinksCurrent(*task, result.Acceptance) {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Delivery acceptance does not link the current requirements to completed matching work items and eligible passed Test evidence.")
	}
	if !deliveryEvidenceCurrent(task, result.AutomatedEvidenceIDs, result.ManualEvidenceIDs) {
		return domain.WithExplanation(domain.ErrTransitionNotAllowed, "Delivery evidence does not match the current Test results and passed user comprehension confirmation.")
	}
	effectiveDigest, err := task.EffectiveRepositoryBindingDigest()
	if err != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The Task repository bindings are invalid and cannot produce a binding digest.")
	}
	outcome := domain.ProcessOutcome{Status: domain.TerminalCompleted, Summary: envelope.Summary, RequirementsRevision: task.Requirements.Revision, Acceptance: result.Acceptance, TestRecordID: result.TestRecordID, ComprehensionRecordID: result.ComprehensionRecordID, AutomatedEvidenceIDs: result.AutomatedEvidenceIDs, ManualEvidenceIDs: result.ManualEvidenceIDs, FinalRepositoryDigest: effectiveDigest, Risks: result.Risks, CompletedAt: now}
	if outcome.Validate() != nil {
		return domain.WithExplanation(domain.ErrInvalidArgument, "The completed outcome has invalid record references, acceptance, risks or completion metadata.")
	}
	task.Outcome, task.CompletedAt = &outcome, &now
	return nil
}

func deliveryEvidenceCurrent(task *domain.ProcessTask, automated, manual []domain.ID) bool {
	byID := map[domain.ID]domain.EvidenceSummary{}
	for _, item := range task.Evidence {
		byID[item.EvidenceID] = item
	}
	expectedAutomated := []domain.ID{}
	expectedManual := []domain.ID{}
	for _, id := range task.Test.EvidenceIDs {
		item, ok := byID[id]
		if !ok || !domain.TestEvidenceEligible(task.Test, item) {
			return false
		}
		if item.Status != domain.EvidencePassed {
			continue
		}
		switch item.Source {
		case domain.EvidenceSourceAutomated:
			expectedAutomated = append(expectedAutomated, id)
		case domain.EvidenceSourceUser:
			expectedManual = append(expectedManual, id)
		case domain.EvidenceSourceStatic, domain.EvidenceSourceHostObserved:
		default:
			return false
		}
	}
	confirmation, ok := byID[task.Comprehension.UserEvidenceID]
	if !ok || confirmation.Source != domain.EvidenceSourceUser || confirmation.Status != domain.EvidencePassed {
		return false
	}
	expectedManual = append(expectedManual, task.Comprehension.UserEvidenceID)
	return sameEvidenceIDs(automated, expectedAutomated) && sameEvidenceIDs(manual, expectedManual)
}

func sameEvidenceIDs(actual, expected []domain.ID) bool {
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

func (s *Service) buildEvidence(taskPlanRevision uint32, inputs []workflow.EvidenceInput, now time.Time) ([]domain.EvidenceSummary, error) {
	items := make([]domain.EvidenceSummary, len(inputs))
	for i, input := range inputs {
		id, err := s.id("evidence")
		if err != nil {
			return nil, err
		}
		digest, err := digestCanonical(input)
		if err != nil {
			return nil, domain.WithExplanation(domain.ErrInternal, "Core could not encode the check input for its evidence digest.")
		}
		items[i] = domain.EvidenceSummary{EvidenceID: id, TaskPlanRevision: taskPlanRevision, Source: input.Source, Name: input.Name, Status: input.Status, Summary: input.Summary, Digest: digest, CommandCount: input.CommandCount, FullSuite: input.FullSuite, FullSuiteReason: input.FullSuiteReason, RecordedAt: now}
		if items[i].Validate() != nil {
			return nil, domain.WithExplanation(domain.ErrInvalidArgument, "The check could not form a valid retained evidence record.")
		}
	}
	return items, nil
}

func invalidateForDestination(task *domain.ProcessTask, destination domain.NodeID) error {
	if destination == domain.NodeRequirements {
		if task.Design != nil {
			if err := appendBaselineHistory(task, domain.BaselineReference{Kind: domain.BaselineDesign, Revision: task.Design.Revision, Digest: task.Design.Digest, Summary: task.Design.Approach, CreatedAt: task.Design.CreatedAt}); err != nil {
				return err
			}
		}
		if task.TaskPlan != nil {
			if err := appendBaselineHistory(task, taskPlanReference(*task.TaskPlan)); err != nil {
				return err
			}
		}
	} else if destination == domain.NodeDesign && task.TaskPlan != nil {
		if err := appendBaselineHistory(task, taskPlanReference(*task.TaskPlan)); err != nil {
			return err
		}
	}
	task.InvalidateForDestination(destination)
	return nil
}

func taskPlanReference(plan domain.TaskPlanBaseline) domain.BaselineReference {
	return domain.BaselineReference{Kind: domain.BaselineTaskPlan, Revision: plan.Revision, Digest: plan.Digest, Summary: plan.WorkItems[0].Summary, CreatedAt: plan.CreatedAt}
}

func appendBaselineHistory(task *domain.ProcessTask, ref domain.BaselineReference) error {
	return task.AppendBaselineHistory(ref)
}

func nextBaselineRevision(task *domain.ProcessTask, kind domain.BaselineKind) uint32 {
	highest := uint32(0)
	for _, ref := range task.BaselineHistory {
		if ref.Kind == kind && ref.Revision > highest {
			highest = ref.Revision
		}
	}
	switch kind {
	case domain.BaselineRequirements:
		if task.Requirements != nil && task.Requirements.Revision > highest {
			highest = task.Requirements.Revision
		}
	case domain.BaselineDesign:
		if task.Design != nil && task.Design.Revision > highest {
			highest = task.Design.Revision
		}
	case domain.BaselineTaskPlan:
		if task.TaskPlan != nil && task.TaskPlan.Revision > highest {
			highest = task.TaskPlan.Revision
		}
	}
	return highest + 1
}

func acceptanceCovered(items []domain.WorkItem, count int) bool {
	covered := make([]bool, count)
	for _, item := range items {
		for _, index := range item.AcceptanceIndexes {
			if int(index) < count {
				covered[index] = true
			}
		}
	}
	for _, value := range covered {
		if !value {
			return false
		}
	}
	return count > 0
}

func requirementsDigest(input workflow.RequirementsBaselineInput, artifacts []domain.ArtifactReference) (domain.Digest, error) {
	return digestCanonical(struct {
		Goal               string                     `json:"goal"`
		Scope              []string                   `json:"scope"`
		OutOfScope         []string                   `json:"out_of_scope"`
		AcceptanceCriteria []string                   `json:"acceptance_criteria"`
		Constraints        []string                   `json:"constraints"`
		Assumptions        []string                   `json:"assumptions"`
		ArtifactRefs       []domain.ArtifactReference `json:"artifact_refs"`
	}{input.Goal, input.Scope, input.OutOfScope, input.AcceptanceCriteria, input.Constraints, input.Assumptions, artifacts})
}

func designDigest(input workflow.DesignBaselineInput, artifacts []domain.ArtifactReference) (domain.Digest, error) {
	return digestCanonical(struct {
		RequirementsRevision    uint32                     `json:"requirements_revision"`
		Approach                string                     `json:"approach"`
		Components              []string                   `json:"components"`
		Decisions               []string                   `json:"decisions"`
		RejectedAlternatives    []string                   `json:"rejected_alternatives"`
		ComplexityJustification []string                   `json:"complexity_justification"`
		Risks                   []string                   `json:"risks"`
		ArtifactRefs            []domain.ArtifactReference `json:"artifact_refs"`
	}{input.RequirementsRevision, input.Approach, input.Components, input.Decisions, input.RejectedAlternatives, input.ComplexityJustification, input.Risks, artifacts})
}

func taskPlanDigest(input workflow.TasksBaselineInput, artifacts []domain.ArtifactReference) (domain.Digest, error) {
	return digestCanonical(struct {
		DesignRevision   uint32                     `json:"design_revision"`
		WorkItems        []domain.WorkItem          `json:"work_items"`
		VerificationPlan domain.VerificationPlan    `json:"verification_plan"`
		ArtifactRefs     []domain.ArtifactReference `json:"artifact_refs"`
	}{input.DesignRevision, input.WorkItems, input.VerificationPlan, artifacts})
}
