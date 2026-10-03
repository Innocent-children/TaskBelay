package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
	"github.com/Innocent-children/taskbelay/internal/store"
)

type pendingScopeStore struct {
	*store.SQLite
	stages     int
	failCommit bool
}

func (s *pendingScopeStore) StageActionOperation(ctx context.Context, task domain.ProcessTask, commit domain.ActionCommit) error {
	s.stages++
	return s.SQLite.StageActionOperation(ctx, task, commit)
}
func (s *pendingScopeStore) CommitActionOperation(ctx context.Context, id domain.ID, mutation store.TaskMutation) error {
	if s.failCommit {
		return store.ErrStorageUnavailable
	}
	return s.SQLite.CommitActionOperation(ctx, id, mutation)
}

type pendingScopeFixture struct {
	service    *Service
	storage    *pendingScopeStore
	task       domain.ProcessTask
	root, data string
}

func newPendingScopeFixture(t *testing.T) *pendingScopeFixture {
	t.Helper()
	root := fileScopeGitRepository(t)
	// The final carried path is untracked, so removing it disappears from TaskSurface.
	for i := 0; i < 63; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("carry-%02d.txt", i)), []byte("base\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	fileScopeRunGit(t, root, "add", ".")
	fileScopeRunGit(t, root, "commit", "-m", "Carried base")
	for i := 0; i < 64; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("carry-%02d.txt", i)), []byte("carried\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	data := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(data, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	storage := &pendingScopeStore{SQLite: db}
	service, err := NewService(storage, repository.NewGitObserver())
	if err != nil {
		t.Fatal(err)
	}
	opened, err := service.OpenTask(context.Background(), OpenTaskRequest{
		RequestID: "pending-open", Host: domain.HostCodex, RepositoryPath: root,
		WorkspaceOrigin: &WorkspaceOriginInput{Mode: domain.WorkspaceModeCurrentBranch, SourceType: "local", CarryChanges: true, BaseBranch: "main", BaseCommit: fileScopeRunGit(t, root, "rev-parse", "HEAD"), TaskBranch: "main", ProvisioningReceiptID: "pending-receipt"},
		NewTask:         &NewTaskInput{Request: "Expand one prepared write without authorizing carried content.", MethodProfile: domain.MethodPlain}})
	if err != nil {
		t.Fatal(err)
	}
	f := &pendingScopeFixture{service: service, storage: storage, task: opened.Task, root: root, data: data}
	f.submit(t, "requirements_ready", requirementsNodeResult("Explicit scope", []string{"Keep carry and decide the new path explicitly"}))
	f.submit(t, "design_ready", designResultWithoutRevision("Use the existing scope decision"))
	item := workItem("change", []uint32{0}, nil)
	item["expected_paths"] = []string{"planned.txt"}
	f.submit(t, "tasks_plan_saved", tasksResultWithoutRevision([]map[string]any{item}))
	f.submit(t, "tasks_ready", confirmedPlanResult(f.task))
	denied, err := service.PrepareFileChange(context.Background(), PrepareFileChangeRequest{Host: domain.HostCodex, RepositoryPath: root, ToolName: "apply_patch", Paths: []string{filepath.Join(root, "extra.txt")}, PathParseComplete: true, IntentDigest: digestOf("d")})
	if err != nil || denied.Decision != FileChangeDeny {
		t.Fatalf("prepare=%+v err=%v", denied, err)
	}
	f.task, err = db.LoadTask(context.Background(), f.task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if f.task.CurrentNode != domain.NodeBlocked || len(f.task.FileScopeRecords) != 1 || f.task.FileScopeRecords[0].Observed {
		t.Fatal("expected the original unexecuted pending")
	}
	return f
}
func (f *pendingScopeFixture) submit(t *testing.T, transition string, result map[string]any) {
	t.Helper()
	result["problem_class"] = "none"
	request := actionSubmission(t, f.task, domain.ID(fmt.Sprintf("submit-%d", f.task.Revision)), domain.TransitionID(transition), result)
	out, err := f.service.SubmitAction(context.Background(), request)
	if err != nil {
		t.Fatalf("%s: %v", transition, err)
	}
	f.task = out.Task
}
func (f *pendingScopeFixture) request() RecoverActionRequest {
	return RecoverActionRequest{Host: domain.HostCodex, TaskID: f.task.TaskID, ActionID: f.task.CurrentAction.ActionID, ExpectedRevision: f.task.Revision, FileScopeDecision: &domain.FileScopeDecisionInput{Choice: domain.FileScopeExpandScope, Reason: "Explicitly return to TASKS and review every exact carried path."}}
}
func TestPendingScopeSQLiteExpandAndConfirm(t *testing.T) {
	f := newPendingScopeFixture(t)
	before := f.task
	resolved, err := f.service.ResolveBlockerAction(context.Background(), f.request(), "expand-pending")
	if err != nil {
		t.Fatal(err)
	}
	f.task = resolved.Task
	if f.task.CurrentNode != domain.NodeTasks || f.task.TaskPlan != nil || f.task.FileScopeRecords[0].Decision != domain.FileScopeExpandScope || f.task.FileScopeRecords[0].RequestID != before.FileScopeRecords[0].RequestID {
		t.Fatal("original pending/plan transition was not retained")
	}
	paths := []string{"planned.txt", "extra.txt"}
	for i := 0; i < 64; i++ {
		paths = append(paths, fmt.Sprintf("carry-%02d.txt", i))
	}
	item := workItem("preserve-and-change", []uint32{0}, nil)
	item["expected_paths"] = paths
	f.submit(t, "tasks_plan_saved", tasksResultWithoutRevision([]map[string]any{item}))
	f.submit(t, "tasks_ready", confirmedPlanResult(f.task))
	if len(f.task.UnexplainedChangedPaths(f.task.Repository, nil)) != 0 {
		t.Fatal("explicit plan did not cover carried content")
	}
	next, err := f.service.GetNextAction(context.Background(), GetNextActionRequest{Host: domain.HostCodex, TaskID: f.task.TaskID})
	if err != nil || next.CurrentNode != domain.NodeImplement {
		t.Fatalf("next=%+v err=%v", next, err)
	}
	denied, err := f.service.PrepareFileChange(context.Background(), PrepareFileChangeRequest{Host: domain.HostCodex, RepositoryPath: f.root, ToolName: "apply_patch", Paths: []string{filepath.Join(f.root, "outside.txt")}, PathParseComplete: true, IntentDigest: digestOf("e")})
	if err != nil || denied.Decision != FileChangeDeny {
		t.Fatalf("new plan bypassed write guard: %+v %v", denied, err)
	}
}
func TestPendingScopeSQLiteRejectsNewFactsWithoutStage(t *testing.T) {
	for _, name := range []string{"index", "delete", "delete-untracked", "mode", "new", "head", "branch", "stale-action", "stale-revision"} {
		t.Run(name, func(t *testing.T) {
			f := newPendingScopeFixture(t)
			before := f.task
			request := f.request()
			switch name {
			case "index":
				fileScopeRunGit(t, f.root, "add", "carry-00.txt")
			case "delete":
				if err := os.Remove(filepath.Join(f.root, "carry-00.txt")); err != nil {
					t.Fatal(err)
				}
			case "delete-untracked":
				if err := os.Remove(filepath.Join(f.root, "carry-63.txt")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(filepath.Join(f.root, "carry-00.txt"), 0755); err != nil {
					t.Fatal(err)
				}
			case "new":
				if err := os.WriteFile(filepath.Join(f.root, "outside.txt"), []byte("outside"), 0644); err != nil {
					t.Fatal(err)
				}
			case "head":
				fileScopeRunGit(t, f.root, "commit", "--allow-empty", "-m", "Different HEAD")
			case "branch":
				fileScopeRunGit(t, f.root, "branch", "-m", "other")
			case "stale-action":
				request.ActionID = "stale-action"
			case "stale-revision":
				request.ExpectedRevision--
			}
			stages := f.storage.stages
			_, err := f.service.ResolveBlockerAction(context.Background(), request, "reject-fact")
			if err == nil {
				t.Fatal("unreviewed change accepted")
			}
			saved, loadErr := f.storage.LoadTask(context.Background(), before.TaskID)
			events, eventErr := f.storage.LoadTaskEvents(context.Background(), before.TaskID)
			if loadErr != nil || eventErr != nil || saved.Revision != before.Revision || len(events) != int(before.Revision) || f.storage.stages != stages {
				t.Fatal("rejected decision wrote state or staged an operation")
			}
		})
	}
}
func TestPendingScopeSQLiteRecoversStagedDecision(t *testing.T) {
	f := newPendingScopeFixture(t)
	before := f.task
	f.storage.failCommit = true
	if _, err := f.service.ResolveBlockerAction(context.Background(), f.request(), "staged-expand"); !errors.Is(err, domain.ErrStorageUnavailable) {
		t.Fatalf("injected failure=%v", err)
	}
	if _, err := f.service.ResolveBlockerAction(context.Background(), f.request(), "duplicate"); !errors.Is(err, domain.ErrRecoveryUnavailable) {
		t.Fatalf("unrecorded operation was bypassed: %v", err)
	}
	if err := f.storage.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(context.Background(), filepath.Join(f.data, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	f.storage = &pendingScopeStore{SQLite: reopened}
	// Both SQLite and the service reopen before recovering the persisted operation.
	service, err := NewService(f.storage, repository.NewGitObserver())
	if err != nil {
		t.Fatal(err)
	}
	f.service = service
	fileScopeRunGit(t, f.root, "add", "carry-00.txt")
	drifted, driftErr := service.RecoverAction(context.Background(), RecoverActionRequest{Host: domain.HostCodex, TaskID: before.TaskID, ActionID: before.CurrentAction.ActionID})
	if driftErr != nil || drifted.Task.CurrentNode != domain.NodeBlocked || drifted.Task.Revision != before.Revision {
		t.Fatalf("recovery did not retain the existing blocker: node=%s revision=%d err=%v", drifted.Task.CurrentNode, drifted.Task.Revision, driftErr)
	}
	read, readErr := service.GetTask(context.Background(), GetTaskRequest{Host: domain.HostCodex, TaskID: before.TaskID})
	if readErr != nil || read.RecoveryAssessment == nil || read.RecoveryAssessment.NextAdvice != "stop_for_repository_drift" {
		t.Fatalf("recovery drift assessment=%+v err=%v", read.RecoveryAssessment, readErr)
	}
	retained, loadErr := f.storage.LoadTask(context.Background(), before.TaskID)
	operation, found, operationErr := f.storage.LoadActionOperation(context.Background(), before.TaskID)
	if loadErr != nil || operationErr != nil || !found || operation.AppliedRevision != nil || retained.Revision != before.Revision {
		t.Fatal("drift during recovery replaced the original blocker or operation")
	}
	fileScopeRunGit(t, f.root, "reset", "HEAD", "--", "carry-00.txt")
	result, err := service.RecoverAction(context.Background(), RecoverActionRequest{Host: domain.HostCodex, TaskID: before.TaskID, ActionID: before.CurrentAction.ActionID})
	if err != nil || result.Task.CurrentNode != domain.NodeTasks {
		t.Fatalf("recover=%+v err=%v", result.Task.CurrentNode, err)
	}
	f.task = result.Task
	events, err := f.storage.LoadTaskEvents(context.Background(), before.TaskID)
	if err != nil || len(events) != int(before.Revision+1) || events[len(events)-1].RequestID != "staged-expand" {
		t.Fatalf("recovery audit=%+v err=%v", events, err)
	}
}
func TestPendingScopeSQLiteConcurrentCAS(t *testing.T) {
	f := newPendingScopeFixture(t)
	// Separate store wrappers avoid mutable test counters across goroutines.
	services := make([]*Service, 2)
	for i := range services {
		var err error
		services[i], err = NewService(f.storage.SQLite, repository.NewGitObserver())
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range services {
		wg.Add(1)
		go func(i int, s *Service) {
			defer wg.Done()
			<-start
			_, err := s.ResolveBlockerAction(context.Background(), f.request(), domain.ID(fmt.Sprintf("concurrent-%d", i)))
			results <- err
		}(i, s)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	saved, err := f.storage.LoadTask(context.Background(), f.task.TaskID)
	if err != nil || successes != 1 || saved.Revision != f.task.Revision+1 {
		t.Fatalf("CAS successes=%d revision=%d err=%v", successes, saved.Revision, err)
	}
}
