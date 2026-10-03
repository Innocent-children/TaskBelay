package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInvalidationMatrixRequirementsRevisionClearsAllDownstreamAuthority(t *testing.T) {
	task := invalidationMatrixTask(t)
	intent, repository := task.Intent, task.Repository
	evidence, history := append([]EvidenceSummary(nil), task.Evidence...), append([]BaselineReference(nil), task.BaselineHistory...)
	task.InvalidateForDestination(NodeRequirements)
	if task.Design != nil || task.TaskPlan != nil || task.Implementation != nil || task.Test != nil || task.Comprehension != nil {
		t.Fatal("requirements revision retained downstream authority")
	}
	if !reflect.DeepEqual(task.Intent, intent) || !reflect.DeepEqual(task.Repository, repository) || !reflect.DeepEqual(task.Evidence, evidence) || !reflect.DeepEqual(task.BaselineHistory, history) {
		t.Fatal("requirements revision changed immutable identity, claim authority, evidence, or history")
	}
}

func TestInvalidationMatrixDesignRevisionPreservesRequirementsAndClearsDependents(t *testing.T) {
	task := invalidationMatrixTask(t)
	requirements := task.Requirements
	task.InvalidateForDestination(NodeDesign)
	if task.Requirements != requirements || task.TaskPlan != nil || task.Implementation != nil || task.Test != nil || task.Comprehension != nil {
		t.Fatal("design revision invalidation matrix mismatch")
	}
}

func TestInvalidationMatrixTaskPlanRevisionPreservesRequirementsDesignAndClearsDependents(t *testing.T) {
	task := invalidationMatrixTask(t)
	requirements, design := task.Requirements, task.Design
	task.InvalidateForDestination(NodeTasks)
	if task.Requirements != requirements || task.Design != design || task.Implementation != nil || task.Test != nil || task.Comprehension != nil {
		t.Fatal("task-plan revision invalidation matrix mismatch")
	}
}

func TestInvalidationMatrixImplementationAndRefactorChangesClearTestComprehension(t *testing.T) {
	for _, destination := range []NodeID{NodeImplement, NodeTest, NodeRefactor} {
		t.Run(string(destination), func(t *testing.T) {
			task := invalidationMatrixTask(t)
			requirements, design, plan := task.Requirements, task.Design, task.TaskPlan
			task.InvalidateForDestination(destination)
			if task.Requirements != requirements || task.Design != design || task.TaskPlan != plan || task.Test != nil || task.Comprehension != nil {
				t.Fatal("implementation/refactor invalidation matrix mismatch")
			}
		})
	}
}

func TestInvalidationMatrixTestReplacementInvalidatesOldComprehension(t *testing.T) {
	task := invalidationMatrixTask(t)
	oldTest := task.Test
	task.InvalidateForDestination(NodeComprehensionReview)
	if task.Test != oldTest || task.Comprehension != nil {
		t.Fatal("new TEST authority did not preserve the new test and invalidate old comprehension")
	}
}

func TestInvalidationMatrixComprehensionFailureCreatesNoPassingAuthority(t *testing.T) {
	for _, destination := range []NodeID{NodeImplement, NodeRefactor, NodeDesign, NodeTest, NodeRequirements} {
		t.Run(string(destination), func(t *testing.T) {
			task := invalidationMatrixTask(t)
			task.Comprehension = nil
			task.Outcome = nil
			task.InvalidateForDestination(destination)
			if task.Comprehension != nil || task.Outcome != nil {
				t.Fatal("failed comprehension created passing or delivery authority")
			}
		})
	}
}

