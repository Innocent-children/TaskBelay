package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
)

const hookSchema = `
CREATE TABLE hook_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 agent TEXT NOT NULL, session_id TEXT NOT NULL, round_id TEXT NOT NULL,
 kind TEXT NOT NULL, tool_call_id TEXT NOT NULL, at TEXT NOT NULL,
 event TEXT NOT NULL, raw_input TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending', decision TEXT NOT NULL DEFAULT '{}',
 error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX hook_events_session ON hook_events(workspace_id,agent,session_id,id);
PRAGMA user_version=4;
`

func (s *Store) BeginHook(ctx context.Context, workspace string, e event.Event, raw []byte) (int64, error) {
	if e.SessionID == "" || (e.Agent != "claude" && e.Agent != "codex") {
		return 0, fmt.Errorf("hook 缺少可核对的来源")
	}
	if e.Command != nil {
		// 环境赋值由命令解析器摘要化，不能在原始 hook 中留下另一份凭据。
		command := *e.Command
		command.Text = "[命令详见脱敏验证记录]"
		e.Command = &command
		raw = nil
		if e.Result != nil {
			result := *e.Result
			result.Output = ""
			e.Result = &result
		}
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		at := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, `INSERT INTO hook_events_v11(workspace_id,agent,session_id,round_id,kind,tool_call_id,at,event,raw_input) VALUES(?,?,?,?,?,?,?,?,?)`, workspace, e.Agent, e.SessionID, e.RoundID, e.Kind, e.ToolCallID, at, string(encoded), string(raw))
		if err != nil {
			return err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return err
		}
		// 只有消息能更新可写轮次；迟到的工具事件不能把 session 倒退到旧轮次。
		_, err = tx.ExecContext(ctx, `INSERT INTO task_sessions_v11 VALUES(?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,agent,session_id) DO UPDATE SET activity_at=excluded.activity_at`, workspace, e.Agent, e.SessionID, "", "", at, at)
		return err
	})
	return id, err
}

func (s *Store) FinishHook(ctx context.Context, id int64, d event.Decision, failure error) error {
	encoded, err := json.Marshal(d)
	if err != nil {
		return err
	}
	state, message := "recorded", ""
	if failure != nil {
		state, message = "failed", failure.Error()
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE hook_events_v11 SET state=?,decision=?,error=? WHERE id=?", state, string(encoded), message, id)
		return err
	})
}
