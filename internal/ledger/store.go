package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	FastWait   = 20 * time.Millisecond
	NormalWait = 200 * time.Millisecond
)

type Store struct {
	db        *sql.DB
	waitLimit time.Duration
	dataDir   string
}

func (s *Store) DataDir() string { return s.dataDir }

func Open(ctx context.Context, dataDir string, waitLimit time.Duration) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if waitLimit <= 0 || waitLimit > NormalWait {
		return nil, fmt.Errorf("数据库等待预算须在 0 至 %s 之间", NormalWait)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("建立数据目录: %w", err)
	}
	path, err := filepath.Abs(filepath.Join(dataDir, "taskbelay.db"))
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开账本文件: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	uri := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	query := uri.Query()
	query.Set("_pragma", "foreign_keys(1)")
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, waitLimit: waitLimit, dataDir: dataDir}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化账本: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

/**
 * connection 把连接池等待和 SQLite 的忙等待放进同一个预算。
 * 每次借连接都重设 busy_timeout，不能沿用上一次事件较长的剩余时间。
 */
func (s *Store) connection(ctx context.Context) (*sql.Conn, context.Context, func(), error) {
	ctx, cancel := context.WithTimeout(ctx, s.waitLimit)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	release := func() {
		conn.Close()
		cancel()
	}
	deadline, _ := ctx.Deadline()
	millis := max(0, time.Until(deadline).Milliseconds())
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", millis)); err != nil {
		release()
		return nil, nil, nil, err
	}
	return conn, ctx, release, nil
}

func (s *Store) initialize(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.waitLimit)
	defer cancel()
	for {
		err := s.initializeOnce(ctx)
		var sqliteErr *sqlite.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&0xff != sqlite3.SQLITE_BUSY {
			return err
		}
		// 同时首次启动的进程可能在切换 WAL 时立即返回 SQLITE_BUSY，
		// 不调用 SQLite 的忙等待；重试仍共用本次初始化的原有预算。
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Store) initializeOnce(ctx context.Context) error {
	conn, ctx, release, err := s.connection(ctx)
	if err != nil {
		return fmt.Errorf("取得初始化连接: %w", err)
	}
	defer release()
	var mode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("读取日志模式: %w", err)
	}
	if mode != "wal" {
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
			return fmt.Errorf("启用 WAL: %w", err)
		}
		if mode != "wal" {
			return fmt.Errorf("账本未能启用 WAL: %s", mode)
		}
	}
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取账本版本: %w", err)
	}
	if version > schemaVersion {
		return fmt.Errorf("不支持账本版本 %d", version)
	}
	if version == schemaVersion {
		return nil
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始账本迁移: %w", err)
	}
	defer tx.Rollback()
	// 等待写锁期间，其他短进程可能已完成迁移。
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("不支持账本版本 %d", version)
	}
	if version == schemaVersion {
		return tx.Commit()
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    path TEXT NOT NULL,
    current_task_id TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS settings (
    scope TEXT NOT NULL CHECK (scope IN ('user', 'project', 'task')),
    scope_id TEXT NOT NULL,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    PRIMARY KEY (scope, scope_id, name),
    CHECK ((scope = 'user' AND scope_id = '') OR (scope != 'user' AND scope_id != ''))
);
PRAGMA user_version=1;
`)
		if err != nil {
			return err
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, observationSchema); err != nil {
			return err
		}
	}
	if version < 3 {
		if _, err := tx.ExecContext(ctx, taskSchema); err != nil {
			return err
		}
	}
	if version < 4 {
		if _, err := tx.ExecContext(ctx, hookSchema); err != nil {
			return err
		}
	}
	if version < 5 {
		if _, err := tx.ExecContext(ctx, mcpContextSchema); err != nil {
			return err
		}
	}
	if version < 6 {
		if _, err := tx.ExecContext(ctx, summarySchema); err != nil {
			return err
		}
	}
	if version < 7 {
		if _, err := tx.ExecContext(ctx, checkSchema); err != nil {
			return err
		}
	}
	if version < 8 {
		if _, err := tx.ExecContext(ctx, verificationSchema); err != nil {
			return err
		}
	}
	if version < 9 {
		if _, err := tx.ExecContext(ctx, planSchema); err != nil {
			return err
		}
	}
	if version < 10 {
		if _, err := tx.ExecContext(ctx, retentionSchema); err != nil {
			return err
		}
	}
	if err := migrateV11(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RegisterWorkspace(ctx context.Context, id, projectID, path string) error {
	if id == "" || projectID == "" || path == "" {
		return fmt.Errorf("工作区身份、项目身份和路径不能为空")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspaces_v11 (id, project_id, path) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id, path=excluded.path
WHERE workspaces_v11.project_id != excluded.project_id OR workspaces_v11.path != excluded.path`, id, projectID, path)
		return err
	})
}
