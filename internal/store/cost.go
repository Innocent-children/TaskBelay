package store

import (
	"context"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

type ControlUsage struct {
	Calls     int64 `json:"calls"`
	WallMS    int64 `json:"wall_ms"`
	UnknownMS int64 `json:"unknown_ms"`
	Pending   int64 `json:"pending"`
}

func (s *Store) BeginControlCall(ctx context.Context, id string, pid int, owner string) error {
	_, e := s.db.ExecContext(ctx, "INSERT INTO control_calls(call_id,started_at,completed_ms,owner_pid,owner_identity) VALUES (?,?,NULL,?,?)", id, time.Now().UTC().Format(time.RFC3339Nano), pid, owner)
	return e
}
func (s *Store) EndControlCall(ctx context.Context, id string, ms int64) error {
	return s.endControlCall(ctx, id, ms, 0)
}
func (s *Store) endControlCall(ctx context.Context, id string, ms, unknown int64) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, "UPDATE control_calls SET completed_ms=? WHERE call_id=? AND completed_ms IS NULL", ms, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 1 {
		if _, e = tx.ExecContext(ctx, "UPDATE control_totals SET calls=calls+1,wall_ms=wall_ms+?,unknown_ms=unknown_ms+?", ms, unknown); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, "DELETE FROM control_calls WHERE completed_ms IS NOT NULL AND rowid NOT IN (SELECT rowid FROM control_calls ORDER BY rowid DESC LIMIT 256)"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ControlCost(ctx context.Context, alive func(int, string) bool) (ControlUsage, error) {
	var u ControlUsage
	e := s.db.QueryRowContext(ctx, "SELECT calls,wall_ms,unknown_ms FROM control_totals").Scan(&u.Calls, &u.WallMS, &u.UnknownMS)
	if e != nil {
		return u, e
	}
	rows, e := s.db.QueryContext(ctx, "SELECT call_id,started_at,owner_pid,owner_identity FROM control_calls WHERE completed_ms IS NULL")
	if e != nil {
		return u, e
	}
	defer rows.Close()
	dead := map[string]int64{}
	for rows.Next() {
		var id, start, identity string
		var pid int
		if e = rows.Scan(&id, &start, &pid, &identity); e != nil {
			return u, e
		}
		at, e := time.Parse(time.RFC3339Nano, start)
		if e != nil {
			return u, domain.Fail("STORAGE_CORRUPT", "", "invalid retained control-call timestamp")
		}
		duration := time.Since(at).Milliseconds()
		if duration < 0 {
			duration = 0
		}
		if alive != nil && !alive(pid, identity) {
			// The actual end is unknown. The interval through first detection is
			// a conservative estimate, which may include offline time.
			dead[id] = duration
			continue
		}
		u.Pending++
		u.Calls++
		u.WallMS += duration
	}
	err := rows.Err()
	rows.Close()
	if err != nil {
		return u, err
	}
	for id, uncertain := range dead {
		if e = s.endControlCall(ctx, id, 0, uncertain); e != nil {
			return u, e
		}
	}
	if len(dead) > 0 {
		// Re-read after settlement: another process may have settled the same call.
		return s.ControlCost(ctx, alive)
	}
	return u, nil
}
