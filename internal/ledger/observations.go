package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Innocent-children/taskbelay/internal/event"
	"github.com/Innocent-children/taskbelay/internal/gitstate"
)

const observationSchema = `
ALTER TABLE workspaces ADD COLUMN latest_observation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workspaces ADD COLUMN record_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workspaces ADD COLUMN branch TEXT NOT NULL DEFAULT '';
ALTER TABLE workspaces ADD COLUMN head TEXT NOT NULL DEFAULT '';
CREATE TABLE observations (
 id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 task_id TEXT NOT NULL, data TEXT NOT NULL
);
CREATE INDEX observations_workspace ON observations(workspace_id, id);
CREATE TABLE observed_files (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), path TEXT NOT NULL,
 blob TEXT NOT NULL, mode TEXT NOT NULL, observation_id INTEGER NOT NULL REFERENCES observations(id),
 workspace_update INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id, path)
);
CREATE TABLE file_changes (
 id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 observation_id INTEGER NOT NULL REFERENCES observations(id), data TEXT NOT NULL
);
CREATE INDEX changes_workspace ON file_changes(workspace_id, id);
CREATE TABLE edits (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), agent TEXT NOT NULL,
 session_id TEXT NOT NULL, call_id TEXT NOT NULL, before_observation INTEGER NOT NULL REFERENCES observations(id),
 after_observation INTEGER NOT NULL DEFAULT 0, data TEXT NOT NULL,
 PRIMARY KEY(workspace_id, agent, session_id, call_id)
);
CREATE TABLE workspace_links (
 parent_id TEXT NOT NULL REFERENCES workspaces(id), task_id TEXT NOT NULL,
 child_id TEXT NOT NULL REFERENCES workspaces(id), path TEXT NOT NULL,
 PRIMARY KEY(parent_id, task_id, child_id)
);
PRAGMA user_version=2;
`

var ErrObservationConflict = errors.New("观察期间账本已有新记录，请按当前状态重新核对")
var ErrEditExists = errors.New("该工具调用已记录")

type ObservedFile struct {
	gitstate.File
	ObservationID   int64
	WorkspaceUpdate bool
}

type ObservationView struct {
	LatestID int64
	Revision int64
	Context  gitstate.Context
	Files    map[string]ObservedFile
}

type Observation struct {
	ID          int64
	WorkspaceID string
	TaskID      string
	Agent       string
	SessionID   string
	RoundID     string
	Trigger     string
	At          time.Time
	Context     gitstate.Context
	Tree        string
	Full        bool
	Complete    bool
	Files       map[string]gitstate.File
	Boundaries  []gitstate.Boundary
	Incomplete  []gitstate.Incomplete
	SnapshotRef string
	EvidenceRef string
}

type FileChange struct {
	Path              string
	BeforeObservation int64
	AfterObservation  int64
	At                time.Time
	Kind              string
	ToolCallID        string
	Before            gitstate.File
	After             gitstate.File
	Fragments         []gitstate.Fragment
}

type PreparedFile struct {
	Path         string
	Before       gitstate.File
	Expected     gitstate.File
	Verifiable   bool
	Verification string
}

type EditRecord struct {
	PlanCheck         *PlanEditCheck
	WorkspaceID       string
	Agent             string
	SessionID         string
	ToolCallID        string
	BeforeObservation int64
	AfterObservation  int64
	Request           []event.FileEdit
	Files             []PreparedFile
	Decision          event.Decision
}

/**
 * 将 G01 的路径身份接到稳定的 git 目录身份，保留原有项目与任务设置。
 * 目录移动只更新位置；身份和位置都没变时不申请写锁。
 */
