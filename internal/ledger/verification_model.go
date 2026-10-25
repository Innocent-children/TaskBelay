package ledger

import (
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/shell"
	"github.com/Innocent-children/taskbelay/internal/testrun"
)

const verificationSchema = `
CREATE TABLE verifications (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT NOT NULL UNIQUE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), task_id TEXT NOT NULL,
 agent TEXT NOT NULL, session_id TEXT NOT NULL, round_id TEXT NOT NULL, call_id TEXT NOT NULL,
 command_key TEXT NOT NULL, denied INTEGER NOT NULL, executed INTEGER NOT NULL,
 state TEXT NOT NULL, data TEXT NOT NULL,
 UNIQUE(workspace_id,agent,session_id,call_id)
);
CREATE INDEX verification_previous ON verifications(workspace_id,agent,session_id,command_key,seq);
CREATE INDEX verification_task ON verifications(task_id,seq);
CREATE TABLE verification_rounds (
 workspace_id TEXT NOT NULL, agent TEXT NOT NULL, session_id TEXT NOT NULL, round_id TEXT NOT NULL,
 notified INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id,agent,session_id,round_id)
);
PRAGMA user_version=8;
`

type VerificationRecord struct {
	ID               string                    `json:"id"`
	WorkspaceID      string                    `json:"workspace_id"`
	TaskID           string                    `json:"task_id"`
	Actor            Actor                     `json:"actor"`
	RequestHash      string                    `json:"request_hash"`
	RequestDirectory string                    `json:"request_directory,omitempty"`
	Command          shell.Script              `json:"command"`
	CommandKey       string                    `json:"command_key"`
	Tree             string                    `json:"tree"`
	InputRevision    int64                     `json:"input_revision"`
	EnvironmentID    string                    `json:"environment_id"`
	Scope            testrun.VerificationScope `json:"scope"`
	State            string                    `json:"state"`
	ExitCode         *int                      `json:"exit_code,omitempty"`
	StartedAt        time.Time                 `json:"started_at"`
	EndedAt          time.Time                 `json:"ended_at"`
	Denied           bool                      `json:"denied"`
	Executed         bool                      `json:"executed"`
	PostOnly         bool                      `json:"post_only,omitempty"`
	PreviousID       string                    `json:"previous_id,omitempty"`
	Issue            *Issue                    `json:"issue,omitempty"`
	RerunNoteID      string                    `json:"rerun_note_id,omitempty"`
	UserRequested    bool                      `json:"user_requested,omitempty"`
}

type VerificationEnd struct {
	Record VerificationRecord
	Notify bool
	Count  int
}

func VerificationState(result *event.ToolResult) string {
	if result == nil {
		return "unknown"
	}
	if result.State == event.Interrupted {
		return "interrupted"
	}
	if result.State == event.Running {
		return "running"
	}
	if result.ExitCode != nil {
		if *result.ExitCode == 0 && result.State == event.Completed {
			return "passed"
		}
		if *result.ExitCode != 0 {
			return "failed"
		}
	}
	if result.State == event.Failed {
		return "failed"
	}
	return "unknown"
}
