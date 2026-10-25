package ledger

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestColdOpenWaitsForWALTransition(t *testing.T) {
	dir := t.TempDir()
	blocker, err := sql.Open("sqlite", filepath.Join(dir, "taskbelay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	if _, err := blocker.Exec("CREATE TABLE preserved(value TEXT); INSERT INTO preserved VALUES('existing')"); err != nil {
		t.Fatal(err)
	}
	tx, err := blocker.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var value string
	if err := tx.QueryRow("SELECT value FROM preserved").Scan(&value); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	start := time.Now()
	blocked, openErr := Open(ctx, dir, NormalWait)
	cancel()
	if openErr == nil {
		blocked.Close()
		t.Fatal("enabled WAL while the reader held its transaction")
	}
	if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
		t.Fatalf("initialization exceeded remaining event budget: %s", elapsed)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(25 * time.Millisecond)
		released <- tx.Rollback()
	}()
	s, openErr := Open(t.Context(), dir, NormalWait)
	if err := <-released; err != nil {
		t.Fatal(err)
	}
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer s.Close()
	if err := s.db.QueryRow("SELECT value FROM preserved").Scan(&value); err != nil || value != "existing" {
		t.Fatal(value, err)
	}
}

func TestWALReadAndBoundedBusyWrite(t *testing.T) {
	dir := t.TempDir()
	writer, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	fast, err := Open(t.Context(), dir, FastWait)
	if err != nil {
		t.Fatal(err)
	}
	defer fast.Close()
	normal, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer normal.Close()
	var mode string
	if err := writer.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q, err = %v", mode, err)
	}
	mustSet(t, writer, "", User, "workflow", "semi")
	tx, err := writer.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE settings SET value='auto' WHERE name='workflow'"); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, fast, "").Values["workflow"].Value; got != "semi" {
		t.Fatalf("reader saw uncommitted value %q", got)
	}
	for _, test := range []struct {
		name   string
		store  *Store
		budget time.Duration
	}{
		{"fast event", fast, time.Second},
		{"remaining event budget", normal, 25 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), test.budget)
			defer cancel()
			start := time.Now()
			err := test.store.SetSetting(ctx, "", User, "enforcement", "hint")
			if err == nil {
				t.Fatal("write succeeded while another process held the write lock")
			}
			if elapsed := time.Since(start); elapsed > 150*time.Millisecond {
				t.Fatalf("busy write exceeded fast/remaining budget: %s", elapsed)
			}
		})
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, fast, "").Values["enforcement"]; got.Value != "block" || got.Source != Default {
		t.Fatalf("failed transaction published settings: %+v", got)
	}
	mustSet(t, fast, "", User, "enforcement", "hint")
}

func TestLedgerSharedByProcesses(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared ledger")
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for _, setting := range []struct{ name, value string }{{"workflow", "semi"}, {"enforcement", "hint"}, {"analysis.tests", "on"}} {
		command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLedgerProcessHelper$")
		command.Env = append(os.Environ(), "TASKBELAY_TEST_DB="+dir, "TASKBELAY_TEST_KEY="+setting.name, "TASKBELAY_TEST_VALUE="+setting.value)
		output := new(bytes.Buffer)
		command.Stdout, command.Stderr = output, output
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		outputs = append(outputs, output)
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Errorf("child process: %v\n%s", err, outputs[i])
		}
	}
	reader, err := Open(t.Context(), dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	settings := readSettings(t, reader, "")
	for name, want := range map[string]string{"workflow": "semi", "enforcement": "hint", "analysis.tests": "on"} {
		if got := settings.Values[name].Value; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestLedgerProcessHelper(t *testing.T) {
	dir := os.Getenv("TASKBELAY_TEST_DB")
	if dir == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	store, err := Open(ctx, dir, NormalWait)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetSetting(ctx, "", User, os.Getenv("TASKBELAY_TEST_KEY"), os.Getenv("TASKBELAY_TEST_VALUE")); err != nil {
		t.Fatal(err)
	}
}