func (s *Store) RegisterGitWorkspace(ctx context.Context, w gitstate.Workspace) error {
	if w.ID == "" || w.ProjectID == "" || w.Root == "" || w.GitDir == "" || w.CommonDir == "" {
		return fmt.Errorf("工作区身份和 git 路径不能为空")
	}
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	var project, path string
	err = conn.QueryRowContext(ctx, "SELECT project_id, path FROM workspaces WHERE id=?", w.ID).Scan(&project, &path)
	if err == nil && project == w.ProjectID && path == w.Root {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces(id,project_id,path,current_task_id)
SELECT ?,?,?,current_task_id FROM workspaces WHERE id=? ON CONFLICT(id) DO NOTHING`, w.ID, w.ProjectID, w.Root, w.GitDir); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces(id,project_id,path) VALUES (?,?,?)
ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id,path=excluded.path`, w.ID, w.ProjectID, w.Root); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(scope,scope_id,name,value,revision)
SELECT scope,?,name,value,revision FROM settings WHERE scope='project' AND scope_id=?
ON CONFLICT(scope,scope_id,name) DO NOTHING`, w.ProjectID, w.CommonDir); err != nil {
		return err
	}
	// 保留旧路径记录以供核查，新的观察记录都使用稳定身份。
	return tx.Commit()
}

/**
 * paths 非 nil 时只查询指定目标。读事务同时取得修订和文件基线，
 * 避免把另一进程刚写入的修订与更早的文件内容混在一起。
 */
func (s *Store) ObservationState(ctx context.Context, workspace string, paths []string) (ObservationView, error) {
	view := ObservationView{Files: make(map[string]ObservedFile)}
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return view, err
	}
	defer release()
	// DSN 默认使用立即写事务；这里显式 BEGIN，仅建立读取快照。
	if _, err = conn.ExecContext(ctx, "BEGIN"); err != nil {
		return view, err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	err = conn.QueryRowContext(ctx, "SELECT latest_observation,record_revision,branch,head FROM workspaces WHERE id=?", workspace).Scan(&view.LatestID, &view.Revision, &view.Context.Branch, &view.Context.HEAD)
	if err != nil {
		return view, err
	}
	read := func(rows *sql.Rows) error {
		defer rows.Close()
		for rows.Next() {
			var path string
			var file ObservedFile
			if err := rows.Scan(&path, &file.Blob, &file.Mode, &file.ObservationID, &file.WorkspaceUpdate); err != nil {
				return err
			}
			view.Files[path] = file
		}
		return rows.Err()
	}
	query := "SELECT path,blob,mode,observation_id,workspace_update FROM observed_files WHERE workspace_id=?"
	if paths == nil {
		rows, err := conn.QueryContext(ctx, query, workspace)
		if err != nil {
			return view, err
		}
		if err := read(rows); err != nil {
			return view, err
		}
	} else {
		for _, path := range paths {
			rows, err := conn.QueryContext(ctx, query+" AND path=?", workspace, path)
			if err != nil {
				return view, err
			}
			if err := read(rows); err != nil {
				return view, err
			}
		}
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return view, err
}

/**
 * 文件采集完成后，以短事务一起发布观察、变化证据和编辑阶段。
 * 并发观察使旧基线失效时返回冲突，不覆盖另一进程的新记录。
 */
func (s *Store) SaveObservation(ctx context.Context, view ObservationView, observation *Observation, changes []FileChange, edit *EditRecord) error {
	if observation.WorkspaceID == "" {
		return fmt.Errorf("观察缺少工作区")
	}
	data, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var latest, revision int64
	var current gitstate.Context
	if err := tx.QueryRowContext(ctx, "SELECT latest_observation,record_revision,branch,head FROM workspaces WHERE id=?", observation.WorkspaceID).Scan(&latest, &revision, &current.Branch, &current.HEAD); err != nil {
		return err
	}
	// 只增加观察编号而没有改变文件或 Git 上下文时，原基线仍有效。
	if latest != view.LatestID && (view.LatestID == 0 || revision != view.Revision || current != view.Context) {
		return ErrObservationConflict
	}
	if edit != nil {
		var before, after int64
		err := tx.QueryRowContext(ctx, "SELECT before_observation,after_observation FROM edits WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", edit.WorkspaceID, edit.Agent, edit.SessionID, edit.ToolCallID).Scan(&before, &after)
		if edit.BeforeObservation == 0 && err == nil {
			return ErrEditExists
		}
		if edit.BeforeObservation != 0 && (err != nil || before != edit.BeforeObservation || after != 0) {
			return ErrEditExists
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO observations(workspace_id,task_id,data) VALUES (?,?,?)", observation.WorkspaceID, observation.TaskID, string(data))
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	updated := make(map[string]bool)
	for i := range changes {
		change := changes[i]
		if change.Kind != "agent_edit" && change.Kind != "workspace_update" {
			return fmt.Errorf("未知的变化来源")
		}
		change.AfterObservation, change.At = id, observation.At
		if change.Kind == "workspace_update" {
			updated[change.Path] = true
		}
		data, err := json.Marshal(change)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO file_changes(workspace_id,observation_id,data) VALUES (?,?,?)", observation.WorkspaceID, id, string(data)); err != nil {
			return err
		}
	}
	changed := false
	for path, file := range observation.Files {
		old := view.Files[path]
		if old.File != file {
			changed = true
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO observed_files(workspace_id,path,blob,mode,observation_id,workspace_update) VALUES (?,?,?,?,?,?)
ON CONFLICT(workspace_id,path) DO UPDATE SET blob=excluded.blob,mode=excluded.mode,observation_id=excluded.observation_id,workspace_update=MAX(observed_files.workspace_update,excluded.workspace_update)`, observation.WorkspaceID, path, file.Blob, file.Mode, id, updated[path])
		if err != nil {
			return err
		}
	}
	var publishedEdit *EditRecord
	if edit != nil {
		copy := *edit
		if edit.PlanCheck != nil {
			proof := *edit.PlanCheck
			proof.Issues = append([]Issue(nil), proof.Issues...)
			copy.PlanCheck = &proof
		}
		if copy.BeforeObservation == 0 {
			if err := publishPlanEdit(ctx, tx, *observation, &copy); err != nil {
				return err
			}
		}
		if copy.BeforeObservation == 0 {
			copy.BeforeObservation = id
		} else {
			copy.AfterObservation = id
		}
		data, err := json.Marshal(copy)
		if err != nil {
			return err
		}
		publishedEdit = &copy
		if edit.BeforeObservation == 0 {
			_, err = tx.ExecContext(ctx, "INSERT INTO edits VALUES (?,?,?,?,?,?,?)", copy.WorkspaceID, copy.Agent, copy.SessionID, copy.ToolCallID, id, 0, string(data))
		} else {
			_, err = tx.ExecContext(ctx, "UPDATE edits SET after_observation=?,data=? WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", id, string(data), copy.WorkspaceID, copy.Agent, copy.SessionID, copy.ToolCallID)
		}
		if err != nil {
			return err
		}
	}
	delta := 0
	if changed || len(changes) > 0 {
		delta = 1
	}
	_, err = tx.ExecContext(ctx, "UPDATE workspaces SET latest_observation=?,record_revision=record_revision+?,branch=?,head=? WHERE id=?", id, delta, observation.Context.Branch, observation.Context.HEAD, observation.WorkspaceID)
	if err != nil {
		return err
	}
	published := *observation
	published.ID = id
	if err := syncTaskObservation(ctx, tx, published, int64(delta)); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	observation.ID = id
	if edit != nil {
		*edit = *publishedEdit
	}
	return nil
}

