package execution

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Innocent-children/taskbelay/internal/domain"
	"github.com/Innocent-children/taskbelay/internal/repository"
)

type Run struct {
	RunID             string        `json:"run_id"`
	TaskID            string        `json:"task_id"`
	CheckID           string        `json:"check_id"`
	CheckDigest       domain.Digest `json:"check_digest"`
	Check             domain.Check  `json:"check"`
	Before            Fingerprint   `json:"before"`
	After             *Fingerprint  `json:"after,omitempty"`
	Status            string        `json:"status"`
	ExitCode          *int          `json:"exit_code,omitempty"`
	DurationMS        int64         `json:"duration_ms"`
	Stdout            string        `json:"stdout"`
	Stderr            string        `json:"stderr"`
	StdoutDigest      domain.Digest `json:"stdout_digest"`
	StderrDigest      domain.Digest `json:"stderr_digest"`
	OutputBytes       int64         `json:"output_bytes"`
	Truncated         bool          `json:"truncated"`
	OwnerPID          int           `json:"owner_pid"`
	OwnerIdentity     string        `json:"owner_identity"`
	StartedAt         time.Time     `json:"started_at"`
	FinishedAt        *time.Time    `json:"finished_at,omitempty"`
	Reconciliation    string        `json:"reconciliation,omitempty"`
	ResolutionUserRef string        `json:"resolution_user_ref,omitempty"`
}

func Owner() (int, string, error) {
	pid := os.Getpid()
	id, e := processIdentity(pid)
	return pid, id, e
}
func OwnerAlive(r Run) bool {
	v, e := processIdentity(r.OwnerPID)
	return e == nil && v == r.OwnerIdentity
}
func (r Run) InputKey() domain.Digest {
	return domain.Hash([]any{r.CheckDigest, r.Before.Inputs, r.Before.Environment})
}

type output struct {
	mu       sync.Mutex
	h        hash.Hash
	prefix   []byte
	bytes    int64
	cancel   context.CancelFunc
	overflow bool
}

func newOutput(cancel context.CancelFunc) *output { return &output{h: sha256.New(), cancel: cancel} }
func (w *output) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bytes += int64(len(b))
	if w.bytes > 8*1024*1024 {
		w.overflow = true
		w.cancel()
	}
	w.h.Write(b)
	n := 65536 - len(w.prefix)
	if n > len(b) {
		n = len(b)
	}
	if n > 0 {
		w.prefix = append(w.prefix, b[:n]...)
	}
	return len(b), nil
}
func Perform(ctx context.Context, t domain.Task, r Run) Run {
	started := time.Now()
	timeout, cancel := context.WithTimeout(ctx, time.Duration(r.Check.TimeoutMS)*time.Millisecond)
	defer cancel()
	cwd, e := repository.SafePath(t.Repositories[0].Origin.CanonicalWorktreeRoot, ".")
	if entry, ok := t.Repository(r.Check.RepositoryKey); ok {
		cwd, e = repository.SafePath(entry.Origin.CanonicalWorktreeRoot, r.Check.Cwd)
	}
	if e != nil {
		r.Status = "failed"
		r.Stderr = "working directory is unavailable"
		return finish(r, started)
	}
	c := exec.Command(r.Before.Executable, r.Check.Argv[1:]...)
	c.Dir = cwd
	c.Env = os.Environ()
	configureProcess(c)
	out, errout := newOutput(cancel), newOutput(cancel)
	c.Stdout = out
	c.Stderr = errout
	if e = c.Start(); e != nil {
		r.Status = "failed"
		r.Stderr = "approved process could not start"
		return finish(r, started)
	}
	stop, close, e := bindProcess(c)
	if e != nil {
		c.Process.Kill()
		c.Wait()
		r.Status = "failed"
		r.Stderr = "process lifetime control could not be established"
		return finish(r, started)
	}
	defer close()
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case e = <-done:
	case <-timeout.Done():
		stop()
		e = <-done
	}
	r.Status = "passed"
	code := c.ProcessState.ExitCode()
	r.ExitCode = &code
	if e != nil {
		r.Status = "failed"
	}
	if timeout.Err() != nil {
		r.Status = "interrupted"
		if errors.Is(timeout.Err(), context.DeadlineExceeded) {
			r.Status = "timeout"
		}
	}
	if out.overflow || errout.overflow {
		r.Status = "output_limit"
	}
	r.Stdout = string(out.prefix)
	r.Stderr = string(errout.prefix)
	r.StdoutDigest = domain.Digest(hex.EncodeToString(out.h.Sum(nil)))
	r.StderrDigest = domain.Digest(hex.EncodeToString(errout.h.Sum(nil)))
	r.OutputBytes = out.bytes + errout.bytes
	r.Truncated = out.bytes > 65536 || errout.bytes > 65536
	after, e := Observe(context.WithoutCancel(ctx), t, r.Check)
	if e == nil {
		r.After = &after
		if after.Inputs != r.Before.Inputs || after.Environment != r.Before.Environment {
			r.Status = "invalidated"
		}
	} else if r.Status == "passed" {
		r.Status = "invalidated"
	}
	return finish(r, started)
}
func finish(r Run, start time.Time) Run {
	now := time.Now().UTC()
	r.FinishedAt = &now
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}

