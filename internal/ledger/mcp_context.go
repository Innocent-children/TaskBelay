package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const mcpContextSchema = `
CREATE TABLE mcp_contexts (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 agent TEXT NOT NULL, session_id TEXT NOT NULL, call_id TEXT NOT NULL,
 round_id TEXT NOT NULL, tool_name TEXT NOT NULL, at TEXT NOT NULL,
 PRIMARY KEY(workspace_id,agent,session_id,call_id)
);
PRAGMA user_version=5;
`

func (s *Store) BindMCPContext(ctx context.Context, workspace string, actor Actor, tool string) error {
	if err := actor.Validate(); err != nil {
		return err
	}
	if actor.CallID == "" || tool == "" {
		return fmt.Errorf("MCP 前置身份缺少调用编号或工具名")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var round, name string
		err := tx.QueryRowContext(ctx, "SELECT round_id,tool_name FROM mcp_contexts WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", workspace, actor.Agent, actor.SessionID, actor.CallID).Scan(&round, &name)
		if err == nil {
			if round != actor.RoundID || name != tool {
				return fmt.Errorf("MCP 调用编号与已记录的轮次或工具冲突")
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO mcp_contexts VALUES(?,?,?,?,?,?,?)", workspace, actor.Agent, actor.SessionID, actor.CallID, actor.RoundID, tool, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) MCPContext(ctx context.Context, workspace string, actor Actor, tool string) (Actor, error) {
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		var recordedTool string
		err := q.QueryRowContext(ctx, "SELECT round_id,tool_name FROM mcp_contexts WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", workspace, actor.Agent, actor.SessionID, actor.CallID).Scan(&actor.RoundID, &recordedTool)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("尚无本次 MCP 调用的前置身份记录，无法确定轮次；请确认 TaskBelay 的 MCP PreToolUse hook 已加载")
		}
		if err != nil {
			return err
		}
		if recordedTool != tool {
			return fmt.Errorf("MCP 工具名与前置身份记录不符")
		}
		return actor.Validate()
	})
	return actor, err
}
