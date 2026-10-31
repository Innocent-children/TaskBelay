package ledger

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

const taskSchema = `
CREATE TABLE tasks (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), state TEXT NOT NULL,
 branch TEXT NOT NULL, data TEXT NOT NULL
);
CREATE INDEX tasks_workspace ON tasks(workspace_id,state,branch);
CREATE TABLE messages (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), agent TEXT NOT NULL,
 session_id TEXT NOT NULL, round_id TEXT NOT NULL, event_id TEXT NOT NULL,
 observation_id INTEGER NOT NULL REFERENCES observations(id), data TEXT NOT NULL,
 UNIQUE(workspace_id,agent,session_id,event_id)
);
CREATE TABLE task_sessions (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), agent TEXT NOT NULL,
 session_id TEXT NOT NULL, round_id TEXT NOT NULL, last_message_id TEXT NOT NULL,
 started_at TEXT NOT NULL, activity_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id,agent,session_id)
);
CREATE TABLE sources (
 id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 task_id TEXT NOT NULL, data TEXT NOT NULL
);
CREATE TABLE requirement_versions (
 task_id TEXT NOT NULL REFERENCES tasks(id), id TEXT NOT NULL, revision INTEGER NOT NULL,
 data TEXT NOT NULL, PRIMARY KEY(task_id,id,revision)
);
CREATE TABLE requirement_locations (
 task_id TEXT NOT NULL REFERENCES tasks(id), id TEXT NOT NULL, revision INTEGER NOT NULL,
 observation_id INTEGER NOT NULL REFERENCES observations(id), data TEXT NOT NULL,
 PRIMARY KEY(task_id,id,revision)
);
CREATE TABLE notes (
 seq INTEGER PRIMARY KEY AUTOINCREMENT, id TEXT UNIQUE,
 task_id TEXT NOT NULL REFERENCES tasks(id), active INTEGER NOT NULL,
 consumed INTEGER NOT NULL DEFAULT 0, invalidated_by TEXT NOT NULL DEFAULT '', data TEXT NOT NULL
);
CREATE INDEX notes_task ON notes(task_id,seq);
CREATE TABLE task_calls (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), agent TEXT NOT NULL,
 session_id TEXT NOT NULL, call_id TEXT NOT NULL, operation TEXT NOT NULL,
 input_hash TEXT NOT NULL, result TEXT NOT NULL,
 PRIMARY KEY(workspace_id,agent,session_id,call_id)
);
CREATE TABLE task_updates (
 task_id TEXT NOT NULL REFERENCES tasks(id), observation_id INTEGER NOT NULL REFERENCES observations(id),
 data TEXT NOT NULL, PRIMARY KEY(task_id,observation_id)
);
PRAGMA user_version=3;
`

type Actor struct {
	Agent     string `json:"agent"`
	SessionID string `json:"session_id"`
	RoundID   string `json:"round_id"`
	CallID    string `json:"tool_call_id,omitempty"`
}

func (a Actor) Validate() error {
	if (a.Agent != "claude" && a.Agent != "codex") || a.SessionID == "" || a.RoundID == "" {
		return fmt.Errorf("缺少可核对的 Agent、session 或轮次")
	}
	return nil
}

type Call struct {
	WorkspaceID string
	Actor       Actor
	Operation   string
	InputHash   string
}

func NewCall(workspace string, actor Actor, operation string, input any) (Call, error) {
	if err := actor.Validate(); err != nil {
		return Call{}, err
	}
	if workspace == "" || actor.CallID == "" {
		return Call{}, fmt.Errorf("写入请求缺少工作区或工具调用编号")
	}
	data, err := json.Marshal(struct {
		Round string
		Input any
	}{actor.RoundID, input})
	if err != nil {
		return Call{}, err
	}
	return Call{workspace, actor, operation, fmt.Sprintf("%x", sha256.Sum256(data))}, nil
}

