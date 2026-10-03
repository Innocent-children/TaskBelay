package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"github.com/Innocent-children/taskbelay/internal/domain"
)

// Preparation and resolution are part of the same transactions as their Task
// snapshots and events. Audit rows survive resolution and later renames.
func writeBranchRename(ctx context.Context, tx *sql.Tx, m TaskMutation) error {
	if m.Event.Kind == domain.OperationPrepareTaskBranchRename {
		if m.Task.BranchRename == nil {
			return ErrInvalidArgument
		}
		raw, err := json.Marshal(m.Task.BranchRename)
		if err != nil {
			return ErrInvalidArgument
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO branch_rename_operations(rename_id,task_id,request_id,preparation,resolved_revision,resolution) VALUES(?,?,?,?,NULL,NULL)`, m.Task.BranchRename.RenameID, m.Task.TaskID, m.Event.RequestID, raw)
		if err != nil {
			return storageFailure(err, "The branch-rename audit preparation could not be saved.")
		}
		return nil
	}
	if m.BranchRenameChoice != "" || m.Claim == ClaimRelease {
		choice := m.BranchRenameChoice
		if m.Claim == ClaimRelease {
			choice = "task_cancelled"
		}
		if choice != "complete" && choice != "cancel" && choice != "task_cancelled" || m.Task.BranchRename != nil {
			return ErrInvalidArgument
		}
		result, err := tx.ExecContext(ctx, `UPDATE branch_rename_operations SET resolved_revision=?,resolution=? WHERE task_id=? AND resolved_revision IS NULL`, m.Task.Revision, choice, m.Task.TaskID)
		if err != nil {
			return storageFailure(err, "The branch-rename resolution could not be saved.")
		}
		count, _ := result.RowsAffected()
		if choice != "task_cancelled" && count != 1 {
			return ErrRevisionConflict
		}
	}
	return nil
}

func verifyBranchRenameHistory(ctx context.Context, q queryer, task domain.ProcessTask, events []TaskEvent) error {
	var version string
	if err := q.QueryRowContext(ctx, `SELECT version FROM schema_metadata`).Scan(&version); err != nil {
		return ErrStorageUnavailable
	}
	if version == priorDatabaseSchemaVersion {
		if task.BranchRename != nil {
			return ErrStorageUnavailable
		}
		return nil
	}
	rows, err := q.QueryContext(ctx, `SELECT rename_id,request_id,preparation,resolved_revision,resolution FROM branch_rename_operations WHERE task_id=?`, task.TaskID)
	if err != nil {
		return ErrStorageUnavailable
	}
	defer rows.Close()
	byRequest := map[domain.ID]TaskEvent{}
	byRevision := map[uint64]TaskEvent{}
	expected := 0
	for _, event := range events {
		byRequest[event.RequestID] = event
		byRevision[event.Revision] = event
		if event.Kind == domain.OperationPrepareTaskBranchRename {
			expected++
		}
	}
	count, pending := 0, 0
	for rows.Next() {
		var id, request string
		var raw []byte
		var resolved sql.NullInt64
		var choice sql.NullString
		if rows.Scan(&id, &request, &raw, &resolved, &choice) != nil {
			return ErrStorageUnavailable
		}
		var preparation domain.TaskBranchRename
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&preparation) != nil || preparation.Validate() != nil || id != string(preparation.RenameID) {
			return ErrStorageUnavailable
		}
		canonical, _ := json.Marshal(preparation)
		if !bytes.Equal(raw, canonical) {
			return ErrStorageUnavailable
		}
		digest := sha256.Sum256(raw)
		event, found := byRequest[domain.ID(request)]
		if !found || event.Kind != domain.OperationPrepareTaskBranchRename || event.PayloadDigest != domain.Digest(hex.EncodeToString(digest[:])) || event.SourceNode != preparation.ResumeNode || event.DestinationNode != domain.NodeBlocked || !event.CreatedAt.Equal(preparation.PreparedAt) {
			return ErrStorageUnavailable
		}
		count++
		if !resolved.Valid {
			pending++
			saved, _ := json.Marshal(task.BranchRename)
			if choice.Valid || task.LastOperation == nil || task.LastOperation.OperationID != event.RequestID || task.Revision != event.Revision || !bytes.Equal(saved, raw) {
				return ErrStorageUnavailable
			}
			continue
		}
		resolution, found := byRevision[uint64(resolved.Int64)]
		if !choice.Valid || !found || resolved.Int64 != int64(event.Revision+1) || resolution.SourceNode != domain.NodeBlocked {
			return ErrStorageUnavailable
		}
		if choice.String == "task_cancelled" {
			if resolution.DestinationNode != domain.NodeCancelled || resolution.Kind != domain.OperationCancelTask && resolution.Kind != domain.OperationAbandonTask {
				return ErrStorageUnavailable
			}
		} else if choice.String == "complete" || choice.String == "cancel" {
			if resolution.Kind != domain.OperationApplyAction || resolution.DestinationNode != preparation.ResumeNode || resolution.TransitionReason != "Branch rename "+id+" "+choice.String+": "+preparation.SourceBranch+" -> "+preparation.TargetBranch {
				return ErrStorageUnavailable
			}
		} else {
			return ErrStorageUnavailable
		}
	}
	if rows.Err() != nil || count != expected || pending > 1 || (task.BranchRename != nil) != (pending == 1) {
		return ErrStorageUnavailable
	}
	return nil
}
