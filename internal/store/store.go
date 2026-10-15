// Package store owns current-only SQLite persistence, CAS and operation identity.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	_ "modernc.org/sqlite"
)

const DatabaseSchemaVersion = "2.5.0"
const MaxSnapshotBytes = 8 * 1024 * 1024

var schema = []string{
	`CREATE TABLE schema_metadata(version TEXT PRIMARY KEY)`,
	`CREATE TABLE tasks(task_id TEXT PRIMARY KEY, revision INTEGER NOT NULL, status TEXT NOT NULL, snapshot BLOB NOT NULL, runs INTEGER NOT NULL DEFAULT 0, execution_ms INTEGER NOT NULL DEFAULT 0, reserved_ms INTEGER NOT NULL DEFAULT 0, uncertain_ms INTEGER NOT NULL DEFAULT 0, core_calls INTEGER NOT NULL DEFAULT 0, core_ms INTEGER NOT NULL DEFAULT 0, observation_ms INTEGER NOT NULL DEFAULT 0, host_ms INTEGER NOT NULL DEFAULT 0, wait_ms INTEGER NOT NULL DEFAULT 0, tokens INTEGER NOT NULL DEFAULT 0)`,
	`CREATE TABLE claims(root TEXT PRIMARY KEY, instance TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL REFERENCES tasks(task_id))`,
	`CREATE TABLE operations(task_id TEXT NOT NULL REFERENCES tasks(task_id), operation_id TEXT NOT NULL, kind TEXT NOT NULL, digest TEXT NOT NULL, revision INTEGER NOT NULL, run_id TEXT NOT NULL, PRIMARY KEY(task_id,operation_id))`,
	`CREATE TABLE runs(run_id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(task_id), check_id TEXT NOT NULL, status TEXT NOT NULL, snapshot BLOB NOT NULL, created_at TEXT NOT NULL, input_key TEXT NOT NULL)`,
	`CREATE INDEX runs_task ON runs(task_id,created_at)`,
	`CREATE INDEX runs_context ON runs(task_id,input_key,created_at)`,
	`CREATE TABLE failures(task_id TEXT NOT NULL, signature TEXT NOT NULL, count INTEGER NOT NULL, PRIMARY KEY(task_id,signature))`,
	`CREATE TABLE control_calls(call_id TEXT PRIMARY KEY,started_at TEXT NOT NULL,completed_ms INTEGER,owner_pid INTEGER NOT NULL,owner_identity TEXT NOT NULL)`,
	`CREATE TABLE control_totals(calls INTEGER NOT NULL,wall_ms INTEGER NOT NULL,unknown_ms INTEGER NOT NULL)`,
	`CREATE TABLE host_usage_sources(task_id TEXT NOT NULL REFERENCES tasks(task_id), source_id TEXT NOT NULL, observed_through TEXT NOT NULL, tokens INTEGER, active_ms INTEGER, wait_ms INTEGER, coverage TEXT NOT NULL, PRIMARY KEY(task_id,source_id))`,
}

type Store struct {
	db        *sql.DB
	Directory string
}

func Open(ctx context.Context, file string) (*Store, error) {
	if !filepath.IsAbs(file) {
		return nil, domain.Invalid("data_directory", "must be absolute")
	}
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return nil, e
	}
	// Inspect an existing file read-only before allowing SQLite to write anything.
	info, e := os.Lstat(file)
	if e == nil {
		if !info.Mode().IsRegular() {
			return nil, domain.Fail("STORAGE_UNAVAILABLE", "", "database must be a regular file")
		}
		db, e := sql.Open("sqlite", file+"?mode=ro")
		if e != nil {
			return nil, e
		}
		e = verify(ctx, db)
		db.Close()
		if e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	db, e := sql.Open("sqlite", file)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, Directory: filepath.Dir(file)}
	for _, p := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL"} {
		if _, e = db.ExecContext(ctx, p); e != nil {
			db.Close()
			return nil, e
		}
	}
	if info == nil {
		tx, e := db.BeginTx(ctx, nil)
		if e != nil {
			db.Close()
			return nil, e
		}
		for _, q := range schema {
			if _, e = tx.ExecContext(ctx, q); e != nil {
				tx.Rollback()
				db.Close()
				return nil, e
			}
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_metadata VALUES (?)", DatabaseSchemaVersion); e == nil {
			_, e = tx.ExecContext(ctx, "INSERT INTO control_totals VALUES (0,0,0)")
		}
		if e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
		if e != nil {
			db.Close()
			return nil, e
		}
	}
	return s, nil
}