type TaskState string

const (
	TaskActive    TaskState = "active"
	TaskPending   TaskState = "pending_delivery"
	TaskPaused    TaskState = "paused"
	TaskCompleted TaskState = "completed"
	TaskCanceled  TaskState = "canceled"
)

type TaskRecord struct {
	ID                    string           `json:"id"`
	WorkspaceID           string           `json:"workspace_id"`
	Title                 string           `json:"title"`
	State                 TaskState        `json:"state"`
	StartMessageID        string           `json:"start_message_id"`
	OriginObservationID   int64            `json:"origin_observation_id"`
	OriginTree            string           `json:"origin_tree"`
	OriginRef             string           `json:"origin_ref"`
	ResumeObservationID   int64            `json:"resume_observation_id"`
	LastObservationID     int64            `json:"last_observation_id"`
	Context               gitstate.Context `json:"context"`
	Workflow              string           `json:"workflow"`
	WorkflowHistory       []WorkflowChange `json:"workflow_history,omitempty"`
	SettingsRevision      string           `json:"settings_revision"`
	Revision              int64            `json:"revision"`
	RequirementRevision   int64            `json:"requirement_revision"`
	PlanRevision          int64            `json:"plan_revision"`
	AuthorizationRevision int64            `json:"authorization_revision"`
	RecordRevision        int64            `json:"record_revision"`
	NextRequirement       int              `json:"next_requirement"`
	CreatedAt             time.Time        `json:"created_at"`
	UpdatedAt             time.Time        `json:"updated_at"`
	PausedAt              *time.Time       `json:"paused_at,omitempty"`
	EndedAt               *time.Time       `json:"ended_at,omitempty"`
	EndMessageSeq         int64            `json:"end_message_seq,omitempty"`
	Cancellation          *Citation        `json:"cancellation,omitempty"`
}

type Message struct {
	ID            string           `json:"id"`
	Seq           int64            `json:"seq"`
	WorkspaceID   string           `json:"workspace_id"`
	Actor         Actor            `json:"actor"`
	EventID       string           `json:"event_id"`
	Text          string           `json:"text"`
	At            time.Time        `json:"at"`
	ObservationID int64            `json:"observation_id"`
	Context       gitstate.Context `json:"context"`
}

type Citation struct {
	SourceID string `json:"source_id"`
	Quote    string `json:"quote"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
}

type Source struct {
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	TaskID        string    `json:"task_id,omitempty"`
	Kind          string    `json:"kind"`
	Text          string    `json:"text"`
	Path          string    `json:"path,omitempty"`
	Blob          string    `json:"blob,omitempty"`
	ObservationID int64     `json:"observation_id"`
	Version       int64     `json:"version,omitempty"`
	Confirmation  *Citation `json:"confirmation,omitempty"`
	At            time.Time `json:"at"`
}

type Location struct {
	AnalysisTree  string    `json:"analysis_tree,omitempty"`
	At            string    `json:"at"`
	Quote         string    `json:"quote,omitempty"`
	SourceID      string    `json:"source_id,omitempty"`
	Purpose       string    `json:"purpose,omitempty"`
	Change        string    `json:"change,omitempty"`
	Kind          string    `json:"kind,omitempty"`
	BeforeAt      string    `json:"before_at,omitempty"`
	File          string    `json:"file,omitempty"`
	Blob          string    `json:"blob,omitempty"`
	BeforeBlob    string    `json:"before_blob,omitempty"`
	ObservationID int64     `json:"observation_id,omitempty"`
	Citation      *Citation `json:"citation,omitempty"`
	Pending       string    `json:"pending,omitempty"`
}

type Requirement struct {
	ID            string     `json:"id"`
	TaskID        string     `json:"task_id"`
	Content       string     `json:"content"`
	Acceptance    string     `json:"acceptance"`
	Status        string     `json:"status"`
	Kind          string     `json:"kind"`
	Citation      *Citation  `json:"citation,omitempty"`
	ImplementedBy []Location `json:"implemented_by"`
	Revision      int64      `json:"revision"`
	Author        Actor      `json:"author"`
	At            time.Time  `json:"at"`
}

type DecisionTarget struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	At     string `json:"at,omitempty"`
	Effect string `json:"effect,omitempty"`
	Scope  string `json:"scope,omitempty"`
}

type Note struct {
	ID                  string          `json:"id"`
	TaskID              string          `json:"task_id"`
	Kind                string          `json:"kind"`
	Text                string          `json:"text"`
	Target              *DecisionTarget `json:"target,omitempty"`
	Citation            *Citation       `json:"citation,omitempty"`
	Author              Actor           `json:"author"`
	At                  time.Time       `json:"at"`
	ObservationID       int64           `json:"observation_id"`
	InputRevision       int64           `json:"input_revision"`
	RequirementRevision int64           `json:"requirement_revision"`
	PlanRevision        int64           `json:"plan_revision"`
	SettingsRevision    string          `json:"settings_revision"`
	Active              bool            `json:"active"`
	Consumed            bool            `json:"consumed"`
	InvalidatedBy       string          `json:"invalidated_by,omitempty"`
}

type StartTaskInput struct {
	Title          string `json:"title,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	StartMessageID string `json:"start_message_id,omitempty"`
	Workflow       string `json:"workflow,omitempty"`
}

