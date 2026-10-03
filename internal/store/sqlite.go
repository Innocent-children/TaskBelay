package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/workflow"
	_ "modernc.org/sqlite"
)

type SQLite struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*SQLite, error) {
	if path == "" {
		return nil, ErrInvalidArgument
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, ErrInvalidArgument
	}
	if info, err := os.Stat(absolute); err == nil {
		if !info.Mode().IsRegular() {
			return nil, ErrStorageUnavailable
		}
		if info.Size() > 0 {
			if err := preflightExisting(ctx, absolute); err != nil {
				return nil, err
			}
		} else if found, err := hasSQLiteSidecar(absolute); err != nil {
			return nil, ErrStorageUnavailable
		} else if found {
			return nil, ErrSchemaUnsupported
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, ErrStorageUnavailable
	}
	dsn := dataSource(absolute, false)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, ErrStorageUnavailable
	}
	if err := bootstrapCurrentSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := preflightDatabase(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}
func dataSource(path string, readOnly bool) string {
	clean := filepath.Clean(path)
	uriPath := filepath.ToSlash(clean)
	if filepath.VolumeName(clean) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := &url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Set("_foreign_keys", "on")
	q.Set("_busy_timeout", strconv.FormatInt(domain.SQLiteBusyTimeout.Milliseconds(), 10))
	if readOnly {
		q.Set("mode", "ro")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func hasSQLiteSidecar(path string) (bool, error) {
	for _, suffix := range []string{"-journal", "-shm", "-wal"} {
		info, err := os.Lstat(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if !info.Mode().IsRegular() || info.Size() > 0 {
			return true, nil
		}
	}
	return false, nil
}
func preflightExisting(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", dataSource(path, true))
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		return ErrSchemaUnsupported
	}
	return preflightDatabase(ctx, db)
}

/**
 * Schema and related rows must come from one SQLite snapshot while other Core
 * processes may commit to the same database.
 */
func preflightDatabase(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ErrStorageUnavailable
	}
	defer tx.Rollback()
	return preflightSnapshot(ctx, tx)
}

func preflightSnapshot(ctx context.Context, q queryer) error {
	if err := verifyReadableSchema(ctx, q); err != nil {
		return err
	}
	integrity, err := q.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return ErrStorageUnavailable
	}
	invalid := integrity.Next()
	failed := integrity.Err()
	integrity.Close()
	if invalid || failed != nil {
		return ErrStorageUnavailable
	}
	rows, err := q.QueryContext(ctx, `SELECT task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at,archived_at FROM tasks`)
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer rows.Close()
	standard := workflow.StandardProcess().Reference
	tasks := map[string]preflightTask{}
	for rows.Next() {
		var taskID, originHost, processID, digest, node, worktreeInstance, createdAt, updatedAt string
		var archivedAt sql.NullString
		var revision int64
		var snapshot []byte
		if err := rows.Scan(&taskID, &originHost, &processID, &digest, &node, &revision, &worktreeInstance, &snapshot, &createdAt, &updatedAt, &archivedAt); err != nil {
			return ErrStorageUnavailable
		}
		if processID != string(standard.ID) || digest != string(standard.DefinitionDigest) {
			return ErrProcessUnsupported
		}
		task, err := decodeTask(snapshot)
		if err != nil {
			return err
		}
		if string(task.TaskID) != taskID || string(task.OriginHost) != originHost || task.Process != standard || string(task.CurrentNode) != node || int64(task.Revision) != revision || string(task.Repository.WorktreeInstanceDigest) != worktreeInstance || formatTime(task.CreatedAt) != createdAt || formatTime(task.UpdatedAt) != updatedAt {
			return ErrStorageUnavailable
		}
		archive, err := decodeArchiveTime(nullableString(archivedAt))
		if err != nil || archive != nil && !task.CurrentNode.Terminal() {
			return ErrStorageUnavailable
		}
		if _, exists := tasks[taskID]; exists {
			return ErrStorageUnavailable
		}
		expectedClaims := map[string]string{}
		for _, claim := range repositoryClaims(task) {
			expectedClaims[string(claim.identity)] = claim.root
		}
		tasks[taskID] = preflightTask{task: task, originHost: originHost, terminal: task.CurrentNode.Terminal(), expectedClaims: expectedClaims}
	}
	if rows.Err() != nil || rows.Close() != nil {
		return ErrStorageUnavailable
	}
	operations, err := q.QueryContext(ctx, `SELECT task_id,operation_id,process_id,process_definition_digest,source_node,expected_revision,action_id,action_kind,repository_binding_digest,issuance_identity_digest,issuance_history_digest,issuance_content_digest,payload,payload_digest,prepared_at,applied_revision FROM action_operations`)
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer operations.Close()
	seenOperations := map[domain.ID]bool{}
	for operations.Next() {
		operation, scanErr := scanActionOperation(operations)
		if scanErr != nil || seenOperations[operation.TaskID] {
			return ErrStorageUnavailable
		}
		item, exists := tasks[string(operation.TaskID)]
		if !exists || workflow.ValidateActionCommit(item.task, operation.Commit) != nil {
			return ErrStorageUnavailable
		}
		if operation.AppliedRevision == nil {
			if !actionOperationMatchesCurrentTask(item.task, operation.Commit) {
				return ErrStorageUnavailable
			}
		} else if !operation.RecordedBy(item.task) {
			return ErrStorageUnavailable
		}
		seenOperations[operation.TaskID] = true
	}
	if operations.Err() != nil || operations.Close() != nil {
		return ErrStorageUnavailable
	}
	relocations, err := q.QueryContext(ctx, `SELECT relocation_id,task_id,request_id,source_binding_digest,prepared_at,resolved_revision FROM relocation_operations`)
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer relocations.Close()
	unresolved := map[string]preflightRelocation{}
	relocationHistory := map[string][]preflightRelocation{}
	for relocations.Next() {
		var relocationID, taskID, requestID, sourceDigest, preparedAt string
		var resolved sql.NullInt64
		if relocations.Scan(&relocationID, &taskID, &requestID, &sourceDigest, &preparedAt, &resolved) != nil {
			return ErrStorageUnavailable
		}
		prepared, parseErr := time.Parse(time.RFC3339Nano, preparedAt)
		task, exists := tasks[taskID]
		if !exists || !domain.ID(relocationID).IsValid() || !domain.ID(requestID).IsValid() || !domain.Digest(sourceDigest).IsValid() || parseErr != nil || prepared.Location() != time.UTC {
			return ErrStorageUnavailable
		}
		record := preflightRelocation{
			relocationID:        relocationID,
			requestID:           requestID,
			sourceBindingDigest: sourceDigest,
			preparedAt:          prepared,
		}
		relocationHistory[taskID] = append(relocationHistory[taskID], record)
		if resolved.Valid {
			if resolved.Int64 < 1 || uint64(resolved.Int64) > task.task.Revision {
				return ErrStorageUnavailable
			}
			record.resolvedRevision = uint64(resolved.Int64)
			history := relocationHistory[taskID]
			history[len(history)-1] = record
			relocationHistory[taskID] = history
			continue
		}
		if unresolved[taskID].relocationID != "" {
			return ErrStorageUnavailable
		}
		unresolved[taskID] = record
	}
	if relocations.Err() != nil || relocations.Close() != nil {
		return ErrStorageUnavailable
	}
	for taskID, item := range tasks {
		pending := unresolved[taskID]
		if (item.task.Relocation == nil) != (pending.relocationID == "") || item.task.Relocation != nil &&
			(string(item.task.Relocation.RelocationID) != pending.relocationID ||
				string(item.task.Relocation.SourceBindingDigest) != pending.sourceBindingDigest ||
				!item.task.Relocation.PreparedAt.Equal(pending.preparedAt) || item.task.LastOperation == nil ||
				item.task.LastOperation.Kind != domain.OperationPrepareTaskRelocation ||
				string(item.task.LastOperation.OperationID) != pending.requestID) {
			return ErrStorageUnavailable
		}
	}
	claims, err := q.QueryContext(ctx, `SELECT worktree_instance_digest,canonical_worktree_root,task_id,origin_host FROM repository_claims`)
	if err != nil {
		return ErrSchemaUnsupported
	}
	defer claims.Close()
	claimCount := map[string]int{}
	for claims.Next() {
		var worktreeInstance, root, taskID, originHost string
		if err := claims.Scan(&worktreeInstance, &root, &taskID, &originHost); err != nil {
			claims.Close()
			return ErrStorageUnavailable
		}
		task, exists := tasks[taskID]
		expectedRoot, claimed := task.expectedClaims[worktreeInstance]
		if !exists || task.terminal || !claimed || expectedRoot != root || task.originHost != originHost {
			claims.Close()
			return ErrStorageUnavailable
		}
		claimCount[taskID]++
		if claimCount[taskID] > len(task.expectedClaims) {
			claims.Close()
			return ErrStorageUnavailable
		}
	}
	if claims.Err() != nil || claims.Close() != nil {
		return ErrStorageUnavailable
	}
	for taskID, task := range tasks {
		if task.terminal && claimCount[taskID] != 0 || !task.terminal && claimCount[taskID] != len(task.expectedClaims) {
			return ErrStorageUnavailable
		}
		events, err := loadTaskEvents(ctx, q, domain.ID(taskID))
		if err != nil {
			return ErrStorageUnavailable
		}
		traversals := make([]workflow.CommittedTraversal, len(events))
		eventsByRequest := make(map[domain.ID][]TaskEvent, len(events))
		eventsByRevision := make(map[uint64]TaskEvent, len(events))
		for index, event := range events {
			if !storedTaskEventValid(task.task, event) {
				return ErrStorageUnavailable
			}
			eventsByRequest[event.RequestID] = append(eventsByRequest[event.RequestID], event)
			eventsByRevision[event.Revision] = event
			traversals[index] = workflow.CommittedTraversal{Revision: event.Revision, Kind: event.Kind, Source: event.SourceNode, Destination: event.DestinationNode, TransitionID: event.TransitionID, Reason: event.TransitionReason, CreatedAt: event.CreatedAt}
		}
		for _, relocation := range relocationHistory[taskID] {
			matches := eventsByRequest[domain.ID(relocation.requestID)]
			if len(matches) != 1 {
				return ErrStorageUnavailable
			}
			event := matches[0]
			if event.Kind != domain.OperationPrepareTaskRelocation ||
				event.DestinationNode != domain.NodeBlocked || !event.SourceNode.Normal() ||
				event.ObservedBindingDigest == nil || string(*event.ObservedBindingDigest) != relocation.sourceBindingDigest ||
				!event.CreatedAt.Equal(relocation.preparedAt) {
				return ErrStorageUnavailable
			}
			if relocation.resolvedRevision != 0 {
				resolvedEvent, exists := eventsByRevision[relocation.resolvedRevision]
				if !exists || relocation.resolvedRevision != event.Revision+1 || !relocationResolutionEvent(resolvedEvent) {
					return ErrStorageUnavailable
				}
			}
		}
		if err := verifyBranchRenameHistory(ctx, q, task.task, events); err != nil {
			return err
		}
		if !workflow.ProjectControlCenterGraph(task.task, traversals).Safe {
			return ErrStorageUnavailable
		}
	}
	return nil
}

func storedTaskEventValid(task domain.ProcessTask, event TaskEvent) bool {
	if !event.EventID.IsValid() || event.TaskID != task.TaskID || event.Revision == 0 || event.Revision > task.Revision ||
		!event.Kind.IsValid() || !event.SourceNode.IsValid() || !event.DestinationNode.IsValid() ||
		!event.RequestID.IsValid() || !event.PayloadDigest.IsValid() || event.CreatedAt.IsZero() || event.CreatedAt.Location() != time.UTC ||
		len(event.RepositoryDeltaPaths) > domain.MaxRepositoryDeltaPaths {
		return false
	}
	if event.ActionID != nil && !event.ActionID.IsValid() {
		return false
	}
	if event.ObservedBindingDigest != nil && !event.ObservedBindingDigest.IsValid() {
		return false
	}
	if event.TransitionID != nil && !event.TransitionID.IsValid() {
		return false
	}
	for index, path := range event.RepositoryDeltaPaths {
		if task.ValidateRepositoryPath(path) != nil || index > 0 && event.RepositoryDeltaPaths[index-1] >= path {
			return false
		}
	}
	return true
}

func relocationResolutionEvent(event TaskEvent) bool {
	switch event.Kind {
	case domain.OperationApplyAction:
		return event.SourceNode == domain.NodeBlocked && event.DestinationNode.Normal()
	case domain.OperationCancelTask, domain.OperationAbandonTask:
		return event.DestinationNode == domain.NodeCancelled
	default:
		return false
	}
}

type preflightTask struct {
	task           domain.ProcessTask
	originHost     string
	terminal       bool
	expectedClaims map[string]string
}

type preflightRelocation struct {
	relocationID        string
	requestID           string
	sourceBindingDigest string
	preparedAt          time.Time
	resolvedRevision    uint64
}

func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return ErrStorageUnavailable
	}
	return nil
}
func (s *SQLite) LoadTask(ctx context.Context, id domain.ID) (domain.ProcessTask, error) {
	return s.load(ctx, `SELECT task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at FROM tasks WHERE task_id=?`, string(id))
}
func (s *SQLite) LoadActiveTask(ctx context.Context, identity domain.Digest) (domain.ProcessTask, error) {
	return s.load(ctx, `SELECT t.task_id,t.origin_host,t.process_id,t.process_definition_digest,t.current_node,t.revision,t.worktree_instance_digest,t.snapshot,t.created_at,t.updated_at FROM repository_claims c JOIN tasks t ON t.task_id=c.task_id WHERE c.worktree_instance_digest=?`, string(identity))
}
func (s *SQLite) LoadActiveTaskByCanonicalRoot(ctx context.Context, root string) (domain.ProcessTask, error) {
	return s.load(ctx, `SELECT t.task_id,t.origin_host,t.process_id,t.process_definition_digest,t.current_node,t.revision,t.worktree_instance_digest,t.snapshot,t.created_at,t.updated_at FROM repository_claims c JOIN tasks t ON t.task_id=c.task_id WHERE c.canonical_worktree_root=?`, root)
}
func (s *SQLite) load(ctx context.Context, query, arg string) (domain.ProcessTask, error) {
	if s == nil || s.db == nil {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The Task store is not open.")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ProcessTask{}, storageFailure(err, "The Task read transaction could not be started.")
	}
	defer tx.Rollback()
	task, err := scanStoredTask(tx.QueryRowContext(ctx, query, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProcessTask{}, domain.WithExplanation(ErrTaskNotFound, "No saved Task matches the requested Task identity or workspace claim.")
	}
	if err != nil {
		return domain.ProcessTask{}, storageFailure(err, "The Task row could not be read or decoded from storage.")
	}
	return task, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanStoredTask(row rowScanner) (domain.ProcessTask, error) {
	var taskID, host, processID, digest, node, identity, created, updated string
	var revision int64
	var snapshot []byte
	err := row.Scan(&taskID, &host, &processID, &digest, &node, &revision, &identity, &snapshot, &created, &updated)
	if err != nil {
		return domain.ProcessTask{}, err
	}
	task, err := decodeTask(snapshot)
	if err != nil {
		return domain.ProcessTask{}, err
	}
	if taskID != string(task.TaskID) || host != string(task.OriginHost) || processID != string(task.Process.ID) || digest != string(task.Process.DefinitionDigest) || node != string(task.CurrentNode) || revision != int64(task.Revision) || identity != string(task.Repository.WorktreeInstanceDigest) || created != formatTime(task.CreatedAt) || updated != formatTime(task.UpdatedAt) {
		return domain.ProcessTask{}, domain.WithExplanation(ErrStorageUnavailable, "The Task row identity, revision, workspace or timestamps disagree with its saved snapshot.")
	}
	return task, nil
}
func (s *SQLite) CommitTask(ctx context.Context, m TaskMutation) error {
	if s == nil || s.db == nil || validateMutation(m) != nil {
		return domain.WithExplanation(ErrInvalidArgument, "The Task mutation cannot be committed because the store is closed or the mutation is invalid.")
	}
	snapshot, err := encodeTask(m.Task)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return storageFailure(err, "The Task transaction could not be started.")
	}
	defer tx.Rollback()
	if err := clearSupersededActionOperation(ctx, tx, m); err != nil {
		return err
	}
	if err := writeTaskMutation(ctx, tx, m, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storageFailure(err, "The Task transaction commit did not return success; read the saved Task before retrying.")
	}
	return nil
}

func clearSupersededActionOperation(ctx context.Context, tx *sql.Tx, mutation TaskMutation) error {
	if mutation.Event.Kind == domain.OperationOpenTask {
		return nil
	}
	operation, found, err := loadActionOperationTx(ctx, tx, mutation.Task.TaskID)
	if err != nil || !found {
		return err
	}
	if operation.AppliedRevision == nil {
		if mutation.Event.Kind != domain.OperationCancelTask && mutation.Event.Kind != domain.OperationAbandonTask {
			return domain.WithExplanation(ErrRevisionConflict, "A pending Action operation must be recovered before this Task mutation can replace it.")
		}
	} else {
		current, loadErr := scanStoredTask(tx.QueryRowContext(ctx, `SELECT task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at FROM tasks WHERE task_id=?`, mutation.Task.TaskID))
		if loadErr != nil || !operation.RecordedBy(current) {
			return domain.WithExplanation(ErrStorageUnavailable, "The retained Action operation is not recorded by the current stored Task.")
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM action_operations WHERE task_id=?`, mutation.Task.TaskID); err != nil {
		return domain.WithExplanation(ErrStorageUnavailable, "The previous Action operation could not be removed from storage.")
	}
	return nil
}

func writeTaskMutation(ctx context.Context, tx *sql.Tx, m TaskMutation, snapshot []byte) error {
	if err := writeBranchRename(ctx, tx, m); err != nil {
		return err
	}
	if err := validateBaselineHistoryMutation(ctx, tx, m); err != nil {
		return err
	}
	var err error
	if m.ExpectedRevision == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO tasks(task_id,origin_host,process_id,process_definition_digest,current_node,revision,worktree_instance_digest,snapshot,created_at,updated_at,archived_at) VALUES(?,?,?,?,?,?,?,?,?,?,NULL)`, m.Task.TaskID, m.Task.OriginHost, m.Task.Process.ID, m.Task.Process.DefinitionDigest, m.Task.CurrentNode, m.Task.Revision, m.Task.Repository.WorktreeInstanceDigest, snapshot, formatTime(m.Task.CreatedAt), formatTime(m.Task.UpdatedAt))
	} else {
		result, e := tx.ExecContext(ctx, `UPDATE tasks SET current_node=?,revision=?,worktree_instance_digest=?,snapshot=?,updated_at=? WHERE task_id=? AND revision=? AND process_id=? AND process_definition_digest=?`, m.Task.CurrentNode, m.Task.Revision, m.Task.Repository.WorktreeInstanceDigest, snapshot, formatTime(m.Task.UpdatedAt), m.Task.TaskID, m.ExpectedRevision, m.Task.Process.ID, m.Task.Process.DefinitionDigest)
		err = e
		if err == nil {
			n, _ := result.RowsAffected()
			if n != 1 {
				return domain.WithExplanation(ErrRevisionConflict, "The Task revision changed before the snapshot update was written.")
			}
		}
	}
	if err != nil {
		return storageFailure(err, "The Task snapshot could not be inserted or updated in tasks.")
	}
	if err := insertEvent(ctx, tx, m.Event); err != nil {
		return err
	}
	if err := applyClaim(ctx, tx, m); err != nil {
		return err
	}
	if m.Event.Kind == domain.OperationPrepareTaskRelocation {
		if m.Task.Relocation == nil {
			return domain.WithExplanation(ErrInvalidArgument, "A relocation-preparation mutation must include its prepared relocation record.")
		}
		r := m.Task.Relocation
		if _, err := tx.ExecContext(ctx, `INSERT INTO relocation_operations(task_id,relocation_id,request_id,source_binding_digest,prepared_at,resolved_revision) VALUES(?,?,?,?,?,NULL)`, m.Task.TaskID, r.RelocationID, m.Event.RequestID, r.SourceBindingDigest, formatTime(r.PreparedAt)); err != nil {
			return domain.WithExplanation(ErrStorageUnavailable, "The relocation preparation could not be inserted into relocation_operations.")
		}
	}
	if m.Claim == ClaimReplace || m.Claim == ClaimRelease {
		if _, err := tx.ExecContext(ctx, `UPDATE relocation_operations SET resolved_revision=? WHERE task_id=? AND resolved_revision IS NULL`, m.Task.Revision, m.Task.TaskID); err != nil {
			return domain.WithExplanation(ErrStorageUnavailable, "The relocation resolved revision could not be updated in relocation_operations.")
		}
	}
	return nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, e TaskEvent) error {
	var duplicate int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_events WHERE task_id=? AND request_id=?`, e.TaskID, e.RequestID).Scan(&duplicate); err != nil {
		return storageFailure(err, "The event request identity could not be checked for an existing Task event.")
	}
	if duplicate != 0 {
		return domain.WithExplanation(ErrRevisionConflict, "A Task event already exists for this request identity.")
	}
	var transition, reason, action, observedBinding any
	if e.TransitionID != nil {
		transition = string(*e.TransitionID)
	}
	if e.TransitionReason != "" {
		reason = e.TransitionReason
	}
	if e.ActionID != nil {
		action = string(*e.ActionID)
	}
	if e.ObservedBindingDigest != nil {
		observedBinding = string(*e.ObservedBindingDigest)
	}
	paths := e.RepositoryDeltaPaths
	if paths == nil {
		paths = []string{}
	}
	encodedPaths, err := json.Marshal(paths)
	if err != nil {
		return domain.WithExplanation(ErrInvalidArgument, "The changed repository paths could not be encoded for the Task event.")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO task_events(event_id,task_id,revision,event_type,source_node,destination_node,transition_id,transition_reason,action_id,observed_binding_digest,repository_delta_paths,request_id,payload_digest,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, e.EventID, e.TaskID, e.Revision, e.Kind, e.SourceNode, e.DestinationNode, transition, reason, action, observedBinding, encodedPaths, e.RequestID, e.PayloadDigest, formatTime(e.CreatedAt))
	if err != nil {
		return storageFailure(err, "The Task event could not be inserted into task_events.")
	}
	return nil
}
func applyClaim(ctx context.Context, tx *sql.Tx, m TaskMutation) error {
	identities := repositoryClaimIdentities(m.Task)
	switch m.Claim {
	case ClaimAcquire:
		for _, claim := range repositoryClaims(m.Task) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO repository_claims(worktree_instance_digest,canonical_worktree_root,task_id,origin_host,claimed_at) VALUES(?,?,?,?,?)`, claim.identity, claim.root, m.Task.TaskID, m.Task.OriginHost, formatTime(m.Event.CreatedAt)); err != nil {
				return domain.WithExplanation(ErrActiveTaskConflict, "The workspace claim could not be acquired in repository_claims; an existing claim or a storage write failure prevented insertion.")
			}
		}
	case ClaimRetain:
		if err := validateClaimSet(ctx, tx, m.Task, identities); err != nil {
			return storageFailure(err, "The saved repository claims do not match the Task scope being retained.")
		}
	case ClaimRelease:
		if err := validateClaimSet(ctx, tx, m.Task, identities); err != nil {
			return storageFailure(err, "The saved repository claims do not match the Task scope being released.")
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM repository_claims WHERE task_id=?`, m.Task.TaskID)
		if err != nil {
			return storageFailure(err, "The Task repository claims could not be deleted.")
		}
		n, _ := result.RowsAffected()
		if n != int64(len(identities)) {
			return domain.WithExplanation(ErrStorageUnavailable, "The number of deleted repository claims differs from the Task repository count.")
		}
	case ClaimReplace:
		if len(m.PreviousClaims) == 0 || validateClaimSet(ctx, tx, m.Task, m.PreviousClaims) != nil {
			return domain.WithExplanation(ErrStorageUnavailable, "Replacing repository claims requires the complete original claim set to match storage.")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM repository_claims WHERE task_id=?`, m.Task.TaskID); err != nil {
			return domain.WithExplanation(ErrStorageUnavailable, "The original repository claims could not be removed during relocation.")
		}
		for _, claim := range repositoryClaims(m.Task) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO repository_claims(worktree_instance_digest,canonical_worktree_root,task_id,origin_host,claimed_at) VALUES(?,?,?,?,?)`, claim.identity, claim.root, m.Task.TaskID, m.Task.OriginHost, formatTime(m.Event.CreatedAt)); err != nil {
				return domain.WithExplanation(ErrActiveTaskConflict, "The destination workspace claim could not be acquired; an existing claim or a storage write failure prevented insertion.")
			}
		}
	default:
		return domain.WithExplanation(ErrInvalidArgument, "The mutation specifies an unsupported repository-claim operation.")
	}
	return nil
}

func validateClaimSet(ctx context.Context, tx *sql.Tx, task domain.ProcessTask, expected []domain.Digest) error {
	rows, err := tx.QueryContext(ctx, `SELECT worktree_instance_digest,origin_host FROM repository_claims WHERE task_id=? ORDER BY worktree_instance_digest`, task.TaskID)
	if err != nil {
		return storageFailure(err, "The Task repository claims could not be read.")
	}
	defer rows.Close()
	actual := make([]string, 0, len(expected))
	for rows.Next() {
		var identity, host string
		if rows.Scan(&identity, &host) != nil || host != string(task.OriginHost) {
			return domain.WithExplanation(ErrStorageUnavailable, "A repository claim could not be decoded or its host differs from the Task owner.")
		}
		actual = append(actual, identity)
	}
	if rows.Err() != nil || rows.Close() != nil || len(actual) != len(expected) {
		return domain.WithExplanation(ErrStorageUnavailable, "The repository claim rows could not be read completely or their count differs from the expected scope.")
	}
	want := make([]string, len(expected))
	for i, identity := range expected {
		want[i] = string(identity)
	}
	sort.Strings(want)
	for i := range want {
		if want[i] != actual[i] {
			return domain.WithExplanation(ErrStorageUnavailable, "A saved workspace claim identity does not match the expected Task repository scope.")
		}
	}
	return nil
}
func validateMutation(m TaskMutation) error {
	if workflow.ValidateProcessTask(m.Task) != nil || m.Task.Revision != m.ExpectedRevision+1 || m.Event.TaskID != m.Task.TaskID || m.Event.Revision != m.Task.Revision || m.Event.DestinationNode != m.Task.CurrentNode || !m.Event.SourceNode.IsValid() || !m.Event.DestinationNode.IsValid() || m.Event.EventID == "" || m.Event.RequestID == "" || !m.Event.PayloadDigest.IsValid() {
		return domain.WithExplanation(ErrInvalidArgument, "The proposed Task and event must agree on identity, revision, node and operation digest.")
	}
	op := m.Task.LastOperation
	if op == nil || op.Validate() != nil || op.OperationID != m.Event.RequestID || op.Kind != m.Event.Kind || op.FromRevision != m.ExpectedRevision || op.ToRevision != m.Task.Revision || op.PayloadDigest != m.Event.PayloadDigest || !op.CommittedAt.Equal(m.Event.CreatedAt) || !sameOptionalID(op.ActionID, m.Event.ActionID) {
		return domain.WithExplanation(ErrInvalidArgument, "The Task last_operation does not match the event identity, revision, digest, time or Action.")
	}
	if m.Event.TransitionID != nil {
		definition := workflow.StandardProcess()
		transition, err := workflow.TransitionFor(definition, m.Event.SourceNode, *m.Event.TransitionID)
		if err != nil || transition.Destination != m.Event.DestinationNode || (transition.ReasonRequired != (m.Event.TransitionReason != "")) {
			return domain.WithExplanation(ErrInvalidArgument, "The event transition, destination or required reason does not match the process definition.")
		}
	}
	if m.Event.ObservedBindingDigest != nil && !m.Event.ObservedBindingDigest.IsValid() {
		return domain.WithExplanation(ErrInvalidArgument, "The event observed binding digest is not a valid SHA-256 digest.")
	}
	if len(m.Event.RepositoryDeltaPaths) > domain.MaxRepositoryDeltaPaths {
		return domain.WithExplanation(ErrInvalidArgument, "The event exceeds the maximum number of repository delta paths.")
	}
	for index, path := range m.Event.RepositoryDeltaPaths {
		if m.Task.ValidateRepositoryPath(path) != nil || index > 0 && m.Event.RepositoryDeltaPaths[index-1] >= path {
			return domain.WithExplanation(ErrInvalidArgument, "Event repository paths must be in the Task scope, sorted and unique.")
		}
	}
	if m.Claim == ClaimReplace {
		if len(m.PreviousClaims) != len(repositoryClaimIdentities(m.Task)) {
			return domain.WithExplanation(ErrInvalidArgument, "A claim replacement must include one previous claim for each Task repository.")
		}
		seen := map[domain.Digest]bool{}
		for _, identity := range m.PreviousClaims {
			if !identity.IsValid() || seen[identity] {
				return domain.WithExplanation(ErrInvalidArgument, "Previous workspace claim identities must be valid and unique.")
			}
			seen[identity] = true
		}
	} else if len(m.PreviousClaims) != 0 {
		return domain.WithExplanation(ErrInvalidArgument, "Previous workspace claims are only accepted for claim replacement.")
	}
	return nil
}
func sameOptionalID(a, b *domain.ID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func formatTime(v time.Time) string { return v.UTC().Format(time.RFC3339Nano) }

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
