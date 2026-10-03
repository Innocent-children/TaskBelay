package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

func (s *SQLite) SetTaskArchived(ctx context.Context, mutation ArchiveTaskMutation) (*time.Time, error) {
	if s == nil || s.db == nil || ctx == nil || !mutation.TaskID.IsValid() || mutation.ExpectedRevision == 0 || mutation.Archived && (mutation.ArchivedAt.IsZero() || mutation.ArchivedAt.Location() != time.UTC) {
		return nil, ErrInvalidArgument
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	defer tx.Rollback()
	item, err := loadControlCenterTaskRow(ctx, tx, mutation.TaskID)
	if err != nil {
		return nil, err
	}
	if item.Task.Revision != mutation.ExpectedRevision {
		return nil, ErrRevisionConflict
	}
	if !item.Task.CurrentNode.Terminal() {
		return nil, ErrTaskTerminal
	}
	if mutation.Archived && item.ArchivedAt != nil {
		return cloneTime(item.ArchivedAt), nil
	}
	if !mutation.Archived && item.ArchivedAt == nil {
		return nil, nil
	}
	var archived any
	if mutation.Archived {
		archived = formatTime(mutation.ArchivedAt)
	}
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET archived_at=? WHERE task_id=? AND revision=?`, archived, mutation.TaskID, mutation.ExpectedRevision)
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return nil, ErrRevisionConflict
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrStorageUnavailable
	}
	if !mutation.Archived {
		return nil, nil
	}
	value := mutation.ArchivedAt
	return &value, nil
}

func (s *SQLite) PurgeTask(ctx context.Context, mutation PurgeTaskMutation) error {
	if s == nil || s.db == nil || ctx == nil || !mutation.TaskID.IsValid() || mutation.ExpectedRevision == 0 || mutation.TypedTaskID != mutation.TaskID || !mutation.Irreversible || mutation.Reason == "" || mutation.Reason != strings.TrimSpace(mutation.Reason) || len(mutation.Reason) > domain.MaxReasonBytes {
		return ErrInvalidArgument
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ErrStorageUnavailable
	}
	defer tx.Rollback()
	item, err := loadControlCenterTaskRow(ctx, tx, mutation.TaskID)
	if err != nil {
		return err
	}
	if item.Task.Revision != mutation.ExpectedRevision {
		return ErrRevisionConflict
	}
	if !item.Task.CurrentNode.Terminal() {
		return ErrTaskTerminal
	}
	var claims int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM repository_claims WHERE task_id=?`, mutation.TaskID).Scan(&claims); err != nil {
		return ErrStorageUnavailable
	}
	if claims != 0 {
		return ErrStorageUnavailable
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM branch_rename_operations WHERE task_id=?`, mutation.TaskID); err != nil {
		return ErrStorageUnavailable
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM action_operations WHERE task_id=?`, mutation.TaskID); err != nil {
		return ErrStorageUnavailable
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM relocation_operations WHERE task_id=?`, mutation.TaskID); err != nil {
		return ErrStorageUnavailable
	}
	events, err := tx.ExecContext(ctx, `DELETE FROM task_events WHERE task_id=?`, mutation.TaskID)
	if err != nil {
		return ErrStorageUnavailable
	}
	eventCount, _ := events.RowsAffected()
	if eventCount != int64(item.Task.Revision) {
		return ErrStorageUnavailable
	}
	claimsResult, err := tx.ExecContext(ctx, `DELETE FROM repository_claims WHERE task_id=?`, mutation.TaskID)
	if err != nil {
		return ErrStorageUnavailable
	}
	claimCount, _ := claimsResult.RowsAffected()
	if claimCount != 0 {
		return ErrStorageUnavailable
	}
	taskResult, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE task_id=? AND revision=?`, mutation.TaskID, mutation.ExpectedRevision)
	if err != nil {
		return ErrStorageUnavailable
	}
	taskCount, _ := taskResult.RowsAffected()
	if taskCount != 1 {
		return ErrRevisionConflict
	}
	if err := tx.Commit(); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}

func loadControlCenterTaskRow(ctx context.Context, tx *sql.Tx, id domain.ID) (ControlCenterTask, error) {
	item, err := scanControlCenterTask(tx.QueryRowContext(ctx, `SELECT task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at,archived_at FROM tasks WHERE task_id=?`, id))
	if err == sql.ErrNoRows {
		return ControlCenterTask{}, ErrTaskNotFound
	}
	if err != nil {
		return ControlCenterTask{}, ErrStorageUnavailable
	}
	return item, nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
