package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

type errorEntry struct {
	Time        time.Time `json:"time"`
	Error       string    `json:"error"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
}

func logError(env platform.Env, cause error) error {
	return logWorkspaceError(env, "", cause)
}

func logWorkspaceError(env platform.Env, workspace string, cause error) error {
	dir := filepath.Join(env.DataDir, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	now := time.Now()
	message := cause.Error()
	if len(message) > 32*1024 {
		message = message[:32*1024] + "（日志过长，已截断）"
	}
	data, err := json.Marshal(errorEntry{Time: now, Error: message, WorkspaceID: workspace})
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, now.Format("2006-01-02")+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func logFiles(env platform.Env) (*os.Root, []string, error) {
	dataRoot, err := os.OpenRoot(env.DataDir)
	if err != nil {
		return nil, nil, err
	}
	defer dataRoot.Close()
	root, err := dataRoot.OpenRoot("logs")
	if err != nil {
		return nil, nil, err
	}
	dir, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	entries, err := dir.ReadDir(-1)
	dir.Close()
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if _, err := time.Parse("2006-01-02.log", name); err == nil {
			names = append(names, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return root, names, nil
}

func pruneLogs(ctx context.Context, env platform.Env, now time.Time) error {
	root, names, err := logFiles(env)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	cutoff := now.AddDate(0, 0, -6).Format("2006-01-02") + ".log"
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if name < cutoff {
			if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func recentErrors(env platform.Env, workspace string, now time.Time) ([]errorEntry, error) {
	root, names, err := logFiles(env)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var entries []errorEntry
	for _, name := range names {
		if name < now.AddDate(0, 0, -6).Format("2006-01-02")+".log" {
			continue
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return nil, err
		}
		if info.Size() > 128*1024 {
			if _, err := file.Seek(-128*1024, io.SeekEnd); err != nil {
				file.Close()
				return nil, err
			}
		}
		scan := bufio.NewScanner(file)
		scan.Buffer(make([]byte, 4096), 128*1024)
		for scan.Scan() {
			var entry errorEntry
			if json.Unmarshal(scan.Bytes(), &entry) == nil && (entry.WorkspaceID == "" || entry.WorkspaceID == workspace) {
				entry.Error = strings.TrimSpace(entry.Error)
				entries = append(entries, entry)
			}
		}
		err = scan.Err()
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(entries) >= 8 {
			break
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Time.After(entries[j].Time) })
	if len(entries) > 8 {
		entries = entries[:8]
	}
	return entries, nil
}
