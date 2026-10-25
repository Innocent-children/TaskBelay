package eval_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/check"
	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/ledger"
)

func TestStaticCancellationKeepsPartialReportWithinOriginalDeadline(t *testing.T) {
	service, task, _, _ := deliveryFixture(t)
	var source strings.Builder
	source.WriteString("package main\n")
	for i := range 2500 {
		fmt.Fprintf(&source, "func value%d(n int)int {\n", i)
		for range 18 {
			source.WriteString("n += 1\n")
		}
		source.WriteString("return n }\n")
	}
	verifiedTaskEdit(t, service, task.ID, "large", "large.go", "", source.String())
	status, err := service.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	observation, err := service.Store.Observation(t.Context(), status.View.Task.LastObservationID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, end := context.WithTimeout(t.Context(), 5*time.Second)
	defer end()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := time.AfterFunc(75*time.Millisecond, cancel)
	defer timer.Stop()
	started := time.Now()
	result, err := check.Run(ctx, service.Store, service.Workspace, taskActor("cancel-static"), status.View, observation, check.RunInput{Refresh: true})
	if err != nil || result.Status != ledger.CheckPartial || result.ID == 0 || len(result.Missing) == 0 {
		t.Fatal(result.Status, result.Missing, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatal("cancellation exceeded reserved publish time", elapsed)
	}
	stored, err := service.Store.LatestCheck(t.Context(), task.ID, "static", "")
	if err != nil || stored == nil || stored.ID != result.ID {
		t.Fatal("lost partial observation", stored, err)
	}
}

func TestHookAndEditFailureStagesPreserveCancellation(t *testing.T) {
	service, task, _, base := deliveryFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	e := edit("failure-phase", "main.go", base, base)
	e.WorkingDir = service.Workspace.Root
	if _, err := service.Handle(ctx, e, nil); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "登记 hook 开始") {
		t.Fatal("hook registration lost phase or cancellation", err)
	}
	if _, err := service.BeforeEdit(ctx, e, task.ID, "block"); !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "读取编辑前观察") {
		t.Fatal("edit observation lost phase or cancellation", err)
	}
}

func TestHookOutputAndFinishErrorsRemainSeparate(t *testing.T) {
	for _, cancelFinish := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelFinish), func(t *testing.T) {
			service, _, _, _ := deliveryFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			a := taskActor("failure-output")
			e := event.Event{Kind: event.SessionStart, Agent: a.Agent, SessionID: a.SessionID, RoundID: a.RoundID, ToolCallID: a.CallID, WorkingDir: service.Workspace.Root}
			err := service.HandleOutput(ctx, e, nil, func(event.Decision) error {
				if cancelFinish {
					cancel()
				}
				return io.ErrClosedPipe
			})
			if !errors.Is(err, io.ErrClosedPipe) || !strings.Contains(err.Error(), "输出 hook 决定") {
				t.Fatal("output failure lost phase or cause", err)
			}
			if errors.Is(err, context.Canceled) != cancelFinish || strings.Contains(err.Error(), "登记 hook 结束") != cancelFinish {
				t.Fatal("output and finish failures were conflated", err)
			}
			db, err := sql.Open("sqlite", filepath.Join(service.Store.DataDir(), "taskbelay.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var state, message string
			err = db.QueryRowContext(t.Context(), "SELECT state,error FROM hook_events WHERE workspace_id=? AND tool_call_id=?", service.Workspace.ID, a.CallID).Scan(&state, &message)
			if err != nil {
				t.Fatal(err)
			}
			if cancelFinish {
				if state != "pending" || message != "" {
					t.Fatal("expired finish fabricated a completed record", state, message)
				}
			} else if state != "failed" || !strings.Contains(message, "输出 hook 决定") {
				t.Fatal("recorded failure lost diagnostic context", state, message)
			}
		})
	}
}
