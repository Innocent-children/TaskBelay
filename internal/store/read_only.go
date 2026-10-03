package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
)

// OpenReadOnly reads an existing live database, including its WAL. Schema
// creation and all mutation statements are disabled by the SQLite connection.
func OpenReadOnly(ctx context.Context, path string) (*SQLite, error) {
	if path == "" {
		return nil, ErrInvalidArgument
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, ErrInvalidArgument
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrStorageUnavailable
	}
	db, err := sql.Open("sqlite", dataSource(absolute, true))
	if err != nil {
		return nil, ErrStorageUnavailable
	}
	db.SetMaxOpenConns(1)
	if err := verifyReadableSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &SQLite{db: db}, nil
}