type query interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verify(ctx context.Context, q query) error {
	var v string
	if e := q.QueryRowContext(ctx, "SELECT version FROM schema_metadata").Scan(&v); e != nil || v != DatabaseSchemaVersion {
		return domain.Fail("SCHEMA_UNSUPPORTED", "", "database is not the current engineering-control schema; it was not modified")
	}
	rows, e := q.QueryContext(ctx, "SELECT sql FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name")
	if e != nil {
		return e
	}
	defer rows.Close()
	actual := map[string]bool{}
	for rows.Next() {
		var x string
		if e = rows.Scan(&x); e != nil {
			return e
		}
		actual[strings.Join(strings.Fields(x), " ")] = true
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if len(actual) != len(schema) {
		return domain.Fail("SCHEMA_UNSUPPORTED", "", "unexpected schema objects; database was not modified")
	}
	for _, x := range schema {
		if !actual[strings.Join(strings.Fields(x), " ")] {
			return domain.Fail("SCHEMA_UNSUPPORTED", "", "schema definition differs from current contract; database was not modified")
		}
	}
	return nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Get(ctx context.Context, id string) (domain.Task, error) {
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return domain.Task{}, e
	}
	defer tx.Rollback()
	var b []byte
	var revision uint64
	var status string
	var u domain.Usage
	e = tx.QueryRowContext(ctx, `SELECT snapshot,revision,status,runs,execution_ms,reserved_ms,uncertain_ms,core_calls,core_ms,observation_ms,host_ms,wait_ms,tokens FROM tasks WHERE task_id=?`, id).Scan(&b, &revision, &status, &u.Runs, &u.ExecutionMS, &u.ReservedMS, &u.UncertainMS, &u.CoreCalls, &u.CoreMS, &u.ObservationMS, &u.HostMS, &u.WaitMS, &u.Tokens)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Task{}, domain.Fail("TASK_NOT_FOUND", "task_id", "task does not exist")
	}
	if e != nil {
		return domain.Task{}, e
	}
	var t domain.Task
	if len(b) > MaxSnapshotBytes || decodeSnapshot(b, &t) != nil || t.Validate() != nil || t.ValidateRecords() != nil || t.TaskID != id || t.Revision != revision || t.Status != status {
		return domain.Task{}, domain.Fail("STORAGE_CORRUPT", "", "saved task does not satisfy the current contract")
	}
	u.Host, e = hostUsage(ctx, tx, id)
	if e != nil {
		return domain.Task{}, e
	}
	t.Usage = u
	return t, tx.Commit()
}

type Operation struct {
	ID       string
	Kind     string
	Digest   domain.Digest
	RunID    string
	Revision uint64
}

func (s *Store) Operation(ctx context.Context, task, id string) (Operation, bool, error) {
	var o Operation
	o.ID = id
	e := s.db.QueryRowContext(ctx, "SELECT kind,digest,run_id,revision FROM operations WHERE task_id=? AND operation_id=?", task, id).Scan(&o.Kind, &o.Digest, &o.RunID, &o.Revision)
	if errors.Is(e, sql.ErrNoRows) {
		return o, false, nil
	}
	return o, e == nil, e
}
func encode(t domain.Task) ([]byte, error) {
	if e := t.Validate(); e != nil {
		return nil, e
	}
	if e := t.ValidateRecords(); e != nil {
		return nil, e
	}
	b, e := json.Marshal(t)
	if e != nil {
		return nil, e
	}
	if len(b) > MaxSnapshotBytes {
		return nil, domain.Fail("RESOURCE_LIMIT", "task", "current task snapshot exceeds 8 MiB")
	}
	return b, nil
}
func (s *Store) Create(ctx context.Context, t domain.Task, o Operation) error {
	b, e := encode(t)
	if e != nil {
		return e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, "INSERT INTO tasks(task_id,revision,status,snapshot) VALUES (?,?,?,?)", t.TaskID, t.Revision, t.Status, b); e != nil {
		return domain.Fail("CONFLICT", "task_id", "task or operation identity already exists")
	}
	for _, r := range t.Repositories {
		if _, e = tx.ExecContext(ctx, "INSERT INTO claims VALUES (?,?,?)", r.Origin.CanonicalWorktreeRoot, r.Binding.WorktreeInstanceDigest, t.TaskID); e != nil {
			return domain.Fail("WORKSPACE_CLAIMED", "repository_path", "workspace already belongs to an active task")
		}
	}
	if e = insertOperation(ctx, tx, t, o); e != nil {
		return e
	}
	return tx.Commit()
}
func insertOperation(ctx context.Context, tx *sql.Tx, t domain.Task, o Operation) error {
	if o.ID == "" {
		return nil
	}
	_, e := tx.ExecContext(ctx, "INSERT INTO operations VALUES (?,?,?,?,?,?)", t.TaskID, o.ID, o.Kind, o.Digest, t.Revision, o.RunID)
	return e
}

