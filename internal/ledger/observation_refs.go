package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

func (s *Store) StageRefCleanup(ctx context.Context, ref gitstate.SnapshotRef, ready time.Time) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var conflicts bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cleanup_items_v11 WHERE kind='ref' AND target=? AND (workspace_id!=? OR (expected!=? AND NOT (?='commit' AND expected='')))) OR EXISTS(SELECT 1 FROM observations_v11 WHERE json_extract(data,'$.SnapshotRef')=? OR json_extract(data,'$.EvidenceRef')=?)`, ref.Ref, ref.WorkspaceID, ref.Object, ref.Type, ref.Ref, ref.Ref).Scan(&conflicts); err != nil {
			return err
		}
		if conflicts {
			return fmt.Errorf("快照引用已有其他归属: %s", ref.Ref)
		}
		if err := queueCleanup(ctx, tx, ref.WorkspaceID, "ref", ref.Ref, ref.Object, ready); err != nil {
			return err
		}
		if ref.Type == "commit" {
			_, err := tx.ExecContext(ctx, `DELETE FROM cleanup_items_v11 WHERE workspace_id=? AND kind='ref' AND target=? AND expected=''`, ref.WorkspaceID, ref.Ref)
			return err
		}
		return nil
	})
}

func observationRefs(observation Observation) []gitstate.SnapshotRef {
	return []gitstate.SnapshotRef{
		{Ref: observation.SnapshotRef, Object: observation.SnapshotObject, Type: observation.SnapshotType, WorkspaceID: observation.ReferenceWorkspace},
		{Ref: observation.EvidenceRef, Object: observation.EvidenceObject, Type: observation.EvidenceType, WorkspaceID: observation.ReferenceWorkspace},
	}
}

func publishObservationRefs(ctx context.Context, tx *sql.Tx, observation Observation) error {
	for _, ref := range observationRefs(observation) {
		if ref.Ref == "" {
			continue
		}
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Type != "tree" || ref.WorkspaceID != observation.WorkspaceID {
			return fmt.Errorf("观察引用归属或类型不符: %s", ref.Ref)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM cleanup_items_v11 WHERE workspace_id=? AND kind='ref' AND target=? AND expected=? AND julianday(ready_at)>julianday(?)`, ref.WorkspaceID, ref.Ref, ref.Object, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("观察引用没有有效的预清理登记: %s", ref.Ref)
		}
	}
	return nil
}

func queueObservationRefs(ctx context.Context, tx *sql.Tx, workspace string, observation Observation, now time.Time) error {
	for _, ref := range observationRefs(observation) {
		if ref.Ref == "" {
			continue
		}
		if ref.Object != "" && (ref.WorkspaceID != workspace || ref.Validate() != nil) {
			return fmt.Errorf("观察 %d 引用身份无效", observation.ID)
		}
		// schema 10 观察没有对象字段；空 expected 只允许释放经元数据核对的旧 commit。
		if err := queueCleanup(ctx, tx, workspace, "ref", ref.Ref, ref.Object, now); err != nil {
			return err
		}
	}
	return nil
}
