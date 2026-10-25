package eval_test

import (
	"strings"
	"testing"
	"time"
)

func TestMaintenanceReleasesOnlyUnreferencedExpiredSnapshots(t *testing.T) {
	old := time.Now().Add(-40 * 24 * time.Hour).Format(time.RFC3339)
	t.Setenv("GIT_AUTHOR_DATE", old)
	t.Setenv("GIT_COMMITTER_DATE", old)
	service, task, _, _ := deliveryFixture(t)
	before, err := service.Workspace.Capture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := service.Workspace.Retain(t.Context(), "interrupted-session", "orphan", before.Tree)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	t.Setenv("GIT_AUTHOR_DATE", now.Format(time.RFC3339))
	t.Setenv("GIT_COMMITTER_DATE", now.Format(time.RFC3339))
	recent, err := service.Workspace.Retain(t.Context(), "in-flight-session", "candidate", before.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Maintain(t.Context(), now); err != nil {
		t.Fatal(err)
	}
	refs := git(t, service.Workspace.Root, "for-each-ref", "--format=%(refname)", "refs/taskbelay/")
	if strings.Contains(refs, orphan.Ref) || !strings.Contains(refs, recent.Ref) || !strings.Contains(refs, task.OriginRef) {
		t.Fatal(refs, task.OriginRef)
	}
	if _, err := service.Store.Observation(t.Context(), task.OriginObservationID); err != nil {
		t.Fatal("active task origin lost", err)
	}
	after, err := service.Workspace.Capture(t.Context())
	if err != nil || before.Tree != after.Tree {
		t.Fatal("maintenance changed workspace", err)
	}
}
