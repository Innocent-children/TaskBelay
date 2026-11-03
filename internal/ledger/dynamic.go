package ledger

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) ClaimDynamic(ctx context.Context, task, version, owner string, until time.Time, refresh bool) (bool, error) {
	claimed := false
	err := s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `INSERT INTO dynamic_jobs_v12 VALUES(?,?,?,?, 'running')
ON CONFLICT(task_id) DO UPDATE SET input_version=excluded.input_version,owner=excluded.owner,expires_at=excluded.expires_at,state='running'
WHERE (dynamic_jobs_v12.state!='running' OR dynamic_jobs_v12.expires_at<?)
AND (dynamic_jobs_v12.input_version!=excluded.input_version OR dynamic_jobs_v12.state='running' OR ?)`, task, version, owner, until.UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), refresh)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		claimed = n == 1
		return err
	})
	return claimed, err
}

func (s *Store) FinishDynamic(ctx context.Context, task, owner, state string) error {
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "UPDATE dynamic_jobs_v12 SET state=? WHERE task_id=? AND owner=?", state, task, owner)
		return err
	})
}

func (s *Store) ValidationRunning(ctx context.Context, workspace string) (bool, error) {
	var running bool
	err := s.read(ctx, func(ctx context.Context, q queryer) error {
		return q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM verifications_v12 WHERE workspace_id=? AND json_extract(data,'$.state')='running' AND json_extract(data,'$.scope.Validation')=1)`, workspace).Scan(&running)
	})
	return running, err
}
