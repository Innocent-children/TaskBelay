package ledger

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Workspace 是一个 git 工作区。
type Workspace struct {
	Path string // 根目录的绝对路径
	Key  string // 比较路径用的写法，Key 相同就是同一个工作区
}

// Session 是一个 Agent session 的记录。
type Session struct {
	Agent        string
	ID           string
	StartedAt    time.Time
	LastActiveAt time.Time
}

// Message 是一条用户消息。
type Message struct {
	ID      int64
	Agent   string
	Session string
	Turn    string
	Text    string // 原文
	State   string // 到达时的工作区状态
	At      time.Time
}

// TouchSession 记下 session 在 ws 里有一次活动：没有记录时新建，有记录时更新最后活动时间。
// 工作区第一次出现时一并记下。返回工作区的编号。
func (l *Ledger) TouchSession(ctx context.Context, ws Workspace, agent, session string, now time.Time) (int64, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO workspaces (path, key) VALUES (?, ?) ON CONFLICT (key) DO NOTHING`, ws.Path, ws.Key); err != nil {
		return 0, err
	}
	var wsID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE key = ?`, ws.Key).Scan(&wsID); err != nil {
		return 0, err
	}
	at := now.UnixMilli()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (agent, session_id, workspace_id, started_at, last_active_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (agent, session_id) DO UPDATE SET workspace_id = excluded.workspace_id, last_active_at = excluded.last_active_at`,
		agent, session, wsID, at, at); err != nil {
		return 0, err
	}
	return wsID, tx.Commit()
}

// AddMessage 在工作区 wsID 下记一条用户消息，返回消息编号。
func (l *Ledger) AddMessage(ctx context.Context, wsID int64, m Message) (int64, error) {
	res, err := l.db.ExecContext(ctx, `
		INSERT INTO messages (workspace_id, agent, session_id, turn_id, text, state, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		wsID, m.Agent, m.Session, m.Turn, m.Text, m.State, m.At.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FindWorkspace 按 key 找工作区，没有记录时 found 为 false。
func (l *Ledger) FindWorkspace(ctx context.Context, key string) (id int64, found bool, err error) {
	err = l.db.QueryRowContext(ctx, `SELECT id FROM workspaces WHERE key = ?`, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// RecentSessions 返回工作区 wsID 里最近活动的 limit 个 session，最近的在前，以及 session 的总数。
func (l *Ledger) RecentSessions(ctx context.Context, wsID int64, limit int) ([]Session, int, error) {
	var total int
	if err := l.db.QueryRowContext(ctx, `SELECT count(*) FROM sessions WHERE workspace_id = ?`, wsID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := l.db.QueryContext(ctx, `
		SELECT agent, session_id, started_at, last_active_at FROM sessions
		WHERE workspace_id = ? ORDER BY last_active_at DESC LIMIT ?`, wsID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Session
	for rows.Next() {
		var s Session
		var started, active int64
		if err := rows.Scan(&s.Agent, &s.ID, &started, &active); err != nil {
			return nil, 0, err
		}
		s.StartedAt, s.LastActiveAt = time.UnixMilli(started), time.UnixMilli(active)
		list = append(list, s)
	}
	return list, total, rows.Err()
}

// RecentMessages 返回工作区 wsID 里最近的 limit 条用户消息，按到达的先后排列。
func (l *Ledger) RecentMessages(ctx context.Context, wsID int64, limit int) ([]Message, error) {
	rows, err := l.db.QueryContext(ctx, `
		SELECT id, agent, session_id, turn_id, text, state, created_at FROM (
			SELECT * FROM messages WHERE workspace_id = ? ORDER BY created_at DESC, id DESC LIMIT ?
		) ORDER BY created_at, id`, wsID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Message
	for rows.Next() {
		var m Message
		var at int64
		if err := rows.Scan(&m.ID, &m.Agent, &m.Session, &m.Turn, &m.Text, &m.State, &at); err != nil {
			return nil, err
		}
		m.At = time.UnixMilli(at)
		list = append(list, m)
	}
	return list, rows.Err()
}
