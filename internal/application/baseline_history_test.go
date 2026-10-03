package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func TestBaselineHistorySQLiteReworkRestartAndPagination(t *testing.T) {
	ctx := context.Background()
	s, _, _ := phase5Service(t)
	path := filepath.Join(t.TempDir(), "tasks.db")
	database, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { database.Close() }()
	s.taskStore = database
	task := openPhase5Task(t, s)
	var want []domain.BaselineReference
	for cycle := uint32(1); cycle <= 40; cycle++ {
		task = applyPhase5(t, s, task, "requirements_ready", "", requirementsNodeResult(fmt.Sprintf("Goal %d", cycle), []string{"criterion"}))
		task = applyPhase5(t, s, task, "design_ready", "", designNodeResult(cycle, "Design"))
		task = applyPhase5(t, s, task, "tasks_ready", "", tasksNodeResult(cycle, []map[string]any{workItem("work-a", []uint32{0}, nil)}))
		if task.Requirements.Revision != cycle || task.Design.Revision != cycle || task.TaskPlan.Revision != cycle {
			t.Fatal("revision did not advance")
		}
		if cycle < 40 {
			want = append(want, domain.BaselineReference{Kind: domain.BaselineRequirements, Revision: cycle, Digest: task.Requirements.Digest, Summary: task.Requirements.Goal, CreatedAt: task.Requirements.CreatedAt})
		}
		want = append(want, domain.BaselineReference{Kind: domain.BaselineDesign, Revision: cycle, Digest: task.Design.Digest, Summary: task.Design.Approach, CreatedAt: task.Design.CreatedAt}, taskPlanReference(*task.TaskPlan))
		task = applyPhase5(t, s, task, "implementation_requires_requirements", "Acceptance changed.", implementationNodeResult(cycle, nil, true, []string{"Requirement gap"}))
		if len(task.BaselineHistory) != int(cycle)*3-1 {
			t.Fatal("history was truncated")
		}
		if cycle%5 == 0 {
			database.Close()
			database, err = store.Open(ctx, path)
			if err != nil {
				t.Fatalf("restart cycle %d: %v", cycle, err)
			}
			s.taskStore = database
			task, err = database.LoadTask(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var got []domain.BaselineReference
	var after uint64
	for {
		result, err := s.GetTask(ctx, GetTaskRequest{Host: domain.HostCodex, TaskID: task.TaskID, BaselineHistory: &domain.BaselineHistoryQuery{Revision: task.Revision, After: after, Limit: 7}})
		if err != nil {
			t.Fatal(err)
		}
		page := result.BaselineHistory
		for _, entry := range page.Entries {
			if entry.Sequence != uint64(len(got)+1) {
				t.Fatal("pagination gap/duplicate")
			}
			got = append(got, entry.Reference)
		}
		if page.NextAfter == nil {
			break
		}
		after = *page.NextAfter
	}
	// The history ordering follows actual invalidation, while all references remain exact.
	byKey := map[string]domain.BaselineReference{}
	for _, ref := range got {
		byKey[fmt.Sprintf("%s/%d", ref.Kind, ref.Revision)] = ref
	}
	if len(got) != 119 || len(byKey) != 119 {
		t.Fatalf("history count=%d unique=%d", len(got), len(byKey))
	}
	for _, ref := range want {
		if byKey[fmt.Sprintf("%s/%d", ref.Kind, ref.Revision)] != ref {
			t.Fatalf("reference lost: %+v", ref)
		}
	}
	stale := task.Revision
	task = applyPhase5(t, s, task, "requirements_ready", "", requirementsNodeResult("Goal 41", []string{"criterion"}))
	if _, err := s.GetTask(ctx, GetTaskRequest{Host: domain.HostCodex, TaskID: task.TaskID, BaselineHistory: &domain.BaselineHistoryQuery{Revision: stale, After: 7, Limit: 7}}); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("stale page: %v", err)
	}

}

func TestBaselineHistoryLargeActionFailureRollbackAndRecovery(t *testing.T) {
	ctx := context.Background()
	s, _, _ := phase5Service(t)
	path := filepath.Join(t.TempDir(), "tasks.db")
	database, err := store.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s.taskStore = database
	task := openPhase5Task(t, s)
	// Seed a valid >1 MiB historical snapshot in this isolated database; the
	// following submission, stage, failed commit and recovery use the real service.
	task = applyPhase5(t, s, task, "requirements_ready", "", requirementsNodeResult("Goal", []string{"criterion"}))
	task = applyPhase5(t, s, task, "design_requires_requirements", "Gap.", map[string]any{"problem_class": "requirement_gap", "baseline": nil, "findings": []string{"Gap"}})
	for i := 1; i <= 600; i++ {
		task.BaselineHistory = append(task.BaselineHistory, domain.BaselineReference{Kind: domain.BaselineRequirements, Revision: uint32(i), Digest: task.Requirements.Digest, Summary: strings.Repeat("r", domain.MaxEvidenceSummaryBytes), CreatedAt: task.CreatedAt})
	}
	task.Requirements.Revision = 601
	snapshot, err := json.Marshal(task)
	if err != nil || len(snapshot) <= domain.MaxPersistedTaskSnapshotBytes {
		t.Fatalf("fixture bytes=%d err=%v", len(snapshot), err)
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`UPDATE tasks SET snapshot=? WHERE task_id=?`, snapshot, task.TaskID); err != nil {
		t.Fatal(err)
	}
	database.Close()
	database, err = store.Open(ctx, path)
	if err != nil {
		t.Fatalf("large restart: %v", err)
	}
	defer database.Close()
	s.taskStore = database
	before, err := database.LoadTask(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	var beforeEvents int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM task_events`).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TRIGGER fail_history_event BEFORE INSERT ON task_events BEGIN SELECT RAISE(ABORT,'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = s.SubmitAction(ctx, requirementsSubmission(t, task, "history-failure"))
	if err == nil {
		t.Fatal("injected write failure succeeded")
	}
	t.Logf("injected action error: %v", err)
	loaded, err := database.LoadTask(ctx, task.TaskID)
	if err != nil || !reflect.DeepEqual(loaded, before) {
		t.Fatalf("failure changed Task/history: %v", err)
	}
	operation, found, err := database.LoadActionOperation(ctx, task.TaskID)
	if err != nil || !found || operation.AppliedRevision != nil {
		t.Fatalf("pending operation lost: %v", err)
	}
	var count int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM task_events`).Scan(&count); err != nil || count != beforeEvents {
		t.Fatalf("partial events=%d err=%v", count, err)
	}
	if _, err := raw.Exec(`DROP TRIGGER fail_history_event`); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.RecoverAction(ctx, RecoverActionRequest{Host: domain.HostCodex, TaskID: task.TaskID, ActionID: task.CurrentAction.ActionID})
	if err != nil || recovered.Task.Revision != task.Revision+1 || len(recovered.Task.BaselineHistory) != 601 || !reflect.DeepEqual(recovered.Task.BaselineHistory[:600], before.BaselineHistory) || recovered.Task.Requirements.Revision != 602 {
		t.Fatalf("pending operation recovery: %v", err)
	}
}