type RequirementInput struct {
	ID            string      `json:"id,omitempty"`
	Content       string      `json:"content"`
	Quote         string      `json:"quote,omitempty"`
	SourceID      string      `json:"source_id,omitempty"`
	Acceptance    string      `json:"acceptance"`
	Status        string      `json:"status,omitempty"`
	ImplementedBy *[]Location `json:"implemented_by,omitempty"`
	BaseRevision  *int64      `json:"base_revision,omitempty"`
}

type NoteInput struct {
	Kind     string          `json:"kind"`
	Text     string          `json:"text"`
	Target   *DecisionTarget `json:"target,omitempty"`
	Quote    string          `json:"quote,omitempty"`
	SourceID string          `json:"source_id,omitempty"`
}

type TaskConflict struct {
	CurrentTaskID string `json:"current_task_id"`
}

func (e *TaskConflict) Error() string {
	return "当前任务已变化，请读取最新任务后重试"
}

type RequirementConflict struct {
	Latest Requirement `json:"latest"`
}

func (e *RequirementConflict) Error() string {
	return "需求修订已变化，请合并最新版本后重试"
}

var ErrCallConflict = errors.New("同一工具调用编号已用于其他请求")

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *Store) write(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureSchema(ctx, tx); err != nil {
		return err
	}
	if err := f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) read(ctx context.Context, f func(context.Context, queryer) error) error {
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	if err := ensureSchema(ctx, conn); err != nil {
		return err
	}
	if err := f(ctx, conn); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}

func replay(ctx context.Context, q queryer, call Call, result any) (bool, error) {
	var operation, hash, data string
	err := q.QueryRowContext(ctx, "SELECT operation,input_hash,result FROM task_calls_v11 WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID, call.Actor.CallID).Scan(&operation, &hash, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if operation != call.Operation || hash != call.InputHash {
		return false, ErrCallConflict
	}
	return true, json.Unmarshal([]byte(data), result)
}

func (s *Store) Replay(ctx context.Context, call Call, result any) (bool, error) {
	var found bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var err error
		found, err = replay(ctx, q, call, result)
		return err
	})
	return found, err
}

func saveCall(ctx context.Context, tx *sql.Tx, call Call, result any) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO task_calls_v11 VALUES (?,?,?,?,?,?,?,?)", call.WorkspaceID, call.Actor.Agent, call.Actor.SessionID, call.Actor.CallID, call.Operation, call.InputHash, string(data), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
