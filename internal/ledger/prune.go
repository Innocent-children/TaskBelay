package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func sourceReferenced(id string) string {
	return strings.ReplaceAll(`(
EXISTS(SELECT 1 FROM tasks_v11 WHERE json_extract(data,'$.start_message_id')=@id OR json_extract(data,'$.cancellation.source_id')=@id)
OR EXISTS(SELECT 1 FROM messages_v11 m JOIN tasks_v11 t ON t.workspace_id=m.workspace_id JOIN messages_v11 start ON start.id=json_extract(t.data,'$.start_message_id') WHERE m.id=@id AND m.seq>=start.seq AND (COALESCE(json_extract(t.data,'$.end_message_seq'),0)=0 OR m.seq<=json_extract(t.data,'$.end_message_seq')))
OR EXISTS(SELECT 1 FROM requirement_versions_v11 WHERE json_extract(data,'$.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM requirement_versions_v11 r,json_each(r.data,'$.implemented_by') l WHERE json_extract(l.value,'$.source_id')=@id OR json_extract(l.value,'$.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM requirement_locations_v11 r,json_each(r.data) l WHERE json_extract(l.value,'$.source_id')=@id OR json_extract(l.value,'$.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM notes_v11 WHERE json_extract(data,'$.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM plans_v11 WHERE json_extract(data,'$.request_message_id')=@id OR json_extract(data,'$.confirmation.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM plans_v11 p,json_each(p.data,'$.revocations') r WHERE json_extract(r.value,'$.citation.source_id')=@id)
OR EXISTS(SELECT 1 FROM sources_v11 WHERE json_extract(data,'$.confirmation.source_id')=@id)
)`, "@id", id)
}

func observationReferenced(id string) string {
	return strings.ReplaceAll(`(
EXISTS(SELECT 1 FROM workspaces_v11 WHERE latest_observation=@id)
OR EXISTS(SELECT 1 FROM observed_files_v11 WHERE observation_id=@id)
OR EXISTS(SELECT 1 FROM tasks_v11 WHERE json_extract(data,'$.origin_observation_id')=@id OR json_extract(data,'$.resume_observation_id')=@id OR json_extract(data,'$.last_observation_id')=@id)
OR EXISTS(SELECT 1 FROM tasks_v11 t,json_each(t.data,'$.workflow_history') h WHERE json_extract(h.value,'$.observation_id')=@id)
OR EXISTS(SELECT 1 FROM task_updates_v11 WHERE observation_id=@id)
OR EXISTS(SELECT 1 FROM requirement_locations_v11 WHERE observation_id=@id)
OR EXISTS(SELECT 1 FROM requirement_versions_v11 r,json_each(r.data,'$.implemented_by') l WHERE json_extract(l.value,'$.observation_id')=@id)
OR EXISTS(SELECT 1 FROM notes_v11 WHERE json_extract(data,'$.observation_id')=@id)
OR EXISTS(SELECT 1 FROM plans_v11 WHERE json_extract(data,'$.observation_id')=@id OR json_extract(data,'$.confirmation.message.observation_id')=@id)
OR EXISTS(SELECT 1 FROM sources_v11 WHERE json_extract(data,'$.observation_id')=@id)
OR EXISTS(SELECT 1 FROM messages_v11 WHERE observation_id=@id)
OR EXISTS(SELECT 1 FROM session_summaries_v11 WHERE observation_id=@id)
OR EXISTS(SELECT 1 FROM edits_v11 WHERE before_observation=@id OR after_observation=@id)
OR EXISTS(SELECT 1 FROM file_changes_v11 WHERE observation_id=@id OR json_extract(data,'$.BeforeObservation')=@id)
)`, "@id", id)
}

func sessionTaskRoot(alias string) string {
	return strings.ReplaceAll(`(
EXISTS(SELECT 1 FROM observations_v11 o JOIN tasks_v11 t ON t.id=o.task_id WHERE o.workspace_id=@s.workspace_id AND json_extract(o.data,'$.Agent')=@s.agent AND json_extract(o.data,'$.SessionID')=@s.session_id)
OR EXISTS(SELECT 1 FROM requirement_versions_v11 WHERE json_extract(data,'$.author.agent')=@s.agent AND json_extract(data,'$.author.session_id')=@s.session_id)
OR EXISTS(SELECT 1 FROM notes_v11 WHERE json_extract(data,'$.author.agent')=@s.agent AND json_extract(data,'$.author.session_id')=@s.session_id)
OR EXISTS(SELECT 1 FROM plans_v11 WHERE json_extract(data,'$.author.agent')=@s.agent AND json_extract(data,'$.author.session_id')=@s.session_id)
OR EXISTS(SELECT 1 FROM verifications_v11 v JOIN tasks_v11 t ON t.id=v.task_id WHERE v.workspace_id=@s.workspace_id AND v.agent=@s.agent AND v.session_id=@s.session_id)
)`, "@s", alias)
}

/**
 * 候选先按工作区和时间取主键。任务各自原子删除；独立记录每类至多 256 条。
 * 保留关系只核对当前候选，不复制所有保留任务的正文，也不解压任何正文。
 */
