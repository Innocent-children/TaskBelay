package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	FastWait   = 20 * time.Millisecond
	NormalWait = 200 * time.Millisecond
)

type Store struct {
	db         *sql.DB
	waitLimit  time.Duration
	dataDir    string
	indexPages indexPageCache
	checkPages checkPageCache
}

func (s *Store) DataDir() string { return s.dataDir }

func Open(ctx context.Context, dataDir string, waitLimit time.Duration) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if waitLimit <= 0 || waitLimit > NormalWait {
		return nil, fmt.Errorf("数据库等待时限须大于 0，且不超过 %s", NormalWait)
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
	// Windows 盘符路径也须以 / 开头，避免盘符被解析为 URI 主机名。
	if !strings.HasPrefix(uri.Path, "/") {
		uri.Path = "/" + uri.Path
	}
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
 * connection 让连接池等待和 SQLite 的忙等待共用一个时限。
 * 每次取得连接都重设 busy_timeout，不能沿用上一次事件较长的剩余时间。
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

/**
 * beginWrite 避免递增忙等待在短预算内错过并发写锁的释放。
 * 只重试尚未开始的事务；连接等待、重试和业务写入共用调用方的截止时间。
 */
func beginWrite(ctx context.Context, conn *sql.Conn) (*sql.Tx, error) {
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 0"); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tx, err := conn.BeginTx(ctx, nil)
		var sqliteErr *sqlite.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&0xff != sqlite3.SQLITE_BUSY {
			return tx, err
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
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
		// 不调用 SQLite 的忙等待；重试仍受本次初始化的原有时限约束。
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
	tx, err := beginWrite(ctx, conn)
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
	if version < 11 {
		if err := migrateV11(ctx, tx); err != nil {
			return err
		}
	}
	if version < 12 {
		if err := migrateV12(ctx, tx); err != nil {
			return err
		}
	}
	if version < 13 {
		if _, err := tx.ExecContext(ctx, indexPageSchema); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, indexPruneSchema); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RegisterWorkspace(ctx context.Context, id, projectID, path string) error {
	if id == "" || projectID == "" || path == "" {
		return fmt.Errorf("工作区标识、项目标识和路径不能为空")
	}
	return s.write(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO workspaces_v12 (id, project_id, path) VALUES (?, ?, ?)
ON CONFLICT(id) DO UPDATE SET project_id=excluded.project_id, path=excluded.path
WHERE workspaces_v12.project_id != excluded.project_id OR workspaces_v12.path != excluded.path`, id, projectID, path)
		return err
	})
}