func (s *Store) Edit(ctx context.Context, workspace, agent, session, call string) (EditRecord, error) {
	var record EditRecord
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return record, err
	}
	defer release()
	var data string
	if err := conn.QueryRowContext(ctx, "SELECT data FROM edits WHERE workspace_id=? AND agent=? AND session_id=? AND call_id=?", workspace, agent, session, call).Scan(&data); err != nil {
		return record, err
	}
	err = json.Unmarshal([]byte(data), &record)
	return record, err
}

func (s *Store) Observation(ctx context.Context, id int64) (Observation, error) {
	var observation Observation
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return observation, err
	}
	defer release()
	var data string
	if err := conn.QueryRowContext(ctx, "SELECT data FROM observations WHERE id=?", id).Scan(&data); err != nil {
		return observation, err
	}
	err = json.Unmarshal([]byte(data), &observation)
	observation.ID = id
	return observation, err
}

func (s *Store) Changes(ctx context.Context, workspace string, afterObservation int64) ([]FileChange, error) {
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := conn.QueryContext(ctx, "SELECT data FROM file_changes WHERE workspace_id=? AND observation_id>? ORDER BY id", workspace, afterObservation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var changes []FileChange
	for rows.Next() {
		var data string
		var change FileChange
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &change); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}

func (s *Store) LinkWorkspace(ctx context.Context, parent, task, child, path string) error {
	if parent == "" || task == "" || child == "" || parent == child || path == "" {
		return fmt.Errorf("工作区任务联系不完整")
	}
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return err
	}
	defer release()
	_, err = conn.ExecContext(ctx, `INSERT INTO workspace_links VALUES (?,?,?,?) ON CONFLICT(parent_id,task_id,child_id) DO UPDATE SET path=excluded.path`, parent, task, child, path)
	return err
}