type receipt struct {
	Run       Run    `json:"run"`
	Signature string `json:"signature"`
}

func receiptKey(dir string) ([]byte, error) {
	p := filepath.Join(dir, "execution.key")
	b, e := os.ReadFile(p)
	if e == nil {
		if len(b) != 32 {
			return nil, domain.Fail("STORAGE_CORRUPT", "", "invalid execution receipt key")
		}
		return b, nil
	}
	if !os.IsNotExist(e) {
		return nil, e
	}
	b = make([]byte, 32)
	if _, e = rand.Read(b); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		return os.ReadFile(p)
	}
	if e != nil {
		return nil, e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return b, e
}
func signature(key, b []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func WriteReceipt(dir string, r Run) error {
	key, e := receiptKey(dir)
	if e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	b, e = json.Marshal(receipt{r, signature(key, b)})
	if e != nil {
		return e
	}
	root := filepath.Join(dir, "receipts")
	if e = os.MkdirAll(root, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(root, "receipt-")
	if e != nil {
		return e
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(temp, filepath.Join(root, r.RunID+".json"))
}
func ReadReceipt(dir string, pending Run) (Run, error) {
	if !domain.ID(pending.RunID).IsValid() {
		return Run{}, domain.Invalid("run_id", "invalid run identity")
	}
	b, e := os.ReadFile(filepath.Join(dir, "receipts", pending.RunID+".json"))
	if e != nil {
		return Run{}, e
	}
	if len(b) > 1024*1024 {
		return Run{}, domain.Fail("STORAGE_CORRUPT", "", "receipt exceeds its byte limit")
	}
	var v receipt
	if json.Unmarshal(b, &v) != nil {
		return Run{}, domain.Fail("STORAGE_CORRUPT", "", "invalid execution receipt")
	}
	key, e := receiptKey(dir)
	if e != nil {
		return Run{}, e
	}
	raw, _ := json.Marshal(v.Run)
	if !hmac.Equal([]byte(v.Signature), []byte(signature(key, raw))) || v.Run.RunID != pending.RunID || v.Run.TaskID != pending.TaskID || v.Run.CheckDigest != pending.CheckDigest || v.Run.Before != pending.Before || v.Run.FinishedAt == nil || v.Run.Status == "running" {
		return Run{}, domain.Fail("STORAGE_CORRUPT", "", "receipt identity or integrity does not match the reserved execution")
	}
	return v.Run, nil
}

// OwnedReceipt verifies that a compacted artifact was written by this Core data store.
func OwnedReceipt(dir, id string, b []byte) bool {
	if len(b) > 1024*1024 {
		return false
	}
	var v receipt
	if json.Unmarshal(b, &v) != nil || v.Run.RunID != id {
		return false
	}
	key, e := os.ReadFile(filepath.Join(dir, "execution.key"))
	if e != nil || len(key) != 32 {
		return false
	}
	raw, e := json.Marshal(v.Run)
	return e == nil && hmac.Equal([]byte(v.Signature), []byte(signature(key, raw)))
}