func (s *Store) Prune(ctx context.Context, workspace string, now time.Time) error {
	cutoff := now.Add(-30 * 24 * time.Hour).UTC().Format(time.RFC3339Nano)
	var tasks []string
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		rows, err := q.QueryContext(ctx, `SELECT id FROM tasks_v11 WHERE workspace_id=? AND state IN ('completed','canceled') AND julianday(json_extract(data,'$.ended_at'))<julianday(?) ORDER BY seq LIMIT 8`, workspace, cutoff)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			tasks = append(tasks, id)
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, id := range tasks {
		if err := s.pruneTask(ctx, workspace, id, cutoff, now); err != nil {
			return err
		}
	}
	activeSession := `EXISTS(SELECT 1 FROM task_sessions_v11 s WHERE s.workspace_id=c.workspace_id AND s.agent=c.agent AND s.session_id=c.session_id AND julianday(s.activity_at)>=julianday(?))`
	collections := []struct {
		table, where string
		args         []any
	}{
		{"sources_v11", `julianday(json_extract(c.data,'$.at'))<julianday(?) AND NOT EXISTS(SELECT 1 FROM tasks_v11 WHERE id=c.task_id) AND NOT ` + sourceReferenced("c.id"), []any{cutoff}},
		{"messages_v11", `julianday(json_extract(c.data,'$.at'))<julianday(?) AND NOT EXISTS(SELECT 1 FROM sources_v11 WHERE id=c.id) AND NOT ` + sourceReferenced("c.id"), []any{cutoff}},
		{"verifications_v11", `julianday(json_extract(c.data,'$.started_at'))<julianday(?) AND NOT EXISTS(SELECT 1 FROM tasks_v11 WHERE id=c.task_id) AND NOT EXISTS(SELECT 1 FROM verifications_v11 WHERE json_extract(data,'$.previous_id')=c.id) AND NOT ` + activeSession + ` AND NOT ` + sessionTaskRoot("c"), []any{cutoff, cutoff}},
		{"edits_v11", `EXISTS(SELECT 1 FROM observations_v11 o WHERE o.id=c.before_observation AND julianday(json_extract(o.data,'$.At'))<julianday(?)) AND NOT EXISTS(SELECT 1 FROM observations_v11 o JOIN tasks_v11 t ON t.id=o.task_id WHERE o.id=c.before_observation OR o.id=c.after_observation) AND NOT ` + activeSession + ` AND NOT ` + sessionTaskRoot("c"), []any{cutoff, cutoff}},
		{"file_changes_v11", `julianday(json_extract(c.data,'$.At'))<julianday(?) AND NOT EXISTS(SELECT 1 FROM observations_v11 o JOIN tasks_v11 t ON t.id=o.task_id WHERE o.id=c.observation_id)`, []any{cutoff}},
		{"session_summaries_v11", `julianday(c.delivered_at)<julianday(?) AND NOT ` + activeSession + ` AND NOT ` + sessionTaskRoot("c") + ` AND NOT EXISTS(SELECT 1 FROM messages_v11 m WHERE m.workspace_id=c.workspace_id AND m.agent=c.agent AND m.session_id=c.session_id)`, []any{cutoff, cutoff}},
		{"task_sessions_v11", `julianday(c.activity_at)<julianday(?) AND NOT ` + sessionTaskRoot("c") + ` AND NOT EXISTS(SELECT 1 FROM messages_v11 m WHERE m.workspace_id=c.workspace_id AND m.agent=c.agent AND m.session_id=c.session_id) AND NOT EXISTS(SELECT 1 FROM session_summaries_v11 a WHERE a.workspace_id=c.workspace_id AND a.agent=c.agent AND a.session_id=c.session_id)`, []any{cutoff}},
	}
	for _, table := range []string{"hook_events_v11", "mcp_contexts_v11", "task_calls_v11", "check_feedback_v11"} {
		condition := `julianday(c.at)<julianday(?) AND NOT EXISTS(SELECT 1 FROM task_sessions_v11 s WHERE s.workspace_id=c.workspace_id AND s.agent=c.agent AND s.session_id=c.session_id)`
		if table == "check_feedback_v11" {
			condition = `(c.at='' OR julianday(c.at)<julianday(?)) AND NOT EXISTS(SELECT 1 FROM issue_blocks_v11 WHERE feedback_id=c.id) AND NOT EXISTS(SELECT 1 FROM task_sessions_v11 s WHERE s.workspace_id=c.workspace_id AND s.agent=c.agent AND s.session_id=c.session_id)`
		}
		collections = append(collections, struct {
			table, where string
			args         []any
		}{table, condition, []any{cutoff}})
	}
	collections = append(collections, struct {
		table, where string
		args         []any
	}{"verification_rounds_v11", `NOT EXISTS(SELECT 1 FROM task_sessions_v11 s WHERE s.workspace_id=c.workspace_id AND s.agent=c.agent AND s.session_id=c.session_id) AND NOT EXISTS(SELECT 1 FROM verifications_v11 v WHERE v.workspace_id=c.workspace_id AND v.agent=c.agent AND v.session_id=c.session_id AND v.round_id=c.round_id)`, nil})
	for _, collection := range collections {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		args := append([]any{workspace}, collection.args...)
		query := `SELECT c.rowid FROM ` + collection.table + ` c WHERE c.workspace_id=? AND ` + collection.where + ` ORDER BY c.rowid LIMIT 256`
		var candidates bool
		if err := s.read(ctx, func(ctx context.Context, q queryer) error {
			return q.QueryRowContext(ctx, `SELECT EXISTS(`+query+`)`, args...).Scan(&candidates)
		}); err != nil {
			return err
		}
		if !candidates {
			continue
		}
		if err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `DELETE FROM `+collection.table+` WHERE rowid IN (`+query+`)`, args...)
			return err
		}); err != nil {
			return err
		}
	}
	if err := s.pruneObservations(ctx, workspace, cutoff, now); err != nil {
		return err
	}
	return s.pruneFileSets(ctx)
}