func TestInvalidationBaselineHistoryMonotonicRevisionBounds(t *testing.T) {
	valid := invalidationMatrixTask(t)
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid revision chain: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ProcessTask)
	}{
		{"requirements revision skips one", func(task *ProcessTask) {
			task.Requirements.Revision = 3
			task.Design, task.TaskPlan, task.Implementation, task.Test, task.Comprehension = nil, nil, nil, nil, nil
		}},
		{"design revision starts above one", func(task *ProcessTask) {
			task.Design.Revision = 2
			task.TaskPlan, task.Implementation, task.Test, task.Comprehension = nil, nil, nil, nil
		}},
		{"task-plan revision starts above one", func(task *ProcessTask) {
			task.TaskPlan.Revision = 2
			task.Implementation, task.Test, task.Comprehension = nil, nil, nil
		}},
		{"history repeats current requirements", func(task *ProcessTask) { task.BaselineHistory[0].Revision = task.Requirements.Revision }},
		{"history contains future requirements", func(task *ProcessTask) { task.BaselineHistory[0].Revision = task.Requirements.Revision + 1 }},
		{"history repeats revision", func(task *ProcessTask) { task.BaselineHistory = append(task.BaselineHistory, task.BaselineHistory[0]) }},
		{"history omits first revision", func(task *ProcessTask) {
			task.BaselineHistory[0].Revision = 2
			task.Requirements.Revision = 3
			task.Design, task.TaskPlan, task.Implementation, task.Test, task.Comprehension = nil, nil, nil, nil, nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := invalidationMatrixTask(t)
			tc.mutate(&task)
			if err := task.Validate(); err == nil {
				t.Fatal("invalid revision/history chain accepted")
			}
		})
	}
}

func TestInvalidationBaselineHistoryFieldsAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*BaselineReference)
	}{
		{"kind", func(ref *BaselineReference) { ref.Kind = "future" }},
		{"revision", func(ref *BaselineReference) { ref.Revision = 0 }},
		{"digest", func(ref *BaselineReference) { ref.Digest = "bad" }},
		{"summary", func(ref *BaselineReference) { ref.Summary = "" }},
		{"time", func(ref *BaselineReference) { ref.CreatedAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := invalidationMatrixTask(t)
			tc.mutate(&task.BaselineHistory[0])
			if err := task.Validate(); err == nil {
				t.Fatal("invalid history reference accepted")
			}
		})
	}

	task := validProcessTaskForDomainTest(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), matrixDigest('a'))
	for i := 1; i <= 600; i++ {
		task.BaselineHistory = append(task.BaselineHistory, BaselineReference{Kind: BaselineRequirements, Revision: uint32(i), Digest: matrixDigest('a'), Summary: strings.Repeat("r", MaxEvidenceSummaryBytes), CreatedAt: task.CreatedAt})
	}
	if size, err := compactJSONSize(task); err != nil || size <= MaxPersistedTaskSnapshotBytes {
		t.Fatalf("large history fixture bytes=%d err=%v", size, err)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("complete history rejected: %v", err)
	}

	oversized := invalidationMatrixTask(t)
	oversized.Intent.Request = strings.Repeat("r", MaxPersistedTaskSnapshotBytes)
	if err := oversized.Validate(); err == nil {
		t.Fatal("aggregate above persisted snapshot limit accepted")
	}
}

func TestInvalidationAggregateRejectsEveryStaleDownstreamAuthority(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ProcessTask)
	}{
		{"design", func(task *ProcessTask) { task.Design.RequirementsRevision++ }},
		{"task plan", func(task *ProcessTask) { task.TaskPlan.DesignRevision++ }},
		{"implementation revision", func(task *ProcessTask) { task.Implementation.TaskPlanRevision++ }},
		{"test requirements", func(task *ProcessTask) { task.Test.RequirementsRevision++ }},
		{"test design", func(task *ProcessTask) { task.Test.DesignRevision++ }},
		{"test task plan", func(task *ProcessTask) { task.Test.TaskPlanRevision++ }},
		{"test repository", func(task *ProcessTask) { task.Test.ContentDigest = matrixDigest('f') }},
		{"comprehension requirements", func(task *ProcessTask) { task.Comprehension.RequirementsRevision++ }},
		{"comprehension design", func(task *ProcessTask) { task.Comprehension.DesignRevision++ }},
		{"comprehension task plan", func(task *ProcessTask) { task.Comprehension.TaskPlanRevision++ }},
		{"comprehension repository", func(task *ProcessTask) { task.Comprehension.ContentDigest = matrixDigest('f') }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := invalidationMatrixTask(t)
			tc.mutate(&task)
			if err := task.Validate(); err == nil {
				t.Fatal("stale authority accepted")
			}
		})
	}
}

