package recovery

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func pendingScopeFixture() (domain.ProcessTask, RepositoryScopeObservation, domain.BlockerResolutionPayload) {
	base := recoveryBinding('a')
	for index := 0; index < 64; index++ {
		entry := recoveryEntry(fmt.Sprintf("carry-%02d.go", index), 'b')
		entry.BaseMode, entry.BaseContentDigest = "100644", recoveryDigest('a')
		base.TaskSurface = append(base.TaskSurface, entry)
	}
	resume := domain.NodeImplement
	task := domain.ProcessTask{
		CurrentNode: domain.NodeBlocked, ResumeNode: &resume, Repository: base,
		TaskPlan: &domain.TaskPlanBaseline{WorkItems: []domain.WorkItem{{ExpectedPaths: []string{"planned.go"}}}},
		Blocker: &domain.ProcessBlocker{BlockerID: "blocker", Cause: domain.BlockerCauseFileScopeDecision,
			Condition: domain.BlockerCondition{Kind: domain.BlockerConditionResolveFileScope, ScopeRequestID: "scope"}},
		FileScopeRecords: []domain.FileScopeRecord{{RequestID: "scope", Decision: domain.FileScopePending, Paths: []string{"extra.go"}}},
	}
	payload := domain.BlockerResolutionPayload{BlockerID: "blocker", Condition: task.Blocker.Condition,
		ObservedBindingDigest: base.BindingDigest,
		FileScopeDecision:     &domain.FileScopeDecisionInput{Choice: domain.FileScopeExpandScope, Reason: "Revise the exact file plan."}}
	return task, RepositoryScopeObservation{Primary: base.Clone()}, payload
}

func TestPendingScopeDecisionChecksCurrentLayersWithoutGrantingCarry(t *testing.T) {
	for _, choice := range []domain.FileScopeDecision{domain.FileScopeAllowOnce, domain.FileScopeExpandScope, domain.FileScopeReject} {
		t.Run(string(choice), func(t *testing.T) {
			task, observed, payload := pendingScopeFixture()
			payload.FileScopeDecision.Choice = choice
			comparison := RepositoryScopeComparison{Relation: RepositoryExact, ObservedDigest: observed.Primary.BindingDigest}
			if err := ValidateBlockerResolution(task, observed, comparison, payload); err != nil {
				t.Fatal(err)
			}
			if paths := task.UnexplainedChangedPaths(observed.Primary, nil); len(paths) != 64 {
				t.Fatalf("decision silently authorized carried paths: %v", paths)
			}
			task.FileScopeRecords[0].Observed = true
			if err := ValidateBlockerResolution(task, observed, comparison, payload); !errors.Is(err, domain.ErrRepositoryDrift) {
				t.Fatalf("observed scope was relaxed: %v", err)
			}
		})
	}
	for _, test := range []struct {
		name   string
		change func(*domain.RepositoryBinding)
	}{
		{"added", func(b *domain.RepositoryBinding) { b.TaskSurface = append(b.TaskSurface, recoveryEntry("new.go", 'c')) }},
		{"removed", func(b *domain.RepositoryBinding) { b.TaskSurface = b.TaskSurface[1:] }},
		{"effective-content", func(b *domain.RepositoryBinding) { b.TaskSurface[0].ContentDigest = recoveryDigest('c') }},
		{"base-content", func(b *domain.RepositoryBinding) { b.TaskSurface[0].BaseContentDigest = recoveryDigest('c') }},
		{"index-content-normalized-unchanged", func(b *domain.RepositoryBinding) { b.TaskSurface[0].IndexContentDigest = recoveryDigest('c') }},
		{"worktree-content", func(b *domain.RepositoryBinding) { b.TaskSurface[0].WorktreeContentDigest = recoveryDigest('c') }},
		{"base-mode", func(b *domain.RepositoryBinding) { b.TaskSurface[0].BaseMode = "100755" }},
		{"index-mode", func(b *domain.RepositoryBinding) { b.TaskSurface[0].IndexMode = "100755" }},
		{"worktree-mode", func(b *domain.RepositoryBinding) { b.TaskSurface[0].WorktreeMode = "100755" }},
		{"change-type", func(b *domain.RepositoryBinding) { b.TaskSurface[0].ChangeType = domain.RepositoryChangeDeleted }},
		{"head", func(b *domain.RepositoryBinding) { b.CurrentHead = "changed-head" }},
		{"branch", func(b *domain.RepositoryBinding) { branch := "other"; b.CurrentBranch = &branch }},
		{"instance", func(b *domain.RepositoryBinding) { b.WorktreeInstanceDigest = recoveryDigest('c') }},
	} {
		t.Run(test.name, func(t *testing.T) {
			task, observed, payload := pendingScopeFixture()
			test.change(&observed.Primary)
			comparison := RepositoryScopeComparison{Relation: RepositoryExact, ObservedDigest: observed.Primary.BindingDigest}
			if err := ValidateBlockerResolution(task, observed, comparison, payload); !errors.Is(err, domain.ErrRepositoryDrift) {
				t.Fatalf("unreviewed change accepted: %v", err)
			}
		})
	}
}

func TestPendingScopeDeltaUsesQualifiedPathsAndIncludesDeletion(t *testing.T) {
	task, observed, _ := pendingScopeFixture()
	task.PrimaryRepositoryKey = "core"
	task.TaskPlan.WorkItems[0].ExpectedPaths = []string{"core::planned.go"}
	task.AdditionalRepositories = []domain.RepositoryScopeEntry{{Key: "docs", Binding: task.Repository.Clone()}}
	observed.Additional = []domain.RepositoryScopeEntry{{Key: "docs", Binding: task.Repository.Clone()}}
	observed.Primary.TaskSurface = append(observed.Primary.TaskSurface, recoveryEntry("planned.go", 'c'))
	observed.Additional[0].Binding.TaskSurface = observed.Additional[0].Binding.TaskSurface[1:]
	paths := pendingWriteScopeDelta(task, observed)
	if len(paths) != 1 || paths[0] != "docs::carry-00.go" {
		t.Fatalf("scope delta=%v", paths)
	}
}
