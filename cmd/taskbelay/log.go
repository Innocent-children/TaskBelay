package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// logKeepDays 是日志文件保留的天数，包括当天。
const logKeepDays = 7

// openLog 打开 dir 下当天的日志文件，按天一个文件，并删除超过保留天数的旧文件。
func openLog(dir string, now time.Time) (*slog.Logger, func() error, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	pruneLogs(dir, now)
	f, err := os.OpenFile(filepath.Join(dir, now.Format(time.DateOnly)+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewTextHandler(f, nil)), f.Close, nil
}

// pruneLogs 删除 dir 下日期早于保留期的日志文件。文件名不是“日期.log”的文件不动。
func pruneLogs(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	oldestKept := today.AddDate(0, 0, -(logKeepDays - 1))
	for _, e := range entries {
		day, err := time.ParseInLocation(time.DateOnly+".log", e.Name(), now.Location())
		if err != nil {
			continue
		}
		if day.Before(oldestKept) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