type Charge struct {
	InputKey         domain.Digest
	FailureSignature domain.Digest
	Runs             int64
	ReserveMS        int64
	ExecutionMS      int64
	ReleaseMS        int64
	UncertainMS      int64
	HostUsage        *domain.HostUsageReport
}

func (s *Store) Save(ctx context.Context, t domain.Task, expected uint64, o Operation, c Charge, runID string, run any, runStatus, checkID string) error {
	b, e := encode(t)
	if e != nil {
		return e
	}
	if t.Revision != expected+1 {
		return domain.Invalid("revision", "mutation must advance one revision")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var used, reserved, spent, uncertain int64
	if e = tx.QueryRowContext(ctx, "SELECT runs,reserved_ms,execution_ms,uncertain_ms FROM tasks WHERE task_id=?", t.TaskID).Scan(&used, &reserved, &spent, &uncertain); e != nil {
		return e
	}
	if c.Runs > 0 && t.Authorization.Limits.Runs > 0 && used+c.Runs > t.Authorization.Limits.Runs {
		return &domain.Error{Code: "COST_LIMIT", Message: "cumulative run limit cannot reserve this check; no process was started", Details: []domain.ContractViolation{{Path: "authorization.limits.runs", Rule: "quantity", Message: "lifetime runs cannot exceed the authorized limit"}}, Budget: domain.NewBudgetLine("runs", used, 0, 0, t.Authorization.Limits.Runs).Failure(c.Runs)}
	}
	if c.ReserveMS > 0 && t.Authorization.Limits.ExecutionMS > 0 && spent+uncertain+reserved+c.ReserveMS > t.Authorization.Limits.ExecutionMS {
		return &domain.Error{Code: "COST_LIMIT", Message: "cumulative execution time cannot reserve this check; no process was started", Details: []domain.ContractViolation{{Path: "authorization.limits.execution_ms", Rule: "quantity", Message: "reserve the command timeout without lowering acceptance quality"}}, Budget: domain.NewBudgetLine("execution_ms", spent, reserved, uncertain, t.Authorization.Limits.ExecutionMS).Failure(c.ReserveMS)}
	}
	delta, e := saveHostUsage(ctx, tx, t.TaskID, c.HostUsage)
	if e != nil {
		return e
	}
	r, e := tx.ExecContext(ctx, `UPDATE tasks SET revision=?,status=?,snapshot=?,runs=runs+?,reserved_ms=reserved_ms+?-?,execution_ms=execution_ms+?,uncertain_ms=uncertain_ms+?,host_ms=host_ms+?,wait_ms=wait_ms+?,tokens=tokens+? WHERE task_id=? AND revision=?`, t.Revision, t.Status, b, c.Runs, c.ReserveMS, c.ReleaseMS, c.ExecutionMS, c.UncertainMS, delta[1], delta[2], delta[0], t.TaskID, expected)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return domain.Fail("REVISION_CONFLICT", "revision", "task changed; read current state before submitting a new operation")
	}
	if e = insertOperation(ctx, tx, t, o); e != nil {
		return e
	}
	if runID != "" {
		rb, e := json.Marshal(run)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO runs VALUES (?,?,?,?,?,?,?) ON CONFLICT(run_id) DO UPDATE SET status=excluded.status,snapshot=excluded.snapshot`, runID, t.TaskID, checkID, runStatus, rb, time.Now().UTC().Format(time.RFC3339Nano), c.InputKey); e != nil {
			return e
		}
	}
	if c.FailureSignature != "" {
		if _, e = tx.ExecContext(ctx, `INSERT INTO failures VALUES (?,?,1) ON CONFLICT(task_id,signature) DO UPDATE SET count=count+1`, t.TaskID, c.FailureSignature); e != nil {
			return e
		}
	}
	if t.Status != "active" {
		if _, e = tx.ExecContext(ctx, "DELETE FROM claims WHERE task_id=?", t.TaskID); e != nil {
			return e
		}
	}
	// Retain live operations and enough recent identity to reconcile response loss.
	if _, e = tx.ExecContext(ctx, `DELETE FROM operations WHERE task_id=? AND kind<>'open' AND revision < ? AND (run_id='' OR run_id NOT IN (SELECT run_id FROM runs WHERE status IN ('running','uncertain')))`, t.TaskID, int64(t.Revision)-256); e != nil {
		return e
	}
	keep := map[string]bool{}
	for _, v := range t.Evidence {
		keep[v.ID] = true
	}
	for _, v := range t.Attempts {
		keep[v.RunID] = true
	}
	rows, e := tx.QueryContext(ctx, "SELECT run_id,status FROM runs WHERE task_id=? ORDER BY created_at DESC", t.TaskID)
	if e != nil {
		return e
	}
	remove := []string{}
	i := 0
	for rows.Next() {
		var id, status string
		rows.Scan(&id, &status)
		if i >= 128 && !keep[id] && status != "running" && status != "uncertain" {
			remove = append(remove, id)
		}
		i++
	}
	rows.Close()
	for _, id := range remove {
		if _, e = tx.ExecContext(ctx, "DELETE FROM runs WHERE run_id=?", id); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (s *Store) AddCost(ctx context.Context, id string, core, observe int64) error {
	_, e := s.db.ExecContext(ctx, "UPDATE tasks SET core_ms=core_ms+?,observation_ms=observation_ms+? WHERE task_id=?", core, observe, id)
	return e
}
func (s *Store) StartCall(ctx context.Context, id string) error {
	_, e := s.db.ExecContext(ctx, "UPDATE tasks SET core_calls=core_calls+1 WHERE task_id=?", id)
	return e
}
func (s *Store) AddObservation(ctx context.Context, id string, duration int64) error {
	_, e := s.db.ExecContext(ctx, "UPDATE tasks SET observation_ms=observation_ms+? WHERE task_id=?", duration, id)
	return e
}
func (s *Store) Claim(ctx context.Context, root string) (string, error) {
	var id string
	e := s.db.QueryRowContext(ctx, "SELECT task_id FROM claims WHERE root=?", root).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return "", nil
	}
	return id, e
}

type ListQuery struct {
	Search          string      `json:"search,omitempty" description:"Optional request/task ID substring, at most 512 UTF-8 bytes."`
	Host            domain.Host `json:"host,omitempty" description:"Task处为Host枚举；usage.host为Host用量对象；安装registration.host说明实际核对程度。"`
	Page            int         `json:"page,omitempty" description:"当前页码，从1开始。"`
	PageSize        int         `json:"page_size,omitempty" description:"Default 20; integer 1..100. Filters precede pagination."`
	IncludeArchived bool        `json:"include_archived,omitempty" description:"Default false; true includes terminal archived tasks without restoring them."`
	Status          string      `json:"status,omitempty" description:"对象状态；Task、Run、工作区、安装结果各有独立枚举，见公共状态表。"`
}
type TaskPage struct {
	Items   []domain.Task
	Page    int
	HasNext bool
}

func (s *Store) List(ctx context.Context, q ListQuery) (TaskPage, error) {
	q.Search = strings.TrimSpace(q.Search)
	if len(q.Search) > 512 || q.Host != "" && !q.Host.IsValid() {
		return TaskPage{}, domain.Invalid("filter", "search must be at most 512 bytes and host must be supported")
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 1000000 || q.PageSize < 1 || q.PageSize > 100 {
		return TaskPage{}, domain.Invalid("page", "page must be positive and page_size must be 1..100")
	}
	if q.Status != "" && q.Status != "active" && q.Status != "done" && q.Status != "cancelled" {
		return TaskPage{}, domain.Invalid("status", "requires active, done or cancelled")
	}
	rows, e := s.db.QueryContext(ctx, `SELECT task_id FROM tasks
	 WHERE (? OR json_extract(snapshot,'$.archived')=0) AND (?='' OR status=?)
	 AND (?='' OR json_extract(snapshot,'$.host')=?)
	 AND (?='' OR instr(lower(task_id || ' ' || json_extract(snapshot,'$.authorization.request')),lower(?))>0)
	 ORDER BY rowid DESC LIMIT ? OFFSET ?`, q.IncludeArchived, q.Status, q.Status, q.Host, q.Host, q.Search, q.Search, q.PageSize+1, (q.Page-1)*q.PageSize)
	if e != nil {
		return TaskPage{}, e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return TaskPage{}, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return TaskPage{}, e
	}
	page := TaskPage{Items: []domain.Task{}, Page: q.Page, HasNext: len(ids) > q.PageSize}
	if page.HasNext {
		ids = ids[:q.PageSize]
	}
	for _, id := range ids {
		t, e := s.Get(ctx, id)
		if e != nil {
			return TaskPage{}, e
		}
		page.Items = append(page.Items, t)
	}
	return page, nil
}
func (s *Store) StatusCounts(ctx context.Context) (map[string]int, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT status,COUNT(*) FROM tasks WHERE json_extract(snapshot,'$.archived')=0 GROUP BY status`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if e = rows.Scan(&status, &n); e != nil {
			return nil, e
		}
		counts[status] = n
	}
	return counts, rows.Err()
}
func (s *Store) Run(ctx context.Context, id string, out any) error {
	var b []byte
	e := s.db.QueryRowContext(ctx, "SELECT snapshot FROM runs WHERE run_id=?", id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return domain.Fail("RUN_NOT_FOUND", "run_id", "run is absent or compacted; it must not be replayed")
	}
	if e != nil {
		return e
	}
	if len(b) > MaxSnapshotBytes || decodeSnapshot(b, out) != nil {
		return domain.Fail("STORAGE_CORRUPT", "", "invalid saved execution")
	}
	return nil
}

