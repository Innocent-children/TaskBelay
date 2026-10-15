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
	RequirementDigest domain.Digest    `json:"requirement_digest" description:"Digest of goals and constraints served by the execution."`
	Rationale         string           `json:"rationale" description:"这项/这次检查如何覆盖具体目标或补充当前缺口。"`
	NewInformation    string           `json:"new_information,omitempty" description:"New actual diagnostic information for an unchanged failed-check retry."`
	FullSuiteBasis    string           `json:"full_suite_basis,omitempty" description:"Required for a full suite: the actual gap targeted checks cannot cover."`
	RunID             string           `json:"run_id" description:"实际执行身份，来自原生check run；不重新构造执行来找结果。"`
	OperationID       string           `json:"operation_id" description:"稳定逻辑操作身份；同一已提交身份不能用于不同请求。"`
	TaskID            string           `json:"task_id" description:"唯一Core Task身份；不从标题、目录或临时会话ID猜测。"`
	CheckID           string           `json:"check_id" description:"当前已保存检查的稳定身份。"`
	CheckDigest       domain.Digest    `json:"check_digest" description:"Digest of the check, associated goals and relevant constraints."`
	Check             domain.Check     `json:"check" description:"当前检查定义及相关要求的摘要。"`
	Before            Fingerprint      `json:"before" description:"Actual fingerprints before execution."`
	After             *Fingerprint     `json:"after,omitempty" description:"Observed execution conditions after the process returned."`
	Status            string           `json:"status" description:"对象状态；Task、Run、工作区、安装结果各有独立枚举，见公共状态表。"`
	ExitCode          *int             `json:"exit_code,omitempty" description:"CLI进程或运行实际退出码；CLI的0不代替run.status。"`
	DurationMS        int64            `json:"duration_ms" description:"真实运行耗时毫秒；未知结果的0值不能解释为没有执行。"`
	Stdout            string           `json:"stdout" description:"实际标准输出文本；CLI展示中为空字符串表示没有输出。"`
	Stderr            string           `json:"stderr" description:"独立诊断文本；不与stdout合并后解析JSON。"`
	StdoutDigest      domain.Digest    `json:"stdout_digest" description:"Digest of complete stdout, including truncated bytes."`
	StderrDigest      domain.Digest    `json:"stderr_digest" description:"Digest of complete stderr, including truncated bytes."`
	OutputBytes       int64            `json:"output_bytes" description:"真实输出总字节数，详情通过get_run读取。"`
	Truncated         bool             `json:"truncated" description:"是否只返回输出前缀。"`
	OwnerPID          int              `json:"owner_pid" description:"Original execution owner PID, paired with startup identity."`
	OwnerIdentity     string           `json:"owner_identity" description:"Original Core process startup identity, not merely a PID."`
	Process           *ProcessIdentity `json:"process" description:"实际进程身份；执行尚未激活或不公开此信息时为null。"`
	StartedAt         time.Time        `json:"started_at" description:"实际开始时间，UTC RFC3339。"`
	FinishedAt        *time.Time       `json:"finished_at,omitempty" description:"实际结束时间，UTC；尚未结束或未知时按类型为null。"`
	Reconciliation    string           `json:"reconciliation,omitempty" description:"未知运行的副作用检查说明；没有核对记录时为null。"`
	ResolutionUserRef string           `json:"resolution_user_ref,omitempty" description:"Actual decision for reconciling a non-repeatable run; empty when unnecessary."`
}

type ProcessIdentity struct {
	PID      int    `json:"pid" description:"实际进程ID，必须结合启动身份，不能单独判定进程仍为原命令。"`
	Identity string `json:"identity" description:"OS process startup identity paired with pid; a reused PID does not match this process."`
}

func ProcessActive(ctx context.Context, process *ProcessIdentity) (bool, error) {
	if process == nil {
		return false, nil
	}
	return processActive(ctx, *process)
}

func Owner() (int, string, error) {
	pid := os.Getpid()
	id, e := processIdentity(pid)
	return pid, id, e
}
func OwnerStatus(r Run) (bool, error) {
	v, e := processIdentity(r.OwnerPID)
	if errors.Is(e, errProcessEnded) {
		return false, nil
	}
	return v == r.OwnerIdentity, e
}
func OwnerAlive(r Run) bool { alive, e := OwnerStatus(r); return e != nil || alive }

var errProcessEnded = errors.New("process ended")

func (r Run) InputKey() domain.Digest {
	// Names, prose and timeout changes do not turn the same execution into new information.
	return domain.Hash([]any{r.Check.Argv, r.RequirementDigest, r.Before.Inputs, r.Before.Environment})
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
func Perform(ctx context.Context, t domain.Task, r Run, recordStarted func(context.Context, Run) error) Run {
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
	out, errout := newOutput(cancel), newOutput(cancel)
	c.Stdout = out
	c.Stderr = errout
	stop, close, e := startProcess(c, func(process ProcessIdentity) error {
		started := r
		started.Process = &process
		if e := recordStarted(timeout, started); e != nil {
			return e
		}
		r = started
		return nil
	})
	if e != nil {
		if c.Process != nil {
			c.Process.Kill()
			c.Wait()
		}
		r.Status = "failed"
		r.Stderr = "process identity could not be saved and activated"
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
	Run       Run    `json:"run" description:"本次实际或恢复运行的摘要，必须检查status。"`
	Signature string `json:"signature" description:"Receipt integrity signature; cannot be submitted to manufacture execution."`
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
	if !hmac.Equal([]byte(v.Signature), []byte(signature(key, raw))) || v.Run.RunID != pending.RunID || v.Run.TaskID != pending.TaskID || v.Run.CheckDigest != pending.CheckDigest || v.Run.Before != pending.Before || domain.Hash(v.Run.Process) != domain.Hash(pending.Process) || v.Run.FinishedAt == nil || v.Run.Status == "running" {
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