func invalidationMatrixTask(t *testing.T) ProcessTask {
	t.Helper()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	digest := matrixDigest('a')
	task := validProcessTaskForDomainTest(now, digest)
	task.Requirements = &RequirementsBaseline{Revision: 2, Digest: digest, Goal: "Current goal", AcceptanceCriteria: []string{"accepted"}, CreatedAt: now}
	task.Design = &DesignBaseline{Revision: 1, Digest: digest, RequirementsRevision: 2, Approach: "Direct design", Decisions: []string{"Reuse the boundary"}, CreatedAt: now}
	task.TaskPlan = &TaskPlanBaseline{Revision: 1, Digest: digest, DesignRevision: 1, WorkItems: []WorkItem{{WorkItemID: "work", Summary: "Implement work", ExpectedPaths: []string{"internal/work.go"}, AcceptanceIndexes: []uint32{0}, VerificationSteps: []string{"Run targeted tests"}}}, VerificationPlan: validDomainVerificationPlan(), CreatedAt: now}
	task.TaskPlan.Confirmation = &PlanConfirmation{Source: EvidenceSourceUser, Status: EvidencePassed, Summary: "Fixture user approved the complete plan.", RequirementsDigest: task.Requirements.Digest, DesignDigest: task.Design.Digest, TaskPlanDigest: task.TaskPlan.Digest, TaskPlanRevision: task.TaskPlan.Revision}
	task.TaskPlan.ConfirmedAt = &now
	task.Implementation = &ImplementationRecord{Revision: 1, TaskPlanRevision: 1, ContentDigest: digest, CompletedWorkItemIDs: []ID{"work"}, Summary: "Implemented work", CreatedAt: now}
	automated := EvidenceSummary{EvidenceID: "automated", TaskPlanRevision: 1, Source: EvidenceSourceAutomated, Name: "targeted", Status: EvidencePassed, Summary: "Targeted tests passed", Digest: digest, CommandCount: 1, RecordedAt: now}
	user := EvidenceSummary{EvidenceID: "user", TaskPlanRevision: 1, Source: EvidenceSourceUser, Name: "confirmation", Status: EvidencePassed, Summary: "User confirmed understanding", Digest: digest, RecordedAt: now}
	task.Evidence = []EvidenceSummary{automated, user}
	task.Test = &TestRecord{RecordID: "test", RequirementsRevision: 2, DesignRevision: 1, TaskPlanRevision: 1, ContentDigest: digest, EvidenceIDs: []ID{"automated"}, CompletedAt: now}
	task.Comprehension = &ComprehensionAssessment{RecordID: "comprehension", TestRecordID: "test", RequirementsRevision: 2, DesignRevision: 1, TaskPlanRevision: 1, ContentDigest: digest, ExplainedComponents: []string{"component"}, UserEvidenceID: "user", ConfirmedAt: now}
	task.CurrentNode = NodeDelivery
	task.CurrentAction.Kind = ActionCompleteDelivery
	task.CurrentAction.NodeID = NodeDelivery
	task.CurrentAction.PayloadContract = "delivery-result"
	task.CurrentAction.NodeContract = NodeContractProjection{Purpose: "Reconcile delivery.", EntryConditions: []string{"authorities current"}, CompletionConditions: []string{"delivery complete"}}
	task.CurrentAction.SemanticMethodSteps = []SemanticMethodStep{{StepID: "delivery.reconcile_acceptance", Purpose: "Reconcile acceptance.", Required: true}}
	task.CurrentAction.AvailableTransitions = nil
	task.BaselineHistory = []BaselineReference{{Kind: BaselineRequirements, Revision: 1, Digest: digest, Summary: "Original goal", CreatedAt: now}}
	return task
}

func matrixDigest(value byte) Digest { return Digest(strings.Repeat(string(value), 64)) }
