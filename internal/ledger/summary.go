package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const summarySchema = `
CREATE TABLE session_summaries (
 workspace_id TEXT NOT NULL, agent TEXT NOT NULL, session_id TEXT NOT NULL,
 input_version TEXT NOT NULL, settings_revision TEXT NOT NULL, guidance_version TEXT NOT NULL,
 observation_id INTEGER NOT NULL REFERENCES observations(id), delivered_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id,agent,session_id),
 FOREIGN KEY(workspace_id,agent,session_id) REFERENCES task_sessions(workspace_id,agent,session_id)
);
PRAGMA user_version=6;
`

type SummaryState struct {
	InputVersion     string
	SettingsRevision string
	GuidanceVersion  string
	ObservationID    int64
}

type SessionActivity struct {
	Agent     string
	SessionID string
}

func (s *Store) SummaryState(ctx context.Context, workspace string, actor Actor) (SummaryState, error) {
	var state SummaryState
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		err := q.QueryRowContext(ctx, `SELECT input_version,settings_revision,guidance_version,observation_id FROM session_summaries_v11
WHERE workspace_id=? AND agent=? AND session_id=?`, workspace, actor.Agent, actor.SessionID).Scan(&state.InputVersion, &state.SettingsRevision, &state.GuidanceVersion, &state.ObservationID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	return state, err
}

func (s *Store) ActiveSessions(ctx context.Context, workspace string, now time.Time) ([]SessionActivity, error) {
	var sessions []SessionActivity
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, "SELECT agent,session_id,activity_at FROM task_sessions_v11 WHERE workspace_id=? ORDER BY agent,session_id", workspace)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var session SessionActivity
			var raw string
			if err := rows.Scan(&session.Agent, &session.SessionID, &raw); err != nil {
				return err
			}
			at, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return err
			}
			if age := now.Sub(at); age >= 0 && age <= 10*time.Minute {
				sessions = append(sessions, session)
			}
		}
		return rows.Err()
	})
	return sessions, err
}

func (s *Store) TouchSession(ctx context.Context, workspace string, actor Actor) error {
	if actor.SessionID == "" || (actor.Agent != "claude" && actor.Agent != "codex") {
		return fmt.Errorf("session 活动缺少来源")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		at := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO task_sessions_v11 VALUES(?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,agent,session_id) DO UPDATE SET activity_at=excluded.activity_at`, workspace, actor.Agent, actor.SessionID, "", "", at, at)
		return err
	})
}

/**
 * 摘要只记录已采用的输入版本，不改变任务或检查修订。生成期间若另一进程
 * 改了任务、设置或观察点，保留旧的交付标记，下一次事件重新生成。
 */
func (s *Store) SaveSummaryState(ctx context.Context, workspace string, actor Actor, view TaskView, state SummaryState) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := checkCurrent(ctx, tx, workspace, view.CurrentTaskID); err != nil {
			return err
		}
		var latest int64
		if err := tx.QueryRowContext(ctx, "SELECT latest_observation FROM workspaces_v11 WHERE id=?", workspace).Scan(&latest); err != nil {
			return err
		}
		if latest != state.ObservationID {
			return ErrObservationConflict
		}
		if view.Task != nil {
			task, err := loadTask(ctx, tx, workspace, view.Task.ID)
			if err != nil {
				return err
			}
			if task.Revision != view.Task.Revision {
				return &TaskConflict{CurrentTaskID: view.CurrentTaskID}
			}
		}
		settings, err := effectiveSettings(ctx, tx, workspace, view.CurrentTaskID, false)
		if err != nil {
			return err
		}
		if settings.Revision != state.SettingsRevision {
			return fmt.Errorf("摘要生成期间有效设置已变化")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO session_summaries_v11 VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(workspace_id,agent,session_id) DO UPDATE SET input_version=excluded.input_version,
settings_revision=excluded.settings_revision,guidance_version=excluded.guidance_version,observation_id=excluded.observation_id,delivered_at=excluded.delivered_at`, workspace, actor.Agent, actor.SessionID, state.InputVersion, state.SettingsRevision, state.GuidanceVersion, state.ObservationID, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

func (s *Store) ChangedSince(ctx context.Context, workspace string, observation int64) (bool, error) {
	var changed bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM file_changes_v11 WHERE workspace_id=? AND observation_id>?)", workspace, observation).Scan(&changed)
	})
	return changed, err
}

func (s *Store) LatestHookError(ctx context.Context, workspace string) (string, error) {
	var message string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		err := q.QueryRowContext(ctx, "SELECT at || ' ' || error FROM hook_events_v11 WHERE workspace_id=? AND error!='' ORDER BY id DESC LIMIT 1", workspace).Scan(&message)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	return message, err
}
