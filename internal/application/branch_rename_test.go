package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func renameGitTask(t *testing.T) (*Service, *store.SQLite, domain.ProcessTask, string, string) {
	t.Helper()
	root := fileScopeGitRepository(t)
	if err := os.WriteFile(filepath.Join(root, "tracked"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fileScopeRunGit(t, root, "add", "tracked")
	fileScopeRunGit(t, root, "commit", "-m", "tracked content")
	dbPath := filepath.Join(t.TempDir(), "tasks.db")
	db, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewService(db, repository.NewGitObserver())
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.OpenTask(context.Background(), OpenTaskRequest{RequestID: "open", Host: domain.HostCodex, RepositoryPath: root, WorkspaceOrigin: &WorkspaceOriginInput{Mode: domain.WorkspaceModeCurrentBranch, SourceType: "local", BaseBranch: "main", BaseCommit: fileScopeRunGit(t, root, "rev-parse", "HEAD"), TaskBranch: "main", ProvisioningReceiptID: "receipt"}, NewTask: &NewTaskInput{Request: "Pure branch rename", MethodProfile: domain.MethodPlain}})
	if err != nil {
		t.Fatal(err)
	}
	return s, db, result.Task, root, dbPath
}
func prepareRename(t *testing.T, s *Service, task domain.ProcessTask) domain.ProcessTask {
	t.Helper()
	result, err := s.PrepareTaskBranchRename(context.Background(), PrepareTaskBranchRenameRequest{RequestID: "prepare", Host: task.OriginHost, TaskID: task.TaskID, ExpectedRevision: task.Revision, RepositoryKey: task.EffectivePrimaryRepositoryKey(), TargetBranch: "task-renamed", Reason: "Correct the Task branch name."})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.CurrentNode != domain.NodeBlocked || result.Task.CurrentAction.ActionID == task.CurrentAction.ActionID {
		t.Fatal("rename did not replace the Action with BLOCKED")
	}
	return result.Task
}
func renameDecision(task domain.ProcessTask, choice string) RecoverActionRequest {
	return RecoverActionRequest{Host: task.OriginHost, TaskID: task.TaskID, ExpectedRevision: task.Revision, ActionID: task.CurrentAction.ActionID, RenameID: task.BranchRename.RenameID, RenameChoice: choice}
}
func TestBranchRenameCompleteRestartAndEffectiveBranch(t *testing.T) {
	s, db, original, root, dbPath := renameGitTask(t)
	ctx := context.Background()
	task := prepareRename(t, s, original)
	fileScopeRunGit(t, root, "branch", "-m", "main", "task-renamed")
	// A restart after the Host effect retains the original blocker and never repeats Git.
	db.Close()
	freshDB, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer freshDB.Close()
	s.taskStore = freshDB
	guarded, err := s.GetNextAction(ctx, GetNextActionRequest{Host: task.OriginHost, TaskID: task.TaskID})
	if err != nil || guarded.Action.ActionID != task.CurrentAction.ActionID {
		t.Fatalf("guard: %v", err)
	}
	resolved, err := s.ResolveBlockerAction(ctx, renameDecision(task, "complete"), "complete")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Task.WorkspaceOrigin != original.WorkspaceOrigin || *resolved.Task.Repository.CurrentBranch != "task-renamed" || resolved.Task.Repository.CurrentHead != original.Repository.CurrentHead || resolved.Task.CurrentNode != original.CurrentNode {
		t.Fatal("origin or facts were not preserved")
	}
	guarded, err = s.GetNextAction(ctx, GetNextActionRequest{Host: task.OriginHost, TaskID: task.TaskID})
	if err != nil || guarded.CurrentNode != original.CurrentNode {
		t.Fatalf("renamed branch treated as drift: %v", err)
	}
	if !relocationDestinationHistoryAllowed(resolved.Task.Repository, resolved.Task.Repository) {
		t.Fatal("relocation ignored effective branch")
	}
	freshDB.Close()
	reopened, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
}
func TestBranchRenameCancelAndRejectChangedFacts(t *testing.T) {
	for _, scenario := range []string{"cancel", "source-recreated", "same-head-switch", "content", "index", "head", "cancel-after-rename", "existing-target"} {
		t.Run(scenario, func(t *testing.T) {
			s, _, original, root, _ := renameGitTask(t)
			if scenario == "existing-target" {
				fileScopeRunGit(t, root, "branch", "task-renamed")
				_, err := s.PrepareTaskBranchRename(context.Background(), PrepareTaskBranchRenameRequest{RequestID: "prepare", Host: original.OriginHost, TaskID: original.TaskID, ExpectedRevision: original.Revision, RepositoryKey: original.EffectivePrimaryRepositoryKey(), TargetBranch: "task-renamed", Reason: "Correct name"})
				if err == nil {
					t.Fatal("existing target allowed")
				}
				return
			}
			task := prepareRename(t, s, original)
			choice := "complete"
			switch scenario {
			case "cancel":
				choice = "cancel"
			case "same-head-switch":
				fileScopeRunGit(t, root, "switch", "-c", "task-renamed")
			default:
				fileScopeRunGit(t, root, "branch", "-m", "main", "task-renamed")
				switch scenario {
				case "source-recreated":
					fileScopeRunGit(t, root, "branch", "main")
				case "content":
					if err := os.WriteFile(filepath.Join(root, "tracked"), []byte("changed\n"), 0600); err != nil {
						t.Fatal(err)
					}
				case "index":
					fileScopeRunGit(t, root, "update-index", "--assume-unchanged", "tracked")
				case "head":
					fileScopeRunGit(t, root, "commit", "--allow-empty", "-m", "extra commit")
				case "cancel-after-rename":
					choice = "cancel"
				}
			}
			result, err := s.ResolveBlockerAction(context.Background(), renameDecision(task, choice), "resolve")
			if scenario == "cancel" {
				if err != nil || result.Task.CurrentNode != original.CurrentNode || *result.Task.Repository.CurrentBranch != "main" {
					t.Fatalf("cancel: %v", err)
				}
			} else if err == nil {
				t.Fatal("changed facts accepted")
			}
		})
	}
}
func TestBranchRenamePendingCommitRecoveryAndOldAction(t *testing.T) {
	s, db, original, root, dbPath := renameGitTask(t)
	ctx := context.Background()
	task := prepareRename(t, s, original)
	old := actionSubmission(t, original, "old-action", "requirements_ready", requirementsNodeResult("Goal", []string{"criterion"}))
	if _, err := s.SubmitAction(ctx, old); !errors.Is(err, domain.ErrActionStale) {
		t.Fatalf("old Action: %v", err)
	}
	fileScopeRunGit(t, root, "branch", "-m", "main", "task-renamed")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_rename_event BEFORE INSERT ON task_events BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveBlockerAction(ctx, renameDecision(task, "complete"), "complete")
	if err == nil {
		t.Fatal("injected failure ignored")
	}
	saved, err := db.LoadTask(ctx, task.TaskID)
	if err != nil || saved.Revision != task.Revision {
		t.Fatalf("Task changed: %v", err)
	}
	operation, found, err := db.LoadActionOperation(ctx, task.TaskID)
	if err != nil || !found || operation.AppliedRevision != nil {
		t.Fatalf("missing pending operation: %v", err)
	}
	var unresolved int
	if err = raw.QueryRow(`SELECT COUNT(*) FROM branch_rename_operations WHERE resolved_revision IS NULL`).Scan(&unresolved); err != nil || unresolved != 1 {
		t.Fatal("audit did not roll back")
	}
	if _, err = raw.Exec(`DROP TRIGGER fail_rename_event`); err != nil {
		t.Fatal(err)
	}
	assessed, err := s.GetTask(ctx, GetTaskRequest{Host: task.OriginHost, TaskID: task.TaskID})
	if err != nil || assessed.RecoveryAssessment == nil || assessed.RecoveryAssessment.Classification != domain.RecoveryCompletedButUnrecorded {
		t.Fatalf("assessment: %+v %v", assessed.RecoveryAssessment, err)
	}
	recovered, err := s.RecoverAction(ctx, RecoverActionRequest{Host: task.OriginHost, TaskID: task.TaskID, ActionID: task.CurrentAction.ActionID})
	if err != nil || recovered.Task.CurrentNode != original.CurrentNode {
		t.Fatalf("recover: %v", err)
	}
	if _, err = s.RecoverAction(ctx, RecoverActionRequest{Host: task.OriginHost, TaskID: task.TaskID, ActionID: task.CurrentAction.ActionID}); err != nil {
		t.Fatal(err)
	}
}

func TestBranchRenameRejectsPendingOrdinaryOperation(t *testing.T) {
	s, db, task, _, dbPath := renameGitTask(t)
	ctx := context.Background()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_node_event BEFORE INSERT ON task_events BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	request := requirementsSubmission(t, task, "ordinary")
	if _, err = s.SubmitAction(ctx, request); !errors.Is(err, domain.ErrStorageUnavailable) {
		t.Fatalf("expected injected storage failure, got %v", err)
	}
	if _, found, err := db.LoadActionOperation(ctx, task.TaskID); err != nil || !found {
		t.Fatalf("no saved ordinary operation: %v", err)
	}
	if _, err = raw.Exec(`DROP TRIGGER fail_node_event`); err != nil {
		t.Fatal(err)
	}
	_, err = s.PrepareTaskBranchRename(ctx, PrepareTaskBranchRenameRequest{RequestID: "prepare", Host: task.OriginHost, TaskID: task.TaskID, ExpectedRevision: task.Revision, RepositoryKey: task.EffectivePrimaryRepositoryKey(), TargetBranch: "renamed", Reason: "Correct branch"})
	if !errors.Is(err, domain.ErrRecoveryUnavailable) {
		t.Fatalf("pending ordinary operation not protected: %v", err)
	}
	stored, err := db.LoadTask(ctx, task.TaskID)
	if err != nil || stored.Revision != task.Revision {
		t.Fatal("prepare changed pending Task")
	}
}

func TestBranchRenameAndOrdinarySubmissionCAS(t *testing.T) {
	s, db, task, _, _ := renameGitTask(t)
	ctx := context.Background()
	request := requirementsSubmission(t, task, "ordinary")
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := s.PrepareTaskBranchRename(ctx, PrepareTaskBranchRenameRequest{RequestID: "prepare", Host: task.OriginHost, TaskID: task.TaskID, ExpectedRevision: task.Revision, RepositoryKey: task.EffectivePrimaryRepositoryKey(), TargetBranch: "renamed", Reason: "Correct branch"})
		results <- err
	}()
	go func() { <-start; _, err := s.SubmitAction(ctx, request); results <- err }()
	close(start)
	a, b := <-results, <-results
	if (a == nil) == (b == nil) {
		t.Fatalf("expected one committed winner: %v; %v", a, b)
	}
	saved, err := db.LoadTask(ctx, task.TaskID)
	if err != nil || saved.Revision != task.Revision+1 {
		t.Fatalf("CAS: %v", err)
	}
}

func TestBranchRenameRejectsOtherRepositoryChange(t *testing.T) {
	s, _, _, root, _ := renameGitTask(t)
	// Use a new empty store and two unchanged real repositories, not a scope mutation.
	db := fileScopeDatabase(t)
	s.taskStore = db
	other := fileScopeGitRepository(t)
	origin := func(path string) *WorkspaceOriginInput {
		return &WorkspaceOriginInput{Mode: domain.WorkspaceModeCurrentBranch, SourceType: "local", BaseBranch: "main", BaseCommit: fileScopeRunGit(t, path, "rev-parse", "HEAD"), TaskBranch: "main", ProvisioningReceiptID: "receipt"}
	}
	opened, err := s.OpenTask(context.Background(), OpenTaskRequest{RequestID: "multi-open", Host: domain.HostCodex, RepositoryPath: root, PrimaryRepositoryKey: "primary", WorkspaceOrigin: origin(root), AdditionalRepositories: []AdditionalRepositoryInput{{Key: "other", RepositoryPath: other, WorkspaceOrigin: *origin(other)}}, NewTask: &NewTaskInput{Request: "Rename exactly one repository", MethodProfile: domain.MethodPlain}})
	if err != nil {
		t.Fatal(err)
	}
	task := prepareRename(t, s, opened.Task)
	fileScopeRunGit(t, root, "branch", "-m", "main", "task-renamed")
	fileScopeRunGit(t, other, "commit", "--allow-empty", "-m", "unrelated advance")
	if _, err = s.ResolveBlockerAction(context.Background(), renameDecision(task, "complete"), "complete"); err == nil {
		t.Fatal("other repository drift accepted")
	}
}

func TestBranchRenameTaskCancellationPreservesVerifiedEffectiveBranch(t *testing.T) {
	for _, execute := range []bool{false, true} {
		t.Run(fmt.Sprint(execute), func(t *testing.T) {
			s, db, original, root, dbPath := renameGitTask(t)
			task := prepareRename(t, s, original)
			if execute {
				fileScopeRunGit(t, root, "branch", "-m", "main", "task-renamed")
			}
			result, err := s.CancelTask(context.Background(), CancelTaskRequest{RequestID: "cancel", Host: task.OriginHost, TaskID: task.TaskID, ExpectedRevision: task.Revision, Reason: "End Task"})
			if err != nil {
				t.Fatal(err)
			}
			expected := "main"
			if execute {
				expected = "task-renamed"
			}
			if result.Task.CurrentNode != domain.NodeCancelled || *result.Task.Repository.CurrentBranch != expected || result.Task.BranchRename != nil || result.Task.WorkspaceOrigin != original.WorkspaceOrigin {
				t.Fatal("cancellation lost verified branch facts")
			}
			db.Close()
			reopened, err := store.Open(context.Background(), dbPath)
			if err != nil {
				t.Fatal(err)
			}
			reopened.Close()
		})
	}
}
