package application

import (
	"context"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/store"
)

func TestHostUsageSamplesAreAtomicMonotonicAndKeepUnknownDistinct(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	if v.Task.Usage.Host.Coverage != "unreported" || v.Task.Usage.Host.Tokens != nil {
		t.Fatal(v.Task.Usage.Host)
	}
	report := &domain.HostUsageReport{SourceID: "session-task", ObservedThrough: time.Now().UTC(), Coverage: "reported", Totals: domain.HostUsageTotals{Tokens: ptr(int64(0)), ActiveMS: ptr(int64(12)), WaitMS: ptr(int64(3))}}
	q := CheckpointRequest{Identity: Identity{v.Task.TaskID, "usage", v.Task.Revision}, HostUsage: report}
	v, err := s.Checkpoint(ctx, q)
	if err != nil || v.Task.Usage.Host.Tokens == nil || *v.Task.Usage.Host.Tokens != 0 || v.Task.Usage.HostMS != 12 {
		t.Fatal(err, v.Task.Usage)
	}
	replay, err := s.Checkpoint(ctx, q)
	if err != nil || !replay.Replayed || replay.Task.Usage.HostMS != 12 {
		t.Fatal("replayed cost", err)
	}
	q.Identity = Identity{v.Task.TaskID, "same-sample-new-operation", v.Task.Revision}
	v, err = s.Checkpoint(ctx, q)
	if err != nil || v.Task.Usage.HostMS != 12 {
		t.Fatal("sample counted twice", err)
	}
	report.ObservedThrough = report.ObservedThrough.Add(time.Second)
	report.Totals.ActiveMS = ptr(int64(11))
	q.Identity = Identity{v.Task.TaskID, "backwards", v.Task.Revision}
	_, err = s.Checkpoint(ctx, q)
	if code(err) != "HOST_USAGE_REGRESSION" {
		t.Fatal(err)
	}
	saved, _ := s.Store.Get(ctx, v.Task.TaskID)
	if saved.Revision != v.Task.Revision || saved.Usage.HostMS != 12 {
		t.Fatal("failed sample changed state")
	}
	report.Totals.ActiveMS = ptr(int64(15))
	q.Identity.Revision--
	_, err = s.Checkpoint(ctx, q)
	if code(err) != "REVISION_CONFLICT" {
		t.Fatal(err)
	}
	q.Identity = Identity{v.Task.TaskID, "next-sample", v.Task.Revision}
	v, err = s.Checkpoint(ctx, q)
	if err != nil || v.Task.Usage.HostMS != 15 {
		t.Fatal("CAS failure changed high-water mark", err)
	}
	report.SourceID = "new-nonoverlapping-source"
	report.Coverage = "partial"
	report.Totals = domain.HostUsageTotals{Tokens: ptr(int64(7))}
	q.Identity = Identity{v.Task.TaskID, "partial", v.Task.Revision}
	v, err = s.Checkpoint(ctx, q)
	if err != nil || v.Task.Usage.Host.Coverage != "partial" || v.Task.Usage.Tokens != 7 {
		t.Fatal(err)
	}
}

func TestExplicitUnknownHostCapStopsOnlyNewExecutions(t *testing.T) {
	s, v, _ := commandProposal(t, 1, 3000)
	ctx := context.Background()
	a := v.Task.Authorization
	a.Limits.Tokens = 1000
	a.UserRef = "actual-token-limit"
	v, err := s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "token-limit", v.Task.Revision}, Authorization: &a, Reason: "User specified token cap"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "unknown-usage", v.Task.Revision}, CheckID: "check-0", Rationale: "Required verification"})
	if code(err) != "HOST_USAGE_UNAVAILABLE" {
		t.Fatal(err)
	}
	saved, err := s.Inspect(ctx, v.Task.TaskID, false)
	if err != nil || saved.Task.Usage.Runs != 0 {
		t.Fatal(err)
	}
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "actual-usage", v.Task.Revision}, HostUsage: &domain.HostUsageReport{SourceID: "fixture", ObservedThrough: time.Now().UTC(), Coverage: "reported", Totals: domain.HostUsageTotals{Tokens: ptr(int64(20)), ActiveMS: ptr(int64(0)), WaitMS: ptr(int64(0))}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "known-usage", v.Task.Revision}, CheckID: "check-0", Rationale: "Required verification"})
	if err != nil || run.Run.Status != "passed" {
		t.Fatal(err)
	}
}

func TestHostUsageTransactionRollsBackSourceAfterCASConflict(t *testing.T) {
	s, v, _ := fixture(t)
	ctx := context.Background()
	stale := v.Task
	v, err := s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "advance", v.Task.Revision}})
	if err != nil {
		t.Fatal(err)
	}
	sample := &domain.HostUsageReport{SourceID: "cas-source", ObservedThrough: time.Now().UTC(), Coverage: "reported", Totals: domain.HostUsageTotals{Tokens: ptr(int64(11)), ActiveMS: ptr(int64(12)), WaitMS: ptr(int64(0))}}
	expected := stale.Revision
	stale.Revision++
	err = s.Store.Save(ctx, stale, expected, store.Operation{ID: "stale-cas", Kind: "checkpoint", Digest: domain.Hash("stale")}, store.Charge{HostUsage: sample}, "", nil, "", "")
	if code(err) != "REVISION_CONFLICT" {
		t.Fatal(err)
	}
	saved, err := s.Store.Get(ctx, v.Task.TaskID)
	if err != nil || saved.Usage.Host.Coverage != "unreported" || saved.Usage.Tokens != 0 {
		t.Fatal("failed CAS retained sample", err, saved.Usage)
	}
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "apply-sample", v.Task.Revision}, HostUsage: sample})
	if err != nil || v.Task.Usage.Tokens != 11 || v.Task.Usage.HostMS != 12 {
		t.Fatal("sample high-water mark escaped rollback", err, v.Task.Usage)
	}
}

func TestReachedHostLimitStillIdentifiesPassingResultForReuse(t *testing.T) {
	s, v, _ := commandProposal(t, 1, 3000)
	ctx := context.Background()
	executed, err := s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "before-cap", v.Task.Revision}, CheckID: "check-0", Rationale: "Verify before the Host reaches its cap"})
	if err != nil {
		t.Fatal(err)
	}
	v = executed.View
	a := v.Task.Authorization
	a.Limits.Tokens = 10
	a.UserRef = "actual-cap"
	v, err = s.Revise(ctx, ReviseRequest{Identity: Identity{v.Task.TaskID, "set-cap", v.Task.Revision}, Authorization: &a, Reason: "Actual explicit token cap"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Checkpoint(ctx, CheckpointRequest{Identity: Identity{v.Task.TaskID, "cap-report", v.Task.Revision}, HostUsage: &domain.HostUsageReport{SourceID: "source", ObservedThrough: time.Now().UTC(), Coverage: "reported", Totals: domain.HostUsageTotals{Tokens: ptr(int64(10)), ActiveMS: ptr(int64(0)), WaitMS: ptr(int64(0))}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Execute(ctx, RunRequest{Identity: Identity{v.Task.TaskID, "reuse-after-cap", v.Task.Revision}, CheckID: "check-0", Rationale: "Reuse the applicable saved result"})
	if code(err) != "CHECK_ALREADY_SATISFIED" {
		t.Fatal("already completed work requested more budget", err)
	}
	saved, _ := s.Store.Get(ctx, v.Task.TaskID)
	if saved.Usage.Runs != 1 {
		t.Fatal("passing result reran")
	}
}
