package domain

import "time"

type LastOperation struct {
	OperationID   ID            `json:"operation_id"`
	Kind          OperationKind `json:"kind"`
	ActionID      *ID           `json:"action_id"`
	FromRevision  uint64        `json:"from_revision"`
	ToRevision    uint64        `json:"to_revision"`
	PayloadDigest Digest        `json:"payload_digest"`
	CommittedAt   time.Time     `json:"committed_at"`
}

func (o LastOperation) Validate() error {
	if validateID(o.OperationID) != nil || !o.Kind.IsValid() ||
		o.ToRevision == 0 || o.ToRevision != o.FromRevision+1 || validateDigest(o.PayloadDigest) != nil ||
		validateUTC(o.CommittedAt) != nil {
		return ErrInvalidArgument
	}
	switch o.Kind {
	case OperationOpenTask:
		if o.FromRevision != 0 || o.ActionID != nil {
			return ErrInvalidArgument
		}
	case OperationApplyAction, OperationPrepareFileChange, OperationObserveWorkspace:
		if o.FromRevision == 0 || o.ActionID == nil || validateID(*o.ActionID) != nil {
			return ErrInvalidArgument
		}
	case OperationCancelTask, OperationPrepareTaskRelocation, OperationPrepareTaskBranchRename, OperationAbandonTask:
		if o.FromRevision == 0 || o.ActionID != nil {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	return nil
}
