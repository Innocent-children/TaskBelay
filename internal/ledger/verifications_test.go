package ledger

import (
	"errors"
	"strings"
	"testing"

	"github.com/Innocent-children/taskbelay/internal/event"
)

func TestVerificationRequestsComparePreviousAndReplayAtomically(t *testing.T) {
	s, actor, input, _, _ := checkFixture(t)
	first := VerificationRecord{WorkspaceID: "w", TaskID: input.TaskID, Actor: actor, CommandKey: "same", RequestHash: "request", InputRevision: input.EditRevision}
	stored, err := s.BeginVerification(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != "unknown" || stored.Executed {
		t.Fatal(stored)
	}
	if repeat, err := s.BeginVerification(t.Context(), first); err != nil || repeat.ID != stored.ID {
		t.Fatal("duplicate request not idempotent", repeat, err)
	}
	first.RequestHash = "changed"
	if _, err := s.BeginVerification(t.Context(), first); err == nil {
		t.Fatal("call ID accepted different command")
	}
	first.Actor.CallID = "other"
	if _, err := s.BeginVerification(t.Context(), first); !errors.Is(err, ErrVerificationChanged) {
		t.Fatal("concurrent old previous accepted", err)
	}
	unknown, err := s.EndVerification(t.Context(), "w", actor, &event.ToolResult{State: event.Completed})
	if err != nil || unknown.Record.State != "unknown" || unknown.Record.ExitCode != nil {
		t.Fatal(unknown, err)
	}
	zero := 0
	passed, err := s.EndVerification(t.Context(), "w", actor, &event.ToolResult{State: event.Completed, ExitCode: &zero})
	if err != nil || passed.Record.State != "passed" {
		t.Fatal(passed, err)
	}
	stale, err := s.EndVerification(t.Context(), "w", actor, &event.ToolResult{State: event.Running})
	if err != nil || stale.Record.State != "passed" {
		t.Fatal("late startup response overwrote completion", stale, err)
	}
}

func TestCommandHookDoesNotKeepEnvironmentOrOutputCopies(t *testing.T) {
	s, actor, _, _, _ := checkFixture(t)
	e := event.Event{Kind: event.CommandUse, Agent: actor.Agent, SessionID: actor.SessionID, RoundID: actor.RoundID, ToolCallID: "shell", Command: &event.Command{Text: "SECRET=do-not-save go test", Shell: event.POSIX}, Result: &event.ToolResult{State: event.Completed, Output: "do-not-save"}}
	id, err := s.BeginHook(t.Context(), "w", e, []byte(`{"raw":"do-not-save"}`))
	if err != nil {
		t.Fatal(err)
	}
	var data, raw string
	if err := s.db.QueryRow("SELECT event,raw_input FROM hook_events WHERE id=?", id).Scan(&data, &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data+raw, "do-not-save") || raw != "" {
		t.Fatal("environment persisted in raw event", data, raw)
	}
	if e.Command.Text != "SECRET=do-not-save go test" || e.Result.Output != "do-not-save" {
		t.Fatal("sanitizing storage changed execution input")
	}
}
