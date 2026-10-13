package mcp

import (
	"github.com/Innocent-children/taskbelay/internal/application"
	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/execution"
)

type RepositorySummary struct {
	Key          domain.RepositoryKey `json:"key"`
	Path         string               `json:"repository_path"`
	Branch       string               `json:"branch"`
	ChangedPaths []string             `json:"changed_paths"`
}

type TaskSnapshot struct {
	TaskID         string                 `json:"task_id"`
	Host           domain.Host            `json:"host"`
	Status         string                 `json:"status"`
	Revision       uint64                 `json:"revision"`
	Authorization  domain.Authorization   `json:"authorization"`
	Plan           domain.Plan            `json:"plan"`
	Checks         []domain.Check         `json:"checks"`
	Repositories   []RepositorySummary    `json:"repositories"`
	Progress       []domain.Progress      `json:"progress"`
	Issue          *domain.Issue          `json:"issue"`
	Evidence       []domain.Evidence      `json:"evidence"`
	Usage          domain.Usage           `json:"usage"`
	Acceptance     []domain.Acceptance    `json:"acceptance"`
	Summary        string                 `json:"summary"`
	Risks          []string               `json:"risks"`
	Archived       bool                   `json:"archived"`
	BudgetDecision *domain.BudgetApproval `json:"budget_decision,omitempty"`
}

type TaskResult struct {
	Task                TaskSnapshot        `json:"task"`
	PlanDigest          domain.Digest       `json:"plan_digest"`
	AuthorizationDigest domain.Digest       `json:"authorization_digest"`
	ContentDigest       domain.Digest       `json:"content_digest"`
	Decision            domain.Decision     `json:"decision"`
	Refreshed           bool                `json:"refreshed"`
	Replayed            bool                `json:"replayed"`
	Budget              domain.BudgetReport `json:"budget"`
	Details             *application.View   `json:"details,omitempty"`
}

type RunSummary struct {
	RunID       string `json:"run_id"`
	CheckID     string `json:"check_id"`
	Status      string `json:"status"`
	ExitCode    *int   `json:"exit_code,omitempty"`
	DurationMS  int64  `json:"duration_ms"`
	OutputBytes int64  `json:"output_bytes"`
	Truncated   bool   `json:"truncated"`
}

type RunResult struct {
	View TaskResult `json:"view"`
	Run  RunSummary `json:"run"`
}

/**
 * Routine responses retain current requirements and results. Repository
 * fingerprints, historical attempts and full output are available on demand.
 */
func taskResult(v application.View, detail bool) TaskResult {
	t := v.Task
	repositories := []RepositorySummary{}
	for _, r := range t.Repositories {
		paths := []string{}
		for _, entry := range r.Binding.TaskSurface {
			paths = append(paths, t.QualifiedPath(r.Key, entry.Path))
		}
		repositories = append(repositories, RepositorySummary{Key: r.Key, Path: r.Origin.CanonicalWorktreeRoot, Branch: r.Origin.TaskBranch, ChangedPaths: paths})
	}
	latest := map[string]string{}
	keep := map[string]bool{}
	for _, e := range t.Evidence {
		latest[e.CheckID+":"+e.Source] = e.ID
	}
	for _, id := range latest {
		keep[id] = true
	}
	for _, f := range v.Decision.Evidence {
		if f.Applicable {
			keep[f.EvidenceID] = true
		}
	}
	evidence := []domain.Evidence{}
	for _, e := range t.Evidence {
		if keep[e.ID] {
			evidence = append(evidence, e)
		}
	}
	decision := v.Decision
	decision.Evidence = []domain.Freshness{}
	for _, f := range v.Decision.Evidence {
		if keep[f.EvidenceID] {
			decision.Evidence = append(decision.Evidence, f)
		}
	}
	result := TaskResult{
		Task:       TaskSnapshot{TaskID: t.TaskID, Host: t.Host, Status: t.Status, Revision: t.Revision, Authorization: t.Authorization, Plan: t.Plan, Checks: t.Checks, Repositories: repositories, Progress: t.Progress, Issue: t.Issue, Evidence: evidence, Usage: t.Usage, Acceptance: t.Acceptance, Summary: t.Summary, Risks: t.Risks, Archived: t.Archived, BudgetDecision: t.BudgetDecision},
		PlanDigest: v.PlanDigest, AuthorizationDigest: v.AuthorizationDigest, ContentDigest: v.ContentDigest, Decision: decision, Refreshed: v.Refreshed, Replayed: v.Replayed, Budget: v.Budget,
	}
	if detail {
		result.Details = &v
	}
	return result
}

func runResult(v application.RunResult) RunResult {
	r := v.Run
	return RunResult{View: taskResult(v.View, false), Run: summarizeRun(r)}
}

func summarizeRun(r execution.Run) RunSummary {
	return RunSummary{RunID: r.RunID, CheckID: r.CheckID, Status: r.Status, ExitCode: r.ExitCode, DurationMS: r.DurationMS, OutputBytes: r.OutputBytes, Truncated: r.Truncated}
}