func (s *Store) pruneTask(ctx context.Context, workspace, id, cutoff string, now time.Time) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var expired bool
		err := tx.QueryRowContext(ctx, `SELECT state IN ('completed','canceled') AND julianday(json_extract(data,'$.ended_at'))<julianday(?) FROM tasks_v11 WHERE workspace_id=? AND id=?`, cutoff, workspace, id).Scan(&expired)
		if errors.Is(err, sql.ErrNoRows) || err == nil && !expired {
			return nil
		}
		if err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, `SELECT data FROM plans_v11 WHERE task_id=?`, id)
		if err != nil {
			return err
		}
		var plans []PlanRecord
		for rows.Next() {
			var data string
			var plan PlanRecord
			if err = rows.Scan(&data); err == nil {
				err = json.Unmarshal([]byte(data), &plan)
			}
			if err != nil {
				rows.Close()
				return err
			}
			plans = append(plans, plan)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, plan := range plans {
			expected, err := filepath.Abs(filepath.Join(s.dataDir, "plans", id, fmt.Sprintf("v%d.md", plan.Version)))
			if err != nil || expected != plan.Path {
				return fmt.Errorf("方案文件路径与账本归属不符: %s", plan.ID)
			}
			if err := queueCleanup(ctx, tx, workspace, "plan", plan.Path, contentHash(plan.Content), now); err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`DELETE FROM issue_blocks_v11 WHERE issue_id IN (SELECT json_extract(data,'$.id') FROM issues_v11 WHERE task_id=?)`,
			`DELETE FROM settings_v11 WHERE scope='task' AND scope_id=?`,
			`UPDATE workspaces_v11 SET current_task_id='' WHERE current_task_id=?`,
		} {
			if _, err := tx.ExecContext(ctx, statement, id); err != nil {
				return err
			}
		}
		for _, table := range []string{"requirement_locations_v11", "requirement_versions_v11", "notes_v11", "task_updates_v11", "checks_v11", "finish_requests_v11", "issues_v11", "plans_v11", "workspace_links_v11"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE task_id=?`, id); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tasks_v11 WHERE id=? AND workspace_id=?`, id, workspace)
		return err
	})
}

func (s *Store) pruneObservations(ctx context.Context, workspace, cutoff string, now time.Time) error {
	query := `SELECT c.id FROM observations_v11 c WHERE c.workspace_id=? AND julianday(json_extract(c.data,'$.At'))<julianday(?) AND NOT EXISTS(SELECT 1 FROM tasks_v11 WHERE id=c.task_id) AND NOT ` + observationReferenced("c.id") + ` ORDER BY c.id LIMIT 256`
	var candidate bool
	if err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, `SELECT EXISTS(`+query+`)`, workspace, cutoff).Scan(&candidate)
	}); err != nil {
		return err
	}
	if !candidate {
		return nil
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id,data FROM observations_v11 WHERE id IN (`+query+`)`, workspace, cutoff)
		if err != nil {
			return err
		}
		var observations []Observation
		for rows.Next() {
			var id int64
			var data string
			var observation Observation
			if err = rows.Scan(&id, &data); err == nil {
				err = json.Unmarshal([]byte(data), &observation)
			}
			if err != nil {
				rows.Close()
				return err
			}
			observation.ID = id
			observations = append(observations, observation)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, observation := range observations {
			if err := queueObservationRefs(ctx, tx, workspace, observation, now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM observations_v11 WHERE id=?`, observation.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) pruneFileSets(ctx context.Context) error {
	const query = `SELECT f.id FROM file_sets_v11 f WHERE NOT EXISTS(SELECT 1 FROM observations_v11 WHERE files_set_id=f.id) ORDER BY f.id LIMIT 256`
	var candidate bool
	if err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, `SELECT EXISTS(`+query+`)`).Scan(&candidate)
	}); err != nil {
		return err
	}
	if !candidate {
		return nil
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM file_sets_v11 WHERE id IN (`+query+`)`)
		return err
	})
}