/**
 * Record the stopped-at-launch process before allowing the approved command to execute.
 * The reservation and task revision stay unchanged; only this run's observed identity advances.
 */
func (s *Store) RecordStartedRun(ctx context.Context, id string, reserved, started any) error {
	before, e := json.Marshal(reserved)
	if e != nil {
		return e
	}
	after, e := json.Marshal(started)
	if e != nil {
		return e
	}
	result, e := s.db.ExecContext(ctx, "UPDATE runs SET snapshot=? WHERE run_id=? AND status='running' AND snapshot=?", after, id, before)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return domain.Fail("EXECUTION_UNCERTAIN", "run_id", "execution reservation changed before process activation")
	}
	return nil
}
func decodeSnapshot(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var trailing any
	if e := d.Decode(&trailing); e != io.EOF {
		return fmt.Errorf("snapshot has trailing data")
	}
	return nil
}
func (s *Store) Pending(ctx context.Context, task string) ([]string, error) {
	rows, e := s.db.QueryContext(ctx, "SELECT run_id FROM runs WHERE task_id=? AND status IN ('running','uncertain')", task)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
func (s *Store) Failure(ctx context.Context, task string, signature domain.Digest) (int, error) {
	var count int
	e := s.db.QueryRowContext(ctx, "SELECT count FROM failures WHERE task_id=? AND signature=?", task, signature).Scan(&count)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, nil
	}
	return count, e
}
func (s *Store) LatestRun(ctx context.Context, task string, key domain.Digest, out any) (bool, error) {
	var id string
	e := s.db.QueryRowContext(ctx, "SELECT run_id FROM runs WHERE task_id=? AND input_key=? ORDER BY created_at DESC LIMIT 1", task, key).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return true, s.Run(ctx, id, out)
}
func (s *Store) HasRun(ctx context.Context, id string) (bool, error) {
	var n int
	e := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runs WHERE run_id=?", id).Scan(&n)
	return n == 1, e
}
func (s *Store) Purge(ctx context.Context, id string, revision uint64) error {
	t, e := s.Get(ctx, id)
	if e != nil {
		return e
	}
	if t.Status == "active" || t.Revision != revision {
		return domain.Fail("PURGE_NOT_ALLOWED", "task_id", "only the exact terminal task can be explicitly purged")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, table := range []string{"operations", "runs", "failures", "claims", "host_usage_sources"} {
		if _, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE task_id=?", table), id); e != nil {
			return e
		}
	}
	r, e := tx.ExecContext(ctx, "DELETE FROM tasks WHERE task_id=? AND revision=? AND status<>'active'", id, revision)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return domain.Fail("REVISION_CONFLICT", "revision", "task changed")
	}
	return tx.Commit()
}
