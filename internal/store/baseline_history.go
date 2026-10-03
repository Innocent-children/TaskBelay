package store

import (
	"context"
	"database/sql"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

// Check the exact preceding snapshot in the same CAS transaction. Old references
// are append-only; a valid revision chain alone must not permit rewriting history.
func validateBaselineHistoryMutation(ctx context.Context, tx *sql.Tx, m TaskMutation) error {
	if m.ExpectedRevision == 0 {
		return nil
	}
	prior, err := scanStoredTask(tx.QueryRowContext(ctx, `SELECT task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at FROM tasks WHERE task_id=?`, m.Task.TaskID))
	if err != nil {
		return storageFailure(err, "The preceding Task could not be read for its baseline history transaction.")
	}
	if prior.Revision != m.ExpectedRevision {
		return ErrRevisionConflict
	}
	if len(m.Task.BaselineHistory) < len(prior.BaselineHistory) {
		return domain.WithExplanation(ErrInvalidArgument, "A saved baseline reference cannot be removed or rewritten.")
	}
	for index, ref := range prior.BaselineHistory {
		if m.Task.BaselineHistory[index] != ref {
			return domain.WithExplanation(ErrInvalidArgument, "A saved baseline reference cannot be removed or rewritten.")
		}
	}
	return nil
}
