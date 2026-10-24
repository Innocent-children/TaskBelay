// Package ledger 读写任务账本。账本是一个所有工作区共用的 SQLite 文件，
// 两个 Agent 的 hook 进程和 MCP server 会同时读写它。
package ledger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Ledger 是打开的任务账本。
type Ledger struct {
	db *sql.DB
}

// Open 打开 path 处的账本，文件不存在时新建，并建好全部记录。
// 打开 WAL 模式，等锁最多 5 秒；事务一开始就拿写锁，避免多个进程同时写时互相卡住。
func Open(path string) (*Ledger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("建立账本 %s：%w", path, err)
	}
	return &Ledger{db: db}, nil
}

// Close 关闭账本。
func (l *Ledger) Close() error {
	return l.db.Close()
}
