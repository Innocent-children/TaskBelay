package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Innocent-children/taskbelay/internal/platform"
)

func TestDailyLogsRetainSevenDaysAndStatusWorksDuringDatabaseFailure(t *testing.T) {
	env := platform.Env{DataDir: t.TempDir()}
	if err := logError(env, errors.New("已采集但数据库无法记录的错误")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, days := range []int{6, 7, 30} {
		name := filepath.Join(env.DataDir, "logs", now.AddDate(0, 0, -days).Format("2006-01-02")+".log")
		if err := os.WriteFile(name, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	unknown := filepath.Join(env.DataDir, "logs", "user-notes.txt")
	if err := os.WriteFile(unknown, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := pruneLogs(t.Context(), env, now); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{6, 7, 30} {
		_, err := os.Stat(filepath.Join(env.DataDir, "logs", now.AddDate(0, 0, -days).Format("2006-01-02")+".log"))
		if days == 6 && err != nil || days > 6 && !os.IsNotExist(err) {
			t.Fatal(days, err)
		}
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatal("unrelated file removed", err)
	}
	dir := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	if err := os.WriteFile(filepath.Join(env.DataDir, "taskbelay.db"), []byte("not a database"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := runStatus(t.Context(), env, dir, &output); err == nil || !strings.Contains(output.String(), "状态未完成") || !strings.Contains(output.String(), "已采集但数据库无法记录的错误") {
		t.Fatal(output.String(), err)
	}
}

func TestHitsArgumentsAndEmptyExport(t *testing.T) {
	env := platform.Env{DataDir: t.TempDir()}
	dir := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	for _, args := range [][]string{{"--since", "0d"}, {"--since", "-1d"}, {"--since", "14h"}, {"--since"}, {"--wrong", "14d"}} {
		var output bytes.Buffer
		if err := runHits(t.Context(), args, env, dir, &output); err == nil {
			t.Fatal(args)
		}
	}
	var output bytes.Buffer
	if err := run(t.Context(), []string{"hits", "--since", "14d"}, env, dir, &output); err != nil || output.Len() != 0 {
		t.Fatal(output.String(), err)
	}
}
